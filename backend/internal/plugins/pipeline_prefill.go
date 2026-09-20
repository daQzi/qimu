package plugins

import (
	"encoding/json"
	"infinite-canvas/backend/internal/plugins/contracts"
)

// Prefill is an initial draft, never a submitted value. It is evaluated once,
// when the input request is created, so resume cannot overwrite user edits.
func pipelineInputDraft(files contracts.PackageFiles, step contracts.PipelineStep, input, outputs map[string]json.RawMessage) (string, error) {
	if len(step.Inputs) == 0 {
		return "", nil
	}
	values, err := resolveBindings(step.Inputs, input, outputs)
	if err != nil {
		return "", err
	}
	for field, value := range values {
		if err = contracts.ValidateData(files, step.FormSchemaRef+"#/properties/"+field, value); err != nil {
			return "", issue(400, "operation_input_invalid", "上游预填内容不符合人工输入字段合同")
		}
	}
	raw, err := json.Marshal(values)
	if err != nil {
		return "", err
	}
	if len(raw) > 64<<10 {
		return "", issue(400, "operation_input_invalid", "人工输入预填超过 64 KiB")
	}
	return string(raw), nil
}
