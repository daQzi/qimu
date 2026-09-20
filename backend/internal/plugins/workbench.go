package plugins

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"infinite-canvas/backend/internal/model"
	"infinite-canvas/backend/internal/plugins/contracts"
	"infinite-canvas/backend/internal/repository"
)

type WorkbenchView struct {
	ID         string                      `json:"id"`
	ReleaseID  string                      `json:"releaseId"`
	Version    string                      `json:"version"`
	Definition contracts.Workbench         `json:"definition"`
	Recipes    map[string]contracts.Recipe `json:"recipes"`
	Schema     json.RawMessage             `json:"schema"`
	Schemas    map[string]json.RawMessage  `json:"schemas"`
	SkillID    string                      `json:"skillId,omitempty"`
}
type WorkbenchComposeRequest struct {
	ID        string                      `json:"id"`
	ReleaseID string                      `json:"releaseId"`
	RecipeIDs []string                    `json:"recipeIds"`
	Input     map[string]any              `json:"input"`
	Context   contracts.InvocationContext `json:"context"`
}
type WorkbenchPreview struct {
	contracts.WorkbenchComposition
	Selection         contracts.WorkbenchSelection `json:"selection"`
	Valid             bool                         `json:"valid"`
	ValidationMessage string                       `json:"validationMessage,omitempty"`
}

func (s *Service) workbench(repo *repository.Repository, user, address, releaseID string, ctx contracts.InvocationContext) (WorkbenchView, contracts.PackageFiles, error) {
	view := WorkbenchView{}
	if err := s.admission(); err != nil {
		return view, nil, err
	}
	if err := contracts.Validate("address", []byte(encode(address))); err != nil {
		return view, nil, err
	}
	parts := strings.Split(address, ".")
	state, err := repo.UserPluginState(user, parts[0])
	if err != nil {
		return view, nil, err
	}
	if state == nil || !state.Enabled {
		return view, nil, issue(403, "plugin_disabled", "请先启用工作台所属插件")
	}
	if releaseID == "" {
		releaseID = state.InstalledReleaseID
	}
	if releaseID != state.InstalledReleaseID {
		return view, nil, issue(409, "plugin_version_conflict", "工作台版本已变化，请重新打开")
	}
	release, err := repo.PluginRelease(releaseID)
	if err != nil {
		return view, nil, err
	}
	if release == nil || release.PluginID != parts[0] {
		return view, nil, issue(403, "scope_forbidden", "工作台与发布不匹配")
	}
	if err = checkRelease(repo, user, release); err != nil {
		return view, nil, err
	}
	files, err := s.loadPackage(*release)
	if err != nil {
		return view, nil, err
	}
	_, boards, recipes, err := contracts.WorkbenchDefinitions(files)
	if err != nil {
		return view, nil, err
	}
	board, ok := boards[parts[1]]
	if !ok {
		return view, nil, issue(404, "operation_unavailable", "工作台不存在")
	}
	if !contains(board.HostSurfaces, ctx.HostSurface) {
		return view, nil, issue(400, "operation_input_invalid", "此工作台不支持当前入口")
	}
	if ctx.CanvasID != "" {
		if _, err = repo.CanvasProjectForUser(user, ctx.CanvasID); err != nil {
			return view, nil, issue(403, "scope_forbidden", "画布不存在或不属于当前账号")
		}
	}
	if ctx.ProjectID != "" || ctx.ThreadID != "" || ctx.WorkbenchID != "" {
		return view, nil, issue(400, "operation_input_invalid", "工作台入口仅接受当前画布上下文")
	}
	if (ctx.HostSurface == "canvas") != (ctx.CanvasID != "") {
		return view, nil, issue(400, "operation_input_invalid", "画布入口需要真实画布")
	}
	if board.Operation != "" {
		if _, err = s.resolve(repo, user, board.Operation, releaseID, ctx, InvocationPolicy{PermissionMode: "request_approval"}); err != nil {
			return view, nil, err
		}
	}
	view = WorkbenchView{ID: address, ReleaseID: releaseID, Version: release.Version, Definition: board, Recipes: map[string]contracts.Recipe{}, Schema: files[board.ContextSchemaRef], Schemas: map[string]json.RawMessage{}}
	for name, raw := range files {
		if strings.HasPrefix(name, "schemas/") {
			view.Schemas[name] = raw
		}
	}
	for _, id := range board.Recipes {
		view.Recipes[id] = recipes[id]
	}
	if board.Skill != "" {
		bindings, e := repo.PluginSkillBindings(releaseID)
		if e != nil {
			return view, nil, e
		}
		for _, binding := range bindings {
			if binding.LocalSkillID == board.Skill {
				view.SkillID = binding.SkillID
			}
		}
		if view.SkillID == "" {
			return view, nil, issue(409, "operation_unavailable", "技能绑定不可用")
		}
	}
	return view, files, nil
}

