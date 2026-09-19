package app

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"gorm.io/gorm"
	"infinite-canvas/backend/internal/model"
	"infinite-canvas/backend/internal/plugins"
	"infinite-canvas/backend/internal/plugins/contracts"
	"infinite-canvas/backend/internal/repository"
)

func p05Fixture(t *testing.T, driver string, handler http.HandlerFunc) (*Service, *gorm.DB, string, *atomic.Int32) {
	t.Helper()
	t.Setenv("CANVAS_ALLOWED_PRIVATE_UPSTREAM_HOSTS", "127.0.0.1")
	counter := &atomic.Int32{}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		counter.Add(1)
		if handler != nil {
			handler(w, r)
		} else {
			w.Write([]byte(`{"message":"processed over HTTP"}`))
		}
	}))
	t.Cleanup(upstream.Close)
	s, db, _ := p03Fixture(t, driver)
	if driver == "postgres" {
		t.Setenv("REDIS_URL", os.Getenv("CANVAS_TEST_REDIS_URL"))
	} else {
		t.Setenv("REDIS_URL", "")
	}
	s = New(s.repo, s.dataDir)
	t.Cleanup(func() { s.Close() })
	root := "../plugins/contracts/testdata/pipeline-helper-p05"
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		entry, err := w.Create(strings.TrimPrefix(filepath.ToSlash(path), root+"/"))
		if err != nil {
			return err
		}
		_, err = entry.Write(raw)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if err = w.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err = s.InstallManagedPluginForAdmin(&model.User{ID: "admin", Role: model.UserRoleAdmin}, buf.Bytes(), "pipeline.yingce-plugin"); err != nil {
		t.Fatal(err)
	}
	release, err := s.repo.PluginReleaseByVersion("pipeline-helper", "1.0.0")
	if err != nil || release == nil {
		t.Fatal(err)
	}
	if err = s.ActivateApplicationPlugin(&model.User{ID: "user", Role: model.UserRoleUser}, "pipeline-helper", ApplicationPluginActivation{ReleaseID: release.ID, Enabled: true, GrantedPermissions: []string{"media.read", "connection.use"}}); err != nil {
		t.Fatal(err)
	}
	if _, err = s.SaveApplicationPluginConnection("user", PluginConnectionInput{PluginID: "pipeline-helper", ConnectorID: "api", Name: "test", BaseURL: upstream.URL, Credential: "test-secret", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	return s, db, release.ID, counter
}
func p05Start(t *testing.T, s *Service, release string) plugins.RunView {
	t.Helper()
	req := contracts.Invocation{Operation: "pipeline-helper.process", ReleaseID: release, Input: p03Input(map[string]any{"resourceId": "video-one"})}
	out, err := s.InvokePluginOperation("user", newID(), req)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err = s.applicationPlugins().AdvancePipeline("user", out.RunID); err != nil {
			t.Fatal(err)
		}
	}
	view, err := s.PluginRun("user", out.RunID)
	if err != nil || view.Status != "waiting_input" {
		t.Fatalf("view=%+v err=%v", view, err)
	}
	return view
}
func p05Submit(t *testing.T, s *Service, view plugins.RunView) plugins.RunView {
	t.Helper()
	input := view.Pipeline.Inputs[0]
	value := json.RawMessage(`{"prompt":"user choice"}`)
	next, err := s.UpdatePluginInput("user", view.ID, input.ID, "submit-original", PluginInputUpdate{Revision: input.Revision, Mode: "submit", Value: value})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err = s.applicationPlugins().AdvancePipeline("user", view.ID); err != nil {
			t.Fatal(err)
		}
	}
	next, err = s.PluginRun("user", view.ID)
	if err != nil {
		t.Fatal(err)
	}
	return next
}

