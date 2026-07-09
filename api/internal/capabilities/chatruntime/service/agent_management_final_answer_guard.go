package service

import (
	"strings"

	"github.com/zgiai/zgi/api/internal/capabilities/chatruntime/skillloop"
	"github.com/zgiai/zgi/api/internal/modules/skills"
)

func skillLoopAgentManagementFinalAnswerGuard(prepared *PreparedChat) skillloop.FinalAnswerGuard {
	if prepared == nil || prepared.parts == nil ||
		!skillIDEnabled(prepared.parts.SkillIDs, skills.SkillAgentManagement) {
		return nil
	}
	hasConsoleNavigator := skillIDEnabled(prepared.parts.SkillIDs, skills.SkillConsoleNavigator)
	currentAgentID := currentConsoleAgentID(prepared.parts)
	configReadTargetID := agentManagementConfigReadTargetID(prepared.parts)
	deleteTarget := consoleNavigationRouteHint{Href: "/console/agents", Label: "Agent list"}
	wantsCreatedDetail := hasConsoleNavigator && wantsCreatedAgentDetailNavigationForPrepared(prepared)
	modelDecidesPlan := preparedOperationPlanModelDecides(prepared)
	requiredBindingTools := []string(nil)
	if !modelDecidesPlan {
		requiredBindingTools = agentBindingGuardRequiredToolsForPrepared(prepared)
	}
	operationPlan := map[string]interface{}(nil)
	if prepared.Message != nil {
		operationPlan = mapFromOperationContext(prepared.Message.Metadata["operation_plan"])
	}
	requiresConfigRead := !modelDecidesPlan && configReadTargetID != "" &&
		agentManagementFinalAnswerRequiresConfigRead(prepared, operationPlan) &&
		len(requiredBindingTools) == 0
	if currentAgentID == "" && !wantsCreatedDetail && !requiresConfigRead {
		return nil
	}
	return func(req skillloop.FinalAnswerGuardRequest) (skillloop.FinalAnswerGuardResult, bool) {
		if requiresConfigRead &&
			!finalAnswerGuardHasSuccessfulTool(req, skills.SkillAgentManagement, "get_agent_config") &&
			!finalAnswerGuardHasAttemptedTool(req, skills.SkillAgentManagement, "get_agent_config") {
			return agentConfigReadRequiresToolGuardResult(), true
		}
		if missingTool := firstMissingAgentBindingMutation(req, requiredBindingTools); missingTool != "" {
			return agentBindingRequiresMutationGuardResult(missingTool, requiredBindingTools), true
		}
		if wantsCreatedDetail {
			if createdHref := createdAgentDetailHrefFromCalls(req.SuccessfulToolCalls); createdHref != "" &&
				!clientActionContinuationLoadedRoute(prepared.parts, createdHref) &&
				!finalAnswerGuardHasExactConsoleNavigateCall(req.SuccessfulToolCalls, createdHref) &&
				!finalAnswerGuardHasExactConsoleNavigateCall(req.AttemptedToolCalls, createdHref) {
				return createdAgentRequiresDetailNavigationGuardResult(consoleNavigationRouteHint{
					Href:  createdHref,
					Label: "Agent detail",
				}), true
			}
		}
		if !hasConsoleNavigator || currentAgentID == "" || !finalAnswerGuardHasAgentDeleteCall(req.SuccessfulToolCalls, currentAgentID) {
			return skillloop.FinalAnswerGuardResult{}, false
		}
		if finalAnswerGuardHasExactConsoleNavigateCall(req.SuccessfulToolCalls, deleteTarget.Href) ||
			finalAnswerGuardHasExactConsoleNavigateCall(req.AttemptedToolCalls, deleteTarget.Href) {
			return skillloop.FinalAnswerGuardResult{}, false
		}
		return agentDeleteRequiresListNavigationGuardResult(deleteTarget), true
	}
}

func preparedOperationPlanModelDecides(prepared *PreparedChat) bool {
	if prepared == nil || prepared.Message == nil {
		return false
	}
	plan := mapFromOperationContext(metadataValue(prepared.Message.Metadata, "operation_plan"))
	if len(plan) == 0 {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(stringFromAny(plan["planning_mode"])), "phase_only_model_decides") ||
		strings.EqualFold(strings.TrimSpace(stringFromAny(plan["tool_choice_mode"])), aiChatTurnToolChoiceModelDecides)
}