func (s *Service) Workbench(user, id, release string, ctx contracts.InvocationContext) (WorkbenchView, error) {
	view, _, err := s.workbench(s.repo, user, id, release, ctx)
	return view, err
}

// Catalog only exposes configuration from the user's effective pinned releases.
func (s *Service) Workbenches(user string) ([]WorkbenchView, error) {
	if err := s.admission(); err != nil {
		return nil, err
	}
	apps, err := s.Catalog(user, false)
	if err != nil {
		return nil, err
	}
	result := []WorkbenchView{}
	for _, app := range apps {
		if !app.State.EffectiveEnabled {
			continue
		}
		for _, release := range app.Releases {
			if release.ID != app.State.InstalledReleaseID {
				continue
			}
			if len(release.Manifest.Contributes.Workbenches) == 0 {
				continue
			}
			row, err := s.repo.PluginRelease(release.ID)
			if err != nil {
				return nil, err
			}
			if row == nil {
				return nil, issue(409, "plugin_version_conflict", "工作台发布不存在")
			}
			files, err := s.loadPackage(*row)
			if err != nil {
				return nil, err
			}
			for _, ref := range release.Manifest.Contributes.Workbenches {
				// Listing does not imply an operation is executable in the current context.
				var definition contracts.Workbench
				if err = json.Unmarshal(files[ref.Ref], &definition); err != nil {
					return nil, err
				}
				result = append(result, WorkbenchView{ID: app.ID + "." + ref.ID, ReleaseID: release.ID, Version: release.Version, Definition: definition})
			}
		}
	}
	return result, nil
}

