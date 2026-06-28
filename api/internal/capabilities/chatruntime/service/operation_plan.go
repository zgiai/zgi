package service

import (
	"fmt"
	"reflect"
	"strings"

	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	"github.com/zgiai/zgi/api/internal/modules/skills"
)

const (
	operationPlanVersion = "operation_plan.v1"

	operationPlanStatusRunning   = "running"
	operationPlanStatusCompleted = "completed"
	operationPlanStatusFailed    = "failed"

	operationPlanStepStatusPending   = "pending"
	operationPlanStepStatusCompleted = "completed"
	operationPlanStepStatusFailed    = "failed"
)

func operationPlanFromTurnStrategy(taskID string, parts *chatRequestParts, strategy *AIChatTurnStrategy) map[string]interface{} {
	if parts == nil || strategy == nil {
		return nil
	}
	steps := operationPlanStepsFromTurnStrategy(strategy)
	if len(steps) == 0 {
		return nil
	}
	stepStatus := make(map[string]interface{}, len(steps))
	for _, step := range steps {
		id := strings.TrimSpace(stringFromAny(step["id"]))
		if id != "" {
			stepStatus[id] = stringFromAny(step["status"])
		}
	}
	originalGoal := truncateRunes(strings.TrimSpace(parts.Query), 500)
	if isContinuationIntent(parts.Query) {
		if goal := recentOperationPlanOriginalGoal(parts); goal != "" {
			originalGoal = truncateRunes(goal, 500)
		}
	}
	plan := map[string]interface{}{
		"version":             operationPlanVersion,
		"task_id":             strings.TrimSpace(taskID),
		"original_user_goal":  originalGoal,
		"surface":             normalizeAIChatSurface(parts.Surface),
		"intent":              strategy.Intent,
		"status":              operationPlanStatusRunning,
		"steps":               interfaceSliceFromMapSlice(steps),
		"step_status":         stepStatus,
		"asset_target":        operationPlanAssetTarget(strategy),
		"pending_next_action": operationPlanPendingNextAction(steps),
		"derived_from":        "turn_strategy",
		"retry_policy": map[string]interface{}{
			"max_retries_per_step": 2,
			"on_repeated_failure":  "stop_and_report_actual_tool_result",
		},
		"completion_criteria": operationPlanCompletionCriteria(steps),
	}
	if strings.TrimSpace(strategy.CurrentPage) != "" {
		plan["current_page"] = strings.TrimSpace(strategy.CurrentPage)
	}
	return plan
}

func applyRecentOperationPlansFromBranch(parts *chatRequestParts, branch []*runtimemodel.Message) {
	if parts == nil || len(parts.RecentOperationPlans) > 0 {
		return
	}
	parts.RecentOperationPlans = recentContinuationOperationPlans(branch, recentContinuationTurnLimit)
}

func recentOperationPlanOriginalGoal(parts *chatRequestParts) string {
	plan := firstIncompleteRecentOperationPlan(parts)
	if len(plan) == 0 && parts != nil && len(parts.RecentOperationPlans) > 0 {
		plan = parts.RecentOperationPlans[0]
	}
	return strings.TrimSpace(stringFromAny(plan["original_user_goal"]))
}

func firstIncompleteRecentOperationPlan(parts *chatRequestParts) map[string]interface{} {
	if parts == nil {
		return nil
	}
	for _, plan := range parts.RecentOperationPlans {
		if operationPlanHasIncompleteWork(plan) {
			return plan
		}
	}
	return nil
}

func applyRecentOperationPlanToContinuationStrategy(parts *chatRequestParts, strategy *AIChatTurnStrategy) *AIChatTurnStrategy {
	if parts == nil || strategy == nil || !isContinuationIntent(parts.Query) {
		return strategy
	}
	plan := firstIncompleteRecentOperationPlan(parts)
	if len(plan) == 0 {
		return strategy
	}
	if intent := strings.TrimSpace(stringFromAny(plan["intent"])); intent != "" {
		strategy.Intent = "continue_" + intent
	}
	if target := mapFromOperationContext(plan["asset_target"]); len(target) > 0 {
		if page := strings.TrimSpace(stringFromAny(target["page"])); page != "" {
			strategy.TargetPage = page
		}
		if effect := strings.TrimSpace(stringFromAny(target["effect"])); effect != "" {
			strategy.AssetEffect = effect
		}
		if risk := strings.TrimSpace(stringFromAny(target["risk"])); risk != "" {
			strategy.AssetRisk = risk
		}
	}
	if pending := strings.TrimSpace(stringFromAny(plan["pending_next_action"])); pending != "" {
		strategy.SuccessCriteria = appendUniqueStrings(strategy.SuccessCriteria, "complete pending plan step: "+pending)
	}
	for _, step := range operationPlanPendingExecutableSteps(plan, 3) {
		skillID := strings.TrimSpace(stringFromAny(step["skill_id"]))
		toolName := strings.TrimSpace(stringFromAny(step["tool_name"]))
		if skillID == "" || !skillIDEnabled(parts.SkillIDs, skillID) {
			continue
		}
		strategy.PrimarySkills = appendUniqueStrings(strategy.PrimarySkills, skillID)
		if toolName != "" {
			args := map[string]string(nil)
			if operationPlanStepIsRoute(step) {
				if href := operationPlanStepTargetPage(step); href != "" {
					strategy.TargetPage = href
					args = map[string]string{"href": href}
				}
			}
			strategy = appendPlannedTool(strategy, skillID, toolName, args)
		}
		if operationPlanStepIsRoute(step) {
			strategy.RouteRequired = true
			break
		}
	}
	return strategy
}

func operationPlanPendingExecutableSteps(plan map[string]interface{}, limit int) []map[string]interface{} {
	if len(plan) == 0 || limit <= 0 {
		return nil
	}
	stepStatus := mapFromOperationContext(plan["step_status"])
	out := make([]map[string]interface{}, 0, limit)
	for _, step := range mapSliceFromAny(plan["steps"]) {
		id := strings.TrimSpace(stringFromAny(step["id"]))
		status := operationPlanNormalizeStepStatus(firstNonEmptyString(step["status"], stepStatus[id]))
		if status == operationPlanStepStatusCompleted || status == operationPlanStepStatusFailed {
			continue
		}
		if !operationPlanStepBlocksCompletion(step) {
			continue
		}
		skillID := strings.TrimSpace(stringFromAny(step["skill_id"]))
		toolName := strings.TrimSpace(stringFromAny(step["tool_name"]))
		if skillID == "" && toolName == "" {
			continue
		}
		out = append(out, step)
		if len(out) >= limit || operationPlanStepIsRoute(step) {
			break
		}
	}
	return out
}

func operationPlanStepsFromTurnStrategy(strategy *AIChatTurnStrategy) []map[string]interface{} {
	steps := []map[string]interface{}{}
	seen := map[string]struct{}{}
	add := func(step map[string]interface{}) {
		id := strings.TrimSpace(stringFromAny(step["id"]))
		if id == "" {
			id = fmt.Sprintf("step_%d", len(steps)+1)
			step["id"] = id
		}
		if _, ok := seen[id]; ok {
			return
		}
		seen[id] = struct{}{}
		step["status"] = operationPlanNormalizeStepStatus(stringFromAny(step["status"]))
		steps = append(steps, step)
	}

	if strategy.RequiredNextTool != nil {
		step := map[string]interface{}{
			"id":                operationPlanToolStepID(strategy.RequiredNextTool.SkillID, strategy.RequiredNextTool.ToolName),
			"title":             operationPlanToolStepTitle(strategy.RequiredNextTool.SkillID, strategy.RequiredNextTool.ToolName),
			"status":            operationPlanStepStatusPending,
			"skill_id":          strategy.RequiredNextTool.SkillID,
			"tool_name":         strategy.RequiredNextTool.ToolName,
			"required_evidence": operationPlanToolStepEvidence(strategy.RequiredNextTool.SkillID, strategy.RequiredNextTool.ToolName),
		}
		if href := strings.TrimSpace(strategy.RequiredNextTool.Arguments["href"]); href != "" {
			step["asset_target"] = map[string]interface{}{"page": href}
		}
		add(step)
	}

	routeOccurrences := map[string]int{}
	for _, route := range strategy.RemainingRouteSequence {
		href := strings.TrimSpace(route.Href)
		if href == "" {
			continue
		}
		routeKey := normalizeConsoleNavigationGuardHref(href)
		if routeKey == "" {
			routeKey = href
		}
		routeOccurrences[routeKey]++
		add(map[string]interface{}{
			"id":                operationPlanRouteStepID(href, routeOccurrences[routeKey]),
			"title":             firstNonEmptyString(route.Label, href),
			"status":            route.Status,
			"skill_id":          skillsConsoleNavigatorID(),
			"tool_name":         "navigate",
			"required_evidence": operationPlanToolStepEvidence(skillsConsoleNavigatorID(), "navigate"),
			"asset_target": map[string]interface{}{
				"page": href,
			},
		})
	}

	for _, tool := range strategy.PlannedTools {
		skillID := strings.TrimSpace(tool.SkillID)
		toolName := strings.TrimSpace(tool.ToolName)
		if skillID == "" || toolName == "" {
			continue
		}
		step := map[string]interface{}{
			"id":                operationPlanToolStepID(skillID, toolName),
			"title":             operationPlanToolStepTitle(skillID, toolName),
			"status":            operationPlanStepStatusPending,
			"skill_id":          skillID,
			"tool_name":         toolName,
			"required_evidence": operationPlanToolStepEvidence(skillID, toolName),
		}
		if target := operationPlanToolStepAssetTarget(skillID, toolName); len(target) > 0 {
			step["asset_target"] = target
		}
		add(step)
	}

	addSkillSteps := func(skillIDs []string, role string) {
		for _, skillID := range append([]string{}, skillIDs...) {
			skillID = strings.TrimSpace(skillID)
			if skillID == "" {
				continue
			}
			step := map[string]interface{}{
				"id":       "skill:" + skillID,
				"title":    "Use " + skillID,
				"status":   operationPlanStepStatusPending,
				"skill_id": skillID,
			}
			if role != "" {
				step["role"] = role
			}
			add(step)
		}
	}
	addSkillSteps(strategy.PrimarySkills, "primary")
	addSkillSteps(strategy.SupportingSkills, "supporting")

	if len(strategy.ObservationPoints) > 0 {
		add(map[string]interface{}{
			"id":                 "observe",
			"title":              "Observe result",
			"status":             operationPlanStepStatusPending,
			"observation_points": append([]string{}, strategy.ObservationPoints...),
		})
	}
	if strategy.WaitForContinue {
		add(map[string]interface{}{
			"id":              "wait:continue",
			"title":           "Wait for user continue",
			"status":          operationPlanStepStatusPending,
			"wait_for":        "continue",
			"deferred":        true,
			"execution_scope": strategy.ExecutionScope,
		})
	}
	return steps
}

