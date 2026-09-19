package repository

import (
	"errors"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"infinite-canvas/backend/internal/model"
	"time"
)

var ErrPluginRunResourceReferenced = errors.New("resource referenced by a plugin run")

func (r *Repository) PluginRunForUser(userID, id string) (*model.PluginRun, error) {
	var run model.PluginRun
	err := r.db.First(&run, "id=? AND user_id=?", id, userID).Error
	return &run, err
}

// The parent row lock serializes short plugin admission with Agent cancellation.
// Called inside the same catalog transaction that creates the PluginRun.
func (r *Repository) LockActivePluginAgent(userID, id string, revision int64) error {
	query := r.db
	if r.Dialect() == "postgres" {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var run model.CloudAgentExecution
	if err := query.First(&run, "id=? AND user_id=?", id, userID).Error; err != nil {
		return err
	}
	if run.Revision != revision || run.Status != "running" {
		return ErrCreationConflict
	}
	return nil
}
func (r *Repository) PluginRunByKey(userID, key string) (*model.PluginRun, error) {
	var run model.PluginRun
	err := r.db.First(&run, "user_id=? AND idempotency_key=?", userID, key).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &run, err
}
func (r *Repository) LatestPluginRunForAgent(userID, agentID string) (*model.PluginRun, error) {
	var run model.PluginRun
	err := r.db.Where("user_id=? AND agent_run_id=?", userID, agentID).Order("created_at desc, id desc").First(&run).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &run, err
}
func (r *Repository) CreatePluginRun(run *model.PluginRun, step *model.PluginRunStep) error {
	var count int64
	if err := r.db.Model(&model.PluginRun{}).Where("user_id=?", run.UserID).Count(&count).Error; err != nil {
		return err
	}
	if count >= 10000 {
		return errors.New("plugin run retention quota exceeded")
	}
	if err := r.db.Create(run).Error; err != nil {
		return err
	}
	return r.db.Create(step).Error
}
func (r *Repository) SavePluginRun(run *model.PluginRun, expected int64, stepStatus string) error {
	result := r.db.Model(&model.PluginRun{}).Where("id=? AND user_id=? AND revision=?", run.ID, run.UserID, expected).Updates(map[string]any{"status": run.Status, "revision": run.Revision, "event_sequence": run.EventSequence, "approval_decision": run.ApprovalDecision, "result_json": run.ResultJSON, "source_resource_id": run.SourceResourceID, "projection_status": run.ProjectionStatus, "failure_message": run.FailureMessage, "task_id": run.TaskID, "connection_version_id": run.ConnectionVersionID, "updated_at": time.Now()})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrTaskStateConflict
	}
	return r.db.Model(&model.PluginRunStep{}).Where("run_id=?", run.ID).Updates(map[string]any{"status": stepStatus, "updated_at": time.Now()}).Error
}

func (r *Repository) RecordPluginProjectionConflict(userID, id string, revision int64, message string) error {
	return r.db.Model(&model.PluginRun{}).Where("id=? AND user_id=? AND revision=? AND status=?", id, userID, revision, "waiting_approval").Updates(map[string]any{"projection_status": "conflict", "failure_message": message}).Error
}

func (r *Repository) PluginCanvasProjection(userID, id string) (*model.PluginCanvasProjection, error) {
	var row model.PluginCanvasProjection
	err := r.db.Where("id=? AND user_id=?", id, userID).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &row, err
}

func (r *Repository) CreatePluginCanvasProjection(row *model.PluginCanvasProjection) error {
	return r.db.Create(row).Error
}
func (r *Repository) AppendPluginRunEvent(event *model.PluginRunEvent) error {
	return r.db.Create(event).Error
}
func (r *Repository) PluginRunEvents(userID, runID string, after int64) ([]model.PluginRunEvent, error) {
	if _, err := r.PluginRunForUser(userID, runID); err != nil {
		return nil, err
	}
	events := []model.PluginRunEvent{}
	err := r.db.Where("run_id=? AND sequence>?", runID, after).Order("sequence").Limit(100).Find(&events).Error
	return events, err
}
func (r *Repository) LockResourceForUser(userID, id string) (*model.Resource, error) {
	query := r.db
	if r.Dialect() == "postgres" {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var row model.Resource
	err := query.First(&row, "id=? AND user_id=?", id, userID).Error
	return &row, err
}
