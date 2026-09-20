package app

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gorm.io/gorm"
	"infinite-canvas/backend/internal/model"
	"infinite-canvas/backend/internal/plugins"
	"infinite-canvas/backend/internal/plugins/contracts"
	"infinite-canvas/backend/internal/repository"
)

func p07Fixture(t *testing.T, driver string) (*Service, *gorm.DB, string) {
	t.Helper()
	s, db, _ := p03Fixture(t, driver)
	root := "../plugins/contracts/testdata/canvas-helper-p07"
	var archive bytes.Buffer
	w := zip.NewWriter(&archive)
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
	if _, err = s.InstallManagedPluginForAdmin(&model.User{ID: "admin", Role: model.UserRoleAdmin}, archive.Bytes(), "canvas.yingce-plugin"); err != nil {
		t.Fatal(err)
	}
	release, err := s.repo.PluginReleaseByVersion("canvas-helper", "1.0.0")
	if err != nil || release == nil {
		t.Fatal(err)
	}
	if err = s.ActivateApplicationPlugin(&model.User{ID: "user", Role: model.UserRoleUser}, "canvas-helper", ApplicationPluginActivation{ReleaseID: release.ID, Enabled: true, GrantedPermissions: []string{"canvas.read", "canvas.write"}}); err != nil {
		t.Fatal(err)
	}
	return s, db, release.ID
}
func p07Start(t *testing.T, s *Service, release string) plugins.RunView {
	t.Helper()
	out, err := s.InvokePluginOperation("user", newID(), contracts.Invocation{Operation: "canvas-helper.process", ReleaseID: release, Input: p03Input(map[string]any{})})
	if err != nil {
		t.Fatal(err)
	}
	return p07Advance(t, s, out.RunID)
}
func p07Advance(t *testing.T, s *Service, id string) plugins.RunView {
	t.Helper()
	for i := 0; i < 3; i++ {
		if err := s.applicationPlugins().AdvancePipeline("user", id); err != nil {
			t.Fatal(err)
		}
	}
	v, err := s.PluginRun("user", id)
	if err != nil {
		t.Fatal(err)
	}
	return v
}
func p07Projection(t *testing.T, s *Service, run plugins.RunView, bp, input, instance string) contracts.Invocation {
	t.Helper()
	snapshot, err := s.PluginCanvasSnapshot("user", "canvas-one")
	if err != nil {
		t.Fatal(err)
	}
	values := map[string]any{"runId": run.ID, "blueprintId": bp, "snapshotHash": snapshot["snapshotHash"], "instanceKey": instance}
	if input != "" {
		values["inputRequestId"] = input
	} else {
		values["resultDigest"] = run.ResultRef.Digest
	}
	return contracts.Invocation{Operation: "canvas-helper.place", ReleaseID: run.ReleaseID, Input: p03Input(values), Context: &contracts.InvocationContext{HostSurface: "canvas", CanvasID: "canvas-one"}}
}
func p07Place(t *testing.T, s *Service, request contracts.Invocation) plugins.RunView {
	t.Helper()
	out, err := s.InvokePluginOperation("user", newID(), request)
	if err != nil {
		t.Fatal(err)
	}
	return p03Approve(t, s, out)
}
func p07Document(t *testing.T, s *Service) (*model.CanvasProject, map[string]any) {
	t.Helper()
	canvas, err := s.repo.CanvasProjectForUser("user", "canvas-one")
	if err != nil {
		t.Fatal(err)
	}
	doc, err := creationDocument(canvas.PayloadJSON)
	if err != nil {
		t.Fatal(err)
	}
	return canvas, doc
}
func p07SaveDocument(t *testing.T, db *gorm.DB, canvas *model.CanvasProject, doc map[string]any) {
	t.Helper()
	raw, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	if err = db.Model(canvas).Update("payload_json", string(raw)).Error; err != nil {
		t.Fatal(err)
	}
}

