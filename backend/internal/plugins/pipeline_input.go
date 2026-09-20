package plugins

import (
	"encoding/json"
	"errors"
	"strings"
	"unicode/utf8"

	"gorm.io/gorm"
	"infinite-canvas/backend/internal/kernel"
	"infinite-canvas/backend/internal/model"
	"infinite-canvas/backend/internal/plugins/contracts"
	"infinite-canvas/backend/internal/repository"
)

type InputView struct {
	model.PluginInputRequest
	Schema    json.RawMessage `json:"schema"`
	Draft     json.RawMessage `json:"draft,omitempty"`
	Submitted json.RawMessage `json:"submitted,omitempty"`
}
type PipelineView struct {
	Batch      *BatchState                 `json:"batch,omitempty"`
	Approvals  []model.PluginBatchApproval `json:"approvals,omitempty"`
	Cursor     int                         `json:"cursor"`
	Steps      []contracts.PipelineStep    `json:"steps"`
	ChildRunID string                      `json:"childRunId,omitempty"`
	Inputs     []InputView                 `json:"inputs"`
	Outputs    map[string]json.RawMessage  `json:"outputs"`
	StepRuns   []model.PluginRun           `json:"stepRuns"`
	Schemas    map[string]json.RawMessage  `json:"schemas"`
}

func (s *Service) pipelineView(run *model.PluginRun) (*PipelineView, error) {
	state, err := s.repo.PluginPipeline(run.ID)
	if err != nil {
		return nil, err
	}
	release, err := s.repo.PluginRelease(run.ReleaseID)
	if err != nil {
		return nil, err
	}
	files, err := s.loadPackage(*release)
	if err != nil {
		return nil, err
	}
	p, err := contracts.LoadPipeline(files, run.PipelineID)
	if err != nil {
		return nil, err
	}
	view := &PipelineView{Cursor: state.Cursor, Steps: p.Steps, ChildRunID: state.ChildRunID, Inputs: []InputView{}}
	if state.BatchJSON != "" {
		view.Batch = &BatchState{}
		if err = json.Unmarshal([]byte(state.BatchJSON), view.Batch); err != nil {
			return nil, err
		}
	}
	view.Approvals, err = s.repo.PluginBatchApprovals(run.UserID, run.ID)
	if err != nil {
		return nil, err
	}
	view.Schemas = map[string]json.RawMessage{}
	for path, raw := range files {
		if strings.HasPrefix(path, "schemas/") {
			view.Schemas[path] = json.RawMessage(raw)
		}
	}
	view.StepRuns, err = s.repo.PluginPipelineChildren(run.UserID, run.ID)
	if err != nil {
		return nil, err
	}
	if err = json.Unmarshal([]byte(state.OutputsJSON), &view.Outputs); err != nil {
		return nil, err
	}
	rows, err := s.repo.PluginInputs(run.ID)
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		view.Inputs = append(view.Inputs, InputView{PluginInputRequest: row, Schema: json.RawMessage(row.SchemaJSON), Draft: json.RawMessage(row.DraftJSON), Submitted: json.RawMessage(row.SubmittedJSON)})
	}
	return view, nil
}

type InputUpdate struct {
	Revision int64           `json:"revision"`
	Mode     string          `json:"mode"`
	Value    json.RawMessage `json:"value"`
}

func (s *Service) UpdateInput(user, runID, id, key string, req InputUpdate) (RunView, error) {
	if req.Mode != "draft" && req.Mode != "submit" {
		return RunView{}, issue(400, "operation_input_invalid", "输入操作必须是 draft 或 submit")
	}
	if len(req.Value) > 64<<10 {
		return RunView{}, issue(400, "operation_input_invalid", "输入超过 64KB")
	}
	value, err := contracts.Decode(req.Value)
	if err != nil {
		return RunView{}, issue(400, "operation_input_invalid", "输入 JSON 无效")
	}
	canonical, err := json.Marshal(value)
	if err != nil {
		return RunView{}, err
	}
	if req.Mode == "submit" && (len(key) < 8 || len(key) > 160 || !utf8.ValidString(key) || strings.TrimSpace(key) != key) {
		return RunView{}, issue(400, "operation_input_invalid", "提交需要 8–160 字节幂等键")
	}
	err = s.repo.WithPluginCatalog(func(repo *repository.Repository) error {
		run, err := repo.PluginRunForUser(user, runID)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return kernel.NotFound("插件运行不存在")
		}
		if err != nil {
			return err
		}
		row, err := repo.PluginInput(runID, id)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return kernel.NotFound("输入请求不存在")
		}
		if err != nil {
			return err
		}
		if req.Mode == "submit" && row.SubmissionKey == key {
			if row.SubmissionDigest != hashBytes(canonical) {
				return issue(409, "run_revision_conflict", "同一幂等键不能提交不同内容")
			}
			return nil
		}
		if row.Status != "pending" || row.Revision != req.Revision || !contains([]string{"waiting_input", "running", "waiting_approval"}, run.Status) {
			return issue(409, "run_revision_conflict", "输入已提交、运行已停止或页面已过期")
		}
		var invocation contracts.Invocation
		if err = json.Unmarshal([]byte(run.RequestJSON), &invocation); err != nil {
			return err
		}
		ctx := contracts.InvocationContext{}
		if invocation.Context != nil {
			ctx = *invocation.Context
		}
		resolved, err := s.resolve(repo, user, run.Operation, run.ReleaseID, ctx, InvocationPolicy{PermissionMode: "request_approval"})
		if err != nil {
			return err
		}
		if req.Mode == "submit" {
			p, err := contracts.LoadPipeline(resolved.Files, run.PipelineID)
			if err != nil {
				return err
			}
			schema := ""
			for _, step := range p.Steps {
				if step.Key == row.StepKey && step.Type == "wait_input" {
					schema = step.FormSchemaRef
				}
			}
			if err = contracts.ValidateData(resolved.Files, schema, canonical); err != nil {
				return issue(400, "operation_input_invalid", "请按输入 Schema 补全必填字段，流程仍在等待")
			}
			row.Status = "submitted"
			row.SubmittedJSON = string(canonical)
			row.SubmissionKey = key
			row.SubmissionDigest = hashBytes(canonical)
			run.Status = "running"
		} else {
			row.DraftJSON = string(canonical)
		}
		row.Revision++
		if err = repo.SavePluginInput(row, req.Revision); err != nil {
			return err
		}
		return SaveRunTransition(repo, run, "input."+req.Mode)
	})
	if err != nil {
		return RunView{}, err
	}
	return s.GetRun(user, runID)
}
