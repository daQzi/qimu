package authoring

import (
	"encoding/json"
	"infinite-canvas/backend/internal/plugins/contracts"
	"strings"
	"testing"
)

func TestInspectionValidatesAndDescribesWithoutRunning(t *testing.T) {
	files, err := Template("model-review", "custom-review", "author", contracts.Policy{})
	if err != nil {
		t.Fatal(err)
	}
	before := contracts.PackageDigest(files)
	report, err := Inspect(files, contracts.Policy{})
	if err != nil {
		t.Fatal(err)
	}
	if report.RuntimeVerified || report.Contributions["workbenches"] != 1 || len(report.Operations) != 2 || report.PackageDigest != before {
		t.Fatal(report)
	}
	if report.Operations[0].Execution.Instruction != "" {
		t.Fatal("instructions leaked into diagnostics")
	}
	raw, _ := json.Marshal(report)
	if strings.Contains(string(raw), "apiKey") {
		t.Fatal("unexpected credential")
	}
	if contracts.PackageDigest(files) != before {
		t.Fatal("inspection modified source")
	}
	operation := files["operations/generate.json"]
	files["operations/generate.json"] = []byte(`{"id":"generate","execution":{"kind":"unsupported"}}`)
	if _, err := Inspect(files, contracts.Policy{}); err == nil || !strings.Contains(err.Error(), "operations/generate.json") {
		t.Fatalf("missing invalid contribution filename: %v", err)
	}
	files["operations/generate.json"] = operation
	files["schemas/draft.json"] = []byte(`{"type":"unsupported-type"}`)
	if _, err := Inspect(files, contracts.Policy{}); err == nil {
		t.Fatal("inspection bypassed validator")
	}
}