func wantsCreatedAgentDetailNavigationForPrepared(prepared *PreparedChat) bool {
	if prepared == nil || prepared.parts == nil {
		return false
	}
	if intent := prepared.parts.ModelTurnIntent; intent != nil {
		return intent.OpenCreatedAgentDetail
	}
	if prepared.Message != nil {
		plan := mapFromOperationContext(metadataValue(prepared.Message.Metadata, "operation_plan"))
		if len(plan) > 0 {
			if boolFromPlanValue(plan["open_created_agent_detail"]) {
				return true
			}
		}
	}
	return false
}

func boolFromPlanValue(value interface{}) bool {
	switch v := value.(type) {
	case bool:
		return v
	case string:
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "true", "yes", "1", "required":
			return true
		default:
			return false
		}
	default:
		return strings.EqualFold(strings.TrimSpace(stringFromAny(value)), "true")
	}
}

func agentManagementFinalAnswerRequiresConfigRead(prepared *PreparedChat, plan map[string]interface{}) bool {
	if operationPlanHasToolStepWithStatus(plan, skills.SkillAgentManagement, "get_agent_config", operationPlanStepStatusPending) {
		return true
	}
	if len(plan) > 0 {
		return false
	}
	return false
}

func agentManagementConfigReadTargetID(parts *chatRequestParts) string {
	if parts == nil || !isConsoleAgentsContext(parts.RuntimeContext, parts.RawOperationContext, parts.OperationContext) {
		return ""
	}
	if agentID := currentConsoleAgentID(parts); agentID != "" {
		return agentID
	}
	visible := consoleAgentsPromptVisibleAgents(parts)
	if len(visible) == 0 {
		return ""
	}
	if targetID := agentManagementVisibleIndexTargetID(parts.ModelTurnIntent, visible); targetID != "" {
		return targetID
	}
	return ""
}

func agentManagementVisibleIndexTargetID(intent *AIChatModelTurnIntent, visible []map[string]interface{}) string {
	if intent == nil || intent.TargetVisibleIndex <= 0 || len(visible) == 0 {
		return ""
	}
	for index, agent := range visible {
		visibleIndex := firstPositiveInt(
			intValueFromAny(agent["visible_index"]),
			intValueFromAny(agent["visible_ordinal"]),
			intValueFromAny(agent["visible_rank"]),
			index+1,
		)
		if visibleIndex != intent.TargetVisibleIndex {
			continue
		}
		return strings.TrimSpace(firstNonEmptyString(agent["agent_id"], agent["id"], agent["resource_id"]))
	}
	return ""
}

func agentConfigReadRequiresToolGuardResult() skillloop.FinalAnswerGuardResult {
	return skillloop.FinalAnswerGuardResult{
		SkillID:  skills.SkillAgentManagement,
		ToolName: "get_agent_config",
		Message:  "The user asked for the current Agent configuration, but no agent-management/get_agent_config evidence exists yet.",
		SystemMessage: strings.Join([]string{
			"The candidate final answer is premature for this Agent configuration question.",
			"Call agent-management/get_agent_config for the current Agent before answering.",
			"Base the final answer only on the tool result and page evidence.",
			"If get_agent_config fails, explain that failure truthfully instead of inferring enabled skills, model, provider, bindings, or other configuration from unsupported assumptions.",
		}, " "),
	}
}

func agentBindingGuardRequiredToolsForPrepared(prepared *PreparedChat) []string {
	if prepared == nil {
		return nil
	}
	if prepared.Message != nil {
		plan := mapFromOperationContext(metadataValue(prepared.Message.Metadata, "operation_plan"))
		if tools := agentBindingGuardRequiredToolsFromOperationPlan(plan); len(tools) > 0 {
			return tools
		}
	}
	return nil
}

