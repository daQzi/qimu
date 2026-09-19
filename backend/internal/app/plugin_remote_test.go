package app

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"gorm.io/gorm"
	"infinite-canvas/backend/internal/model"
	"infinite-canvas/backend/internal/plugins"
	"infinite-canvas/backend/internal/plugins/contracts"
)

func p04Archive(t *testing.T, change func(map[string][]byte)) []byte {
	t.Helper()
	root := "../plugins/contracts/testdata/remote-helper-p04"
	files := map[string][]byte{}
	if err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		raw, err := os.ReadFile(path)
		files[strings.TrimPrefix(filepath.ToSlash(path), root+"/")] = raw
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if change != nil {
		change(files)
	}
	var buffer bytes.Buffer
	w := zip.NewWriter(&buffer)
	for p, raw := range files {
		f, err := w.Create(p)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = f.Write(raw); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}
func p04Fixture(t *testing.T, driver, baseURL string, change func(map[string][]byte)) (*Service, *gorm.DB, string) {
	t.Helper()
	s, db, _ := p03Fixture(t, driver)
	if driver == "postgres" {
		t.Setenv("REDIS_URL", os.Getenv("CANVAS_TEST_REDIS_URL"))
	} else {
		t.Setenv("REDIS_URL", "")
	}
	s = New(s.repo, s.dataDir)
	t.Cleanup(func() { _ = s.Close() })
	if s.runtimeErr != nil {
		t.Fatal(s.runtimeErr)
	}
	if _, err := s.InstallManagedPluginForAdmin(&model.User{ID: "admin", Role: model.UserRoleAdmin}, p04Archive(t, change), "remote.yingce-plugin"); err != nil {
		t.Fatal(err)
	}
	release, err := s.repo.PluginReleaseByVersion("remote-helper", "1.0.0")
	if err != nil || release == nil {
		t.Fatal(err)
	}
	if err = s.ActivateApplicationPlugin(&model.User{ID: "user", Role: model.UserRoleUser}, "remote-helper", ApplicationPluginActivation{ReleaseID: release.ID, Enabled: true, GrantedPermissions: []string{"connection.use", "resource.create"}}); err != nil {
		t.Fatal(err)
	}
	if _, err = s.SaveApplicationPluginConnection("user", PluginConnectionInput{PluginID: "remote-helper", ConnectorID: "api", Name: "Test API", BaseURL: baseURL, Credential: "test-only-plugin-secret", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	return s, db, release.ID
}
func p04Request(release, op string) contracts.Invocation {
	return contracts.Invocation{Operation: "remote-helper." + op, ReleaseID: release, Input: p03Input(map[string]any{"prompt": "test"})}
}
func p04Approve(t *testing.T, s *Service, release, op string) plugins.RunView {
	t.Helper()
	out, err := s.InvokePluginOperation("user", newID(), p04Request(release, op))
	if err != nil {
		t.Fatal(err)
	}
	view, err := s.DecidePluginRun("user", out.RunID, out.ApprovalID, "approve", out.Revision)
	if err != nil {
		t.Fatal(err)
	}
	if view.TaskID == nil || view.Status != "running" {
		t.Fatalf("not queued: %+v", view)
	}
	if _, err = s.DecidePluginRun("user", out.RunID, out.ApprovalID, "approve", out.Revision); err != nil {
		t.Fatal("approval replay", err)
	}
	return view
}
func p04Tick(t *testing.T, s *Service, db *gorm.DB) {
	t.Helper()
	past := time.Now().Add(-time.Hour)
	if err := db.Model(&model.Task{}).Where("type=?", model.TaskTypePluginOperation).Update("next_poll_at", past).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&model.PluginConnectionRate{}).Where("1=1").Update("next_request_at", past).Error; err != nil {
		t.Fatal(err)
	}
	if err := s.ProcessNextTask(); err != nil {
		t.Fatal(err)
	}
}
func p04ConnectorChange(files map[string][]byte, edit func(map[string]any)) {
	var doc map[string]any
	_ = json.Unmarshal(files["connectors/api.json"], &doc)
	edit(doc)
	files["connectors/api.json"], _ = json.Marshal(doc)
}

func TestP04RemoteLifecycleAcrossDatabases(t *testing.T) {
	t.Setenv("CANVAS_ALLOWED_PRIVATE_UPSTREAM_HOSTS", "127.0.0.1")
	for _, driver := range []string{"sqlite", "postgres"} {
		t.Run(driver, func(t *testing.T) {
			var submitted atomic.Int32
			var pngBytes bytes.Buffer
			_ = png.Encode(&pngBytes, image.NewRGBA(image.Rect(0, 0, 2, 2)))
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.URL.Path == "/image" {
					w.Header().Set("Content-Type", "image/png")
					w.Write(pngBytes.Bytes())
					return
				}
				if r.Header.Get("Authorization") != "Bearer test-only-plugin-secret" {
					http.Error(w, "wrong auth", 401)
					return
				}
				if r.URL.Path == "/jobs" {
					submitted.Add(1)
					if r.Header.Get("Idempotency-Key") == "" {
						t.Error("missing upstream key")
					}
					fmt.Fprint(w, `{"id":"job1","status":"queued"}`)
					return
				}
				fmt.Fprintf(w, `{"id":"job1","status":"completed","output":{"message":"done","url":"http://%s/image"}}`, r.Host)
			}))
			defer server.Close()
			s, db, release := p04Fixture(t, driver, server.URL, nil)
			if err := db.Create(&model.CreditAccount{UserID: "user", AvailableMicrocredits: 1000}).Error; err != nil {
				t.Fatal(err)
			}
			if _, err := s.SavePluginOperationPrice(&model.User{ID: "admin", Role: model.UserRoleAdmin}, PluginOperationPriceInput{ReleaseID: release, OperationID: "preview", FeeMicrocredits: 100}); err != nil {
				t.Fatal(err)
			}
			view := p04Approve(t, s, release, "preview")
			if submitted.Load() != 0 {
				t.Fatal("network before worker")
			}
			p04Tick(t, s, db)
			p04Tick(t, s, db)
			result, err := s.PluginRun("user", view.ID)
			if err != nil || result.Status != "succeeded" || result.ResultRef == nil {
				t.Fatalf("result=%+v err=%v", result, err)
			}
			if submitted.Load() != 1 {
				t.Fatal("duplicate submit")
			}
			var account model.CreditAccount
			db.First(&account, "user_id=?", "user")
			if account.AvailableMicrocredits != 900 || account.ReservedMicrocredits != 0 {
				t.Fatalf("ledger %+v", account)
			}
			var refs []model.PluginRunResource
			db.Where("run_id=? AND role=?", view.ID, "output").Find(&refs)
			if len(refs) != 1 {
				t.Fatal("missing output reference")
			}
			if err = s.repo.RequireNoRetainedResourceReferences([]string{refs[0].ResourceID}); err == nil {
				t.Fatal("unprotected imported result")
			}
			raw, _ := json.Marshal(result)
			if bytes.Contains(raw, []byte("test-only-plugin-secret")) || bytes.Contains(raw, []byte("http://")) {
				t.Fatal("credential or remote address leaked")
			}
			if _, err = s.PluginRun("other", view.ID); err == nil {
				t.Fatal("foreign result readable")
			}
			if _, err = s.CreateTask("user", CreateTaskRequest{Type: model.TaskTypePluginOperation, Prompt: "forged"}); err == nil {
				t.Fatal("public task API bypassed approval")
			}
		})
	}
}

