package service

import "strings"

const (
	consoleAgentsContextSnapshotKey       = "console_agents_context_snapshot"
	consoleAgentsContextSnapshotSchema    = "zgi.aichat.console_agents_context_snapshot.v1"
	consoleAgentsContextSnapshotMaxAgents = 20
)

var consoleAgentsPageSnapshotMetadataKeys = []string{
	"context_ready",
	"agents_query_status",
	"agents_query_settled",
	"total_agent_count",
	"total_pages",
	"current_page",
	"page_size",
	"visible_range_start",
	"visible_range_end",
	"more_pages_available",
	"visible_agent_count",
	"selected_agent_count",
	"selected_visible_agent_count",
	"context_visible_limit",
	"ordinal_scope",
	"visible_order_basis",
	"sort",
	"sort_key",
	"sort_direction",
	"search",
	"workspace_id",
	"workspace_name",
	"organization_mode",
}

func consoleAgentsContextSnapshot(parts *chatRequestParts) map[string]interface{} {
	if parts == nil || !isConsoleAgentsContext(parts.RuntimeContext, parts.RawOperationContext, parts.OperationContext) {
		return nil
	}
	agents := consoleAgentsPromptVisibleAgents(parts)
	if len(agents) == 0 {
		return nil
	}
	if len(agents) > consoleAgentsContextSnapshotMaxAgents {
		agents = agents[:consoleAgentsContextSnapshotMaxAgents]
	}

	capabilities := consoleAgentsContextSnapshotCapabilities(parts)
	if len(capabilities) == 0 {
		return nil
	}

	route := consoleAgentsContextSnapshotRoute(parts, agents)
	snapshot := map[string]interface{}{
		"schema":         consoleAgentsContextSnapshotSchema,
		"page":           "console.agents",
		"route":          firstNonEmptyString(route, "/console/agents"),
		"capabilities":   capabilities,
		"visible_agents": copyMapSlice(agents),
	}
	for key, value := range consoleAgentsPageSnapshotMetadata(parts) {
		snapshot[key] = value
	}
	if _, ok := snapshot["visible_agent_count"]; !ok {
		snapshot["visible_agent_count"] = len(agents)
	}
	return snapshot
}

func consoleAgentsContextSnapshotRoute(parts *chatRequestParts, agents []map[string]interface{}) string {
	if route := consoleRouteFromRuntimeContext(parts.RuntimeContext); route != "" {
		return route
	}
	for _, source := range []map[string]interface{}{parts.RawOperationContext, parts.OperationContext} {
		for _, resource := range mapSliceFromAny(source["resources"]) {
			if !isConsoleAgentsPageResource(resource) {
				continue
			}
			metadata := mapFromOperationContext(resource["metadata"])
			if route := strings.TrimSpace(firstNonEmptyString(resource["href"], resource["route"], metadata["route"], metadata["href"])); route != "" {
				return route
			}
		}
	}
	if len(agents) == 1 {
		return strings.TrimSpace(firstNonEmptyString(agents[0]["href"], agents[0]["route"]))
	}
	return ""
}

func consoleAgentsPageSnapshotMetadata(parts *chatRequestParts) map[string]interface{} {
	if parts == nil {
		return nil
	}
	for _, source := range []map[string]interface{}{parts.RawOperationContext, parts.OperationContext} {
		metadata := consoleAgentsPageResourceMetadata(source)
		if len(metadata) > 0 {
			return metadata
		}
	}
	return nil
}

