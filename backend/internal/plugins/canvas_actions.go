package plugins

import (
	"encoding/json"
	"infinite-canvas/backend/internal/plugins/contracts"
)

func (s *Service) decorateCanvasActions(run *RunView, files contracts.PackageFiles, manifest contracts.Manifest) error {
	blueprints := []contracts.CanvasBlueprint{}
	for _, ref := range manifest.Contributes.CanvasBlueprints {
		var bp contracts.CanvasBlueprint
		if err := json.Unmarshal(files[ref.Ref], &bp); err != nil {
			return err
		}
		blueprints = append(blueprints, bp)
	}
	for _, ref := range manifest.Contributes.Operations {
		var op contracts.Operation
		if err := json.Unmarshal(files[ref.Ref], &op); err != nil {
			return err
		}
		if op.Execution.Adapter != "canvas.blueprint.instantiate" {
			continue
		}
		for _, bp := range blueprints {
			action := CanvasAction{Operation: manifest.ID + "." + op.ID, ReleaseID: run.ReleaseID, BlueprintID: bp.ID}
			if run.Status == "succeeded" {
				matches := true
				for _, node := range bp.Nodes {
					v, err := contracts.LoadView(files, node.View)
					if err != nil {
						return err
					}
					matches = matches && node.Binding == "result" && contracts.ValidateData(files, v.SchemaRef, run.Result) == nil
				}
				if matches {
					run.CanvasActions = append(run.CanvasActions, action)
				}
				continue
			}
			if run.Pipeline == nil || !contains([]string{"waiting_input", "running", "waiting_approval"}, run.Status) {
				continue
			}
			for _, input := range run.Pipeline.Inputs {
				if input.Status != "pending" {
					continue
				}
				for _, step := range run.Pipeline.Steps {
					if step.Key != input.StepKey {
						continue
					}
					matches, err := contracts.BlueprintMatchesInput(files, bp, step)
					if err != nil {
						return err
					}
					if matches {
						inputAction := action
						inputAction.InputRequestID = input.ID
						run.CanvasActions = append(run.CanvasActions, inputAction)
					}
				}
			}
		}
	}
	return nil
}
