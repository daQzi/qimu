package repository

import (
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"infinite-canvas/backend/internal/model"
	"time"
)

type PluginConcurrencyLimits struct{ Host, User, Connection, Model, Plugin int }

func (r *Repository) AcquirePluginExecutionSlot(task model.Task, slot model.PluginExecutionSlot, limits PluginConcurrencyLimits) (bool, error) {
	acquired := false
	err := r.WithPluginCatalog(func(tx *Repository) error {
		var live int64
		if err := tx.db.Model(&model.Task{}).Where("id=? AND lease_owner=? AND lease_expires_at>?", task.ID, task.LeaseOwner, time.Now()).Count(&live).Error; err != nil {
			return err
		}
		if live != 1 || task.LeaseOwner == "" {
			return ErrTaskStateConflict
		}
		checks := []struct {
			Field, Value string
			Limit        int
		}{{"", "", limits.Host}, {"user_id", slot.UserID, limits.User}, {"connection_id", slot.ConnectionID, limits.Connection}, {"model_key", slot.ModelKey, limits.Model}, {"plugin_id", slot.PluginID, limits.Plugin}}
		for _, check := range checks {
			if check.Limit < 1 {
				return ErrTaskStateConflict
			}
			q := tx.db.Table("plugin_execution_slots AS slots").Joins("JOIN tasks ON tasks.id=slots.task_id AND tasks.lease_owner=slots.owner").Where("tasks.lease_expires_at>? AND tasks.status=? AND slots.task_id<>?", time.Now(), model.TaskStatusRunning, task.ID)
			if check.Field != "" {
				q = q.Where("slots."+check.Field+"=?", check.Value)
			}
			var count int64
			if err := q.Count(&count).Error; err != nil {
				return err
			}
			if count >= int64(check.Limit) {
				return nil
			}
		}
		slot.TaskID = task.ID
		slot.Owner = task.LeaseOwner
		if err := tx.db.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "task_id"}}, UpdateAll: true}).Create(&slot).Error; err != nil {
			return err
		}
		acquired = true
		return nil
	})
	return acquired, err
}
func (r *Repository) ReleasePluginExecutionSlot(task, owner string) error {
	return r.db.Where("task_id=? AND owner=?", task, owner).Delete(&model.PluginExecutionSlot{}).Error
}

type PluginBatchDiagnostics struct {
	ApprovalConflicts int64 `json:"approvalConflicts"`
	BudgetConflicts   int64 `json:"budgetConflicts"`
	Queued            int64 `json:"queued"`
	ActiveSlots       int64 `json:"activeSlots"`
	Unknown           int64 `json:"unknown"`
	ImportPending     int64 `json:"importPending"`
	WaitingApproval   int64 `json:"waitingApproval"`
	PausedPipelines   int64 `json:"pausedPipelines"`
}

func (r *Repository) PluginBatchDiagnostics(user string) (PluginBatchDiagnostics, error) {
	d := PluginBatchDiagnostics{}
	var metric model.PluginBatchMetric
	if err := r.db.Where("user_id=?", user).Limit(1).Find(&metric).Error; err != nil {
		return d, err
	}
	d.ApprovalConflicts = metric.ApprovalConflicts
	d.BudgetConflicts = metric.BudgetConflicts
	queries := []struct {
		Table, Where string
		Args         []any
		Target       *int64
	}{
		{"tasks", "user_id=? AND type=? AND status=?", []any{user, model.TaskTypePluginOperation, model.TaskStatusQueued}, &d.Queued},
		{"plugin_remote_executions", "user_id=? AND state=?", []any{user, "unknown"}, &d.Unknown},
		{"plugin_remote_executions", "user_id=? AND state=?", []any{user, "import_pending"}, &d.ImportPending},
		{"plugin_runs", "user_id=? AND status=?", []any{user, "waiting_approval"}, &d.WaitingApproval},
		{"plugin_runs", "user_id=? AND pipeline_id<>'' AND status=?", []any{user, "paused"}, &d.PausedPipelines},
	}
	for _, q := range queries {
		if err := r.db.Table(q.Table).Where(q.Where, q.Args...).Count(q.Target).Error; err != nil {
			return d, err
		}
	}
	err := r.db.Table("plugin_execution_slots AS slots").Joins("JOIN tasks ON tasks.id=slots.task_id AND tasks.lease_owner=slots.owner").Where("slots.user_id=? AND tasks.lease_expires_at>? AND tasks.status=?", user, time.Now(), model.TaskStatusRunning).Count(&d.ActiveSlots).Error
	return d, err
}
func (r *Repository) RecordPluginBatchConflict(user string, budget bool) error {
	row := model.PluginBatchMetric{UserID: user, ApprovalConflicts: 1}
	if budget {
		row.BudgetConflicts = 1
	}
	updates := map[string]any{"approval_conflicts": gorm.Expr("plugin_batch_metrics.approval_conflicts + 1")}
	if budget {
		updates["budget_conflicts"] = gorm.Expr("plugin_batch_metrics.budget_conflicts + 1")
	}
	return r.db.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "user_id"}}, DoUpdates: clause.Assignments(updates)}).Create(&row).Error
}
