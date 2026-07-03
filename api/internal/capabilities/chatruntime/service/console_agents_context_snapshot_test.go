package service

import "testing"

func TestConsoleAgentsContextSnapshotStoredOutsideOperationContextMetadata(t *testing.T) {
	parts := contextualConsoleAgentsManageCapabilityPartsForTest()
	parts.RuntimeContext = "route=/console/agents/agent-1/agent capabilities=agent.update_config"

	metadata := streamingMessageMetadata(parts)
	if _, ok := metadata["operation_context"]; ok {
		t.Fatalf("operation_context leaked into message metadata: %#v", metadata["operation_context"])
	}
	snapshot := mapFromOperationContext(metadata[consoleAgentsContextSnapshotKey])
	if len(snapshot) == 0 {
		t.Fatalf("missing console agents context snapshot in metadata: %#v", metadata)
	}
	if got := stringMetadataValue(snapshot["schema"]); got != consoleAgentsContextSnapshotSchema {
		t.Fatalf("snapshot schema = %q, want %s", got, consoleAgentsContextSnapshotSchema)
	}
	if got := stringMetadataValue(snapshot["page"]); got != "console.agents" {
		t.Fatalf("snapshot page = %q, want console.agents", got)
	}
	if got := stringMetadataValue(snapshot["route"]); got != "/console/agents/agent-1/agent" {
		t.Fatalf("snapshot route = %q, want current Agent detail route", got)
	}
	agents := mapSliceFromAny(snapshot["visible_agents"])
	if len(agents) != 1 {
		t.Fatalf("snapshot visible_agents length = %d, want 1: %#v", len(agents), snapshot["visible_agents"])
	}
	if agents[0]["agent_id"] != "agent-1" || agents[0]["name"] != "Support Bot" {
		t.Fatalf("snapshot agent = %#v, want agent-1/Support Bot", agents[0])
	}
	if !snapshotHasCapability(snapshot, "agent.inspect") ||
		!snapshotHasCapability(snapshot, "agent.update_config") {
		t.Fatalf("snapshot capabilities = %#v, want inspect and update_config", snapshot["capabilities"])
	}
}

func TestRestoreConsoleAgentsContextFromSnapshotMetadata(t *testing.T) {
	original := contextualConsoleAgentsManageCapabilityPartsForTest()
	original.RuntimeContext = "route=/console/agents/agent-1/agent capabilities=agent.update_config"
	metadata := streamingMessageMetadata(original)

	parts := &chatRequestParts{Query: "verify current agent config"}
	restoreConsoleAgentsContextFromMetadata(parts, metadata, nil)

	if !isConsoleAgentsContext(parts.RuntimeContext, parts.RawOperationContext, parts.OperationContext) {
		t.Fatalf("restored parts are not console agents context: %#v", parts.OperationContext)
	}
	if !hasConsoleAgentsManageCapability(parts.RuntimeContext, parts.RawOperationContext, parts.OperationContext) {
		t.Fatalf("restored parts missing manage capability: %#v", parts.OperationContext)
	}
	agents := consoleAgentsPromptVisibleAgents(parts)
	if len(agents) != 1 {
		t.Fatalf("restored visible agents length = %d, want 1: %#v", len(agents), agents)
	}
	if agents[0]["agent_id"] != "agent-1" || agents[0]["name"] != "Support Bot" {
		t.Fatalf("restored visible agent = %#v, want agent-1/Support Bot", agents[0])
	}

	params := skillRuntimeParametersForPrepared(&PreparedChat{parts: parts})
	if params["console_current_route"] != "/console/agents/agent-1/agent" ||
		params["console_agents_current_route"] != "/console/agents/agent-1/agent" {
		t.Fatalf("restored route params = %#v/%#v, want current Agent detail route", params["console_current_route"], params["console_agents_current_route"])
	}
}

func TestRestoreConsoleAgentsContextFromApprovalEventFallback(t *testing.T) {
	parts := &chatRequestParts{Query: "update current agent config"}
	event := map[string]interface{}{
		"approval_event": map[string]interface{}{
			"assets": []interface{}{
				map[string]interface{}{
					"id":           "agent-1",
					"name":         "Support Bot",
					"type":         "agent",
					"workspace_id": "workspace-1",
				},
			},
		},
	}

	restoreConsoleAgentsContextFromMetadata(parts, nil, event)

	if !isConsoleAgentsContext(parts.RuntimeContext, parts.RawOperationContext, parts.OperationContext) {
		t.Fatalf("fallback restored parts are not console agents context: %#v", parts.OperationContext)
	}
	agents := consoleAgentsPromptVisibleAgents(parts)
	if len(agents) != 1 || agents[0]["agent_id"] != "agent-1" {
		t.Fatalf("fallback visible agents = %#v, want agent-1", agents)
	}
}
