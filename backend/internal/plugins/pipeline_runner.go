package plugins

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"infinite-canvas/backend/internal/kernel"
	"infinite-canvas/backend/internal/model"
	"infinite-canvas/backend/internal/plugins/contracts"
	"infinite-canvas/backend/internal/repository"
)

func (s *Service) createPipeline(repo *repository.Repository, user, key, digest string, raw []byte, resolved resolvedOperation, policy InvocationPolicy) (InvocationOutput, error) {
	p, err := contracts.LoadPipeline(resolved.Files, resolved.Definition.Execution.Pipeline)
	if err != nil {
		return InvocationOutput{}, err
	}
	run := model.PluginRun{ID: kernel.NewID(), UserID: user, IdempotencyKey: key, RequestDigest: digest, ReleaseID: resolved.Release.ID, ReleaseVersion: resolved.Release.Version, AgentRunID: policy.AgentRunID, Operation: resolved.Release.PluginID + "." + resolved.Definition.ID, ContractHash: resolved.ContractHash, RequestJSON: string(raw), PlanJSON: encode(map[string]any{"steps": p.Steps, "notice": "每个副作用步骤仍需单独确认；填写表单不代表授权收费"}), PipelineID: p.ID, Status: "queued", Revision: 1}
	step := model.PluginRunStep{ID: kernel.NewID(), RunID: run.ID, StepKey: "invoke", Attempt: 1, Status: "queued", InputDigest: digest}
	if err = repo.CreatePluginRun(&run, &step); err != nil {
		return InvocationOutput{}, err
	}
	batch := ""
	if batchPipeline(p) {
		batch = encode(newBatchState())
	}
	if err = repo.CreatePluginPipeline(&model.PluginPipelineExecution{RunID: run.ID, OutputsJSON: "{}", BatchJSON: batch}); err != nil {
		return InvocationOutput{}, err
	}
	if err = SaveRunTransition(repo, &run, "pipeline.created"); err != nil {
		return InvocationOutput{}, err
	}
	return invocationRun(run), nil
}

// One bounded, network-free transition per transaction. Catalog locking orders
// revocation/cancel against admission; lease fencing and revision CAS prevent a
// stale scheduler from publishing. The cursor and child admission commit together.
func (s *Service) AdvancePipeline(user, id string) error {
	return s.repo.WithPluginCatalog(func(repo *repository.Repository) error {
		run, err := repo.PluginRunForUser(user, id)
		if err != nil {
			return err
		}
		if run.PipelineID == "" || !contains([]string{"queued", "running", "waiting_input", "waiting_approval", "cancelling"}, run.Status) {
			return nil
		}
		owner := kernel.NewID()
		claimed, err := repo.ClaimPluginPipeline(id, owner)
		if err != nil || !claimed {
			return err
		}
		state, err := repo.PluginPipeline(id)
		if err != nil {
			return err
		}
		local := *s
		local.repo = repo
		before := run.Status
		changed, err := local.advancePipelineStep(repo, run, state)
		if err != nil {
			var appErr *kernel.AppError
			if !errors.As(err, &appErr) {
				return err
			}
			run.Status = "paused"
			run.FailureMessage = appErr.Message
			if state.BatchJSON != "" {
				var batch BatchState
				if e := json.Unmarshal([]byte(state.BatchJSON), &batch); e != nil {
					return e
				}
				batch.StopStatus = "failed"
				batch.BlockedReason = appErr.Message
				state.BatchJSON = encode(batch)
				run.Status = "cancelling"
			}
			changed = true
		}
		if changed || before != run.Status {
			if err = SaveRunTransition(repo, run, "pipeline.updated"); err != nil {
				return err
			}
		}
		return repo.SavePluginPipeline(state, owner)
	})
}

