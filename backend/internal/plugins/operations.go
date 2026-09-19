package plugins

import (
	"encoding/json"
	"errors"
	"gorm.io/gorm"
	"infinite-canvas/backend/internal/kernel"
	"infinite-canvas/backend/internal/model"
	"infinite-canvas/backend/internal/plugins/contracts"
	"infinite-canvas/backend/internal/repository"
	"sort"
	"strings"
	"unicode/utf8"
)

type resolvedOperation struct {
	Release      *model.PluginRelease
	Definition   contracts.Operation
	Adapter      ShortHostAdapter
	Files        contracts.PackageFiles
	ContractHash string
}

func (s *Service) adapterFor(op contracts.Operation) (ShortHostAdapter, error) {
	adapter, ok := s.adapters[op.Execution.Adapter]
	if !ok || adapter.Prepare == nil || op.Execution.Kind != "host" || op.Execution.Mode != "inline" {
		return adapter, issue(409, "operation_unavailable", "宿主尚未提供此执行能力")
	}
	if len(op.Effects) != len(adapter.Effects) {
		return adapter, issue(403, "scope_forbidden", "操作效果与宿主合同不符")
	}
	for _, effect := range adapter.Effects {
		if !contains(op.Effects, effect) {
			return adapter, issue(403, "scope_forbidden", "操作不能降低实际效果")
		}
	}
	for _, permission := range adapter.Permissions {
		if !contains(op.RequiredPermissions, permission) {
			return adapter, issue(403, "scope_forbidden", "操作缺少宿主最低权限声明")
		}
	}
	return adapter, nil
}
func contains(values []string, target string) bool {
	for _, v := range values {
		if v == target {
			return true
		}
	}
	return false
}

func (s *Service) resolve(repo *repository.Repository, userID, address, releaseID string, ctx contracts.InvocationContext, policy InvocationPolicy) (resolvedOperation, error) {
	var resolved resolvedOperation
	if err := s.admission(); err != nil {
		return resolved, err
	}
	raw, _ := json.Marshal(address)
	if err := contracts.Validate("address", raw); err != nil {
		return resolved, issue(400, "operation_input_invalid", "操作名称无效")
	}
	if userID == "" {
		return resolved, kernel.Unauthorized("请先登录")
	}
	if policy.PermissionMode != "read_only" && policy.PermissionMode != "request_approval" && policy.PermissionMode != "auto" {
		return resolved, issue(400, "scope_forbidden", "无效执行权限模式")
	}
	parts := strings.Split(address, ".")
	state, err := repo.UserPluginState(userID, parts[0])
	if err != nil {
		return resolved, err
	}
	if state == nil || !state.Enabled {
		return resolved, issue(403, "plugin_disabled", "请先启用插件")
	}
	if releaseID == "" {
		releaseID = state.InstalledReleaseID
	}
	if state.InstalledReleaseID != releaseID {
		return resolved, issue(409, "plugin_version_conflict", "插件激活版本已变化，请重新查阅合同")
	}
	release, err := repo.PluginRelease(releaseID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return resolved, kernel.NotFound("插件发布不存在")
	}
	if err != nil {
		return resolved, err
	}
	if release.PluginID != parts[0] {
		return resolved, issue(403, "scope_forbidden", "操作与发布版本不匹配")
	}
	if err = checkRelease(repo, userID, release); err != nil {
		return resolved, err
	}
	files, err := s.loadPackage(*release)
	if err != nil {
		return resolved, err
	}
	var manifest contracts.Manifest
	if err = json.Unmarshal(files["manifest.json"], &manifest); err != nil {
		return resolved, err
	}
	var op contracts.Operation
	found := false
	for _, ref := range manifest.Contributes.Operations {
		if ref.ID == parts[1] {
			if err = json.Unmarshal(files[ref.Ref], &op); err != nil {
				return resolved, err
			}
			found = true
			break
		}
	}
	if !found {
		return resolved, kernel.NotFound("操作不存在")
	}
	adapter, err := s.adapterFor(op)
	if err != nil {
		return resolved, err
	}
	var grants []string
	if err = json.Unmarshal([]byte(state.GrantedPermissionsJSON), &grants); err != nil {
		return resolved, err
	}
	for _, p := range op.RequiredPermissions {
		if !contains(grants, p) {
			return resolved, issue(403, "scope_forbidden", "缺少操作所需权限："+p)
		}
	}
	if policy.PermissionMode == "read_only" && (!contains(adapter.Effects, "read") || len(adapter.Effects) != 1) {
		return resolved, issue(403, "scope_forbidden", "只读模式不能调用写操作")
	}
	if err = validateOperationContext(repo, userID, ctx, op); err != nil {
		return resolved, err
	}
	resolved = resolvedOperation{Release: release, Definition: op, Adapter: adapter, Files: files, ContractHash: contracts.OperationDigest(release.Digest, address)}
	return resolved, nil
}

