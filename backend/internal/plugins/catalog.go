package plugins

import (
	"encoding/json"
	"errors"
	"infinite-canvas/backend/internal/kernel"
	"infinite-canvas/backend/internal/model"
	"infinite-canvas/backend/internal/plugins/contracts"
	"sort"
)

func (s *Service) Catalog(userID string, admin bool) ([]ApplicationView, error) {
	if userID == "" {
		return nil, kernel.Unauthorized("请先登录")
	}
	apps, err := s.repo.PluginApplications()
	if err != nil {
		return nil, err
	}
	result := []ApplicationView{}
	if len(apps) > 200 {
		return nil, issue(409, "operation_unavailable", "目录超过当前上限")
	}
	for _, app := range apps {
		if !admin && !app.Installed {
			continue
		}
		item, err := s.applicationView(userID, app)
		if err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, nil
}
func (s *Service) applicationView(userID string, app model.PluginApplication) (ApplicationView, error) {
	item := ApplicationView{PluginApplication: app, Releases: []ReleaseView{}, State: StateView{GrantedPermissions: []string{}, Reason: "plugin_disabled"}}
	state, err := s.repo.UserPluginState(userID, app.ID)
	if err != nil {
		return item, err
	}
	if state != nil {
		item.State.Enabled = state.Enabled
		item.State.InstalledReleaseID = state.InstalledReleaseID
		item.State.Revision = state.Revision
		if state.GrantedPermissionsJSON != "" {
			if err = json.Unmarshal([]byte(state.GrantedPermissionsJSON), &item.State.GrantedPermissions); err != nil {
				return item, err
			}
		}
	}
	releases, err := s.repo.PluginReleases(app.ID)
	if err != nil {
		return item, err
	}
	for _, r := range releases {
		release := ReleaseView{ID: r.ID, Version: r.Version, Digest: r.Digest, Revoked: r.Revoked, Operations: []OperationView{}, DependencyLock: []string{}}
		if err = json.Unmarshal([]byte(r.ManifestJSON), &release.Manifest); err != nil {
			return item, err
		}
		if err = json.Unmarshal([]byte(r.DependencyLockJSON), &release.DependencyLock); err != nil {
			return item, err
		}
		release.Skills, err = s.repo.PluginSkillBindings(r.ID)
		if err != nil {
			return item, err
		}
		if release.Skills == nil {
			release.Skills = []model.PluginSkillBinding{}
		}
		reason := "plugin_disabled"
		if state != nil && state.Enabled && state.InstalledReleaseID == r.ID {
			err = checkRelease(s.repo, userID, &r)
			if err == nil {
				reason = "operation_unavailable"
				item.State.EffectiveEnabled = true
				item.State.Reason = ""
			} else {
				var appErr *kernel.AppError
				if !errors.As(err, &appErr) {
					return item, err
				}
				reason = string(appErr.Reason)
				item.State.Reason = reason
			}
		}
		if r.Revoked {
			reason = "plugin_revoked"
		}
		ops := map[string]contracts.Operation{}
		if err = json.Unmarshal([]byte(r.OperationsJSON), &ops); err != nil {
			return item, err
		}
		keys := []string{}
		for key := range ops {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		grants := map[string]bool{}
		for _, p := range item.State.GrantedPermissions {
			grants[p] = true
		}
		for _, key := range keys {
			op := ops[key]
			opReason := reason
			available := false
			if opReason == "operation_unavailable" {
				for _, p := range op.RequiredPermissions {
					if !grants[p] {
						opReason = "scope_forbidden"
					}
				}
				if opReason == "operation_unavailable" && s.enabled {
					if _, adapterErr := s.adapterFor(op); adapterErr == nil {
						opReason = ""
						available = true
					}
				}
			}
			release.Operations = append(release.Operations, OperationView{Operation: op, Available: available, Reason: opReason})
		}
		item.Releases = append(item.Releases, release)
	}
	return item, nil
}
