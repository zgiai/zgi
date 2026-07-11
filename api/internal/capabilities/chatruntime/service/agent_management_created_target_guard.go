package service

import (
	"encoding/json"
	"strconv"
	"strings"
	"unicode"

	"github.com/zgiai/zgi/api/internal/capabilities/chatruntime/skillloop"
	"github.com/zgiai/zgi/api/internal/modules/skills"
)

func skillLoopCreatedAgentTargetMismatchGuard(prepared *PreparedChat, req skillloop.ToolCallGuardRequest) (skillloop.FinalAnswerGuardResult, bool) {
	if prepared == nil || prepared.Message == nil ||
		!strings.EqualFold(strings.TrimSpace(req.SkillID), skills.SkillAgentManagement) ||
		!agentManagementToolRequiresSingleAgentTarget(req.ToolName) ||
		!skillLoopCreateAndEditPlanActive(prepared) {
		return skillloop.FinalAnswerGuardResult{}, false
	}
	targets := skillLoopBoundAgentTargets(prepared, req)
	if len(targets) == 0 {
		return skillloop.FinalAnswerGuardResult{}, false
	}
	requestedIDs := agentManagementSingleAgentTargetIDs(req.Arguments)
	if len(requestedIDs) == 0 {
		return skillloop.FinalAnswerGuardResult{}, false
	}
	for _, requestedID := range requestedIDs {
		for _, target := range targets {
			if strings.EqualFold(requestedID, target.AgentID) {
				return skillloop.FinalAnswerGuardResult{}, false
			}
		}
	}
	targetLabel := skillLoopCreatedAgentTargetsLabel(targets)
	if targetLabel == "" {
		targetLabel = targets[0].AgentID
	}
	message := strings.Join([]string{
		"agent-management/" + strings.TrimSpace(req.ToolName) + " targeted a different Agent than the one created in this turn.",
		"Continue with the created Agent target: " + targetLabel + ".",
	}, " ")
	systemMessage := strings.Join([]string{
		message,
		"The current create-and-edit task target is the Agent created earlier in this same turn.",
		"Do not use stale current-page or old visible Agent IDs for this follow-up edit.",
		"Retry agent-management/" + strings.TrimSpace(req.ToolName) + " with the bound created Agent target.",
	}, " ")
	return skillloop.FinalAnswerGuardResult{
		SkillID:       skills.SkillAgentManagement,
		ToolName:      strings.TrimSpace(req.ToolName),
		Message:       message,
		SystemMessage: systemMessage,
		Advisory:      true,
	}, true
}

func skillLoopMissingCreatedAgentBindingGuard(prepared *PreparedChat, req skillloop.ToolCallGuardRequest) (skillloop.FinalAnswerGuardResult, bool) {
	if prepared == nil || prepared.Message == nil ||
		!strings.EqualFold(strings.TrimSpace(req.SkillID), skills.SkillAgentManagement) ||
		!agentManagementToolRequiresSingleAgentTarget(req.ToolName) ||
		!skillLoopCreateAndEditPlanActive(prepared) {
		return skillloop.FinalAnswerGuardResult{}, false
	}
	binding := skillLoopPlanArgsBindingForTool(prepared.Message.Metadata, req.SkillID, req.ToolName)
	expr := strings.TrimSpace(binding["agent_id"])
	if expr == "" || !strings.HasPrefix(expr, "$created_agents") {
		return skillloop.FinalAnswerGuardResult{}, false
	}
	if value, ok := skillLoopResolveToolArgBindingExpression(prepared, req, expr); ok && strings.TrimSpace(value) != "" {
		return skillloop.FinalAnswerGuardResult{}, false
	}
	toolLabel := "agent-management/" + strings.TrimSpace(req.ToolName)
	message := toolLabel + " requires the Agent created earlier in this turn, but no successful create_agent result is available."
	systemMessage := strings.Join([]string{
		message,
		"Do not use the current page Agent or any visible stale Agent as a fallback target.",
		"If create_agent failed, retry create_agent with corrected arguments when safe; otherwise report the exact failure to the user.",
	}, " ")
	return skillloop.FinalAnswerGuardResult{
		SkillID:       skills.SkillAgentManagement,
		ToolName:      strings.TrimSpace(req.ToolName),
		Message:       message,
		SystemMessage: systemMessage,
		Advisory:      true,
	}, true
}

