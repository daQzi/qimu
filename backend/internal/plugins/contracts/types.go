package contracts

import "encoding/json"

// These wire types are inert contracts, not GORM models or execution authority.
// Validate raw input before decoding a type: encoding/json alone accepts
// unknown fields, duplicate keys and values outside the supported profile.
type Contribution struct {
	ID  string `json:"id"`
	Ref string `json:"ref"`
}
type Skill struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Entry       string   `json:"entry"`
	Activation  string   `json:"activation"`
	Operations  []string `json:"operations"`
}
type Dependency struct {
	ID       string `json:"id"`
	Version  string `json:"version"`
	Optional bool   `json:"optional"`
}
type Manifest struct {
	APIVersion  string `json:"apiVersion"`
	ID          string `json:"id"`
	Name        string `json:"name"`
	Version     string `json:"version"`
	Description string `json:"description"`
	Publisher   struct {
		ID          string `json:"id"`
		DisplayName string `json:"displayName"`
	} `json:"publisher"`
	Requires struct {
		HostAPI string `json:"hostApi"`
	} `json:"requires"`
	Permissions  []string     `json:"permissions"`
	Dependencies []Dependency `json:"dependencies"`
	Contributes  struct {
		Skills           []Skill        `json:"skills,omitempty"`
		Operations       []Contribution `json:"operations,omitempty"`
		Views            []Contribution `json:"views,omitempty"`
		CanvasBlueprints []Contribution `json:"canvasBlueprints,omitempty"`
	} `json:"contributes"`
}
type Execution struct {
	Kind      string `json:"kind"`
	Mode      string `json:"mode"`
	Adapter   string `json:"adapter,omitempty"`
	Connector string `json:"connector,omitempty"`
	Action    string `json:"action,omitempty"`
	Pipeline  string `json:"pipeline,omitempty"`
}
type Operation struct {
	ID                  string   `json:"id"`
	Description         string   `json:"description"`
	InputSchemaRef      string   `json:"inputSchemaRef"`
	OutputSchemaRef     string   `json:"outputSchemaRef"`
	RequiredPermissions []string `json:"requiredPermissions"`
	Effects             []string `json:"effects"`
	Context             struct {
		RequiresCanvas  bool `json:"requiresCanvas"`
		RequiresProject bool `json:"requiresProject"`
	} `json:"context"`
	Execution  Execution `json:"execution"`
	ResultView string    `json:"resultView,omitempty"`
}
type InvocationContext struct {
	HostSurface string `json:"hostSurface,omitempty"`
	CanvasID    string `json:"canvasId,omitempty"`
	ProjectID   string `json:"projectId,omitempty"`
	ThreadID    string `json:"threadId,omitempty"`
	WorkbenchID string `json:"workbenchId,omitempty"`
}
type Invocation struct {
	Operation string                     `json:"operation"`
	ReleaseID string                     `json:"releaseId"`
	Input     map[string]json.RawMessage `json:"input"`
	Context   *InvocationContext         `json:"context,omitempty"`
}
type ResultRef struct {
	RunID         string `json:"runId"`
	StepKey       string `json:"stepKey"`
	ItemKey       string `json:"itemKey"`
	Attempt       int    `json:"attempt"`
	OutputKey     string `json:"outputKey"`
	SchemaID      string `json:"schemaId"`
	SchemaVersion string `json:"schemaVersion"`
	Digest        string `json:"digest"`
	ResourceID    string `json:"resourceId,omitempty"`
}

type ResultView struct {
	ID        string `json:"id"`
	Component string `json:"component"`
	SchemaRef string `json:"schemaRef"`
	Fields    []struct {
		Path  string `json:"path"`
		Label string `json:"label"`
	} `json:"fields"`
}

type CanvasBlueprint struct {
	ID    string `json:"id"`
	Nodes []struct {
		Key      string `json:"key"`
		NodeType string `json:"nodeType"`
		Title    string `json:"title"`
		Position struct {
			X float64 `json:"x"`
			Y float64 `json:"y"`
		} `json:"position"`
		Binding string `json:"binding"`
		View    string `json:"view"`
	} `json:"nodes"`
}
type SkillBinding struct {
	ReleaseID      string `json:"releaseId"`
	LocalSkillID   string `json:"localSkillId"`
	SkillVersionID string `json:"skillVersionId"`
	EntryDigest    string `json:"entryDigest"`
}
type RunRecord struct {
	ID             string            `json:"id"`
	UserID         string            `json:"userId"`
	EntryOperation string            `json:"entryOperation"`
	ReleaseID      string            `json:"releaseId"`
	IdempotencyKey string            `json:"idempotencyKey"`
	RequestDigest  string            `json:"requestDigest"`
	Status         string            `json:"status"`
	Revision       int64             `json:"revision"`
	Context        InvocationContext `json:"context"`
	ParentRunID    string            `json:"parentRunId,omitempty"`
}
type Binding struct {
	// RawMessage preserves literal null versus an absent literal.
	Literal json.RawMessage `json:"literal,omitempty"`
	From    string          `json:"from,omitempty"`
}
type PipelineStep struct {
	Key           string             `json:"key"`
	Type          string             `json:"type"`
	Operation     string             `json:"operation,omitempty"`
	DependsOn     []string           `json:"dependsOn"`
	Inputs        map[string]Binding `json:"inputs,omitempty"`
	FormSchemaRef string             `json:"formSchemaRef,omitempty"`
	View          string             `json:"view,omitempty"`
	When          json.RawMessage    `json:"when,omitempty"`
	Foreach       json.RawMessage    `json:"foreach,omitempty"`
}
type Pipeline struct {
	ID              string             `json:"id"`
	InputSchemaRef  string             `json:"inputSchemaRef"`
	OutputSchemaRef string             `json:"outputSchemaRef"`
	Steps           []PipelineStep     `json:"steps"`
	Outputs         map[string]Binding `json:"outputs"`
}

type StepRecord struct {
	ID              string      `json:"id"`
	RunID           string      `json:"runId"`
	StepKey         string      `json:"stepKey"`
	ItemKey         string      `json:"itemKey"`
	Attempt         int         `json:"attempt"`
	Status          string      `json:"status"`
	InputDigest     string      `json:"inputDigest"`
	TaskID          string      `json:"taskId,omitempty"`
	SubmissionState string      `json:"submissionState"`
	OutputRefs      []ResultRef `json:"outputRefs"`
}

type InvocationResult struct {
	Digest string          `json:"digest,omitempty"`
	Kind   string          `json:"kind"`
	Result json.RawMessage `json:"result,omitempty"`
	RunID  string          `json:"runId,omitempty"`
	Status string          `json:"status,omitempty"`
}