func TestP04UnknownSubmissionAndFencing(t *testing.T) {
	t.Setenv("CANVAS_ALLOWED_PRIVATE_UPSTREAM_HOSTS", "127.0.0.1")
	var submits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/echo" {
			submits.Add(1)
			conn, _, _ := w.(http.Hijacker).Hijack()
			conn.Close()
			return
		}
		fmt.Fprint(w, `{"message":"ok"}`)
	}))
	defer server.Close()
	s, db, release := p04Fixture(t, "sqlite", server.URL, nil)
	view := p04Approve(t, s, release, "echo")
	p04Tick(t, s, db)
	p04Tick(t, s, db)
	run, err := s.PluginRun("user", view.ID)
	if err != nil || run.Status != "paused" || run.Remote.State != "unknown" || submits.Load() != 1 {
		t.Fatalf("unsafe unknown behavior %+v %v", run, err)
	}
	if _, err = s.ResumePluginRun("user", run.ID, run.Revision, "retry_safe", ""); err == nil {
		t.Fatal("unsafe retry admitted")
	}
	if _, err = s.RetryTask("user", *view.TaskID); err == nil {
		t.Fatal("ordinary task retry bypass")
	}
	if _, err = s.ResumePluginRun("user", run.ID, run.Revision, "confirm_new_attempt", ""); err == nil {
		t.Fatal("new attempt bypassed new quote")
	}
	// A stale lease cannot publish a late response after another worker claims it.
	second := p04Approve(t, s, release, "echo")
	old, err := s.repo.ClaimNextTask("old", time.Second)
	if err != nil || old == nil {
		t.Fatal(err)
	}
	if err = db.Model(&model.Task{}).Where("id=?", old.ID).Update("lease_expires_at", time.Now().Add(-time.Second)).Error; err != nil {
		t.Fatal(err)
	}
	current, err := s.repo.ClaimNextTask("new", time.Minute)
	if err != nil || current == nil || current.ID != *second.TaskID {
		t.Fatal(err)
	}
	if err = s.storePluginResponse(*old, json.RawMessage(`{"message":"late"}`), "upstream_completed", ""); err == nil {
		t.Fatal("stale worker published response")
	}
}

