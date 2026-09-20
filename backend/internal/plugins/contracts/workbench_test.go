package contracts

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func workbenchSample(t *testing.T, name string) PackageFiles {
	t.Helper()
	root := filepath.Join("../../../../examples/plugins", name)
	files := PackageFiles{}
	if err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		name, err := filepath.Rel(root, path)
		files[filepath.ToSlash(name)] = raw
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return files
}

func TestWorkbenchSamplesAndInvalidContracts(t *testing.T) {
	for _, name := range []string{"resource-workbench", "brand-workbench", "brief-workbench"} {
		t.Run(name, func(t *testing.T) {
			if err := ValidatePackage(workbenchSample(t, name), Policy{}); err != nil {
				t.Fatal(err)
			}
		})
	}
	for _, tc := range []struct {
		name, file string
		change     func(map[string]any)
	}{
		{"old host", "manifest.json", func(v map[string]any) { v["requires"].(map[string]any)["hostApi"] = "^3.0.0" }},
		{"unknown recipe", "workbenches/compose.json", func(v map[string]any) { v["recipes"] = []string{"missing"} }},
		{"unknown skill", "workbenches/compose.json", func(v map[string]any) { v["skill"] = "missing" }},
		{"invalid default", "workbenches/compose.json", func(v map[string]any) { v["defaults"] = map[string]any{"tone": false} }},
		{"unknown field", "recipes/social.json", func(v map[string]any) { v["defaults"] = map[string]any{"maxCredits": 999} }},
		{"external operation", "workbenches/compose.json", func(v map[string]any) { v["operation"] = "other.launch" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			files := workbenchSample(t, "brand-workbench")
			var v map[string]any
			json.Unmarshal(files[tc.file], &v)
			tc.change(v)
			files[tc.file] = mustJSON(v)
			if err := ValidatePackage(files, Policy{}); err == nil {
				t.Fatal("accepted invalid workbench")
			}
		})
	}
}

func TestWorkbenchRecipeComposition(t *testing.T) {
	files := workbenchSample(t, "brand-workbench")
	_, boards, recipes, err := WorkbenchDefinitions(files)
	if err != nil {
		t.Fatal(err)
	}
	board := boards["compose"]
	conflict, err := ComposeWorkbench(board, recipes, []string{"social", "formal"}, map[string]any{})
	if err != nil || len(conflict.Conflicts) != 1 {
		t.Fatalf("missing suggestion conflict: %+v %v", conflict, err)
	}
	edits := map[string]any{"tone": "专业", "brand": "用户品牌", "audience": "读者"}
	left, err := ComposeWorkbench(board, recipes, []string{"social", "formal"}, edits)
	if err != nil || len(left.Conflicts) != 0 || left.Input["brand"] != "用户品牌" {
		t.Fatal(left, err)
	}
	right, _ := ComposeWorkbench(board, recipes, []string{"formal", "social"}, edits)
	if !reflect.DeepEqual(left, right) {
		t.Fatal("recipe order changed result")
	}
	conflict, _ = ComposeWorkbench(board, recipes, []string{"product"}, map[string]any{"channel": "社交媒体"})
	if len(conflict.Conflicts) != 1 {
		t.Fatal("requirement bypassed")
	}
	recipes["social"] = Recipe{ID: "social", Requirements: map[string]any{"channel": "社交媒体"}}
	conflict, _ = ComposeWorkbench(board, recipes, []string{"product", "social"}, map[string]any{"channel": "商品详情"})
	if len(conflict.Conflicts) == 0 {
		t.Fatal("contradictory requirements bypassed")
	}
	if _, err = ComposeWorkbench(board, recipes, []string{"social", "social"}, edits); err == nil {
		t.Fatal("duplicate accepted")
	}
	if edits["channel"] != nil || board.Defaults["tone"] != "友好" {
		t.Fatal("composition mutated caller")
	}
}