func (s *Service) composeWorkbench(repo *repository.Repository, user string, req WorkbenchComposeRequest) (WorkbenchView, WorkbenchPreview, error) {
	view, files, err := s.workbench(repo, user, req.ID, req.ReleaseID, req.Context)
	preview := WorkbenchPreview{}
	if err != nil {
		return view, preview, err
	}
	if len(req.RecipeIDs) > 16 || len(req.Input) > 32 {
		return view, preview, issue(400, "operation_input_invalid", "工作台输入超过上限")
	}
	// Apply the same JSON limits before preview and invocation, including numeric
	// interoperability. A successful preview must always be invocable.
	candidate := contracts.WorkbenchSelection{ID: req.ID, ReleaseID: view.ReleaseID, RecipeIDs: req.RecipeIDs, Input: req.Input, Digest: strings.Repeat("0", 64)}
	if err = contracts.Validate("workbenchSelection", []byte(encode(candidate))); err != nil {
		return view, preview, issue(400, "operation_input_invalid", "工作台输入格式无效")
	}
	preview.WorkbenchComposition, err = contracts.ComposeWorkbench(view.Definition, view.Recipes, req.RecipeIDs, req.Input)
	if err != nil {
		return view, preview, issue(400, "operation_input_invalid", err.Error())
	}
	if len(preview.Conflicts) > 0 {
		preview.ValidationMessage = "请先解决配方冲突"
		return view, preview, nil
	}
	if err = contracts.ValidateData(files, view.Definition.ContextSchemaRef, []byte(encode(preview.Input))); err != nil {
		preview.ValidationMessage = "请按字段格式填写所有必填项"
		return view, preview, nil
	}
	preview.Valid = true
	preview.Selection = contracts.WorkbenchSelection{ID: req.ID, ReleaseID: view.ReleaseID, RecipeIDs: append([]string{}, req.RecipeIDs...), Input: preview.Input}
	preview.Selection.Digest = strings.Repeat("0", 64)
	if err = contracts.Validate("workbenchSelection", []byte(encode(preview.Selection))); err != nil {
		return view, WorkbenchPreview{}, issue(400, "operation_input_invalid", "合并后的工作台输入超过合同范围")
	}
	preview.Selection.Digest = ""
	sort.Strings(preview.Selection.RecipeIDs)
	preview.Selection.Digest = hashBytes([]byte(encode(struct {
		Selection  contracts.WorkbenchSelection
		Context    contracts.InvocationContext
		Definition contracts.Workbench
		Recipes    map[string]contracts.Recipe
	}{preview.Selection, req.Context, view.Definition, view.Recipes})))
	return view, preview, nil
}
func (s *Service) ComposeWorkbench(user string, req WorkbenchComposeRequest) (WorkbenchPreview, error) {
	_, preview, err := s.composeWorkbench(s.repo, user, req)
	return preview, err
}
func (s *Service) verifyWorkbench(repo *repository.Repository, user string, selection contracts.WorkbenchSelection, ctx contracts.InvocationContext) (WorkbenchView, WorkbenchPreview, error) {
	ctx.ThreadID = ""
	ctx.WorkbenchID = ""
	raw := []byte(encode(selection))
	if err := contracts.Validate("workbenchSelection", raw); err != nil {
		return WorkbenchView{}, WorkbenchPreview{}, err
	}
	view, preview, err := s.composeWorkbench(repo, user, WorkbenchComposeRequest{ID: selection.ID, ReleaseID: selection.ReleaseID, RecipeIDs: selection.RecipeIDs, Input: selection.Input, Context: ctx})
	if err != nil {
		return view, preview, err
	}
	if !preview.Valid || preview.Selection.Digest != selection.Digest {
		return view, preview, issue(409, "run_revision_conflict", "工作台建议或输入已变化，请重新预览")
	}
	return view, preview, nil
}
func (s *Service) VerifyWorkbench(user string, selection contracts.WorkbenchSelection, ctx contracts.InvocationContext) (WorkbenchView, WorkbenchPreview, error) {
	return s.verifyWorkbench(s.repo, user, selection, ctx)
}
func (s *Service) validateWorkbenchInvocation(repo *repository.Repository, user string, request contracts.Invocation, ctx contracts.InvocationContext) error {
	if request.Workbench == nil {
		return nil
	}
	view, _, err := s.verifyWorkbench(repo, user, *request.Workbench, ctx)
	if err != nil {
		return err
	}
	if request.Operation != view.Definition.Operation || request.ReleaseID != view.ReleaseID {
		return issue(403, "scope_forbidden", "工作台操作或版本不匹配")
	}
	normalized, err := contracts.Decode([]byte(encode(request.Input)))
	if err != nil {
		return err
	}
	if encode(normalized) != encode(request.Workbench.Input) {
		return issue(409, "run_revision_conflict", "操作输入与预览不一致")
	}
	return nil
}

func WorkbenchHistory(run model.PluginRun) (*contracts.WorkbenchSelection, error) {
	var request contracts.Invocation
	if err := json.Unmarshal([]byte(run.RequestJSON), &request); err != nil {
		return nil, fmt.Errorf("decode historical workbench: %w", err)
	}
	return request.Workbench, nil
}