func TestP05RestartInputAndRemoteLifecycle(t *testing.T) {
	for _, driver := range []string{"sqlite", "postgres"} {
		t.Run(driver, func(t *testing.T) {
			s, db, release, calls := p05Fixture(t, driver, nil)
			view := p05Start(t, s, release)
			input := view.Pipeline.Inputs[0]
			if _, err := s.UpdatePluginInput("other", view.ID, input.ID, "foreign-submit", PluginInputUpdate{Revision: 1, Mode: "submit", Value: json.RawMessage(`{"prompt":"bad"}`)}); err == nil {
				t.Fatal("foreign input")
			}
			if _, err := s.UpdatePluginInput("user", view.ID, input.ID, "missing-fields", PluginInputUpdate{Revision: 1, Mode: "submit", Value: json.RawMessage(`{}`)}); err == nil {
				t.Fatal("missing fields advanced")
			}
			draft, err := s.UpdatePluginInput("user", view.ID, input.ID, "", PluginInputUpdate{Revision: 1, Mode: "draft", Value: json.RawMessage(`{"prompt":"draft"}`)})
			if err != nil || draft.Status != "waiting_input" {
				t.Fatal(err)
			}
			if _, err = s.UpdatePluginInput("user", view.ID, input.ID, "stale-form-key", PluginInputUpdate{Revision: 1, Mode: "submit", Value: json.RawMessage(`{"prompt":"stale"}`)}); err == nil {
				t.Fatal("stale form accepted")
			}
			restarted := New(repository.New(db), s.dataDir)
			defer restarted.Close()
			restored, err := restarted.PluginRun("user", view.ID)
			if err != nil || string(restored.Pipeline.Inputs[0].Draft) != `{"prompt":"draft"}` {
				t.Fatal("draft lost", err)
			}
			next := p05Submit(t, restarted, restored)
			if next.Status != "waiting_approval" || calls.Load() != 0 {
				t.Fatalf("approval bypass: %+v", next)
			}
			if _, err = restarted.ResumePluginRun("user", next.ID, next.Revision, "retry_safe", ""); err == nil {
				t.Fatal("resume bypassed approval")
			}
			same, err := restarted.UpdatePluginInput("user", view.ID, input.ID, "submit-original", PluginInputUpdate{Revision: 2, Mode: "submit", Value: json.RawMessage(`{"prompt":"user choice"}`)})
			if err != nil || same.ID != view.ID {
				t.Fatal("idempotent submit", err)
			}
			if _, err = restarted.UpdatePluginInput("user", view.ID, input.ID, "submit-original", PluginInputUpdate{Revision: 2, Mode: "submit", Value: json.RawMessage(`{"prompt":"changed"}`)}); err == nil {
				t.Fatal("same key different value")
			}
			child, err := restarted.PluginRun("user", next.Pipeline.ChildRunID)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = restarted.DecidePluginRun("user", child.ID, child.ApprovalID, "approve", child.Revision); err != nil {
				t.Fatal(err)
			}
			if err = restarted.ProcessNextTask(); err != nil {
				t.Fatal(err)
			}
			for i := 0; i < 3; i++ {
				if err = restarted.applicationPlugins().AdvancePipeline("user", view.ID); err != nil {
					t.Fatal(err)
				}
			}
			done, err := restarted.PluginRun("user", view.ID)
			if err != nil || done.Status != "succeeded" || string(done.Result) != `{"message":"processed over HTTP"}` || calls.Load() != 1 {
				t.Fatalf("done=%+v calls=%d err=%v", done, calls.Load(), err)
			}
			rows, err := restarted.PluginRunEvents("user", view.ID, 0)
			if err != nil || len(rows) < 5 {
				t.Fatal("missing journal", err)
			}
			for i, e := range rows {
				if e.Sequence != int64(i+1) {
					t.Fatal("event gap")
				}
			}
			replay, err := restarted.PluginRunEvents("user", view.ID, rows[1].Sequence)
			if err != nil || len(replay) != len(rows)-2 {
				t.Fatal("bad replay", err)
			}
			history, err := restarted.ListPluginRuns("user", 0)
			if err != nil || len(history) != 1 {
				t.Fatal("child exposed as root", err)
			}
		})
	}
}

