package model

import "time"

const TaskTypePluginOperation = "plugin_operation"

type PluginConnection struct {
	ID          string    `json:"id" gorm:"primaryKey;size:36"`
	UserID      string    `json:"-" gorm:"size:36;not null;uniqueIndex:idx_plugin_connection_scope,priority:1"`
	PluginID    string    `json:"pluginId" gorm:"size:80;not null;uniqueIndex:idx_plugin_connection_scope,priority:2"`
	ConnectorID string    `json:"connectorId" gorm:"size:80;not null;uniqueIndex:idx_plugin_connection_scope,priority:3"`
	Name        string    `json:"name" gorm:"size:120"`
	VersionID   string    `json:"-" gorm:"size:36"`
	Revision    int64     `json:"revision"`
	Enabled     bool      `json:"enabled"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

// Configuration is immutable. In-flight tasks keep their exact version.
type PluginConnectionVersion struct {
	ID           string `gorm:"primaryKey;size:36"`
	ConnectionID string `gorm:"size:36;not null;uniqueIndex:idx_plugin_connection_version,priority:1"`
	Revision     int64  `gorm:"not null;uniqueIndex:idx_plugin_connection_version,priority:2"`
	UserID       string `gorm:"size:36;not null;index"`
	BaseURL      string `gorm:"type:text;not null"`
	SecretCipher string `gorm:"type:text"`
	AuthType     string `gorm:"size:16"`
	CreatedAt    time.Time
}

type PluginConnectionRate struct {
	ID            string `gorm:"primaryKey;size:36"`
	NextRequestAt time.Time
}

// Only administrators can set a fixed platform service fee; vendor BYOK costs
// remain external and are never reported as platform credits.
type PluginOperationPrice struct {
	ID              string    `json:"id" gorm:"primaryKey;size:64"`
	ReleaseID       string    `json:"releaseId" gorm:"size:36;not null;uniqueIndex:idx_plugin_operation_price,priority:1"`
	OperationID     string    `json:"operationId" gorm:"size:80;not null;uniqueIndex:idx_plugin_operation_price,priority:2"`
	FeeMicrocredits int64     `json:"feeMicrocredits"`
	Revision        int64     `json:"revision"`
	UpdatedAt       time.Time `json:"updatedAt"`
}

type PluginRemoteExecution struct {
	TaskID              string     `json:"taskId" gorm:"primaryKey;size:36"`
	RunID               string     `json:"runId" gorm:"size:36;not null;uniqueIndex"`
	UserID              string     `json:"-" gorm:"size:36;not null;index"`
	ConnectionVersionID string     `json:"-" gorm:"size:36;not null;index"`
	SubmissionKey       string     `json:"-" gorm:"size:80;not null;uniqueIndex"`
	Attempt             int        `json:"attempt" gorm:"not null"`
	State               string     `json:"submissionState" gorm:"size:32;not null"`
	PreparedCipher      string     `json:"-" gorm:"type:text"`
	RequestCipher       string     `json:"-" gorm:"type:text"`
	FirstSubmittedAt    *time.Time `json:"-"`
	ResponseCipher      string     `json:"-" gorm:"type:text"`
	ResponseDigest      string     `json:"-" gorm:"size:64"`
	ProviderJobID       string     `json:"providerJobId,omitempty" gorm:"size:160"`
	CancelRequested     bool       `json:"cancelRequested"`
	CancelSent          bool       `json:"-"`
	CancelStatus        string     `json:"cancelStatus,omitempty" gorm:"size:64"`
	FailureReason       string     `json:"failureReason,omitempty" gorm:"size:80"`
	FailureMessage      string     `json:"failureMessage,omitempty" gorm:"type:text"`
	PollFailures        int        `json:"-"`
	ImportAttempts      int        `json:"importAttempts"`
	CreatedAt           time.Time  `json:"createdAt"`
	UpdatedAt           time.Time  `json:"updatedAt"`
}

type PluginRunResource struct {
	RunID      string `gorm:"size:36;not null;primaryKey"`
	ResourceID string `gorm:"size:80;not null;primaryKey;index"`
	UserID     string `gorm:"size:36;not null;index"`
	Role       string `gorm:"size:16;not null"`
}
