package plugins

import (
	"encoding/json"
	"infinite-canvas/backend/internal/plugins/contracts"
	"testing"
)

func TestPipelineInputDraft(t *testing.T) {
	files := contracts.PackageFiles{"schemas/form.json": []byte(`{"type":"object","properties":{"name":{"type":"string"},"confirmed":{"type":"boolean"}},"required":["name","confirmed"],"additionalProperties":false}`)}
	step := contracts.PipelineStep{FormSchemaRef: "schemas/form.json", Inputs: map[string]contracts.Binding{"name": {From: "steps/analyze#/name"}}}
	outputs := map[string]json.RawMessage{"analyze": json.RawMessage(`{"name":"人物一"}`)}
	draft, err := pipelineInputDraft(files, step, nil, outputs)
	if err != nil || draft != `{"name":"人物一"}` {
		t.Fatalf("partial draft: %s, %v", draft, err)
	}
	if err = contracts.ValidateData(files, step.FormSchemaRef, []byte(draft)); err == nil {
		t.Fatal("partial draft must not satisfy submission schema")
	}
	for _, raw := range []string{`{"name":false}`, `{}`, `null`} {
		outputs["analyze"] = json.RawMessage(raw)
		if _, err = pipelineInputDraft(files, step, nil, outputs); err == nil {
			t.Fatalf("invalid result accepted: %s", raw)
		}
	}
	delete(outputs, "analyze")
	if _, err = pipelineInputDraft(files, step, nil, outputs); err == nil {
		t.Fatal("missing source accepted")
	}
	step.Inputs = nil
	if draft, err = pipelineInputDraft(files, step, nil, nil); err != nil || draft != "" {
		t.Fatalf("legacy empty draft: %s, %v", draft, err)
	}
}