func operationPlanAssetTarget(strategy *AIChatTurnStrategy) map[string]interface{} {
	target := map[string]interface{}{}
	if page := strings.TrimSpace(strategy.TargetPage); page != "" {
		target["page"] = page
	}
	if effect := strings.TrimSpace(strategy.AssetEffect); effect != "" {
		target["effect"] = effect
	}
	if risk := strings.TrimSpace(strategy.AssetRisk); risk != "" {
		target["risk"] = risk
	}
	if len(target) == 0 {
		return nil
	}
	return target
}

func operationPlanPendingNextAction(steps []map[string]interface{}) string {
	for _, step := range steps {
		if !operationPlanStepBlocksCompletion(step) {
			continue
		}
		status := strings.TrimSpace(stringFromAny(step["status"]))
		if status == operationPlanStepStatusFailed {
			return "none"
		}
	}
	for _, step := range steps {
		if !operationPlanStepBlocksCompletion(step) {
			continue
		}
		status := strings.TrimSpace(stringFromAny(step["status"]))
		if status == "" || status == operationPlanStepStatusPending {
			return firstNonEmptyString(step["title"], step["id"])
		}
	}
	return "none"
}

func operationPlanToolStepID(skillID, toolName string) string {
	skillID = strings.TrimSpace(skillID)
	toolName = strings.TrimSpace(toolName)
	if skillID == "" && toolName == "" {
		return ""
	}
	if toolName == "" {
		return "skill:" + skillID
	}
	return "tool:" + skillID + "/" + toolName
}

func operationPlanRouteStepID(href string, occurrence int) string {
	href = strings.TrimSpace(href)
	if href == "" {
		return ""
	}
	id := "route:" + href
	if occurrence <= 1 {
		return id
	}
	return fmt.Sprintf("%s#%d", id, occurrence)
}

func operationPlanToolStepTitle(skillID, toolName string) string {
	switch {
	case isConsoleNavigatorNavigateTool(skillID, toolName):
		return "Navigate to page"
	case isKnownArtifactGeneratorToolCall(skillID, toolName):
		return "Generate temporary file"
	case isFileManagerSaveToolCall(skillID, toolName):
		return "Save generated file to File Management"
	case isFileManagerDeleteToolCall(skillID, toolName):
		return "Delete resolved file"
	default:
		return "Run " + operationPlanToolStepID(skillID, toolName)
	}
}

func operationPlanToolStepEvidence(skillID, toolName string) []string {
	switch {
	case isConsoleNavigatorNavigateTool(skillID, toolName):
		return []string{"client_action.status=success", "client_action.result.loaded_href matches target page"}
	case isKnownArtifactGeneratorToolCall(skillID, toolName):
		return []string{"tool_call.status=success", "generated_files contains temporary artifact", "artifact file_id or tool_file_id"}
	case isFileManagerSaveToolCall(skillID, toolName):
		return []string{"tool_call.status=success", "generated_files contains managed_file artifact", "managed_file_id or upload_file_id", "saved filename"}
	case isFileManagerDeleteToolCall(skillID, toolName):
		return []string{"tool_call.status=success", "deleted file_id or deleted_count result"}
	case strings.EqualFold(strings.TrimSpace(skillID), skills.SkillFileReader) && strings.EqualFold(strings.TrimSpace(toolName), "read_file"):
		return []string{"tool_call.status=success", "result.content or explicit empty-content status", "resolved file name or file_id"}
	case strings.EqualFold(strings.TrimSpace(skillID), skills.SkillAgentManagement):
		return []string{"tool_call.status=success", "agent_id when applicable", "updated_fields or returned binding state when applicable"}
	default:
		if strings.TrimSpace(toolName) != "" {
			return []string{"tool_call.status=success", "tool result supports the final claim"}
		}
		return nil
	}
}

func operationPlanCompletionCriteria(steps []map[string]interface{}) []string {
	criteria := []string{
		"Final answer must be based on successful tool results and ledger evidence.",
		"Do not claim external asset creation, update, deletion, navigation, or read completion without matching evidence.",
		"If a required step failed or lacks evidence, report the actual failure or missing confirmation.",
		"Treat read, list, observe, and navigation steps as evidence collection points unless the final answer claims that specific action completed.",
	}
	for _, step := range steps {
		if !operationPlanStepRequiresStrictCompletionEvidence(step) {
			continue
		}
		if title := strings.TrimSpace(stringFromAny(step["title"])); title != "" {
			criteria = append(criteria, "Asset-changing step must have matching execution evidence before claiming completion: "+title)
		}
	}
	return criteria
}

func operationPlanStepRequiresStrictCompletionEvidence(step map[string]interface{}) bool {
	if !operationPlanStepBlocksCompletion(step) {
		return false
	}
	if operationPlanStepIsRoute(step) {
		return false
	}
	if strings.TrimSpace(stringFromAny(step["wait_for"])) != "" {
		return true
	}
	toolName := strings.TrimSpace(stringFromAny(step["tool_name"]))
	if toolName == "" {
		return false
	}
	target := mapFromOperationContext(step["asset_target"])
	effect := strings.ToLower(strings.TrimSpace(stringFromAny(target["effect"])))
	if effect != "" && effect != "read" {
		return true
	}
	return skillLoopToolNameLooksAssetMutation(toolName)
}

func operationPlanToolStepAssetTarget(skillID, toolName string) map[string]interface{} {
	switch {
	case isKnownArtifactGeneratorToolCall(skillID, toolName):
		return map[string]interface{}{"effect": "create_temporary_artifact"}
	case isFileManagerSaveToolCall(skillID, toolName):
		return map[string]interface{}{"effect": "create", "asset_type": "file", "target": "file_management"}
	case isFileManagerDeleteToolCall(skillID, toolName):
		return map[string]interface{}{"effect": "delete", "asset_type": "file"}
	case strings.EqualFold(strings.TrimSpace(skillID), skills.SkillAgentManagement):
		return operationPlanAgentManagementAssetTarget(toolName)
	default:
		return nil
	}
}

func operationPlanAgentManagementAssetTarget(toolName string) map[string]interface{} {
	switch strings.TrimSpace(toolName) {
	case "create_agent":
		return map[string]interface{}{"effect": "create", "asset_type": "agent"}
	case "update_agent_identity", "update_agent_config", "replace_agent_memory_slots":
		return map[string]interface{}{"effect": "update", "asset_type": "agent"}
	case "replace_agent_skill_bindings":
		return map[string]interface{}{"effect": "update", "asset_type": "agent_skill", "owner_asset_type": "agent"}
	case "replace_agent_knowledge_bindings":
		return map[string]interface{}{"effect": "update", "asset_type": "knowledge_base", "owner_asset_type": "agent"}
	case "replace_agent_database_bindings":
		return map[string]interface{}{"effect": "update", "asset_type": "database_table", "owner_asset_type": "agent"}
	case "replace_agent_workflow_bindings":
		return map[string]interface{}{"effect": "update", "asset_type": "workflow", "owner_asset_type": "agent"}
	case "delete_agent":
		return map[string]interface{}{"effect": "delete", "asset_type": "agent"}
	case "delete_agents":
		return map[string]interface{}{"effect": "delete", "asset_type": "agent", "operation_mode": "batch"}
	case "list_agents", "get_agent", "get_agent_config", "list_available_models",
		"list_agent_skill_candidates", "list_agent_knowledge_candidates",
		"list_agent_database_candidates", "list_agent_database_tables",
		"list_agent_workflow_binding_candidates":
		return map[string]interface{}{"effect": "read", "asset_type": "agent"}
	default:
		return nil
	}
}

