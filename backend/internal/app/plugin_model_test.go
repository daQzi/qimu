package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"gorm.io/gorm"
	"infinite-canvas/backend/internal/mediaanalysis"
	"infinite-canvas/backend/internal/model"
	"infinite-canvas/backend/internal/plugins"
	"infinite-canvas/backend/internal/plugins/contracts"
)

func p11Fixture(t *testing.T, driver string) (*Service, *gorm.DB, string, mediaanalysis.Report) {
	t.Helper()
	s, db, _ := p03Fixture(t, driver)
	s = New(s.repo, s.dataDir)
	t.Cleanup(func() { s.Close() })
	profile := DefaultModelCapabilityConfigForModel(string(model.ChannelInterfaceChatCompletion), "text-test")
	profile.Text.References.PromptMaxChars = 200000
	profile.Text.References.MaxVideos = 1
	profile.Text.References.MaxVideoBytes = 100 << 20
	capability := mustEncodeModelCapabilityConfig(t, profile)
	for _, row := range []any{
		&model.ModelChannel{ID: "channel", Scope: model.ChannelScopeSystem, Enabled: true, Name: "P11 isolated model", BaseURL: "https://example.invalid/v1"},
		&model.ChannelModel{ID: "cm", ChannelID: "channel", ModelKey: "text-test", Capability: "text", Protocol: model.ChannelInterfaceChatCompletion, CapabilityConfigJSON: capability, BillingMode: "fixed_request", UnitPriceMicrocredits: 100, PriceConfigured: true, Enabled: true},
		&model.ChannelModelPriceTier{ID: "tier", ChannelModelID: "cm", SelectorKey: "{}", SelectorJSON: "{}", BillingMode: "fixed_request", UnitPriceMicrocredits: 100, PriceConfigured: true, Enabled: true},
		&model.CreditAccount{UserID: "user", AvailableMicrocredits: 10000},
	} {
		if err := db.Create(row).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Model(&model.Resource{}).Where("id=?", "video-one").Updates(map[string]any{"size": 4096, "duration_ms": 10000}).Error; err != nil {
		t.Fatal(err)
	}
	resource, err := s.repo.ResourceForUser("user", "video-one")
	if err != nil {
		t.Fatal(err)
	}
	report := mediaanalysis.Report{SchemaVersion: 1, Source: pluginVideoSource(resource), Coverage: mediaanalysis.Span{StartMs: 0, EndMs: 10000}, AudioAnalyzed: false,
		Shots:    []mediaanalysis.Shot{{ID: "shot1", Span: mediaanalysis.Span{StartMs: 0, EndMs: 10000}, Description: "合同测试镜头", Camera: "固定", Uncertainties: []string{}}},
		Entities: []mediaanalysis.Entity{{ID: "person1", Kind: "character", Name: "人物一", Description: "合同测试人物", Evidence: []mediaanalysis.Evidence{{ShotID: "shot1", AtMs: 1000, Description: "测试证据"}}, Uncertainties: []string{}}},
		Dialogue: []mediaanalysis.Utterance{}, Limitations: []string{"测试数据，不是实际识别；音轨未分析"}}
	release := installWorkbenchSample(t, s, "video-localization")
	return s, db, release, report
}
func p11Plan() mediaanalysis.Plan {
	return mediaanalysis.Plan{Mappings: []mediaanalysis.Replacement{{EntityID: "person1", Kind: "character", Source: "人物一", Target: "保持剧情角色", Prompt: "保持人物关系与镜头顺序", Reason: "未接入替换服务"}}, Limitations: []string{"不执行生成"}}
}
func p11Request(release, operation string, report mediaanalysis.Report) contracts.Invocation {
	input := map[string]any{"resourceId": "video-one", "model": map[string]string{"channelId": "channel", "model": "text-test"}, "country": "日本", "language": "日语", "style": "保留剧情"}
	if operation == "suggest" {
		delete(input, "resourceId")
		input["report"] = report
	}
	return contracts.Invocation{Operation: "video-localization." + operation, ReleaseID: release, Input: p03Input(input)}
}
func p11Complete(t *testing.T, db *gorm.DB, taskID string, value any) {
	t.Helper()
	text, _ := json.Marshal(value)
	result, _ := json.Marshal(map[string]any{"mode": "text", "text": string(text)})
	if err := db.Model(&model.Task{}).Where("id=?", taskID).Updates(map[string]any{"status": model.TaskStatusSucceeded, "result_json": string(result), "completed_at": time.Now()}).Error; err != nil {
		t.Fatal(err)
	}
}

func TestP11ModelAdmissionAndResult(t *testing.T) {
	for _, driver := range []string{"sqlite", "postgres"} {
		t.Run(driver, func(t *testing.T) {
			s, db, release, report := p11Fixture(t, driver)
			req := p11Request(release, "analyze", report)
			out, err := s.InvokePluginOperation("user", "p11-model-analysis", req)
			if err != nil {
				t.Fatal(err)
			}
			var count int64
			db.Model(&model.Task{}).Count(&count)
			if count != 0 {
				t.Fatal("task before approval")
			}
			view, err := s.PluginRun("user", out.RunID)
			if err != nil {
				t.Fatal(err)
			}
			if view.Status != "waiting_approval" || strings.Contains(string(view.Preview), "BaseURL") {
				t.Fatal(view)
			}
			approved, err := s.DecidePluginRun("user", view.ID, view.ApprovalID, "approve", view.Revision)
			if err != nil {
				t.Fatal(err)
			}
			task, err := s.repo.TaskForUser("user", *approved.TaskID)
			if err != nil {
				t.Fatal(err)
			}
			if task.Type != "canvas_text" || task.PluginRunID == nil || task.AuthorizedChargeMicrocredits != 100 {
				t.Fatalf("wrong task: %+v", task)
			}
			var order model.BillingOrder
			if err = db.First(&order, "id=?", task.BillingOrderID).Error; err != nil {
				t.Fatal(err)
			}
			if !order.ChargeLimitSet || order.ChargeLimitMicrocredits != 100 {
				t.Fatal("missing ceiling")
			}
			if _, err = s.DecidePluginRun("user", view.ID, view.ApprovalID, "approve", view.Revision); err != nil {
				t.Fatal("approval replay", err)
			}
			db.Model(&model.Task{}).Count(&count)
			if count != 1 {
				t.Fatal("duplicate task")
			}
			p11Complete(t, db, task.ID, report)
			result, err := s.PluginRun("user", view.ID)
			if err != nil || result.Status != "succeeded" || result.ResultRef == nil {
				t.Fatalf("%+v %v", result, err)
			}
			if _, err = s.PluginRun("other", view.ID); err == nil {
				t.Fatal("cross account access")
			}
			if _, err = s.RetryTask("user", task.ID); err == nil {
				t.Fatal("retry bypassed approval")
			}
		})
	}
}

func TestP11ChangedQuoteCapabilityAndInvalidOutput(t *testing.T) {
	s, db, release, report := p11Fixture(t, "sqlite")
	req := p11Request(release, "analyze", report)
	out, err := s.InvokePluginOperation("user", "p11-price-change", req)
	if err != nil {
		t.Fatal(err)
	}
	view, _ := s.PluginRun("user", out.RunID)
	if err = db.Model(&model.ChannelModelPriceTier{}).Where("id=?", "tier").Update("unit_price_microcredits", 200).Error; err != nil {
		t.Fatal(err)
	}
	if _, err = s.DecidePluginRun("user", view.ID, view.ApprovalID, "approve", view.Revision); err == nil {
		t.Fatal("stale price approved")
	}
	var count int64
	db.Model(&model.Task{}).Count(&count)
	if count != 0 {
		t.Fatal("task created on stale quote")
	}
	if err = db.Model(&model.ChannelModelPriceTier{}).Where("id=?", "tier").Update("unit_price_microcredits", 100).Error; err != nil {
		t.Fatal(err)
	}
	out, err = s.InvokePluginOperation("user", "p11-new-quote", req)
	if err != nil {
		t.Fatal(err)
	}
	view, err = s.PluginRun("user", out.RunID)
	if err != nil {
		t.Fatal(err)
	}
	approved, err := s.DecidePluginRun("user", view.ID, view.ApprovalID, "approve", view.Revision)
	if err != nil {
		t.Fatal(err)
	}
	report.Source.Digest = strings.Repeat("0", 64)
	p11Complete(t, db, *approved.TaskID, report)
	done, err := s.PluginRun("user", view.ID)
	if err != nil || done.Status != "failed" || len(done.Result) > 0 {
		t.Fatalf("invalid output succeeded: %+v %v", done, err)
	}
	if err = db.Model(&model.ChannelModel{}).Where("id=?", "cm").Update("enabled", false).Error; err != nil {
		t.Fatal(err)
	}
	if _, err = s.InvokePluginOperation("user", "p11-disabled-model", req); err == nil {
		t.Fatal("disabled model admitted")
	}
}

func TestP11PipelineManualCorrectionAndFreeze(t *testing.T) {
	for _, driver := range []string{"sqlite", "postgres"} {
		t.Run(driver, func(t *testing.T) {
			s, db, release, report := p11Fixture(t, driver)
			out, err := s.InvokePluginOperation("user", "p11-full-pipeline", p11Request(release, "process", report))
			if err != nil {
				t.Fatal(err)
			}
			advance := func() plugins.RunView {
				t.Helper()
				s.advancePluginPipelines()
				v, e := s.PluginRun("user", out.RunID)
				if e != nil {
					t.Fatal(e)
				}
				return v
			}
			view := advance()
			child, err := s.PluginRun("user", view.Pipeline.ChildRunID)
			if err != nil {
				t.Fatal(err)
			}
			child, err = s.DecidePluginRun("user", child.ID, child.ApprovalID, "approve", child.Revision)
			if err != nil {
				t.Fatal(err)
			}
			p11Complete(t, db, *child.TaskID, report)
			view = advance()
			if view.Status != "waiting_input" {
				view = advance()
			}
			if view.Status != "waiting_input" || len(view.Pipeline.Inputs) != 1 {
				t.Fatalf("not reviewing: %+v", view)
			}
			input := view.Pipeline.Inputs[0]
			projection := p07Projection(t, s, view, "review-board", input.ID, "p11-review")
			projection.Operation = "video-localization.place-input"
			placed := p07Place(t, s, projection)
			if placed.Status != "succeeded" {
				t.Fatal("input projection failed")
			}
			var draft map[string]any
			if err = json.Unmarshal(input.Draft, &draft); err != nil {
				t.Fatal(err)
			}
			bad := report
			bad.Entities = append([]mediaanalysis.Entity{}, report.Entities...)
			bad.Entities[0].Evidence = []mediaanalysis.Evidence{{ShotID: "missing", AtMs: 10, Description: "invalid"}}
			draft["report"] = bad
			raw, _ := json.Marshal(draft)
			if _, err = s.UpdatePluginInput("user", view.ID, input.ID, "p11-invalid-input", PluginInputUpdate{Revision: input.Revision, Mode: "submit", Value: raw}); err == nil {
				t.Fatal("invalid evidence submitted")
			}
			draft["report"] = report
			raw, _ = json.Marshal(draft)
			if _, err = s.UpdatePluginInput("user", view.ID, input.ID, "p11-valid-input", PluginInputUpdate{Revision: input.Revision, Mode: "submit", Value: raw}); err != nil {
				t.Fatal(err)
			}
			view = advance()
			if view.Pipeline.ChildRunID == "" {
				view = advance()
			}
			child, err = s.PluginRun("user", view.Pipeline.ChildRunID)
			if err != nil {
				t.Fatal(err)
			}
			child, err = s.DecidePluginRun("user", child.ID, child.ApprovalID, "approve", child.Revision)
			if err != nil {
				t.Fatal(err)
			}
			p11Complete(t, db, *child.TaskID, p11Plan())
			view = advance()
			if view.Status != "waiting_input" {
				view = advance()
			}
			input = view.Pipeline.Inputs[len(view.Pipeline.Inputs)-1]
			if err = json.Unmarshal(input.Draft, &draft); err != nil {
				t.Fatal(err)
			}
			draft["confirmed"] = true
			raw, _ = json.Marshal(draft)
			if _, err = s.UpdatePluginInput("user", view.ID, input.ID, "p11-freeze-plan", PluginInputUpdate{Revision: input.Revision, Mode: "submit", Value: raw}); err != nil {
				t.Fatal(err)
			}
			view = advance()
			if view.Status != "succeeded" {
				view = advance()
			}
			if view.Status != "succeeded" {
				t.Fatalf("not frozen: %+v", view)
			}
			projection = p07Projection(t, s, view, "plan-board", "", "p11-result")
			projection.Operation = "video-localization.place-result"
			if placed = p07Place(t, s, projection); placed.Status != "succeeded" {
				t.Fatal("result projection failed")
			}
			var count int64
			db.Model(&model.Task{}).Count(&count)
			if count != 2 {
				t.Fatalf("confirmation generated extra tasks: %d", count)
			}
			derived, err := s.DerivePluginRun("user", view.ID, "p11-derive-confirm", PluginDeriveRequest{ReuseCompleted: true, ForceSteps: []string{"confirm"}})
			if err != nil {
				t.Fatal(err)
			}
			for i := 0; i < 5 && derived.Status != "waiting_input"; i++ {
				s.advancePluginPipelines()
				derived, err = s.PluginRun("user", derived.ID)
				if err != nil {
					t.Fatal(err)
				}
			}
			if derived.Status != "waiting_input" {
				t.Fatalf("derived input: %+v", derived)
			}
			var pending plugins.InputView
			for _, row := range derived.Pipeline.Inputs {
				if row.StepKey == "confirm" {
					pending = row
				}
			}
			if pending.Status != "pending" || len(pending.Draft) == 0 {
				t.Fatal("DAG prefill missing")
			}
			db.Model(&model.Task{}).Count(&count)
			if count != 2 {
				t.Fatal("derived confirmation repeated paid analysis")
			}
			var updated map[string]any
			json.Unmarshal(pending.Draft, &updated)
			updated["style"] = "用户修改"
			raw, _ = json.Marshal(updated)
			if _, err = s.UpdatePluginInput("user", derived.ID, pending.ID, "", PluginInputUpdate{Revision: pending.Revision, Mode: "draft", Value: raw}); err != nil {
				t.Fatal(err)
			}
			s.advancePluginPipelines()
			derived, err = s.PluginRun("user", derived.ID)
			if err != nil {
				t.Fatal(err)
			}
			for _, row := range derived.Pipeline.Inputs {
				if row.StepKey == "confirm" && !strings.Contains(string(row.Draft), "用户修改") {
					t.Fatal("DAG prefill overwrote edit")
				}
			}
			old, err := s.PluginRun("user", view.ID)
			if err != nil || old.Status != "succeeded" {
				t.Fatal("derivation overwrote history", err)
			}
		})
	}
}

func TestP11NormalTextWorkerAndCancel(t *testing.T) {
	t.Setenv("CANVAS_ALLOWED_PRIVATE_UPSTREAM_HOSTS", "127.0.0.1")
	s, db, release, report := p11Fixture(t, "sqlite")
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		payload, _ := json.Marshal(p11Plan())
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"id": "p11-test-response", "choices": []any{map[string]any{"message": map[string]any{"role": "assistant", "content": string(payload)}}}})
	}))
	defer server.Close()
	if err := db.Model(&model.ModelChannel{}).Where("id=?", "channel").Updates(map[string]any{"base_url": server.URL + "/v1", "api_key": "isolated-test-key"}).Error; err != nil {
		t.Fatal(err)
	}
	out, err := s.InvokePluginOperation("user", "p11-worker-test", p11Request(release, "suggest", report))
	if err != nil {
		t.Fatal(err)
	}
	view, _ := s.PluginRun("user", out.RunID)
	view, err = s.DecidePluginRun("user", view.ID, view.ApprovalID, "approve", view.Revision)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.ProcessNextTask(); err != nil {
		t.Fatal(err)
	}
	view, err = s.PluginRun("user", view.ID)
	if err != nil || view.Status != "succeeded" || calls.Load() != 1 {
		t.Fatalf("worker: %+v %v calls=%d", view, err, calls.Load())
	}
	out, err = s.InvokePluginOperation("user", "p11-cancel-test", p11Request(release, "suggest", report))
	if err != nil {
		t.Fatal(err)
	}
	view, _ = s.PluginRun("user", out.RunID)
	view, err = s.DecidePluginRun("user", view.ID, view.ApprovalID, "approve", view.Revision)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.CancelPluginRun("user", view.ID, view.Revision); err != nil {
		t.Fatal(err)
	}
	s.advancePluginPipelines()
	view, err = s.PluginRun("user", view.ID)
	if err != nil || view.Status != "cancelled" || calls.Load() != 1 {
		t.Fatalf("cancel: %+v %v", view, err)
	}
	var account model.CreditAccount
	if err = db.First(&account, "user_id=?", "user").Error; err != nil {
		t.Fatal(err)
	}
	if account.AvailableMicrocredits != 9900 {
		t.Fatalf("model charged twice or cancellation not refunded: %+v", account)
	}
}
