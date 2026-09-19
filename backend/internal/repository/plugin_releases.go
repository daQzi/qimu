package repository

import (
	"errors"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"infinite-canvas/backend/internal/model"
	"time"
)

var ErrPluginNamespaceConflict = errors.New("plugin namespace belongs to another registry")

func (r *Repository) ClaimPluginNamespace(id, kind string) error {
	if err := r.db.Clauses(clause.OnConflict{DoNothing: true}).Create(&model.PluginNamespace{ID: id, Kind: kind}).Error; err != nil {
		return err
	}
	var claim model.PluginNamespace
	if err := r.db.First(&claim, "id=?", id).Error; err != nil {
		return err
	}
	if claim.Kind != kind {
		return ErrPluginNamespaceConflict
	}
	return nil
}

// WithPluginCatalog serializes publication, grants and revocation, including
// dependency checks, across processes without holding a network request open.
func (r *Repository) WithPluginCatalog(fn func(*Repository) error) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&model.PluginCatalogLock{ID: 1}).Error; err != nil {
			return err
		}
		if err := tx.Model(&model.PluginCatalogLock{}).Where("id = ?", 1).UpdateColumn("revision", gorm.Expr("revision + 1")).Error; err != nil {
			return err
		}
		return fn(New(tx))
	})
}
func (r *Repository) PluginApplications() ([]model.PluginApplication, error) {
	var rows []model.PluginApplication
	err := r.db.Order("id").Limit(201).Find(&rows).Error
	return rows, err
}
func (r *Repository) PluginApplication(id string) (*model.PluginApplication, error) {
	var row model.PluginApplication
	err := r.db.First(&row, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &row, err
}
func (r *Repository) SavePluginApplication(row *model.PluginApplication) error {
	return r.db.Save(row).Error
}
func (r *Repository) PluginReleases(id string) ([]model.PluginRelease, error) {
	var rows []model.PluginRelease
	err := r.db.Where("plugin_id = ?", id).Order("created_at desc, id desc").Find(&rows).Error
	return rows, err
}
func (r *Repository) PluginRelease(id string) (*model.PluginRelease, error) {
	var row model.PluginRelease
	err := r.db.First(&row, "id = ?", id).Error
	return &row, err
}
func (r *Repository) PluginReleaseByVersion(id, version string) (*model.PluginRelease, error) {
	var row model.PluginRelease
	err := r.db.First(&row, "plugin_id = ? AND version = ?", id, version).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &row, err
}
func (r *Repository) CreatePluginRelease(row *model.PluginRelease, deps []model.PluginReleaseDependency) error {
	if err := r.db.Create(row).Error; err != nil {
		return err
	}
	if len(deps) > 0 {
		return r.db.Create(&deps).Error
	}
	return nil
}
func (r *Repository) RevokePluginRelease(id string) error {
	return r.db.Model(&model.PluginRelease{}).Where("id = ?", id).Update("revoked", true).Error
}
func (r *Repository) PluginSkillBindings(releaseID string) ([]model.PluginSkillBinding, error) {
	var rows []model.PluginSkillBinding
	err := r.db.Where("release_id = ?", releaseID).Find(&rows).Error
	return rows, err
}
func (r *Repository) CreatePluginSkill(binding *model.PluginSkillBinding, skill *model.Skill, version *model.SkillVersion, files []model.SkillFile) error {
	for _, value := range []any{skill, version, binding} {
		if err := r.db.Create(value).Error; err != nil {
			return err
		}
	}
	if len(files) > 0 {
		return r.db.Create(&files).Error
	}
	return nil
}
func (r *Repository) SaveApplicationUserState(state *model.UserPluginState, skills map[string]string) error {
	// Changing dependency grants/version invalidates dependent activations. A
	// subsequent explicit activation recomputes each skill's required grants.
	if err := r.db.Model(&model.UserPluginState{}).Where("user_id=? AND plugin_id<>? AND enabled=? AND installed_release_id IN (SELECT d.release_id FROM plugin_release_dependencies d JOIN plugin_releases pr ON pr.id=d.dependency_release_id WHERE pr.plugin_id=?)", state.UserID, state.PluginID, true, state.PluginID).Updates(map[string]any{"enabled": false, "revision": gorm.Expr("revision + 1"), "updated_at": time.Now()}).Error; err != nil {
		return err
	}
	if err := r.db.Save(state).Error; err != nil {
		return err
	}
	if err := r.db.Model(&model.UserSkillState{}).Where("user_id = ? AND skill_id IN (SELECT b.skill_id FROM plugin_skill_bindings b JOIN plugin_releases r ON r.id=b.release_id WHERE r.plugin_id=?)", state.UserID, state.PluginID).Updates(map[string]any{"added": false, "updated_at": time.Now()}).Error; err != nil {
		return err
	}
	for id, version := range skills {
		row := model.UserSkillState{ID: uuid.NewString(), UserID: state.UserID, SkillID: id, InstalledVersionID: version, Added: true, AutoUpdate: false}
		if err := r.SetUserSkillAdded(&row); err != nil {
			return err
		}
	}
	return nil
}

func (r *Repository) ApplicationPackageReferenced(key string) (bool, error) {
	var n int64
	err := r.db.Model(&model.PluginRelease{}).Where("package_key=?", key).Count(&n).Error
	return n > 0, err
}
func (r *Repository) PluginReleaseDependencies(id string) ([]model.PluginReleaseDependency, error) {
	var rows []model.PluginReleaseDependency
	err := r.db.Where("release_id = ?", id).Find(&rows).Error
	return rows, err
}

// Query boundary shared by discovery and file reads. Dependency rows contain
// the transitive, immutable required-release lock, not current latest versions.
func (r *Repository) usablePluginSkillIDs(userID string) *gorm.DB {
	return r.db.Table("plugin_skill_bindings b").Select("b.skill_id").
		Joins("JOIN plugin_releases pr ON pr.id=b.release_id").
		Joins("JOIN plugin_applications pa ON pa.id=pr.plugin_id").
		Joins("JOIN user_plugin_states ups ON ups.plugin_id=pa.id AND ups.user_id=?", userID).
		Joins("JOIN user_skill_states uss ON uss.skill_id=b.skill_id AND uss.user_id=?", userID).
		Where("pa.installed=? AND pa.available=? AND pr.revoked=? AND ups.enabled=? AND ups.installed_release_id=pr.id AND uss.added=?", true, true, false, true, true).
		Where(`NOT EXISTS (SELECT 1 FROM plugin_release_dependencies d
 LEFT JOIN plugin_releases dr ON dr.id=d.dependency_release_id
 LEFT JOIN plugin_applications da ON da.id=dr.plugin_id
 LEFT JOIN user_plugin_states du ON du.plugin_id=da.id AND du.user_id=?
 WHERE d.release_id=pr.id AND (dr.id IS NULL OR da.id IS NULL OR dr.revoked=? OR da.installed=? OR da.available=? OR du.id IS NULL OR du.enabled=? OR du.installed_release_id IS NULL OR du.installed_release_id<>dr.id))`, userID, true, false, false, false)
}
func (r *Repository) PluginSkillUsable(userID, skillID string) (bool, error) {
	var n int64
	err := r.usablePluginSkillIDs(userID).Where("b.skill_id=?", skillID).Count(&n).Error
	return n > 0, err
}
