package repository

import (
	"errors"
	"gorm.io/gorm"
	"infinite-canvas/backend/internal/model"
)

var ErrAgentThreadConflict = errors.New("agent thread revision changed")

func (r *Repository) WithTransaction(fn func(*Repository) error) error {
	return r.db.Transaction(func(tx *gorm.DB) error { return fn(New(tx)) })
}

func (r *Repository) AgentThread(user, id string) (*model.AgentThread, error) {
	var row model.AgentThread
	err := r.db.Where("id = ? AND user_id = ?", id, user).First(&row).Error
	return &row, err
}
func (r *Repository) AgentThreadByKey(user, key string) (*model.AgentThread, error) {
	var row model.AgentThread
	err := r.db.Where("user_id = ? AND client_key = ?", user, key).First(&row).Error
	return &row, err
}
func (r *Repository) CreateAgentThread(row *model.AgentThread) error { return r.db.Create(row).Error }
func (r *Repository) AgentThreads(user, canvas string, offset int) ([]model.AgentThread, error) {
	rows := []model.AgentThread{}
	q := r.db.Where("user_id = ?", user)
	if canvas != "" {
		q = q.Where("id IN (SELECT thread_id FROM agent_thread_canvases WHERE user_id = ? AND canvas_id = ?)", user, canvas)
	}
	err := q.Order("updated_at DESC, id DESC").Offset(offset).Limit(50).Find(&rows).Error
	return rows, err
}
func (r *Repository) WithAgentThread(user, id string, fn func(*model.AgentThread, *Repository) error) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		// Acquire the write lock before any read, also serializing SQLite writers.
		locked := tx.Model(&model.AgentThread{}).Where("id = ? AND user_id = ?", id, user).UpdateColumn("revision", gorm.Expr("revision"))
		if locked.Error != nil {
			return locked.Error
		}
		if locked.RowsAffected != 1 {
			return gorm.ErrRecordNotFound
		}
		repo := New(tx)
		row, err := repo.AgentThread(user, id)
		if err != nil {
			return err
		}
		return fn(row, repo)
	})
}
func (r *Repository) SaveAgentThread(row *model.AgentThread, previous int64) error {
	result := r.db.Model(&model.AgentThread{}).Where("id = ? AND user_id = ? AND revision = ?", row.ID, row.UserID, previous).Updates(map[string]any{"title": row.Title, "canvas_id": row.CanvasID, "revision": row.Revision, "context_revision": row.ContextRevision, "last_sequence": row.LastSequence, "last_agent_run_id": row.LastAgentRunID, "updated_at": row.UpdatedAt})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrAgentThreadConflict
	}
	return nil
}
func (r *Repository) AgentThreadEntryByKey(thread, key string) (*model.AgentThreadEntry, error) {
	var row model.AgentThreadEntry
	err := r.db.Where("thread_id = ? AND client_key = ?", thread, key).First(&row).Error
	return &row, err
}
func (r *Repository) CreateAgentThreadEntry(row *model.AgentThreadEntry) error {
	return r.db.Create(row).Error
}
func (r *Repository) AgentThreadEntries(user, thread string, before int64) ([]model.AgentThreadEntry, error) {
	rows := []model.AgentThreadEntry{}
	q := r.db.Where("thread_id = ? AND user_id = ?", thread, user)
	if before > 0 {
		q = q.Where("sequence < ?", before)
	}
	err := q.Order("sequence DESC").Limit(20).Find(&rows).Error
	return rows, err
}
func (r *Repository) BindAgentThreadCanvas(row *model.AgentThreadCanvas) error {
	var count int64
	if err := r.db.Model(row).Where("thread_id = ? AND canvas_id = ?", row.ThreadID, row.CanvasID).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	return r.db.Create(row).Error
}
func (r *Repository) AgentThreadCanvases(user, thread string) ([]model.AgentThreadCanvas, error) {
	rows := []model.AgentThreadCanvas{}
	err := r.db.Where("thread_id = ? AND user_id = ?", thread, user).Order("created_at").Find(&rows).Error
	return rows, err
}
func (r *Repository) PluginRunsForAgent(user, agent string) ([]model.PluginRun, error) {
	rows := []model.PluginRun{}
	err := r.db.Where("user_id = ? AND agent_run_id = ? AND parent_run_id = ?", user, agent, "").Order("created_at").Limit(256).Find(&rows).Error
	return rows, err
}