func skillLoopBoundAgentTargets(prepared *PreparedChat, req skillloop.ToolCallGuardRequest) []skillLoopCreatedAgentTargetRef {
	binding := skillLoopPlanArgsBindingForTool(prepared.Message.Metadata, req.SkillID, req.ToolName)
	if expr := strings.TrimSpace(binding["agent_id"]); expr != "" {
		targets := skillLoopCreatedAgentTargets(prepared, req)
		selector, _, ok := skillLoopParseCreatedAgentsBinding(expr)
		if !ok {
			return nil
		}
		target, ok := skillLoopSelectCreatedAgentTarget(targets, selector)
		if !ok || target.AgentID == "" {
			return nil
		}
		return []skillLoopCreatedAgentTargetRef{target}
	}
	targets := skillLoopCreatedAgentTargets(prepared, req)
	if len(targets) != 1 {
		return nil
	}
	return targets
}

func skillLoopCreatedAgentTargetsLabel(targets []skillLoopCreatedAgentTargetRef) string {
	labels := make([]string, 0, len(targets))
	for _, target := range targets {
		if target.AgentID == "" {
			continue
		}
		label := target.AgentID
		if target.AgentName != "" {
			label = target.AgentName + " (" + target.AgentID + ")"
		}
		labels = append(labels, label)
	}
	return strings.Join(labels, ", ")
}

func skillLoopCreateAndEditPlanActive(prepared *PreparedChat) bool {
	if prepared == nil || prepared.Message == nil {
		return false
	}
	plan := mapFromOperationContext(prepared.Message.Metadata["operation_plan"])
	if len(plan) == 0 {
		return false
	}
	if structuredPlan := mapFromOperationContext(plan["structured_plan"]); len(structuredPlan) > 0 &&
		strings.EqualFold(strings.TrimSpace(stringFromAny(structuredPlan["intent"])), "agent.create_and_edit") {
		return true
	}
	capabilityGoals := preparedAgentCapabilityGoals(prepared)
	if !skillLoopAgentManagementCreateEffectActive(prepared, plan) {
		return false
	}
	if agentCapabilityGoalsRequireConfigMutation(capabilityGoals) {
		return true
	}
	if prepared.parts != nil && agentManagementModelIntentRequestsIdentityMutation(prepared.parts.ModelTurnIntent) {
		return true
	}
	if operationPlanHasToolStepWithStatus(plan, skills.SkillAgentManagement, "update_agent_identity", operationPlanStepStatusPending) ||
		operationPlanHasToolStepWithStatus(plan, skills.SkillAgentManagement, "update_agent_identity", operationPlanStepStatusCompleted) {
		return true
	}
	return false
}

func skillLoopAgentManagementCreateEffectActive(prepared *PreparedChat, plan map[string]interface{}) bool {
	if len(plan) > 0 {
		if target := mapFromOperationContext(plan["asset_target"]); strings.EqualFold(strings.TrimSpace(stringFromAny(target["effect"])), "create") {
			return true
		}
		if operationPlanHasToolStepWithStatus(plan, skills.SkillAgentManagement, "create_agent", operationPlanStepStatusPending) ||
			operationPlanHasToolStepWithStatus(plan, skills.SkillAgentManagement, "create_agent", operationPlanStepStatusCompleted) {
			return true
		}
	}
	if prepared != nil && prepared.parts != nil && prepared.parts.ModelTurnIntent != nil &&
		strings.EqualFold(strings.TrimSpace(prepared.parts.ModelTurnIntent.AssetEffect), "create") {
		return true
	}
	if prepared != nil && prepared.Message != nil &&
		len(successfulMetadataToolCalls(prepared.Message.Metadata, skills.SkillAgentManagement, "create_agent")) > 0 {
		return true
	}
	return false
}

