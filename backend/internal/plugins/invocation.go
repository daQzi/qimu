package plugins

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"gorm.io/gorm"
	"infinite-canvas/backend/internal/kernel"
	"infinite-canvas/backend/internal/model"
	"infinite-canvas/backend/internal/plugins/contracts"
	"infinite-canvas/backend/internal/repository"
	"strings"
	"unicode/utf8"
)

type InvocationOutput struct {
	Kind       string          `json:"kind"`
	Result     json.RawMessage `json:"result,omitempty"`
	Digest     string          `json:"digest,omitempty"`
	RunID      string          `json:"runId,omitempty"`
	Status     string          `json:"status,omitempty"`
	Revision   int64           `json:"revision,omitempty"`
	ApprovalID string          `json:"approvalId,omitempty"`
}
type RunView struct {
	model.PluginRun
	Preview          json.RawMessage              `json:"preview,omitempty"`
	Result           json.RawMessage              `json:"result,omitempty"`
	ResultRef        *contracts.ResultRef         `json:"resultRef,omitempty"`
	View             *contracts.ResultView        `json:"view,omitempty"`
	CanvasActions    []CanvasAction               `json:"canvasActions,omitempty"`
	ExecutionAdapter string                       `json:"executionAdapter,omitempty"`
	Remote           *model.PluginRemoteExecution `json:"remote,omitempty"`
	Pipeline         *PipelineView                `json:"pipeline,omitempty"`
}

func hashBytes(value []byte) string { sum := sha256.Sum256(value); return hex.EncodeToString(sum[:]) }