func TestP05ConcurrentSchedulingCancelAndRevocation(t *testing.T) {
	for _, driver := range []string{"sqlite", "postgres"} {
		t.Run(driver, func(t *testing.T) {
			s, db, release, calls := p05Fixture(t, driver, nil)
			view := p05Start(t, s, release)
			next := p05Submit(t, s, view)
			var wg sync.WaitGroup
			errs := make(chan error, 6)
			for i := 0; i < 6; i++ {
				wg.Add(1)
				go func() { defer wg.Done(); errs <- s.applicationPlugins().AdvancePipeline("user", view.ID) }()
			}
			wg.Wait()
			close(errs)
			for err := range errs {
				if err != nil {
					t.Fatal(err)
				}
			}
			var n int64
			db.Model(&model.PluginRun{}).Where("parent_run_id=?", view.ID).Count(&n)
			if n != 1 {
				t.Fatal("duplicate child", n)
			}
			current, _ := s.PluginRun("user", view.ID)
			if _, err := s.CancelPluginRun("user", view.ID, current.Revision); err != nil {
				t.Fatal(err)
			}
			for i := 0; i < 2; i++ {
				if err := s.applicationPlugins().AdvancePipeline("user", view.ID); err != nil {
					t.Fatal(err)
				}
			}
			child, _ := s.PluginRun("user", next.Pipeline.ChildRunID)
			if _, err := s.DecidePluginRun("user", child.ID, child.ApprovalID, "approve", child.Revision); err == nil {
				t.Fatal("cancelled child approved")
			}
			if calls.Load() != 0 {
				t.Fatal("cancel submitted HTTP")
			}
			pending := p05Start(t, s, release)
			in := pending.Pipeline.Inputs[0]
			state, _ := s.repo.UserPluginState("user", "pipeline-helper")
			if err := s.ActivateApplicationPlugin(&model.User{ID: "user", Role: model.UserRoleUser}, "pipeline-helper", ApplicationPluginActivation{ReleaseID: release, Enabled: false, Revision: state.Revision}); err != nil {
				t.Fatal(err)
			}
			if _, err := s.UpdatePluginInput("user", pending.ID, in.ID, "revoked-submit", PluginInputUpdate{Revision: in.Revision, Mode: "submit", Value: json.RawMessage(`{"prompt":"no"}`)}); err == nil {
				t.Fatal("revoked input")
			}
			if _, err := s.CancelPluginRun("user", pending.ID, pending.Revision); err != nil {
				t.Fatal("disabled cancellation", err)
			}
			state, _ = s.repo.UserPluginState("user", "pipeline-helper")
			if err := s.ActivateApplicationPlugin(&model.User{ID: "user", Role: model.UserRoleUser}, "pipeline-helper", ApplicationPluginActivation{ReleaseID: release, Enabled: true, GrantedPermissions: []string{"media.read", "connection.use"}, Revision: state.Revision}); err != nil {
				t.Fatal(err)
			}
			paused := p05Start(t, s, release)
			future := time.Now().Add(time.Minute)
			if err := db.Model(&model.PluginPipelineExecution{}).Where("run_id=?", paused.ID).Updates(map[string]any{"lease_owner": "new-owner", "lease_expires_at": future}).Error; err != nil {
				t.Fatal(err)
			}
			stale, _ := s.repo.PluginPipeline(paused.ID)
			stale.Cursor = 99
			if err := s.repo.SavePluginPipeline(stale, "old-owner"); err == nil {
				t.Fatal("stale lease wrote")
			}
		})
	}
}

func TestP05UnknownChildAndFailureDoNotResubmit(t *testing.T) {
	s, _, release, calls := p05Fixture(t, "sqlite", func(w http.ResponseWriter, r *http.Request) { c, _, _ := w.(http.Hijacker).Hijack(); c.Close() })
	next := p05Submit(t, s, p05Start(t, s, release))
	child, _ := s.PluginRun("user", next.Pipeline.ChildRunID)
	if _, err := s.DecidePluginRun("user", child.ID, child.ApprovalID, "approve", child.Revision); err != nil {
		t.Fatal(err)
	}
	if err := s.ProcessNextTask(); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if err := s.applicationPlugins().AdvancePipeline("user", next.ID); err != nil {
			t.Fatal(err)
		}
	}
	child, _ = s.PluginRun("user", child.ID)
	if child.Status != "paused" || calls.Load() != 1 {
		t.Fatal("unknown replay", child.Status, calls.Load())
	}
	current, _ := s.PluginRun("user", next.ID)
	if _, err := s.CancelPluginRun("user", current.ID, current.Revision); err != nil {
		t.Fatal(err)
	}
}

