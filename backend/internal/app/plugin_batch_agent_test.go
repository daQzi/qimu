package app

import (
	"encoding/json"
	"infinite-canvas/backend/internal/model"
	"infinite-canvas/backend/internal/plugins"
	"testing"
)

func TestP06AgentDerivationUsesLeaseAndCurrentBudgetOwner(t *testing.T) {
	s, db, release, _ := p05Fixture(t, "sqlite", nil, "batch")
	view := p06Start(t, s, release, `[{"id":"a","prompt":"a","enabled":true}]`)
	view = p06Finish(t, s, db, view.ID)
	agent := model.CloudAgentExecution{ID: "derive-agent", UserID: "user", Status: "running", Revision: 2}
	if err := db.Create(&agent).Error; err != nil {
		t.Fatal(err)
	}
	state := cloudAgentRuntime{}
	state.Request.PermissionMode = "request_approval"
	var call cloudAgentCall
	call.ID = "derive-call"
	call.Function.Name = "plugin_run_derive"
	raw, _ := json.Marshal(map[string]any{"runId": view.ID, "inputs": map[string]any{}, "forceSteps": []string{"remote"}})
	call.Function.Arguments = string(raw)
	stale := agent
	stale.Revision = 1
	if _, err := s.executeCloudAgentPluginTool(&stale, &state, call); err == nil {
		t.Fatal("stale agent derived")
	}
	value, err := s.executeCloudAgentPluginTool(&agent, &state, call)
	if err != nil {
		t.Fatal(err)
	}
	derived := value.(plugins.RunView)
	stored, err := s.PluginRun("user", derived.ID)
	if err != nil || stored.AgentRunID != agent.ID || stored.Pipeline.Batch.ReuseCompleted {
		t.Fatalf("budget owner/default cache %+v %v", stored.PluginRun, err)
	}
	again, err := s.executeCloudAgentPluginTool(&agent, &state, call)
	if err != nil || again.(plugins.RunView).ID != derived.ID {
		t.Fatal("agent replay", err)
	}
	call.ID = "second-derive"
	if _, err = s.executeCloudAgentPluginTool(&agent, &state, call); err == nil {
		t.Fatal("agent replaced an active pending execution")
	}
	state.Request.PermissionMode = "read_only"
	call.ID = "readonly"
	if _, err = s.executeCloudAgentPluginTool(&agent, &state, call); err == nil {
		t.Fatal("read-only agent derived")
	}
	if !cloudAgentWrite("plugin_run_derive") {
		t.Fatal("derive must be classified as mutation")
	}
}
