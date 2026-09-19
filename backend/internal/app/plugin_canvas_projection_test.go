package app

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"infinite-canvas/backend/internal/database"
	"infinite-canvas/backend/internal/kernel"
	"infinite-canvas/backend/internal/model"
	"infinite-canvas/backend/internal/plugins"
	"infinite-canvas/backend/internal/plugins/contracts"
	"infinite-canvas/backend/internal/repository"
)

func p03Fixture(t *testing.T, driver string) (*Service, *gorm.DB, string) {
	t.Helper()
	var dialector gorm.Dialector = sqlite.Open(filepath.Join(t.TempDir(), "p03.db") + "?_journal_mode=WAL&_busy_timeout=5000")
	if driver == "postgres" {
		dsn := os.Getenv("CANVAS_TEST_POSTGRES_DSN")
		if dsn == "" {
			t.Skip("isolated PostgreSQL DSN not configured")
		}
		base, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
		if err != nil {
			t.Fatal(err)
		}
		schema := "p03_" + strings.ReplaceAll(newID(), "-", "")
		if err = base.Exec(`CREATE SCHEMA "` + schema + `"`).Error; err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { base.Exec(`DROP SCHEMA "` + schema + `" CASCADE`); sql, _ := base.DB(); sql.Close() })
		u, err := url.Parse(dsn)
		if err != nil {
			t.Fatal(err)
		}
		q := u.Query()
		q.Set("search_path", schema)
		u.RawQuery = q.Encode()
		dialector = postgres.Open(u.String())
	}
	db, err := gorm.Open(dialector, &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	sql, _ := db.DB()
	sql.SetMaxOpenConns(4)
	t.Cleanup(func() { sql.Close() })
	if err = database.MigrateSchema(db); err != nil {
		t.Fatal(err)
	}
	s := &Service{repo: repository.New(db), dataDir: t.TempDir()}
	for _, row := range []any{
		&model.User{ID: "user", Username: "p03", Role: model.UserRoleUser, Status: model.UserStatusActive},
		&model.User{ID: "other", Username: "p03-other", Role: model.UserRoleUser, Status: model.UserStatusActive},
		&model.Resource{ID: "video-one", UserID: "user", Kind: "video", Status: model.ResourceStatusReady, MimeType: "video/mp4", Width: 640},
		&model.CanvasProject{ID: "canvas-one", UserID: "user", Title: "P03", PayloadJSON: `{"id":"canvas-one","nodes":[],"connections":[],"viewport":{"x":0,"y":0,"k":1}}`},
		&model.CanvasProject{ID: "foreign-canvas", UserID: "other", Title: "Foreign", PayloadJSON: `{"nodes":[],"connections":[]}`},
	} {
		if err = db.Create(row).Error; err != nil {
			t.Fatal(err)
		}
	}
	var archive bytes.Buffer
	w := zip.NewWriter(&archive)
	root := "../plugins/contracts/testdata/resource-helper-p03"
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
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
	if _, err = s.InstallManagedPluginForAdmin(&model.User{ID: "admin", Role: model.UserRoleAdmin}, archive.Bytes(), "p03.yingce-plugin"); err != nil {
		t.Fatal(err)
	}
	release, err := s.repo.PluginReleaseByVersion("resource-helper", "1.3.0")
	if err != nil || release == nil {
		t.Fatal(err)
	}
	if err = s.ActivateApplicationPlugin(&model.User{ID: "user", Role: model.UserRoleUser}, "resource-helper", ApplicationPluginActivation{ReleaseID: release.ID, Enabled: true, GrantedPermissions: []string{"media.read", "resource.create", "canvas.read", "canvas.write"}}); err != nil {
		t.Fatal(err)
	}
	return s, db, release.ID
}

