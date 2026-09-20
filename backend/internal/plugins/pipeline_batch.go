package plugins

import (
	"encoding/json"
	"errors"
	"slices"
	"strings"

	"infinite-canvas/backend/internal/kernel"
	"infinite-canvas/backend/internal/model"
	"infinite-canvas/backend/internal/plugins/contracts"
	"infinite-canvas/backend/internal/repository"
)

const maxBatchItems = 256

type BatchItem struct {
	Key        string                     `json:"itemKey"`
	Input      map[string]json.RawMessage `json:"input,omitempty"`
	Digest     string                     `json:"digest,omitempty"`
	ChildRunID string                     `json:"childRunId,omitempty"`
	ReusedFrom string                     `json:"reusedFrom,omitempty"`
	Status     string                     `json:"status"`
	Result     json.RawMessage            `json:"result,omitempty"`
}
type BatchStep struct {
	Items []BatchItem `json:"items"`
	Done  bool        `json:"done"`
}
type BatchState struct {
	DerivationDigest string                `json:"derivationDigest,omitempty"`
	Steps            map[string]*BatchStep `json:"steps"`
	StopStatus       string                `json:"stopStatus,omitempty"`
	ReuseCompleted   bool                  `json:"reuseCompleted"`
	ForceSteps       []string              `json:"forceSteps,omitempty"`
	BlockedReason    string                `json:"blockedReason,omitempty"`
}

func newBatchState() BatchState { return BatchState{Steps: map[string]*BatchStep{}} }
func batchPipeline(p contracts.Pipeline) bool {
	for i, step := range p.Steps {
		if len(step.When) > 0 || len(step.Foreach) > 0 || (i > 0 && !slices.Contains(step.DependsOn, p.Steps[i-1].Key)) {
			return true
		}
	}
	return false
}

func bindingValue(from string, input map[string]json.RawMessage, outputs map[string]json.RawMessage) (json.RawMessage, error) {
	v, err := resolveBindings(map[string]contracts.Binding{"value": {From: from}}, input, outputs)
	return v["value"], err
}
func conditionMatches(raw json.RawMessage, input map[string]json.RawMessage, outputs map[string]json.RawMessage) (bool, error) {
	if len(raw) == 0 {
		return true, nil
	}
	var c struct {
		Exists string              `json:"exists"`
		Equals []contracts.Binding `json:"equals"`
	}
	if err := json.Unmarshal(raw, &c); err != nil {
		return false, err
	}
	if c.Exists != "" {
		_, err := bindingValue(c.Exists, input, outputs)
		return err == nil, nil
	}
	if len(c.Equals) != 2 {
		return false, issue(400, "operation_input_invalid", "条件必须有两个比较值")
	}
	v, err := resolveBindings(map[string]contracts.Binding{"a": c.Equals[0], "b": c.Equals[1]}, input, outputs)
	if err != nil {
		return false, err
	}
	a, err := contracts.Decode(v["a"])
	if err != nil {
		return false, err
	}
	b, err := contracts.Decode(v["b"])
	return encode(a) == encode(b), err
}

// Expand the whole step before admitting any item, rejecting duplicate identities
// and missing bindings atomically. Array order is input order, never completion order.
func expandBatchStep(step contracts.PipelineStep, input map[string]json.RawMessage, outputs map[string]json.RawMessage) (*BatchStep, int, error) {
	values := []json.RawMessage{json.RawMessage(`null`)}
	var loop struct {
		From           string `json:"from"`
		ItemKey        string `json:"itemKey"`
		MaxConcurrency int    `json:"maxConcurrency"`
	}
	limit := 1
	if len(step.Foreach) > 0 {
		if err := json.Unmarshal(step.Foreach, &loop); err != nil {
			return nil, 0, err
		}
		raw, err := bindingValue(loop.From, input, outputs)
		if err != nil {
			return nil, 0, err
		}
		if len(raw) == 0 || raw[0] != '[' || json.Unmarshal(raw, &values) != nil || len(values) > maxBatchItems {
			return nil, 0, issue(400, "operation_input_invalid", "foreach 必须引用不超过 256 项的数组")
		}
		limit = loop.MaxConcurrency
	}
	row := &BatchStep{Items: []BatchItem{}}
	seen := map[string]bool{}
	for _, v := range values {
		bound := map[string]json.RawMessage{}
		for k, value := range outputs {
			bound[k] = value
		}
		bound["$item"] = v
		item := BatchItem{Key: "", Status: "pending"}
		if loop.From != "" {
			raw, err := bindingValue("item#"+loop.ItemKey, input, bound)
			if err != nil || json.Unmarshal(raw, &item.Key) != nil || strings.TrimSpace(item.Key) != item.Key || item.Key == "" || len(item.Key) > 120 || seen[item.Key] {
				return nil, 0, issue(400, "operation_input_invalid", "itemKey 必须是唯一、非空且不超过 120 字节的稳定字符串")
			}
			seen[item.Key] = true
		}
		matches, err := conditionMatches(step.When, input, bound)
		if err != nil {
			return nil, 0, err
		}
		if !matches {
			item.Status = "skipped"
			item.Result = json.RawMessage(`null`)
		} else {
			item.Input, err = resolveBindings(step.Inputs, input, bound)
			if err != nil {
				return nil, 0, err
			}
			item.Digest = hashBytes([]byte(encode(item.Input)))
		}
		row.Items = append(row.Items, item)
	}
	return row, limit, nil
}