func TestP04CancellationNeedsConfirmation(t *testing.T) {
	t.Setenv("CANVAS_ALLOWED_PRIVATE_UPSTREAM_HOSTS", "127.0.0.1")
	var cancelled atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/jobs":
			fmt.Fprint(w, `{"id":"job1","status":"queued"}`)
		case "/jobs/job1/cancel":
			cancelled.Store(true)
			fmt.Fprint(w, `{"status":"running"}`)
		default:
			if cancelled.Load() {
				fmt.Fprint(w, `{"status":"cancelled"}`)
			} else {
				fmt.Fprint(w, `{"status":"running"}`)
			}
		}
	}))
	defer server.Close()
	s, db, release := p04Fixture(t, "sqlite", server.URL, nil)
	view := p04Approve(t, s, release, "preview")
	p04Tick(t, s, db)
	view, _ = s.PluginRun("user", view.ID)
	if _, err := s.CancelPluginRun("user", view.ID, view.Revision); err != nil {
		t.Fatal(err)
	}
	p04Tick(t, s, db)
	view, _ = s.PluginRun("user", view.ID)
	if view.Status != "cancelling" {
		t.Fatalf("ack mistaken for cancellation: %+v", view)
	}
	p04Tick(t, s, db)
	view, _ = s.PluginRun("user", view.ID)
	if view.Status != "cancelled" || view.Remote.CancelStatus != "confirmed" {
		t.Fatalf("cancel not confirmed %+v", view)
	}
}

func TestP04SafeRecoveryAndExpiredImport(t *testing.T) {
	t.Setenv("CANVAS_ALLOWED_PRIVATE_UPSTREAM_HOSTS", "127.0.0.1")
	var submits, polls atomic.Int32
	var pngBytes bytes.Buffer
	_ = png.Encode(&pngBytes, image.NewRGBA(image.Rect(0, 0, 2, 2)))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/jobs":
			submits.Add(1)
			conn, _, _ := w.(http.Hijacker).Hijack()
			conn.Close()
		case "/expired":
			http.Error(w, "expired private signature", 403)
		case "/image":
			w.Write(pngBytes.Bytes())
		default:
			if strings.HasPrefix(r.URL.Path, "/requests/") {
				fmt.Fprint(w, `{"id":"job1","status":"queued"}`)
				return
			}
			path := "expired"
			if polls.Add(1) > 1 {
				path = "image"
			}
			fmt.Fprintf(w, `{"status":"completed","output":{"message":"done","url":"http://%s/%s"}}`, r.Host, path)
		}
	}))
	defer server.Close()
	s, db, release := p04Fixture(t, "sqlite", server.URL, nil)
	view := p04Approve(t, s, release, "preview")
	for i := 0; i < 4; i++ {
		p04Tick(t, s, db)
	}
	view, err := s.PluginRun("user", view.ID)
	if err != nil || view.Status != "succeeded" || submits.Load() != 1 || polls.Load() != 2 {
		t.Fatalf("recovery %+v submits=%d polls=%d err=%v", view, submits.Load(), polls.Load(), err)
	}
}

