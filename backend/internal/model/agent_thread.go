package model

import "time"

// A thread indexes immutable execution receipts; runs retain their own state and billing.
type AgentThread struct {
	ID              string    `json:"id" gorm:"primaryKey;size:80"`
	UserID          string    `json:"-" gorm:"index;size:36;uniqueIndex:idx_agent_thread_client,priority:1"`
	ClientKey       string    `json:"-" gorm:"size:128;uniqueIndex:idx_agent_thread_client,priority:2"`
	RequestHash     string    `json:"-" gorm:"size:64"`
	Title           string    `json:"title" gorm:"size:240"`
	CanvasID        string    `json:"canvasId,omitempty" gorm:"size:80;index"`
	Revision        int64     `json:"revision"`
	ContextRevision int64     `json:"contextRevision"`
	LastSequence    int64     `json:"lastSequence"`
	LastAgentRunID  string    `json:"lastAgentRunId,omitempty" gorm:"size:80"`
	CreatedAt       time.Time `json:"createdAt"`
	UpdatedAt       time.Time `json:"updatedAt" gorm:"index"`
}

type AgentThreadEntry struct {
	ThreadID        string    `json:"threadId" gorm:"primaryKey;size:80;uniqueIndex:idx_agent_thread_entry_key,priority:1"`
	Sequence        int64     `json:"sequence" gorm:"primaryKey;autoIncrement:false"`
	UserID          string    `json:"-" gorm:"index;size:36"`
	ClientKey       string    `json:"-" gorm:"size:128;uniqueIndex:idx_agent_thread_entry_key,priority:2"`
	RequestHash     string    `json:"-" gorm:"size:64"`
	Kind            string    `json:"kind" gorm:"size:24"`
	RunID           string    `json:"runId" gorm:"index;size:80"`
	Prompt          string    `json:"prompt" gorm:"type:text"`
	ContextRevision int64     `json:"contextRevision"`
	ContextJSON     string    `json:"-" gorm:"type:text"`
	CreatedAt       time.Time `json:"createdAt"`
}

type AgentThreadCanvas struct {
	ThreadID  string    `json:"threadId" gorm:"primaryKey;size:80"`
	CanvasID  string    `json:"canvasId" gorm:"primaryKey;size:80"`
	UserID    string    `json:"-" gorm:"index;size:36"`
	CreatedAt time.Time `json:"createdAt"`
}

// Keep the persisted table name independent of the inflector's treatment of "canvas".
func (AgentThreadCanvas) TableName() string { return "agent_thread_canvases" }