func agentBindingGuardRequiredToolsFromOperationPlan(plan map[string]interface{}) []string {
	if len(plan) == 0 {
		return nil
	}
	actions := map[string]string{}
	mergeActions := func(next map[string]string) {
		for field, action := range next {
			canonicalField := operationPlanAgentConfigCanonicalField(field)
			canonicalAction := operationPlanCanonicalAgentConfigBindingAction(action)
			if canonicalField == "" || canonicalAction == "" {
				continue
			}
			if operationPlanCanonicalAgentConfigBindingAction(actions[canonicalField]) == "" {
				actions[canonicalField] = canonicalAction
			}
		}
	}

	mergeActions(agentCapabilityGoalsExpectedBindingActions(agentCapabilityGoalsFromOperationPlan(plan)))
	for _, step := range mapSliceFromAny(plan["steps"]) {
		mergeActions(operationPlanAgentConfigBindingActionsFromAny(step[operationPlanExpectedBindingActionsKey]))
		if args := mapFromOperationContext(step["arguments"]); len(args) > 0 {
			mergeActions(operationPlanAgentConfigBindingActionsFromAny(args[operationPlanExpectedBindingActionsKey]))
		}
	}
	if structured := mapFromOperationContext(plan["structured_plan"]); len(structured) > 0 {
		mergeActions(agentCapabilityGoalsExpectedBindingActions(agentCapabilityGoalsFromMaps(structured["capability_goals"])))
		for _, operation := range mapSliceFromAny(structured["operations"]) {
			mergeActions(operationPlanAgentConfigBindingActionsFromAny(operation[operationPlanExpectedBindingActionsKey]))
			if args := mapFromOperationContext(operation["arguments"]); len(args) > 0 {
				mergeActions(operationPlanAgentConfigBindingActionsFromAny(args[operationPlanExpectedBindingActionsKey]))
			}
		}
	}
	return agentBindingGuardRequiredToolsFromActions(actions)
}

func firstMissingAgentBindingMutation(req skillloop.FinalAnswerGuardRequest, requiredTools []string) string {
	for _, toolName := range requiredTools {
		toolName = strings.TrimSpace(toolName)
		if toolName == "" {
			continue
		}
		if agentBindingMutationRequirementSatisfied(req, toolName) {
			continue
		}
		return toolName
	}
	return ""
}

func agentBindingRequiresMutationGuardResult(missingTool string, requiredTools []string) skillloop.FinalAnswerGuardResult {
	missing := []string{}
	for _, requirement := range requiredTools {
		missing = appendUniqueStrings(missing, agentBindingRequirementDisplay(requirement))
	}
	if len(missing) == 0 {
		missing = []string{agentBindingRequirementDisplay(missingTool)}
	}
	messageLines := []string{
		"The user explicitly requested Agent binding changes, but at least one requested binding section has no mutation evidence yet.",
		"Before claiming completion, call agent-management/update_agent_config with the remaining binding changes.",
	}
	systemLines := []string{
		"The candidate final answer is premature for this Agent binding request.",
		"Requested binding sections still missing evidence: " + strings.Join(missing, ", ") + ".",
		"The accepted agent-management binding mutation tool for this turn is update_agent_config.",
		"The currently missing section should be represented as update_agent_config patch fields; missing marker: " + agentBindingRequirementDisplay(missingTool) + ".",
		"Use update_agent_config when the current config and exact candidate IDs are known, because it can update multiple binding sections in one governed call.",
		"If a mutation tool fails, explain that failure; do not claim the binding was completed.",
	}
	return skillloop.FinalAnswerGuardResult{
		SkillID:       skills.SkillAgentManagement,
		ToolName:      "update_agent_config",
		Message:       strings.Join(messageLines, " "),
		SystemMessage: strings.Join(systemLines, " "),
	}
}

func finalAnswerGuardHasAgentDeleteCall(calls []skillloop.SkillToolCallRef, agentID string) bool {
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return false
	}
	for _, call := range calls {
		if !strings.EqualFold(strings.TrimSpace(call.SkillID), skills.SkillAgentManagement) {
			continue
		}
		switch strings.TrimSpace(call.ToolName) {
		case "delete_agent":
			if strings.TrimSpace(firstNonEmptyString(
				skillToolCallArgumentString(call.Arguments, "agent_id"),
				skillToolCallArgumentString(call.Arguments, "id"),
				skillToolCallArgumentString(call.Arguments, "asset_id"),
			)) == agentID {
				return true
			}
		case "delete_agents":
			if finalAnswerGuardBatchAgentDeleteHasTarget(call, agentID) {
				return true
			}
		}
	}
	return false
}

