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
	"testing"

	"gorm.io/gorm"
	"infinite-canvas/backend/internal/model"
	"infinite-canvas/backend/internal/plugins"
	"infinite-canvas/backend/internal/plugins/contracts"
	"infinite-canvas/backend/internal/repository"
)

func p02Package(t *testing.T, change func(map[string][]byte)) []byte {
	t.Helper()
	files := map[string][]byte{}
	root := "../plugins/contracts/testdata/resource-helper"
	if err := filepath.WalkDir(root, func(name string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		raw, err := os.ReadFile(name)
		files[strings.TrimPrefix(filepath.ToSlash(name), root+"/")] = raw
		return err
	}); err != nil {
		t.Fatal(err)
	}
	var manifest map[string]any
	json.Unmarshal(files["manifest.json"], &manifest)
	manifest["version"] = "1.2.0"
	manifest["permissions"] = []string{"media.read", "resource.create", "canvas.read", "canvas.write"}
	c := manifest["contributes"].(map[string]any)
	c["operations"] = append(c["operations"].([]any), map[string]any{"id": "snapshot-video", "ref": "operations/snapshot-video.json"})
	files["manifest.json"], _ = json.Marshal(manifest)
	var op map[string]any
	json.Unmarshal(files["operations/inspect-video.json"], &op)
	op["id"] = "snapshot-video"
	op["description"] = "保存视频元数据快照"
	op["requiredPermissions"] = []string{"media.read", "resource.create"}
	op["effects"] = []string{"draft_write"}
	op["execution"] = map[string]any{"kind": "host", "adapter": "resource.snapshot", "mode": "inline"}
	files["operations/snapshot-video.json"], _ = json.Marshal(op)
	if change != nil {
		change(files)
	}
	var buffer bytes.Buffer
	w := zip.NewWriter(&buffer)
	for name, raw := range files {
		entry, err := w.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		entry.Write(raw)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}
func p02Fixture(t *testing.T) (*Service, *gorm.DB, string) {
	t.Helper()
	s, db, _, _ := creationTestService(t)
	for _, v := range []any{
		&model.User{ID: "user", Username: "plugin-user", Role: model.UserRoleUser, Status: model.UserStatusActive},
		&model.User{ID: "other", Username: "plugin-other", Role: model.UserRoleUser, Status: model.UserStatusActive},
		&model.Resource{ID: "video-one", UserID: "user", Kind: "video", Status: model.ResourceStatusReady, MimeType: "video/mp4", Width: 640, Height: 480, PublicURL: "https://private.invalid/video?signature=never-leak"},
		&model.Resource{ID: "foreign-video", UserID: "other", Kind: "video", Status: model.ResourceStatusReady, MimeType: "video/mp4"},
	} {
		if err := db.Create(v).Error; err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.InstallManagedPluginForAdmin(&model.User{ID: "admin", Role: model.UserRoleAdmin}, p02Package(t, nil), "sample.yingce-plugin"); err != nil {
		t.Fatal(err)
	}
	r, err := s.repo.PluginReleaseByVersion("resource-helper", "1.2.0")
	if err != nil || r == nil {
		t.Fatal(err)
	}
	if err = s.ActivateApplicationPlugin(&model.User{ID: "user", Role: model.UserRoleUser}, "resource-helper", ApplicationPluginActivation{ReleaseID: r.ID, Enabled: true, GrantedPermissions: []string{"media.read", "resource.create"}}); err != nil {
		t.Fatal(err)
	}
	return s, db, r.ID
}
func p02Request(release, operation, resource string) contracts.Invocation {
	raw, _ := json.Marshal(resource)
	return contracts.Invocation{Operation: "resource-helper." + operation, ReleaseID: release, Input: map[string]json.RawMessage{"resourceId": raw}}
}

func TestP02ReadAndDurableApproval(t *testing.T) {
	s, db, release := p02Fixture(t)
	read, err := s.InvokePluginOperation("user", "", p02Request(release, "inspect-video", "video-one"))
	if err != nil || read.Kind != "inline" {
		t.Fatalf("read %v %v", read, err)
	}
	if bytes.Contains(read.Result, []byte("never-leak")) || bytes.Contains(read.Result, []byte("durationMs")) {
		t.Fatal("secret or fabricated metadata exposed")
	}
	var count int64
	db.Model(&model.PluginRun{}).Count(&count)
	if count != 0 {
		t.Fatal("read created run")
	}
	write := p02Request(release, "snapshot-video", "video-one")
	output, err := s.InvokePluginOperation("user", "snapshot-key", write)
	if err != nil || output.Status != "waiting_approval" {
		t.Fatalf("snapshot %v %v", output, err)
	}
	duplicate, err := s.InvokePluginOperation("user", "snapshot-key", write)
	if err != nil || duplicate.RunID != output.RunID {
		t.Fatal("duplicate durable request", err)
	}
	before, err := s.PluginRun("user", output.RunID)
	if err != nil || len(before.Result) > 0 {
		t.Fatal("unapproved output published", err)
	}
	if _, err = s.PluginRun("other", output.RunID); err == nil {
		t.Fatal("foreign run readable")
	}
	approved, err := s.DecidePluginRun("user", output.RunID, output.ApprovalID, "approve", output.Revision)
	if err != nil || approved.Status != "succeeded" || approved.ResultRef == nil {
		t.Fatalf("approval %v", err)
	}
	if _, err = s.DecidePluginRun("user", output.RunID, output.ApprovalID, "approve", output.Revision); err != nil {
		t.Fatal("approval retry", err)
	}
	db.Model(&model.PluginRun{}).Count(&count)
	if count != 1 {
		t.Fatal("duplicate run")
	}
	events, err := s.repo.PluginRunEvents("user", output.RunID, 0)
	if err != nil || len(events) != 4 {
		t.Fatalf("events %d %v", len(events), err)
	}
	for i, event := range events {
		if event.Sequence != int64(i+1) {
			t.Fatal("event sequence")
		}
	}
	snapshot, err := s.repo.ResourceReferenceSnapshot("user", "", []string{"video-one"})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, ref := range snapshot.Direct {
		found = found || ref.ID == output.RunID
	}
	if !found {
		t.Fatal("result source deletion unprotected")
	}
}

func TestP02RejectsScopeSchemaAndChangedInput(t *testing.T) {
	s, db, release := p02Fixture(t)
	for _, resource := range []string{"foreign-video", "missing"} {
		if _, err := s.InvokePluginOperation("user", "", p02Request(release, "inspect-video", resource)); err == nil {
			t.Fatal("invalid ownership accepted")
		}
	}
	req := p02Request(release, "inspect-video", "video-one")
	req.Input["unexpected"] = json.RawMessage("true")
	if _, err := s.InvokePluginOperation("user", "", req); err == nil {
		t.Fatal("invalid schema")
	}
	req = p02Request(release, "snapshot-video", "video-one")
	if _, err := s.applicationPlugins().Invoke("user", "read-only-key", req, plugins.InvocationPolicy{PermissionMode: "read_only"}); err == nil {
		t.Fatal("read-only mode wrote")
	}
	output, err := s.InvokePluginOperation("user", "changed-source", req)
	if err != nil {
		t.Fatal(err)
	}
	if err = db.Model(&model.Resource{}).Where("id=?", "video-one").Update("width", 800).Error; err != nil {
		t.Fatal(err)
	}
	if _, err = s.DecidePluginRun("user", output.RunID, output.ApprovalID, "approve", output.Revision); err == nil {
		t.Fatal("stale approval used changed source")
	}
	app, _ := s.repo.PluginApplication("resource-helper")
	if err = s.ManageApplicationPlugin(&model.User{ID: "admin", Role: model.UserRoleAdmin}, app.ID, ApplicationPluginManagement{Action: "availability", Available: false, Revision: app.Revision}); err != nil {
		t.Fatal(err)
	}
	if _, err = s.InvokePluginOperation("user", "", p02Request(release, "inspect-video", "video-one")); err == nil {
		t.Fatal("disabled plugin invoked")
	}
	if _, err = s.CancelPluginRun("user", output.RunID, output.Revision); err != nil {
		t.Fatal("disabled plugin cannot be cancelled", err)
	}
}

func TestP02ConcurrentInvocationAndCancellation(t *testing.T) {
	s, db, release := p02Fixture(t)
	req := p02Request(release, "snapshot-video", "video-one")
	results := make(chan plugins.InvocationOutput, 2)
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			v, e := s.InvokePluginOperation("user", "concurrent-p02", req)
			results <- v
			errs <- e
		}()
	}
	wg.Wait()
	first, second := <-results, <-results
	for i := 0; i < 2; i++ {
		if err := <-errs; err != nil {
			t.Fatal(err)
		}
	}
	if first.RunID != second.RunID {
		t.Fatal("duplicate runs")
	}
	if _, err := s.CancelPluginRun("user", first.RunID, first.Revision); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DecidePluginRun("user", first.RunID, first.ApprovalID, "approve", first.Revision); err == nil {
		t.Fatal("approval executed cancelled run")
	}
	var count int64
	db.Model(&model.PluginRun{}).Count(&count)
	if count != 1 {
		t.Fatal(count)
	}
}

