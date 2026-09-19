package model

import "time"

// PluginPlatformState stores the administrator-controlled availability of a
// plugin. User activation can never make an unavailable plugin effective.
type PluginPlatformState struct {
	PluginID  string    `json:"pluginId" gorm:"primaryKey;size:120"`
	Available bool      `json:"available" gorm:"index"`
	UpdatedBy string    `json:"updatedBy" gorm:"index;size:36"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// UserPluginState stores one user's activation choice for an application
// plugin. Legacy system-scoped protocol plugins do not use this table;
// uploaded v3 applications pin their release and grants here.
type UserPluginState struct {
	ID                     string    `json:"id" gorm:"primaryKey;size:36"`
	UserID                 string    `json:"userId" gorm:"size:36;index;uniqueIndex:idx_user_plugin_state_user_plugin,priority:1"`
	PluginID               string    `json:"pluginId" gorm:"size:120;index;uniqueIndex:idx_user_plugin_state_user_plugin,priority:2"`
	Enabled                bool      `json:"enabled" gorm:"index"`
	InstalledReleaseID     string    `json:"installedReleaseId,omitempty" gorm:"size:36;index"`
	GrantedPermissionsJSON string    `json:"-" gorm:"type:text"`
	Revision               int64     `json:"revision" gorm:"not null;default:0"`
	CreatedAt              time.Time `json:"createdAt"`
	UpdatedAt              time.Time `json:"updatedAt"`
}

// PluginApplication owns the stable ID and publication policy; releases below
// are immutable. Uninstalling retains this identity and every historical row.
type PluginApplication struct {
	ID          string    `json:"id" gorm:"primaryKey;size:120"`
	PublisherID string    `json:"publisherId" gorm:"size:80;not null"`
	Installed   bool      `json:"installed" gorm:"not null"`
	Available   bool      `json:"available" gorm:"not null"`
	Revision    int64     `json:"revision" gorm:"not null"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}
type PluginRelease struct {
	ID                 string    `json:"id" gorm:"primaryKey;size:36"`
	PluginID           string    `json:"pluginId" gorm:"size:120;not null;uniqueIndex:idx_plugin_release_version,priority:1"`
	Version            string    `json:"version" gorm:"size:40;not null;uniqueIndex:idx_plugin_release_version,priority:2"`
	Digest             string    `json:"digest" gorm:"size:64;not null"`
	ManifestJSON       string    `json:"-" gorm:"type:text;not null"`
	OperationsJSON     string    `json:"-" gorm:"type:text;not null"`
	DependencyLockJSON string    `json:"-" gorm:"type:text;not null"`
	PackageKey         string    `json:"-" gorm:"size:160;not null"`
	InstalledBy        string    `json:"installedBy" gorm:"size:36"`
	Revoked            bool      `json:"revoked" gorm:"not null"`
	CreatedAt          time.Time `json:"createdAt"`
}
type PluginSkillBinding struct {
	ID             string `json:"id" gorm:"primaryKey;size:36"`
	ReleaseID      string `json:"releaseId" gorm:"size:36;not null;uniqueIndex:idx_plugin_skill_local,priority:1"`
	LocalSkillID   string `json:"localSkillId" gorm:"size:80;not null;uniqueIndex:idx_plugin_skill_local,priority:2"`
	SkillID        string `json:"skillId" gorm:"size:36;not null;uniqueIndex"`
	SkillVersionID string `json:"skillVersionId" gorm:"size:36;not null;uniqueIndex"`
}

// A single transactional row serializes catalog mutation on SQLite/PostgreSQL.
type PluginCatalogLock struct {
	ID       int   `gorm:"primaryKey"`
	Revision int64 `gorm:"not null"`
}

type PluginReleaseDependency struct {
	ReleaseID           string `gorm:"primaryKey;size:36"`
	DependencyReleaseID string `gorm:"primaryKey;size:36;index"`
}

// PluginNamespace prevents concurrent legacy/v3 uploads claiming the same ID.
// Claims are retained on uninstall; reclaiming identities is not automatic.
type PluginNamespace struct {
	ID   string `gorm:"primaryKey;size:120"`
	Kind string `gorm:"size:24;not null"`
}
