package app

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"infinite-canvas/backend/internal/model"
	"infinite-canvas/backend/internal/plugins/authoring"
	"infinite-canvas/backend/internal/plugins/contracts"
	"infinite-canvas/backend/internal/repository"
	"infinite-canvas/backend/internal/skills"
)

// This is an author-created package using real host adapters, not a test adapter.
func TestPluginAuthoringDeliveryLifecycle(t *testing.T) {
	for _, driver := range []string{"sqlite", "postgres"} {
		t.Run(driver, func(t *testing.T) {
			s, db, _ := p03Fixture(t, driver)
			actor := &model.User{ID: "user", Role: model.UserRoleUser}
			admin := &model.User{ID: "admin", Role: model.UserRoleAdmin}
			permissions := []string{"media.read", "resource.create", "canvas.read", "canvas.write"}
			files, err := authoring.Template("resource", "delivery-check", "studio", contracts.Policy{})
			if err != nil {
				t.Fatal(err)
			}
			dir := filepath.Join(t.TempDir(), "delivery-check")
			if err = authoring.Init(dir, files); err != nil {
				t.Fatal(err)
			}
			files, err = authoring.ReadDirectory(dir)
			if err != nil {
				t.Fatal(err)
			}
			install := func(files contracts.PackageFiles, version string) string {
				t.Helper()
				raw, err := authoring.Pack(files, contracts.Policy{})
				if err != nil {
					t.Fatal(err)
				}
				if _, err = s.InstallManagedPluginForAdmin(admin, raw, "delivery.yingce-plugin"); err != nil {
					t.Fatal(err)
				}
				release, err := s.repo.PluginReleaseByVersion("delivery-check", version)
				if err != nil || release == nil {
					t.Fatal("release missing", err)
				}
				return release.ID
			}
			release := install(files, "1.0.0")
			request := contracts.Invocation{Operation: "delivery-check.inspect-video", ReleaseID: release, Input: p03Input(map[string]any{"resourceId": "video-one"})}
			if _, err = s.InvokePluginOperation("user", "", request); err == nil {
				t.Fatal("executed without activation")
			}
			activate := func(id string, enabled bool) {
				t.Helper()
				var revision int64
				state, err := s.repo.UserPluginState("user", "delivery-check")
				if err != nil {
					t.Fatal(err)
				}
				if state != nil {
					revision = state.Revision
				}
				if err = s.ActivateApplicationPlugin(actor, "delivery-check", ApplicationPluginActivation{ReleaseID: id, Enabled: enabled, GrantedPermissions: permissions, Revision: revision}); err != nil {
					t.Fatal(err)
				}
			}
			activate(release, true)
			bindings, err := s.repo.PluginSkillBindings(release)
			if err != nil || len(bindings) != 1 {
				t.Fatal("skill binding missing", err)
			}
			skillService := skills.New(s.repo, s.dataDir, nil)
			body, err := skillService.SkillPackageFile("user", bindings[0].SkillID, "SKILL.md")
			if err != nil || !strings.Contains(body.Content, "delivery-check.inspect-video") {
				t.Fatal("skill not available", err)
			}
			if _, err = s.DescribePluginOperation("user", request.Operation, release, PluginOperationContext{}); err != nil {
				t.Fatal(err)
			}
			read, err := s.InvokePluginOperation("user", "", request)
			if err != nil || read.Digest == "" {
				t.Fatal("real resource read failed", err)
			}
			snapshot := request
			snapshot.Operation = "delivery-check.snapshot-video"
			snapshot.Input = p03Input(map[string]any{"resourceId": "video-one", "expectedDigest": read.Digest})
			pending, err := s.InvokePluginOperation("user", "delivery-snapshot", snapshot)
			if err != nil || pending.Status != "waiting_approval" {
				t.Fatal("missing approval", err)
			}
			source := p03Approve(t, s, pending)
			if source.Status != "succeeded" || source.ResultRef == nil {
				t.Fatal("snapshot failed", source)
			}
			projection := p03Projection(t, s, source, "default")
			projection.Operation = "delivery-check.place-result"
			output, err := s.InvokePluginOperation("user", "delivery-projection", projection)
			if err != nil || p03NodeCount(t, s) != 0 {
				t.Fatal("unapproved canvas mutation", err)
			}
			p03Approve(t, s, output)
			if p03NodeCount(t, s) != 1 {
				t.Fatal("missing canvas node")
			}
			// Rebuilding the service must reuse the original run and canvas binding.
			s = &Service{repo: repository.New(db), dataDir: s.dataDir}
			replay, err := s.InvokePluginOperation("user", "delivery-projection", projection)
			if err != nil || replay.RunID != output.RunID || p03NodeCount(t, s) != 1 {
				t.Fatal("recovery duplicated work", err)
			}
			newer, err := authoring.WithVersion(files, "1.1.0")
			if err != nil {
				t.Fatal(err)
			}
			newRelease := install(newer, "1.1.0")
			state, err := s.repo.UserPluginState("user", "delivery-check")
			if err != nil || state.InstalledReleaseID != release {
				t.Fatal("install silently switched user version", err)
			}
			activate(newRelease, true)
			historical, err := s.PluginRun("user", source.ID)
			if err != nil || historical.ReleaseID != release || !bytes.Equal(historical.Result, source.Result) {
				t.Fatal("upgrade changed historical result", err)
			}
			activate(newRelease, false)
			request.ReleaseID = newRelease
			if _, err = s.InvokePluginOperation("user", "", request); err == nil {
				t.Fatal("disabled plugin executed")
			}
			historical, err = s.PluginRun("user", source.ID)
			if err != nil || !json.Valid(historical.Result) || p03NodeCount(t, s) != 1 {
				t.Fatal("disable erased history", err)
			}
			if _, err = s.PluginRun("other", source.ID); err == nil {
				t.Fatal("cross-account history readable")
			}
		})
	}
}
