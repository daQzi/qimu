package model

import "time"

// The cursor is orchestration state only. Heavy work belongs to child Run/Task.
type PluginPipelineExecution struct {
	RunID          string `gorm:"primaryKey;size:36"`
	Cursor         int
	ChildRunID     string `gorm:"size:36;index"`
	OutputsJSON    string `gorm:"type:text;not null"`
	BatchJSON      string `gorm:"type:text"`
	LeaseOwner     string `gorm:"size:36"`
	LeaseExpiresAt *time.Time
	UpdatedAt      time.Time
}

// An exact, expiring authorization receipt, not a second wallet.
type PluginBatchApproval struct {
	ID                 string    `json:"id" gorm:"primaryKey;size:36"`
	RunID              string    `json:"runId" gorm:"size:36;index;not null"`
	UserID             string    `json:"-" gorm:"size:36;not null"`
	Digest             string    `json:"digest" gorm:"size:64;not null;uniqueIndex:idx_plugin_batch_digest,priority:2"`
	ScopeJSON          string    `json:"-" gorm:"type:text;not null"`
	AmountMicrocredits int64     `json:"amountMicrocredits"`
	Count              int       `json:"count"`
	ExpiresAt          time.Time `json:"expiresAt"`
	CreatedAt          time.Time `json:"createdAt"`
}

// Slot validity follows the Task lease, including worker heartbeat renewal.
type PluginExecutionSlot struct {
	TaskID       string `gorm:"primaryKey;size:36"`
	Owner        string `gorm:"size:80;not null"`
	UserID       string `gorm:"size:36;index"`
	ConnectionID string `gorm:"size:36;index"`
	PluginID     string `gorm:"size:80;index"`
	ModelKey     string `gorm:"size:64;index"`
}

type PluginBatchMetric struct {
	UserID            string `gorm:"primaryKey;size:36"`
	ApprovalConflicts int64
	BudgetConflicts   int64
}

type PluginInputRequest struct {
	ID               string    `json:"id" gorm:"primaryKey;size:36"`
	RunID            string    `json:"runId" gorm:"size:36;not null;uniqueIndex:idx_plugin_input_step,priority:1"`
	StepKey          string    `json:"stepKey" gorm:"size:80;not null;uniqueIndex:idx_plugin_input_step,priority:2"`
	Revision         int64     `json:"revision"`
	Status           string    `json:"status" gorm:"size:24;not null"`
	SchemaJSON       string    `json:"-" gorm:"type:text;not null"`
	DraftJSON        string    `json:"-" gorm:"type:text"`
	SubmittedJSON    string    `json:"-" gorm:"type:text"`
	SubmissionKey    string    `json:"-" gorm:"size:160"`
	SubmissionDigest string    `json:"-" gorm:"size:64"`
	CreatedAt        time.Time `json:"createdAt"`
	UpdatedAt        time.Time `json:"updatedAt"`
}