func (s *Service) Invoke(userID, key string, request contracts.Invocation, policy InvocationPolicy) (InvocationOutput, error) {
	raw, err := json.Marshal(request)
	if err != nil {
		return InvocationOutput{}, err
	}
	if err = contracts.Validate("invocation", raw); err != nil {
		return InvocationOutput{}, issue(400, "operation_input_invalid", "调用参数不符合合同")
	}
	// Decode then marshal once to normalize insignificant input JSON whitespace.
	value, err := contracts.Decode(raw)
	if err != nil {
		return InvocationOutput{}, err
	}
	raw, err = json.Marshal(value)
	if err != nil {
		return InvocationOutput{}, err
	}
	var normalized contracts.Invocation
	if err = json.Unmarshal(raw, &normalized); err != nil {
		return InvocationOutput{}, err
	}
	ctx := contracts.InvocationContext{}
	if normalized.Context != nil {
		ctx = *normalized.Context
	}
	envelope, err := json.Marshal(struct {
		Request json.RawMessage
		Mode    string
		Agent   string
	}{raw, policy.PermissionMode, policy.AgentRunID})
	if err != nil {
		return InvocationOutput{}, err
	}
	requestHash := hashBytes(envelope)
	output := InvocationOutput{}
	err = s.repo.WithPluginCatalog(func(repo *repository.Repository) error {
		if policy.AgentRunID != "" {
			if err := repo.LockActivePluginAgent(userID, policy.AgentRunID, policy.AgentRevision); err != nil {
				if errors.Is(err, repository.ErrCreationConflict) || errors.Is(err, gorm.ErrRecordNotFound) {
					return issue(409, "run_revision_conflict", "Agent 已停止或检查点已变化")
				}
				return err
			}
		}
		resolved, err := s.resolve(repo, userID, request.Operation, request.ReleaseID, ctx, policy)
		if err != nil {
			return err
		}
		input, err := json.Marshal(normalized.Input)
		if err != nil {
			return err
		}
		if err = contracts.ValidateData(resolved.Files, resolved.Definition.InputSchemaRef, input); err != nil {
			return issue(400, "operation_input_invalid", "输入不符合操作 Schema")
		}
		readOnly := len(resolved.Adapter.Effects) == 1 && resolved.Adapter.Effects[0] == "read"
		if !readOnly {
			if len(key) < 8 || len(key) > 160 || !utf8.ValidString(key) || strings.TrimSpace(key) != key {
				return issue(400, "operation_input_invalid", "持久调用需要 8–160 字节的幂等键")
			}
			existing, err := repo.PluginRunByKey(userID, key)
			if err != nil {
				return err
			}
			if existing != nil {
				if existing.RequestDigest != requestHash {
					return issue(409, "run_revision_conflict", "幂等键已用于不同请求")
				}
				output = invocationRun(*existing)
				return nil
			}
		}
		hostContext := HostOperationContext{InvocationContext: ctx, ReleaseID: resolved.Release.ID, Files: resolved.Files}
		if resolved.Definition.Execution.Kind == "pipeline" {
			output, err = s.createPipeline(repo, userID, key, requestHash, raw, resolved, policy)
			return err
		}
		var plan PreparedOperation
		var remote PreparedRemoteOperation
		if resolved.Definition.Execution.Kind == "http" {
			remote, err = s.remote.Prepare(repo, userID, normalized, hostContext, policy)
			plan = PreparedOperation{Result: remote.Preview, SourceDigest: remote.SourceDigest}
		} else {
			plan, err = resolved.Adapter.Prepare(repo, userID, normalized.Input, hostContext)
		}
		if err != nil {
			return err
		}
		if len(plan.Result) > 64<<10 {
			return issue(400, "upstream_output_invalid", "操作结果超过短操作上限")
		}
		if resolved.Definition.Execution.Kind != "http" {
			if err = contracts.ValidateData(resolved.Files, resolved.Definition.OutputSchemaRef, plan.Result); err != nil {
				return issue(400, "upstream_output_invalid", "操作输出不符合 Schema")
			}
		}
		if readOnly {
			output = InvocationOutput{Kind: "inline", Result: plan.Result, Digest: hashBytes(plan.Result)}
			return nil
		}
		run := model.PluginRun{ID: kernel.NewID(), UserID: userID, AgentRunID: policy.AgentRunID, IdempotencyKey: key, RequestDigest: requestHash, ReleaseID: resolved.Release.ID, ReleaseVersion: resolved.Release.Version, Operation: request.Operation, ContractHash: resolved.ContractHash, RequestJSON: string(raw), PlanJSON: string(plan.Result), SourceResourceID: plan.SourceResourceID, SourceDigest: plan.SourceDigest, Status: "waiting_approval", Revision: 1, ApprovalID: kernel.NewID()}
		run.ConnectionVersionID = remote.ConnectionVersionID
		stepStatus := run.Status
		step := model.PluginRunStep{ID: kernel.NewID(), RunID: run.ID, StepKey: "invoke", ItemKey: "", Attempt: 1, Status: stepStatus, InputDigest: hashBytes(input)}
		if err = repo.CreatePluginRun(&run, &step); err != nil {
			return err
		}
		if len(remote.ResourceIDs) > 0 {
			if err = repo.AddPluginRunResources(userID, run.ID, "input", remote.ResourceIDs); err != nil {
				return err
			}
		}
		run.Revision++
		if err = appendRunEvent(repo, &run, "run.created"); err != nil {
			return err
		}
		event := "approval.requested"
		if run.Status == "succeeded" {
			event = "result.ready"
		}
		if err = appendRunEvent(repo, &run, event); err != nil {
			return err
		}
		if err = repo.SavePluginRun(&run, 1, stepStatus); err != nil {
			return err
		}
		output = invocationRun(run)
		return nil
	})
	return output, err
}
func invocationRun(run model.PluginRun) InvocationOutput {
	return InvocationOutput{Kind: "run", RunID: run.ID, Status: run.Status, Revision: run.Revision, ApprovalID: run.ApprovalID}
}
func appendRunEvent(repo *repository.Repository, run *model.PluginRun, kind string) error {
	run.EventSequence++
	payload, err := json.Marshal(map[string]any{"status": run.Status, "revision": run.Revision})
	if err != nil {
		return err
	}
	return repo.AppendPluginRunEvent(&model.PluginRunEvent{ID: kernel.NewID(), RunID: run.ID, Sequence: run.EventSequence, Type: kind, PayloadJSON: string(payload)})
}

