package contracts

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func p07Package(t *testing.T) PackageFiles {
	t.Helper()
	files := PackageFiles{}
	err := filepath.WalkDir("testdata/canvas-helper-p07", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		raw, err := os.ReadFile(path)
		files[strings.TrimPrefix(filepath.ToSlash(path), "testdata/canvas-helper-p07/")] = raw
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}
func TestP07PresentationContracts(t *testing.T) {
	files := p07Package(t)
	if err := ValidatePackage(files, Policy{}); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*CanvasBlueprint){
		func(bp *CanvasBlueprint) { bp.Nodes[0].Actions = []string{"model.generate"} },
		func(bp *CanvasBlueprint) { bp.Nodes[0].Actions = []string{"input.submit"} },
		func(bp *CanvasBlueprint) { bp.Nodes[0].NodeType = "video" },
		func(bp *CanvasBlueprint) { bp.Nodes[0].Binding = "input" },
		func(bp *CanvasBlueprint) { bp.Connections[0].To = "missing" },
		func(bp *CanvasBlueprint) { bp.Connections = append(bp.Connections, bp.Connections[0]) },
		func(bp *CanvasBlueprint) {
			for len(bp.Nodes) < 20 {
				node := bp.Nodes[0]
				node.Key = string(rune('a' + len(bp.Nodes)))
				bp.Nodes = append(bp.Nodes, node)
			}
		},
	} {
		var bp CanvasBlueprint
		json.Unmarshal(files["blueprints/results.json"], &bp)
		mutate(&bp)
		changed := p07Package(t)
		changed["blueprints/results.json"], _ = json.Marshal(bp)
		if err := ValidatePackage(changed, Policy{}); err == nil {
			t.Fatal("invalid blueprint accepted")
		}
	}
	var v ResultView
	json.Unmarshal(files["views/mapping.json"], &v)
	v.Component = "table/v1"
	files["views/mapping.json"], _ = json.Marshal(v)
	if err := ValidatePackage(files, Policy{}); err == nil {
		t.Fatal("read-only input view accepted")
	}
}