func TestP05AgentInputUsesCurrentLeaseAndParentBudget(t *testing.T) {
	s, db, release, calls := p05Fixture(t, "sqlite", nil)
	config := mustEncodeModelCapabilityConfig(t, DefaultModelCapabilityConfigForModel(string(model.ChannelInterfaceChatCompletion), "text-test"))
	for _, item := range []any{&model.ModelChannel{ID: "channel", Scope: model.ChannelScopeSystem, Enabled: true, Name: "测试"}, &model.ChannelModel{ID: "cm", ChannelID: "channel", ModelKey: "text-test", Capability: "text", Protocol: model.ChannelInterfaceChatCompletion, CapabilityConfigJSON: config, BillingMode: "fixed_request", UnitPriceMicrocredits: 100, PriceConfigured: true, Enabled: true}, &model.ChannelModelPriceTier{ID: "tier", ChannelModelID: "cm", SelectorKey: "{}", SelectorJSON: "{}", BillingMode: "fixed_request", UnitPriceMicrocredits: 100, PriceConfigured: true, Enabled: true}, &model.CreditAccount{UserID: "user", AvailableMicrocredits: 10000}} {
		if err := db.Create(item).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Create(&model.CanvasProject{ID: "agent-canvas", UserID: "user", PayloadJSON: `{"nodes":[]}`}).Error; err != nil {
		t.Fatal(err)
	}
	req := agentTestRequest()
	req.PermissionMode = "request_approval"
	req.PluginToolsVersion = 1
	req.Budget.MaxCredits = 0.00015
	root, err := s.CreateCloudAgentRun("user", req, "")
	if err != nil {
		t.Fatal(err)
	}
	agent, err := s.repo.CloudAgent("user", root.ID)
	if err != nil {
		t.Fatal(err)
	}
	state, err := cloudAgentDecode(agent)
	if err != nil {
		t.Fatal(err)
	}
	out, err := s.applicationPlugins().Invoke("user", "agent-pipeline-start", contracts.Invocation{Operation: "pipeline-helper.process", ReleaseID: release, Input: p03Input(map[string]any{"resourceId": "video-one"})}, plugins.InvocationPolicy{PermissionMode: "request_approval", AgentRunID: agent.ID, AgentRevision: agent.Revision})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err = s.applicationPlugins().AdvancePipeline("user", out.RunID); err != nil {
			t.Fatal(err)
		}
	}
	view, _ := s.PluginRun("user", out.RunID)
	in := view.Pipeline.Inputs[0]
	var call cloudAgentCall
	call.ID = "p05-input-call"
	call.Function.Name = "plugin_input_submit"
	args := map[string]any{"runId": view.ID, "inputRequestId": in.ID, "revision": in.Revision, "value": map[string]any{}}
	raw, _ := json.Marshal(args)
	call.Function.Arguments = string(raw)
	if _, err = s.executeCloudAgentPluginTool(agent, &state, call); err == nil {
		t.Fatal("Agent guessed missing data")
	}
	args["value"] = map[string]any{"prompt": "explicit user content"}
	raw, _ = json.Marshal(args)
	call.Function.Arguments = string(raw)
	stale := *agent
	stale.Revision--
	if _, err = s.executeCloudAgentPluginTool(&stale, &state, call); err == nil {
		t.Fatal("stale Agent submitted input")
	}
	if _, err = s.executeCloudAgentPluginTool(agent, &state, call); err != nil {
		t.Fatal(err)
	}
	if _, err = s.executeCloudAgentPluginTool(agent, &state, call); err != nil {
		t.Fatal("Agent replay", err)
	}
	// Agent turn may end before the expensive step is admitted.
	if err = db.Model(&model.CloudAgentExecution{}).Where("id=?", agent.ID).Update("status", "succeeded").Error; err != nil {
		t.Fatal(err)
	}
	if _, err = s.SavePluginOperationPrice(&model.User{ID: "admin", Role: model.UserRoleAdmin}, PluginOperationPriceInput{ReleaseID: release, OperationID: "echo", FeeMicrocredits: 100}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err = s.applicationPlugins().AdvancePipeline("user", view.ID); err != nil {
			t.Fatal(err)
		}
	}
	view, _ = s.PluginRun("user", view.ID)
	child, _ := s.PluginRun("user", view.Pipeline.ChildRunID)
	if child.AgentRunID != agent.ID || child.ParentStepKey != "remote" {
		t.Fatal("lost parent budget/step identity")
	}
	if _, err = s.DecidePluginRun("user", child.ID, child.ApprovalID, "approve", child.Revision); err == nil {
		t.Fatal("parent budget bypass")
	}
	latest, err := s.repo.LatestPluginRunForAgent("user", agent.ID)
	if err != nil || latest.ID != view.ID {
		t.Fatal("Agent recovery lost pipeline", err)
	}
	if calls.Load() != 0 {
		t.Fatal("unfunded request sent")
	}
}

func TestP05FailedChildClosesPipeline(t *testing.T) {
	s, _, release, _ := p05Fixture(t, "sqlite", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(`{"unexpected":true}`)) })
	view := p05Submit(t, s, p05Start(t, s, release))
	child, _ := s.PluginRun("user", view.Pipeline.ChildRunID)
	if _, err := s.DecidePluginRun("user", child.ID, child.ApprovalID, "approve", child.Revision); err != nil {
		t.Fatal(err)
	}
	if err := s.ProcessNextTask(); err != nil {
		t.Fatal(err)
	}
	if err := s.applicationPlugins().AdvancePipeline("user", view.ID); err != nil {
		t.Fatal(err)
	}
	current, _ := s.PluginRun("user", view.ID)
	if current.Status != "failed" {
		t.Fatalf("failure not propagated: %s", current.Status)
	}
}
