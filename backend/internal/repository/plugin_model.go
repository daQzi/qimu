package repository

import "infinite-canvas/backend/internal/model"

func (r *Repository) PluginModelTaskCount(user, agent string) (int64, error) {
	var count int64
	err := r.db.Model(&model.Task{}).Where("user_id=? AND agent_run_id=? AND plugin_run_id IS NOT NULL AND type=?", user, agent, "canvas_text").Count(&count).Error
	return count, err
}

func (r *Repository) LinkPluginModelTask(run, task string) error {
	return r.db.Model(&model.PluginRunStep{}).Where("run_id=? AND step_key=?", run, "invoke").Update("task_id", task).Error
}
func (r *Repository) PendingPluginModelRuns() ([]model.PluginRun, error) {
	rows := []model.PluginRun{}
	err := r.db.Table("plugin_runs").Select("plugin_runs.*").Joins("JOIN tasks ON tasks.id=plugin_runs.task_id AND tasks.user_id=plugin_runs.user_id").Where("plugin_runs.connection_version_id='' AND plugin_runs.status IN ? AND (plugin_runs.status=? OR tasks.status IN ?)", []string{"running", "cancelling"}, "cancelling", []string{"succeeded", "failed", "cancelled"}).Order("plugin_runs.updated_at,plugin_runs.id").Limit(100).Find(&rows).Error
	return rows, err
}
