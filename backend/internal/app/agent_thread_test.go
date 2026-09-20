package app

import (
	"encoding/json"
	"sync"
	"testing"

	"gorm.io/gorm"
	"infinite-canvas/backend/internal/model"
)

func threadTestRequest() CloudAgentRequest {
	req := agentTestRequest()
	req.CanvasID = ""
	req.ContextScope = []string{}
	req.HostSurface = "agent-home"
	return req
}
func requireThreadStatus(t *testing.T, err error, status int) {
	t.Helper()
	if e, ok := err.(*AppError); !ok || e.Status != status {
		t.Fatalf("expected %d, got %v", status, err)
	}
}

func TestAgentThreadAtomicAdmissionReplayAndIsolation(t *testing.T) {
	s, db, _, _ := creationTestService(t)
	thread, err := s.CreateAgentThread("user", AgentThreadCreate{ClientKey: "thread-test-key"})
	if err != nil {
		t.Fatal(err)
	}
	same, err := s.CreateAgentThread("user", AgentThreadCreate{ClientKey: "thread-test-key"})
	if err != nil || same.ID != thread.ID {
		t.Fatalf("create retry %v", err)
	}
	req := AgentThreadMessage{Revision: thread.Revision, Request: threadTestRequest()}
	// A failed receipt write must roll back both task admission and reservation.
	if err = db.Callback().Create().Before("gorm:create").Register("thread-receipt-fail", func(tx *gorm.DB) {
		if tx.Statement.Table == "agent_thread_entries" {
			tx.AddError(gorm.ErrInvalidData)
		}
	}); err != nil {
		t.Fatal(err)
	}
	if _, err = s.AppendAgentThreadMessage("user", thread.ID, req); err == nil {
		t.Fatal("receipt failure accepted")
	}
	db.Callback().Create().Remove("thread-receipt-fail")
	var count int64
	db.Model(&model.Task{}).Where("operation = ?", cloudAgentOperation).Count(&count)
	if count != 0 {
		t.Fatal("orphan task after rollback")
	}
	var account model.CreditAccount
	db.First(&account, "user_id = ?", "user")
	if account.AvailableMicrocredits != 10000 {
		t.Fatal("fee escaped rollback")
	}
	first, err := s.AppendAgentThreadMessage("user", thread.ID, req)
	if err != nil {
		t.Fatal(err)
	}
	// Retry precedes current profile resolution and current context validation.
	if _, err = s.UpdateCloudAgentProfile("user", AgentProfileRequest{Scope: "user", Content: "新的偏好"}); err != nil {
		t.Fatal(err)
	}
	replay, err := s.AppendAgentThreadMessage("user", thread.ID, req)
	if err != nil || replay.Run.ID != first.Run.ID {
		t.Fatalf("replay %v", err)
	}
	_, err = s.GetAgentThread("other", thread.ID, 0)
	requireThreadStatus(t, err, 404)
	_, err = s.AppendAgentThreadMessage("other", thread.ID, req)
	requireThreadStatus(t, err, 404)
	changed := req
	changed.Request.Prompt = "不同消息"
	_, err = s.AppendAgentThreadMessage("user", thread.ID, changed)
	requireThreadStatus(t, err, 409)
	changed.Request.IdempotencyKey = "next-thread-message"
	changed.Revision = first.Thread.Revision
	_, err = s.AppendAgentThreadMessage("user", thread.ID, changed)
	requireThreadStatus(t, err, 409)
	db.Model(&model.Task{}).Where("operation = ?", cloudAgentOperation).Count(&count)
	if count != 1 {
		t.Fatal("duplicate task")
	}
	view, err := s.GetAgentThread("user", thread.ID, 0)
	if err != nil || len(view.Entries) != 1 || view.Entries[0].Run.ID != first.Run.ID {
		t.Fatalf("durable view %v", err)
	}
}