func agentManagementModelIntentRequestsIdentityMutation(intent *AIChatModelTurnIntent) bool {
	return modelTurnIntentHasRecommendedCapability(intent,
		"agent.identity",
		"agent.profile",
		"agent.name",
		"agent.description",
		"agent.icon",
	)
}

type skillLoopCreatedAgentTargetRef struct {
	AgentID   string
	AgentName string
	Href      string
	ClientKey string
}

func skillLoopCreatedAgentTargets(prepared *PreparedChat, req skillloop.ToolCallGuardRequest) []skillLoopCreatedAgentTargetRef {
	if prepared == nil || prepared.Message == nil {
		return nil
	}
	calls := append(successfulMetadataToolCalls(prepared.Message.Metadata, skills.SkillAgentManagement, "create_agent"),
		matchingSkillToolCalls(req.SuccessfulToolCalls, skills.SkillAgentManagement, "create_agent")...)
	targets := make([]skillLoopCreatedAgentTargetRef, 0, len(calls))
	seen := map[string]struct{}{}
	for _, call := range calls {
		target, ok := skillLoopCreatedAgentTargetFromCall(call)
		if !ok {
			continue
		}
		if _, exists := seen[strings.ToLower(target.AgentID)]; exists {
			continue
		}
		seen[strings.ToLower(target.AgentID)] = struct{}{}
		targets = append(targets, target)
	}
	return targets
}

func skillLoopCreatedAgentTargetFromCall(call skillloop.SkillToolCallRef) (skillLoopCreatedAgentTargetRef, bool) {
	agent := mapFromOperationContext(call.Result["agent"])
	agentID := strings.TrimSpace(firstNonEmptyString(
		call.Result["agent_id"],
		call.Result["id"],
		agent["agent_id"],
		agent["id"],
	))
	if agentID == "" {
		return skillLoopCreatedAgentTargetRef{}, false
	}
	agentName := strings.TrimSpace(firstNonEmptyString(
		call.Result["agent_name"],
		call.Result["name"],
		agent["agent_name"],
		agent["name"],
		call.Arguments["name"],
		call.Arguments["agent_name"],
	))
	clientKey := strings.TrimSpace(firstNonEmptyString(
		call.Result["client_key"],
		agent["client_key"],
		call.Arguments["client_key"],
	))
	if clientKey == "" {
		clientKey = normalizeAgentBindingClientKey(agentName)
	}
	return skillLoopCreatedAgentTargetRef{
		AgentID:   agentID,
		AgentName: agentName,
		Href: strings.TrimSpace(firstNonEmptyString(
			call.Result["href"],
			agent["href"],
		)),
		ClientKey: clientKey,
	}, true
}

func normalizeAgentBindingClientKey(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return ""
	}
	var b strings.Builder
	lastUnderscore := false
	for _, r := range value {
		switch {
		case unicode.IsSpace(r) || r == '-' || r == '_':
			if !lastUnderscore && b.Len() > 0 {
				b.WriteRune('_')
				lastUnderscore = true
			}
		default:
			b.WriteRune(r)
			lastUnderscore = false
		}
	}
	return strings.Trim(b.String(), "_")
}

func skillLoopResolveCreatedAgentsBinding(prepared *PreparedChat, req skillloop.ToolCallGuardRequest, expr string) (string, bool) {
	targets := skillLoopCreatedAgentTargets(prepared, req)
	if len(targets) == 0 {
		return "", false
	}
	selector, field, ok := skillLoopParseCreatedAgentsBinding(expr)
	if !ok {
		return "", false
	}
	target, ok := skillLoopSelectCreatedAgentTarget(targets, selector)
	if !ok {
		return "", false
	}
	switch field {
	case "agent_id", "id":
		return target.AgentID, target.AgentID != ""
	case "name", "agent_name":
		return target.AgentName, target.AgentName != ""
	case "href":
		return target.Href, target.Href != ""
	case "client_key":
		return target.ClientKey, target.ClientKey != ""
	default:
		return "", false
	}
}