func p03Input(values map[string]any) map[string]json.RawMessage {
	raw, _ := json.Marshal(values)
	var input map[string]json.RawMessage
	_ = json.Unmarshal(raw, &input)
	return input
}
func p03Snapshot(t *testing.T, s *Service, release string) plugins.RunView {
	t.Helper()
	read, err := s.InvokePluginOperation("user", "", p02Request(release, "inspect-video", "video-one"))
	if err != nil {
		t.Fatal(err)
	}
	request := p02Request(release, "snapshot-video", "video-one")
	request.Input["expectedDigest"], _ = json.Marshal(read.Digest)
	output, err := s.InvokePluginOperation("user", newID(), request)
	if err != nil {
		t.Fatal(err)
	}
	view, err := s.DecidePluginRun("user", output.RunID, output.ApprovalID, "approve", output.Revision)
	if err != nil {
		t.Fatal(err)
	}
	return view
}
func p03Projection(t *testing.T, s *Service, source plugins.RunView, instance string) contracts.Invocation {
	t.Helper()
	snapshot, err := s.PluginCanvasSnapshot("user", "canvas-one")
	if err != nil {
		t.Fatal(err)
	}
	return contracts.Invocation{Operation: "resource-helper.place-result", ReleaseID: source.ReleaseID, Context: &contracts.InvocationContext{HostSurface: "canvas", CanvasID: "canvas-one"}, Input: p03Input(map[string]any{"runId": source.ID, "resultDigest": source.ResultRef.Digest, "blueprintId": "inspect-board", "snapshotHash": snapshot["snapshotHash"], "instanceKey": instance})}
}
func p03Approve(t *testing.T, s *Service, output plugins.InvocationOutput) plugins.RunView {
	t.Helper()
	view, err := s.DecidePluginRun("user", output.RunID, output.ApprovalID, "approve", output.Revision)
	if err != nil {
		t.Fatal(err)
	}
	return view
}
func p03NodeCount(t *testing.T, s *Service) int {
	t.Helper()
	canvas, err := s.repo.CanvasProjectForUser("user", "canvas-one")
	if err != nil {
		t.Fatal(err)
	}
	doc, err := creationDocument(canvas.PayloadJSON)
	if err != nil {
		t.Fatal(err)
	}
	return len(creationMaps(doc["nodes"]))
}

