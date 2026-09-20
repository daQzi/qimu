package contracts

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestP05SequentialContract(t *testing.T) {
	files := PackageFiles{}
	err := filepath.WalkDir("testdata/pipeline-helper-p05", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		raw, err := os.ReadFile(path)
		files[strings.TrimPrefix(filepath.ToSlash(path), "testdata/pipeline-helper-p05/")] = raw
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if err = ValidatePackage(files, Policy{}); err != nil {
		t.Fatal(err)
	}
	original := append([]byte{}, files["pipelines/process.json"]...)
	for _, scenario := range []string{"valid", "old-host", "unknown-field", "wrong-type", "missing-dependency"} {
		t.Run("prefill-"+scenario, func(t *testing.T) {
			copyFiles := PackageFiles{}
			for k, v := range files {
				copyFiles[k] = v
			}
			var manifest map[string]any
			json.Unmarshal(copyFiles["manifest.json"], &manifest)
			host := "^3.3.0"
			if scenario == "old-host" {
				host = "^3.2.0"
			}
			manifest["requires"].(map[string]any)["hostApi"] = host
			copyFiles["manifest.json"], _ = json.Marshal(manifest)
			var p Pipeline
			json.Unmarshal(original, &p)
			p.Steps[1].Inputs = map[string]Binding{"prompt": {From: "steps/inspect#/name"}}
			switch scenario {
			case "unknown-field":
				p.Steps[1].Inputs = map[string]Binding{"unknown": {Literal: json.RawMessage(`"text"`)}}
			case "wrong-type":
				p.Steps[1].Inputs = map[string]Binding{"prompt": {Literal: json.RawMessage(`false`)}}
			case "missing-dependency":
				p.Steps[1].DependsOn = []string{}
			}
			copyFiles["pipelines/process.json"], _ = json.Marshal(p)
			err := ValidatePackage(copyFiles, Policy{})
			if (err == nil) != (scenario == "valid") {
				t.Fatalf("unexpected admission: %v", err)
			}
		})
	}
	for _, mutate := range []func(*Pipeline){
		func(p *Pipeline) { p.Steps[2].DependsOn = []string{} },
		func(p *Pipeline) { p.Steps[2].Operation = "pipeline-helper.process" },
		func(p *Pipeline) { p.Steps[2].Operation = "foreign.echo" },
		func(p *Pipeline) { p.Steps[2].Inputs["prompt"] = Binding{From: "steps/inspect#/name"} },
		func(p *Pipeline) { p.Steps[1].FormSchemaRef = "schemas/missing.json" },
	} {
		var p Pipeline
		if err = json.Unmarshal(original, &p); err != nil {
			t.Fatal(err)
		}
		mutate(&p)
		files["pipelines/process.json"], _ = json.Marshal(p)
		if err = ValidatePackage(files, Policy{}); err == nil {
			t.Fatal("unsupported pipeline accepted")
		}
	}
	files["pipelines/process.json"] = original
	var op Operation
	json.Unmarshal(files["operations/process.json"], &op)
	op.Effects = []string{"read"}
	files["operations/process.json"], _ = json.Marshal(op)
	if err = ValidatePackage(files, Policy{}); err == nil {
		t.Fatal("weaker effects accepted")
	}
}
