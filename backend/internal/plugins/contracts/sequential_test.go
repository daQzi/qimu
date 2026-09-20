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
