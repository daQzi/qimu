package repository

import (
	"errors"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"infinite-canvas/backend/internal/model"
	"time"
)

func (r *Repository) PluginConnection(userID, pluginID, connectorID string) (*model.PluginConnection, error) {
	var row model.PluginConnection
	err := r.db.Where("user_id=? AND plugin_id=? AND connector_id=?", userID, pluginID, connectorID).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &row, err
}
func (r *Repository) PluginConnections(userID string) ([]model.PluginConnection, error) {
	var rows []model.PluginConnection
	err := r.db.Where("user_id=?", userID).Order("updated_at desc").Limit(100).Find(&rows).Error
	return rows, err
}
func (r *Repository) PluginConnectionVersion(userID, id string) (*model.PluginConnectionVersion, error) {
	var row model.PluginConnectionVersion
	err := r.db.Where("user_id=? AND id=?", userID, id).First(&row).Error
	return &row, err
}

// The caller holds the plugin catalog transaction, including the old revision check.
func (r *Repository) SavePluginConnection(row *model.PluginConnection, version *model.PluginConnectionVersion) error {
	if err := r.db.Create(version).Error; err != nil {
		return err
	}
	return r.db.Save(row).Error
}
func (r *Repository) PluginOperationPrice(releaseID, operationID string) (*model.PluginOperationPrice, error) {
	var row model.PluginOperationPrice
	err := r.db.Where("release_id=? AND operation_id=?", releaseID, operationID).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return &model.PluginOperationPrice{ReleaseID: releaseID, OperationID: operationID}, nil
	}
	return &row, err
}
func (r *Repository) SavePluginOperationPrice(row *model.PluginOperationPrice) error {
	return r.db.Save(row).Error
}
func (r *Repository) PluginRemoteForRun(userID, runID string) (*model.PluginRemoteExecution, error) {
	var row model.PluginRemoteExecution
	err := r.db.Where("user_id=? AND run_id=?", userID, runID).First(&row).Error
	return &row, err
}
func (r *Repository) PluginRemoteTask(taskID string) (*model.PluginRemoteExecution, error) {
	var row model.PluginRemoteExecution
	err := r.db.First(&row, "task_id=?", taskID).Error
	return &row, err
}
func (r *Repository) CreatePluginRemote(row *model.PluginRemoteExecution) error {
	if err := r.db.Create(row).Error; err != nil {
		return err
	}
	return r.db.Model(&model.PluginRunStep{}).Where("run_id=? AND step_key=?", row.RunID, "invoke").Update("task_id", row.TaskID).Error
}
func (r *Repository) SavePluginRemote(row *model.PluginRemoteExecution) error {
	return r.db.Save(row).Error
}
func (r *Repository) ResumePluginBilling(id string) error {
	if id == "" {
		return nil
	}
	return r.db.Model(&model.BillingOrder{}).Where("id=? AND status=?", id, model.BillingStatusUncertain).Updates(map[string]any{"status": model.BillingStatusRunning, "error": "", "updated_at": time.Now()}).Error
}

// Each claim has a unique owner. The row lock also serializes cancellation and
// resumption with result publication; expired writers cannot change local truth.
func (r *Repository) WithPluginTask(task model.Task, fn func(*Repository, *model.Task, *model.PluginRemoteExecution, *model.PluginRun) error) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		repo := New(tx)
		current, err := repo.LockPluginTask(task.UserID, task.ID)
		if err != nil {
			return err
		}
		if current.Type != model.TaskTypePluginOperation || current.Status != model.TaskStatusRunning || current.LeaseOwner != task.LeaseOwner || current.LeaseExpiresAt == nil || !current.LeaseExpiresAt.After(time.Now()) {
			return ErrTaskStateConflict
		}
		execution, err := repo.PluginRemoteTask(task.ID)
		if err != nil {
			return err
		}
		run, err := repo.PluginRunForUser(task.UserID, execution.RunID)
		if err != nil {
			return err
		}
		return fn(repo, current, execution, run)
	})
}
func (r *Repository) LockPluginTask(userID, id string) (*model.Task, error) {
	query := r.db
	if r.Dialect() == "postgres" {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var row model.Task
	err := query.Where("id=? AND user_id=?", id, userID).First(&row).Error
	return &row, err
}
func (r *Repository) SavePluginTask(task *model.Task) error {
	return r.db.Model(&model.Task{}).Where("id=? AND user_id=?", task.ID, task.UserID).Select("*").Omit("id", "created_at").Updates(task).Error
}

// Must be called while holding the task lock. Limits the whole connection (all
// versions) to one API request/second across workers and hosts.
func (r *Repository) AllowPluginRequest(connectionID string) (bool, time.Time, error) {
	if err := r.db.Clauses(clause.OnConflict{DoNothing: true}).Create(&model.PluginConnectionRate{ID: connectionID}).Error; err != nil {
		return false, time.Time{}, err
	}
	query := r.db
	if r.Dialect() == "postgres" {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var row model.PluginConnectionRate
	if err := query.First(&row, "id=?", connectionID).Error; err != nil {
		return false, time.Time{}, err
	}
	now := time.Now()
	if row.NextRequestAt.After(now) {
		return false, row.NextRequestAt, nil
	}
	return true, now, r.db.Model(&row).Update("next_request_at", now.Add(time.Second)).Error
}
func (r *Repository) AddPluginRunResources(userID, runID, role string, ids []string) error {
	for _, id := range ids {
		resource, err := r.LockResourceForUser(userID, id)
		if err != nil {
			return err
		}
		if resource.Status != model.ResourceStatusReady {
			return errors.New("plugin resource is not ready")
		}
		if err = r.db.Clauses(clause.OnConflict{DoNothing: true}).Create(&model.PluginRunResource{RunID: runID, ResourceID: id, UserID: userID, Role: role}).Error; err != nil {
			return err
		}
	}
	return nil
}
func (r *Repository) ReleasePluginInputResources(userID, runID string) error {
	return r.db.Where("user_id=? AND run_id=? AND role=?", userID, runID, "input").Delete(&model.PluginRunResource{}).Error
}
func (r *Repository) PluginAgentCharges(userID, agentID string) (int64, error) {
	var total int64
	err := r.db.Table("billing_orders AS b").Select("COALESCE(SUM(b.amount_microcredits),0)").Joins("JOIN plugin_runs AS p ON p.task_id=b.task_id AND p.user_id=b.user_id").Where("p.user_id=? AND p.agent_run_id=?", userID, agentID).Scan(&total).Error
	return total, err
}
func (r *Repository) LockPluginAgentBudget(userID, id string) (*model.CloudAgentExecution, error) {
	query := r.db
	if r.Dialect() == "postgres" {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var row model.CloudAgentExecution
	err := query.Where("user_id=? AND id=?", userID, id).First(&row).Error
	return &row, err
}
