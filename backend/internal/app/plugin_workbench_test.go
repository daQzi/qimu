package app

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"infinite-canvas/backend/internal/model"
	"infinite-canvas/backend/internal/plugins/authoring"
	"infinite-canvas/backend/internal/plugins/contracts"
)

func installWorkbenchSample(t *testing.T, s *Service, name string, changes ...func(contracts.PackageFiles)) string {
	t.Helper()
	files, err := authoring.ReadDirectory(filepath.Join("../../../examples/plugins", name))
	if err != nil {
		t.Fatal(err)
	}
	for _, change := range changes {
		change(files)
	}
	raw, err := authoring.Pack(files, contracts.Policy{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.InstallManagedPluginForAdmin(&model.User{ID: "admin", Role: model.UserRoleAdmin}, raw, name+".yingce-plugin"); err != nil {
		t.Fatal(err)
	}
	var manifest contracts.Manifest
	if err = json.Unmarshal(files["manifest.json"], &manifest); err != nil {
		t.Fatal(err)
	}
	release, err := s.repo.PluginReleaseByVersion(name, manifest.Version)
	if err != nil || release == nil {
		t.Fatal("missing release", err)
	}
	if err = s.ActivateApplicationPlugin(&model.User{ID: "user", Role: model.UserRoleUser}, name, ApplicationPluginActivation{ReleaseID: release.ID, Enabled: true, GrantedPermissions: manifest.Permissions}); err != nil {
		t.Fatal(err)
	}
	return release.ID
}

func TestWorkbenchAdmissionAndSnapshot(t *testing.T) {
	for _, driver := range []string{"sqlite", "postgres"} {
		t.Run(driver, func(t *testing.T) {
			s, _, _ := p03Fixture(t, driver)
			release := installWorkbenchSample(t, s, "resource-workbench", func(files contracts.PackageFiles) {
				var manifest map[string]any
				json.Unmarshal(files["manifest.json"], &manifest)
				manifest["contributes"].(map[string]any)["workbenches"] = []any{map[string]string{"id": "inspect", "ref": "workbenches/inspect.json"}, map[string]string{"id": "snapshot", "ref": "workbenches/snapshot.json"}}
				files["manifest.json"], _ = json.Marshal(manifest)
				files["workbenches/snapshot.json"] = []byte(`{"id":"snapshot","name":"保存检查","description":"保存已确认的检查结果","hostSurfaces":["agent-home"],"contextSchemaRef":"schemas/snapshot-input.json","operation":"resource-workbench.snapshot-video","recipes":[],"defaults":{},"output":"result"}`)
			})
			ctx := contracts.InvocationContext{HostSurface: "agent-home"}
			catalog, err := s.PluginWorkbenches("user")
			if err != nil || len(catalog) != 2 {
				t.Fatal(catalog, err)
			}
			view, err := s.PluginWorkbench("user", "resource-workbench.inspect", release, ctx)
			if err != nil || view.SkillID == "" {
				t.Fatal("skill not bound", err)
			}
			req := PluginWorkbenchComposeRequest{ID: view.ID, ReleaseID: release, RecipeIDs: []string{}, Input: map[string]any{"resourceId": "video-one"}, Context: ctx}
			preview, err := s.ComposePluginWorkbench("user", req)
			if err != nil || !preview.Valid {
				t.Fatal(preview, err)
			}
			invocation := contracts.Invocation{Operation: view.Definition.Operation, ReleaseID: release, Input: p03Input(preview.Input), Context: &ctx, Workbench: &preview.Selection}
			output, err := s.InvokePluginOperation("user", "", invocation)
			if err != nil || output.Kind != "inline" || output.Digest == "" {
				t.Fatal(output, err)
			}
			snapshotPreview, err := s.ComposePluginWorkbench("user", PluginWorkbenchComposeRequest{ID: "resource-workbench.snapshot", ReleaseID: release, RecipeIDs: []string{}, Input: map[string]any{"resourceId": "video-one", "expectedDigest": output.Digest}, Context: ctx})
			if err != nil || !snapshotPreview.Valid {
				t.Fatal("snapshot preview", err)
			}
			snapshotRequest := contracts.Invocation{Operation: "resource-workbench.snapshot-video", ReleaseID: release, Input: p03Input(snapshotPreview.Input), Context: &ctx, Workbench: &snapshotPreview.Selection}
			pending, err := s.InvokePluginOperation("user", "workbench-snapshot", snapshotRequest)
			if err != nil || pending.Status != "waiting_approval" {
				t.Fatal("approval bypass", err)
			}
			replay, err := s.InvokePluginOperation("user", "workbench-snapshot", snapshotRequest)
			if err != nil || replay.RunID != pending.RunID {
				t.Fatal("duplicate snapshot", err)
			}
			snapshot := p03Approve(t, s, pending)
			historical, err := s.PluginRun("user", snapshot.ID)
			if err != nil || historical.Workbench == nil || historical.Workbench.Digest != snapshotPreview.Selection.Digest {
				t.Fatal("lost snapshot", err)
			}
			invocation.Input = p03Input(map[string]any{"resourceId": "different"})
			if _, err = s.InvokePluginOperation("user", "", invocation); err == nil {
				t.Fatal("input tamper allowed")
			}
			invocation.Input = p03Input(preview.Input)
			original := preview.Selection.Digest
			preview.Selection.Digest = strings.Repeat("0", 64)
			if _, err = s.InvokePluginOperation("user", "", invocation); err == nil {
				t.Fatal("digest tamper allowed")
			}
			preview.Selection.Digest = original
			if _, err = s.ComposePluginWorkbench("other", req); err == nil {
				t.Fatal("cross-user access allowed")
			}
			req.Context = contracts.InvocationContext{HostSurface: "canvas", CanvasID: "foreign-canvas"}
			if _, err = s.ComposePluginWorkbench("user", req); err == nil {
				t.Fatal("foreign canvas allowed")
			}
			req.Context = ctx
			req.Input = map[string]any{}
			incomplete, err := s.ComposePluginWorkbench("user", req)
			if err != nil || incomplete.Valid {
				t.Fatal("missing field accepted", err)
			}
			// Installing a new release does not move this user's pin. Explicitly
			// activating it rejects old launch snapshots but keeps old run history.
			files, err := authoring.ReadDirectory("../../../examples/plugins/resource-workbench")
			if err != nil {
				t.Fatal(err)
			}
			newer, err := authoring.WithVersion(files, "1.1.0")
			if err != nil {
				t.Fatal(err)
			}
			raw, err := authoring.Pack(newer, contracts.Policy{})
			if err != nil {
				t.Fatal(err)
			}
			if _, err = s.InstallManagedPluginForAdmin(&model.User{ID: "admin", Role: model.UserRoleAdmin}, raw, "newer.yingce-plugin"); err != nil {
				t.Fatal(err)
			}
			stateBefore, _ := s.repo.UserPluginState("user", "resource-workbench")
			if stateBefore.InstalledReleaseID != release {
				t.Fatal("install silently upgraded user")
			}
			newerRelease, err := s.repo.PluginReleaseByVersion("resource-workbench", "1.1.0")
			if err != nil {
				t.Fatal(err)
			}
			if err = s.ActivateApplicationPlugin(&model.User{ID: "user", Role: model.UserRoleUser}, "resource-workbench", ApplicationPluginActivation{ReleaseID: newerRelease.ID, Enabled: true, Revision: stateBefore.Revision, GrantedPermissions: []string{"media.read", "resource.create", "canvas.read", "canvas.write"}}); err != nil {
				t.Fatal(err)
			}
			if _, err = s.InvokePluginOperation("user", "", invocation); err == nil {
				t.Fatal("stale release invoked")
			}
			historical, err = s.PluginRun("user", snapshot.ID)
			if err != nil || historical.Workbench == nil || historical.Workbench.Digest != snapshotPreview.Selection.Digest {
				t.Fatal("upgrade rewrote snapshot", err)
			}
			state, _ := s.repo.UserPluginState("user", "resource-workbench")
			if err = s.ActivateApplicationPlugin(&model.User{ID: "user", Role: model.UserRoleUser}, "resource-workbench", ApplicationPluginActivation{ReleaseID: newerRelease.ID, Enabled: false, Revision: state.Revision}); err != nil {
				t.Fatal(err)
			}
			if _, err = s.InvokePluginOperation("user", "", invocation); err == nil {
				t.Fatal("disabled workbench invoked")
			}
			historical, err = s.PluginRun("user", snapshot.ID)
			if err != nil || historical.Workbench == nil || historical.Workbench.ReleaseID != release {
				t.Fatal("disable erased history", err)
			}
		})
	}
}

func TestWorkbenchAgentThreadSnapshotAndIsolation(t *testing.T) {
	s, db, _, _ := creationTestService(t)
	if err := db.Create(&model.User{ID: "user", Username: "user", Role: model.UserRoleUser, Status: model.UserStatusActive}).Error; err != nil {
		t.Fatal(err)
	}
	// The Agent fixture already has the real text model and account budget.
	release := installWorkbenchSample(t, s, "brand-workbench")
	preview, err := s.ComposePluginWorkbench("user", PluginWorkbenchComposeRequest{ID: "brand-workbench.compose", ReleaseID: release, RecipeIDs: []string{"social"}, Input: map[string]any{"brand": "品牌 A", "audience": "创作者"}, Context: contracts.InvocationContext{HostSurface: "agent-home"}})
	if err != nil || !preview.Valid {
		t.Fatal(preview, err)
	}
	thread, err := s.CreateAgentThread("user", AgentThreadCreate{ClientKey: "workbench-thread"})
	if err != nil {
		t.Fatal(err)
	}
	req := AgentThreadMessage{Revision: thread.Revision, Request: threadTestRequest()}
	req.Request.Workbench = &preview.Selection
	first, err := s.AppendAgentThreadMessage("user", thread.ID, req)
	if err != nil {
		t.Fatal(err)
	}
	_, state, err := s.cloudAgentTask("user", first.Run.ID)
	if err != nil || state.Request.Workbench == nil || state.Request.Workbench.Digest != preview.Selection.Digest || len(state.Skills) != 1 {
		t.Fatal("missing frozen workbench/skill", err)
	}
	replay, err := s.AppendAgentThreadMessage("user", thread.ID, req)
	if err != nil || replay.Run.ID != first.Run.ID {
		t.Fatal("replay differs", err)
	}
	view, err := s.GetAgentThread("user", thread.ID, 0)
	if err != nil || view.Entries[0].Context.Workbench.Digest != preview.Selection.Digest {
		t.Fatal("thread context not frozen", err)
	}
	db.Model(&model.Task{}).Where("id = ?", first.Run.ID).Updates(map[string]any{"status": model.TaskStatusSucceeded, "result_json": `{"text":"done"}`})
	db.Model(&model.CloudAgentExecution{}).Where("id = ?", first.Run.ID).Update("status", "completed")
	req.Revision = first.Thread.Revision
	req.Request.IdempotencyKey = "workbench-second"
	req.Request.Workbench = nil
	if _, err = s.AppendAgentThreadMessage("user", thread.ID, req); err == nil {
		t.Fatal("workbench removed from existing thread")
	}
	var count int64
	db.Model(&model.Task{}).Where("operation = ?", cloudAgentOperation).Count(&count)
	if count != 1 {
		t.Fatal("rejected change admitted task")
	}
}
