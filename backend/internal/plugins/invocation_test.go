package plugins

import (
	"encoding/json"
	"testing"

	"infinite-canvas/backend/internal/model"
	"infinite-canvas/backend/internal/plugins/contracts"
	"infinite-canvas/backend/internal/repository"
)

func invocationPackage(t *testing.T) []byte {
	return samplePackage(t, func(files map[string][]byte) {
		changeManifest(files, func(manifest map[string]any) {
			manifest["version"] = "1.2.0"
			manifest["permissions"] = []any{"media.read", "resource.create"}
			contributes := manifest["contributes"].(map[string]any)
			contributes["operations"] = append(contributes["operations"].([]any), map[string]any{"id": "snapshot-video", "ref": "operations/snapshot-video.json"})
		})
		var op map[string]any
		_ = json.Unmarshal(files["operations/inspect-video.json"], &op)
		op["id"] = "snapshot-video"
		op["requiredPermissions"] = []any{"media.read", "resource.create"}
		op["effects"] = []any{"draft_write"}
		op["execution"] = map[string]any{"kind": "host", "adapter": "resource.snapshot", "mode": "inline"}
		files["operations/snapshot-video.json"], _ = json.Marshal(op)
	})
}

func invocationAdapters() []ShortHostAdapter {
	prepare := func(repo *repository.Repository, userID string, input map[string]json.RawMessage, _ contracts.InvocationContext) (PreparedOperation, error) {
		var id string
		_ = json.Unmarshal(input["resourceId"], &id)
		resource, err := repo.LockResourceForUser(userID, id)
		if err != nil {
			return PreparedOperation{}, err
		}
		raw, _ := json.Marshal(map[string]any{"resourceId": resource.ID, "kind": "video", "name": resource.ID, "mimeType": resource.MimeType})
		return PreparedOperation{Result: raw, SourceResourceID: resource.ID, SourceDigest: hashBytes(raw)}, nil
	}
	return []ShortHostAdapter{
		{ID: "resource.inspect", Permissions: []string{"media.read"}, Effects: []string{"read"}, Prepare: prepare},
		{ID: "resource.snapshot", Permissions: []string{"media.read", "resource.create"}, Effects: []string{"draft_write"}, Prepare: prepare},
	}
}

func TestP02InvocationAcrossDatabases(t *testing.T) {
	for _, driver := range []string{"sqlite", "postgres"} {
		t.Run(driver, func(t *testing.T) {
			db := testDB(t, driver)
			repo := repository.New(db)
			service := New(repo, t.TempDir(), true).WithAdapters(invocationAdapters())
			id, err := service.Install("admin", invocationPackage(t), nil)
			if err != nil {
				t.Fatal(err)
			}
			releases, _ := repo.PluginReleases(id)
			release := releases[0]
			if err = service.Activate("user", id, Activation{ReleaseID: release.ID, Enabled: true, GrantedPermissions: []string{"media.read", "resource.create"}}); err != nil {
				t.Fatal(err)
			}
			if err = db.Create(&model.Resource{ID: "video", UserID: "user", Kind: "video", Status: model.ResourceStatusReady, MimeType: "video/mp4"}).Error; err != nil {
				t.Fatal(err)
			}
			input, _ := json.Marshal("video")
			read := contracts.Invocation{Operation: id + ".inspect-video", ReleaseID: release.ID, Input: map[string]json.RawMessage{"resourceId": input}}
			if result, err := service.Invoke("user", "", read, InvocationPolicy{PermissionMode: "read_only"}); err != nil || result.Kind != "inline" {
				t.Fatalf("inline read: %+v %v", result, err)
			}
			write := contracts.Invocation{Operation: id + ".snapshot-video", ReleaseID: release.ID, Input: map[string]json.RawMessage{"resourceId": input}}
			pending, err := service.Invoke("user", "same-request", write, InvocationPolicy{PermissionMode: "request_approval"})
			if err != nil || pending.Status != "waiting_approval" {
				t.Fatalf("pending: %+v %v", pending, err)
			}
			duplicate, err := service.Invoke("user", "same-request", write, InvocationPolicy{PermissionMode: "request_approval"})
			if err != nil || duplicate.RunID != pending.RunID {
				t.Fatalf("idempotency: %+v %v", duplicate, err)
			}
			if _, err = service.Decide("user", pending.RunID, pending.ApprovalID, "approve", pending.Revision); err != nil {
				t.Fatal(err)
			}
			var count int64
			if err = db.Model(&model.PluginRun{}).Count(&count).Error; err != nil || count != 1 {
				t.Fatalf("run count=%d err=%v", count, err)
			}
		})
	}
}