func consoleAgentsPageResourceMetadata(source map[string]interface{}) map[string]interface{} {
	if len(source) == 0 {
		return nil
	}
	metadata := make(map[string]interface{}, len(consoleAgentsPageSnapshotMetadataKeys))
	for _, resource := range mapSliceFromAny(source["resources"]) {
		if !isConsoleAgentsPageResource(resource) {
			continue
		}
		rawMetadata := mapFromOperationContext(resource["metadata"])
		if len(rawMetadata) == 0 {
			continue
		}
		for _, key := range consoleAgentsPageSnapshotMetadataKeys {
			if _, exists := metadata[key]; exists {
				continue
			}
			value, ok := rawMetadata[key]
			if !ok {
				continue
			}
			if sanitized, ok := sanitizedOperationScalar(value); ok {
				metadata[key] = sanitized
			}
		}
	}
	if len(metadata) > 0 {
		return metadata
	}
	return nil
}

func consoleAgentsContextSnapshotCapabilities(parts *chatRequestParts) []interface{} {
	if parts == nil {
		return nil
	}
	seen := map[string]struct{}{}
	out := []interface{}{}
	add := func(id string) {
		id = strings.TrimSpace(id)
		if id == "" {
			return
		}
		if _, ok := seen[id]; ok {
			return
		}
		seen[id] = struct{}{}
		out = append(out, map[string]interface{}{"id": id})
	}

	if hasConsoleAgentsReadCapability(parts.RuntimeContext, parts.RawOperationContext, parts.OperationContext) {
		add("agent.list_visible")
		add("agent.inspect")
	}
	if hasConsoleAgentsManageCapability(parts.RuntimeContext, parts.RawOperationContext, parts.OperationContext) {
		add("agent.update_identity")
		add("agent.update_config")
		add("agent.delete_visible")
	}
	return out
}

func restoreConsoleAgentsContextFromMetadata(parts *chatRequestParts, metadata map[string]interface{}, event map[string]interface{}) {
	if parts == nil {
		return
	}
	if isConsoleAgentsContext(parts.RuntimeContext, parts.RawOperationContext, parts.OperationContext) &&
		len(consoleAgentsPromptVisibleAgents(parts)) > 0 {
		return
	}

	snapshot := mapFromOperationContext(metadata[consoleAgentsContextSnapshotKey])
	if len(snapshot) == 0 {
		snapshot = consoleAgentsContextSnapshotFromApprovalEvent(event)
	}
	operationContext := consoleAgentsOperationContextFromSnapshot(snapshot)
	if operationContext == nil {
		return
	}

	route := firstNonEmptyString(snapshot["route"], "/console/agents")
	parts.RuntimeContext = "Restored Console Agents page context from the original AIChat turn. route=" + route
	parts.RawOperationContext = copyStringAnyMap(operationContext)
	normalized, ledger := normalizeOperationContext(operationContext)
	parts.OperationContext = normalized
	parts.OperationLedger = ledger
}