func TestP02StoppedAgentCannotAdmitSnapshot(t *testing.T) {
	s, db, release := p02Fixture(t)
	req := agentTestRequest()
	req.HostSurface = "agent-home"
	req.CanvasID = ""
	req.ContextScope = nil
	req.PermissionMode = "auto"
	root, err := s.CreateCloudAgentRun("user", req, "")
	if err != nil {
		t.Fatal(err)
	}
	run, err := s.repo.CloudAgent("user", root.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err = db.Model(&model.CloudAgentExecution{}).Where("id=?", root.ID).Update("status", "cancelled").Error; err != nil {
		t.Fatal(err)
	}
	if _, err = s.applicationPlugins().Invoke("user", "stopped-agent", p02Request(release, "snapshot-video", "video-one"), plugins.InvocationPolicy{PermissionMode: "auto", AgentRunID: root.ID, AgentRevision: run.Revision}); err == nil {
		t.Fatal("cancelled Agent created a snapshot")
	}
	var count int64
	db.Model(&model.PluginRun{}).Count(&count)
	if count != 0 {
		t.Fatal("cancelled Agent persisted a run")
	}
	output, err := s.applicationPlugins().Invoke("user", "auto-still-confirms", p02Request(release, "snapshot-video", "video-one"), plugins.InvocationPolicy{PermissionMode: "auto"})
	if err != nil || output.Status != "waiting_approval" {
		t.Fatalf("auto bypassed user confirmation: %+v %v", output, err)
	}
}

func p02AgentTool(t *testing.T, s *Service, id, name, arguments, callID string) cloudAgentRuntime {
	t.Helper()
	run, err := s.repo.CloudAgent("user", id)
	if err != nil {
		t.Fatal(err)
	}
	state, err := cloudAgentDecode(run)
	if err != nil {
		t.Fatal(err)
	}
	call := cloudAgentCall{ID: callID}
	call.Function.Name = name
	call.Function.Arguments = arguments
	state.ActiveTaskID = ""
	state.Calls = []cloudAgentCall{call}
	state.CallIndex = 0
	if err = s.repo.MutateCloudAgent("user", id, run.Revision, func(current *model.CloudAgentExecution, _ *repository.Repository) error {
		return cloudAgentSave(current, &state)
	}); err != nil {
		t.Fatal(err)
	}
	run, _ = s.repo.CloudAgent("user", id)
	state, err = cloudAgentDecode(run)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.advanceCloudAgentTool(run, &state); err != nil {
		t.Fatal(err)
	}
	run, _ = s.repo.CloudAgent("user", id)
	state, err = cloudAgentDecode(run)
	if err != nil {
		t.Fatal(err)
	}
	return state
}
func TestP02CanvaslessAgentToolsAndRecovery(t *testing.T) {
	s, db, release := p02Fixture(t)
	req := agentTestRequest()
	req.HostSurface = "agent-home"
	req.CanvasID = ""
	req.ContextScope = nil
	req.PermissionMode = "request_approval"
	root, err := s.CreateCloudAgentRun("user", req, "")
	if err != nil {
		t.Fatal("canvasless admission", err)
	}
	run, _ := s.repo.CloudAgent("user", root.ID)
	state, err := cloudAgentDecodeForExecution(run)
	if err != nil {
		t.Fatal("recovery", err)
	}
	for _, name := range []string{"canvas_apply_ops", "generate_media", "image_layer_split", "canvas_get_state"} {
		if cloudAgentToolAllowed(state.Request, name) {
			t.Fatal("canvasless tool exposed", name)
		}
	}
	if !cloudAgentToolAllowed(state.Request, "operation_invoke") {
		t.Fatal("plugin tools missing")
	}
	describe, _ := json.Marshal(map[string]any{"operation": "resource-helper.inspect-video", "releaseId": release})
	state = p02AgentTool(t, s, root.ID, "operation_describe", string(describe), "describe")
	if state.PluginLocks["resource-helper.inspect-video"].ReleaseID != release {
		t.Fatal("version not locked")
	}
	args, _ := json.Marshal(map[string]any{"operation": "resource-helper.inspect-video", "input": map[string]any{"resourceId": "video-one"}})
	state = p02AgentTool(t, s, root.ID, "operation_invoke", string(args), "inspect")
	last := state.Canonical.Messages[len(state.Canonical.Messages)-1]
	if !strings.Contains(last["content"].(string), "video-one") {
		t.Fatalf("Agent invocation %v", last)
	}
	var canvases int64
	db.Model(&model.CanvasProject{}).Count(&canvases)
	if canvases != 0 {
		t.Fatal("fake canvas created")
	}
	describe, _ = json.Marshal(map[string]any{"operation": "resource-helper.snapshot-video", "releaseId": release})
	p02AgentTool(t, s, root.ID, "operation_describe", string(describe), "describe-write")
	args, _ = json.Marshal(map[string]any{"operation": "resource-helper.snapshot-video", "input": map[string]any{"resourceId": "video-one"}})
	state = p02AgentTool(t, s, root.ID, "operation_invoke", string(args), "write")
	if state.PendingExecution == nil {
		t.Fatal("approval reference missing")
	}
	if state.Approval != nil {
		t.Fatal("plugin approval copied into canvas approval")
	}
	first := state.PendingExecution.ID
	readArgs, _ := json.Marshal(map[string]any{"operation": "resource-helper.inspect-video", "input": map[string]any{"resourceId": "video-one"}})
	state = p02AgentTool(t, s, root.ID, "operation_invoke", string(readArgs), "read-while-waiting")
	last = state.Canonical.Messages[len(state.Canonical.Messages)-1]
	if !strings.Contains(last["content"].(string), `"kind":"inline"`) {
		t.Fatalf("pending snapshot replaced a read: %v", last)
	}
	if state.PendingExecution == nil || state.PendingExecution.ID != first {
		t.Fatal("read discarded pending snapshot")
	}
	state = p02AgentTool(t, s, root.ID, "operation_invoke", string(args), "write")
	if state.PendingExecution.ID != first {
		t.Fatal("Agent replay created duplicate run")
	}
}

func TestP02PinnedVersionAndCanvasRequirement(t *testing.T) {
	s, db, oldRelease := p02Fixture(t)
	data := p02Package(t, func(files map[string][]byte) {
		var manifest map[string]any
		json.Unmarshal(files["manifest.json"], &manifest)
		manifest["version"] = "1.3.0"
		files["manifest.json"], _ = json.Marshal(manifest)
		var op map[string]any
		json.Unmarshal(files["operations/inspect-video.json"], &op)
		op["context"] = map[string]any{"requiresCanvas": true, "requiresProject": false}
		files["operations/inspect-video.json"], _ = json.Marshal(op)
	})
	if _, err := s.InstallManagedPluginForAdmin(&model.User{ID: "admin", Role: model.UserRoleAdmin}, data, "canvas-required.yingce-plugin"); err != nil {
		t.Fatal(err)
	}
	release, err := s.repo.PluginReleaseByVersion("resource-helper", "1.3.0")
	if err != nil || release == nil {
		t.Fatal(err)
	}
	if err = s.ActivateApplicationPlugin(&model.User{ID: "user", Role: model.UserRoleUser}, "resource-helper", ApplicationPluginActivation{ReleaseID: release.ID, Enabled: true, GrantedPermissions: []string{"media.read", "resource.create"}, Revision: 1}); err != nil {
		t.Fatal(err)
	}
	if _, err = s.InvokePluginOperation("user", "", p02Request(oldRelease, "inspect-video", "video-one")); err == nil {
		t.Fatal("old lock silently switched to new version")
	}
	if _, err = s.DescribePluginOperation("user", "resource-helper.inspect-video", release.ID, contracts.InvocationContext{HostSurface: "agent-home"}); err == nil {
		t.Fatal("required canvas was fabricated")
	}
	if err = db.Create(&model.CanvasProject{ID: "owned-canvas", UserID: "user", Title: "test", PayloadJSON: `{"nodes":[]}`}).Error; err != nil {
		t.Fatal(err)
	}
	if _, err = s.DescribePluginOperation("user", "resource-helper.inspect-video", release.ID, contracts.InvocationContext{HostSurface: "canvas", CanvasID: "owned-canvas"}); err != nil {
		t.Fatal(err)
	}
}

func TestP02CanvaslessAgentWorkerToolSurface(t *testing.T) {
	t.Setenv("CANVAS_ALLOWED_PRIVATE_UPSTREAM_HOSTS", "127.0.0.1")
	t.Setenv("REDIS_URL", "")
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		encoded, _ := json.Marshal(body["tools"])
		text := string(encoded)
		for _, required := range []string{"operation_search", "operation_describe", "operation_invoke", "plugin_run_get", "result_read"} {
			if !strings.Contains(text, required) {
				t.Errorf("missing plugin tool %s: %s", required, text)
			}
		}
		for _, forbidden := range []string{"canvas_apply_ops", "canvas_get_state", "generate_media", "task_get"} {
			if strings.Contains(text, forbidden) {
				t.Errorf("canvasless request exposed %s", forbidden)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"插件能力目录已就绪"}}]}`))
	}))
	defer upstream.Close()
	s, db, _ := p02Fixture(t)
	s = New(s.repo, s.dataDir)
	t.Cleanup(func() { _ = s.Close() })
	if err := db.Model(&model.ModelChannel{}).Where("id = ?", "channel").Updates(map[string]any{"base_url": upstream.URL, "api_key": "test-only"}).Error; err != nil {
		t.Fatal(err)
	}
	req := agentTestRequest()
	req.HostSurface, req.CanvasID, req.ContextScope = "agent-home", "", nil
	run, err := s.CreateCloudAgentRun("user", req, "")
	if err != nil {
		t.Fatal(err)
	}
	if err = s.ProcessNextTask(); err != nil {
		t.Fatal(err)
	}
	var task model.Task
	if err = db.First(&task, "id = ?", run.ID).Error; err != nil {
		t.Fatal(err)
	}
	if task.Status != model.TaskStatusSucceeded || taskResultText(task.ResultJSON) != "插件能力目录已就绪" || task.ProjectID != "" {
		t.Fatalf("canvasless worker result: %+v", task)
	}
}
