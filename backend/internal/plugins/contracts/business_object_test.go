package contracts

import (
	"encoding/json"
	"testing"
)

func TestBusinessObjectPackageContracts(t *testing.T) {
	for _, name := range []string{"brand-points", "brand-script"} {
		if err := ValidatePackage(workbenchSample(t, name), Policy{}); err != nil {
			t.Fatal(name, err)
		}
	}
	for _, tc := range []struct {
		file   string
		change func(map[string]any)
	}{
		{"manifest.json", func(v map[string]any) { v["requires"].(map[string]any)["hostApi"] = "^3.1.0" }},
		{"manifest.json", func(v map[string]any) { v["permissions"] = []string{} }},
		{"operations/read.json", func(v map[string]any) { v["requiredPermissions"] = []string{} }},
		{"operations/read.json", func(v map[string]any) { v["effects"] = []string{"draft_write"} }},
		{"operations/read.json", func(v map[string]any) { v["execution"].(map[string]any)["adapter"] = "constructor" }},
		{"workbenches/compose.json", func(v map[string]any) { v["objectInputs"] = map[string]string{"reference": "product/v1"} }},
		{"workbenches/compose.json", func(v map[string]any) { v["objectInputs"] = map[string]string{"missing": "brand/v1"} }},
		{"schemas/read-input.json", func(v map[string]any) { v["required"] = []string{} }},
	} {
		files := workbenchSample(t, "brand-script")
		var v map[string]any
		json.Unmarshal(files[tc.file], &v)
		tc.change(v)
		files[tc.file] = mustJSON(v)
		if err := ValidatePackage(files, Policy{}); err == nil {
			t.Fatal("accepted invalid", tc.file, string(files[tc.file]))
		}
	}
}
