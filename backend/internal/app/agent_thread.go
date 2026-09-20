package app

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"gorm.io/gorm"
	"infinite-canvas/backend/internal/kernel"
	"infinite-canvas/backend/internal/model"
	"infinite-canvas/backend/internal/repository"
)

type AgentThreadCreate struct {
	ClientKey string `json:"clientKey"`
	Title     string `json:"title"`
	CanvasID  string `json:"canvasId"`
}
type AgentThreadBinding struct {
	Revision int64  `json:"revision"`
	CanvasID string `json:"canvasId"`
}
type AgentThreadMessage struct {
	Revision int64             `json:"revision"`
	Request  CloudAgentRequest `json:"request"`
}
type AgentThreadReference struct {
	Revision  int64  `json:"revision"`
	ClientKey string `json:"clientKey"`
	Kind      string `json:"kind"`
	RunID     string `json:"runId"`
}
type AgentThreadReceipt struct {
	Kind   string `json:"kind"`
	RunID  string `json:"runId"`
	Status string `json:"status"`
}
type AgentThreadEntryView struct {
	model.AgentThreadEntry
	Context    *CloudAgentRequest   `json:"context,omitempty"`
	Run        *CloudAgentRun       `json:"run,omitempty"`
	References []AgentThreadReceipt `json:"references"`
}
type AgentThreadView struct {
	Thread     *model.AgentThread        `json:"thread"`
	Entries    []AgentThreadEntryView    `json:"entries"`
	Canvases   []model.AgentThreadCanvas `json:"canvases"`
	NextBefore int64                     `json:"nextBefore,omitempty"`
}
type AgentThreadMessageResult struct {
	Thread *model.AgentThread `json:"thread"`
	Run    *CloudAgentRun     `json:"run"`
}