func SaveRunTransition(repo *repository.Repository, run *model.PluginRun, event string) error {
	expected := run.Revision
	run.Revision++
	if err := appendRunEvent(repo, run, event); err != nil {
		return err
	}
	return repo.SavePluginRun(run, expected, run.Status)
}
func (s *Service) GetRun(userID, id string, viewID ...string) (RunView, error) {
	run, err := s.repo.PluginRunForUser(userID, id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return RunView{}, kernel.NotFound("插件运行不存在")
	}
	if err != nil {
		return RunView{}, err
	}
	view := RunView{PluginRun: *run, Preview: json.RawMessage(run.PlanJSON)}
	if run.Status == "succeeded" {
		view.Result = json.RawMessage(run.ResultJSON)
		view.ResultRef = &contracts.ResultRef{RunID: run.ID, StepKey: "invoke", Attempt: max(1, run.Attempt), OutputKey: "result", SchemaID: run.Operation + "/result", SchemaVersion: run.ReleaseVersion, Digest: hashBytes([]byte(run.ResultJSON))}
	}
	if err := s.decorateRunView(&view, viewID...); err != nil {
		return RunView{}, err
	}
	if run.TaskID != nil {
		remote, err := s.repo.PluginRemoteForRun(userID, id)
		if err != nil {
			return RunView{}, err
		}
		view.Remote = remote
	}
	if run.PipelineID != "" {
		view.Pipeline, err = s.pipelineView(run)
		if err != nil {
			return RunView{}, err
		}
	}
	return view, nil
}