func validateOperationContext(repo *repository.Repository, userID string, ctx contracts.InvocationContext, op contracts.Operation) error {
	if ctx.HostSurface != "" && ctx.HostSurface != "agent-home" && ctx.HostSurface != "canvas" && ctx.HostSurface != "editor" {
		return issue(400, "operation_input_invalid", "未知上下文入口")
	}
	if ctx.ThreadID != "" || ctx.WorkbenchID != "" {
		return issue(409, "operation_unavailable", "尚未开放工作台或会话绑定上下文")
	}
	if (op.Context.RequiresCanvas || ctx.HostSurface == "canvas") && ctx.CanvasID == "" {
		return issue(409, "operation_unavailable", "此操作需要真实画布")
	}
	if op.Context.RequiresProject && ctx.ProjectID == "" {
		return issue(409, "operation_unavailable", "此操作需要真实项目")
	}
	if ctx.CanvasID != "" {
		canvas, err := repo.CanvasProjectForUser(userID, ctx.CanvasID)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return issue(403, "scope_forbidden", "画布不存在或不属于当前用户")
			}
			return err
		}
		if ctx.ProjectID != "" && ctx.ProjectID != canvas.ProjectID {
			return issue(403, "scope_forbidden", "画布与项目不匹配")
		}
	}
	if ctx.ProjectID != "" {
		if _, err := repo.ProjectForUser(userID, ctx.ProjectID); err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return issue(403, "scope_forbidden", "项目不存在或不属于当前用户")
			}
			return err
		}
	}
	return nil
}

func (s *Service) Describe(userID, address, releaseID string, ctx contracts.InvocationContext, policy InvocationPolicy) (OperationDescription, error) {
	resolved, err := s.resolve(s.repo, userID, address, releaseID, ctx, policy)
	if err != nil {
		return OperationDescription{}, err
	}
	schemas := map[string]json.RawMessage{}
	size := 0
	for path, raw := range resolved.Files {
		if strings.HasPrefix(path, "schemas/") {
			size += len(raw)
			if size > 24000 {
				return OperationDescription{}, issue(409, "operation_unavailable", "操作 Schema 超过当前描述接口上限")
			}
			schemas[path] = json.RawMessage(raw)
		}
	}
	var manifest contracts.Manifest
	if err = json.Unmarshal(resolved.Files["manifest.json"], &manifest); err != nil {
		return OperationDescription{}, err
	}
	var view *contracts.ResultView
	for _, ref := range manifest.Contributes.Views {
		if ref.ID == resolved.Definition.ResultView {
			if err = json.Unmarshal(resolved.Files[ref.Ref], &view); err != nil {
				return OperationDescription{}, err
			}
		}
	}
	return OperationDescription{Address: address, ReleaseID: resolved.Release.ID, ContractHash: resolved.ContractHash, Definition: resolved.Definition, Schemas: schemas, Available: true, ResultView: view}, nil
}

func (s *Service) Search(userID, query string, offset int, ctx contracts.InvocationContext, policy InvocationPolicy) ([]OperationSummary, int, error) {
	if userID == "" {
		return nil, 0, kernel.Unauthorized("请先登录")
	}
	if policy.PermissionMode != "read_only" && policy.PermissionMode != "request_approval" && policy.PermissionMode != "auto" {
		return nil, 0, issue(400, "scope_forbidden", "无效执行模式")
	}
	if offset < 0 || offset > 12800 || utf8.RuneCountInString(query) > 200 {
		return nil, 0, issue(400, "operation_input_invalid", "检索范围无效")
	}
	if err := s.admission(); err != nil {
		return nil, 0, err
	}
	states, err := s.repo.UserPluginStates(userID)
	if err != nil {
		return nil, 0, err
	}
	items := []OperationSummary{}
	query = strings.ToLower(strings.TrimSpace(query))
	for _, state := range states {
		if !state.Enabled || state.InstalledReleaseID == "" {
			continue
		}
		release, err := s.repo.PluginRelease(state.InstalledReleaseID)
		if err != nil {
			return nil, 0, err
		}
		if err = checkRelease(s.repo, userID, release); err != nil {
			var appErr *kernel.AppError
			if errors.As(err, &appErr) {
				continue
			}
			return nil, 0, err
		}
		var ops map[string]contracts.Operation
		if err = json.Unmarshal([]byte(release.OperationsJSON), &ops); err != nil {
			return nil, 0, err
		}
		var grants []string
		if err = json.Unmarshal([]byte(state.GrantedPermissionsJSON), &grants); err != nil {
			return nil, 0, err
		}
		for _, op := range ops {
			address := release.PluginID + "." + op.ID
			if query != "" && !strings.Contains(strings.ToLower(address+" "+op.Description), query) {
				continue
			}
			adapter, err := s.adapterFor(op)
			if err != nil {
				continue
			}
			allowed := true
			for _, p := range op.RequiredPermissions {
				allowed = allowed && contains(grants, p)
			}
			if !allowed {
				continue
			}
			if policy.PermissionMode == "read_only" && (len(adapter.Effects) != 1 || adapter.Effects[0] != "read") {
				continue
			}
			if err = validateOperationContext(s.repo, userID, ctx, op); err != nil {
				var appErr *kernel.AppError
				if errors.As(err, &appErr) {
					continue
				}
				return nil, 0, err
			}
			description := []rune(op.Description)
			if len(description) > 400 {
				description = description[:400]
			}
			items = append(items, OperationSummary{Operation: address, ReleaseID: release.ID, Description: string(description), Effects: adapter.Effects})
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Operation < items[j].Operation })
	if offset >= len(items) {
		return []OperationSummary{}, 0, nil
	}
	end := min(offset+20, len(items))
	next := 0
	if end < len(items) {
		next = end
	}
	return items[offset:end], next, nil
}
