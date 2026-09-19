package plugins

import (
	"encoding/json"
	"infinite-canvas/backend/internal/model"
	"infinite-canvas/backend/internal/plugins/contracts"
	"infinite-canvas/backend/internal/repository"
)

type PreparedOperation struct {
	Result           json.RawMessage
	SourceResourceID string
	SourceDigest     string
	ProjectsToCanvas bool
	// Commit runs only after explicit approval, in the run transaction.
	Commit func(*repository.Repository) error
}

type HostOperationContext struct {
	contracts.InvocationContext
	ReleaseID string
	Files     contracts.PackageFiles
}

// ShortHostAdapter prepares bounded local data in the caller's transaction.
// Remote operations use RemoteHost admission and the existing Task worker.
type ShortHostAdapter struct {
	ID          string
	Permissions []string
	Effects     []string
	Prepare     func(*repository.Repository, string, map[string]json.RawMessage, HostOperationContext) (PreparedOperation, error)
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

// Remote admission creates no network traffic. Task creation and reservation
// run in the same transaction as the approved PluginRun transition.
type PreparedRemoteOperation struct {
	Preview             json.RawMessage
	SourceDigest        string
	ConnectionVersionID string
	ResourceIDs         []string
	Enqueue             func(*repository.Repository, *model.PluginRun) error
}
type RemoteHost interface {
	Prepare(*repository.Repository, string, contracts.Invocation, HostOperationContext, InvocationPolicy) (PreparedRemoteOperation, error)
	Cancel(*repository.Repository, *model.PluginRun) error
	Resume(*repository.Repository, *model.PluginRun, RemoteResumeRequest) error
}
type RemoteResumeRequest struct {
	Action        string
	ProviderJobID string
}

func (s *Service) WithRemoteHost(host RemoteHost) *Service { s.remote = host; return s }

type OperationDescription struct {
	Address      string                     `json:"operation"`
	ReleaseID    string                     `json:"releaseId"`
	ContractHash string                     `json:"contractHash"`
	Definition   contracts.Operation        `json:"definition"`
	Schemas      map[string]json.RawMessage `json:"schemas"`
	Available    bool                       `json:"available"`
	ResultView   *contracts.ResultView      `json:"resultView,omitempty"`
	Reason       string                     `json:"reason,omitempty"`
}
type OperationSummary struct {
	Operation   string   `json:"operation"`
	ReleaseID   string   `json:"releaseId"`
	Description string   `json:"description"`
	Effects     []string `json:"effects"`
}