func finalAnswerGuardBatchAgentDeleteHasTarget(call skillloop.SkillToolCallRef, agentID string) bool {
	hasItemEvidence := false
	for _, item := range mapSliceFromAny(call.Result["item_results"]) {
		hasItemEvidence = true
		if strings.TrimSpace(firstNonEmptyString(
			item["agent_id"],
			item["id"],
			item["asset_id"],
			item["resource_id"],
		)) == agentID &&
			finalAnswerGuardAgentDeleteItemSucceeded(item["status"]) {
			return true
		}
	}
	group := mapFromOperationContext(call.Result["operation_group"])
	for _, item := range mapSliceFromAny(group["item_results"]) {
		hasItemEvidence = true
		if strings.TrimSpace(firstNonEmptyString(
			item["agent_id"],
			item["id"],
			item["asset_id"],
			item["resource_id"],
		)) == agentID &&
			finalAnswerGuardAgentDeleteItemSucceeded(item["status"]) {
			return true
		}
	}
	if hasItemEvidence {
		return false
	}
	return false
}

func finalAnswerGuardAgentDeleteItemSucceeded(status interface{}) bool {
	switch strings.ToLower(strings.TrimSpace(stringFromAny(status))) {
	case "succeeded", "success", "completed":
		return true
	default:
		return false
	}
}

func finalAnswerGuardHasExactConsoleNavigateCall(calls []skillloop.SkillToolCallRef, href string) bool {
	href = normalizeConsoleNavigationGuardHref(href)
	if href == "" {
		return false
	}
	for _, call := range calls {
		if !isConsoleNavigatorNavigateTool(call.SkillID, call.ToolName) {
			continue
		}
		if normalizeConsoleNavigationGuardHref(skillToolCallArgumentString(call.Arguments, "href")) == href {
			return true
		}
	}
	return false
}

func agentBindingGuardRequiredToolsFromActions(actions map[string]string) []string {
	if len(actions) == 0 {
		return nil
	}
	out := []string{}
	for field, action := range actions {
		field = operationPlanAgentConfigCanonicalField(field)
		if field != "" && operationPlanCanonicalAgentConfigBindingAction(action) != "" {
			out = appendUniqueStrings(out, agentBindingUpdateConfigRequirement(field))
		}
	}
	return out
}

func agentBindingUpdateConfigRequirement(field string) string {
	field = operationPlanAgentConfigCanonicalField(field)
	if field == "" {
		return "update_agent_config"
	}
	return "update_agent_config:" + field
}

func agentBindingRequirementField(requirement string) string {
	requirement = strings.TrimSpace(requirement)
	if strings.HasPrefix(requirement, "update_agent_config:") {
		return operationPlanAgentConfigCanonicalField(strings.TrimPrefix(requirement, "update_agent_config:"))
	}
	descriptor, ok := agentBindingToolDescriptorForTool(requirement)
	if !ok {
		return ""
	}
	return operationPlanAgentConfigCanonicalField(descriptor.field)
}

func agentBindingRequirementDisplay(requirement string) string {
	if field := agentBindingRequirementField(requirement); field != "" {
		return "update_agent_config." + field
	}
	return "update_agent_config"
}

func agentBindingMutationRequirementSatisfied(req skillloop.FinalAnswerGuardRequest, toolName string) bool {
	if finalAnswerGuardHasSuccessfulTool(req, skills.SkillAgentManagement, toolName) ||
		finalAnswerGuardHasAttemptedTool(req, skills.SkillAgentManagement, toolName) {
		return true
	}
	if field := agentBindingRequirementField(toolName); field != "" {
		if agentBindingUpdateConfigCoversField(req.SuccessfulToolCalls, field) {
			return true
		}
		if agentBindingLegacyMutationCoversField(req.SuccessfulToolCalls, field) {
			return true
		}
		if agentBindingLegacyMutationCoversField(req.AttemptedToolCalls, field) {
			return true
		}
		return agentBindingUpdateConfigAttemptedWithoutSuccessfulResult(req)
	}
	if agentBindingUpdateConfigCoversTool(req.SuccessfulToolCalls, toolName) {
		return true
	}
	return agentBindingUpdateConfigAttemptedWithoutSuccessfulResult(req)
}

