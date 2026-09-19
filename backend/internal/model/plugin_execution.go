package model

import "time"

// P02 runs hold one short host operation. They are not a second task queue.
type PluginRun struct {
	ID               string    `json:"id" gorm:"primaryKey;size:36"`
	UserID           string    `json:"-" gorm:"size:36;not null;uniqueIndex:idx_plugin_run_key,priority:1"`
	IdempotencyKey   string    `json:"-" gorm:"size:160;not null;uniqueIndex:idx_plugin_run_key,priority:2"`
	RequestDigest    string    `json:"-" gorm:"size:64;not null"`
	ReleaseID        string    `json:"releaseId" gorm:"size:36;not null;index"`
	ReleaseVersion   string    `json:"releaseVersion" gorm:"size:40;not null"`
	AgentRunID       string    `json:"agentRunId,omitempty" gorm:"size:36;index"`
	Operation        string    `json:"operation" gorm:"size:161;not null"`
	ContractHash     string    `json:"contractHash" gorm:"size:64;not null"`
	RequestJSON      string    `json:"-" gorm:"type:text;not null"`
	PlanJSON         string    `json:"-" gorm:"type:text;not null"`
	ResultJSON       string    `json:"-" gorm:"type:text"`
	SourceResourceID string    `json:"sourceResourceId,omitempty" gorm:"size:80;index"`
	SourceDigest     string    `json:"-" gorm:"size:64"`
	Status           string    `json:"status" gorm:"size:32;not null;index"`
	Revision         int64     `json:"revision" gorm:"not null"`
	EventSequence    int64     `json:"eventSequence" gorm:"not null"`
	ApprovalID       string    `json:"approvalId,omitempty" gorm:"size:36"`
	ApprovalDecision string    `json:"approvalDecision,omitempty" gorm:"size:16"`
	ProjectionStatus string    `json:"projectionStatus,omitempty" gorm:"size:24"`
	FailureMessage   string    `json:"failureMessage,omitempty" gorm:"type:text"`
	CreatedAt        time.Time `json:"createdAt"`
	UpdatedAt        time.Time `json:"updatedAt"`
}

// One immutable binding per canvas/result/template/instance; node content remains user-owned.
type PluginCanvasProjection struct {
	ID           string `gorm:"primaryKey;size:64"`
	UserID       string `gorm:"size:36;not null;index"`
	CanvasID     string `gorm:"size:80;not null;index"`
	SourceRunID  string `gorm:"size:36;not null;index"`
	ReleaseID    string `gorm:"size:36;not null"`
	BlueprintID  string `gorm:"size:80;not null"`
	BindingsJSON string `gorm:"type:text;not null"`
	NodesJSON    string `gorm:"type:text;not null"`
	ResultJSON   string `gorm:"type:text;not null"`
	CreatedAt    time.Time
}
type PluginRunStep struct {
	ID          string  `gorm:"primaryKey;size:36"`
	RunID       string  `gorm:"size:36;not null;uniqueIndex:idx_plugin_run_step,priority:1"`
	StepKey     string  `gorm:"size:80;not null;uniqueIndex:idx_plugin_run_step,priority:2"`
	ItemKey     string  `gorm:"size:120;not null;uniqueIndex:idx_plugin_run_step,priority:3"`
	Attempt     int     `gorm:"not null;uniqueIndex:idx_plugin_run_step,priority:4"`
	Status      string  `gorm:"size:32;not null"`
	InputDigest string  `gorm:"size:64;not null"`
	TaskID      *string `gorm:"size:36;uniqueIndex"`
	CreatedAt   time.Time
	UpdatedAt   time.Time
}
type PluginRunEvent struct {
	ID          string    `json:"-" gorm:"primaryKey;size:36"`
	RunID       string    `json:"runId" gorm:"size:36;not null;uniqueIndex:idx_plugin_event_sequence,priority:1"`
	Sequence    int64     `json:"sequence" gorm:"not null;uniqueIndex:idx_plugin_event_sequence,priority:2"`
	Type        string    `json:"type" gorm:"size:64;not null"`
	PayloadJSON string    `json:"-" gorm:"type:text;not null"`
	CreatedAt   time.Time `json:"createdAt"`
}
