package app

import (
	"infinite-canvas/backend/internal/plugins"
	"infinite-canvas/backend/internal/plugins/contracts"
)

type PluginWorkbenchComposeRequest = plugins.WorkbenchComposeRequest
type PluginWorkbenchPreview = plugins.WorkbenchPreview
type PluginWorkbenchView = plugins.WorkbenchView

func (s *Service) PluginWorkbenches(user string) ([]PluginWorkbenchView, error) {
	if err := s.pluginOperationAccess(user); err != nil {
		return nil, err
	}
	return s.applicationPlugins().Workbenches(user)
}
func (s *Service) PluginWorkbench(user, id, release string, ctx contracts.InvocationContext) (PluginWorkbenchView, error) {
	if err := s.pluginOperationAccess(user); err != nil {
		return PluginWorkbenchView{}, err
	}
	return s.applicationPlugins().Workbench(user, id, release, ctx)
}
func (s *Service) ComposePluginWorkbench(user string, req PluginWorkbenchComposeRequest) (PluginWorkbenchPreview, error) {
	if err := s.pluginOperationAccess(user); err != nil {
		return PluginWorkbenchPreview{}, err
	}
	return s.applicationPlugins().ComposeWorkbench(user, req)
}
