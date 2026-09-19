// Package plugins manages versioned application contributions. P01 installs
// metadata and skills only; operation execution is deliberately unavailable.
package plugins

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"gorm.io/gorm"
	"infinite-canvas/backend/internal/kernel"
	"infinite-canvas/backend/internal/model"
	"infinite-canvas/backend/internal/plugins/contracts"
	"infinite-canvas/backend/internal/protocol"
	"infinite-canvas/backend/internal/repository"
	"infinite-canvas/backend/internal/skills"
)

type Service struct {
	adapters map[string]ShortHostAdapter
	repo     *repository.Repository
	dataDir  string
	enabled  bool
}

func New(repo *repository.Repository, dataDir string, enabled bool) *Service {
	return &Service{repo: repo, dataDir: dataDir, enabled: enabled}
}
func issue(status int, reason, message string) error {
	return &kernel.AppError{Status: status, Code: status, Reason: kernel.ErrorReason(reason), Message: message}
}
func (s *Service) admission() error {
	if !s.enabled {
		return issue(403, "plugin_disabled", "应用插件安装和启用已由平台关闭")
	}
	return nil
}
func encode(v any) string { raw, _ := json.Marshal(v); return string(raw) }

type Activation struct {
	ReleaseID          string   `json:"releaseId"`
	Enabled            bool     `json:"enabled"`
	GrantedPermissions []string `json:"grantedPermissions"`
	Revision           int64    `json:"revision"`
}
type ManagementChange struct {
	Action    string `json:"action"`
	Available bool   `json:"available"`
	ReleaseID string `json:"releaseId"`
	Revision  int64  `json:"revision"`
}
type OperationView struct {
	contracts.Operation
	Available bool   `json:"available"`
	Reason    string `json:"reason"`
}
type ReleaseView struct {
	ID             string                     `json:"id"`
	Version        string                     `json:"version"`
	Digest         string                     `json:"digest"`
	Revoked        bool                       `json:"revoked"`
	Manifest       contracts.Manifest         `json:"manifest"`
	Skills         []model.PluginSkillBinding `json:"skills"`
	Operations     []OperationView            `json:"operations"`
	DependencyLock []string                   `json:"dependencyLock"`
}
type StateView struct {
	Enabled            bool     `json:"enabled"`
	EffectiveEnabled   bool     `json:"effectiveEnabled"`
	InstalledReleaseID string   `json:"installedReleaseId"`
	GrantedPermissions []string `json:"grantedPermissions"`
	Revision           int64    `json:"revision"`
	Reason             string   `json:"reason"`
}
type ApplicationView struct {
	model.PluginApplication
	Releases []ReleaseView `json:"releases"`
	State    StateView     `json:"state"`
}

