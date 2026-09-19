package plugins

import (
	"encoding/json"
	"strings"

	"infinite-canvas/backend/internal/plugins/contracts"
)

type CanvasAction struct {
	Operation   string `json:"operation"`
	ReleaseID   string `json:"releaseId"`
	BlueprintID string `json:"blueprintId"`
}

// Presentation is always read from the immutable, hash-verified release.
// Disabling a plugin stops new calls, not access to its existing results.
func (s *Service) decorateRunView(view *RunView, viewID ...string) error {
	release, err := s.repo.PluginRelease(view.ReleaseID)
	if err != nil {
		return err
	}
	files, err := s.loadPackage(*release)
	if err != nil {
		return err
	}
	var manifest contracts.Manifest
	if err = json.Unmarshal(files["manifest.json"], &manifest); err != nil {
		return err
	}
	for _, ref := range manifest.Contributes.Operations {
		var op contracts.Operation
		if err = json.Unmarshal(files[ref.Ref], &op); err != nil {
			return err
		}
		if manifest.ID+"."+op.ID == view.Operation {
			view.ExecutionAdapter = op.Execution.Adapter
			selected := op.ResultView
			if len(viewID) > 0 && viewID[0] != "" {
				selected = viewID[0]
			}
			for _, vr := range manifest.Contributes.Views {
				if vr.ID != selected {
					continue
				}
				var definition contracts.ResultView
				if err = json.Unmarshal(files[vr.Ref], &definition); err != nil {
					return err
				}
				view.View = &definition
			}
			if selected != "" && view.View == nil {
				return issue(400, "operation_input_invalid", "结果视图不存在")
			}
			if view.View != nil && view.Status == "succeeded" {
				if err = contracts.ValidateData(files, view.View.SchemaRef, view.Result); err != nil {
					return issue(400, "operation_input_invalid", "结果不符合视图合同")
				}
			}
		}
		if view.Status == "succeeded" && op.Execution.Adapter == "canvas.blueprint.instantiate" {
			for _, bp := range manifest.Contributes.CanvasBlueprints {
				view.CanvasActions = append(view.CanvasActions, CanvasAction{Operation: manifest.ID + "." + op.ID, ReleaseID: release.ID, BlueprintID: bp.ID})
			}
		}
	}
	// Projection receipts are not new business results to recursively project.
	if strings.HasPrefix(view.ExecutionAdapter, "canvas.") {
		view.CanvasActions = nil
	}
	return nil
}