func (s *Service) advancePipelineStep(repo *repository.Repository, run *model.PluginRun, state *model.PluginPipelineExecution) (bool, error) {
	if state.BatchJSON != "" {
		return s.advanceBatch(repo, run, state)
	}
	if state.ChildRunID != "" {
		child, err := repo.PluginRunForUser(run.UserID, state.ChildRunID)
		if err != nil {
			return false, err
		}
		if run.Status == "cancelling" {
			if contains([]string{"succeeded", "failed", "cancelled"}, child.Status) {
				run.Status = "cancelled"
				return true, nil
			}
			if child.Status != "cancelling" {
				if _, err = s.Cancel(run.UserID, child.ID, child.Revision); err != nil {
					return false, err
				}
			}
			return false, nil
		}
		if child.Status != "succeeded" {
			switch child.Status {
			case "cancelled", "failed":
				run.Status = child.Status
				run.FailureMessage = child.FailureMessage
			case "waiting_approval":
				run.Status = "waiting_approval"
			default:
				run.Status = "running"
			}
			return false, nil
		}
	}
	if run.Status == "cancelling" {
		run.Status = "cancelled"
		return true, nil
	}
	var request contracts.Invocation
	if err := json.Unmarshal([]byte(run.RequestJSON), &request); err != nil {
		return false, err
	}
	ctx := contracts.InvocationContext{}
	if request.Context != nil {
		ctx = *request.Context
	}
	// Every new step rechecks current grants and the pinned package; monitoring
	// already-admitted work above remains possible after disable/revocation.
	if s.runAccess != nil {
		if err := s.runAccess(repo, run.UserID); err != nil {
			return false, err
		}
	}
	resolved, err := s.resolve(repo, run.UserID, run.Operation, run.ReleaseID, ctx, InvocationPolicy{PermissionMode: "request_approval"})
	if err != nil {
		return false, err
	}
	p, err := contracts.LoadPipeline(resolved.Files, run.PipelineID)
	if err != nil {
		return false, err
	}
	outputs := map[string]json.RawMessage{}
	if err = json.Unmarshal([]byte(state.OutputsJSON), &outputs); err != nil {
		return false, err
	}
	if state.Cursor < 0 || state.Cursor > len(p.Steps) {
		return false, fmt.Errorf("invalid pipeline cursor")
	}
	if state.ChildRunID != "" {
		child, err := repo.PluginRunForUser(run.UserID, state.ChildRunID)
		if err != nil {
			return false, err
		}
		outputs[p.Steps[state.Cursor].Key] = json.RawMessage(child.ResultJSON)
		if err = savePipelineOutputs(state, outputs); err != nil {
			return false, err
		}
		state.Cursor++
		state.ChildRunID = ""
		run.Status = "running"
		return true, nil
	}
	if state.Cursor == len(p.Steps) {
		result, err := resolveBindings(p.Outputs, request.Input, outputs)
		if err != nil {
			return false, err
		}
		raw := []byte(encode(result))
		if len(raw) > 64<<10 {
			return false, issue(400, "upstream_output_invalid", "流程结果超过上限")
		}
		if err = contracts.ValidateData(resolved.Files, p.OutputSchemaRef, raw); err != nil {
			return false, issue(400, "upstream_output_invalid", "流程结果不符合 Schema")
		}
		run.ResultJSON = string(raw)
		run.Status = "succeeded"
		return true, nil
	}
	step := p.Steps[state.Cursor]
	if step.Type == "wait_input" {
		rows, err := repo.PluginInputs(run.ID)
		if err != nil {
			return false, err
		}
		for _, row := range rows {
			if row.StepKey == step.Key {
				if row.Status == "submitted" {
					outputs[step.Key] = json.RawMessage(row.SubmittedJSON)
					if err = savePipelineOutputs(state, outputs); err != nil {
						return false, err
					}
					state.Cursor++
					run.Status = "running"
					return true, nil
				}
				run.Status = "waiting_input"
				return false, nil
			}
		}
		schema := resolved.Files[step.FormSchemaRef]
		draft, err := pipelineInputDraft(resolved.Files, step, request.Input, outputs)
		if err != nil {
			return false, err
		}
		row := model.PluginInputRequest{ID: kernel.NewID(), RunID: run.ID, StepKey: step.Key, Revision: 1, Status: "pending", SchemaJSON: string(schema)}
		row.DraftJSON = draft
		if err = repo.CreatePluginInput(&row); err != nil {
			return false, err
		}
		run.Status = "waiting_input"
		return true, nil
	}
	input, err := resolveBindings(step.Inputs, request.Input, outputs)
	if err != nil {
		return false, err
	}
	out, err := s.Invoke(run.UserID, "pipeline:"+run.ID+":"+step.Key, contracts.Invocation{Operation: step.Operation, ReleaseID: run.ReleaseID, Input: input, Context: request.Context}, InvocationPolicy{PermissionMode: "request_approval"})
	if err != nil {
		return false, err
	}
	if out.Kind == "run" {
		if err = repo.LinkPluginChild(run.UserID, out.RunID, run.ID, run.AgentRunID, step.Key); err != nil {
			return false, err
		}
		state.ChildRunID = out.RunID
		run.Status = out.Status
	} else {
		// Bounded read adapters return durable values here, never re-read on resume.
		outputs[step.Key] = out.Result
		if err = savePipelineOutputs(state, outputs); err != nil {
			return false, err
		}
		state.Cursor++
		run.Status = "running"
		var value struct {
			ResourceID string `json:"resourceId"`
		}
		if json.Unmarshal(out.Result, &value) == nil && value.ResourceID != "" {
			if _, err = repo.LockResourceForUser(run.UserID, value.ResourceID); err != nil {
				return false, err
			}
			if err = repo.AddPluginRunResources(run.UserID, run.ID, "input", []string{value.ResourceID}); err != nil {
				return false, err
			}
		}
	}
	return true, nil
}