func TestP07CanvasInputLifecycle(t *testing.T) {
	for _, driver := range []string{"sqlite", "postgres"} {
		t.Run(driver, func(t *testing.T) {
			s, db, release := p07Fixture(t, driver)
			run := p07Start(t, s, release)
			if run.Status != "waiting_input" || len(run.Pipeline.Inputs) != 1 || run.Pipeline.Inputs[0].View.Component != "mapping-editor/v1" || len(run.CanvasActions) != 1 {
				t.Fatalf("missing presentation: %+v", run)
			}
			input := run.Pipeline.Inputs[0]
			request := p07Projection(t, s, run, "mapping-input", input.ID, "default")
			out, err := s.InvokePluginOperation("user", "input-project", request)
			if err != nil {
				t.Fatal(err)
			}
			if p03NodeCount(t, s) != 0 {
				t.Fatal("write before approval")
			}
			p03Approve(t, s, out)
			p07Place(t, s, p07Projection(t, s, run, "mapping-input", input.ID, "default"))
			if p03NodeCount(t, s) != 1 {
				t.Fatal("duplicate projection")
			}
			_, doc := p07Document(t, s)
			binding := creationMaps(doc["nodes"])[0]["metadata"].(map[string]any)["pluginInput"].(map[string]any)
			if binding["runId"] != run.ID || binding["inputRequestId"] != input.ID {
				t.Fatal("wrong input binding")
			}
			if _, err = s.UpdatePluginInput("other", run.ID, input.ID, "foreign-input", PluginInputUpdate{Revision: input.Revision, Mode: "submit", Value: json.RawMessage(`{}`)}); err == nil {
				t.Fatal("foreign input allowed")
			}
			if _, err = s.UpdatePluginInput("user", run.ID, input.ID, "invalid-input", PluginInputUpdate{Revision: input.Revision, Mode: "submit", Value: json.RawMessage(`{"mappings":[]}`)}); err == nil {
				t.Fatal("invalid input advanced")
			}
			value := json.RawMessage(`{"mappings":[{"source":"原角色","target":"新角色"}]}`)
			draft, err := s.UpdatePluginInput("user", run.ID, input.ID, "draft", PluginInputUpdate{Revision: input.Revision, Mode: "draft", Value: value})
			if err != nil {
				t.Fatal(err)
			}
			if draft.Status != "waiting_input" {
				t.Fatal("draft advanced run")
			}
			if _, err = s.UpdatePluginInput("user", run.ID, input.ID, "stale-input", PluginInputUpdate{Revision: input.Revision, Mode: "submit", Value: value}); err == nil {
				t.Fatal("stale input accepted")
			}
			// Deletion/undo affects only the projection. A new service can finish the same input.
			canvas, doc := p07Document(t, s)
			doc["nodes"] = []any{}
			p07SaveDocument(t, db, canvas, doc)
			s = &Service{repo: repository.New(db), dataDir: s.dataDir}
			persisted, err := s.PluginRun("user", run.ID)
			if err != nil || persisted.Status != "waiting_input" || !bytes.Equal(persisted.Pipeline.Inputs[0].Draft, value) {
				t.Fatalf("lost draft: %v", err)
			}
			if _, err = s.InvokePluginOperation("user", "restore-deleted", p07Projection(t, s, run, "mapping-input", input.ID, "default")); err == nil {
				t.Fatal("silently recreated deleted binding")
			}
			p07Place(t, s, p07Projection(t, s, run, "mapping-input", input.ID, "new"))
			update := PluginInputUpdate{Revision: input.Revision + 1, Mode: "submit", Value: value}
			for i := 0; i < 2; i++ {
				if _, err = s.UpdatePluginInput("user", run.ID, input.ID, "submit-once", update); err != nil {
					t.Fatal(err)
				}
			}
			run = p07Advance(t, s, run.ID)
			if run.Status != "waiting_input" || len(run.CanvasActions) != 1 || run.CanvasActions[0].BlueprintID != "media-input" {
				t.Fatalf("next input guidance missing: %+v", run)
			}
			media := run.Pipeline.Inputs[1]
			p07Place(t, s, p07Projection(t, s, run, "media-input", media.ID, "default"))
			if _, err = s.UpdatePluginInput("user", run.ID, media.ID, "submit-media", PluginInputUpdate{Revision: media.Revision, Mode: "submit", Value: json.RawMessage(`{"before":"video-one","after":"video-one"}`)}); err != nil {
				t.Fatal(err)
			}
			run = p07Advance(t, s, run.ID)
			if run.Status != "succeeded" || len(run.CanvasActions) != 1 || run.CanvasActions[0].BlueprintID != "results" {
				t.Fatalf("result actions missing: %+v", run)
			}
			p07Place(t, s, p07Projection(t, s, run, "results", "", "default"))
			_, doc = p07Document(t, s)
			if len(creationMaps(doc["nodes"])) != 5 || len(creationMaps(doc["connections"])) != 2 {
				t.Fatal("incomplete blueprint")
			}
			for _, id := range []string{"table", "cards", "compare"} {
				v, err := s.PluginRun("user", run.ID, id)
				if err != nil || v.View.ID != id {
					t.Fatalf("view %s: %v", id, err)
				}
			}
			var tasks int64
			db.Model(&model.Task{}).Count(&tasks)
			if tasks != 0 {
				t.Fatal("presentation started model work")
			}
			// A changed flow edge conflicts without changing the successful source.
			canvas, doc = p07Document(t, s)
			doc["connections"] = []any{}
			p07SaveDocument(t, db, canvas, doc)
			if _, err = s.InvokePluginOperation("user", "edge-conflict", p07Projection(t, s, run, "results", "", "default")); err == nil {
				t.Fatal("deleted edge silently restored")
			}
			latest, err := s.PluginRun("user", run.ID)
			if err != nil || latest.ResultRef.Digest != run.ResultRef.Digest {
				t.Fatal("result changed on conflict")
			}
		})
	}
}