func agentBindingUpdateConfigAttemptedWithoutSuccessfulResult(req skillloop.FinalAnswerGuardRequest) bool {
	if finalAnswerGuardHasSuccessfulTool(req, skills.SkillAgentManagement, "update_agent_config") {
		return false
	}
	return finalAnswerGuardHasAttemptedTool(req, skills.SkillAgentManagement, "update_agent_config")
}

func agentBindingUpdateConfigCoversTool(calls []skillloop.SkillToolCallRef, toolName string) bool {
	if field := agentBindingRequirementField(toolName); field != "" {
		return agentBindingUpdateConfigCoversField(calls, field)
	}
	for _, call := range calls {
		if !strings.EqualFold(strings.TrimSpace(call.SkillID), skills.SkillAgentManagement) ||
			!strings.EqualFold(strings.TrimSpace(call.ToolName), "update_agent_config") {
			continue
		}
		if agentBindingUpdateConfigResultCoversTool(call.Result, toolName) {
			return true
		}
	}
	return false
}

func agentBindingUpdateConfigCoversField(calls []skillloop.SkillToolCallRef, field string) bool {
	field = operationPlanAgentConfigCanonicalField(field)
	if field == "" {
		return false
	}
	for _, call := range calls {
		if !strings.EqualFold(strings.TrimSpace(call.SkillID), skills.SkillAgentManagement) ||
			!strings.EqualFold(strings.TrimSpace(call.ToolName), "update_agent_config") {
			continue
		}
		if agentBindingUpdateConfigResultCoversField(call.Result, field) {
			return true
		}
	}
	return false
}

func agentBindingLegacyMutationCoversField(calls []skillloop.SkillToolCallRef, field string) bool {
	field = operationPlanAgentConfigCanonicalField(field)
	if field == "" {
		return false
	}
	for _, call := range calls {
		if !strings.EqualFold(strings.TrimSpace(call.SkillID), skills.SkillAgentManagement) {
			continue
		}
		descriptor, ok := agentBindingToolDescriptorForTool(call.ToolName)
		if !ok || !strings.EqualFold(operationPlanAgentConfigCanonicalField(descriptor.field), field) {
			continue
		}
		return true
	}
	return false
}

func agentBindingUpdateConfigResultCoversTool(result map[string]interface{}, toolName string) bool {
	field, kind := agentBindingRequirementFieldAndKind(toolName)
	if field == "" && kind == "" {
		return false
	}
	if field != "" && agentBindingUpdateConfigResultCoversField(result, field) {
		return true
	}
	return agentBindingUpdateConfigResultCoversKind(result, kind)
}

func agentBindingUpdateConfigResultCoversField(result map[string]interface{}, field string) bool {
	field = operationPlanAgentConfigCanonicalField(field)
	if field == "" {
		return false
	}
	if field != "" && stringSliceContainsFold(stringSliceFromAny(result["updated_fields"]), field) {
		return true
	}
	for _, key := range []string{"binding_changes", "config_changes"} {
		for _, change := range mapSliceFromAny(result[key]) {
			if field != "" && strings.EqualFold(strings.TrimSpace(stringFromAny(change["field"])), field) {
				return true
			}
		}
	}
	return false
}

func agentBindingUpdateConfigResultCoversKind(result map[string]interface{}, kind string) bool {
	if kind != "" && strings.EqualFold(strings.TrimSpace(stringFromAny(result["binding_kind"])), kind) {
		return true
	}
	for _, key := range []string{"binding_changes", "config_changes"} {
		for _, change := range mapSliceFromAny(result[key]) {
			if kind != "" && strings.EqualFold(strings.TrimSpace(stringFromAny(change["binding_kind"])), kind) {
				return true
			}
		}
	}
	return false
}

func agentBindingRequirementFieldAndKind(toolName string) (string, string) {
	descriptor, ok := agentBindingToolDescriptorForTool(toolName)
	if !ok {
		return "", ""
	}
	return operationPlanAgentConfigCanonicalField(descriptor.field), strings.TrimSpace(descriptor.bindingKind)
}
