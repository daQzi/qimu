package plugins

import (
	"encoding/json"
	"infinite-canvas/backend/internal/plugins/contracts"
	"testing"
)

func TestPipelineBindingsPreserveNumbersAndRejectMissing(t *testing.T) {
	input := map[string]json.RawMessage{"count": json.RawMessage("9007199254740991"), "data": json.RawMessage(`{"a/b":[null,{"~key":"kept"}]}`)}
	value, err := resolveBindings(map[string]contracts.Binding{"count": {From: "input#/count"}, "null": {From: "input#/data/a~1b/0"}, "escaped": {From: "input#/data/a~1b/1/~0key"}}, input, nil)
	if err != nil || string(value["count"]) != "9007199254740991" || string(value["null"]) != "null" || string(value["escaped"]) != `"kept"` {
		t.Fatalf("value=%s err=%v", encode(value), err)
	}
	for _, path := range []string{"input#/missing", "input#/data/a~1b/01", "input#/data/a~1b/2", "steps/absent#/value"} {
		if _, err = resolveBindings(map[string]contracts.Binding{"value": {From: path}}, input, nil); err == nil {
			t.Fatal("missing reference accepted", path)
		}
	}
	input["count"] = json.RawMessage("9007199254740993")
	if _, err = resolveBindings(map[string]contracts.Binding{"count": {From: "input#/count"}}, input, nil); err == nil {
		t.Fatal("unsafe integer must be rejected, not rounded")
	}
}
