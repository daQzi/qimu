package app

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"infinite-canvas/backend/internal/kernel"
	"infinite-canvas/backend/internal/model"
	"infinite-canvas/backend/internal/plugins"
	"infinite-canvas/backend/internal/plugins/contracts"
	"infinite-canvas/backend/internal/repository"
)

type cloudAgentPluginLock struct {
	ReleaseID    string `json:"releaseId"`
	ContractHash string `json:"contractHash"`
}
type cloudAgentExecutionRef struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
}

func cloudAgentHasPendingPluginInvocation(state cloudAgentRuntime) bool {
	for _, call := range state.Calls {
		if call.Function.Name == "operation_invoke" {
			return true
		}
	}
	return false
}
func cloudAgentPluginDescriptionReceipt(result any) any {
	if description, ok := result.(plugins.OperationDescription); ok {
		return map[string]any{"operation": description.Address, "releaseId": description.ReleaseID, "contractHash": description.ContractHash}
	}
	return result
}

func isCloudAgentPluginTool(name string) bool {
	switch name {
	case "operation_search", "operation_describe", "operation_invoke", "plugin_run_get", "plugin_run_resume", "plugin_run_cancel", "result_read":
		return true
	}
	return false
}
func (s *Service) advanceCloudAgentPluginTool(run *model.CloudAgentExecution, state *cloudAgentRuntime, call cloudAgentCall) error {
	var result any
	err := s.pluginOperationAccess(run.UserID)
	if err == nil {
		result, err = s.executeCloudAgentPluginTool(run, state, call)
	}
	return s.repo.MutateCloudAgent(run.UserID, run.ID, run.Revision, func(current *model.CloudAgentExecution, _ *repository.Repository) error {
		cloudAgentToolResult(run.ID, state, call, result, err)
		return cloudAgentSave(current, state)
	})
}
func (s *Service) executeCloudAgentPluginTool(run *model.CloudAgentExecution, state *cloudAgentRuntime, call cloudAgentCall) (any, error) {
	service := s.applicationPlugins()
	ctx := contracts.InvocationContext{HostSurface: "agent-home"}
	if state.Request.CanvasID != "" {
		ctx.HostSurface = "canvas"
		ctx.CanvasID = state.Request.CanvasID
	}
	policy := plugins.InvocationPolicy{PermissionMode: state.Request.PermissionMode, AgentRunID: run.ID, AgentRevision: run.Revision}
	switch call.Function.Name {
	case "operation_search":
		var args struct {
			Query  string `json:"query"`
			Offset int    `json:"offset"`
		}
		if err := decodeCloudAgentJSONObject(call.Function.Arguments, &args); err != nil {
			return nil, err
		}
		items, next, err := service.Search(run.UserID, args.Query, args.Offset, ctx, policy)
		return map[string]any{"operations": items, "nextOffset": next}, err
	case "operation_describe", "operation_invoke":
		var args struct {
			Operation string                     `json:"operation"`
			ReleaseID string                     `json:"releaseId"`
			Input     map[string]json.RawMessage `json:"input"`
		}
		if err := decodeCloudAgentJSONObject(call.Function.Arguments, &args); err != nil {
			return nil, err
		}
		lock, locked := state.PluginLocks[args.Operation]
		if locked {
			if args.ReleaseID != "" && args.ReleaseID != lock.ReleaseID {
				return nil, creationConflict("本轮操作版本已经固定，请开始新一轮使用其他版本")
			}
			args.ReleaseID = lock.ReleaseID
		}
		if call.Function.Name == "operation_describe" {
			if len(state.PluginLocks) >= 64 && !locked {
				return nil, BadAuthRequest("本轮固定操作已达上限")
			}
			description, err := service.Describe(run.UserID, args.Operation, args.ReleaseID, ctx, policy)
			if err != nil {
				return nil, err
			}
			if locked && description.ContractHash != lock.ContractHash {
				return nil, creationConflict("操作合同摘要已变化")
			}
			if state.PluginLocks == nil {
				state.PluginLocks = map[string]cloudAgentPluginLock{}
			}
			state.PluginLocks[args.Operation] = cloudAgentPluginLock{ReleaseID: description.ReleaseID, ContractHash: description.ContractHash}
			return description, nil
		}
		if !locked {
			return nil, BadAuthRequest("请先调用 operation_describe 读取并固定该操作合同")
		}
		description, err := service.Describe(run.UserID, args.Operation, lock.ReleaseID, ctx, policy)
		if err != nil {
			return nil, err
		}
		if description.ContractHash != lock.ContractHash {
			return nil, creationConflict("操作合同不匹配")
		}
		digest := sha256.Sum256([]byte(run.ID + "\x00" + call.ID))
		key := "agent:" + hex.EncodeToString(digest[:])
		if state.PendingExecution != nil && !(len(description.Definition.Effects) == 1 && description.Definition.Effects[0] == "read") {
			pending, err := service.GetRun(run.UserID, state.PendingExecution.ID)
			if err != nil {
				return nil, err
			}
			if pending.Status == "waiting_approval" && pending.IdempotencyKey != key {
				return nil, &kernel.AppError{Status: 409, Code: 409, Reason: "approval_required", Message: "已有快照等待用户确认，请先处理运行 " + pending.ID}
			}
			if pending.Status != "waiting_approval" {
				state.PendingExecution = nil
			}
		}
		output, err := service.Invoke(run.UserID, key, contracts.Invocation{Operation: args.Operation, ReleaseID: lock.ReleaseID, Input: args.Input, Context: &ctx}, policy)
		if err == nil && output.Kind == "run" {
			state.PendingExecution = &cloudAgentExecutionRef{Kind: "plugin_run", ID: output.RunID}
		}
		return output, err
	default:
		var args struct {
			RunID    string `json:"runId"`
			Revision int64  `json:"revision"`
		}
		if err := decodeCloudAgentJSONObject(call.Function.Arguments, &args); err != nil {
			return nil, err
		}
		if call.Function.Name == "plugin_run_cancel" {
			if state.Request.PermissionMode == "read_only" {
				return nil, Forbidden("只读模式不能取消插件运行")
			}
			return service.Cancel(run.UserID, args.RunID, args.Revision)
		}
		result, err := service.GetRun(run.UserID, args.RunID)
		if err != nil {
			return nil, err
		}
		if call.Function.Name == "result_read" {
			if result.Status != "succeeded" {
				return nil, BadAuthRequest("运行尚无成功结果")
			}
			return map[string]any{"result": result.Result, "reference": result.ResultRef}, nil
		}
		return result, nil
	}
}
