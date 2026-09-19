package plugins

import (
	"encoding/json"
	"infinite-canvas/backend/internal/plugins/contracts"
	"infinite-canvas/backend/internal/repository"
)

type PreparedOperation struct {
	Result           json.RawMessage
	SourceResourceID string
	SourceDigest     string
}

// ShortHostAdapter prepares bounded local data in the caller's transaction.
// Remote/long-running adapters must use Task executors in a later phase.
type ShortHostAdapter struct {
	ID          string
	Permissions []string
	Effects     []string
	Prepare     func(*repository.Repository, string, map[string]json.RawMessage, contracts.InvocationContext) (PreparedOperation, error)
}

func (s *Service) WithAdapters(adapters []ShortHostAdapter) *Service {
	s.adapters = map[string]ShortHostAdapter{}
	for _, adapter := range adapters {
		s.adapters[adapter.ID] = adapter
	}
	return s
}

type InvocationPolicy struct {
	PermissionMode string
	AgentRunID     string
	AgentRevision  int64
}
type OperationDescription struct {
	Address      string                     `json:"operation"`
	ReleaseID    string                     `json:"releaseId"`
	ContractHash string                     `json:"contractHash"`
	Definition   contracts.Operation        `json:"definition"`
	Schemas      map[string]json.RawMessage `json:"schemas"`
	Available    bool                       `json:"available"`
	Reason       string                     `json:"reason,omitempty"`
}
type OperationSummary struct {
	Operation   string   `json:"operation"`
	ReleaseID   string   `json:"releaseId"`
	Description string   `json:"description"`
	Effects     []string `json:"effects"`
}