func withOperationPlanTaskID(metadata map[string]interface{}, taskID string) map[string]interface{} {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return metadata
	}
	plan := mapFromOperationContext(metadata["operation_plan"])
	if len(plan) == 0 {
		return metadata
	}
	next := copyStringAnyMap(metadata)
	plan = copyStringAnyMap(plan)
	plan["task_id"] = taskID
	next["operation_plan"] = plan
	return next
}

func applyOperationPlanInvocationState(metadata map[string]interface{}, invocations []map[string]interface{}) {
	plan := mapFromOperationContext(metadata["operation_plan"])
	if len(plan) == 0 {
		return
	}
	steps := mapSliceFromAny(plan["steps"])
	if len(steps) == 0 {
		return
	}
	stepStatus := mapFromOperationContext(plan["step_status"])
	if stepStatus == nil {
		stepStatus = map[string]interface{}{}
	}

	var last map[string]interface{}
	for _, invocation := range invocations {
		if !operationPlanInvocationIsActionable(invocation) {
			continue
		}
		status := operationPlanStatusFromInvocation(invocation)
		applied := false
		if operationPlanInvocationIsConsoleRouteNavigation(invocation) {
			if operationPlanApplyMatchingInvocationState(steps, stepStatus, invocation, status, false) {
				applied = true
			}
			if operationPlanRouteInvocationShouldSetRouteStep(invocation, status) &&
				operationPlanApplyFirstMatchingRouteStep(steps, stepStatus, invocation, status) {
				applied = true
			}
			if operationPlanInvocationShouldUpdateCurrentPage(invocation, status) {
				if href := operationPlanInvocationHref(invocation); href != "" {
					plan["current_page"] = href
					applied = true
				}
			}
		} else if operationPlanApplyMatchingInvocationState(steps, stepStatus, invocation, status, true) {
			applied = true
		}
		if operationPlanInvocationCompletesObservation(invocation, status) {
			operationPlanSetStepStatus(steps, stepStatus, "observe", operationPlanStepStatusCompleted)
			applied = true
		}
		if applied {
			last = invocation
		}
	}

	if last != nil {
		plan["tool_result"] = operationPlanToolResult(last)
		operationPlanAttachOperationGroupResult(plan, last)
	}
	plan["steps"] = mapsToInterfaceSlice(steps)
	plan["step_status"] = stepStatus
	plan["pending_next_action"] = operationPlanPendingNextAction(steps)
	plan["status"] = operationPlanStatusFromSteps(steps)
	metadata["operation_plan"] = plan
}

func ensureOperationPlanInvocationStep(metadata map[string]interface{}, invocation map[string]interface{}) {
	if len(metadata) == 0 || !operationPlanInvocationIsActionable(invocation) {
		return
	}
	plan := mapFromOperationContext(metadata["operation_plan"])
	if len(plan) == 0 {
		return
	}
	skillID := strings.TrimSpace(stringFromAny(invocation["skill_id"]))
	toolName := strings.TrimSpace(stringFromAny(invocation["tool_name"]))
	if skillID == "" || toolName == "" {
		return
	}
	steps := mapSliceFromAny(plan["steps"])
	stepStatus := mapFromOperationContext(plan["step_status"])
	if stepStatus == nil {
		stepStatus = map[string]interface{}{}
	}
	stepID := operationPlanToolStepID(skillID, toolName)
	found := false
	for _, step := range steps {
		if strings.TrimSpace(stringFromAny(step["id"])) == stepID {
			found = true
			break
		}
	}
	if !found {
		step := map[string]interface{}{
			"id":                stepID,
			"title":             operationPlanToolStepTitle(skillID, toolName),
			"status":            operationPlanStepStatusPending,
			"skill_id":          skillID,
			"tool_name":         toolName,
			"required_evidence": operationPlanToolStepEvidence(skillID, toolName),
		}
		if target := operationPlanToolStepAssetTarget(skillID, toolName); len(target) > 0 {
			step["asset_target"] = target
		}
		steps = append(steps, step)
	}

	status := operationPlanStatusFromInvocation(invocation)
	operationPlanApplyMatchingInvocationState(steps, stepStatus, invocation, status, true)
	if operationPlanInvocationCompletesObservation(invocation, status) {
		operationPlanSetStepStatus(steps, stepStatus, "observe", operationPlanStepStatusCompleted)
	}
	plan["tool_result"] = operationPlanToolResult(invocation)
	operationPlanAttachOperationGroupResult(plan, invocation)
	plan["steps"] = mapsToInterfaceSlice(steps)
	plan["step_status"] = stepStatus
	plan["pending_next_action"] = operationPlanPendingNextAction(steps)
	plan["status"] = operationPlanStatusFromSteps(steps)
	metadata["operation_plan"] = plan
}

func amendOperationPlanToolStep(metadata map[string]interface{}, skillID string, toolName string, reason string) {
	if len(metadata) == 0 {
		return
	}
	plan := mapFromOperationContext(metadata["operation_plan"])
	if len(plan) == 0 {
		return
	}
	skillID = strings.TrimSpace(skillID)
	toolName = strings.TrimSpace(toolName)
	if skillID == "" || toolName == "" {
		return
	}

	steps := mapSliceFromAny(plan["steps"])
	stepStatus := mapFromOperationContext(plan["step_status"])
	if stepStatus == nil {
		stepStatus = map[string]interface{}{}
	}
	stepID := operationPlanToolStepID(skillID, toolName)
	found := false
	for _, step := range steps {
		if strings.TrimSpace(stringFromAny(step["id"])) != stepID {
			continue
		}
		found = true
		if operationPlanNormalizeStepStatus(stringFromAny(step["status"])) == "" {
			step["status"] = operationPlanStepStatusPending
		}
		break
	}
	if !found {
		step := map[string]interface{}{
			"id":                stepID,
			"title":             operationPlanToolStepTitle(skillID, toolName),
			"status":            operationPlanStepStatusPending,
			"skill_id":          skillID,
			"tool_name":         toolName,
			"amended":           true,
			"required_evidence": operationPlanToolStepEvidence(skillID, toolName),
		}
		if target := operationPlanToolStepAssetTarget(skillID, toolName); len(target) > 0 {
			step["asset_target"] = target
		}
		steps = append(steps, step)
	}
	if _, ok := stepStatus[stepID]; !ok {
		stepStatus[stepID] = operationPlanStepStatusPending
	}

	reason = strings.TrimSpace(reason)
	amendment := map[string]interface{}{
		"skill_id":  skillID,
		"tool_name": toolName,
		"step_id":   stepID,
	}
	if reason != "" {
		amendment["reason"] = reason
	}
	amendments := mapSliceFromAny(plan["amendments"])
	alreadyRecorded := false
	for _, item := range amendments {
		if strings.TrimSpace(stringFromAny(item["step_id"])) == stepID {
			alreadyRecorded = true
			break
		}
	}
	if !alreadyRecorded {
		amendments = append(amendments, amendment)
	}

	plan["amended"] = true
	plan["amendments"] = mapsToInterfaceSlice(amendments)
	plan["steps"] = mapsToInterfaceSlice(steps)
	plan["step_status"] = stepStatus
	plan["pending_next_action"] = operationPlanPendingNextAction(steps)
	if strings.EqualFold(strings.TrimSpace(stringFromAny(plan["status"])), operationPlanStatusCompleted) {
		plan["status"] = operationPlanStatusRunning
	} else {
		plan["status"] = operationPlanStatusFromSteps(steps)
	}
	metadata["operation_plan"] = plan
}

func amendOperationPlanRepeatedToolStep(metadata map[string]interface{}, skillID string, toolName string, reason string) {
	if len(metadata) == 0 {
		return
	}
	plan := mapFromOperationContext(metadata["operation_plan"])
	if len(plan) == 0 {
		return
	}
	skillID = strings.TrimSpace(skillID)
	toolName = strings.TrimSpace(toolName)
	if skillID == "" || toolName == "" {
		return
	}

	steps := mapSliceFromAny(plan["steps"])
	stepStatus := mapFromOperationContext(plan["step_status"])
	if stepStatus == nil {
		stepStatus = map[string]interface{}{}
	}
	baseID := operationPlanToolStepID(skillID, toolName)
	repeatIndex := 2
	for {
		candidateID := fmt.Sprintf("%s#%d", baseID, repeatIndex)
		exists := false
		for _, step := range steps {
			if strings.TrimSpace(stringFromAny(step["id"])) == candidateID {
				status := operationPlanNormalizeStepStatus(firstNonEmptyString(step["status"], stepStatus[candidateID]))
				if status == "" || status == operationPlanStepStatusPending {
					return
				}
				exists = true
				break
			}
		}
		if !exists {
			step := map[string]interface{}{
				"id":                candidateID,
				"title":             operationPlanToolStepTitle(skillID, toolName),
				"status":            operationPlanStepStatusPending,
				"skill_id":          skillID,
				"tool_name":         toolName,
				"amended":           true,
				"repeat_of":         baseID,
				"required_evidence": operationPlanToolStepEvidence(skillID, toolName),
			}
			if target := operationPlanToolStepAssetTarget(skillID, toolName); len(target) > 0 {
				step["asset_target"] = target
			}
			if reason = strings.TrimSpace(reason); reason != "" {
				step["reason"] = reason
			}
			steps = append(steps, step)
			stepStatus[candidateID] = operationPlanStepStatusPending
			break
		}
		repeatIndex++
	}

	plan["amended"] = true
	plan["steps"] = mapsToInterfaceSlice(steps)
	plan["step_status"] = stepStatus
	plan["pending_next_action"] = operationPlanPendingNextAction(steps)
	if strings.EqualFold(strings.TrimSpace(stringFromAny(plan["status"])), operationPlanStatusCompleted) {
		plan["status"] = operationPlanStatusRunning
	} else {
		plan["status"] = operationPlanStatusFromSteps(steps)
	}
	metadata["operation_plan"] = plan
}