func savePipelineOutputs(state *model.PluginPipelineExecution, outputs map[string]json.RawMessage) error {
	raw, err := json.Marshal(outputs)
	if err != nil {
		return err
	}
	if len(raw) > 256<<10 {
		return issue(400, "upstream_output_invalid", "流程步骤累计结果超过 256KB，请由操作输出摘要和资源引用")
	}
	state.OutputsJSON = string(raw)
	return nil
}

func resolveBindings(bindings map[string]contracts.Binding, input map[string]json.RawMessage, outputs map[string]json.RawMessage) (map[string]json.RawMessage, error) {
	result := map[string]json.RawMessage{}
	for key, b := range bindings {
		if len(b.Literal) > 0 {
			result[key] = b.Literal
			continue
		}
		parts := strings.SplitN(b.From, "#", 2)
		if len(parts) != 2 {
			return nil, issue(400, "operation_input_invalid", "无效流程引用")
		}
		var raw []byte
		if parts[0] == "input" {
			raw = []byte(encode(input))
		} else if parts[0] == "item" {
			raw = outputs["$item"]
		} else {
			raw = outputs[strings.TrimPrefix(parts[0], "steps/")]
		}
		value, err := contracts.Decode(raw)
		if err != nil {
			return nil, issue(400, "operation_input_invalid", "步骤结果尚不可用")
		}
		if parts[1] != "" {
			for _, part := range strings.Split(strings.TrimPrefix(parts[1], "/"), "/") {
				part = strings.ReplaceAll(strings.ReplaceAll(part, "~1", "/"), "~0", "~")
				switch object := value.(type) {
				case map[string]any:
					var ok bool
					value, ok = object[part]
					if !ok {
						return nil, issue(400, "operation_input_invalid", "流程引用字段不存在")
					}
				case []any:
					i, e := strconv.Atoi(part)
					if e != nil || i < 0 || i >= len(object) || strconv.Itoa(i) != part {
						return nil, issue(400, "operation_input_invalid", "流程数组引用无效")
					}
					value = object[i]
				default:
					return nil, issue(400, "operation_input_invalid", "流程引用路径无效")
				}
			}
		}
		result[key] = json.RawMessage(encode(value))
	}
	return result, nil
}

func (s *Service) cancelPipeline(repo *repository.Repository, run *model.PluginRun, revision int64) error {
	if run.Revision != revision || contains([]string{"succeeded", "failed", "cancelled"}, run.Status) {
		return issue(409, "run_revision_conflict", "流程状态已变化")
	}
	state, err := repo.PluginPipeline(run.ID)
	if err != nil {
		return err
	}
	if state.BatchJSON != "" {
		run.Status = "cancelling"
		return SaveRunTransition(repo, run, "pipeline.cancel.requested")
	}
	run.Status = "cancelled"
	if state.ChildRunID != "" {
		child, err := repo.PluginRunForUser(run.UserID, state.ChildRunID)
		if err != nil {
			return err
		}
		if !contains([]string{"succeeded", "failed", "cancelled"}, child.Status) {
			local := *s
			local.repo = repo
			if _, err = local.Cancel(run.UserID, child.ID, child.Revision); err != nil {
				return err
			}
			run.Status = "cancelling"
		}
	}
	return SaveRunTransition(repo, run, "pipeline.cancelled")
}