func TestP04ConnectionVersionQuoteAndIsolation(t *testing.T) {
	t.Setenv("CANVAS_ALLOWED_PRIVATE_UPSTREAM_HOSTS", "127.0.0.1")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, `{"message":"ok"}`) }))
	defer server.Close()
	s, db, release := p04Fixture(t, "sqlite", server.URL, nil)
	views, err := s.ApplicationPluginConnections("user")
	if err != nil || len(views) != 1 {
		t.Fatal(err)
	}
	var versions []model.PluginConnectionVersion
	db.Find(&versions)
	if len(versions) != 1 || strings.Contains(versions[0].SecretCipher, "test-only-plugin-secret") {
		t.Fatal("plaintext credential stored")
	}
	other, err := s.ApplicationPluginConnections("other")
	if err != nil || len(other) != 0 {
		t.Fatal("foreign connection exposed", err)
	}
	out, err := s.InvokePluginOperation("user", "quote-change", p04Request(release, "echo"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.SavePluginOperationPrice(&model.User{ID: "admin", Role: model.UserRoleAdmin}, PluginOperationPriceInput{ReleaseID: release, OperationID: "echo", FeeMicrocredits: 1}); err != nil {
		t.Fatal(err)
	}
	if _, err = s.DecidePluginRun("user", out.RunID, out.ApprovalID, "approve", out.Revision); err == nil {
		t.Fatal("stale quote approved")
	}
	if _, err = s.SaveApplicationPluginConnection("user", PluginConnectionInput{PluginID: "remote-helper", ConnectorID: "api", BaseURL: "http://127.0.0.1:1", Revision: 1, Enabled: true}); err == nil {
		t.Fatal("secret silently sent to new target")
	}
	if _, err = s.SavePluginOperationPrice(&model.User{ID: "other", Role: model.UserRoleUser}, PluginOperationPriceInput{ReleaseID: release, OperationID: "echo", FeeMicrocredits: 0}); err == nil {
		t.Fatal("user could change platform fee")
	}
}

func TestP04LateResponseCannotUndoCancellation(t *testing.T) {
	t.Setenv("CANVAS_ALLOWED_PRIVATE_UPSTREAM_HOSTS", "127.0.0.1")
	entered, release := make(chan struct{}), make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(entered)
		<-release
		fmt.Fprint(w, `{"message":"late"}`)
	}))
	defer server.Close()
	s, _, version := p04Fixture(t, "sqlite", server.URL, nil)
	run := p04Approve(t, s, version, "echo")
	done := make(chan error, 1)
	go func() { done <- s.ProcessNextTask() }()
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		close(release)
		t.Fatal("worker did not submit")
	}
	current, err := s.PluginRun("user", run.ID)
	if err != nil {
		close(release)
		t.Fatal(err)
	}
	if _, err = s.CancelPluginRun("user", run.ID, current.Revision); err != nil {
		close(release)
		t.Fatal(err)
	}
	close(release)
	if err = <-done; err != nil {
		t.Fatal(err)
	}
	current, err = s.PluginRun("user", run.ID)
	if err != nil || current.Status != "cancelled" || current.ResultRef != nil || current.Remote.CancelStatus != "completed_after_cancel" {
		t.Fatalf("late success leaked: %+v %v", current, err)
	}
}

func TestP04AgentBudgetIncludesExistingModelCharge(t *testing.T) {
	t.Setenv("CANVAS_ALLOWED_PRIVATE_UPSTREAM_HOSTS", "127.0.0.1")
	server := httptest.NewServer(http.NotFoundHandler())
	defer server.Close()
	s, db, _ := p02Fixture(t)
	if _, err := s.InstallManagedPluginForAdmin(&model.User{ID: "admin", Role: model.UserRoleAdmin}, p04Archive(t, nil), "remote.yingce-plugin"); err != nil {
		t.Fatal(err)
	}
	r, err := s.repo.PluginReleaseByVersion("remote-helper", "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if err = s.ActivateApplicationPlugin(&model.User{ID: "user", Role: model.UserRoleUser}, "remote-helper", ApplicationPluginActivation{ReleaseID: r.ID, Enabled: true, GrantedPermissions: []string{"connection.use", "resource.create"}}); err != nil {
		t.Fatal(err)
	}
	if _, err = s.SaveApplicationPluginConnection("user", PluginConnectionInput{PluginID: "remote-helper", ConnectorID: "api", BaseURL: server.URL, Credential: "only-test-key", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	if _, err = s.SavePluginOperationPrice(&model.User{ID: "admin", Role: model.UserRoleAdmin}, PluginOperationPriceInput{ReleaseID: r.ID, OperationID: "echo", FeeMicrocredits: 100}); err != nil {
		t.Fatal(err)
	}
	if err = db.Create(&model.CanvasProject{ID: "agent-canvas", UserID: "user", PayloadJSON: `{"nodes":[]}`}).Error; err != nil {
		t.Fatal(err)
	}
	req := agentTestRequest()
	req.PermissionMode = "request_approval"
	req.Budget.MaxCredits = 0.00015
	root, err := s.CreateCloudAgentRun("user", req, "")
	if err != nil {
		t.Fatal(err)
	}
	agent, err := s.repo.CloudAgent("user", root.ID)
	if err != nil {
		t.Fatal(err)
	}
	out, err := s.applicationPlugins().Invoke("user", "agent-budget-quote", p04Request(r.ID, "echo"), plugins.InvocationPolicy{PermissionMode: "request_approval", AgentRunID: root.ID, AgentRevision: agent.Revision})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.DecidePluginRun("user", out.RunID, out.ApprovalID, "approve", out.Revision); err == nil {
		t.Fatal("model + plugin exceeded Agent budget")
	}
	var count int64
	db.Model(&model.Task{}).Where("type=?", model.TaskTypePluginOperation).Count(&count)
	if count != 0 {
		t.Fatal("unfunded task queued")
	}
}