func consoleAgentsOperationContextFromSnapshot(snapshot map[string]interface{}) map[string]interface{} {
	if len(snapshot) == 0 || !strings.EqualFold(strings.TrimSpace(stringMetadataValue(snapshot["page"])), "console.agents") {
		return nil
	}
	agents := mapSliceFromAny(snapshot["visible_agents"])
	if len(agents) == 0 {
		return nil
	}

	route := strings.TrimSpace(firstNonEmptyString(snapshot["route"], "/console/agents"))
	pageMetadata := map[string]interface{}{
		"page":          "console.agents",
		"route":         route,
		"resource_kind": "page",
	}
	for _, key := range consoleAgentsPageSnapshotMetadataKeys {
		if value, ok := snapshot[key]; ok && value != nil {
			pageMetadata[key] = value
		}
	}

	resources := make([]interface{}, 0, len(agents)+1)
	resources = append(resources, map[string]interface{}{
		"resource_id":   "console.agents",
		"resource_type": "page",
		"type":          "page",
		"title":         "console.agents",
		"href":          route,
		"metadata":      pageMetadata,
	})
	for _, agent := range agents {
		agentID := strings.TrimSpace(firstNonEmptyString(agent["agent_id"], agent["id"], agent["resource_id"]))
		name := strings.TrimSpace(firstNonEmptyString(agent["name"], agent["title"], agent["agent_name"]))
		if agentID == "" || name == "" {
			continue
		}
		href := normalizeAgentDetailHref(firstNonEmptyString(agent["href"]))
		if href == "" {
			href = consoleAgentDetailHref(agentID)
		}
		metadata := map[string]interface{}{
			"resource_kind": "agent",
			"agent_id":      agentID,
			"name":          name,
			"href":          href,
		}
		for _, key := range []string{"visible_index", "agent_type", "workspace_id", "description", "selected", "can_edit"} {
			if value, ok := agent[key]; ok && value != nil {
				metadata[key] = value
			}
		}
		resources = append(resources, map[string]interface{}{
			"resource_id":   agentID,
			"resource_type": "agent",
			"type":          "agent",
			"title":         name,
			"href":          href,
			"source":        "Agents page",
			"status":        "available",
			"metadata":      metadata,
		})
	}
	if len(resources) <= 1 {
		return nil
	}

	capabilities := mapSliceFromAny(snapshot["capabilities"])
	if len(capabilities) == 0 {
		capabilities = []map[string]interface{}{
			{"id": "agent.list_visible"},
			{"id": "agent.inspect"},
		}
	}
	capabilityItems := make([]interface{}, 0, len(capabilities))
	for _, capability := range capabilities {
		id := strings.TrimSpace(firstNonEmptyString(capability["id"], capability["tool_id"], capability["capability_id"]))
		if id == "" {
			continue
		}
		capabilityItems = append(capabilityItems, map[string]interface{}{"id": id})
	}
	if len(capabilityItems) == 0 {
		return nil
	}

	return map[string]interface{}{
		"schema":       "zgi.aichat.operation_context.v1",
		"version":      1,
		"resources":    resources,
		"capabilities": capabilityItems,
		"summary": map[string]interface{}{
			"resource_count":   len(resources),
			"capability_count": len(capabilityItems),
		},
	}
}

func consoleAgentsContextSnapshotFromApprovalEvent(event map[string]interface{}) map[string]interface{} {
	approvalEvent := toolGovernanceApprovalEventFromEvent(event)
	if len(approvalEvent) == 0 {
		return nil
	}
	assets := mapSliceFromAny(approvalEvent["assets"])
	if len(assets) == 0 {
		return nil
	}
	agents := make([]map[string]interface{}, 0, len(assets))
	for index, asset := range assets {
		if !strings.EqualFold(strings.TrimSpace(firstNonEmptyString(asset["type"], asset["asset_type"], asset["resource_type"])), "agent") {
			continue
		}
		agentID := strings.TrimSpace(stringFromAny(asset["id"]))
		name := strings.TrimSpace(firstNonEmptyString(asset["name"], asset["title"], asset["agent_name"]))
		if agentID == "" || name == "" {
			continue
		}
		agent := map[string]interface{}{
			"visible_index": index + 1,
			"agent_id":      agentID,
			"id":            agentID,
			"name":          name,
			"href":          consoleAgentDetailHref(agentID),
		}
		if workspaceID := strings.TrimSpace(stringFromAny(asset["workspace_id"])); workspaceID != "" {
			agent["workspace_id"] = workspaceID
		}
		agents = append(agents, agent)
	}
	if len(agents) == 0 {
		return nil
	}
	return map[string]interface{}{
		"schema":              "zgi.aichat.console_agents_context_snapshot.approval_fallback.v1",
		"page":                "console.agents",
		"route":               firstNonEmptyString(agents[0]["href"], "/console/agents"),
		"visible_agent_count": len(agents),
		"capabilities": []interface{}{
			map[string]interface{}{"id": "agent.list_visible"},
			map[string]interface{}{"id": "agent.inspect"},
			map[string]interface{}{"id": "agent.update_identity"},
			map[string]interface{}{"id": "agent.update_config"},
		},
		"visible_agents": agents,
	}
}
