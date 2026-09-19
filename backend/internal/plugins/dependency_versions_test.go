package plugins

import (
	"infinite-canvas/backend/internal/repository"
	"testing"
)

func TestApplicationRejectsIncompatibleDependencyVersions(t *testing.T) {
	db := testDB(t, "sqlite")
	repo := repository.New(db)
	s := New(repo, t.TempDir(), true)
	pure := func(id, version string, deps []any) []byte {
		return samplePackage(t, func(files map[string][]byte) {
			changeManifest(files, func(m map[string]any) {
				m["id"] = id
				m["version"] = version
				m["dependencies"] = deps
				c := m["contributes"].(map[string]any)
				c["skills"].([]any)[0].(map[string]any)["operations"] = []any{}
				delete(c, "operations")
				delete(c, "views")
				delete(c, "canvasBlueprints")
			})
		})
	}
	dep := func(id, version string) any { return map[string]any{"id": id, "version": version, "optional": false} }
	for _, data := range [][]byte{pure("base-helper", "1.0.0", []any{}), pure("base-helper", "1.1.0", []any{}), pure("middle-helper", "1.0.0", []any{dep("base-helper", "1.0.0")})} {
		if _, err := s.Install("admin", data, nil); err != nil {
			t.Fatal(err)
		}
	}
	_, err := s.Install("admin", pure("root-helper", "1.0.0", []any{dep("base-helper", "1.1.0"), dep("middle-helper", "1.0.0")}), nil)
	requireReason(t, err, "plugin_version_conflict")
	app, err := repo.PluginApplication("root-helper")
	if err != nil || app != nil {
		t.Fatal("invalid dependency tree published")
	}
}
