package app

import (
	"encoding/json"
	"errors"
	"net/url"
	"strings"
	"time"

	"infinite-canvas/backend/internal/kernel"
	"infinite-canvas/backend/internal/model"
	"infinite-canvas/backend/internal/plugins/contracts"
	"infinite-canvas/backend/internal/repository"
)

type PluginConnectionInput struct {
	PluginID    string `json:"pluginId"`
	ConnectorID string `json:"connectorId"`
	Name        string `json:"name"`
	BaseURL     string `json:"baseUrl"`
	Credential  string `json:"credential"`
	Enabled     bool   `json:"enabled"`
	Revision    int64  `json:"revision"`
}
type PluginConnectionView struct {
	ID                   string    `json:"id"`
	PluginID             string    `json:"pluginId"`
	ConnectorID          string    `json:"connectorId"`
	Name                 string    `json:"name"`
	BaseURL              string    `json:"baseUrl"`
	Revision             int64     `json:"revision"`
	Enabled              bool      `json:"enabled"`
	CredentialConfigured bool      `json:"credentialConfigured"`
	AuthType             string    `json:"authType"`
	UpdatedAt            time.Time `json:"updatedAt"`
}

func (s *Service) ApplicationPluginConnections(userID string) ([]PluginConnectionView, error) {
	if err := s.pluginOperationAccess(userID); err != nil {
		return nil, err
	}
	rows, err := s.repo.PluginConnections(userID)
	if err != nil {
		return nil, err
	}
	views := make([]PluginConnectionView, 0, len(rows))
	for _, row := range rows {
		version, readErr := s.repo.PluginConnectionVersion(userID, row.VersionID)
		if readErr != nil {
			return nil, readErr
		}
		views = append(views, pluginConnectionView(row, *version, version.AuthType))
	}
	return views, nil
}
func pluginConnectionView(row model.PluginConnection, version model.PluginConnectionVersion, authType string) PluginConnectionView {
	return PluginConnectionView{ID: row.ID, PluginID: row.PluginID, ConnectorID: row.ConnectorID, Name: row.Name, BaseURL: version.BaseURL, Revision: row.Revision, Enabled: row.Enabled, CredentialConfigured: version.SecretCipher != "", AuthType: authType, UpdatedAt: row.UpdatedAt}
}
func (s *Service) SaveApplicationPluginConnection(userID string, input PluginConnectionInput) (PluginConnectionView, error) {
	if err := s.pluginOperationAccess(userID); err != nil {
		return PluginConnectionView{}, err
	}
	input.PluginID = strings.TrimSpace(input.PluginID)
	input.ConnectorID = strings.TrimSpace(input.ConnectorID)
	input.Name = strings.TrimSpace(input.Name)
	input.BaseURL = strings.TrimSpace(input.BaseURL)
	if input.PluginID == "" || input.ConnectorID == "" || len([]rune(input.Name)) > 120 {
		return PluginConnectionView{}, BadAuthRequest("连接名称或插件标识无效")
	}
	description, err := s.applicationPlugins().DescribeConnector(userID, input.PluginID, input.ConnectorID)
	if err != nil {
		return PluginConnectionView{}, err
	}
	parsed, err := ValidateCustomRelayURL(input.BaseURL)
	if err != nil {
		return PluginConnectionView{}, err
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return PluginConnectionView{}, BadAuthRequest("连接地址不能包含查询参数或片段")
	}
	var out PluginConnectionView
	err = s.repo.WithPluginCatalog(func(repo *repository.Repository) error {
		row, err := repo.PluginConnection(userID, input.PluginID, input.ConnectorID)
		if err != nil {
			return err
		}
		if row == nil {
			if input.Revision != 0 {
				return repository.ErrCreationConflict
			}
			row = &model.PluginConnection{ID: newID(), UserID: userID, PluginID: input.PluginID, ConnectorID: input.ConnectorID, CreatedAt: time.Now().UTC()}
		} else if row.Revision != input.Revision {
			return repository.ErrCreationConflict
		}
		secret := input.Credential
		if secret == "" && row.VersionID != "" {
			old, err := repo.PluginConnectionVersion(userID, row.VersionID)
			if err != nil {
				return err
			}
			oldURL, _ := url.Parse(old.BaseURL)
			if oldURL.Scheme != parsed.Scheme || oldURL.Host != parsed.Host {
				return BadAuthRequest("变更连接目标时必须重新填写凭据")
			}
			secret, err = s.decryptSettingSecret(old.SecretCipher)
			if err != nil {
				return err
			}
		}
		if description.Definition.Auth.Type != "none" && secret == "" {
			return BadAuthRequest("该连接需要凭据")
		}
		if len(secret) > 8192 || strings.ContainsAny(secret, "\r\n") {
			return BadAuthRequest("连接凭据无效")
		}
		cipher, err := s.encryptSettingSecret(secret)
		if err != nil {
			return err
		}
		row.Name = input.Name
		row.Enabled = input.Enabled
		row.Revision++
		row.UpdatedAt = time.Now().UTC()
		version := &model.PluginConnectionVersion{ID: newID(), ConnectionID: row.ID, Revision: row.Revision, UserID: userID, BaseURL: parsed.String(), SecretCipher: cipher, AuthType: description.Definition.Auth.Type, CreatedAt: time.Now().UTC()}
		row.VersionID = version.ID
		if err = repo.SavePluginConnection(row, version); err != nil {
			return err
		}
		out = pluginConnectionView(*row, *version, description.Definition.Auth.Type)
		return nil
	})
	if errors.Is(err, repository.ErrCreationConflict) {
		return PluginConnectionView{}, creationConflict("连接配置已变化，请刷新后重试")
	}
	return out, err
}

