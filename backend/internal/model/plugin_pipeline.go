package model

import "time"

// The cursor is orchestration state only. Heavy work belongs to child Run/Task.
type PluginPipelineExecution struct {
	RunID          string `gorm:"primaryKey;size:36"`
	Cursor         int
	ChildRunID     string `gorm:"size:36;index"`
	OutputsJSON    string `gorm:"type:text;not null"`
	LeaseOwner     string `gorm:"size:36"`
	LeaseExpiresAt *time.Time
	UpdatedAt      time.Time
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