func recordOperationPlanToolDeviation(metadata map[string]interface{}, skillID string, toolName string, reason string) {
	recordOperationPlanToolDeviationWithOutcome(metadata, skillID, toolName, reason, "allowed")
}

func recordOperationPlanToolBlockedDeviation(metadata map[string]interface{}, skillID string, toolName string, reason string) {
	recordOperationPlanToolDeviationWithOutcome(metadata, skillID, toolName, reason, "blocked")
}

func recordOperationPlanToolDeviationWithOutcome(metadata map[string]interface{}, skillID string, toolName string, reason string, outcome string) {
	if len(metadata) == 0 {
		return
	}
	plan := mapFromOperationContext(metadata["operation_plan"])
	if len(plan) == 0 {
		return
	}
	skillID = strings.TrimSpace(skillID)
	toolName = strings.TrimSpace(toolName)
	if skillID == "" {
		return
	}
	deviation := map[string]interface{}{
		"skill_id": skillID,
	}
	if toolName != "" {
		deviation["tool_name"] = toolName
	}
	if reason = strings.TrimSpace(reason); reason != "" {
		deviation["reason"] = reason
	}
	if outcome = strings.TrimSpace(outcome); outcome != "" {
		deviation["outcome"] = outcome
	}
	if target := operationPlanToolStepAssetTarget(skillID, toolName); len(target) > 0 {
		deviation["asset_target"] = target
	}

	deviationKey := "deviations"
	if strings.EqualFold(outcome, "blocked") {
		deviationKey = "blocked_deviations"
	}
	deviations := mapSliceFromAny(plan[deviationKey])
	for _, item := range deviations {
		if strings.EqualFold(strings.TrimSpace(stringFromAny(item["skill_id"])), skillID) &&
			strings.EqualFold(strings.TrimSpace(stringFromAny(item["tool_name"])), toolName) &&
			strings.EqualFold(strings.TrimSpace(stringFromAny(item["reason"])), reason) &&
			strings.EqualFold(strings.TrimSpace(stringFromAny(item["outcome"])), outcome) {
			metadata["operation_plan"] = plan
			return
		}
	}
	deviations = append(deviations, deviation)
	plan[deviationKey] = mapsToInterfaceSlice(deviations)
	metadata["operation_plan"] = plan
}

func operationPlanApplyMatchingInvocationState(steps []map[string]interface{}, stepStatus map[string]interface{}, invocation map[string]interface{}, status string, includeRouteSteps bool) bool {
	applied := false
	for _, step := range steps {
		if !includeRouteSteps && operationPlanStepIsRoute(step) {
			continue
		}
		if !operationPlanStepMatchesInvocation(step, invocation) {
			continue
		}
		if operationPlanSetStepFromInvocation(step, stepStatus, status, invocation) {
			applied = true
		}
	}
	return applied
}

func operationPlanApplyFirstMatchingRouteStep(steps []map[string]interface{}, stepStatus map[string]interface{}, invocation map[string]interface{}, status string) bool {
	for _, step := range steps {
		if !operationPlanStepIsRoute(step) {
			continue
		}
		if !operationPlanStepMatchesInvocation(step, invocation) {
			continue
		}
		if operationPlanStepAlreadyAppliedInvocation(step, invocation, status) {
			return false
		}
		if operationPlanSetStepFromInvocation(step, stepStatus, status, invocation) {
			return true
		}
	}
	return false
}

func operationPlanSetStepFromInvocation(step map[string]interface{}, stepStatus map[string]interface{}, status string, invocation map[string]interface{}) bool {
	id := strings.TrimSpace(stringFromAny(step["id"]))
	if id == "" {
		return false
	}
	current := operationPlanNormalizeStepStatus(firstNonEmptyString(step["status"], stepStatus[id]))
	if current == operationPlanStepStatusCompleted && status != operationPlanStepStatusFailed {
		return false
	}
	if current == operationPlanStepStatusCompleted && status == operationPlanStepStatusFailed {
		return false
	}
	if current == operationPlanStepStatusFailed && status == operationPlanStepStatusPending {
		return false
	}
	if current == status {
		return false
	}
	step["status"] = status
	stepStatus[id] = status
	if invocationID := operationPlanInvocationPlanID(invocation); invocationID != "" {
		step["last_invocation_id"] = invocationID
		step["last_invocation_kind"] = strings.TrimSpace(stringFromAny(invocation["kind"]))
	}
	if group := operationPlanOperationGroupFromInvocation(invocation); len(group) > 0 {
		step["operation_group"] = group
		if targetSet := operationPlanTargetSetFromOperationGroup(group); len(targetSet) > 0 {
			step["target_set"] = targetSet
		}
		if itemSteps := operationPlanItemStepsFromOperationGroup(group); len(itemSteps) > 0 {
			step["item_steps"] = itemSteps
		}
	}
	return true
}

func operationPlanStepAlreadyAppliedInvocation(step map[string]interface{}, invocation map[string]interface{}, status string) bool {
	invocationID := operationPlanInvocationPlanID(invocation)
	if invocationID == "" {
		return false
	}
	if strings.TrimSpace(stringFromAny(step["last_invocation_id"])) != invocationID {
		return false
	}
	return operationPlanNormalizeStepStatus(stringFromAny(step["status"])) == status
}

func operationPlanAttachOperationGroupResult(plan map[string]interface{}, invocation map[string]interface{}) {
	if len(plan) == 0 {
		return
	}
	group := operationPlanOperationGroupFromInvocation(invocation)
	if len(group) == 0 {
		return
	}
	plan["operation_group"] = group
	if targetSet := operationPlanTargetSetFromOperationGroup(group); len(targetSet) > 0 {
		plan["target_set"] = targetSet
	}
	if itemSteps := operationPlanItemStepsFromOperationGroup(group); len(itemSteps) > 0 {
		plan["item_steps"] = itemSteps
	}
	if status := strings.TrimSpace(stringFromAny(group["status"])); status != "" {
		plan["operation_group_status"] = status
	}
}

func operationPlanOperationGroupFromInvocation(invocation map[string]interface{}) map[string]interface{} {
	result := mapFromOperationContext(invocation["result"])
	if len(result) == 0 {
		return nil
	}
	group := mapFromOperationContext(result["operation_group"])
	if len(group) == 0 {
		return nil
	}
	return operationPlanCompactOperationGroup(group)
}

func operationPlanCompactOperationGroup(group map[string]interface{}) map[string]interface{} {
	out := map[string]interface{}{}
	for _, key := range []string{
		"id",
		"type",
		"operation",
		"asset_type",
		"status",
		"target_count",
		"success_count",
		"failed_count",
	} {
		if value, ok := group[key]; ok && value != nil {
			out[key] = value
		}
	}
	if targets := operationPlanCompactOperationItems(group["targets"], 12); len(targets) > 0 {
		out["targets"] = targets
	}
	if results := operationPlanCompactOperationItems(group["item_results"], 20); len(results) > 0 {
		out["item_results"] = results
	}
	return out
}

func operationPlanTargetSetFromOperationGroup(group map[string]interface{}) []interface{} {
	targets := operationPlanCompactOperationItems(group["targets"], 20)
	if len(targets) > 0 {
		return targets
	}
	return operationPlanCompactOperationItems(group["item_results"], 20)
}

func operationPlanItemStepsFromOperationGroup(group map[string]interface{}) []interface{} {
	items := mapSliceFromAny(group["item_results"])
	if len(items) == 0 {
		return nil
	}
	out := make([]interface{}, 0, minInt(len(items), 20))
	for index, item := range items {
		if len(out) >= 20 {
			break
		}
		targetID := firstNonEmptyString(item["agent_id"], item["id"], item["asset_id"], item["resource_id"])
		name := firstNonEmptyString(item["agent_name"], item["name"], item["asset_name"], item["resource_name"])
		status := strings.ToLower(strings.TrimSpace(stringFromAny(item["status"])))
		if status == "" {
			status = operationPlanStepStatusPending
		}
		step := map[string]interface{}{
			"id":     fmt.Sprintf("item:%d", index+1),
			"status": status,
		}
		if targetID != "" {
			step["id"] = "item:" + targetID
			step["target_id"] = targetID
		}
		if name != "" {
			step["target_name"] = name
		}
		if errText := strings.TrimSpace(stringFromAny(item["error"])); errText != "" {
			step["error"] = errText
		}
		out = append(out, step)
	}
	return out
}