func skillLoopParseCreatedAgentsBinding(expr string) (map[string]string, string, bool) {
	expr = strings.TrimSpace(expr)
	if expr == "" || !strings.HasPrefix(expr, "$created_agents") {
		return nil, "", false
	}
	rest := strings.TrimPrefix(expr, "$created_agents")
	selector := map[string]string{}
	if strings.HasPrefix(rest, "[") {
		end := strings.Index(rest, "]")
		if end < 0 {
			return nil, "", false
		}
		rawSelector := strings.TrimSpace(rest[1:end])
		rest = rest[end+1:]
		if rawSelector != "" {
			if idx, err := strconv.Atoi(rawSelector); err == nil {
				selector["index"] = strconv.Itoa(idx)
			} else {
				parts := strings.SplitN(rawSelector, "=", 2)
				if len(parts) != 2 {
					return nil, "", false
				}
				key := strings.ToLower(strings.TrimSpace(parts[0]))
				value := strings.Trim(strings.TrimSpace(parts[1]), `"'`)
				if key == "" || value == "" {
					return nil, "", false
				}
				selector[key] = value
			}
		}
	}
	if !strings.HasPrefix(rest, ".") {
		return nil, "", false
	}
	field := strings.ToLower(strings.TrimSpace(strings.TrimPrefix(rest, ".")))
	if field == "" || strings.Contains(field, ".") {
		return nil, "", false
	}
	return selector, field, true
}

func skillLoopSelectCreatedAgentTarget(targets []skillLoopCreatedAgentTargetRef, selector map[string]string) (skillLoopCreatedAgentTargetRef, bool) {
	if len(targets) == 0 {
		return skillLoopCreatedAgentTargetRef{}, false
	}
	if len(selector) == 0 {
		return targets[0], true
	}
	if rawIndex := strings.TrimSpace(selector["index"]); rawIndex != "" {
		index, err := strconv.Atoi(rawIndex)
		if err != nil || index < 0 || index >= len(targets) {
			return skillLoopCreatedAgentTargetRef{}, false
		}
		return targets[index], true
	}
	if name := strings.TrimSpace(selector["name"]); name != "" {
		for _, target := range targets {
			if strings.EqualFold(strings.TrimSpace(target.AgentName), name) {
				return target, true
			}
		}
	}
	if clientKey := normalizeAgentBindingClientKey(selector["client_key"]); clientKey != "" {
		for _, target := range targets {
			if strings.EqualFold(normalizeAgentBindingClientKey(target.ClientKey), clientKey) ||
				strings.EqualFold(normalizeAgentBindingClientKey(target.AgentName), clientKey) {
				return target, true
			}
		}
	}
	return skillLoopCreatedAgentTargetRef{}, false
}

func agentManagementToolRequiresSingleAgentTarget(toolName string) bool {
	switch strings.TrimSpace(toolName) {
	case "get_agent",
		"get_agent_config",
		"update_agent_identity",
		"update_agent_config",
		"replace_agent_memory_slots",
		"list_agent_skill_candidates",
		"list_agent_knowledge_candidates",
		"list_agent_database_candidates",
		"list_agent_database_tables",
		"list_agent_workflow_binding_candidates":
		return true
	default:
		return false
	}
}

func agentManagementSingleAgentTargetIDs(args map[string]interface{}) []string {
	if len(args) == 0 {
		return nil
	}
	ids := []string{}
	addID := func(value interface{}) {
		if id := strings.TrimSpace(firstNonEmptyString(value)); id != "" {
			ids = appendUniqueStrings(ids, id)
		}
	}
	for _, key := range []string{"agent_id", "id", "asset_id", "resource_id"} {
		addID(args[key])
	}
	for _, agent := range mapSliceFromAny(args["agents"]) {
		addID(firstNonEmptyString(agent["agent_id"], agent["id"], agent["asset_id"], agent["resource_id"]))
	}
	if raw := strings.TrimSpace(stringFromAny(args["agents"])); raw != "" {
		var parsed []map[string]interface{}
		if err := json.Unmarshal([]byte(raw), &parsed); err == nil {
			for _, agent := range parsed {
				addID(firstNonEmptyString(agent["agent_id"], agent["id"], agent["asset_id"], agent["resource_id"]))
			}
		}
	}
	return ids
}
