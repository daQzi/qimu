package repository

import (
	"infinite-canvas/backend/internal/model"
	"time"
)

func (r *Repository) CreatePluginPipeline(p *model.PluginPipelineExecution) error {
	return r.db.Create(p).Error
}
func (r *Repository) PluginPipeline(id string) (*model.PluginPipelineExecution, error) {
	var row model.PluginPipelineExecution
	err := r.db.First(&row, "run_id=?", id).Error
	return &row, err
}
func (r *Repository) SavePluginPipeline(p *model.PluginPipelineExecution, owner string) error {
	result := r.db.Model(&model.PluginPipelineExecution{}).Where("run_id=? AND lease_owner=? AND lease_expires_at>?", p.RunID, owner, time.Now()).Updates(map[string]any{"cursor": p.Cursor, "child_run_id": p.ChildRunID, "outputs_json": p.OutputsJSON, "batch_json": p.BatchJSON, "lease_owner": "", "lease_expires_at": nil, "updated_at": time.Now()})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrTaskStateConflict
	}
	return nil
}
func (r *Repository) ClaimPluginPipeline(id, owner string) (bool, error) {
	expires := time.Now().Add(30 * time.Second)
	q := r.db.Model(&model.PluginPipelineExecution{}).Where("run_id=? AND (lease_owner='' OR lease_expires_at IS NULL OR lease_expires_at<=?)", id, time.Now()).Updates(map[string]any{"lease_owner": owner, "lease_expires_at": expires})
	return q.RowsAffected == 1, q.Error
}
func (r *Repository) PendingPluginPipelines() ([]model.PluginRun, error) {
	rows := []model.PluginRun{}
	err := r.db.Table("plugin_runs").Select("plugin_runs.*").Joins("JOIN plugin_pipeline_executions AS pipeline ON pipeline.run_id=plugin_runs.id").Where("plugin_runs.pipeline_id<>'' AND plugin_runs.status IN ?", []string{"queued", "running", "waiting_input", "waiting_approval", "cancelling"}).Order("pipeline.updated_at, plugin_runs.id").Limit(100).Find(&rows).Error
	return rows, err
}
func (r *Repository) ListPluginRuns(user string, offset int) ([]model.PluginRun, error) {
	rows := []model.PluginRun{}
	err := r.db.Where("user_id=? AND (parent_run_id='' OR parent_run_id IS NULL)", user).Order("created_at desc, id desc").Offset(offset).Limit(30).Find(&rows).Error
	return rows, err
}
func (r *Repository) LinkPluginChild(user, id, parent, agent, step string) error {
	root, err := r.PluginRunForUser(user, parent)
	if err != nil {
		return err
	}
	if err := r.db.Model(&model.PluginRun{}).Where("id=? AND user_id=? AND (parent_run_id='' OR parent_run_id=?)", id, user, parent).Updates(map[string]any{"parent_run_id": parent, "agent_run_id": agent, "parent_step_key": step, "attempt": root.Attempt}).Error; err != nil {
		return err
	}
	return r.db.Model(&model.PluginRunStep{}).Where("run_id=?", id).Update("attempt", root.Attempt).Error
}
func (r *Repository) BindPluginDerivedAgent(user, id, agent string) error {
	return r.db.Model(&model.PluginRun{}).Where("id=? AND user_id=? AND derived_from_run_id<>''", id, user).Update("agent_run_id", agent).Error
}
func (r *Repository) PluginPipelineChildren(user, parent string) ([]model.PluginRun, error) {
	rows := []model.PluginRun{}
	err := r.db.Where("user_id=? AND parent_run_id=?", user, parent).Order("created_at, id").Limit(256).Find(&rows).Error
	return rows, err
}

func (r *Repository) SetPluginDerivation(run, source string, attempt int, batch string) error {
	var original model.PluginRun
	if err := r.db.First(&original, "id=?", source).Error; err != nil {
		return err
	}
	if err := r.db.Model(&model.PluginRun{}).Where("id=?", run).Updates(map[string]any{"derived_from_run_id": source, "attempt": attempt, "agent_run_id": original.AgentRunID}).Error; err != nil {
		return err
	}
	if err := r.db.Model(&model.PluginRunStep{}).Where("run_id=?", run).Update("attempt", attempt).Error; err != nil {
		return err
	}
	return r.db.Model(&model.PluginPipelineExecution{}).Where("run_id=?", run).Update("batch_json", batch).Error
}
func (r *Repository) CreatePluginBatchApproval(row *model.PluginBatchApproval) error {
	return r.db.Create(row).Error
}
func (r *Repository) PluginBatchApprovals(user, run string) ([]model.PluginBatchApproval, error) {
	rows := []model.PluginBatchApproval{}
	err := r.db.Where("user_id=? AND run_id=?", user, run).Order("created_at").Find(&rows).Error
	return rows, err
}
func (r *Repository) CopyPluginResultResources(user, source, target string) error {
	var rows []model.PluginRunResource
	if err := r.db.Where("user_id=? AND run_id=?", user, source).Find(&rows).Error; err != nil {
		return err
	}
	for _, row := range rows {
		if err := r.AddPluginRunResources(user, target, "result", []string{row.ResourceID}); err != nil {
			return err
		}
	}
	return nil
}
func (r *Repository) CreatePluginInput(row *model.PluginInputRequest) error {
	return r.db.Create(row).Error
}
func (r *Repository) PluginInput(run, id string) (*model.PluginInputRequest, error) {
	var row model.PluginInputRequest
	err := r.db.First(&row, "run_id=? AND id=?", run, id).Error
	return &row, err
}
func (r *Repository) PluginInputs(run string) ([]model.PluginInputRequest, error) {
	rows := []model.PluginInputRequest{}
	err := r.db.Where("run_id=?", run).Order("created_at, id").Find(&rows).Error
	return rows, err
}
func (r *Repository) SavePluginInput(row *model.PluginInputRequest, revision int64) error {
	q := r.db.Model(&model.PluginInputRequest{}).Where("id=? AND revision=?", row.ID, revision).Updates(map[string]any{"revision": row.Revision, "status": row.Status, "draft_json": row.DraftJSON, "submitted_json": row.SubmittedJSON, "submission_key": row.SubmissionKey, "submission_digest": row.SubmissionDigest, "updated_at": time.Now()})
	if q.Error != nil {
		return q.Error
	}
	if q.RowsAffected != 1 {
		return ErrTaskStateConflict
	}
	return nil
}