func TestP03ProjectionLifecycle(t *testing.T) {
	for _, driver := range []string{"sqlite", "postgres"} {
		t.Run(driver, func(t *testing.T) {
			s, db, release := p03Fixture(t, driver)
			source := p03Snapshot(t, s, release)
			if source.View == nil || len(source.CanvasActions) != 1 {
				t.Fatal("missing declarative presentation/actions")
			}
			request := p03Projection(t, s, source, "default")
			first, err := s.InvokePluginOperation("user", "project-first", request)
			if err != nil {
				t.Fatal(err)
			}
			second, err := s.InvokePluginOperation("user", "project-second", request)
			if err != nil {
				t.Fatal(err)
			}
			if p03NodeCount(t, s) != 0 || first.Status != "waiting_approval" {
				t.Fatal("projection wrote before approval")
			}
			// Two admitted requests approving concurrently still produce one binding.
			var wg sync.WaitGroup
			errs := make(chan error, 2)
			for _, out := range []plugins.InvocationOutput{first, second} {
				wg.Add(1)
				go func(out plugins.InvocationOutput) {
					defer wg.Done()
					_, err := s.DecidePluginRun("user", out.RunID, out.ApprovalID, "approve", out.Revision)
					errs <- err
				}(out)
			}
			wg.Wait()
			close(errs)
			for err := range errs {
				if err != nil {
					t.Fatal(err)
				}
			}
			if p03NodeCount(t, s) != 1 {
				t.Fatal("duplicate nodes")
			}
			var count int64
			db.Model(&model.PluginCanvasProjection{}).Count(&count)
			if count != 1 {
				t.Fatal("duplicate binding")
			}
			resumed := &Service{repo: repository.New(db), dataDir: s.dataDir}
			replay, err := resumed.InvokePluginOperation("user", "project-first", request)
			if err != nil || replay.RunID != first.RunID {
				t.Fatal("retry identity lost", err)
			}
			p03Approve(t, resumed, replay)
			// Successful result remains unchanged after projection, and source stays protected.
			got, err := resumed.PluginRun("user", source.ID)
			if err != nil || !bytes.Equal(got.Result, source.Result) {
				t.Fatal("business result changed", err)
			}
			refs, err := s.repo.ResourceReferenceSnapshot("user", "", []string{"video-one"})
			if err != nil || len(refs.Direct) == 0 {
				t.Fatal("source reference lost", err)
			}
			canvas, _ := s.repo.CanvasProjectForUser("user", "canvas-one")
			doc, _ := creationDocument(canvas.PayloadJSON)
			nodes := creationMaps(doc["nodes"])
			nodes[0]["createdAt"] = "2026-09-19T00:00:00Z"
			doc["nodes"] = nodes
			hydrated, _ := json.Marshal(doc)
			if err = db.Model(canvas).Update("payload_json", string(hydrated)).Error; err != nil {
				t.Fatal(err)
			}
			duplicate, err := s.InvokePluginOperation("user", "hydrated-node", p03Projection(t, s, source, "default"))
			if err != nil {
				t.Fatal(err)
			}
			p03Approve(t, s, duplicate)
			nodes[0]["title"] = "用户修改的名称"
			doc["nodes"] = nodes
			raw, _ := json.Marshal(doc)
			if err = db.Model(canvas).Update("payload_json", string(raw)).Error; err != nil {
				t.Fatal(err)
			}
			_, err = s.InvokePluginOperation("user", "changed-node", p03Projection(t, s, source, "default"))
			var appErr *kernel.AppError
			if !errors.As(err, &appErr) || appErr.Reason != "projection_conflict" {
				t.Fatalf("modified node not protected: %v", err)
			}
			fresh, err := s.InvokePluginOperation("user", "new-instance", p03Projection(t, s, source, "another"))
			if err != nil {
				t.Fatal(err)
			}
			p03Approve(t, s, fresh)
			if p03NodeCount(t, s) != 2 {
				t.Fatal("new instance failed")
			}
			canvas, _ = s.repo.CanvasProjectForUser("user", "canvas-one")
			if !strings.Contains(canvas.PayloadJSON, "用户修改的名称") {
				t.Fatal("overwrote user change")
			}
			layout, _ := creationDocument(canvas.PayloadJSON)
			placed := creationMaps(layout["nodes"])
			left := placed[0]["position"].(map[string]any)["x"].(float64)
			right := placed[1]["position"].(map[string]any)["x"].(float64)
			if right <= left+placed[0]["width"].(float64) {
				t.Fatal("automatic layout overlaps existing result")
			}
			doc, _ = creationDocument(canvas.PayloadJSON)
			doc["nodes"] = []any{}
			raw, _ = json.Marshal(doc)
			if err = db.Model(canvas).Update("payload_json", string(raw)).Error; err != nil {
				t.Fatal(err)
			}
			if _, err = s.InvokePluginOperation("user", "deleted-node", p03Projection(t, s, source, "another")); err == nil {
				t.Fatal("deleted node silently restored")
			}
		})
	}
}