func TestAgentThreadBindingFreezesHistoricalContext(t *testing.T) {
	s, db, creationID, _ := creationTestService(t)
	for _, canvas := range []model.CanvasProject{{ID: "agent-canvas", UserID: "user", PayloadJSON: `{"nodes":[]}`}, {ID: "foreign", UserID: "other", PayloadJSON: `{"nodes":[]}`}} {
		if err := db.Create(&canvas).Error; err != nil {
			t.Fatal(err)
		}
	}
	thread, err := s.CreateAgentThread("user", AgentThreadCreate{ClientKey: "binding-thread"})
	if err != nil {
		t.Fatal(err)
	}
	first, err := s.AppendAgentThreadMessage("user", thread.ID, AgentThreadMessage{Revision: 1, Request: threadTestRequest()})
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.BindAgentThread("user", thread.ID, AgentThreadBinding{Revision: first.Thread.Revision, CanvasID: "foreign"})
	requireThreadStatus(t, err, 404)
	bound, err := s.BindAgentThread("user", thread.ID, AgentThreadBinding{Revision: first.Thread.Revision, CanvasID: "agent-canvas"})
	if err != nil {
		t.Fatal(err)
	}
	listed, err := s.ListAgentThreads("user", "agent-canvas", 0)
	if err != nil || len(listed) != 1 || listed[0].ID != thread.ID {
		t.Fatalf("canvas thread index: %v %v", listed, err)
	}
	view, err := s.GetAgentThread("user", thread.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if view.Entries[0].Context.CanvasID != "" || view.Entries[0].ContextRevision != 1 {
		t.Fatal("binding rewrote historical context")
	}
	if err = db.Model(&model.Task{}).Where("id = ?", first.Run.ID).Updates(map[string]any{"status": model.TaskStatusSucceeded, "result_json": `{"text":"done"}`}).Error; err != nil {
		t.Fatal(err)
	}
	if err = db.Model(&model.CloudAgentExecution{}).Where("id = ?", first.Run.ID).Update("status", "completed").Error; err != nil {
		t.Fatal(err)
	}
	req := agentTestRequest()
	req.IdempotencyKey = "thread-on-canvas"
	second, err := s.AppendAgentThreadMessage("user", thread.ID, AgentThreadMessage{Revision: bound.Revision, Request: req})
	if err != nil {
		t.Fatal(err)
	}
	if second.Run.ParentID != first.Run.ID || second.Run.CanvasID != "agent-canvas" {
		t.Fatal("lost cross-surface continuation")
	}
	req.ThreadID = thread.ID
	_, err = s.CreateCloudAgentRun("user", req, first.Run.ID)
	requireThreadStatus(t, err, 400)
	ref := AgentThreadReference{Revision: second.Thread.Revision, ClientKey: "creation-link", Kind: "creation", RunID: creationID}
	linked, err := s.AppendAgentThreadReference("user", thread.ID, ref)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.AppendAgentThreadReference("user", thread.ID, ref); err != nil {
		t.Fatal(err)
	}
	foreign, _ := s.CreateAgentThread("other", AgentThreadCreate{ClientKey: "other-thread"})
	ref.Revision = foreign.Revision
	_, err = s.AppendAgentThreadReference("other", foreign.ID, ref)
	requireThreadStatus(t, err, 404)
	view, err = s.GetAgentThread("user", thread.ID, 0)
	if err != nil || len(view.Entries) != 3 || view.Thread.Revision != linked.Revision {
		t.Fatalf("reference %v", err)
	}
	b, _ := json.Marshal(view)
	var raw map[string]any
	if err = json.Unmarshal(b, &raw); err != nil {
		t.Fatal(err)
	}
}

func TestAgentThreadConcurrentTabs(t *testing.T) {
	s, db, _, _ := creationTestService(t)
	thread, err := s.CreateAgentThread("user", AgentThreadCreate{ClientKey: "concurrent-thread"})
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for _, key := range []string{"concurrent-turn-a", "concurrent-turn-b"} {
		wg.Add(1)
		go func(key string) {
			defer wg.Done()
			req := threadTestRequest()
			req.IdempotencyKey = key
			_, err := s.AppendAgentThreadMessage("user", thread.ID, AgentThreadMessage{Revision: 1, Request: req})
			results <- err
		}(key)
	}
	wg.Wait()
	close(results)
	success := 0
	for err := range results {
		if err == nil {
			success++
		} else {
			requireThreadStatus(t, err, 409)
		}
	}
	if success != 1 {
		t.Fatalf("accepted %d tabs", success)
	}
	var count int64
	db.Model(&model.Task{}).Where("operation = ?", cloudAgentOperation).Count(&count)
	if count != 1 {
		t.Fatal("duplicate chargeable task")
	}
}
