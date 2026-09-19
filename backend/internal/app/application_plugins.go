package app

import (
	"encoding/json"
	"errors"
	"os"
	"strings"

	"infinite-canvas/backend/internal/model"
	"infinite-canvas/backend/internal/plugins"
	"infinite-canvas/backend/internal/protocol"
	"infinite-canvas/backend/internal/repository"
)

type ApplicationPluginActivation = plugins.Activation
type ApplicationPluginManagement = plugins.ManagementChange
type ApplicationPluginView = plugins.ApplicationView

func (s *Service) applicationPlugins() *plugins.Service {
	setting := strings.ToLower(strings.TrimSpace(os.Getenv("CANVAS_APPLICATION_PLUGINS_ENABLED")))
	return plugins.New(s.repo, s.dataDir, setting == "" || setting == "true" || setting == "1").WithAdapters(pluginHostAdapters()).WithRemoteHost(pluginRemoteHost{svc: s}).WithRunAccess(func(repo *repository.Repository, user string) error {
		return (&Service{repo: repo}).pluginOperationAccess(user)
	})
}
func (s *Service) ApplicationPlugins(actor *model.User) ([]ApplicationPluginView, error) {
	if actor == nil || actor.ID == "" {
		return nil, Forbidden("请先登录")
	}
	if actor.Role != model.UserRoleAdmin {
		if err := s.RequireFeature(FeaturePluginCenter); err != nil {
			return nil, err
		}
	}
	return s.applicationPlugins().Catalog(actor.ID, actor.Role == model.UserRoleAdmin)
}
func (s *Service) ActivateApplicationPlugin(actor *model.User, id string, req ApplicationPluginActivation) error {
	if actor == nil || actor.ID == "" {
		return Forbidden("请先登录")
	}
	if actor.Role != model.UserRoleAdmin {
		if err := s.RequireFeature(FeaturePluginCenter); err != nil {
			return err
		}
	}
	return s.applicationPlugins().Activate(actor.ID, id, req)
}
func (s *Service) ManageApplicationPlugin(actor *model.User, id string, req ApplicationPluginManagement) error {
	if err := s.RequireAdmin(actor); err != nil {
		return err
	}
	return s.applicationPlugins().Manage(actor.ID, id, req)
}
func (s *Service) PruneApplicationPluginOrphans(actor *model.User, dryRun bool) ([]string, error) {
	if err := s.RequireAdmin(actor); err != nil {
		return nil, err
	}
	items, err := s.applicationPlugins().PruneOrphans(dryRun)
	if err != nil {
		return items, err
	}
	if !dryRun && len(items) > 0 {
		if err = s.appendAdminAudit(actor, "application_plugin.orphans.prune", "plugin", "archives", "清理无引用应用插件归档", map[string]any{"packages": items}); err != nil {
			return items, err
		}
	}
	return items, nil
}

// InstallManagedPluginForAdmin dispatches only after reading the bounded ZIP
// envelope. Legacy registries remain authoritative for v1/v2.
func (s *Service) InstallManagedPluginForAdmin(actor *model.User, data []byte, fileName string) (any, error) {
	if err := s.RequireAdmin(actor); err != nil {
		return nil, err
	}
	s.applicationPluginMu.Lock()
	defer s.applicationPluginMu.Unlock()
	pkg, err := protocol.ReadPluginPackageEnvelope(data)
	if err != nil {
		return nil, BadAuthRequest(err.Error())
	}
	var header struct {
		APIVersion string `json:"apiVersion"`
		ID         string `json:"id"`
	}
	if err = json.Unmarshal(pkg.ManifestRaw, &header); err != nil {
		return nil, BadAuthRequest("插件清单无法解析")
	}
	if header.APIVersion != "yingce.plugin/v3" {
		parsed, err := protocol.ParsePluginPackage(data)
		if err != nil {
			return nil, BadAuthRequest(err.Error())
		}
		legacyID := strings.TrimSpace(parsed.Manifest.Metadata.ID)
		if err := s.repo.ClaimPluginNamespace(legacyID, "legacy"); err != nil {
			if errors.Is(err, repository.ErrPluginNamespaceConflict) {
				return nil, Forbidden("插件 ID 已由应用版本目录管理")
			}
			return nil, err
		}
		owned, err := s.repo.PluginApplication(legacyID)
		if err != nil {
			return nil, err
		}
		if owned != nil {
			return nil, Forbidden("插件 ID 已由应用插件版本目录管理")
		}
		return s.InstallPluginForAdmin(actor, data, fileName)
	}
	reserved := knownPluginIDs(s.Plugins())
	id, err := s.applicationPlugins().Install(actor.ID, data, reserved)
	if err != nil {
		return nil, err
	}
	return map[string]any{"id": id, "apiVersion": "yingce.plugin/v3", "kind": "application"}, nil
}