func threadError(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return kernel.NotFound("会话或引用不存在")
	}
	if errors.Is(err, repository.ErrAgentThreadConflict) {
		return creationConflict("会话已更新，请重新读取后提交")
	}
	return err
}
func threadHash(v any) string {
	b, _ := json.Marshal(v)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
func (s *Service) validateThreadCanvas(user, id string) error {
	if id == "" {
		return nil
	}
	if err := validateCloudAgentID(id, "画布 ID", 80); err != nil {
		return err
	}
	_, err := s.repo.CanvasProjectForUser(user, id)
	return threadError(err)
}
func (s *Service) CreateAgentThread(user string, req AgentThreadCreate) (*model.AgentThread, error) {
	if user == "" {
		return nil, kernel.Unauthorized("请先登录")
	}
	if err := validateCloudAgentID(req.ClientKey, "幂等键", 128); err != nil || utf8.RuneCountInString(req.ClientKey) < 8 {
		return nil, BadAuthRequest("需要 8–128 个字符的幂等键")
	}
	req.Title = strings.TrimSpace(req.Title)
	if !utf8.ValidString(req.Title) || utf8.RuneCountInString(req.Title) > 80 || strings.ContainsRune(req.Title, 0) {
		return nil, BadAuthRequest("会话标题无效")
	}
	if existing, err := s.repo.AgentThreadByKey(user, req.ClientKey); err == nil {
		if existing.RequestHash != threadHash(req) {
			return nil, creationConflict("幂等键已用于不同会话")
		}
		return existing, nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	if err := s.validateThreadCanvas(user, req.CanvasID); err != nil {
		return nil, err
	}
	now := time.Now()
	row := &model.AgentThread{ID: "thread-" + threadHash([]string{user, req.ClientKey})[:48], UserID: user, ClientKey: req.ClientKey, Title: req.Title, CanvasID: req.CanvasID, Revision: 1, ContextRevision: 1, CreatedAt: now, UpdatedAt: now}
	if row.Title == "" {
		row.Title = "新对话"
	}
	row.RequestHash = threadHash(req)
	err := s.repo.WithTransaction(func(repo *repository.Repository) error {
		if err := repo.CreateAgentThread(row); err != nil {
			return err
		}
		if row.CanvasID != "" {
			return repo.BindAgentThreadCanvas(&model.AgentThreadCanvas{ThreadID: row.ID, CanvasID: row.CanvasID, UserID: user, CreatedAt: now})
		}
		return nil
	})
	if err != nil {
		if existing, e := s.repo.AgentThreadByKey(user, req.ClientKey); e == nil {
			if existing.RequestHash != row.RequestHash {
				return nil, creationConflict("幂等键已用于不同会话")
			}
			return existing, nil
		}
		return nil, err
	}
	return row, nil
}
func (s *Service) ListAgentThreads(user, canvas string, offset int) ([]model.AgentThread, error) {
	if user == "" {
		return nil, kernel.Unauthorized("请先登录")
	}
	if offset < 0 || offset > 100000 {
		return nil, BadAuthRequest("会话分页无效")
	}
	return s.repo.AgentThreads(user, canvas, offset)
}
func (s *Service) BindAgentThread(user, id string, req AgentThreadBinding) (*model.AgentThread, error) {
	if err := s.validateThreadCanvas(user, req.CanvasID); err != nil {
		return nil, err
	}
	var result *model.AgentThread
	err := s.repo.WithAgentThread(user, id, func(row *model.AgentThread, repo *repository.Repository) error {
		if row.Revision != req.Revision {
			return repository.ErrAgentThreadConflict
		}
		result = row
		if row.CanvasID == req.CanvasID {
			return nil
		}
		row.CanvasID = req.CanvasID
		row.Revision++
		row.ContextRevision++
		row.UpdatedAt = time.Now()
		if req.CanvasID != "" {
			if err := repo.BindAgentThreadCanvas(&model.AgentThreadCanvas{ThreadID: id, CanvasID: req.CanvasID, UserID: user, CreatedAt: row.UpdatedAt}); err != nil {
				return err
			}
		}
		return repo.SaveAgentThread(row, req.Revision)
	})
	return result, threadError(err)
}

// The thread lock, task admission, fee reservation and receipt commit together.
// Only this entry can set ThreadID or select a cross-canvas continuation parent.
func (s *Service) AppendAgentThreadMessage(user, id string, req AgentThreadMessage) (*AgentThreadMessageResult, error) {
	// Preserve the existing process-local quota critical section while admission
	// uses a transaction-scoped repository. The scoped service has its own lock.
	s.storageMu.Lock()
	defer s.storageMu.Unlock()
	if req.Request.ThreadID != "" {
		return nil, BadAuthRequest("threadId 由会话入口确定")
	}
	if err := validateCloudAgentID(req.Request.IdempotencyKey, "幂等键", 128); err != nil || utf8.RuneCountInString(req.Request.IdempotencyKey) < 8 {
		return nil, BadAuthRequest("需要 8–128 个字符的幂等键")
	}
	hash := threadHash(req)
	var result *AgentThreadMessageResult
	err := s.repo.WithAgentThread(user, id, func(row *model.AgentThread, repo *repository.Repository) error {
		// Transaction-scoped dependencies must not reuse repositories or mutexes from
		// the outer service. Worker drain state remains shared; admission never runs a worker.
		local := &Service{repo: repo, dataDir: s.dataDir, workers: s.backgroundWorkers(), runtimeErr: s.runtimeErr, pluginRuntimeErr: s.pluginRuntimeErr}
		if entry, e := repo.AgentThreadEntryByKey(id, req.Request.IdempotencyKey); e == nil {
			if entry.Kind != "agent" || entry.RequestHash != hash {
				return creationConflict("幂等键已用于不同消息")
			}
			run, e := local.CloudAgentRun(user, entry.RunID)
			result = &AgentThreadMessageResult{Thread: row, Run: run}
			return e
		} else if !errors.Is(e, gorm.ErrRecordNotFound) {
			return e
		}
		if row.Revision != req.Revision {
			return repository.ErrAgentThreadConflict
		}
		if row.CanvasID != req.Request.CanvasID {
			return creationConflict("会话画布已变化，请重新读取上下文")
		}
		if row.LastAgentRunID != "" {
			previous, e := local.CloudAgentRun(user, row.LastAgentRunID)
			if e != nil {
				return e
			}
			if !cloudAgentRunTerminal(previous.Status) || previous.CleanupPending {
				return creationConflict("上一轮仍在执行，请等待结束")
			}
		}
		input := req.Request
		input.ThreadID = id
		// Namespace keys so a legacy task cannot be accidentally adopted by a thread.
		input.IdempotencyKey = "thread-turn-" + threadHash([]string{id, req.Request.IdempotencyKey})
		run, e := local.createCloudAgentRun(user, input, row.LastAgentRunID)
		if e != nil {
			return e
		}
		_, frozen, e := local.cloudAgentTask(user, run.ID)
		if e != nil {
			return e
		}
		context, e := json.Marshal(frozen.Request)
		if e != nil {
			return e
		}
		now := time.Now()
		entry := &model.AgentThreadEntry{ThreadID: id, UserID: user, Sequence: row.LastSequence + 1, ClientKey: req.Request.IdempotencyKey, RequestHash: hash, Kind: "agent", RunID: run.ID, Prompt: input.Prompt, ContextRevision: row.ContextRevision, ContextJSON: string(context), CreatedAt: now}
		if e = repo.CreateAgentThreadEntry(entry); e != nil {
			return e
		}
		row.LastSequence = entry.Sequence
		row.LastAgentRunID = run.ID
		row.Revision++
		row.UpdatedAt = now
		if row.Title == "新对话" {
			runes := []rune(strings.TrimSpace(input.Prompt))
			row.Title = string(runes[:min(len(runes), 60)])
		}
		if e = repo.SaveAgentThread(row, req.Revision); e != nil {
			return e
		}
		result = &AgentThreadMessageResult{Thread: row, Run: run}
		return nil
	})
	return result, threadError(err)
}

// References point to owned facts only. They never submit or replay execution.
func (s *Service) AppendAgentThreadReference(user, id string, req AgentThreadReference) (*model.AgentThread, error) {
	if err := validateCloudAgentID(req.ClientKey, "幂等键", 128); err != nil {
		return nil, err
	}
	if err := validateCloudAgentID(req.RunID, "运行 ID", 80); err != nil {
		return nil, err
	}
	var result *model.AgentThread
	err := s.repo.WithAgentThread(user, id, func(row *model.AgentThread, repo *repository.Repository) error {
		result = row
		hash := threadHash([]string{req.Kind, req.RunID})
		if entry, e := repo.AgentThreadEntryByKey(id, req.ClientKey); e == nil {
			if entry.RequestHash != hash {
				return creationConflict("幂等键已用于不同引用")
			}
			return nil
		} else if !errors.Is(e, gorm.ErrRecordNotFound) {
			return e
		}
		if row.Revision != req.Revision {
			return repository.ErrAgentThreadConflict
		}
		local := &Service{repo: repo, dataDir: s.dataDir}
		if _, e := local.threadReceipt(user, req.Kind, req.RunID); e != nil {
			return e
		}
		now := time.Now()
		entry := &model.AgentThreadEntry{ThreadID: id, UserID: user, Sequence: row.LastSequence + 1, ClientKey: req.ClientKey, RequestHash: hash, Kind: req.Kind, RunID: req.RunID, ContextRevision: row.ContextRevision, CreatedAt: now}
		if e := repo.CreateAgentThreadEntry(entry); e != nil {
			return e
		}
		row.LastSequence = entry.Sequence
		row.Revision++
		row.UpdatedAt = now
		return repo.SaveAgentThread(row, req.Revision)
	})
	return result, threadError(err)
}
func (s *Service) threadReceipt(user, kind, id string) (AgentThreadReceipt, error) {
	receipt := AgentThreadReceipt{Kind: kind, RunID: id}
	switch kind {
	case "plugin":
		r, e := s.repo.PluginRunForUser(user, id)
		if e != nil {
			return receipt, threadError(e)
		}
		receipt.Status = string(r.Status)
	case "creation":
		r, e := s.repo.CreationRun(user, id)
		if e != nil {
			return receipt, threadError(e)
		}
		receipt.Status = r.Status
	default:
		return receipt, BadAuthRequest("仅支持插件或创作运行引用")
	}
	return receipt, nil
}
func (s *Service) GetAgentThread(user, id string, before int64) (*AgentThreadView, error) {
	if before < 0 {
		return nil, BadAuthRequest("消息游标无效")
	}
	row, err := s.repo.AgentThread(user, id)
	if err != nil {
		return nil, threadError(err)
	}
	entries, err := s.repo.AgentThreadEntries(user, id, before)
	if err != nil {
		return nil, err
	}
	bindings, err := s.repo.AgentThreadCanvases(user, id)
	if err != nil {
		return nil, err
	}
	view := &AgentThreadView{Thread: row, Entries: []AgentThreadEntryView{}, Canvases: bindings}
	if len(entries) == 20 {
		view.NextBefore = entries[len(entries)-1].Sequence
	}
	for i := len(entries) - 1; i >= 0; i-- {
		entry := entries[i]
		item := AgentThreadEntryView{AgentThreadEntry: entry, References: []AgentThreadReceipt{}}
		if entry.Kind == "agent" {
			var context CloudAgentRequest
			if err = json.Unmarshal([]byte(entry.ContextJSON), &context); err != nil {
				return nil, err
			}
			item.Context = &context
			item.Run, err = s.CloudAgentRun(user, entry.RunID)
			if err != nil {
				return nil, err
			}
			plugins, e := s.repo.PluginRunsForAgent(user, entry.RunID)
			if e != nil {
				return nil, e
			}
			for _, p := range plugins {
				item.References = append(item.References, AgentThreadReceipt{Kind: "plugin", RunID: p.ID, Status: string(p.Status)})
			}
		} else {
			receipt, e := s.threadReceipt(user, entry.Kind, entry.RunID)
			if e != nil {
				return nil, e
			}
			item.References = append(item.References, receipt)
		}
		view.Entries = append(view.Entries, item)
	}
	current, err := s.repo.AgentThread(user, id)
	if err != nil {
		return nil, threadError(err)
	}
	if current.Revision != row.Revision {
		return nil, threadError(repository.ErrAgentThreadConflict)
	}
	return view, nil
}
