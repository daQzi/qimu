package contracts

import (
	"encoding/json"
	"slices"
	"strings"
)

// P05 admits a single explicit chain. Fan-out, conditions, nested pipelines and
// cross-package steps stay closed until their authorization contracts exist.
func LoadPipeline(files PackageFiles, id string) (Pipeline, error) {
	var manifest Manifest
	if err := json.Unmarshal(files["manifest.json"], &manifest); err != nil {
		return Pipeline{}, err
	}
	for _, ref := range manifest.Contributes.Pipelines {
		if ref.ID == id {
			var p Pipeline
			err := json.Unmarshal(files[ref.Ref], &p)
			return p, err
		}
	}
	return Pipeline{}, invalid("package_reference_invalid", "pipeline not registered")
}

func validateSequentialPackage(files PackageFiles, doc map[string]any) error {
	schemaBytes := 0
	for path, raw := range files {
		if strings.HasPrefix(path, "schemas/") {
			schemaBytes += len(raw)
		}
	}
	if schemaBytes > 24000 {
		return invalid("operation_unavailable", "pipeline schema description exceeds 24000 bytes")
	}
	raw, _ := json.Marshal(doc)
	var op Operation
	if err := json.Unmarshal(raw, &op); err != nil {
		return err
	}
	p, err := LoadPipeline(files, op.Execution.Pipeline)
	if err != nil {
		return err
	}
	if p.InputSchemaRef != op.InputSchemaRef || p.OutputSchemaRef != op.OutputSchemaRef {
		return invalid("contract_invalid", "pipeline entry schemas differ")
	}
	var m Manifest
	if err := json.Unmarshal(files["manifest.json"], &m); err != nil {
		return err
	}
	ops := map[string]Operation{}
	for _, ref := range m.Contributes.Operations {
		var child Operation
		if err := json.Unmarshal(files[ref.Ref], &child); err != nil {
			return err
		}
		ops[m.ID+"."+child.ID] = child
	}
	effects := map[string]bool{"draft_write": true}
	required := map[string]bool{}
	for i, step := range p.Steps {
		if len(step.When) > 0 || len(step.Foreach) > 0 || (i == 0 && len(step.DependsOn) != 0) || (i > 0 && !slices.Contains(step.DependsOn, p.Steps[i-1].Key)) {
			return invalid("operation_unavailable", "P05 requires an explicit sequential chain")
		}
		if step.Type == "wait_input" {
			if step.View != "" {
				return invalid("operation_unavailable", "custom input views require P07")
			}
			if _, ok := files[step.FormSchemaRef]; !ok || !strings.HasPrefix(step.FormSchemaRef, "schemas/") {
				return invalid("package_reference_invalid", "input schema")
			}
			continue
		}
		child, ok := ops[step.Operation]
		if !ok || child.Execution.Kind == "pipeline" {
			return invalid("operation_unavailable", "P05 steps must reference local non-pipeline operations")
		}
		for _, e := range child.Effects {
			effects[e] = true
		}
		for _, r := range child.RequiredPermissions {
			required[r] = true
		}
		if (child.Context.RequiresCanvas && !op.Context.RequiresCanvas) || (child.Context.RequiresProject && !op.Context.RequiresProject) {
			return invalid("scope_forbidden", "pipeline context weaker than step")
		}
	}
	for _, e := range op.Effects {
		delete(effects, e)
	}
	for _, r := range op.RequiredPermissions {
		delete(required, r)
	}
	if len(effects) > 0 || len(required) > 0 {
		return invalid("scope_forbidden", "pipeline must declare aggregate effects and permissions")
	}
	return nil
}