func (s *Service) advanceBatch(repo *repository.Repository, run *model.PluginRun, state *model.PluginPipelineExecution) (changed bool, err error) {
	b := newBatchState()
	if err = json.Unmarshal([]byte(state.BatchJSON), &b); err != nil {
		return false, err
	}
	defer func() { state.BatchJSON = encode(b) }()
	outputs := map[string]json.RawMessage{}
	if err = json.Unmarshal([]byte(state.OutputsJSON), &outputs); err != nil {
		return false, err
	}
	children, err := repo.PluginPipelineChildren(run.UserID, run.ID)
	if err != nil {
		return false, err
	}
	byID := map[string]model.PluginRun{}
	active := 0
	if run.Status == "cancelling" && b.StopStatus == "" {
		b.StopStatus = "cancelled"
	}
	for _, child := range children {
		byID[child.ID] = child
		if b.StopStatus == "" && (child.Status == "failed" || child.Status == "cancelled") {
			b.StopStatus = "failed"
			run.FailureMessage = child.FailureMessage
		}
		if !contains([]string{"succeeded", "failed", "cancelled"}, child.Status) {
			active++
		}
	}
	for _, step := range b.Steps {
		for i := range step.Items {
			item := &step.Items[i]
			if child, ok := byID[item.ChildRunID]; ok {
				if item.Status != child.Status {
					changed = true
				}
				item.Status = child.Status
				if child.Status == "succeeded" {
					item.Result = json.RawMessage(child.ResultJSON)
				}
			}
		}
	}
	if run.Status == "cancelling" && b.StopStatus == "" {
		b.StopStatus = "cancelled"
	}
	if b.StopStatus != "" {
		for _, child := range children {
			if !contains([]string{"succeeded", "failed", "cancelled", "cancelling"}, child.Status) {
				if _, err = s.Cancel(run.UserID, child.ID, child.Revision); err != nil {
					return false, err
				}
			}
		}
		if active == 0 {
			run.Status = b.StopStatus
			for _, step := range b.Steps {
				for i := range step.Items {
					if step.Items[i].Status == "pending" {
						step.Items[i].Status = "cancelled"
					}
				}
			}
		} else {
			run.Status = "cancelling"
		}
		return true, nil
	}
	var request contracts.Invocation
	if err = json.Unmarshal([]byte(run.RequestJSON), &request); err != nil {
		return false, err
	}
	ctx := contracts.InvocationContext{}
	if request.Context != nil {
		ctx = *request.Context
	}
	// Recheck admission after monitoring already-admitted children above.
	if s.runAccess != nil {
		err = s.runAccess(repo, run.UserID)
	}
	var resolved resolvedOperation
	if err == nil {
		resolved, err = s.resolve(repo, run.UserID, run.Operation, run.ReleaseID, ctx, InvocationPolicy{PermissionMode: "request_approval"})
	}
	if err != nil {
		var appErr *kernel.AppError
		if !errors.As(err, &appErr) {
			return false, err
		}
		b.BlockedReason = "权限、账号或插件版本不可用，停止新步骤"
		run.FailureMessage = b.BlockedReason
		if active > 0 {
			run.Status = "running"
		} else {
			run.Status = "paused"
		}
		return true, nil
	}
	b.BlockedReason = ""
	p, err := contracts.LoadPipeline(resolved.Files, run.PipelineID)
	if err != nil {
		return false, err
	}
	inputs, err := repo.PluginInputs(run.ID)
	if err != nil {
		return false, err
	}
	admitted := 0
	waitingInput := false
	waitingApproval := false
	for _, step := range p.Steps {
		if row := b.Steps[step.Key]; row != nil && row.Done {
			continue
		}
		ready := true
		for _, dep := range step.DependsOn {
			if b.Steps[dep] == nil || !b.Steps[dep].Done {
				ready = false
			}
		}
		if !ready {
			continue
		}
		if step.Type == "wait_input" {
			var value json.RawMessage
			found := false
			for _, row := range inputs {
				if row.StepKey == step.Key {
					found = true
					if row.Status == "submitted" {
						value = json.RawMessage(row.SubmittedJSON)
					}
				}
			}
			if len(value) == 0 {
				if !found {
					row := model.PluginInputRequest{ID: kernel.NewID(), RunID: run.ID, StepKey: step.Key, Revision: 1, Status: "pending", SchemaJSON: string(resolved.Files[step.FormSchemaRef])}
					if err = repo.CreatePluginInput(&row); err != nil {
						return false, err
					}
					changed = true
				}
				waitingInput = true
				continue
			}
			outputs[step.Key] = value
			b.Steps[step.Key] = &BatchStep{Done: true, Items: []BatchItem{{Status: "succeeded", Result: value}}}
			changed = true
			continue
		}
		limit := 1
		if len(step.Foreach) > 0 {
			var loop struct {
				MaxConcurrency int `json:"maxConcurrency"`
			}
			if err = json.Unmarshal(step.Foreach, &loop); err != nil {
				return false, err
			}
			limit = loop.MaxConcurrency
		}
		row := b.Steps[step.Key]
		if row == nil {
			row, limit, err = expandBatchStep(step, request.Input, outputs)
			if err != nil {
				return false, err
			}
			total := len(row.Items)
			for _, other := range b.Steps {
				total += len(other.Items)
			}
			if total > maxBatchItems {
				return false, issue(400, "operation_input_invalid", "流程累计展开超过 256 项")
			}
			if len(encode(row))+len(encode(b))+len(step.Key)+4 > 1<<20 {
				return false, issue(400, "operation_input_invalid", "批次展开状态超过 1MB，请使用资源引用并缩小每项输入")
			}
			b.Steps[step.Key] = row
			changed = true
		}
		inflight := 0
		for _, item := range row.Items {
			if item.ChildRunID != "" && !contains([]string{"succeeded", "skipped"}, item.Status) {
				inflight++
			}
		}
		for i := range row.Items {
			item := &row.Items[i]
			if item.Status == "waiting_approval" {
				waitingApproval = true
			}
			if item.Status != "pending" || inflight >= limit || active >= 8 || admitted >= 8 {
				continue
			}
			if reused, e := s.reuseBatchItem(repo, run, b, step, item, request.Context); e != nil {
				return false, e
			} else if reused {
				changed = true
				continue
			}
			key := "pipeline:" + run.ID + ":" + hashBytes([]byte(step.Key+"\x00"+item.Key))
			out, e := s.Invoke(run.UserID, key, contracts.Invocation{Operation: step.Operation, ReleaseID: run.ReleaseID, Input: item.Input, Context: request.Context}, InvocationPolicy{PermissionMode: "request_approval"})
			if e != nil {
				return false, e
			}
			admitted++
			changed = true
			if out.Kind == "run" {
				item.ChildRunID = out.RunID
				item.Status = out.Status
				if err = repo.LinkPluginChild(run.UserID, out.RunID, run.ID, run.AgentRunID, step.Key); err != nil {
					return false, err
				}
				inflight++
				active++
				waitingApproval = waitingApproval || out.Status == "waiting_approval"
			} else {
				item.Status = "succeeded"
				item.Result = out.Result
				var value struct {
					ResourceID string `json:"resourceId"`
				}
				if json.Unmarshal(out.Result, &value) == nil && value.ResourceID != "" {
					if err = repo.AddPluginRunResources(run.UserID, run.ID, "input", []string{value.ResourceID}); err != nil {
						return false, err
					}
				}
			}
		}
		done := true
		results := []json.RawMessage{}
		for _, item := range row.Items {
			if !contains([]string{"succeeded", "skipped"}, item.Status) {
				done = false
			}
			results = append(results, item.Result)
		}
		if done {
			row.Done = true
			if len(step.Foreach) > 0 {
				outputs[step.Key] = json.RawMessage(encode(results))
			} else {
				outputs[step.Key] = results[0]
			}
			changed = true
		}
	}
	state.Cursor = 0
	for _, row := range b.Steps {
		if row.Done {
			state.Cursor++
		}
	}
	if err = savePipelineOutputs(state, outputs); err != nil {
		return false, err
	}
	if len(encode(b)) > 1<<20 {
		return false, issue(400, "operation_input_invalid", "批次状态超过 1MB，请缩小输入或返回资源引用")
	}
	run.Status = "running"
	if waitingApproval {
		run.Status = "waiting_approval"
	} else if waitingInput && active == 0 {
		run.Status = "waiting_input"
	}
	if state.Cursor == len(p.Steps) {
		result, e := resolveBindings(p.Outputs, request.Input, outputs)
		if e != nil {
			return false, e
		}
		raw := []byte(encode(result))
		if len(raw) > 64<<10 || contracts.ValidateData(resolved.Files, p.OutputSchemaRef, raw) != nil {
			return false, issue(400, "upstream_output_invalid", "流程结果超过限制或不符合 Schema")
		}
		run.Status = "succeeded"
		run.ResultJSON = string(raw)
		changed = true
	}
	return changed, nil
}