func operationPlanCompactOperationItems(value interface{}, limit int) []interface{} {
	items := mapSliceFromAny(value)
	if len(items) == 0 || limit <= 0 {
		return nil
	}
	out := make([]interface{}, 0, minInt(len(items), limit))
	for _, item := range items {
		if len(out) >= limit {
			break
		}
		compact := map[string]interface{}{}
		for _, key := range []string{
			"index",
			"id",
			"agent_id",
			"agent_name",
			"name",
			"type",
			"asset_type",
			"workspace_id",
			"status",
			"effect",
			"href",
			"error",
		} {
			if value, ok := item[key]; ok && value != nil {
				compact[key] = value
			}
		}
		if len(compact) > 0 {
			out = append(out, compact)
		}
	}
	return out
}

func operationPlanInvocationPlanID(invocation map[string]interface{}) string {
	for _, key := range []string{"runtime_id", "action_id", "call_id"} {
		if value := strings.TrimSpace(stringFromAny(invocation[key])); value != "" {
			return key + ":" + value
		}
	}
	kind := strings.TrimSpace(stringFromAny(invocation["kind"]))
	skillID := strings.TrimSpace(stringFromAny(invocation["skill_id"]))
	toolName := strings.TrimSpace(stringFromAny(invocation["tool_name"]))
	href := operationPlanInvocationHref(invocation)
	if kind == "" && skillID == "" && toolName == "" && href == "" {
		return ""
	}
	return strings.Join([]string{kind, skillID, toolName, href}, ":")
}

func applyOperationPlanArtifactState(metadata map[string]interface{}, files []map[string]interface{}) {
	plan := mapFromOperationContext(metadata["operation_plan"])
	if len(plan) == 0 || len(files) == 0 {
		return
	}
	steps := mapSliceFromAny(plan["steps"])
	stepStatus := mapFromOperationContext(plan["step_status"])
	if stepStatus == nil {
		stepStatus = map[string]interface{}{}
	}

	assetFiles := make([]interface{}, 0, minInt(len(files), 8))
	logicalAssets := map[string]struct{}{}
	temporaryCount := 0
	managedCount := 0
	for _, file := range files {
		if len(assetFiles) >= 8 {
			break
		}
		item := compactOperationPlanArtifactFile(file)
		if len(item) == 0 {
			continue
		}
		assetFiles = append(assetFiles, item)
		if isManagedFileArtifact(file) {
			managedCount++
		} else {
			temporaryCount++
		}
		if key := operationPlanArtifactLogicalKey(file); key != "" {
			logicalAssets[key] = struct{}{}
		}
		skillID := strings.TrimSpace(stringFromAny(file["skill_id"]))
		toolName := strings.TrimSpace(stringFromAny(file["tool_name"]))
		if skillID != "" {
			operationPlanSetMatchingStepStatus(steps, stepStatus, skillID, toolName, operationPlanStepStatusCompleted)
		}
	}
	if len(assetFiles) == 0 {
		return
	}
	logicalAssetCount := len(logicalAssets)
	if logicalAssetCount == 0 {
		logicalAssetCount = len(assetFiles)
	}
	unsavedFiles := compactUnsavedOperationPlanGeneratedFiles(files)

	assetState := map[string]interface{}{
		"schema_version":         "operation_plan.asset_state.v1",
		"lifecycle_source":       "generated_files",
		"generated_files":        assetFiles,
		"generated_count":        logicalAssetCount,
		"logical_asset_count":    logicalAssetCount,
		"lifecycle_record_count": len(assetFiles),
		"temporary_count":        temporaryCount,
		"managed_count":          managedCount,
	}
	if len(unsavedFiles) > 0 {
		assetState["unsaved_count"] = len(unsavedFiles)
		assetState["unsaved_generated_files"] = mapsToInterfaceSlice(unsavedFiles)
	}
	plan["asset_state"] = assetState
	operationPlanSetStepStatus(steps, stepStatus, "observe", operationPlanStepStatusCompleted)
	if operationPlanRequiresManagedFileSave(plan, steps) && len(unsavedFiles) > 0 {
		operationPlanSetStepStatus(steps, stepStatus, "skill:"+skills.SkillFileManager, operationPlanStepStatusPending)
		operationPlanSetStepStatus(steps, stepStatus, operationPlanToolStepID(skills.SkillFileManager, "save_file_to_management"), operationPlanStepStatusPending)
	}
	if operationPlanRequiresManagedFileSave(plan, steps) && len(unsavedFiles) == 0 && managedCount > 0 {
		operationPlanSetStepStatus(steps, stepStatus, "skill:"+skills.SkillFileGenerator, operationPlanStepStatusCompleted)
		operationPlanSetStepStatus(steps, stepStatus, "skill:"+skills.SkillChartGenerator, operationPlanStepStatusCompleted)
	}
	plan["steps"] = mapsToInterfaceSlice(steps)
	plan["step_status"] = stepStatus
	plan["pending_next_action"] = operationPlanPendingNextAction(steps)
	if operationPlanRequiresManagedFileSave(plan, steps) && len(unsavedFiles) > 0 {
		plan["pending_next_action"] = "save_remaining_generated_files_to_file_management"
	}
	plan["status"] = operationPlanStatusFromSteps(steps)
	latest := files[len(files)-1]
	plan["tool_result"] = map[string]interface{}{
		"kind":      "artifact",
		"status":    firstNonEmptyString(latest["target"], "generated"),
		"skill_id":  stringFromAny(latest["skill_id"]),
		"tool_name": stringFromAny(latest["tool_name"]),
		"message":   compactForPrompt(firstNonEmptyString(latest["filename"], latest["name"], latest["file_id"]), 240),
	}
	metadata["operation_plan"] = plan
}

func finalizeOperationPlanForResult(metadata map[string]interface{}) {
	plan := mapFromOperationContext(metadata["operation_plan"])
	if len(plan) == 0 {
		return
	}
	steps := mapSliceFromAny(plan["steps"])
	if len(steps) == 0 {
		return
	}
	stepStatus := mapFromOperationContext(plan["step_status"])
	if stepStatus == nil {
		stepStatus = map[string]interface{}{}
	}

	for _, step := range steps {
		if !operationPlanStepBlocksCompletion(step) {
			continue
		}
		status := operationPlanNormalizeStepStatus(firstNonEmptyString(step["status"], stepStatus[stringFromAny(step["id"])]))
		if status == operationPlanStepStatusFailed {
			plan["steps"] = mapsToInterfaceSlice(steps)
			plan["step_status"] = stepStatus
			plan["pending_next_action"] = "none"
			plan["status"] = operationPlanStatusFailed
			metadata["operation_plan"] = plan
			return
		}
		if status == operationPlanStepStatusCompleted {
			continue
		}
		if operationPlanStepRequiresRuntimeAction(step) {
			plan["steps"] = mapsToInterfaceSlice(steps)
			plan["step_status"] = stepStatus
			plan["pending_next_action"] = operationPlanPendingNextAction(steps)
			plan["status"] = operationPlanStatusFromSteps(steps)
			metadata["operation_plan"] = plan
			return
		}
	}

	for _, step := range steps {
		if !operationPlanStepBlocksCompletion(step) {
			continue
		}
		status := operationPlanNormalizeStepStatus(firstNonEmptyString(step["status"], stepStatus[stringFromAny(step["id"])]))
		if status == operationPlanStepStatusCompleted {
			continue
		}
		if operationPlanStepRequiresRuntimeAction(step) {
			continue
		}
		if id := strings.TrimSpace(stringFromAny(step["id"])); id != "" {
			operationPlanSetStepStatus(steps, stepStatus, id, operationPlanStepStatusCompleted)
		}
	}

	plan["steps"] = mapsToInterfaceSlice(steps)
	plan["step_status"] = stepStatus
	plan["pending_next_action"] = operationPlanPendingNextAction(steps)
	plan["status"] = operationPlanStatusFromSteps(steps)
	metadata["operation_plan"] = plan
}

func operationPlanStepRequiresRuntimeAction(step map[string]interface{}) bool {
	if len(step) == 0 {
		return false
	}
	id := strings.TrimSpace(stringFromAny(step["id"]))
	if id == "" {
		return false
	}
	if id == "observe" {
		return false
	}
	if strings.TrimSpace(stringFromAny(step["wait_for"])) != "" || strings.HasPrefix(id, "wait:") {
		return true
	}
	if operationPlanStepIsRoute(step) {
		return true
	}
	return strings.TrimSpace(stringFromAny(step["tool_name"])) != ""
}

func operationPlanRequiresManagedFileSave(plan map[string]interface{}, steps []map[string]interface{}) bool {
	if len(plan) == 0 {
		return false
	}
	intent := strings.TrimSpace(stringFromAny(plan["intent"]))
	if intent == "save_generated_file_to_file_management" {
		return true
	}
	target := mapFromOperationContext(plan["asset_target"])
	if strings.TrimSpace(stringFromAny(target["effect"])) != "create" {
		return false
	}
	for _, step := range steps {
		if strings.TrimSpace(stringFromAny(step["id"])) == "skill:"+skills.SkillFileManager {
			return true
		}
	}
	return false
}