func TestP03ProjectionScopeAndStaleApproval(t *testing.T) {
	s, db, release := p03Fixture(t, "sqlite")
	source := p03Snapshot(t, s, release)
	request := p03Projection(t, s, source, "default")
	if _, err := s.applicationPlugins().Invoke("user", "readonly-key", request, plugins.InvocationPolicy{PermissionMode: "read_only"}); err == nil {
		t.Fatal("read-only mutation")
	}
	for _, change := range []func(*contracts.Invocation){
		func(r *contracts.Invocation) { r.Context = nil },
		func(r *contracts.Invocation) { r.Context = &contracts.InvocationContext{CanvasID: "foreign-canvas"} },
		func(r *contracts.Invocation) {
			r.Input["resultDigest"] = json.RawMessage(`"` + strings.Repeat("0", 64) + `"`)
		},
		func(r *contracts.Invocation) { r.Input["runId"] = json.RawMessage(`"missing"`) },
	} {
		bad := p03Projection(t, s, source, "default")
		change(&bad)
		if _, err := s.InvokePluginOperation("user", newID(), bad); err == nil {
			t.Fatal("invalid binding accepted")
		}
	}
	if _, err := s.PluginRun("other", source.ID); err == nil {
		t.Fatal("foreign run leaked")
	}
	output, err := s.InvokePluginOperation("user", "stale-approval", request)
	if err != nil {
		t.Fatal(err)
	}
	canvas, _ := s.repo.CanvasProjectForUser("user", "canvas-one")
	doc, _ := creationDocument(canvas.PayloadJSON)
	doc["title"] = "changed"
	raw, _ := json.Marshal(doc)
	if err = db.Model(canvas).Update("payload_json", string(raw)).Error; err != nil {
		t.Fatal(err)
	}
	if _, err = s.DecidePluginRun("user", output.RunID, output.ApprovalID, "approve", output.Revision); err == nil {
		t.Fatal("stale approval accepted")
	}
	view, err := s.PluginRun("user", output.RunID)
	if err != nil || view.ProjectionStatus != "conflict" || view.Status != "waiting_approval" {
		t.Fatalf("lost conflict: %+v %v", view, err)
	}
	if p03NodeCount(t, s) != 0 {
		t.Fatal("partial write")
	}
	if _, err = s.CancelPluginRun("user", view.ID, view.Revision); err != nil {
		t.Fatal(err)
	}
	if _, err = s.DecidePluginRun("user", output.RunID, output.ApprovalID, "approve", output.Revision); err == nil {
		t.Fatal("cancelled approval accepted")
	}
	refreshed, err := s.InvokePluginOperation("user", "after-refresh", p03Projection(t, s, source, "default"))
	if err != nil {
		t.Fatal(err)
	}
	p03Approve(t, s, refreshed)
	if p03NodeCount(t, s) != 1 {
		t.Fatal("rebind failed")
	}
	if _, err = applyCloudAgentCanvasPlan(map[string]any{"nodes": []any{}}, []agentCanvasOp{{Type: "add_node", ID: "forged", NodeType: "plugin-result"}}); err == nil {
		t.Fatal("generic tool forged result")
	}
}

func TestP03AgentUsesGenericOperationForCanvas(t *testing.T) {
	s, db, _ := p02Fixture(t)
	pkg, err := os.ReadFile("../../../plugin-packages/application-examples/resource-helper-1.3.0.yingce-plugin")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.InstallManagedPluginForAdmin(&model.User{ID: "admin", Role: model.UserRoleAdmin}, pkg, "p03.yingce-plugin"); err != nil {
		t.Fatal(err)
	}
	release, err := s.repo.PluginReleaseByVersion("resource-helper", "1.3.0")
	if err != nil {
		t.Fatal(err)
	}
	stateRow, err := s.repo.UserPluginState("user", "resource-helper")
	if err != nil {
		t.Fatal(err)
	}
	if err = s.ActivateApplicationPlugin(&model.User{ID: "user", Role: model.UserRoleUser}, "resource-helper", ApplicationPluginActivation{ReleaseID: release.ID, Enabled: true, Revision: stateRow.Revision, GrantedPermissions: []string{"media.read", "resource.create", "canvas.read", "canvas.write"}}); err != nil {
		t.Fatal(err)
	}
	if err = db.Create(&model.CanvasProject{ID: "canvas-one", UserID: "user", Title: "P03", PayloadJSON: `{"id":"canvas-one","nodes":[],"connections":[]}`}).Error; err != nil {
		t.Fatal(err)
	}
	source := p03Snapshot(t, s, release.ID)
	req := agentTestRequest()
	req.CanvasID = "canvas-one"
	req.PermissionMode = "auto"
	root, err := s.CreateCloudAgentRun("user", req, "")
	if err != nil {
		t.Fatal(err)
	}
	describe, _ := json.Marshal(map[string]any{"operation": "resource-helper.place-result", "releaseId": release.ID})
	state := p02AgentTool(t, s, root.ID, "operation_describe", string(describe), "describe-blueprint")
	if state.PluginLocks["resource-helper.place-result"].ReleaseID != release.ID {
		t.Fatal("blueprint contract not locked")
	}
	request := p03Projection(t, s, source, "agent")
	args, _ := json.Marshal(map[string]any{"operation": request.Operation, "input": request.Input})
	state = p02AgentTool(t, s, root.ID, "operation_invoke", string(args), "invoke-blueprint")
	if state.PendingExecution == nil {
		t.Fatalf("no plugin approval reference: %+v", state.Canonical.Messages)
	}
	view, err := s.PluginRun("user", state.PendingExecution.ID)
	if err != nil || view.Status != "waiting_approval" || p03NodeCount(t, s) != 0 {
		t.Fatal("Agent auto bypassed approval", err)
	}
	if _, err = s.DecidePluginRun("user", view.ID, view.ApprovalID, "approve", view.Revision); err != nil {
		t.Fatal(err)
	}
	state = p02AgentTool(t, s, root.ID, "result_read", `{"runId":"`+view.ID+`"}`, "read-projection")
	last := state.Canonical.Messages[len(state.Canonical.Messages)-1]
	if !strings.Contains(last["content"].(string), "source-info") || p03NodeCount(t, s) != 1 {
		t.Fatal("Agent did not receive durable canvas receipt")
	}
}

