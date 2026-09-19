package app

import (
	"errors"
	"gorm.io/gorm"
	"infinite-canvas/backend/internal/model"
	"infinite-canvas/backend/internal/plugins"
	"log"
)

type PluginInputUpdate = plugins.InputUpdate

func (s *Service) UpdatePluginInput(user, run, id, key string, req PluginInputUpdate) (plugins.RunView, error) {
	if err := s.pluginOperationAccess(user); err != nil {
		return plugins.RunView{}, err
	}
	return s.applicationPlugins().UpdateInput(user, run, id, key, req)
}
func (s *Service) ListPluginRuns(user string, offset int) ([]model.PluginRun, error) {
	if offset < 0 || offset > 10000 {
		return nil, BadAuthRequest("运行游标无效")
	}
	return s.repo.ListPluginRuns(user, offset)
}
func (s *Service) PluginRunEvents(user, id string, after int64) ([]model.PluginRunEvent, error) {
	if after < 0 {
		return nil, BadAuthRequest("事件游标无效")
	}
	run, err := s.repo.PluginRunForUser(user, id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, NotFound("插件运行不存在")
	}
	if err != nil {
		return nil, err
	}
	if after > run.EventSequence {
		return nil, creationConflict("事件游标超出当前快照，请刷新运行")
	}
	return s.repo.PluginRunEvents(user, id, after)
}
func (s *Service) advancePluginPipelines() {
	rows, err := s.repo.PendingPluginPipelines()
	if err != nil {
		log.Printf("plugin pipeline scan failed: %v", err)
		return
	}
	for _, run := range rows {
		if s.IsDraining() {
			return
		}
		if err = s.applicationPlugins().AdvancePipeline(run.UserID, run.ID); err != nil {
			log.Printf("plugin pipeline advance failed: run=%s error=%v", run.ID, err)
		}
	}
}