func compactUnsavedOperationPlanGeneratedFiles(files []map[string]interface{}) []map[string]interface{} {
	if len(files) == 0 {
		return nil
	}
	savedSourceIDs := map[string]struct{}{}
	for _, file := range files {
		if !isManagedFileArtifact(file) {
			continue
		}
		sourceID := strings.TrimSpace(firstNonEmptyString(
			file["source_tool_file_id"],
			file["source_file_id"],
			file["tool_file_id"],
		))
		if sourceID != "" {
			savedSourceIDs[sourceID] = struct{}{}
		}
	}
	out := make([]map[string]interface{}, 0)
	for _, file := range files {
		if isManagedFileArtifact(file) {
			continue
		}
		args := generatedArtifactMapSaveArguments(file)
		toolFileID := strings.TrimSpace(stringFromAny(args["tool_file_id"]))
		if toolFileID == "" {
			continue
		}
		if _, saved := savedSourceIDs[toolFileID]; saved {
			continue
		}
		item := map[string]interface{}{}
		for _, key := range []string{"file_id", "tool_file_id", "filename", "extension", "mime_type"} {
			if value := strings.TrimSpace(stringFromAny(firstNonEmptyString(file[key], args[key]))); value != "" {
				item[key] = value
			}
		}
		item["source_type"] = "tool_file"
		if filename := strings.TrimSpace(stringFromAny(args["filename"])); filename != "" {
			item["save_filename"] = filename
		}
		if len(item) > 0 {
			out = append(out, item)
		}
	}
	return out
}

func compactOperationPlanArtifactFile(file map[string]interface{}) map[string]interface{} {
	out := map[string]interface{}{}
	for _, key := range []string{
		"file_id",
		"tool_file_id",
		"upload_file_id",
		"source_tool_file_id",
		"filename",
		"name",
		"extension",
		"mime_type",
		"target",
		"transfer_method",
		"workspace_id",
		"managed_file_id",
		"managed_filename",
	} {
		if value := strings.TrimSpace(stringFromAny(file[key])); value != "" {
			out[key] = compactForPrompt(value, 240)
		}
	}
	if size, ok := file["size"]; ok && size != nil {
		out["size"] = size
	}
	if len(out) == 0 {
		return nil
	}
	if _, ok := out["target"]; !ok {
		if isManagedFileArtifact(file) {
			out["target"] = "managed_file"
		} else {
			out["target"] = "temporary_artifact"
		}
	}
	return out
}

func operationPlanArtifactLogicalKey(file map[string]interface{}) string {
	if len(file) == 0 {
		return ""
	}
	if sourceID := strings.TrimSpace(firstNonEmptyString(file["source_tool_file_id"], file["tool_file_id"], file["source_file_id"])); sourceID != "" {
		return "tool:" + sourceID
	}
	if !isManagedFileArtifact(file) {
		if fileID := strings.TrimSpace(stringFromAny(file["file_id"])); fileID != "" {
			return "tool:" + fileID
		}
	}
	name := strings.ToLower(strings.TrimSpace(firstNonEmptyString(file["filename"], file["name"])))
	if name == "" {
		if fileID := strings.TrimSpace(firstNonEmptyString(file["managed_file_id"], file["upload_file_id"], file["file_id"])); fileID != "" {
			return "managed:" + fileID
		}
		return ""
	}
	size := strings.TrimSpace(stringFromAny(file["size"]))
	return "name:" + name + ":size:" + size
}

func operationPlanSetMatchingStepStatus(steps []map[string]interface{}, stepStatus map[string]interface{}, skillID, toolName string, status string) {
	skillID = strings.TrimSpace(skillID)
	toolName = strings.TrimSpace(toolName)
	if skillID == "" {
		return
	}
	for _, step := range steps {
		if current := strings.TrimSpace(stringFromAny(step["status"])); current == operationPlanStepStatusFailed {
			continue
		}
		if !operationPlanStepMatchesInvocation(step, map[string]interface{}{
			"skill_id":  skillID,
			"tool_name": toolName,
		}) {
			continue
		}
		id := strings.TrimSpace(stringFromAny(step["id"]))
		if id == "" {
			continue
		}
		step["status"] = status
		stepStatus[id] = status
	}
}

func operationPlanInvocationCompletesObservation(invocation map[string]interface{}, status string) bool {
	if status != operationPlanStepStatusCompleted {
		return false
	}
	return strings.TrimSpace(stringFromAny(invocation["kind"])) == "client_action"
}

func operationPlanSetStepStatus(steps []map[string]interface{}, stepStatus map[string]interface{}, id string, status string) {
	id = strings.TrimSpace(id)
	if id == "" {
		return
	}
	for _, step := range steps {
		if strings.TrimSpace(stringFromAny(step["id"])) != id {
			continue
		}
		step["status"] = status
		stepStatus[id] = status
		return
	}
}

func operationPlanInvocationIsActionable(invocation map[string]interface{}) bool {
	switch strings.TrimSpace(stringFromAny(invocation["kind"])) {
	case "guardrail":
		return !operationPlanGuardrailIsPlanningFeedback(invocation)
	case "tool_call", "client_action", "tool_governance":
		return true
	default:
		return false
	}
}

func operationPlanGuardrailIsPlanningFeedback(invocation map[string]interface{}) bool {
	args := mapFromOperationContext(invocation["arguments"])
	return strings.EqualFold(strings.TrimSpace(stringFromAny(args["next_step"])), "continue_planning")
}

func operationPlanStatusFromInvocation(invocation map[string]interface{}) string {
	status := operationPlanNormalizeStepStatus(stringFromAny(invocation["status"]))
	if status != operationPlanStepStatusCompleted {
		return status
	}
	if operationPlanInvocationResultSignalsFailure(invocation) {
		return operationPlanStepStatusFailed
	}
	if !operationPlanInvocationHasCompletionEvidence(invocation) {
		return operationPlanStepStatusPending
	}
	return status
}

func operationPlanNormalizeStepStatus(status string) string {
	status = strings.ToLower(strings.TrimSpace(status))
	switch status {
	case "success", "succeeded", "allowed", "approved", operationPlanStepStatusCompleted:
		return operationPlanStepStatusCompleted
	case "error", "failed", "blocked", "rejected":
		return operationPlanStepStatusFailed
	case "", "next", "pending", "waiting", "waiting_client_action", "loading", "running":
		return operationPlanStepStatusPending
	default:
		return operationPlanStepStatusPending
	}
}

func operationPlanInvocationResultSignalsFailure(invocation map[string]interface{}) bool {
	if strings.TrimSpace(stringFromAny(invocation["error"])) != "" {
		return true
	}
	result := mapFromOperationContext(invocation["result"])
	if len(result) == 0 {
		return false
	}
	if operationPlanNormalizeStepStatus(firstNonEmptyString(result["status"], result["result_status"])) == operationPlanStepStatusFailed {
		return true
	}
	if strings.TrimSpace(stringFromAny(result["error"])) != "" {
		return true
	}
	if strings.EqualFold(strings.TrimSpace(stringFromAny(result["content_status"])), "error") {
		return true
	}
	return false
}

func operationPlanInvocationHasCompletionEvidence(invocation map[string]interface{}) bool {
	if strings.TrimSpace(stringFromAny(invocation["kind"])) != "tool_call" {
		return true
	}
	skillID := strings.TrimSpace(stringFromAny(invocation["skill_id"]))
	toolName := strings.TrimSpace(stringFromAny(invocation["tool_name"]))
	result := mapFromOperationContext(invocation["result"])
	switch {
	case isFileManagerSaveToolCall(skillID, toolName):
		return operationPlanManagedFileResultHasEvidence(result)
	case isFileManagerDeleteToolCall(skillID, toolName):
		return operationPlanDeleteFileResultHasEvidence(result)
	case strings.EqualFold(skillID, skills.SkillFileReader) && strings.EqualFold(toolName, "read_file"):
		return operationPlanReadFileResultHasEvidence(result)
	case strings.EqualFold(skillID, skills.SkillAgentManagement):
		return operationPlanAgentManagementResultHasEvidence(toolName, result)
	default:
		return true
	}
}

func operationPlanManagedFileResultHasEvidence(result map[string]interface{}) bool {
	if len(result) == 0 {
		return false
	}
	file := mapFromOperationContext(result["file"])
	identity := firstNonEmptyString(
		result["managed_file_id"],
		result["upload_file_id"],
		result["file_id"],
		file["managed_file_id"],
		file["upload_file_id"],
		file["file_id"],
		file["id"],
	)
	if identity == "" {
		return false
	}
	name := firstNonEmptyString(
		result["managed_filename"],
		result["filename"],
		result["file_name"],
		result["name"],
		file["managed_filename"],
		file["filename"],
		file["file_name"],
		file["name"],
	)
	if name == "" {
		return false
	}
	target := strings.TrimSpace(firstNonEmptyString(result["target"], file["target"]))
	if target == "" {
		return true
	}
	return strings.EqualFold(target, "managed_file")
}

func operationPlanDeleteFileResultHasEvidence(result map[string]interface{}) bool {
	if len(result) == 0 {
		return false
	}
	if intValueFromAny(result["deleted_count"]) > 0 || operationPlanBoolValue(result["deleted"]) {
		return true
	}
	file := mapFromOperationContext(result["file"])
	return operationPlanNormalizeStepStatus(stringFromAny(result["status"])) == operationPlanStepStatusCompleted &&
		firstNonEmptyString(result["file_id"], result["upload_file_id"], file["file_id"], file["id"]) != ""
}

