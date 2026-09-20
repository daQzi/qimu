package app

import (
	"encoding/json"
	"infinite-canvas/backend/internal/model"
	"infinite-canvas/backend/internal/plugins/authoring"
	"infinite-canvas/backend/internal/plugins/contracts"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"
)

func TestP14ModelReviewTemplateRuntime(t *testing.T) {
	for _, driver := range []string{"sqlite", "postgres"} {
		t.Run(driver, func(t *testing.T) {
			t.Setenv("CANVAS_ALLOWED_PRIVATE_UPSTREAM_HOSTS", "127.0.0.1")
			if driver == "postgres" {
				t.Setenv("REDIS_URL", os.Getenv("CANVAS_TEST_REDIS_URL"))
			}
			s, db, _, _ := p11Fixture(t, driver)
			files, err := authoring.Template("model-review", "general-review", "author", contracts.Policy{})
			if err != nil {
				t.Fatal(err)
			}
			packed, err := authoring.Pack(files, contracts.Policy{})
			if err != nil {
				t.Fatal(err)
			}
			if _, err = s.InstallManagedPluginForAdmin(&model.User{ID: "admin", Role: model.UserRoleAdmin}, packed, "review.yingce-plugin"); err != nil {
				t.Fatal(err)
			}
			release, err := s.repo.PluginReleaseByVersion("general-review", "1.0.0")
			if err != nil || release == nil {
				t.Fatal(err)
			}
			if err = s.ActivateApplicationPlugin(&model.User{ID: "user", Role: model.UserRoleUser}, "general-review", ApplicationPluginActivation{ReleaseID: release.ID, Enabled: true, GrantedPermissions: []string{"generation.run"}}); err != nil {
				t.Fatal(err)
			}
			ctx := contracts.InvocationContext{HostSurface: "agent-home"}
			board, err := s.PluginWorkbench("user", "general-review.review", release.ID, ctx)
			if err != nil || board.SkillID == "" {
				t.Fatal("workbench/skill unavailable", err)
			}
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"content": `{"text":"模型草稿"}`}}}})
			}))
			defer server.Close()
			if err = db.Model(&model.ModelChannel{}).Where("id=?", "channel").Updates(map[string]any{"base_url": server.URL + "/v1", "api_key": "isolated"}).Error; err != nil {
				t.Fatal(err)
			}
			request := contracts.Invocation{Operation: "general-review.process", ReleaseID: release.ID, Context: &ctx, Input: p03Input(map[string]any{"goal": "整理一份营销文案草稿", "model": map[string]string{"channelId": "channel", "model": "text-test"}})}
			result, err := s.InvokePluginOperation("user", "model-review-flow", request)
			if err != nil {
				t.Fatal(err)
			}
			s.advancePluginPipelines()
			view, err := s.PluginRun("user", result.RunID)
			if err != nil {
				t.Fatal(err)
			}
			child, err := s.PluginRun("user", view.Pipeline.ChildRunID)
			if err != nil {
				t.Fatal(err)
			}
			if child.Status != "waiting_approval" || calls.Load() != 0 {
				t.Fatal("approval bypassed")
			}
			if _, err = s.DecidePluginRun("user", child.ID, child.ApprovalID, "approve", child.Revision); err != nil {
				t.Fatal(err)
			}
			if err = s.ProcessNextTask(); err != nil {
				t.Fatal(err)
			}
			for i := 0; i < 4; i++ {
				s.advancePluginPipelines()
				view, err = s.PluginRun("user", result.RunID)
				if err != nil {
					t.Fatal(err)
				}
				if view.Status == "waiting_input" {
					break
				}
			}
			if view.Status != "waiting_input" || len(view.Pipeline.Inputs) != 1 {
				t.Fatalf("not waiting for human: %+v", view)
			}
			input := view.Pipeline.Inputs[0]
			var draft map[string]any
			if err = json.Unmarshal(input.Draft, &draft); err != nil || draft["text"] != "模型草稿" || draft["approved"] != nil {
				t.Fatal("draft/consent incorrect", err)
			}
			if _, err = s.UpdatePluginInput("user", view.ID, input.ID, "invalid-review", PluginInputUpdate{Revision: input.Revision, Mode: "submit", Value: json.RawMessage(`{"text":"修改","approved":false}`)}); err == nil {
				t.Fatal("false consent accepted")
			}
			if _, err = s.UpdatePluginInput("user", view.ID, input.ID, "valid-review", PluginInputUpdate{Revision: input.Revision, Mode: "submit", Value: json.RawMessage(`{"text":"用户修正","approved":true}`)}); err != nil {
				t.Fatal(err)
			}
			for i := 0; i < 4; i++ {
				s.advancePluginPipelines()
				view, err = s.PluginRun("user", result.RunID)
				if err != nil {
					t.Fatal(err)
				}
				if view.Status == "succeeded" {
					break
				}
			}
			if view.Status != "succeeded" || calls.Load() != 1 {
				t.Fatalf("completion status=%s calls=%d", view.Status, calls.Load())
			}
			var final map[string]any
			if err = json.Unmarshal(view.Result, &final); err != nil || final["text"] != "用户修正" {
				t.Fatal("user correction lost", err)
			}
			if _, err = s.PluginRun("other", view.ID); err == nil {
				t.Fatal("cross-account result exposed")
			}
		})
	}
}
