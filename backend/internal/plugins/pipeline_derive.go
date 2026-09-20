package plugins

import (
	"encoding/json"
	"errors"
	"gorm.io/gorm"
	"slices"

	"infinite-canvas/backend/internal/kernel"
	"infinite-canvas/backend/internal/model"
	"infinite-canvas/backend/internal/plugins/contracts"
	"infinite-canvas/backend/internal/repository"
)

type DeriveRequest struct {
	Input          map[string]json.RawMessage `json:"input"`
	Inputs         map[string]json.RawMessage `json:"inputs"`
	ForceSteps     []string                   `json:"forceSteps"`
	ReuseCompleted bool                       `json:"reuseCompleted"`
}

func affectedSteps(p contracts.Pipeline, force []string) ([]string, error) {
	known := map[string]bool{}
	for _, step := range p.Steps {
		known[step.Key] = true
	}
	for _, key := range force {
		if !known[key] {
			return nil, issue(400, "operation_input_invalid", "重做步骤不存在")
		}
	}
	result := slices.Clone(force)
	for changed := true; changed; {
		changed = false
		for _, step := range p.Steps {
			if slices.Contains(result, step.Key) {
				continue
			}
			for _, dep := range step.DependsOn {
				if slices.Contains(result, dep) {
					result = append(result, step.Key)
					changed = true
					break
				}
			}
		}
	}
	return result, nil
}
func (s *Service) Derive(user, id, key string, req DeriveRequest) (RunView, error) {
	var resultID string
	err := s.repo.WithPluginCatalog(func(repo *repository.Repository) error {
		source, err := repo.PluginRunForUser(user, id)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return kernel.NotFound("插件运行不存在")
		}
		if err != nil {
			return err
		}
		if source.PipelineID == "" || !contains([]string{"succeeded", "failed", "cancelled"}, source.Status) {
			return issue(409, "run_revision_conflict", "请先完成或停止原流程；未知提交需单独核实，不能通过派生绕过")
		}
		if source.Attempt >= 10000 {
			return issue(409, "run_revision_conflict", "派生次数已达到合同上限")
		}
		children, err := repo.PluginPipelineChildren(user, id)
		if err != nil {
			return err
		}
		for _, child := range children {
			if !contains([]string{"succeeded", "failed", "cancelled"}, child.Status) {
				return issue(409, "run_revision_conflict", "仍有未完成子任务")
			}
			if child.TaskID != nil && child.ConnectionVersionID != "" {
				remote, e := repo.PluginRemoteForRun(user, child.ID)
				if e != nil {
					return e
				}
				if remote.CancelStatus == "unsupported_external_may_continue" {
					return issue(409, "run_revision_conflict", "原外部任务仍可能执行，不能自动派生")
				}
			}
		}
		var original contracts.Invocation
		if err = json.Unmarshal([]byte(source.RequestJSON), &original); err != nil {
			return err
		}
		ctx := contracts.InvocationContext{}
		if original.Context != nil {
			ctx = *original.Context
		}
		resolved, err := s.resolve(repo, user, source.Operation, source.ReleaseID, ctx, InvocationPolicy{PermissionMode: "request_approval"})
		if err != nil {
			return err
		}
		p, err := contracts.LoadPipeline(resolved.Files, source.PipelineID)
		if err != nil {
			return err
		}
		force, err := affectedSteps(p, req.ForceSteps)
		if err != nil {
			return err
		}
		for name, value := range req.Inputs {
			schema := ""
			for _, step := range p.Steps {
				if step.Key == name && step.Type == "wait_input" {
					schema = step.FormSchemaRef
				}
			}
			if schema == "" || contracts.ValidateData(resolved.Files, schema, value) != nil {
				return issue(400, "operation_input_invalid", "派生输入不符合表单 Schema")
			}
		}
		if req.Input != nil {
			if original.Workbench != nil && encode(req.Input) != encode(original.Input) {
				return issue(409, "run_revision_conflict", "工作台顶层输入已固定；修改后请回到工作台重新预览并创建运行")
			}
			original.Input = req.Input
		}
		local := *s
		local.repo = repo
		// Include derivation policy in the idempotency namespace. A repeated key
		// with changed policy is rejected rather than silently reusing a new run.
		existing, err := repo.PluginRunByKey(user, key)
		if err != nil {
			return err
		}
		spec := hashBytes([]byte(encode(struct {
			Source  string
			Request DeriveRequest
		}{id, req})))
		if existing != nil {
			state, e := repo.PluginPipeline(existing.ID)
			if e != nil {
				return e
			}
			var b BatchState
			if json.Unmarshal([]byte(state.BatchJSON), &b) != nil || b.DerivationDigest != spec {
				return issue(409, "run_revision_conflict", "派生幂等键已用于不同请求")
			}
			resultID = existing.ID
			return nil
		}
		out, err := local.Invoke(user, key, original, InvocationPolicy{PermissionMode: "request_approval"})
		if err != nil {
			return err
		}
		resultID = out.RunID
		b := newBatchState()
		b.ReuseCompleted = req.ReuseCompleted
		b.ForceSteps = force
		b.DerivationDigest = spec
		if err = repo.SetPluginDerivation(out.RunID, id, source.Attempt+1, encode(b)); err != nil {
			return err
		}
		rows, err := repo.PluginInputs(id)
		if err != nil {
			return err
		}
		for _, step := range p.Steps {
			if step.Type != "wait_input" {
				continue
			}
			var value json.RawMessage
			if !slices.Contains(force, step.Key) {
				for _, old := range rows {
					if old.StepKey == step.Key && old.Status == "submitted" {
						value = json.RawMessage(old.SubmittedJSON)
					}
				}
			}
			if replacement, ok := req.Inputs[step.Key]; ok {
				value = replacement
			}
			if len(value) == 0 {
				continue
			}
			if err = s.validateInput(repo, user, out.RunID, step, original, value); err != nil {
				return err
			}
			row := model.PluginInputRequest{ID: kernel.NewID(), RunID: out.RunID, StepKey: step.Key, Revision: 1, Status: "submitted", SchemaJSON: string(resolved.Files[step.FormSchemaRef]), SubmittedJSON: string(value)}
			if err = repo.CreatePluginInput(&row); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return RunView{}, err
	}
	return s.GetRun(user, resultID)
}

func (s *Service) reuseBatchItem(repo *repository.Repository, run *model.PluginRun, b BatchState, step contracts.PipelineStep, item *BatchItem, ctx *contracts.InvocationContext) (bool, error) {
	if !b.ReuseCompleted || run.DerivedFromRunID == "" || slices.Contains(b.ForceSteps, step.Key) {
		return false, nil
	}
	source, err := repo.PluginRunForUser(run.UserID, run.DerivedFromRunID)
	if err != nil {
		return false, err
	}
	state, err := repo.PluginPipeline(source.ID)
	if err != nil {
		return false, err
	}
	var old BatchItem
	if state.BatchJSON == "" {
		children, e := repo.PluginPipelineChildren(run.UserID, source.ID)
		if e != nil {
			return false, e
		}
		for _, candidate := range children {
			if candidate.ParentStepKey == step.Key {
				var request contracts.Invocation
				if e = json.Unmarshal([]byte(candidate.RequestJSON), &request); e != nil {
					return false, e
				}
				old = BatchItem{ChildRunID: candidate.ID, Digest: hashBytes([]byte(encode(request.Input)))}
			}
		}
	}
	if state.BatchJSON != "" {
		var previous BatchState
		if err = json.Unmarshal([]byte(state.BatchJSON), &previous); err != nil {
			return false, err
		}
		if row := previous.Steps[step.Key]; row != nil {
			for _, candidate := range row.Items {
				if candidate.Key == item.Key {
					old = candidate
				}
			}
		}
	}
	if old.ChildRunID == "" {
		old.ChildRunID = old.ReusedFrom
		if old.ChildRunID == "" {
			return false, nil
		}
	}
	child, err := repo.PluginRunForUser(run.UserID, old.ChildRunID)
	if err != nil {
		return false, err
	}
	if child.Status != "succeeded" || child.ReleaseID != run.ReleaseID || child.Operation != step.Operation || old.Digest != item.Digest {
		return false, nil
	}
	context := contracts.InvocationContext{}
	if ctx != nil {
		context = *ctx
	}
	resolved, err := s.resolve(repo, run.UserID, step.Operation, run.ReleaseID, context, InvocationPolicy{PermissionMode: "request_approval"})
	if err != nil {
		return false, err
	}
	if resolved.ContractHash != child.ContractHash || (resolved.Definition.Execution.Kind != "http" && resolved.Definition.Execution.Kind != "model") {
		return false, nil
	}
	prepared, err := s.taskHost(resolved.Definition.Execution.Kind).Prepare(repo, run.UserID, contracts.Invocation{Operation: step.Operation, ReleaseID: run.ReleaseID, Input: item.Input, Context: ctx}, HostOperationContext{InvocationContext: context, ReleaseID: run.ReleaseID, Files: resolved.Files}, InvocationPolicy{PermissionMode: "request_approval"})
	if err != nil {
		return false, err
	}
	if prepared.SourceDigest != child.SourceDigest || prepared.ConnectionVersionID != child.ConnectionVersionID {
		return false, nil
	}
	if err = contracts.ValidateData(resolved.Files, resolved.Definition.OutputSchemaRef, []byte(child.ResultJSON)); err != nil {
		return false, err
	}
	if err = repo.CopyPluginResultResources(run.UserID, child.ID, run.ID); err != nil {
		return false, err
	}
	item.Status = "succeeded"
	item.Result = json.RawMessage(child.ResultJSON)
	item.ReusedFrom = child.ID
	return true, nil
}