func operationPlanReadFileResultHasEvidence(result map[string]interface{}) bool {
	if len(result) == 0 {
		return false
	}
	file := mapFromOperationContext(result["file"])
	if firstNonEmptyString(result["file_id"], result["upload_file_id"], file["file_id"], file["id"], file["name"]) == "" {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(stringFromAny(result["content_status"]))) {
	case "empty":
		return true
	case "extracted":
		return strings.TrimSpace(stringFromAny(result["content"])) != "" || intValueFromAny(result["content_chars"]) > 0
	default:
		return false
	}
}

func operationPlanAgentManagementResultHasEvidence(toolName string, result map[string]interface{}) bool {
	toolName = strings.TrimSpace(toolName)
	switch toolName {
	case "create_agent":
		return operationPlanAgentResultID(result) != "" &&
			firstNonEmptyString(result["agent_name"], result["href"], operationPlanAgentResultField(result, "name"), operationPlanAgentResultField(result, "href")) != ""
	case "update_agent_identity":
		return operationPlanAgentResultID(result) != "" && operationPlanValuePresent(result, "updated_fields")
	case "update_agent_config":
		return operationPlanAgentResultID(result) != "" &&
			(operationPlanValuePresent(result, "updated_fields", "config_changes", "binding_changes", "binding_kind", "resource_count", "resource_names") ||
				operationPlanValuePresent(result, "config"))
	case "replace_agent_memory_slots":
		return operationPlanAgentResultID(result) != "" &&
			(operationPlanValuePresent(result, "agent_memory_slots") ||
				operationPlanNestedValuePresent(result, "config", "agent_memory_slots") ||
				operationPlanBoolValue(result["draft_updated"]))
	case "replace_agent_skill_bindings":
		return operationPlanAgentResultID(result) != "" &&
			(operationPlanValuePresent(result, "enabled_skill_ids", "enabled_skill_count", "binding_kind", "resource_count", "resource_names") ||
				operationPlanValuePresent(result, "config"))
	case "replace_agent_knowledge_bindings":
		return operationPlanAgentResultID(result) != "" &&
			(operationPlanValuePresent(result, "knowledge_dataset_ids", "knowledge_dataset_count", "binding_kind", "resource_count", "resource_names") ||
				operationPlanValuePresent(result, "config"))
	case "replace_agent_database_bindings":
		return operationPlanAgentResultID(result) != "" &&
			(operationPlanValuePresent(result, "database_bindings", "database_binding_count", "binding_kind", "resource_count", "resource_names") ||
				operationPlanValuePresent(result, "config"))
	case "replace_agent_workflow_bindings":
		return operationPlanAgentResultID(result) != "" &&
			(operationPlanValuePresent(result, "workflow_bindings", "workflow_binding_count", "binding_kind", "resource_count", "resource_names") ||
				operationPlanValuePresent(result, "config"))
	case "delete_agent":
		return operationPlanAgentResultID(result) != "" &&
			(strings.EqualFold(strings.TrimSpace(stringFromAny(result["effect"])), "deleted") ||
				strings.TrimSpace(stringFromAny(result["route_after_delete"])) != "" ||
				strings.TrimSpace(stringFromAny(result["href"])) != "")
	case "delete_agents":
		return operationPlanBatchOperationResultHasEvidence(result)
	default:
		return true
	}
}

func operationPlanBatchOperationResultHasEvidence(result map[string]interface{}) bool {
	if len(result) == 0 {
		return false
	}
	items := mapSliceFromAny(result["item_results"])
	if len(items) == 0 {
		group := mapFromOperationContext(result["operation_group"])
		items = mapSliceFromAny(group["item_results"])
	}
	if len(items) == 0 {
		return false
	}
	counted := 0
	for _, item := range items {
		status := strings.ToLower(strings.TrimSpace(stringFromAny(item["status"])))
		switch status {
		case "succeeded", "success", "completed", "failed", "skipped", "rejected":
			counted++
		}
	}
	return counted > 0
}

func operationPlanAgentResultID(result map[string]interface{}) string {
	return firstNonEmptyString(result["agent_id"], result["id"], operationPlanAgentResultField(result, "agent_id"), operationPlanAgentResultField(result, "id"))
}

func operationPlanAgentResultField(result map[string]interface{}, key string) string {
	agent := mapFromOperationContext(result["agent"])
	return stringFromAny(agent[key])
}

func operationPlanNestedValuePresent(result map[string]interface{}, nestedKey string, keys ...string) bool {
	nested := mapFromOperationContext(result[nestedKey])
	return operationPlanValuePresent(nested, keys...)
}

func operationPlanValuePresent(values map[string]interface{}, keys ...string) bool {
	if len(values) == 0 {
		return false
	}
	for _, key := range keys {
		if value, ok := values[key]; ok && value != nil {
			return true
		}
	}
	return false
}

func operationPlanBoolValue(value interface{}) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		return strings.EqualFold(strings.TrimSpace(typed), "true") ||
			strings.EqualFold(strings.TrimSpace(typed), "yes") ||
			strings.EqualFold(strings.TrimSpace(typed), "1")
	default:
		return false
	}
}

func operationPlanInvocationIsConsoleRouteNavigation(invocation map[string]interface{}) bool {
	return isConsoleNavigatorNavigateTool(
		stringFromAny(invocation["skill_id"]),
		stringFromAny(invocation["tool_name"]),
	)
}

func operationPlanRouteInvocationShouldSetRouteStep(invocation map[string]interface{}, status string) bool {
	if !operationPlanInvocationIsConsoleRouteNavigation(invocation) {
		return false
	}
	kind := strings.TrimSpace(stringFromAny(invocation["kind"]))
	if kind == "client_action" {
		return status == operationPlanStepStatusCompleted || status == operationPlanStepStatusFailed
	}
	return kind == "tool_call" && status == operationPlanStepStatusFailed
}

func operationPlanInvocationShouldUpdateCurrentPage(invocation map[string]interface{}, status string) bool {
	if status != operationPlanStepStatusCompleted {
		return false
	}
	if strings.TrimSpace(stringFromAny(invocation["kind"])) != "client_action" {
		return false
	}
	return operationPlanInvocationIsConsoleRouteNavigation(invocation)
}

func operationPlanStepMatchesInvocation(step map[string]interface{}, invocation map[string]interface{}) bool {
	stepSkill := strings.TrimSpace(stringFromAny(step["skill_id"]))
	stepTool := strings.TrimSpace(stringFromAny(step["tool_name"]))
	invSkill := strings.TrimSpace(stringFromAny(invocation["skill_id"]))
	invTool := strings.TrimSpace(stringFromAny(invocation["tool_name"]))
	if stepSkill == "" {
		return false
	}
	if !strings.EqualFold(stepSkill, invSkill) {
		return false
	}
	if stepTool != "" &&
		!strings.EqualFold(stepTool, invTool) &&
		!operationPlanStepCoveredByInvocation(stepSkill, stepTool, invSkill, invTool, invocation) {
		return false
	}
	if !operationPlanStepIsRoute(step) {
		return true
	}
	target := operationPlanStepTargetPage(step)
	if target == "" {
		return true
	}
	return consoleNavigationLoadedHrefMatchesTarget(operationPlanInvocationHref(invocation), target)
}

func operationPlanStepCoveredByInvocation(stepSkill, stepTool, invSkill, invTool string, invocation map[string]interface{}) bool {
	if !strings.EqualFold(strings.TrimSpace(stepSkill), strings.TrimSpace(invSkill)) {
		return false
	}
	if strings.EqualFold(strings.TrimSpace(stepSkill), skills.SkillAgentManagement) &&
		strings.EqualFold(strings.TrimSpace(stepTool), "delete_agent") &&
		strings.EqualFold(strings.TrimSpace(invTool), "delete_agents") {
		return operationPlanAgentBatchDeleteInvocationHasEvidence(invocation)
	}
	return false
}

func operationPlanAgentBatchDeleteInvocationHasEvidence(invocation map[string]interface{}) bool {
	result := mapFromOperationContext(invocation["result"])
	if len(result) == 0 || !operationPlanBatchOperationResultHasEvidence(result) {
		return false
	}
	group := mapFromOperationContext(result["operation_group"])
	operation := strings.ToLower(strings.TrimSpace(firstNonEmptyString(result["operation_type"], group["operation"])))
	return operation == "" || operation == "agent.delete" || operation == "agent.delete.batch"
}

func operationPlanStepIsRoute(step map[string]interface{}) bool {
	return strings.HasPrefix(strings.TrimSpace(stringFromAny(step["id"])), "route:")
}

func operationPlanStepTargetPage(step map[string]interface{}) string {
	target := mapFromOperationContext(step["asset_target"])
	return normalizeConsoleNavigationGuardHref(firstNonEmptyString(target["page"], step["href"]))
}

func operationPlanInvocationHref(invocation map[string]interface{}) string {
	if strings.TrimSpace(stringFromAny(invocation["kind"])) == "client_action" {
		if result := mapFromOperationContext(invocation["result"]); len(result) > 0 {
			if href := normalizeConsoleNavigationGuardHref(firstNonEmptyString(result["observed_path"], result["loaded_href"], result["href"], result["target_page"])); href != "" {
				return href
			}
		}
	}
	if href := normalizeConsoleNavigationGuardHref(firstNonEmptyString(invocation["href"], invocation["target_page"])); href != "" {
		return href
	}
	if args := mapFromOperationContext(invocation["arguments"]); len(args) > 0 {
		if href := normalizeConsoleNavigationGuardHref(firstNonEmptyString(args["href"], args["target_page"])); href != "" {
			return href
		}
	}
	if result := mapFromOperationContext(invocation["result"]); len(result) > 0 {
		if href := normalizeConsoleNavigationGuardHref(firstNonEmptyString(result["observed_path"], result["loaded_href"], result["href"], result["target_page"])); href != "" {
			return href
		}
	}
	return ""
}