func (s *Service) Install(actorID string, data []byte, reserved []string) (string, error) {
	if err := s.admission(); err != nil {
		return "", err
	}
	pkg, err := protocol.ReadPluginPackageEnvelope(data)
	if err != nil {
		return "", issue(400, "contract_invalid", err.Error())
	}
	if err = contracts.ValidatePackage(pkg.Files, contracts.Policy{ReservedIDs: reserved}); err != nil {
		return "", issue(400, "contract_invalid", err.Error())
	}
	var manifest contracts.Manifest
	if err = json.Unmarshal(pkg.ManifestRaw, &manifest); err != nil {
		return "", err
	}
	digest := contracts.PackageDigest(pkg.Files)
	preparedVersions := []model.SkillVersion{}
	err = s.repo.WithPluginCatalog(func(repo *repository.Repository) error {
		if err := repo.ClaimPluginNamespace(manifest.ID, "application"); err != nil {
			if errors.Is(err, repository.ErrPluginNamespaceConflict) {
				return issue(409, "scope_forbidden", "插件 ID 已由旧协议注册表管理")
			}
			return err
		}
		application, err := repo.PluginApplication(manifest.ID)
		if err != nil {
			return err
		}
		if application != nil && application.PublisherID != manifest.Publisher.ID {
			return issue(409, "scope_forbidden", "插件 ID 已归属其他发布者")
		}
		existing, err := repo.PluginReleaseByVersion(manifest.ID, manifest.Version)
		if err != nil {
			return err
		}
		if existing != nil {
			if existing.Digest != digest {
				return issue(409, "plugin_version_conflict", "同版本内容不同，请增加发布版本")
			}
			if existing.Revoked {
				return issue(409, "plugin_revoked", "该版本已撤回，不能重新安装")
			}
			if err := s.verifyPackage(*existing); err != nil {
				return err
			}
			if application != nil && !application.Installed {
				application.Installed = true
				application.Available = false
				application.Revision++
				return repo.SavePluginApplication(application)
			}
			return nil
		}
		all, err := repo.PluginApplications()
		if err != nil {
			return err
		}
		if application == nil && len(all) >= 200 {
			return issue(409, "operation_unavailable", "当前受控目录最多登记 200 个应用")
		}
		releases, err := repo.PluginReleases(manifest.ID)
		if err != nil {
			return err
		}
		if len(releases) >= 32 {
			return issue(409, "operation_unavailable", "单应用发布版本数量已达当前上限")
		}
		deps, err := resolveDependencies(repo, manifest)
		if err != nil {
			return err
		}
		if err = validateSkillDependencies(repo, manifest, deps); err != nil {
			return err
		}
		operations := map[string]contracts.Operation{}
		for _, c := range manifest.Contributes.Operations {
			var op contracts.Operation
			if err = json.Unmarshal(pkg.Files[c.Ref], &op); err != nil {
				return err
			}
			operations[op.ID] = op
		}
		releaseID := kernel.NewID()
		key := digest + ".yingce-plugin"
		if err := s.persistPackage(key, pkg.Files); err != nil {
			return err
		}
		if application == nil {
			application = &model.PluginApplication{ID: manifest.ID, PublisherID: manifest.Publisher.ID, Installed: true, Available: true, Revision: 1}
		} else {
			application.Installed = true
			application.Revision++
		}
		if err := repo.SavePluginApplication(application); err != nil {
			return err
		}
		release := model.PluginRelease{ID: releaseID, PluginID: manifest.ID, Version: manifest.Version, Digest: digest, ManifestJSON: string(pkg.ManifestRaw), OperationsJSON: encode(operations), DependencyLockJSON: encode(deps), PackageKey: key, InstalledBy: actorID}
		rows := []model.PluginReleaseDependency{}
		for _, id := range deps {
			rows = append(rows, model.PluginReleaseDependency{ReleaseID: releaseID, DependencyReleaseID: id})
		}
		if err := repo.CreatePluginRelease(&release, rows); err != nil {
			return err
		}
		skillService := skills.New(repo, s.dataDir, nil)
		for _, c := range manifest.Contributes.Skills {
			skillID, versionID := kernel.NewID(), kernel.NewID()
			prefix := path.Dir(c.Entry) + "/"
			files := map[string][]byte{}
			for name, raw := range pkg.Files {
				if strings.HasPrefix(name, prefix) {
					files[strings.TrimPrefix(name, prefix)] = raw
				}
			}
			skill, version, entries, err := skillService.PreparePluginSkill(skillID, versionID, c.Name, c.Description, manifest.Version, manifest.Publisher.DisplayName, files)
			if err != nil {
				return err
			}
			preparedVersions = append(preparedVersions, *version)
			binding := model.PluginSkillBinding{ID: kernel.NewID(), ReleaseID: releaseID, LocalSkillID: c.ID, SkillID: skillID, SkillVersionID: versionID}
			if err := repo.CreatePluginSkill(&binding, skill, version, entries); err != nil {
				return err
			}
		}
		return repo.AppendAdminAudit(&model.AdminAuditEvent{ID: kernel.NewID(), ActorUserID: actorID, Action: "application_plugin.install", TargetType: "plugin", TargetID: manifest.ID, Summary: "登记应用插件不可变发布", MetadataJSON: encode(map[string]string{"releaseId": releaseID, "digest": digest})})
	})
	if err != nil {
		for _, version := range preparedVersions {
			_, readErr := s.repo.SkillVersion(version.ID)
			if errors.Is(readErr, gorm.ErrRecordNotFound) {
				removeErr := os.Remove(filepath.Join(s.dataDir, "skill-packages", filepath.FromSlash(version.PackageKey)))
				if removeErr != nil && !os.IsNotExist(removeErr) {
					err = errors.Join(err, removeErr)
				}
			}
		}
	}
	return manifest.ID, err
}

func validateSkillDependencies(repo *repository.Repository, manifest contracts.Manifest, deps []string) error {
	operations := map[string]map[string]contracts.Operation{}
	for _, id := range deps {
		r, err := repo.PluginRelease(id)
		if err != nil {
			return err
		}
		ops := map[string]contracts.Operation{}
		if err = json.Unmarshal([]byte(r.OperationsJSON), &ops); err != nil {
			return err
		}
		operations[r.PluginID] = ops
	}
	for _, skill := range manifest.Contributes.Skills {
		for _, address := range skill.Operations {
			parts := strings.Split(address, ".")
			if parts[0] == manifest.ID {
				continue
			}
			if _, ok := operations[parts[0]][parts[1]]; !ok {
				return issue(400, "plugin_dependency_missing", "技能引用的固定依赖操作不存在")
			}
		}
	}
	return nil
}

