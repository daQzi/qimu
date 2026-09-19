package plugins

import (
	"encoding/json"
	"errors"
	"gorm.io/gorm"
	"infinite-canvas/backend/internal/kernel"
	"infinite-canvas/backend/internal/plugins/contracts"
	"strings"
)

type ConnectorDescription struct {
	PluginID   string                  `json:"pluginId"`
	ReleaseID  string                  `json:"releaseId"`
	Definition contracts.HTTPConnector `json:"definition"`
}

func (s *Service) DescribeConnector(userID, pluginID, connectorID string) (ConnectorDescription, error) {
	state, err := s.repo.UserPluginState(userID, pluginID)
	if err != nil {
		return ConnectorDescription{}, err
	}
	if state == nil || !state.Enabled || state.InstalledReleaseID == "" {
		return ConnectorDescription{}, issue(403, "plugin_disabled", "请先启用插件")
	}
	release, err := s.repo.PluginRelease(state.InstalledReleaseID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ConnectorDescription{}, kernel.NotFound("插件发布不存在")
	}
	if err != nil {
		return ConnectorDescription{}, err
	}
	if err = checkRelease(s.repo, userID, release); err != nil {
		return ConnectorDescription{}, err
	}
	var grants []string
	if err = json.Unmarshal([]byte(state.GrantedPermissionsJSON), &grants); err != nil {
		return ConnectorDescription{}, err
	}
	if !contains(grants, "connection.use") {
		return ConnectorDescription{}, issue(403, "scope_forbidden", "缺少 connection.use 权限")
	}
	files, err := s.loadPackage(*release)
	if err != nil {
		return ConnectorDescription{}, err
	}
	var manifest contracts.Manifest
	if err = json.Unmarshal(files["manifest.json"], &manifest); err != nil {
		return ConnectorDescription{}, err
	}
	connector, err := contracts.ReadHTTPConnector(files, manifest, connectorID)
	if err != nil {
		return ConnectorDescription{}, kernel.NotFound("连接定义不存在")
	}
	return ConnectorDescription{PluginID: strings.TrimSpace(pluginID), ReleaseID: release.ID, Definition: connector}, nil
}