func operationPlanToolResult(invocation map[string]interface{}) map[string]interface{} {
	result := map[string]interface{}{
		"kind":      stringFromAny(invocation["kind"]),
		"status":    stringFromAny(invocation["status"]),
		"skill_id":  stringFromAny(invocation["skill_id"]),
		"tool_name": stringFromAny(invocation["tool_name"]),
	}
	if message := firstNonEmptyString(invocation["message"], invocation["error"]); message != "" {
		result["message"] = truncateRunes(message, 240)
	}
	if summary := operationPlanResultSummary(invocation); len(summary) > 0 {
		result["result_summary"] = summary
	}
	return result
}

func operationPlanResultSummary(invocation map[string]interface{}) map[string]interface{} {
	payload := mapFromOperationContext(invocation["result"])
	if len(payload) == 0 {
		return nil
	}
	skillID := strings.TrimSpace(stringFromAny(invocation["skill_id"]))
	if strings.EqualFold(skillID, skills.SkillAgentManagement) {
		return operationPlanAgentManagementResultSummary(payload)
	}
	return operationPlanGenericResultSummary(payload)
}

func operationPlanAgentManagementResultSummary(payload map[string]interface{}) map[string]interface{} {
	result := operationPlanCopyFields(payload,
		"status",
		"effect",
		"agent_id",
		"agent_name",
		"workspace_id",
		"href",
		"route_after_delete",
		"updated_fields",
		"model_provider",
		"model",
		"agent_memory_enabled",
		"file_upload",
		"file_upload_enabled",
		"enabled_skill_count",
		"knowledge_dataset_count",
		"database_binding_count",
		"workflow_binding_count",
		"suggested_question_count",
		"model_parameter_count",
		"memory_slot_config_count",
		"knowledge_retrieval_count",
		"binding_kind",
		"change_action",
		"resource_count",
		"resource_names",
		"added_resource_count",
		"added_resource_names",
		"removed_resource_count",
		"removed_resource_names",
		"final_resource_count",
		"final_resource_names",
		"config_changes",
		"binding_changes",
		"draft_updated",
		"reversible",
		"operation_type",
		"operation_group_id",
		"target_count",
		"deleted_count",
		"failed_count",
		"requires_refresh",
		"refresh_target",
		"error",
	)
	if len(result) == 0 {
		return nil
	}
	if group := mapFromOperationContext(payload["operation_group"]); len(group) > 0 {
		result["operation_group"] = operationPlanCompactOperationGroup(group)
	}
	operationPlanAddAgentConfigCounts(result, payload)
	return result
}

func operationPlanGenericResultSummary(payload map[string]interface{}) map[string]interface{} {
	result := operationPlanCopyFields(payload,
		"status",
		"effect",
		"target",
		"file_id",
		"upload_file_id",
		"managed_file_id",
		"tool_file_id",
		"source_file_id",
		"source_tool_file_id",
		"filename",
		"file_name",
		"managed_filename",
		"workspace_id",
		"source_type",
		"mime_type",
		"size",
		"content_status",
		"content_chars",
		"content_truncated",
		"content_error",
		"deleted_count",
		"error",
	)
	if len(result) == 0 {
		return nil
	}
	operationPlanAddNestedFileIdentity(result, payload)
	return result
}

func operationPlanAddAgentConfigCounts(result map[string]interface{}, payload map[string]interface{}) {
	if len(result) == 0 || len(payload) == 0 {
		return
	}
	config := mapFromOperationContext(payload["config"])
	operationPlanAddCollectionCount(result, payload, "enabled_skill_ids", "enabled_skill_count")
	operationPlanAddCollectionCount(result, config, "enabled_skill_ids", "enabled_skill_count")
	operationPlanAddCollectionCount(result, payload, "knowledge_dataset_ids", "knowledge_dataset_count")
	operationPlanAddCollectionCount(result, config, "knowledge_dataset_ids", "knowledge_dataset_count")
	operationPlanAddCollectionCount(result, payload, "database_bindings", "database_binding_count")
	operationPlanAddCollectionCount(result, config, "database_bindings", "database_binding_count")
	operationPlanAddCollectionCount(result, payload, "workflow_bindings", "workflow_binding_count")
	operationPlanAddCollectionCount(result, config, "workflow_bindings", "workflow_binding_count")
	operationPlanAddCollectionCount(result, payload, "suggested_questions", "suggested_question_count")
	operationPlanAddCollectionCount(result, config, "suggested_questions", "suggested_question_count")
	operationPlanAddCollectionCount(result, payload, "agent_memory_slots", "memory_slot_config_count")
	operationPlanAddCollectionCount(result, config, "agent_memory_slots", "memory_slot_config_count")
}

func operationPlanAddCollectionCount(result map[string]interface{}, source map[string]interface{}, collectionKey string, countKey string) {
	if len(result) == 0 || len(source) == 0 {
		return
	}
	if _, exists := result[countKey]; exists {
		return
	}
	count, ok := operationPlanCollectionLen(source[collectionKey])
	if !ok {
		return
	}
	result[countKey] = count
}

func operationPlanAddNestedFileIdentity(result map[string]interface{}, payload map[string]interface{}) {
	if len(result) == 0 || len(payload) == 0 {
		return
	}
	file := mapFromOperationContext(payload["file"])
	if len(file) == 0 {
		return
	}
	operationPlanCopyFirstFileField(result, file, "file_id", "file_id", "id")
	operationPlanCopyFirstFileField(result, file, "upload_file_id", "upload_file_id")
	operationPlanCopyFirstFileField(result, file, "filename", "filename", "file_name", "name")
	operationPlanCopyFirstFileField(result, file, "file_name", "file_name", "name", "filename")
	operationPlanCopyFirstFileField(result, file, "workspace_id", "workspace_id")
	operationPlanCopyFirstFileField(result, file, "mime_type", "mime_type")
	operationPlanCopyFirstFileField(result, file, "extension", "extension")
	if _, exists := result["size"]; !exists {
		if size, ok := file["size"]; ok && size != nil {
			result["size"] = size
		}
	}
}

func operationPlanCopyFirstFileField(result map[string]interface{}, file map[string]interface{}, targetKey string, sourceKeys ...string) {
	if _, exists := result[targetKey]; exists {
		return
	}
	values := make([]interface{}, 0, len(sourceKeys))
	for _, key := range sourceKeys {
		values = append(values, file[key])
	}
	if value := firstNonEmptyString(values...); value != "" {
		result[targetKey] = truncateRunes(value, 240)
	}
}

func operationPlanCollectionLen(value interface{}) (int, bool) {
	if value == nil {
		return 0, false
	}
	switch typed := value.(type) {
	case []interface{}:
		return len(typed), true
	case []string:
		return len(typed), true
	case []map[string]interface{}:
		return len(typed), true
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Slice, reflect.Array:
		return reflected.Len(), true
	default:
		return 0, false
	}
}

func operationPlanCopyFields(payload map[string]interface{}, keys ...string) map[string]interface{} {
	if len(payload) == 0 {
		return nil
	}
	result := map[string]interface{}{}
	seen := map[string]struct{}{}
	for _, key := range keys {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		value, ok := payload[key]
		if !ok || value == nil {
			continue
		}
		if text, ok := value.(string); ok {
			text = strings.TrimSpace(text)
			if text == "" {
				continue
			}
			result[key] = truncateRunes(text, 240)
			continue
		}
		result[key] = value
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func operationPlanStatusFromSteps(steps []map[string]interface{}) string {
	if len(steps) == 0 {
		return operationPlanStatusRunning
	}
	hasBlockingStep := false
	hasPendingStep := false
	for _, step := range steps {
		if !operationPlanStepBlocksCompletion(step) {
			continue
		}
		hasBlockingStep = true
		status := strings.TrimSpace(stringFromAny(step["status"]))
		if status == operationPlanStepStatusFailed {
			return operationPlanStatusFailed
		}
		if status != operationPlanStepStatusCompleted {
			hasPendingStep = true
		}
	}
	if hasBlockingStep && !hasPendingStep {
		return operationPlanStatusCompleted
	}
	return operationPlanStatusRunning
}

func operationPlanStepBlocksCompletion(step map[string]interface{}) bool {
	if len(step) == 0 {
		return false
	}
	id := strings.TrimSpace(stringFromAny(step["id"]))
	if id == "" {
		return false
	}
	if strings.HasPrefix(id, "skill:") &&
		strings.EqualFold(strings.TrimSpace(stringFromAny(step["role"])), "supporting") &&
		strings.TrimSpace(stringFromAny(step["tool_name"])) == "" {
		return false
	}
	return true
}

func interfaceSliceFromMapSlice(input []map[string]interface{}) []interface{} {
	out := make([]interface{}, 0, len(input))
	for _, item := range input {
		out = append(out, item)
	}
	return out
}

func skillsConsoleNavigatorID() string {
	return "console-navigator"
}