func resolveDependencies(repo *repository.Repository, manifest contracts.Manifest) ([]string, error) {
	ids := []string{}
	seen := map[string]bool{}
	versions := map[string]string{}
	var visit func(*model.PluginRelease) error
	visit = func(r *model.PluginRelease) error {
		if seen[r.ID] {
			return nil
		}
		if locked, ok := versions[r.PluginID]; ok && locked != r.ID {
			return issue(409, "plugin_version_conflict", "依赖树要求同一插件的不同版本，无法同时启用")
		}
		versions[r.PluginID] = r.ID
		if r.PluginID == manifest.ID {
			return issue(400, "dependency_cycle", "插件依赖形成循环")
		}
		if len(seen) >= 128 {
			return issue(400, "contract_invalid", "依赖数量超过上限")
		}
		seen[r.ID] = true
		app, err := repo.PluginApplication(r.PluginID)
		if err != nil {
			return err
		}
		if app == nil || !app.Installed || !app.Available || r.Revoked {
			return issue(409, "plugin_dependency_missing", "依赖版本未安装或已停用")
		}
		ids = append(ids, r.ID)
		children, err := repo.PluginReleaseDependencies(r.ID)
		if err != nil {
			return err
		}
		for _, child := range children {
			next, err := repo.PluginRelease(child.DependencyReleaseID)
			if err != nil {
				return err
			}
			if err = visit(next); err != nil {
				return err
			}
		}
		return nil
	}
	for _, d := range manifest.Dependencies {
		if d.Optional {
			for _, skill := range manifest.Contributes.Skills {
				for _, op := range skill.Operations {
					if strings.HasPrefix(op, d.ID+".") {
						return nil, issue(400, "plugin_dependency_missing", "技能必需操作不能依赖可选插件")
					}
				}
			}
			continue
		}
		release, err := repo.PluginReleaseByVersion(d.ID, d.Version)
		if err != nil {
			return nil, err
		}
		if release == nil {
			return nil, issue(409, "plugin_dependency_missing", fmt.Sprintf("缺少依赖 %s@%s", d.ID, d.Version))
		}
		if err = visit(release); err != nil {
			return nil, err
		}
	}
	return ids, nil
}

func checkRelease(repo *repository.Repository, userID string, r *model.PluginRelease) error {
	app, err := repo.PluginApplication(r.PluginID)
	if err != nil {
		return err
	}
	if app == nil || !app.Installed || !app.Available {
		return issue(403, "plugin_disabled", "插件已停用或卸载")
	}
	if r.Revoked {
		return issue(403, "plugin_revoked", "插件版本已撤回")
	}
	deps, err := repo.PluginReleaseDependencies(r.ID)
	if err != nil {
		return err
	}
	for _, d := range deps {
		dep, err := repo.PluginRelease(d.DependencyReleaseID)
		if err != nil {
			return err
		}
		app, err := repo.PluginApplication(dep.PluginID)
		if err != nil {
			return err
		}
		state, err := repo.UserPluginState(userID, dep.PluginID)
		if err != nil {
			return err
		}
		if app == nil || !app.Installed || !app.Available || dep.Revoked || state == nil || !state.Enabled || state.InstalledReleaseID != dep.ID {
			return issue(409, "plugin_dependency_missing", "请先启用依赖的固定版本")
		}
	}
	return nil
}

