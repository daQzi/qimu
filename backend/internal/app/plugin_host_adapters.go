package app

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"gorm.io/gorm"
	"infinite-canvas/backend/internal/kernel"
	"infinite-canvas/backend/internal/model"
	"infinite-canvas/backend/internal/plugins"
	"infinite-canvas/backend/internal/plugins/contracts"
	"infinite-canvas/backend/internal/repository"
	"time"
)

func pluginHostAdapters() []plugins.ShortHostAdapter {
	prepare := func(repo *repository.Repository, userID string, input map[string]json.RawMessage, _ plugins.HostOperationContext) (plugins.PreparedOperation, error) {
		var id string
		if err := json.Unmarshal(input["resourceId"], &id); err != nil || id == "" {
			return plugins.PreparedOperation{}, BadAuthRequest("需要真实资源 ID")
		}
		resource, err := repo.LockResourceForUser(userID, id)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return plugins.PreparedOperation{}, Forbidden("资源不存在或不属于当前用户")
		}
		if err != nil {
			return plugins.PreparedOperation{}, err
		}
		if resource.Kind != "video" || resource.Status != model.ResourceStatusReady {
			return plugins.PreparedOperation{}, BadAuthRequest("请选择已就绪的视频资源")
		}
		result := map[string]any{"resourceId": resource.ID, "kind": "video", "name": resource.ID, "mimeType": resource.MimeType}
		if resource.Width > 0 {
			result["width"] = resource.Width
		}
		if resource.Height > 0 {
			result["height"] = resource.Height
		}
		if resource.DurationMs > 0 {
			result["durationMs"] = resource.DurationMs
		}
		raw, err := json.Marshal(result)
		if err != nil {
			return plugins.PreparedOperation{}, err
		}
		if expected, ok := input["expectedDigest"]; ok {
			var value string
			sum := sha256.Sum256(raw)
			if json.Unmarshal(expected, &value) != nil || value != hex.EncodeToString(sum[:]) {
				return plugins.PreparedOperation{}, creationConflict("视频信息与读取结果不一致，请重新检查后保存")
			}
		}
		digest := sha256.Sum256(append(append([]byte{}, raw...), []byte(resource.UpdatedAt.UTC().Format(time.RFC3339Nano))...))
		return plugins.PreparedOperation{Result: raw, SourceResourceID: resource.ID, SourceDigest: hex.EncodeToString(digest[:])}, nil
	}
	return []plugins.ShortHostAdapter{
		{ID: "resource.inspect", Permissions: []string{"media.read"}, Effects: []string{"read"}, Prepare: prepare},
		{ID: "resource.snapshot", Permissions: []string{"media.read", "resource.create"}, Effects: []string{"draft_write"}, Prepare: prepare},
		{ID: "canvas.blueprint.instantiate", Permissions: []string{"canvas.read", "canvas.write"}, Effects: []string{"draft_write"}, Prepare: preparePluginCanvasProjection},
	}
}
func (s *Service) pluginOperationAccess(userID string) error {
	if userID == "" {
		return kernel.Unauthorized("请先登录")
	}
	actor, err := s.repo.User(userID)
	if err != nil {
		return err
	}
	if actor.Status != model.UserStatusActive {
		return Forbidden("账号不可用")
	}
	if actor.Role != model.UserRoleAdmin {
		return s.RequireFeature(FeaturePluginCenter)
	}
	return nil
}

type PluginOperationInvocation = contracts.Invocation
type PluginOperationContext = contracts.InvocationContext
type PluginOperationDescription = plugins.OperationDescription
type PluginInvocationOutput = plugins.InvocationOutput
type PluginRunView = plugins.RunView

func (s *Service) InvokePluginOperation(userID, key string, request contracts.Invocation) (plugins.InvocationOutput, error) {
	if err := s.pluginOperationAccess(userID); err != nil {
		return plugins.InvocationOutput{}, err
	}
	return s.applicationPlugins().Invoke(userID, key, request, plugins.InvocationPolicy{PermissionMode: "request_approval"})
}
func (s *Service) DescribePluginOperation(userID, address, releaseID string, ctx contracts.InvocationContext) (plugins.OperationDescription, error) {
	if err := s.pluginOperationAccess(userID); err != nil {
		return plugins.OperationDescription{}, err
	}
	return s.applicationPlugins().Describe(userID, address, releaseID, ctx, plugins.InvocationPolicy{PermissionMode: "request_approval"})
}
func (s *Service) SearchPluginOperations(userID, query string, offset int, ctx contracts.InvocationContext) ([]plugins.OperationSummary, int, error) {
	if err := s.pluginOperationAccess(userID); err != nil {
		return nil, 0, err
	}
	return s.applicationPlugins().Search(userID, query, offset, ctx, plugins.InvocationPolicy{PermissionMode: "request_approval"})
}
func (s *Service) PluginRun(userID, id string, viewID ...string) (plugins.RunView, error) {
	return s.applicationPlugins().GetRun(userID, id, viewID...)
}
func (s *Service) DecidePluginRun(userID, id, approvalID, decision string, revision int64) (plugins.RunView, error) {
	if err := s.pluginOperationAccess(userID); err != nil {
		return plugins.RunView{}, err
	}
	return s.applicationPlugins().Decide(userID, id, approvalID, decision, revision)
}
func (s *Service) CancelPluginRun(userID, id string, revision int64) (plugins.RunView, error) {
	return s.applicationPlugins().Cancel(userID, id, revision)
}
func (s *Service) ResumePluginRun(userID, id string, revision int64, action, providerJobID string) (plugins.RunView, error) {
	if err := s.pluginOperationAccess(userID); err != nil {
		return plugins.RunView{}, err
	}
	return s.applicationPlugins().Resume(userID, id, revision, action, providerJobID)
}