func (s *Service) Decide(userID, id, approvalID, decision string, revision int64) (RunView, error) {
	if decision != "approve" && decision != "reject" {
		return RunView{}, issue(400, "operation_input_invalid", "无效审批决定")
	}
	err := s.repo.WithPluginCatalog(func(repo *repository.Repository) error {
		run, err := repo.PluginRunForUser(userID, id)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return kernel.NotFound("插件运行不存在")
		}
		if err != nil {
			return err
		}
		if run.ApprovalID != approvalID || approvalID == "" {
			return issue(409, "run_revision_conflict", "审批标识不匹配")
		}
		if run.ApprovalDecision == decision && (run.Status == "succeeded" || run.Status == "cancelled" || run.TaskID != nil) {
			return nil
		}
		if run.Revision != revision || run.Status != "waiting_approval" {
			return issue(409, "run_revision_conflict", "运行状态已变化")
		}
		if decision == "approve" {
			if run.ParentRunID != "" {
				parent, err := repo.PluginRunForUser(userID, run.ParentRunID)
				if err != nil {
					return err
				}
				if !contains([]string{"queued", "running", "waiting_input", "waiting_approval"}, parent.Status) {
					return issue(409, "run_revision_conflict", "父流程已停止，不能批准新任务")
				}
			}
			var request contracts.Invocation
			if err = json.Unmarshal([]byte(run.RequestJSON), &request); err != nil {
				return err
			}
			ctx := contracts.InvocationContext{}
			if request.Context != nil {
				ctx = *request.Context
			}
			resolved, err := s.resolve(repo, userID, request.Operation, request.ReleaseID, ctx, InvocationPolicy{PermissionMode: "request_approval"})
			if err != nil {
				return err
			}
			if resolved.ContractHash != run.ContractHash {
				return issue(409, "plugin_version_conflict", "合同摘要已变化")
			}
			if resolved.Definition.Execution.Kind == "http" {
				prepared, err := s.remote.Prepare(repo, userID, request, HostOperationContext{InvocationContext: ctx, ReleaseID: resolved.Release.ID, Files: resolved.Files}, InvocationPolicy{PermissionMode: "request_approval", AgentRunID: run.AgentRunID})
				if err != nil {
					return err
				}
				if prepared.SourceDigest != run.SourceDigest || prepared.ConnectionVersionID != run.ConnectionVersionID || hashBytes(prepared.Preview) != hashBytes([]byte(run.PlanJSON)) {
					return issue(409, "quote_changed", "连接、资源或报价已变化，请重新发起运行并确认")
				}
				if err = prepared.Enqueue(repo, run); err != nil {
					return err
				}
				run.Status = "running"
			} else {
				prepared, err := resolved.Adapter.Prepare(repo, userID, request.Input, HostOperationContext{InvocationContext: ctx, ReleaseID: resolved.Release.ID, Files: resolved.Files})
				if err != nil {
					return err
				}
				if prepared.SourceResourceID != run.SourceResourceID || prepared.SourceDigest != run.SourceDigest || hashBytes(prepared.Result) != hashBytes([]byte(run.PlanJSON)) {
					return issue(409, "run_revision_conflict", "资源信息已变化，请发起新快照并重新确认")
				}
				run.Status = "succeeded"
				run.ResultJSON = run.PlanJSON
				if prepared.Commit != nil {
					if err = prepared.Commit(repo); err != nil {
						return err
					}
				}
				if prepared.ProjectsToCanvas {
					run.ProjectionStatus = "applied"
				}
				run.FailureMessage = ""
			}
		} else {
			run.Status = "cancelled"
			run.SourceResourceID = ""
			if run.ConnectionVersionID != "" {
				if err = repo.ReleasePluginInputResources(userID, id); err != nil {
					return err
				}
			}
		}
		run.ApprovalDecision = decision
		run.Revision++
		if err = appendRunEvent(repo, run, "approval.decided"); err != nil {
			return err
		}
		event := "run.cancelled"
		if run.Status == "succeeded" {
			event = "result.ready"
		}
		if run.Status == "running" {
			event = "task.queued"
		}
		if err = appendRunEvent(repo, run, event); err != nil {
			return err
		}
		return repo.SavePluginRun(run, revision, run.Status)
	})
	if err != nil {
		var appErr *kernel.AppError
		if errors.As(err, &appErr) && appErr.Reason == "projection_conflict" {
			if recordErr := s.repo.RecordPluginProjectionConflict(userID, id, revision, appErr.Message); recordErr != nil {
				return RunView{}, recordErr
			}
		}
		return RunView{}, err
	}
	return s.GetRun(userID, id)
}
func (s *Service) Cancel(userID, id string, revision int64) (RunView, error) {
	err := s.repo.WithPluginCatalog(func(repo *repository.Repository) error {
		run, err := repo.PluginRunForUser(userID, id)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return kernel.NotFound("插件运行不存在")
		}
		if err != nil {
			return err
		}
		if run.Status == "cancelled" || run.Status == "cancelling" {
			return nil
		}
		if run.PipelineID != "" {
			return s.cancelPipeline(repo, run, revision)
		}
		if run.TaskID != nil {
			if s.remote == nil || run.Revision != revision || (run.Status != "running" && run.Status != "paused") {
				return issue(409, "run_revision_conflict", "运行状态已变化")
			}
			if err = s.remote.Cancel(repo, run); err != nil {
				return err
			}
			return SaveRunTransition(repo, run, "cancellation.requested")
		}
		if run.Status != "waiting_approval" || run.Revision != revision {
			return issue(409, "run_revision_conflict", "运行已结束或状态已变化")
		}
		run.Status = "cancelled"
		run.SourceResourceID = ""
		if run.ConnectionVersionID != "" {
			if err = repo.ReleasePluginInputResources(userID, id); err != nil {
				return err
			}
		}
		run.Revision++
		if err = appendRunEvent(repo, run, "run.cancelled"); err != nil {
			return err
		}
		return repo.SavePluginRun(run, revision, "cancelled")
	})
	if err != nil {
		return RunView{}, err
	}
	return s.GetRun(userID, id)
}

func (s *Service) Resume(userID, id string, revision int64, action, providerJobID string) (RunView, error) {
	err := s.repo.WithPluginCatalog(func(repo *repository.Repository) error {
		run, err := repo.PluginRunForUser(userID, id)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return kernel.NotFound("插件运行不存在")
		}
		if err != nil {
			return err
		}
		if run.PipelineID != "" {
			if run.Revision != revision || run.Status != "paused" || action != "retry_safe" {
				return issue(409, "run_revision_conflict", "流程当前不可恢复")
			}
			run.Status = "running"
			run.FailureMessage = ""
			return SaveRunTransition(repo, run, "run.resumed")
		}
		if run.Revision != revision || run.Status != "paused" || run.TaskID == nil || s.remote == nil {
			return issue(409, "run_revision_conflict", "运行当前不可恢复")
		}
		if err = s.remote.Resume(repo, run, RemoteResumeRequest{Action: action, ProviderJobID: providerJobID}); err != nil {
			return err
		}
		return SaveRunTransition(repo, run, "run.resumed")
	})
	if err != nil {
		return RunView{}, err
	}
	return s.GetRun(userID, id)
}