func (s *Service) Activate(userID, pluginID string, req Activation) error {
	if userID == "" {
		return kernel.Unauthorized("请先登录")
	}
	if req.Enabled {
		if err := s.admission(); err != nil {
			return err
		}
	}
	return s.repo.WithPluginCatalog(func(repo *repository.Repository) error {
		r, err := repo.PluginRelease(req.ReleaseID)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return kernel.NotFound("发布版本不存在")
		}
		if err != nil {
			return err
		}
		if r.PluginID != pluginID {
			return issue(403, "scope_forbidden", "发布版本不属于该插件")
		}
		state, err := repo.UserPluginState(userID, pluginID)
		if err != nil {
			return err
		}
		if state == nil {
			state = &model.UserPluginState{ID: kernel.NewID(), UserID: userID, PluginID: pluginID}
		}
		if req.Revision != state.Revision {
			return issue(409, "run_revision_conflict", "插件状态已变化，请刷新后重试")
		}
		var manifest contracts.Manifest
		if err = json.Unmarshal([]byte(r.ManifestJSON), &manifest); err != nil {
			return err
		}
		allowed := map[string]bool{}
		for _, p := range manifest.Permissions {
			allowed[p] = true
		}
		grants := map[string]bool{}
		for _, p := range req.GrantedPermissions {
			if !allowed[p] || grants[p] {
				return issue(400, "scope_forbidden", "授权超出插件申请范围或重复")
			}
			grants[p] = true
		}
		selected := map[string]string{}
		if req.Enabled {
			if err = checkRelease(repo, userID, r); err != nil {
				return err
			}
			if err = s.verifyPackage(*r); err != nil {
				return err
			}
			bindings, err := repo.PluginSkillBindings(r.ID)
			if err != nil {
				return err
			}
			for _, binding := range bindings {
				spec := contracts.Skill{}
				for _, v := range manifest.Contributes.Skills {
					if v.ID == binding.LocalSkillID {
						spec = v
						break
					}
				}
				usable := true
				for _, address := range spec.Operations {
					parts := strings.Split(address, ".")
					ops := map[string]contracts.Operation{}
					source := r
					permissions := grants
					if parts[0] != pluginID {
						deps, err := repo.PluginReleaseDependencies(r.ID)
						if err != nil {
							return err
						}
						source = nil
						for _, d := range deps {
							candidate, e := repo.PluginRelease(d.DependencyReleaseID)
							if e != nil {
								return e
							}
							if candidate.PluginID == parts[0] {
								source = candidate
								break
							}
						}
						if source == nil {
							return issue(409, "plugin_dependency_missing", "技能引用缺少依赖")
						}
						depState, e := repo.UserPluginState(userID, source.PluginID)
						if e != nil {
							return e
						}
						permissions = map[string]bool{}
						if depState != nil {
							var values []string
							if e = json.Unmarshal([]byte(depState.GrantedPermissionsJSON), &values); e != nil {
								return e
							}
							for _, v := range values {
								permissions[v] = true
							}
						}
					}
					if err = json.Unmarshal([]byte(source.OperationsJSON), &ops); err != nil {
						return err
					}
					op, ok := ops[parts[1]]
					if !ok {
						return issue(409, "plugin_dependency_missing", "技能引用的依赖操作不存在")
					}
					for _, p := range op.RequiredPermissions {
						usable = usable && permissions[p]
					}
				}
				if usable {
					selected[binding.SkillID] = binding.SkillVersionID
				}
			}
		}
		state.Enabled = req.Enabled
		state.InstalledReleaseID = r.ID
		state.GrantedPermissionsJSON = encode(req.GrantedPermissions)
		state.Revision++
		state.UpdatedAt = time.Now()
		return repo.SaveApplicationUserState(state, selected)
	})
}

func (s *Service) Manage(actorID, id string, change ManagementChange) error {
	return s.repo.WithPluginCatalog(func(repo *repository.Repository) error {
		app, err := repo.PluginApplication(id)
		if err != nil {
			return err
		}
		if app == nil {
			return kernel.NotFound("应用插件不存在")
		}
		if change.Revision != app.Revision {
			return issue(409, "run_revision_conflict", "平台状态已变化，请刷新")
		}
		switch change.Action {
		case "availability":
			if !app.Installed {
				return issue(409, "plugin_disabled", "请先重新安装应用")
			}
			if change.Available {
				if err = s.admission(); err != nil {
					return err
				}
			}
			app.Available = change.Available
		case "uninstall":
			app.Installed = false
			app.Available = false
		case "revoke":
			r, e := repo.PluginRelease(change.ReleaseID)
			if e != nil {
				return e
			}
			if r.PluginID != id {
				return issue(403, "scope_forbidden", "版本不属于该插件")
			}
			if err = repo.RevokePluginRelease(r.ID); err != nil {
				return err
			}
		default:
			return issue(400, "contract_invalid", "未知管理操作")
		}
		app.Revision++
		if err = repo.SavePluginApplication(app); err != nil {
			return err
		}
		return repo.AppendAdminAudit(&model.AdminAuditEvent{ID: kernel.NewID(), ActorUserID: actorID, Action: "application_plugin." + change.Action, TargetType: "plugin", TargetID: id, Summary: "更新应用插件平台状态", MetadataJSON: encode(change)})
	})
}
