package contracts

import "encoding/json"

// Views and blueprints describe host components; they never grant execution authority.
func LoadView(files PackageFiles, id string) (ResultView, error) {
	var manifest Manifest
	if err := json.Unmarshal(files["manifest.json"], &manifest); err != nil {
		return ResultView{}, err
	}
	for _, ref := range manifest.Contributes.Views {
		if ref.ID == id {
			var view ResultView
			err := json.Unmarshal(files[ref.Ref], &view)
			return view, err
		}
	}
	return ResultView{}, invalid("package_reference_invalid", "view not registered")
}

func ValidateInputView(files PackageFiles, id, schema string) error {
	v, err := LoadView(files, id)
	if err != nil {
		return err
	}
	if v.SchemaRef != schema || (v.Component != "key-value/v1" && v.Component != "mapping-editor/v1") {
		return invalid("contract_invalid", "input view requires matching schema and an editable host component")
	}
	return nil
}

func BlueprintMatchesInput(files PackageFiles, bp CanvasBlueprint, step PipelineStep) (bool, error) {
	if step.Type != "wait_input" {
		return false, nil
	}
	for _, node := range bp.Nodes {
		v, err := LoadView(files, node.View)
		if err != nil {
			return false, err
		}
		if node.Binding != "input" || step.FormSchemaRef != v.SchemaRef || (step.View != node.View && !(step.View == "" && v.Component == "key-value/v1")) {
			return false, nil
		}
	}
	return len(bp.Nodes) > 0, nil
}

func ValidateBlueprint(bp CanvasBlueprint, files PackageFiles) error {
	if len(bp.Nodes) == 0 {
		return invalid("contract_invalid", "empty blueprint")
	}
	if len(bp.Nodes)+len(bp.Connections) > 20 {
		return invalid("contract_invalid", "blueprint exceeds 20 canvas mutations")
	}
	keys := map[string]bool{}
	for _, node := range bp.Nodes {
		if node.NodeType != "plugin-result" && node.NodeType != "plugin-input" {
			return invalid("contract_invalid", "unsupported blueprint node")
		}
		if node.Binding != bp.Nodes[0].Binding {
			return invalid("contract_invalid", "one blueprint binds one input request or one successful result")
		}
		if keys[node.Key] {
			return invalid("contract_invalid", "duplicate blueprint key")
		}
		keys[node.Key] = true
		v, err := LoadView(files, node.View)
		if err != nil {
			return err
		}
		if (node.NodeType == "plugin-input") != (node.Binding == "input") {
			return invalid("contract_invalid", "node and binding kinds differ")
		}
		if node.Binding == "input" && v.Component != "key-value/v1" && v.Component != "mapping-editor/v1" {
			return invalid("contract_invalid", "input component is not editable")
		}
		for _, action := range node.Actions {
			if action != "editor.focus" && !(action == "input.submit" && node.Binding == "input") && !(action == "result.continue" && node.Binding == "result") {
				return invalid("contract_invalid", "action is not supported by this node")
			}
		}
	}
	edges := map[string]bool{}
	for _, edge := range bp.Connections {
		key := edge.From + ":" + edge.To
		if !keys[edge.From] || !keys[edge.To] || edge.From == edge.To || edges[key] {
			return invalid("contract_invalid", "invalid blueprint connection")
		}
		edges[key] = true
	}
	return nil
}
