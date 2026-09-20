package contracts

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestP11ModelContractBoundaries(t *testing.T) {
	root := "../../../../examples/plugins/video-localization"
	files := PackageFiles{}
	if err := filepath.WalkDir(root, func(name string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		raw, err := os.ReadFile(name)
		files[strings.TrimPrefix(filepath.ToSlash(name), root+"/")] = raw
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err := ValidatePackage(files, Policy{}); err != nil {
		t.Fatal(err)
	}
	for _, scenario := range []string{"old-host", "missing-media", "false-effect", "foreign-profile", "external-url", "audio-resource", "unknown-validator"} {
		t.Run(scenario, func(t *testing.T) {
			copyFiles := PackageFiles{}
			for k, v := range files {
				copyFiles[k] = v
			}
			var manifest, operation, pipeline map[string]any
			json.Unmarshal(files["manifest.json"], &manifest)
			json.Unmarshal(files["operations/analyze.json"], &operation)
			json.Unmarshal(files["pipelines/localize.json"], &pipeline)
			execution := operation["execution"].(map[string]any)
			switch scenario {
			case "old-host":
				manifest["requires"].(map[string]any)["hostApi"] = "^3.2.0"
			case "missing-media":
				operation["requiredPermissions"] = []string{"generation.run"}
			case "false-effect":
				operation["effects"] = []string{"read"}
			case "foreign-profile":
				execution["outputProfile"] = "arbitrary-code"
			case "external-url":
				execution["baseUrl"] = "https://example.invalid"
			case "audio-resource":
				execution["resources"].(map[string]any)["resourceId"] = "audio"
			case "unknown-validator":
				pipeline["steps"].([]any)[1].(map[string]any)["inputValidator"] = "eval"
			}
			copyFiles["manifest.json"], _ = json.Marshal(manifest)
			copyFiles["operations/analyze.json"], _ = json.Marshal(operation)
			copyFiles["pipelines/localize.json"], _ = json.Marshal(pipeline)
			if ValidatePackage(copyFiles, Policy{}) == nil {
				t.Fatal("unsafe contract accepted")
			}
		})
	}
}
