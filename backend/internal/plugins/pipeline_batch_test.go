package plugins

import (
	"encoding/json"
	"infinite-canvas/backend/internal/plugins/contracts"
	"testing"
)

func TestP06ExpansionAndConditions(t *testing.T) {
	step := contracts.PipelineStep{Key: "batch", Foreach: json.RawMessage(`{"from":"input#/items","itemKey":"/id","maxConcurrency":2}`), Inputs: map[string]contracts.Binding{"prompt": {From: "item#/prompt"}}, When: json.RawMessage(`{"exists":"item#/prompt"}`)}
	input := map[string]json.RawMessage{"items": json.RawMessage(`[{"id":"b","prompt":"B"},{"id":"a"}]`)}
	row, limit, err := expandBatchStep(step, input, nil)
	if err != nil || limit != 2 || len(row.Items) != 2 || row.Items[0].Key != "b" || row.Items[1].Status != "skipped" {
		t.Fatalf("%+v %v", row, err)
	}
	for _, value := range []string{`null`, `{}`, `[{"id":1,"prompt":"x"}]`, `[{"id":"a","prompt":"x"},{"id":"a","prompt":"y"}]`, `[{"id":"","prompt":"x"}]`} {
		input["items"] = json.RawMessage(value)
		if _, _, err = expandBatchStep(step, input, nil); err == nil {
			t.Fatal("invalid items admitted", value)
		}
	}
	items := make([]map[string]string, 257)
	for i := range items {
		items[i] = map[string]string{"id": "x"}
	}
	input["items"], _ = json.Marshal(items)
	if _, _, err = expandBatchStep(step, input, nil); err == nil {
		t.Fatal("expansion cap ignored")
	}
	input["items"] = json.RawMessage(`[]`)
	row, _, err = expandBatchStep(step, input, nil)
	if err != nil || len(row.Items) != 0 {
		t.Fatal("empty batch", err)
	}
	matched, err := conditionMatches(json.RawMessage(`{"equals":[{"literal":{"x":1}},{"literal":{"x":1.0}}]}`), nil, nil)
	if err != nil || !matched {
		t.Fatal("canonical comparison", err)
	}
}
func TestP06DependencyClosure(t *testing.T) {
	p := contracts.Pipeline{Steps: []contracts.PipelineStep{{Key: "last", DependsOn: []string{"middle"}}, {Key: "unrelated"}, {Key: "first"}, {Key: "middle", DependsOn: []string{"first"}}}}
	closure, err := affectedSteps(p, []string{"first"})
	if err != nil || len(closure) != 3 {
		t.Fatal(closure, err)
	}
	if _, err = affectedSteps(p, []string{"missing"}); err == nil {
		t.Fatal("unknown step")
	}
}
