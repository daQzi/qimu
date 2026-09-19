package plugins

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"infinite-canvas/backend/internal/kernel"
	"infinite-canvas/backend/internal/model"
	"infinite-canvas/backend/internal/repository"
	"infinite-canvas/backend/internal/skills"
)

func testDB(t *testing.T, driver string) *gorm.DB {
	t.Helper()
	var dialector gorm.Dialector
	if driver == "postgres" {
		dsn := os.Getenv("CANVAS_TEST_POSTGRES_DSN")
		if dsn == "" {
			t.Skip("isolated PostgreSQL DSN not configured")
		}
		base, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
		if err != nil {
			t.Fatal(err)
		}
		schema := "p01_" + strings.ReplaceAll(kernel.NewID(), "-", "")
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
	} else {
		dialector = sqlite.Open(filepath.Join(t.TempDir(), "test.db") + "?_busy_timeout=5000&_journal_mode=WAL")
	}
	db, err := gorm.Open(dialector, &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	sql, _ := db.DB()
	sql.SetMaxOpenConns(4)
	t.Cleanup(func() { sql.Close() })
	if err = db.AutoMigrate(&model.PluginApplication{}, &model.PluginRelease{}, &model.PluginSkillBinding{}, &model.PluginReleaseDependency{}, &model.PluginCatalogLock{}, &model.PluginNamespace{}, &model.UserPluginState{}, &model.User{}, &model.Skill{}, &model.SkillVersion{}, &model.SkillFile{}, &model.UserSkillState{}, &model.AdminAuditEvent{}, &model.UserIdentity{}); err != nil {
		t.Fatal(err)
	}
	return db
}
func samplePackage(t *testing.T, edit func(map[string][]byte)) []byte {
	t.Helper()
	files := map[string][]byte{}
	root := "contracts/testdata/resource-helper"
	if err := filepath.WalkDir(root, func(p string, d fs.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if d.IsDir() {
			return nil
		}
		b, e := os.ReadFile(p)
		files[strings.TrimPrefix(filepath.ToSlash(p), root+"/")] = b
		return e
	}); err != nil {
		t.Fatal(err)
	}
	if edit != nil {
		edit(files)
	}
	var b bytes.Buffer
	w := zip.NewWriter(&b)
	for p, data := range files {
		entry, e := w.Create(p)
		if e != nil {
			t.Fatal(e)
		}
		entry.Write(data)
	}
	if e := w.Close(); e != nil {
		t.Fatal(e)
	}
	return b.Bytes()
}
func changeManifest(files map[string][]byte, fn func(map[string]any)) {
	var m map[string]any
	json.Unmarshal(files["manifest.json"], &m)
	fn(m)
	files["manifest.json"], _ = json.Marshal(m)
}
func requireReason(t *testing.T, err error, reason string) {
	t.Helper()
	if err == nil || !strings.Contains(fmt.Sprintf("%+v", err), reason) {
		if e, ok := err.(*kernel.AppError); !ok || string(e.Reason) != reason {
			t.Fatalf("want %s, got %v", reason, err)
		}
	}
}

func TestApplicationLifecycle(t *testing.T) {
	for _, driver := range []string{"sqlite", "postgres"} {
		t.Run(driver, func(t *testing.T) {
			db := testDB(t, driver)
			repo := repository.New(db)
			dir := t.TempDir()
			s := New(repo, dir, true)
			data := samplePackage(t, nil)
			id, err := s.Install("admin", data, nil)
			if err != nil {
				t.Fatal(err)
			}
			releases, _ := repo.PluginReleases(id)
			if len(releases) != 1 {
				t.Fatal(releases)
			}
			v1 := releases[0]
			if _, err = s.Install("admin", data, nil); err != nil {
				t.Fatal(err)
			}
			releases, _ = repo.PluginReleases(id)
			if len(releases) != 1 {
				t.Fatal("duplicate release")
			}
			var count int64
			db.Model(&model.SkillVersion{}).Count(&count)
			if count != 1 {
				t.Fatal("duplicate skill version")
			}
			_, err = s.Install("admin", samplePackage(t, func(f map[string][]byte) { f["skills/check-source/SKILL.md"] = []byte("Changed") }), nil)
			requireReason(t, err, "plugin_version_conflict")
			_, err = s.Install("admin", samplePackage(t, func(f map[string][]byte) {
				changeManifest(f, func(m map[string]any) { m["publisher"].(map[string]any)["id"] = "imposter" })
			}), nil)
			requireReason(t, err, "scope_forbidden")
			if err = s.Activate("alice", id, Activation{ReleaseID: v1.ID, Enabled: true, GrantedPermissions: []string{"generation.run"}}); err == nil {
				t.Fatal("overgrant")
			}
			if err = s.Activate("alice", id, Activation{ReleaseID: v1.ID, Enabled: true, GrantedPermissions: []string{"media.read"}}); err != nil {
				t.Fatal(err)
			}
			requireReason(t, s.Activate("alice", id, Activation{ReleaseID: v1.ID, Enabled: true}), "run_revision_conflict")
			skillService := skills.New(repo, dir, nil)
			added, err := skillService.AddedSkills("alice")
			if err != nil || len(added) != 1 {
				t.Fatalf("skills %v %v", added, err)
			}
			skillID := added[0].SkillID
			versionID := added[0].VersionID
			if _, err = skillService.SkillPackageFile("bob", skillID, "SKILL.md"); err == nil {
				t.Fatal("cross-user skill access")
			}
			file, err := skillService.SkillPackageFile("alice", skillID, "SKILL.md")
			if err != nil || !strings.Contains(file.Content, "resource-helper") {
				t.Fatalf("skill body %v", err)
			}
			if err = repo.DeleteSkill(skillID); err == nil {
				t.Fatal("release skill deleted")
			}
			if _, err = skillService.SetSkillAdded("alice", skillID, false); err == nil {
				t.Fatal("plugin ownership bypass")
			}
			catalog, err := s.Catalog("alice", false)
			if err != nil || !catalog[0].State.EffectiveEnabled || catalog[0].Releases[0].Operations[0].Available {
				t.Fatalf("catalog %+v %v", catalog, err)
			}
			_, err = s.Install("admin", samplePackage(t, func(f map[string][]byte) {
				changeManifest(f, func(m map[string]any) { m["version"] = "1.1.0" })
				f["skills/check-source/SKILL.md"] = []byte("New version instructions")
			}), nil)
			if err != nil {
				t.Fatal(err)
			}
			added, err = skillService.AddedSkills("alice")
			if err != nil || len(added) != 1 || added[0].VersionID != versionID {
				t.Fatal("publishing upgraded an active skill")
			}
			releases, _ = repo.PluginReleases(id)
			v2 := releases[0]
			if v2.ID == v1.ID {
				t.Fatal("missing new release")
			}
			if err = s.Activate("alice", id, Activation{ReleaseID: v2.ID, Enabled: true, GrantedPermissions: []string{"media.read"}, Revision: 1}); err != nil {
				t.Fatal(err)
			}
			added, err = skillService.AddedSkills("alice")
			if err != nil || len(added) != 1 || added[0].VersionID == versionID {
				t.Fatalf("upgrade %+v %v", added, err)
			}
			app, _ := repo.PluginApplication(id)
			if err = s.Manage("admin", id, ManagementChange{Action: "availability", Available: false, Revision: app.Revision}); err != nil {
				t.Fatal(err)
			}
			if _, err = skillService.SkillPackageFile("alice", added[0].SkillID, "SKILL.md"); err == nil {
				t.Fatal("platform disable bypass")
			}
			app, _ = repo.PluginApplication(id)
			s.Manage("admin", id, ManagementChange{Action: "availability", Available: true, Revision: app.Revision})
			app, _ = repo.PluginApplication(id)
			if err = s.Manage("admin", id, ManagementChange{Action: "revoke", ReleaseID: v2.ID, Revision: app.Revision}); err != nil {
				t.Fatal(err)
			}
			if _, err = skillService.SkillPackageFile("alice", added[0].SkillID, "SKILL.md"); err == nil {
				t.Fatal("revocation bypass")
			}
			app, _ = repo.PluginApplication(id)
			if err = s.Manage("admin", id, ManagementChange{Action: "uninstall", Revision: app.Revision}); err != nil {
				t.Fatal(err)
			}
			releases, _ = repo.PluginReleases(id)
			db.Model(&model.SkillVersion{}).Count(&count)
			if len(releases) != 2 || count != 2 {
				t.Fatal("uninstall destroyed history")
			}
			if err = s.verifyPackage(v1); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestApplicationDependencyRevocationAndGrants(t *testing.T) {
	db := testDB(t, "sqlite")
	r := repository.New(db)
	s := New(r, t.TempDir(), true)
	id, e := s.Install("admin", samplePackage(t, nil), nil)
	if e != nil {
		t.Fatal(e)
	}
	releases, _ := r.PluginReleases(id)
	base := releases[0]
	dependent := samplePackage(t, func(f map[string][]byte) {
		changeManifest(f, func(m map[string]any) {
			m["id"] = "dependent-helper"
			m["dependencies"] = []any{map[string]any{"id": id, "version": "1.0.0", "optional": false}}
			c := m["contributes"].(map[string]any)
			delete(c, "operations")
			delete(c, "views")
			delete(c, "canvasBlueprints")
		})
	})
	depID, e := s.Install("admin", dependent, nil)
	if e != nil {
		t.Fatal(e)
	}
	releases, _ = r.PluginReleases(depID)
	dep := releases[0]
	requireReason(t, s.Activate("alice", depID, Activation{ReleaseID: dep.ID, Enabled: true}), "plugin_dependency_missing")
	if e = s.Activate("alice", id, Activation{ReleaseID: base.ID, Enabled: true, GrantedPermissions: []string{"media.read"}}); e != nil {
		t.Fatal(e)
	}
	if e = s.Activate("alice", depID, Activation{ReleaseID: dep.ID, Enabled: true}); e != nil {
		t.Fatal(e)
	}
	if e = s.Activate("alice", id, Activation{ReleaseID: base.ID, Enabled: true, GrantedPermissions: []string{}, Revision: 1}); e != nil {
		t.Fatal(e)
	}
	state, _ := r.UserPluginState("alice", depID)
	if state.Enabled {
		t.Fatal("changed dependency grants retained dependent activation")
	}
	app, _ := r.PluginApplication(id)
	s.Manage("admin", id, ManagementChange{Action: "revoke", ReleaseID: base.ID, Revision: app.Revision})
	requireReason(t, s.Activate("alice", depID, Activation{ReleaseID: dep.ID, Enabled: true, Revision: state.Revision}), "plugin_dependency_missing")
}

func TestApplicationOrphansAndDisabledAdmission(t *testing.T) {
	db := testDB(t, "sqlite")
	repo := repository.New(db)
	dir := t.TempDir()
	s := New(repo, dir, false)
	_, err := s.Install("admin", samplePackage(t, nil), nil)
	requireReason(t, err, "plugin_disabled")
	s = New(repo, dir, true)
	id, err := s.Install("admin", samplePackage(t, nil), nil)
	if err != nil {
		t.Fatal(err)
	}
	release, _ := repo.PluginReleases(id)
	root := filepath.Join(dir, "application-plugin-packages")
	fake := strings.Repeat("a", 64) + ".yingce-plugin"
	os.WriteFile(filepath.Join(root, fake), []byte("orphan"), 0600)
	old := time.Now().Add(-48 * time.Hour)
	os.Chtimes(filepath.Join(root, fake), old, old)
	os.Chtimes(filepath.Join(root, release[0].PackageKey), old, old)
	items, err := s.PruneOrphans(true)
	if err != nil || len(items) != 1 {
		t.Fatalf("orphan list %v %v", items, err)
	}
	if _, err = os.Stat(filepath.Join(root, fake)); err != nil {
		t.Fatal("preview deleted file")
	}
	items, err = s.PruneOrphans(false)
	if err != nil || len(items) != 1 {
		t.Fatal(err)
	}
	if err = s.verifyPackage(release[0]); err != nil {
		t.Fatal("referenced release removed")
	}
}

func TestApplicationConcurrentPublicationAndNamespace(t *testing.T) {
	for _, driver := range []string{"sqlite", "postgres"} {
		t.Run(driver, func(t *testing.T) {
			db := testDB(t, driver)
			repo := repository.New(db)
			s := New(repo, t.TempDir(), true)
			data := samplePackage(t, nil)
			var wg sync.WaitGroup
			failures := make(chan error, 2)
			for i := 0; i < 2; i++ {
				wg.Add(1)
				go func() { defer wg.Done(); _, err := s.Install("admin", data, nil); failures <- err }()
			}
			wg.Wait()
			close(failures)
			for err := range failures {
				if err != nil {
					t.Fatal(err)
				}
			}
			var count int64
			db.Model(&model.PluginRelease{}).Count(&count)
			if count != 1 {
				t.Fatal("concurrent publication duplicated release")
			}
			if err := repo.ClaimPluginNamespace("resource-helper", "legacy"); err != repository.ErrPluginNamespaceConflict {
				t.Fatalf("namespace conflict: %v", err)
			}
		})
	}
}

func TestApplicationFailedPublicationIsNotVisible(t *testing.T) {
	db := testDB(t, "sqlite")
	repo := repository.New(db)
	dir := t.TempDir()
	s := New(repo, dir, true)
	// Valid plugin envelope, but beyond the existing skill archive's 8 MiB/file
	// limit; publication must roll back even after creating the first skill.
	data := samplePackage(t, func(files map[string][]byte) {
		changeManifest(files, func(m map[string]any) {
			c := m["contributes"].(map[string]any)
			list := c["skills"].([]any)
			c["skills"] = append(list, map[string]any{"id": "too-large", "name": "too-large", "description": "test", "entry": "skills/too-large/SKILL.md", "activation": "explicit", "operations": []any{}})
		})
		files["skills/too-large/SKILL.md"] = bytes.Repeat([]byte("x"), (8<<20)+1)
	})
	if _, err := s.Install("admin", data, nil); err == nil {
		t.Fatal("oversize skill accepted")
	}
	for _, table := range []any{&model.PluginApplication{}, &model.PluginRelease{}, &model.Skill{}, &model.SkillVersion{}, &model.PluginSkillBinding{}} {
		var n int64
		db.Model(table).Count(&n)
		if n != 0 {
			t.Fatalf("partially published %T", table)
		}
	}
	var files []string
	filepath.WalkDir(filepath.Join(dir, "skill-packages"), func(name string, entry fs.DirEntry, err error) error {
		if err == nil && !entry.IsDir() {
			files = append(files, name)
		}
		return nil
	})
	if len(files) > 0 {
		t.Fatal("rolled-back skill archives remain", files)
	}
}