func TestP07InputProjectionStrongBoundaries(t *testing.T) {
	s, _, release := p07Fixture(t, "sqlite")
	run := p07Start(t, s, release)
	input := run.Pipeline.Inputs[0]
	for _, mutate := range []func(*contracts.Invocation){
		func(r *contracts.Invocation) { r.Context.CanvasID = "foreign-canvas" },
		func(r *contracts.Invocation) { r.Input["inputRequestId"] = json.RawMessage(`"missing"`) },
		func(r *contracts.Invocation) { r.Input["blueprintId"] = json.RawMessage(`"media-input"`) },
		func(r *contracts.Invocation) {
			r.Input["resultDigest"] = json.RawMessage(`"` + strings.Repeat("0", 64) + `"`)
		},
	} {
		r := p07Projection(t, s, run, "mapping-input", input.ID, "default")
		mutate(&r)
		if _, err := s.InvokePluginOperation("user", newID(), r); err == nil {
			t.Fatal("invalid input projection accepted")
		}
	}
	out, err := s.InvokePluginOperation("user", "late-projection", p07Projection(t, s, run, "mapping-input", input.ID, "default"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.UpdatePluginInput("user", run.ID, input.ID, "submit-before-approve", PluginInputUpdate{Revision: input.Revision, Mode: "submit", Value: json.RawMessage(`{"mappings":[{"source":"a","target":"b"}]}`)}); err != nil {
		t.Fatal(err)
	}
	if _, err = s.DecidePluginRun("user", out.RunID, out.ApprovalID, "approve", out.Revision); err == nil {
		t.Fatal("projected stale input")
	}
	if p03NodeCount(t, s) != 0 {
		t.Fatal("partial stale projection")
	}
}

func TestP07ProjectionRollbackOnInterruptedCommit(t *testing.T) {
	for _, driver := range []string{"sqlite", "postgres"} {
		t.Run(driver, func(t *testing.T) {
			s, db, release := p07Fixture(t, driver)
			run := p07Start(t, s, release)
			out, err := s.InvokePluginOperation("user", "atomic-input", p07Projection(t, s, run, "mapping-input", run.Pipeline.Inputs[0].ID, "default"))
			if err != nil {
				t.Fatal(err)
			}
			if err = db.Callback().Create().Before("gorm:create").Register("p07:interrupt", func(tx *gorm.DB) {
				if tx.Statement.Table == "plugin_canvas_projections" {
					tx.AddError(errors.New("P07 injected write interruption"))
				}
			}); err != nil {
				t.Fatal(err)
			}
			_, approvalErr := s.DecidePluginRun("user", out.RunID, out.ApprovalID, "approve", out.Revision)
			if err = db.Callback().Create().Remove("p07:interrupt"); err != nil {
				t.Fatal(err)
			}
			if approvalErr == nil {
				t.Fatal("injected interruption did not fail")
			}
			if p03NodeCount(t, s) != 0 {
				t.Fatal("partial canvas write survived rollback")
			}
			var count int64
			if err = db.Model(&model.PluginCanvasProjection{}).Count(&count).Error; err != nil || count != 0 {
				t.Fatal("partial projection survived rollback", err)
			}
			p03Approve(t, s, out)
			if p03NodeCount(t, s) != 1 {
				t.Fatal("safe retry lost binding")
			}
		})
	}
}

func TestP07AgentProjectsPendingInputWithoutApprovingPipeline(t *testing.T) {
	s, db, release := p07Fixture(t, "sqlite")
	run := p07Start(t, s, release)
	if err := db.Model(&model.PluginRun{}).Where("id = ?", run.ID).Update("status", "waiting_approval").Error; err != nil {
		t.Fatal(err)
	}
	agent := model.CloudAgentExecution{ID: "p07-agent", UserID: "user", Status: "running", Revision: 1}
	if err := db.Create(&agent).Error; err != nil {
		t.Fatal(err)
	}
	state := cloudAgentRuntime{}
	state.Request.CanvasID = "canvas-one"
	state.Request.PermissionMode = "request_approval"
	state.PendingExecution = &cloudAgentExecutionRef{Kind: "plugin_run", ID: run.ID}
	request := p07Projection(t, s, run, "mapping-input", run.Pipeline.Inputs[0].ID, "default")
	var call cloudAgentCall
	call.ID = "describe-input"
	call.Function.Name = "operation_describe"
	raw, _ := json.Marshal(map[string]any{"operation": request.Operation, "releaseId": release})
	call.Function.Arguments = string(raw)
	if _, err := s.executeCloudAgentPluginTool(&agent, &state, call); err != nil {
		t.Fatal(err)
	}
	call.ID = "project-input"
	call.Function.Name = "operation_invoke"
	raw, _ = json.Marshal(map[string]any{"operation": request.Operation, "releaseId": release, "input": request.Input})
	call.Function.Arguments = string(raw)
	value, err := s.executeCloudAgentPluginTool(&agent, &state, call)
	if err != nil {
		t.Fatal(err)
	}
	output := value.(plugins.InvocationOutput)
	if output.Status != "waiting_approval" || p03NodeCount(t, s) != 0 {
		t.Fatal("agent self-approved projection")
	}
	current, err := s.PluginRun("user", run.ID)
	if err != nil || current.Status != "waiting_approval" || current.Pipeline.Inputs[0].Status != "pending" {
		t.Fatal("projection advanced parent")
	}
	call.ID = "second-projection"
	if _, err = s.executeCloudAgentPluginTool(&agent, &state, call); err == nil {
		t.Fatal("unrelated pending canvas approval bypassed")
	}
}