func TestP03ObservedDigestAndRevocation(t *testing.T) {
	s, db, release := p03Fixture(t, "sqlite")
	request := p02Request(release, "snapshot-video", "video-one")
	request.Input["expectedDigest"] = json.RawMessage(`"` + strings.Repeat("f", 64) + `"`)
	if _, err := s.InvokePluginOperation("user", "forged-digest", request); err == nil {
		t.Fatal("forged inline fact accepted")
	}
	source := p03Snapshot(t, s, release)
	projection, err := s.InvokePluginOperation("user", "pending-revocation", p03Projection(t, s, source, "default"))
	if err != nil {
		t.Fatal(err)
	}
	if err = db.Model(&model.UserPluginState{}).Where("user_id=? AND plugin_id=?", "user", "resource-helper").Update("granted_permissions_json", `["media.read","resource.create","canvas.read"]`).Error; err != nil {
		t.Fatal(err)
	}
	if _, err = s.DecidePluginRun("user", projection.RunID, projection.ApprovalID, "approve", projection.Revision); err == nil {
		t.Fatal("revoked canvas.write still committed")
	}
	if p03NodeCount(t, s) != 0 {
		t.Fatal("revoked write created nodes")
	}
	if err = db.Model(&model.UserPluginState{}).Where("user_id=?", "user").Update("enabled", false).Error; err != nil {
		t.Fatal(err)
	}
	if got, err := s.PluginRun("user", source.ID); err != nil || got.ResultRef == nil || got.View == nil {
		t.Fatal("disabled plugin lost result", err)
	}
	if _, err = s.InvokePluginOperation("user", "disabled-read", p02Request(release, "inspect-video", "video-one")); err == nil {
		t.Fatal("disabled plugin executable")
	}
}

func TestP03ProjectionCommitRollsBackCanvasAndHistory(t *testing.T) {
	s, db, release := p03Fixture(t, "sqlite")
	source := p03Snapshot(t, s, release)
	output, err := s.InvokePluginOperation("user", "rollback-projection", p03Projection(t, s, source, "default"))
	if err != nil {
		t.Fatal(err)
	}
	var before int64
	if err = db.Model(&model.CanvasSnapshot{}).Count(&before).Error; err != nil {
		t.Fatal(err)
	}
	if err = db.Exec(`CREATE TRIGGER deny_plugin_projection BEFORE INSERT ON plugin_canvas_projections BEGIN SELECT RAISE(ABORT, 'injected binding write failure'); END`).Error; err != nil {
		t.Fatal(err)
	}
	if _, err = s.DecidePluginRun("user", output.RunID, output.ApprovalID, "approve", output.Revision); err == nil {
		t.Fatal("binding failure ignored")
	}
	if p03NodeCount(t, s) != 0 {
		t.Fatal("canvas committed without binding")
	}
	var after int64
	if err = db.Model(&model.CanvasSnapshot{}).Count(&after).Error; err != nil || after != before {
		t.Fatal("history committed without binding", err)
	}
	view, err := s.PluginRun("user", output.RunID)
	if err != nil || view.Status != "waiting_approval" || view.ResultRef != nil {
		t.Fatal("failed write published success", err)
	}
	if err = db.Exec(`DROP TRIGGER deny_plugin_projection`).Error; err != nil {
		t.Fatal(err)
	}
	p03Approve(t, s, output)
	if p03NodeCount(t, s) != 1 {
		t.Fatal("same approval could not recover")
	}
}