type PluginOperationPriceInput struct {
	ReleaseID       string `json:"releaseId"`
	OperationID     string `json:"operationId"`
	FeeMicrocredits int64  `json:"feeMicrocredits"`
	Revision        int64  `json:"revision"`
}

func (s *Service) PluginOperationPriceForAdmin(actor *model.User, releaseID, operationID string) (*model.PluginOperationPrice, error) {
	if actor == nil || actor.Role != model.UserRoleAdmin {
		return nil, Forbidden("仅管理员可读取平台服务费配置")
	}
	return s.repo.PluginOperationPrice(releaseID, operationID)
}

func (s *Service) SavePluginOperationPrice(actor *model.User, input PluginOperationPriceInput) (*model.PluginOperationPrice, error) {
	if actor == nil || actor.Role != model.UserRoleAdmin {
		return nil, Forbidden("仅管理员可配置插件平台服务费")
	}
	if input.FeeMicrocredits < 0 || input.FeeMicrocredits > 1_000_000_000 {
		return nil, BadAuthRequest("平台服务费超出范围")
	}
	var out *model.PluginOperationPrice
	err := s.repo.WithPluginCatalog(func(repo *repository.Repository) error {
		release, err := repo.PluginRelease(input.ReleaseID)
		if err != nil {
			return err
		}
		files, err := s.applicationPlugins().LoadReleasePackage(*release)
		if err != nil {
			return err
		}
		var manifest contracts.Manifest
		if err = json.Unmarshal(files["manifest.json"], &manifest); err != nil {
			return err
		}
		found := false
		for _, ref := range manifest.Contributes.Operations {
			if ref.ID == input.OperationID {
				found = true
			}
		}
		if !found {
			return kernel.NotFound("操作不存在")
		}
		row, err := repo.PluginOperationPrice(input.ReleaseID, input.OperationID)
		if err != nil {
			return err
		}
		if row.Revision != input.Revision {
			return repository.ErrCreationConflict
		}
		if row.ID == "" {
			row.ID = newID()
		}
		row.FeeMicrocredits = input.FeeMicrocredits
		row.Revision++
		row.UpdatedAt = time.Now().UTC()
		if err = repo.SavePluginOperationPrice(row); err != nil {
			return err
		}
		out = row
		return nil
	})
	if errors.Is(err, repository.ErrCreationConflict) {
		return nil, creationConflict("插件服务费已变化")
	}
	return out, err
}
