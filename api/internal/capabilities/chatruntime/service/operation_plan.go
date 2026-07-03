package service

import (
	"fmt"
	"reflect"
	"strconv"
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

	operationPlanExpectedUpdatedFieldsKey  = "expected_updated_fields"
	operationPlanExpectedBindingActionsKey = "expected_binding_actions"
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
	targetResource := operationPlanAssetTarget(strategy)
	plan := map[string]interface{}{
		"version":             operationPlanVersion,
		"task_id":             strings.TrimSpace(taskID),
		"original_user_goal":  originalGoal,
		"surface":             normalizeAIChatSurface(parts.Surface),
		"intent":              strategy.Intent,
		"status":              operationPlanStatusRunning,
		"steps":               interfaceSliceFromMapSlice(steps),
		"step_status":         stepStatus,
		"asset_target":        targetResource,
		"target_resource":     targetResource,
		"risk_level":          operationPlanRiskLevel(strategy),
		"approval":            operationPlanApprovalPolicy(strategy),
		"approval_required":   operationPlanRequiresApproval(strategy, steps),
		"approval_actions":    operationPlanApprovalActions(steps),
		"success_criteria":    append([]string(nil), strategy.SuccessCriteria...),
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
	if pageEvidence := operationPlanCompactPageEvidence(skillLoopCompletionPageContextEvidence(parts)); len(pageEvidence) > 0 {
		plan["page_evidence"] = pageEvidence
		plan["current_page_evidence"] = pageEvidence
	}
	applyOperationPlanProgress(plan, steps, stepStatus, "", "")
	return plan
}

func operationPlanRiskLevel(strategy *AIChatTurnStrategy) string {
	if strategy == nil {
		return ""
	}
	return strings.TrimSpace(strategy.AssetRisk)
}

func operationPlanApprovalPolicy(strategy *AIChatTurnStrategy) string {
	if strategy == nil {
		return ""
	}
	return strings.TrimSpace(strategy.Approval)
}

func operationPlanRequiresApproval(strategy *AIChatTurnStrategy, steps []map[string]interface{}) bool {
	for _, step := range steps {
		if operationPlanStepRequiresApproval(step) {
			return true
		}
	}
	if len(steps) > 0 {
		return false
	}
	if strategy != nil {
		approval := strings.ToLower(strings.TrimSpace(strategy.Approval))
		if approval != "" && !strings.Contains(approval, "none") {
			return true
		}
	}
	return false
}

func operationPlanApprovalActions(steps []map[string]interface{}) []string {
	actions := []string{}
	for _, step := range steps {
		if !operationPlanStepRequiresApproval(step) {
			continue
		}
		skillID := strings.TrimSpace(stringFromAny(step["skill_id"]))
		toolName := strings.TrimSpace(stringFromAny(step["tool_name"]))
		if skillID == "" || toolName == "" {
			continue
		}
		actions = appendUniqueStrings(actions, operationPlanToolStepID(skillID, toolName))
	}
	return actions
}

func operationPlanStepRequiresApproval(step map[string]interface{}) bool {
	if !operationPlanStepBlocksCompletion(step) || operationPlanStepIsRoute(step) {
		return false
	}
	toolName := strings.TrimSpace(stringFromAny(step["tool_name"]))
	if toolName == "" {
		return false
	}
	target := mapFromOperationContext(step["asset_target"])
	effect := strings.ToLower(strings.TrimSpace(stringFromAny(target["effect"])))
	if effect != "" {
		return effect != "read"
	}
	return skillLoopToolNameLooksAssetMutation(toolName)
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
	return operationPlanPendingExecutableStepsWithRoutePolicy(plan, limit, true)
}

func operationPlanPendingExecutableStepsForToolExposure(plan map[string]interface{}, limit int) []map[string]interface{} {
	return operationPlanPendingExecutableStepsWithRoutePolicy(plan, limit, false)
}

func operationPlanPendingExecutableStepsWithRoutePolicy(plan map[string]interface{}, limit int, stopAtRoute bool) []map[string]interface{} {
	if len(plan) == 0 || limit <= 0 {
		return nil
	}
	stepStatus := mapFromOperationContext(plan["step_status"])
	out := make([]map[string]interface{}, 0, limit)
	for _, step := range mapSliceFromAny(plan["steps"]) {
		status := operationPlanStepResolvedStatus(step, stepStatus)
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
		if len(out) >= limit || (stopAtRoute && operationPlanStepIsRoute(step)) {
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
		stepID := strings.TrimSpace(strategy.RequiredNextTool.StepID)
		if stepID == "" {
			stepID = operationPlanToolStepID(strategy.RequiredNextTool.SkillID, strategy.RequiredNextTool.ToolName)
		}
		step := map[string]interface{}{
			"id":                stepID,
			"title":             operationPlanToolStepTitle(strategy.RequiredNextTool.SkillID, strategy.RequiredNextTool.ToolName),
			"status":            operationPlanStepStatusPending,
			"skill_id":          strategy.RequiredNextTool.SkillID,
			"tool_name":         strategy.RequiredNextTool.ToolName,
			"required_evidence": operationPlanToolStepEvidence(strategy.RequiredNextTool.SkillID, strategy.RequiredNextTool.ToolName),
		}
		if waitFor := strings.TrimSpace(strategy.RequiredNextTool.WaitForStepID); waitFor != "" {
			step["wait_for"] = waitFor
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
		stepID := strings.TrimSpace(tool.StepID)
		if stepID == "" {
			stepID = operationPlanToolStepID(skillID, toolName)
		}
		step := map[string]interface{}{
			"id":                stepID,
			"title":             operationPlanToolStepTitle(skillID, toolName),
			"status":            operationPlanStepStatusPending,
			"skill_id":          skillID,
			"tool_name":         toolName,
			"required_evidence": operationPlanToolStepEvidence(skillID, toolName),
		}
		if waitFor := strings.TrimSpace(tool.WaitForStepID); waitFor != "" {
			step["wait_for"] = waitFor
		}
		if args := cleanStringMapForOperationPlan(tool.Arguments); len(args) > 0 {
			step["arguments"] = args
			if isConsoleNavigatorNavigateTool(skillID, toolName) {
				if href := strings.TrimSpace(stringFromAny(args["href"])); href != "" {
					step["asset_target"] = map[string]interface{}{"page": href}
				}
			}
		}
		if expected := operationPlanNormalizedAgentConfigFieldsFromAny(tool.Arguments[operationPlanExpectedUpdatedFieldsKey]); len(expected) > 0 {
			step[operationPlanExpectedUpdatedFieldsKey] = expected
			step["field_completion_mode"] = "cumulative"
		}
		if expectedActions := operationPlanAgentConfigBindingActionsFromAny(tool.Arguments[operationPlanExpectedBindingActionsKey]); len(expectedActions) > 0 {
			step[operationPlanExpectedBindingActionsKey] = expectedActions
		}
		if strings.EqualFold(stepID, operationPlanPostUpdateAgentConfigReadStepID()) ||
			strings.EqualFold(stepID, operationPlanPostUpdateAgentIdentityReadStepID()) {
			step["phase"] = "post_update_verification"
			step["required_post_update_verification"] = true
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

func cleanStringMapForOperationPlan(values map[string]string) map[string]interface{} {
	if len(values) == 0 {
		return nil
	}
	out := map[string]interface{}{}
	for key, value := range values {
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" || value == "" {
			continue
		}
		out[key] = value
	}
	if len(out) == 0 {
		return nil
	}
	return out
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

func applyOperationPlanProgress(plan map[string]interface{}, steps []map[string]interface{}, stepStatus map[string]interface{}, pendingOverride string, statusOverride string) {
	if len(plan) == 0 {
		return
	}
	if stepStatus == nil {
		stepStatus = map[string]interface{}{}
	}
	operationPlanApplyAgentConfigPostUpdateClosure(steps, stepStatus)
	operationPlanApplyReadOnlyAgentCandidateLookupClosure(plan, steps, stepStatus)
	operationPlanApplyCompletedAgentMutationClosure(plan, steps, stepStatus)
	operationPlanApplyUnusedSkillStepClosure(steps, stepStatus)
	plan["steps"] = mapsToInterfaceSlice(steps)
	plan["step_status"] = stepStatus
	if pendingOverride != "" {
		plan["pending_next_action"] = pendingOverride
	} else {
		plan["pending_next_action"] = operationPlanPendingNextAction(steps)
	}
	if statusOverride != "" {
		plan["status"] = statusOverride
	} else {
		plan["status"] = operationPlanStatusFromSteps(steps)
	}
	completed, failed := operationPlanProgressStepRecords(steps, stepStatus)
	plan["completed_steps"] = mapsToInterfaceSlice(completed)
	plan["failed_steps"] = mapsToInterfaceSlice(failed)
	operationPlanSyncStrategyState(plan)
}

func operationPlanSyncStrategyState(plan map[string]interface{}) {
	if len(plan) == 0 {
		return
	}
	state := mapFromOperationContext(plan["strategy_state"])
	if state == nil {
		state = map[string]interface{}{}
	}
	state["schema_version"] = "operation_plan.strategy_state.v1"
	operationPlanStrategyStateSetString(state, "user_goal", stringFromAny(plan["original_user_goal"]))
	operationPlanStrategyStateSetString(state, "status", stringFromAny(plan["status"]))
	operationPlanStrategyStateSetString(state, "intent", stringFromAny(plan["intent"]))
	operationPlanStrategyStateSetString(state, "current_page", stringFromAny(plan["current_page"]))
	operationPlanStrategyStateSetString(state, "pending_next_action", stringFromAny(plan["pending_next_action"]))
	operationPlanStrategyStateSetString(state, "risk_level", stringFromAny(plan["risk_level"]))
	operationPlanStrategyStateSetString(state, "approval", stringFromAny(plan["approval"]))
	if value, ok := plan["approval_required"].(bool); ok {
		state["approval_required"] = value
	} else {
		delete(state, "approval_required")
	}
	operationPlanStrategyStateSetStringSlice(state, "approval_actions", stringSliceFromAny(plan["approval_actions"]))
	operationPlanStrategyStateSetStringSlice(state, "success_criteria", stringSliceFromAny(plan["success_criteria"]))
	operationPlanStrategyStateSetStringSlice(state, "completion_criteria", stringSliceFromAny(plan["completion_criteria"]))
	if target := mapFromOperationContext(plan["target_resource"]); len(target) > 0 {
		state["target_resource"] = target
	} else if target := mapFromOperationContext(plan["asset_target"]); len(target) > 0 {
		state["target_resource"] = target
	} else {
		delete(state, "target_resource")
	}
	if pageEvidence := operationPlanCompactPageEvidence(mapFromOperationContext(firstNonNil(plan["current_page_evidence"], plan["page_evidence"]))); len(pageEvidence) > 0 {
		state["current_page_evidence"] = pageEvidence
	} else if currentPage := strings.TrimSpace(stringFromAny(plan["current_page"])); currentPage != "" {
		state["current_page_evidence"] = map[string]interface{}{"current_page": compactForPrompt(currentPage, 300)}
	} else {
		delete(state, "current_page_evidence")
	}
	operationPlanStrategyStateSetInterfaceSlice(state, "plan_steps", operationPlanCompactStepsForPrompt(plan["steps"], 12))
	operationPlanStrategyStateSetInterfaceSlice(state, "completed_steps", operationPlanCompactProgressStepRecords(plan["completed_steps"], 12))
	operationPlanStrategyStateSetInterfaceSlice(state, "failed_steps", operationPlanCompactProgressStepRecords(plan["failed_steps"], 12))
	operationPlanStrategyStateSetInterfaceSlice(state, "plan_deviations", skillLoopCompletionPlanDeviations(plan["deviations"], 12))
	operationPlanStrategyStateSetInterfaceSlice(state, "blocked_deviations", skillLoopCompletionPlanDeviations(plan["blocked_deviations"], 12))
	state["completed_step_count"] = len(mapSliceFromAny(plan["completed_steps"]))
	state["failed_step_count"] = len(mapSliceFromAny(plan["failed_steps"]))
	state["plan_deviation_count"] = len(mapSliceFromAny(plan["deviations"]))
	state["blocked_deviation_count"] = len(mapSliceFromAny(plan["blocked_deviations"]))
	if deviations := mapSliceFromAny(plan["deviations"]); len(deviations) > 0 {
		state["last_plan_deviation"] = deviations[len(deviations)-1]
	} else {
		delete(state, "last_plan_deviation")
	}
	if blocked := mapSliceFromAny(plan["blocked_deviations"]); len(blocked) > 0 {
		state["last_blocked_deviation"] = blocked[len(blocked)-1]
	} else {
		delete(state, "last_blocked_deviation")
	}
	plan["strategy_state"] = state
}

func operationPlanStrategyStateSetString(state map[string]interface{}, key string, value string) {
	value = strings.TrimSpace(value)
	if value == "" {
		delete(state, key)
		return
	}
	state[key] = compactForPrompt(value, 500)
}

func operationPlanStrategyStateSetStringSlice(state map[string]interface{}, key string, values []string) {
	if len(values) == 0 {
		delete(state, key)
		return
	}
	state[key] = compactStringSliceForPrompt(values, 12, 240)
}

func operationPlanStrategyStateSetInterfaceSlice(state map[string]interface{}, key string, values []interface{}) {
	if len(values) == 0 {
		delete(state, key)
		return
	}
	state[key] = values
}

func applyOperationPlanPlannerFeedbackState(metadata map[string]interface{}, traces []skills.SkillTrace) {
	if len(metadata) == 0 || len(traces) == 0 {
		return
	}
	plan := mapFromOperationContext(metadata["operation_plan"])
	if len(plan) == 0 {
		return
	}
	changed := false
	for _, trace := range traces {
		if !strings.EqualFold(strings.TrimSpace(trace.Kind), "planner_feedback") {
			continue
		}
		args := trace.Arguments
		nextStep := strings.TrimSpace(stringFromAny(args["next_step"]))
		reason := strings.TrimSpace(stringFromAny(args["reason"]))
		if reason == "" {
			reason = strings.TrimSpace(trace.Error)
		}
		if reason == "" && nextStep != "" {
			reason = nextStep
		}
		if operationPlanPlannerFeedbackIsMissingAgentTarget(nextStep, reason) {
			applyOperationPlanMissingAgentTargetFeedback(plan, trace, reason)
			changed = true
			continue
		}
		if reason != "" || nextStep != "" {
			operationPlanRecordStrategyFeedback(plan, trace, reason, "advisory")
			changed = true
		}
	}
	if changed {
		metadata["operation_plan"] = plan
	}
}

func operationPlanPlannerFeedbackIsMissingAgentTarget(nextStep string, reason string) bool {
	return strings.EqualFold(strings.TrimSpace(nextStep), "answer_missing_agent_target") ||
		strings.EqualFold(strings.TrimSpace(reason), "agent_target_resolution_exhausted")
}

func applyOperationPlanMissingAgentTargetFeedback(plan map[string]interface{}, trace skills.SkillTrace, reason string) {
	if len(plan) == 0 {
		return
	}
	if reason == "" {
		reason = "agent_target_resolution_exhausted"
	}
	args := trace.Arguments
	targetName := strings.TrimSpace(stringFromAny(args["target_name"]))
	operationPlanRecordStrategyFeedback(plan, trace, reason, "failed")
	applyOperationPlanMissingAgentTargetFailure(plan, trace.SkillID, trace.ToolName, reason, args, targetName, nil, nil)
}

func applyOperationPlanMissingAgentTargetFailure(plan map[string]interface{}, skillID string, toolName string, reason string, evidence map[string]interface{}, targetName string, steps []map[string]interface{}, stepStatus map[string]interface{}) {
	if len(plan) == 0 {
		return
	}
	skillID = strings.TrimSpace(skillID)
	toolName = strings.TrimSpace(toolName)
	if skillID == "" {
		skillID = skills.SkillAgentManagement
	}
	if toolName == "" {
		toolName = "list_agents"
	}
	if reason == "" {
		reason = "agent_target_resolution_exhausted"
	}
	if strings.TrimSpace(targetName) == "" {
		targetName = operationPlanAgentTargetNameFromGoal(plan)
	}
	appendOperationPlanToolDeviation(plan, skillID, toolName, reason, "failed")
	plan["target_resolution"] = operationPlanMissingAgentTargetResolution(evidence, targetName, reason)
	plan["failure_reason"] = reason
	plan["failure_message"] = "target Agent could not be resolved from available list_agents evidence"

	if steps == nil {
		steps = mapSliceFromAny(plan["steps"])
	}
	if stepStatus == nil {
		stepStatus = mapFromOperationContext(plan["step_status"])
	}
	if stepStatus == nil {
		stepStatus = map[string]interface{}{}
	}
	for _, step := range steps {
		if !operationPlanStepIsPendingAgentMutation(step, stepStatus) {
			continue
		}
		id := strings.TrimSpace(stringFromAny(step["id"]))
		if id == "" {
			continue
		}
		step["status"] = operationPlanStepStatusFailed
		step["reason"] = reason
		step["error"] = "target Agent could not be resolved"
		if targetName != "" {
			step["target_name"] = targetName
		}
		stepStatus[id] = operationPlanStepStatusFailed
	}
	applyOperationPlanProgress(plan, steps, stepStatus, "none", operationPlanStatusFailed)
}

func applyOperationPlanMissingAgentTargetFromListEvidence(metadata map[string]interface{}, plan map[string]interface{}, steps []map[string]interface{}, stepStatus map[string]interface{}) bool {
	if len(metadata) == 0 || len(plan) == 0 {
		return false
	}
	if !operationPlanHasPendingAgentMutation(steps, stepStatus) {
		return false
	}
	evidence, ok := operationPlanEmptyAgentListLookupEvidence(metadata["skill_invocations"])
	if !ok {
		return false
	}
	reason := "agent_target_resolution_exhausted"
	trace := skills.SkillTrace{
		Kind:     "planner_feedback",
		SkillID:  skills.SkillAgentManagement,
		ToolName: "list_agents",
		Arguments: map[string]interface{}{
			"next_step": "answer_missing_agent_target",
			"reason":    reason,
		},
	}
	if targetName := operationPlanAgentTargetNameFromGoal(plan); targetName != "" {
		trace.Arguments["target_name"] = targetName
		evidence["target_name"] = targetName
	}
	for _, key := range []string{"previous_list_agents_calls", "empty_result_calls"} {
		if value, exists := evidence[key]; exists {
			trace.Arguments[key] = value
		}
	}
	operationPlanRecordStrategyFeedback(plan, trace, reason, "failed")
	applyOperationPlanMissingAgentTargetFailure(plan, skills.SkillAgentManagement, "list_agents", reason, evidence, stringFromAny(evidence["target_name"]), steps, stepStatus)
	return true
}

func operationPlanHasPendingAgentMutation(steps []map[string]interface{}, stepStatus map[string]interface{}) bool {
	for _, step := range steps {
		if operationPlanStepIsPendingAgentMutation(step, stepStatus) {
			return true
		}
	}
	return false
}

func operationPlanEmptyAgentListLookupEvidence(value interface{}) (map[string]interface{}, bool) {
	invocations := mapSliceFromAny(value)
	if len(invocations) == 0 {
		return nil, false
	}
	total := 0
	empty := 0
	for _, invocation := range invocations {
		if !strings.EqualFold(strings.TrimSpace(stringFromAny(invocation["skill_id"])), skills.SkillAgentManagement) ||
			!strings.EqualFold(strings.TrimSpace(stringFromAny(invocation["tool_name"])), "list_agents") {
			continue
		}
		if operationPlanStatusFromInvocation(invocation) != operationPlanStepStatusCompleted {
			continue
		}
		total++
		if operationPlanListAgentsResultIsEmpty(mapFromOperationContext(invocation["result"])) {
			empty++
		}
	}
	if total < 2 || empty < 2 {
		return nil, false
	}
	return map[string]interface{}{
		"previous_list_agents_calls": total,
		"empty_result_calls":         empty,
		"evidence_source":            "list_agents_results",
	}, true
}

func operationPlanListAgentsResultIsEmpty(result map[string]interface{}) bool {
	if len(result) == 0 {
		return false
	}
	if _, exists := result["count"]; exists {
		return intValueFromAny(result["count"]) == 0
	}
	if _, exists := result["agents_count"]; exists {
		return intValueFromAny(result["agents_count"]) == 0
	}
	if agents, exists := result["agents"]; exists {
		return len(mapSliceFromAny(agents)) == 0
	}
	return false
}

func operationPlanApplyMissingAgentSkillCandidateNoop(plan map[string]interface{}, steps []map[string]interface{}, stepStatus map[string]interface{}, invocations []map[string]interface{}) bool {
	if len(plan) == 0 || len(steps) == 0 {
		return false
	}
	step := operationPlanPendingPureAgentSkillBindingStep(steps, stepStatus)
	if len(step) == 0 {
		return false
	}
	evidence, ok := operationPlanEmptyAgentSkillCandidateLookupEvidence(invocations)
	if !ok {
		return false
	}
	reason := "agent_skill_candidate_not_found"
	appendOperationPlanToolDeviation(plan, skills.SkillAgentManagement, "list_agent_skill_candidates", reason, "advisory")
	plan["target_resolution"] = map[string]interface{}{
		"status":          "not_found",
		"asset_type":      "agent_skill",
		"reason":          reason,
		"evidence_source": "list_agent_skill_candidates",
	}
	if query := strings.TrimSpace(stringFromAny(evidence["query"])); query != "" {
		mapFromOperationContext(plan["target_resolution"])["query"] = query
	}
	id := strings.TrimSpace(stringFromAny(step["id"]))
	if id == "" {
		return false
	}
	step["status"] = operationPlanStepStatusCompleted
	step["skipped_reason"] = reason
	step["evidence_gap"] = "requested Agent Skill candidate was not found; no config mutation is needed"
	if query := strings.TrimSpace(stringFromAny(evidence["query"])); query != "" {
		step["target_query"] = query
	}
	if stepStatus == nil {
		stepStatus = map[string]interface{}{}
	}
	stepStatus[id] = operationPlanStepStatusCompleted
	applyOperationPlanProgress(plan, steps, stepStatus, "none", operationPlanStatusCompleted)
	return true
}

func operationPlanPendingPureAgentSkillBindingStep(steps []map[string]interface{}, stepStatus map[string]interface{}) map[string]interface{} {
	for _, step := range steps {
		if !strings.EqualFold(strings.TrimSpace(stringFromAny(step["skill_id"])), skills.SkillAgentManagement) ||
			!strings.EqualFold(strings.TrimSpace(stringFromAny(step["tool_name"])), "update_agent_config") {
			continue
		}
		status := operationPlanStepResolvedStatus(step, stepStatus)
		if status != operationPlanStepStatusPending {
			continue
		}
		fields := operationPlanNormalizedAgentConfigFieldsFromAny(step[operationPlanExpectedUpdatedFieldsKey])
		if len(fields) != 1 || fields[0] != "enabled_skill_ids" {
			continue
		}
		actions := operationPlanAgentConfigBindingActionsFromAny(step[operationPlanExpectedBindingActionsKey])
		if action := operationPlanCanonicalAgentConfigBindingAction(actions["enabled_skill_ids"]); action == "unbind" {
			continue
		}
		return step
	}
	return nil
}

func operationPlanEmptyAgentSkillCandidateLookupEvidence(invocations []map[string]interface{}) (map[string]interface{}, bool) {
	if len(invocations) == 0 {
		return nil, false
	}
	var last map[string]interface{}
	for _, invocation := range invocations {
		if !strings.EqualFold(strings.TrimSpace(stringFromAny(invocation["skill_id"])), skills.SkillAgentManagement) ||
			!strings.EqualFold(strings.TrimSpace(stringFromAny(invocation["tool_name"])), "list_agent_skill_candidates") {
			continue
		}
		if operationPlanStatusFromInvocation(invocation) != operationPlanStepStatusCompleted {
			continue
		}
		result := mapFromOperationContext(invocation["result"])
		if !operationPlanAgentSkillCandidateResultIsEmpty(result) {
			continue
		}
		evidence := map[string]interface{}{
			"evidence_source": "list_agent_skill_candidates",
		}
		if query := strings.TrimSpace(firstNonEmptyString(result["query"], mapFromOperationContext(invocation["arguments"])["query"])); query != "" {
			evidence["query"] = query
		}
		if agentID := strings.TrimSpace(firstNonEmptyString(result["agent_id"], mapFromOperationContext(invocation["arguments"])["agent_id"])); agentID != "" {
			evidence["agent_id"] = agentID
		}
		last = evidence
	}
	if last == nil {
		return nil, false
	}
	return last, true
}

func operationPlanAgentSkillCandidateResultIsEmpty(result map[string]interface{}) bool {
	if len(result) == 0 {
		return false
	}
	if _, exists := result["count"]; exists {
		return intValueFromAny(result["count"]) == 0
	}
	if skills, exists := result["skills"]; exists {
		return len(mapSliceFromAny(skills)) == 0
	}
	return false
}

func operationPlanAgentTargetNameFromGoal(plan map[string]interface{}) string {
	goal := strings.TrimSpace(stringFromAny(plan["original_user_goal"]))
	if goal == "" {
		return ""
	}
	for _, marker := range []string{
		"删除不存在的智能体",
		"删除不存在的 Agent",
		"删除智能体",
		"删除 Agent",
		"移除智能体",
		"移除 Agent",
		"名为",
		"名称为",
		"叫做",
		"named ",
		"called ",
	} {
		index := strings.Index(strings.ToLower(goal), strings.ToLower(marker))
		if index < 0 {
			continue
		}
		return operationPlanTrimTargetNameToken(goal[index+len(marker):])
	}
	return ""
}

func operationPlanTrimTargetNameToken(value string) string {
	value = strings.TrimSpace(value)
	value = strings.Trim(value, "`\"'“”‘’：:")
	if value == "" {
		return ""
	}
	for _, sep := range []string{" 的", "的智能体", "这个", "，", "。", ",", ".", "；", ";", "\n", "\r", "\t", " "} {
		if index := strings.Index(value, sep); index > 0 {
			value = value[:index]
		}
	}
	return strings.Trim(strings.TrimSpace(value), "`\"'“”‘’：:")
}

func operationPlanStepIsPendingAgentMutation(step map[string]interface{}, stepStatus map[string]interface{}) bool {
	if len(step) == 0 {
		return false
	}
	if !strings.EqualFold(strings.TrimSpace(stringFromAny(step["skill_id"])), skills.SkillAgentManagement) {
		return false
	}
	toolName := strings.TrimSpace(stringFromAny(step["tool_name"]))
	if toolName == "" {
		return false
	}
	target := mapFromOperationContext(step["asset_target"])
	if len(target) == 0 {
		target = operationPlanAgentManagementAssetTarget(toolName)
	}
	if strings.EqualFold(strings.TrimSpace(stringFromAny(target["effect"])), "read") {
		return false
	}
	id := strings.TrimSpace(stringFromAny(step["id"]))
	status := operationPlanNormalizeStepStatus(firstNonEmptyString(step["status"], stepStatus[id]))
	return status != operationPlanStepStatusCompleted && status != operationPlanStepStatusFailed
}

func operationPlanMissingAgentTargetResolution(args map[string]interface{}, targetName string, reason string) map[string]interface{} {
	resolution := map[string]interface{}{
		"status":      "missing",
		"target_type": "agent",
		"reason":      strings.TrimSpace(reason),
	}
	if targetName != "" {
		resolution["target_name"] = compactForPrompt(targetName, 240)
	}
	for _, key := range []string{"previous_list_agents_calls", "empty_result_calls"} {
		if value, ok := args[key]; ok && value != nil {
			resolution[key] = value
		}
	}
	return resolution
}

func operationPlanRecordStrategyFeedback(plan map[string]interface{}, trace skills.SkillTrace, reason string, outcome string) {
	if len(plan) == 0 {
		return
	}
	args := trace.Arguments
	record := map[string]interface{}{
		"skill_id": strings.TrimSpace(trace.SkillID),
		"outcome":  strings.TrimSpace(outcome),
	}
	if toolName := strings.TrimSpace(trace.ToolName); toolName != "" {
		record["tool_name"] = toolName
	}
	if nextStep := strings.TrimSpace(stringFromAny(args["next_step"])); nextStep != "" {
		record["next_step"] = nextStep
	}
	if reason = strings.TrimSpace(reason); reason != "" {
		record["reason"] = reason
	}
	if targetName := strings.TrimSpace(stringFromAny(args["target_name"])); targetName != "" {
		record["target_name"] = compactForPrompt(targetName, 240)
	}
	for _, key := range []string{"previous_list_agents_calls", "empty_result_calls"} {
		if value, ok := args[key]; ok && value != nil {
			record[key] = value
		}
	}
	state := mapFromOperationContext(plan["strategy_state"])
	if state == nil {
		state = map[string]interface{}{}
	}
	feedback := mapSliceFromAny(state["planner_feedback"])
	for _, item := range feedback {
		if strings.EqualFold(strings.TrimSpace(stringFromAny(item["skill_id"])), stringFromAny(record["skill_id"])) &&
			strings.EqualFold(strings.TrimSpace(stringFromAny(item["tool_name"])), stringFromAny(record["tool_name"])) &&
			strings.EqualFold(strings.TrimSpace(stringFromAny(item["next_step"])), stringFromAny(record["next_step"])) &&
			strings.EqualFold(strings.TrimSpace(stringFromAny(item["reason"])), stringFromAny(record["reason"])) &&
			strings.EqualFold(strings.TrimSpace(stringFromAny(item["outcome"])), stringFromAny(record["outcome"])) {
			state["last_feedback"] = item
			plan["strategy_state"] = state
			operationPlanSyncStrategyState(plan)
			return
		}
	}
	feedback = append(feedback, record)
	if len(feedback) > 20 {
		feedback = feedback[len(feedback)-20:]
	}
	state["schema_version"] = "operation_plan.strategy_state.v1"
	state["planner_feedback"] = mapsToInterfaceSlice(feedback)
	state["planner_feedback_count"] = len(feedback)
	state["last_feedback"] = record
	plan["strategy_state"] = state
	operationPlanSyncStrategyState(plan)
}

func operationPlanApplyUnusedSkillStepClosure(steps []map[string]interface{}, stepStatus map[string]interface{}) {
	if len(steps) == 0 || stepStatus == nil {
		return
	}
	hasCompletedEvidenceStep := false
	for _, step := range steps {
		if operationPlanStepIsSkillDeclaration(step) {
			continue
		}
		id := strings.TrimSpace(stringFromAny(step["id"]))
		status := operationPlanNormalizeStepStatus(firstNonEmptyString(step["status"], stepStatus[id]))
		if status == operationPlanStepStatusFailed {
			return
		}
		if operationPlanStepBlocksCompletion(step) && status != operationPlanStepStatusCompleted {
			return
		}
		if status == operationPlanStepStatusCompleted {
			hasCompletedEvidenceStep = true
		}
	}
	if !hasCompletedEvidenceStep {
		return
	}
	for _, step := range steps {
		if !operationPlanStepIsSkillDeclaration(step) {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(stringFromAny(step["role"])), "supporting") {
			continue
		}
		id := strings.TrimSpace(stringFromAny(step["id"]))
		status := operationPlanNormalizeStepStatus(firstNonEmptyString(step["status"], stepStatus[id]))
		if status == operationPlanStepStatusCompleted || status == operationPlanStepStatusFailed {
			continue
		}
		step["status"] = operationPlanStepStatusCompleted
		step["skipped_reason"] = "covered_by_completed_operation"
		stepStatus[id] = operationPlanStepStatusCompleted
	}
}

func operationPlanStepIsSkillDeclaration(step map[string]interface{}) bool {
	if len(step) == 0 {
		return false
	}
	id := strings.TrimSpace(stringFromAny(step["id"]))
	return strings.HasPrefix(id, "skill:") &&
		strings.TrimSpace(stringFromAny(step["tool_name"])) == ""
}

func operationPlanApplyAgentConfigPostUpdateClosure(steps []map[string]interface{}, stepStatus map[string]interface{}) {
	if len(steps) == 0 || stepStatus == nil {
		return
	}
	updateConfigCompleted := operationPlanStepStatusByID(steps, stepStatus, operationPlanToolStepID(skills.SkillAgentManagement, "update_agent_config")) == operationPlanStepStatusCompleted
	updateIdentityCompleted := operationPlanStepStatusByID(steps, stepStatus, operationPlanToolStepID(skills.SkillAgentManagement, "update_agent_identity")) == operationPlanStepStatusCompleted
	if !updateConfigCompleted && !updateIdentityCompleted {
		return
	}
	configReadCompleted := operationPlanStepStatusByID(steps, stepStatus, operationPlanPostUpdateAgentConfigReadStepID()) == operationPlanStepStatusCompleted
	identityReadCompleted := operationPlanStepStatusByID(steps, stepStatus, operationPlanPostUpdateAgentIdentityReadStepID()) == operationPlanStepStatusCompleted
	switch {
	case updateConfigCompleted && !configReadCompleted:
		return
	case updateIdentityCompleted && !updateConfigCompleted && !identityReadCompleted && !configReadCompleted:
		return
	}
	for _, step := range steps {
		if !operationPlanPostUpdateClosureCanCoverStep(step) {
			continue
		}
		id := strings.TrimSpace(stringFromAny(step["id"]))
		if id == "" {
			continue
		}
		status := operationPlanNormalizeStepStatus(firstNonEmptyString(step["status"], stepStatus[id]))
		if status == operationPlanStepStatusCompleted || status == operationPlanStepStatusFailed {
			continue
		}
		step["status"] = operationPlanStepStatusCompleted
		step["skipped_reason"] = "covered_by_post_update_agent_config_read"
		stepStatus[id] = operationPlanStepStatusCompleted
	}
}

func operationPlanApplyReadOnlyAgentCandidateLookupClosure(plan map[string]interface{}, steps []map[string]interface{}, stepStatus map[string]interface{}) {
	if len(plan) == 0 || len(steps) == 0 || stepStatus == nil {
		return
	}
	goal := strings.TrimSpace(firstNonEmptyString(plan["original_user_goal"], plan["user_goal"], plan["goal"]))
	if !operationPlanGoalExplicitlyReadOnlyAgentCandidateLookup(goal) {
		return
	}
	candidateTools := operationPlanAgentCandidateLookupToolsFromSteps(steps)
	if len(candidateTools) == 0 {
		return
	}
	for _, toolName := range candidateTools {
		stepID := operationPlanToolStepID(skills.SkillAgentManagement, toolName)
		if operationPlanStepStatusByID(steps, stepStatus, stepID) != operationPlanStepStatusCompleted {
			return
		}
	}

	for _, step := range steps {
		id := strings.TrimSpace(stringFromAny(step["id"]))
		if id == "" {
			continue
		}
		status := operationPlanNormalizeStepStatus(firstNonEmptyString(step["status"], stepStatus[id]))
		if status == operationPlanStepStatusCompleted || status == operationPlanStepStatusFailed {
			continue
		}
		if id == "observe" {
			step["status"] = operationPlanStepStatusCompleted
			step["skipped_reason"] = "covered_by_read_only_agent_candidate_lookup"
			stepStatus[id] = operationPlanStepStatusCompleted
			continue
		}
		if !strings.EqualFold(strings.TrimSpace(stringFromAny(step["skill_id"])), skills.SkillAgentManagement) {
			continue
		}
		toolName := strings.TrimSpace(stringFromAny(step["tool_name"]))
		if !operationPlanReadOnlyAgentCandidateLookupCanSkipTool(toolName) {
			continue
		}
		step["status"] = operationPlanStepStatusCompleted
		step["skipped_reason"] = "covered_by_read_only_agent_candidate_lookup"
		stepStatus[id] = operationPlanStepStatusCompleted
		if operationPlanAgentManagementToolIsMutation(toolName) {
			appendOperationPlanToolDeviation(
				plan,
				skills.SkillAgentManagement,
				toolName,
				"stale_mutation_plan_skipped_for_read_only_candidate_lookup",
				"skipped",
			)
		}
	}
}

func operationPlanGoalExplicitlyReadOnlyAgentCandidateLookup(goal string) bool {
	query := strings.ToLower(strings.TrimSpace(agentManagementSecondaryIntentQuery(goal)))
	if query == "" {
		return false
	}
	if agentBindingMutationRequested(query) {
		return false
	}
	return agentManagementExplicitNoMutationRequested(query) || agentBindingReadOnlyRequested(query)
}

func operationPlanAgentCandidateLookupToolsFromSteps(steps []map[string]interface{}) []string {
	tools := []string{}
	for _, step := range steps {
		if !strings.EqualFold(strings.TrimSpace(stringFromAny(step["skill_id"])), skills.SkillAgentManagement) {
			continue
		}
		toolName := strings.ToLower(strings.TrimSpace(stringFromAny(step["tool_name"])))
		switch toolName {
		case "list_agent_skill_candidates",
			"list_agent_knowledge_candidates",
			"list_agent_database_candidates",
			"list_agent_database_tables",
			"list_agent_workflow_binding_candidates":
			tools = appendUniqueStrings(tools, toolName)
		}
	}
	return tools
}

func operationPlanReadOnlyAgentCandidateLookupCanSkipTool(toolName string) bool {
	switch strings.ToLower(strings.TrimSpace(toolName)) {
	case "get_agent", "get_agent_config":
		return true
	default:
		return operationPlanAgentManagementToolIsMutation(toolName)
	}
}

func operationPlanApplyCompletedAgentMutationClosure(plan map[string]interface{}, steps []map[string]interface{}, stepStatus map[string]interface{}) {
	if len(plan) == 0 || len(steps) == 0 || stepStatus == nil {
		return
	}
	evidence := operationPlanCompletedAgentMutationEvidence(plan, steps, stepStatus)
	if len(evidence) == 0 || operationPlanHasPendingStrictRuntimeStep(steps, stepStatus) {
		return
	}

	for _, step := range steps {
		id := strings.TrimSpace(stringFromAny(step["id"]))
		if id == "" {
			continue
		}
		status := operationPlanNormalizeStepStatus(firstNonEmptyString(step["status"], stepStatus[id]))
		if status == operationPlanStepStatusCompleted || status == operationPlanStepStatusFailed {
			continue
		}
		if !operationPlanCompletedAgentMutationCanCoverStep(plan, step, evidence) {
			continue
		}
		step["status"] = operationPlanStepStatusCompleted
		step["skipped_reason"] = "covered_by_completed_agent_mutation_result"
		if completedBy := strings.TrimSpace(stringFromAny(evidence["step_id"])); completedBy != "" {
			step["covered_by_step_id"] = completedBy
		}
		stepStatus[id] = operationPlanStepStatusCompleted
		appendOperationPlanToolDeviation(
			plan,
			strings.TrimSpace(stringFromAny(step["skill_id"])),
			strings.TrimSpace(stringFromAny(step["tool_name"])),
			"planned_exploration_covered_by_completed_agent_mutation",
			"covered",
		)
	}
}

func operationPlanCompletedAgentMutationEvidence(plan map[string]interface{}, steps []map[string]interface{}, stepStatus map[string]interface{}) map[string]interface{} {
	if len(plan) == 0 {
		return nil
	}
	toolResult := mapFromOperationContext(plan["tool_result"])
	if !strings.EqualFold(strings.TrimSpace(stringFromAny(toolResult["skill_id"])), skills.SkillAgentManagement) {
		return nil
	}
	toolName := strings.TrimSpace(stringFromAny(toolResult["tool_name"]))
	if !operationPlanAgentManagementToolIsMutation(toolName) ||
		operationPlanNormalizeStepStatus(stringFromAny(toolResult["status"])) != operationPlanStepStatusCompleted {
		return nil
	}
	summary := mapFromOperationContext(toolResult["result_summary"])
	if len(summary) == 0 || !operationPlanAgentManagementResultHasEvidence(toolName, summary) {
		return nil
	}
	stepID := operationPlanToolStepID(skills.SkillAgentManagement, toolName)
	if operationPlanStepStatusByID(steps, stepStatus, stepID) != operationPlanStepStatusCompleted {
		return nil
	}
	return map[string]interface{}{
		"step_id":    stepID,
		"tool_name":  toolName,
		"agent_id":   operationPlanAgentResultID(summary),
		"agent_href": strings.TrimSpace(firstNonEmptyString(summary["href"], summary["route_after_delete"])),
	}
}

func operationPlanAgentManagementToolIsMutation(toolName string) bool {
	switch strings.TrimSpace(toolName) {
	case "create_agent",
		"update_agent_identity",
		"update_agent_config",
		"replace_agent_memory_slots",
		"replace_agent_skill_bindings",
		"replace_agent_knowledge_bindings",
		"replace_agent_database_bindings",
		"replace_agent_workflow_bindings",
		"delete_agent",
		"delete_agents":
		return true
	default:
		return false
	}
}

func operationPlanHasPendingStrictRuntimeStep(steps []map[string]interface{}, stepStatus map[string]interface{}) bool {
	for _, step := range steps {
		if !operationPlanStepBlocksCompletion(step) || !operationPlanStepRequiresStrictCompletionEvidence(step) {
			continue
		}
		id := strings.TrimSpace(stringFromAny(step["id"]))
		status := operationPlanNormalizeStepStatus(firstNonEmptyString(step["status"], stepStatus[id]))
		if status != operationPlanStepStatusCompleted && status != operationPlanStepStatusFailed {
			return true
		}
	}
	return false
}

func operationPlanCompletedAgentMutationCanCoverStep(plan map[string]interface{}, step map[string]interface{}, evidence map[string]interface{}) bool {
	if len(step) == 0 {
		return false
	}
	if strings.TrimSpace(stringFromAny(step["required_post_update_verification"])) != "" ||
		operationPlanBoolValue(step["required_post_update_verification"]) {
		return false
	}
	id := strings.TrimSpace(stringFromAny(step["id"]))
	if id == "observe" {
		return true
	}
	if operationPlanStepIsRoute(step) || isConsoleNavigatorNavigateTool(stringFromAny(step["skill_id"]), stringFromAny(step["tool_name"])) {
		return operationPlanCompletedAgentMutationCanCoverRouteStep(plan, step, evidence)
	}
	if !strings.EqualFold(strings.TrimSpace(stringFromAny(step["skill_id"])), skills.SkillAgentManagement) {
		return false
	}
	toolName := strings.TrimSpace(stringFromAny(step["tool_name"]))
	if toolName == "" || operationPlanAgentManagementToolIsMutation(toolName) {
		return false
	}
	target := mapFromOperationContext(step["asset_target"])
	return strings.EqualFold(strings.TrimSpace(stringFromAny(target["effect"])), "read") ||
		skillLoopToolNameLooksReadOnly(toolName)
}

func operationPlanCompletedAgentMutationCanCoverRouteStep(plan map[string]interface{}, step map[string]interface{}, evidence map[string]interface{}) bool {
	if strings.TrimSpace(stringFromAny(step["wait_for"])) != "" {
		return false
	}
	target := operationPlanStepTargetPage(step)
	if target == "" {
		return true
	}
	if currentPage := strings.TrimSpace(stringFromAny(plan["current_page"])); currentPage != "" &&
		consoleNavigationLoadedHrefMatchesTarget(currentPage, target) {
		return true
	}
	pageEvidence := mapFromOperationContext(plan["current_page_evidence"])
	if len(pageEvidence) == 0 {
		pageEvidence = mapFromOperationContext(plan["page_evidence"])
	}
	for _, key := range []string{"current_page", "runtime_route"} {
		if page := strings.TrimSpace(stringFromAny(pageEvidence[key])); page != "" &&
			consoleNavigationLoadedHrefMatchesTarget(page, target) {
			return true
		}
	}
	agentID := strings.TrimSpace(stringFromAny(evidence["agent_id"]))
	if agentID != "" && strings.Contains(target, "/console/agents/"+agentID+"/") {
		return true
	}
	if href := normalizeConsoleNavigationGuardHref(stringFromAny(evidence["agent_href"])); href != "" &&
		consoleNavigationLoadedHrefMatchesTarget(href, target) {
		return true
	}
	return false
}

func operationPlanStepStatusByID(steps []map[string]interface{}, stepStatus map[string]interface{}, id string) string {
	id = strings.TrimSpace(id)
	if id == "" {
		return ""
	}
	for _, step := range steps {
		if strings.TrimSpace(stringFromAny(step["id"])) != id {
			continue
		}
		return operationPlanStepResolvedStatus(step, stepStatus)
	}
	return operationPlanNormalizeStepStatus(stringFromAny(stepStatus[id]))
}

func operationPlanStepResolvedStatus(step map[string]interface{}, stepStatus map[string]interface{}) string {
	if len(step) == 0 {
		return ""
	}
	if id := strings.TrimSpace(stringFromAny(step["id"])); id != "" {
		if status := operationPlanNormalizeStepStatus(stringFromAny(stepStatus[id])); status != "" {
			return status
		}
	}
	return operationPlanNormalizeStepStatus(stringFromAny(step["status"]))
}

func operationPlanPostUpdateClosureCanCoverStep(step map[string]interface{}) bool {
	if len(step) == 0 {
		return false
	}
	id := strings.TrimSpace(stringFromAny(step["id"]))
	if id == "observe" {
		return true
	}
	if !strings.EqualFold(strings.TrimSpace(stringFromAny(step["skill_id"])), skills.SkillAgentManagement) {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(stringFromAny(step["tool_name"]))) {
	case "get_agent",
		"get_agent_config",
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

func applyOperationPlanPageEvidence(metadata map[string]interface{}, pageEvidence map[string]interface{}) {
	if len(metadata) == 0 || len(pageEvidence) == 0 {
		return
	}
	plan := mapFromOperationContext(metadata["operation_plan"])
	if len(plan) == 0 {
		return
	}
	compact := operationPlanCompactPageEvidence(pageEvidence)
	if len(compact) == 0 {
		return
	}
	plan["page_evidence"] = compact
	plan["current_page_evidence"] = compact
	if currentPage := strings.TrimSpace(stringFromAny(compact["current_page"])); currentPage != "" {
		plan["current_page"] = currentPage
	}
	operationPlanSyncStrategyState(plan)
	metadata["operation_plan"] = plan
}

func operationPlanCompactPageEvidence(pageEvidence map[string]interface{}) map[string]interface{} {
	if len(pageEvidence) == 0 {
		return nil
	}
	out := map[string]interface{}{}
	for _, key := range []string{"current_page", "runtime_route", "route_evidence"} {
		if value := strings.TrimSpace(stringFromAny(pageEvidence[key])); value != "" {
			out[key] = compactForPrompt(value, 300)
		}
	}
	if value, ok := pageEvidence["target_route_already_available"].(bool); ok {
		out["target_route_already_available"] = value
	}
	if target := mapFromOperationContext(pageEvidence["resolved_target_from_user_request"]); len(target) > 0 {
		compactTarget := map[string]interface{}{}
		for _, key := range []string{"href", "label"} {
			if value := strings.TrimSpace(stringFromAny(target[key])); value != "" {
				compactTarget[key] = compactForPrompt(value, 240)
			}
		}
		if len(compactTarget) > 0 {
			out["resolved_target_from_user_request"] = compactTarget
		}
	}
	if resources := operationPlanCompactPageEvidenceResources(pageEvidence["resources"], 12); len(resources) > 0 {
		out["resources"] = resources
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func operationPlanCompactPageEvidenceResources(value interface{}, limit int) []interface{} {
	resources := mapSliceFromAny(value)
	if len(resources) == 0 || limit <= 0 {
		return nil
	}
	out := make([]interface{}, 0, minInt(len(resources), limit))
	for _, resource := range resources {
		if len(out) >= limit {
			break
		}
		compact := map[string]interface{}{}
		for _, key := range []string{
			"index",
			"visible_index",
			"resource_id",
			"resource_type",
			"id",
			"agent_id",
			"agent_name",
			"name",
			"title",
			"type",
			"asset_type",
			"workspace_id",
			"status",
			"href",
			"route",
			"context_ready",
			"files_query_status",
			"agents_query_status",
			"visible_file_count",
			"total_file_count",
			"visible_agent_count",
			"loaded_agent_count",
		} {
			value, ok := resource[key]
			if !ok || value == nil {
				continue
			}
			if text := strings.TrimSpace(stringFromAny(value)); text != "" {
				compact[key] = compactForPrompt(text, 240)
				continue
			}
			compact[key] = value
		}
		if len(compact) > 0 {
			out = append(out, compact)
		}
	}
	return out
}

func operationPlanProgressStepRecords(steps []map[string]interface{}, stepStatus map[string]interface{}) ([]map[string]interface{}, []map[string]interface{}) {
	completed := make([]map[string]interface{}, 0)
	failed := make([]map[string]interface{}, 0)
	for _, step := range steps {
		id := strings.TrimSpace(stringFromAny(step["id"]))
		status := operationPlanNormalizeStepStatus(firstNonEmptyString(step["status"], stepStatus[id]))
		switch status {
		case operationPlanStepStatusCompleted:
			if record := operationPlanProgressStepRecord(step, status); len(record) > 0 {
				completed = append(completed, record)
			}
		case operationPlanStepStatusFailed:
			if record := operationPlanProgressStepRecord(step, status); len(record) > 0 {
				failed = append(failed, record)
			}
		}
	}
	return completed, failed
}

func operationPlanProgressStepRecord(step map[string]interface{}, status string) map[string]interface{} {
	if len(step) == 0 {
		return nil
	}
	id := strings.TrimSpace(stringFromAny(step["id"]))
	if id == "" {
		return nil
	}
	record := map[string]interface{}{
		"id":     id,
		"status": status,
	}
	for _, key := range []string{
		"title",
		"skill_id",
		"tool_name",
		"role",
		"target_page",
		"wait_for",
		"reason",
		"error",
		"last_invocation_id",
		"last_invocation_kind",
	} {
		if value, ok := step[key]; ok && value != nil && strings.TrimSpace(stringFromAny(value)) != "" {
			record[key] = value
		}
	}
	for _, key := range []string{
		"asset_target",
		"operation_group",
		"target_set",
		"item_steps",
	} {
		if value, ok := step[key]; ok && value != nil {
			record[key] = value
		}
	}
	return record
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

func operationPlanPostUpdateAgentConfigReadStepID() string {
	return operationPlanToolStepID(skills.SkillAgentManagement, "get_agent_config") + "#post_update"
}

func operationPlanPostUpdateAgentIdentityReadStepID() string {
	return operationPlanToolStepID(skills.SkillAgentManagement, "get_agent") + "#post_update"
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
			if operationPlanStepHasReadEffect(step) {
				criteria = append(criteria, "Verification read step must have matching execution evidence before claiming confirmation: "+title)
				continue
			}
			criteria = append(criteria, "Asset-changing step must have matching execution evidence before claiming completion: "+title)
		}
	}
	return criteria
}

func operationPlanStepHasReadEffect(step map[string]interface{}) bool {
	target := mapFromOperationContext(step["asset_target"])
	return strings.EqualFold(strings.TrimSpace(stringFromAny(target["effect"])), "read")
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
	if operationPlanBoolValue(step["required_post_update_verification"]) {
		return true
	}
	toolName := strings.TrimSpace(stringFromAny(step["tool_name"]))
	if toolName == "" {
		return false
	}
	target := mapFromOperationContext(step["asset_target"])
	effect := strings.ToLower(strings.TrimSpace(stringFromAny(target["effect"])))
	if effect != "" {
		return effect != "read"
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
	occurrences := map[string]int{}
	for _, invocation := range invocations {
		if !operationPlanInvocationIsActionable(invocation) {
			continue
		}
		status := operationPlanStatusFromInvocation(invocation)
		occurrence := operationPlanInvocationOccurrenceFromReplay(occurrences, invocation, status)
		applied := false
		matchedPlanStep := false
		if operationPlanInvocationIsConsoleRouteNavigation(invocation) {
			if operationPlanApplyMatchingInvocationState(steps, stepStatus, invocation, status, false, occurrence) {
				applied = true
				matchedPlanStep = true
			}
			if operationPlanRouteInvocationShouldSetRouteStep(invocation, status) &&
				operationPlanApplyFirstMatchingRouteStep(steps, stepStatus, invocation, status) {
				applied = true
				matchedPlanStep = true
			}
			if operationPlanInvocationShouldUpdateCurrentPage(invocation, status) {
				if href := operationPlanInvocationHref(invocation); href != "" {
					plan["current_page"] = href
					applied = true
				}
			}
		} else if operationPlanApplyMatchingInvocationState(steps, stepStatus, invocation, status, true, occurrence) {
			applied = true
			matchedPlanStep = true
		}
		if operationPlanInvocationCompletesObservation(invocation, status) {
			operationPlanSetStepStatus(steps, stepStatus, "observe", operationPlanStepStatusCompleted)
			applied = true
		}
		if operationPlanInvocationShouldRecordDeviation(invocation, matchedPlanStep) {
			appendOperationPlanToolDeviation(
				plan,
				strings.TrimSpace(stringFromAny(invocation["skill_id"])),
				strings.TrimSpace(stringFromAny(invocation["tool_name"])),
				operationPlanInvocationDeviationReason(invocation),
				operationPlanInvocationDeviationOutcome(status),
			)
		}
		if applied {
			last = invocation
		}
		operationPlanAdvanceInvocationOccurrence(occurrences, invocation, status)
	}

	if last != nil {
		plan["tool_result"] = operationPlanToolResult(last)
		operationPlanAttachOperationGroupResult(plan, last)
	}
	if operationPlanApplyMissingAgentSkillCandidateNoop(plan, steps, stepStatus, invocations) {
		metadata["operation_plan"] = plan
		return
	}
	applyOperationPlanProgress(plan, steps, stepStatus, "", "")
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
	status := operationPlanStatusFromInvocation(invocation)
	occurrence := operationPlanInvocationOccurrenceFromCurrentSteps(steps, stepStatus, invocation, status)
	matchedExistingStep := operationPlanApplyMatchingInvocationState(steps, stepStatus, invocation, status, true, occurrence)
	if !found && !matchedExistingStep && operationPlanInvocationIsExploratoryDeviation(invocation) {
		appendOperationPlanToolDeviation(
			plan,
			skillID,
			toolName,
			operationPlanInvocationDeviationReason(invocation),
			operationPlanInvocationDeviationOutcome(status),
		)
		if operationPlanInvocationShouldUpdateCurrentPage(invocation, status) {
			if href := operationPlanInvocationHref(invocation); href != "" {
				plan["current_page"] = href
			}
		}
		if operationPlanInvocationCompletesObservation(invocation, status) {
			operationPlanSetStepStatus(steps, stepStatus, "observe", operationPlanStepStatusCompleted)
		}
		plan["tool_result"] = operationPlanToolResult(invocation)
		operationPlanAttachOperationGroupResult(plan, invocation)
		applyOperationPlanProgress(plan, steps, stepStatus, "", "")
		metadata["operation_plan"] = plan
		return
	}
	if !found && !matchedExistingStep {
		amendmentReason := operationPlanInvocationAmendmentReason(invocation)
		step := map[string]interface{}{
			"id":                stepID,
			"title":             operationPlanToolStepTitle(skillID, toolName),
			"status":            operationPlanStepStatusPending,
			"skill_id":          skillID,
			"tool_name":         toolName,
			"amended":           true,
			"reason":            amendmentReason,
			"required_evidence": operationPlanToolStepEvidence(skillID, toolName),
		}
		if target := operationPlanToolStepAssetTarget(skillID, toolName); len(target) > 0 {
			step["asset_target"] = target
		}
		steps = append(steps, step)
		appendOperationPlanToolAmendment(plan, skillID, toolName, stepID, amendmentReason)
	}

	if !matchedExistingStep {
		operationPlanApplyMatchingInvocationState(steps, stepStatus, invocation, status, true, occurrence)
	}
	if operationPlanInvocationCompletesObservation(invocation, status) {
		operationPlanSetStepStatus(steps, stepStatus, "observe", operationPlanStepStatusCompleted)
	}
	plan["tool_result"] = operationPlanToolResult(invocation)
	operationPlanAttachOperationGroupResult(plan, invocation)
	applyOperationPlanProgress(plan, steps, stepStatus, "", "")
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

	appendOperationPlanToolAmendment(plan, skillID, toolName, stepID, reason)
	if strings.EqualFold(strings.TrimSpace(stringFromAny(plan["status"])), operationPlanStatusCompleted) {
		applyOperationPlanProgress(plan, steps, stepStatus, "", operationPlanStatusRunning)
	} else {
		applyOperationPlanProgress(plan, steps, stepStatus, "", "")
	}
	metadata["operation_plan"] = plan
}

func appendOperationPlanToolAmendment(plan map[string]interface{}, skillID string, toolName string, stepID string, reason string) {
	if len(plan) == 0 {
		return
	}
	skillID = strings.TrimSpace(skillID)
	stepID = strings.TrimSpace(stepID)
	if skillID == "" || stepID == "" {
		return
	}
	amendment := map[string]interface{}{
		"skill_id": skillID,
		"step_id":  stepID,
	}
	if toolName = strings.TrimSpace(toolName); toolName != "" {
		amendment["tool_name"] = toolName
	}
	if reason = strings.TrimSpace(reason); reason != "" {
		amendment["reason"] = reason
	}
	amendments := mapSliceFromAny(plan["amendments"])
	for _, item := range amendments {
		if strings.TrimSpace(stringFromAny(item["step_id"])) == stepID {
			plan["amended"] = true
			plan["amendments"] = mapsToInterfaceSlice(amendments)
			return
		}
	}
	amendments = append(amendments, amendment)
	plan["amended"] = true
	plan["amendments"] = mapsToInterfaceSlice(amendments)
}

func operationPlanInvocationAmendmentReason(invocation map[string]interface{}) string {
	if operationPlanStatusFromInvocation(invocation) == operationPlanStepStatusFailed {
		return "runtime_recorded_unplanned_failed_tool_step"
	}
	return "runtime_recorded_unplanned_tool_step"
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
	if strings.EqualFold(strings.TrimSpace(stringFromAny(plan["status"])), operationPlanStatusCompleted) {
		applyOperationPlanProgress(plan, steps, stepStatus, "", operationPlanStatusRunning)
	} else {
		applyOperationPlanProgress(plan, steps, stepStatus, "", "")
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
	appendOperationPlanToolDeviation(plan, skillID, toolName, reason, outcome)
	metadata["operation_plan"] = plan
}

func appendOperationPlanToolDeviation(plan map[string]interface{}, skillID string, toolName string, reason string, outcome string) {
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
	reason = strings.TrimSpace(reason)
	if reason != "" {
		deviation["reason"] = reason
	}
	outcome = strings.TrimSpace(outcome)
	if outcome != "" {
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
			return
		}
	}
	deviations = append(deviations, deviation)
	plan[deviationKey] = mapsToInterfaceSlice(deviations)
	operationPlanSyncStrategyState(plan)
}

func operationPlanInvocationShouldRecordDeviation(invocation map[string]interface{}, matchedPlanStep bool) bool {
	if matchedPlanStep {
		return false
	}
	if len(invocation) == 0 {
		return false
	}
	return operationPlanInvocationIsExploratoryDeviation(invocation) ||
		operationPlanInvocationIsConsoleRouteNavigation(invocation)
}

func operationPlanInvocationIsExploratoryDeviation(invocation map[string]interface{}) bool {
	skillID := strings.TrimSpace(stringFromAny(invocation["skill_id"]))
	toolName := strings.TrimSpace(stringFromAny(invocation["tool_name"]))
	if skillID == "" && toolName == "" {
		return false
	}
	if operationPlanInvocationIsConsoleRouteNavigation(invocation) {
		return true
	}
	target := operationPlanToolStepAssetTarget(skillID, toolName)
	effect := strings.ToLower(strings.TrimSpace(stringFromAny(target["effect"])))
	if effect == "read" {
		return true
	}
	return skillLoopToolNameLooksReadOnly(toolName)
}

func operationPlanInvocationDeviationReason(invocation map[string]interface{}) string {
	if operationPlanInvocationIsConsoleRouteNavigation(invocation) {
		return "model_navigated_for_page_context_within_user_goal"
	}
	return "model_collected_unplanned_readonly_evidence"
}

func operationPlanInvocationDeviationOutcome(status string) string {
	switch operationPlanNormalizeStepStatus(status) {
	case operationPlanStepStatusFailed:
		return "failed"
	case operationPlanStepStatusPending:
		return "pending"
	default:
		return "allowed"
	}
}

func operationPlanApplyMatchingInvocationState(steps []map[string]interface{}, stepStatus map[string]interface{}, invocation map[string]interface{}, status string, includeRouteSteps bool, occurrence int) bool {
	applied := false
	appliedStepIDs := map[string]bool{}
	for _, step := range steps {
		if !includeRouteSteps && operationPlanStepIsRoute(step) {
			continue
		}
		if !operationPlanStepOccurrenceMatchesInvocation(step, occurrence) {
			continue
		}
		if !operationPlanStepMatchesInvocation(step, invocation) {
			continue
		}
		if !operationPlanStepWaitForSatisfied(step, steps, stepStatus, invocation, appliedStepIDs) {
			continue
		}
		if operationPlanSetStepFromInvocation(step, stepStatus, status, invocation) {
			applied = true
			if id := strings.TrimSpace(stringFromAny(step["id"])); id != "" {
				appliedStepIDs[id] = true
			}
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

func operationPlanInvocationOccurrenceFromReplay(occurrences map[string]int, invocation map[string]interface{}, status string) int {
	key := operationPlanInvocationOccurrenceKey(invocation)
	if key == "" {
		return 0
	}
	completed := occurrences[key]
	if status == operationPlanStepStatusCompleted &&
		strings.EqualFold(strings.TrimSpace(stringFromAny(invocation["kind"])), "tool_governance") &&
		completed > 0 {
		return completed
	}
	return completed + 1
}

func operationPlanAdvanceInvocationOccurrence(occurrences map[string]int, invocation map[string]interface{}, status string) {
	if occurrences == nil || status != operationPlanStepStatusCompleted {
		return
	}
	if !strings.EqualFold(strings.TrimSpace(stringFromAny(invocation["kind"])), "tool_call") {
		return
	}
	if !operationPlanInvocationHasCompletionEvidence(invocation) {
		return
	}
	key := operationPlanInvocationOccurrenceKey(invocation)
	if key == "" {
		return
	}
	occurrences[key]++
}

func operationPlanInvocationOccurrenceFromCurrentSteps(steps []map[string]interface{}, stepStatus map[string]interface{}, invocation map[string]interface{}, status string) int {
	key := operationPlanInvocationOccurrenceKey(invocation)
	if key == "" {
		return 0
	}
	completed := 0
	for _, step := range steps {
		if operationPlanInvocationOccurrenceKey(step) != key {
			continue
		}
		id := strings.TrimSpace(stringFromAny(step["id"]))
		if operationPlanNormalizeStepStatus(firstNonEmptyString(step["status"], stepStatus[id])) != operationPlanStepStatusCompleted {
			continue
		}
		index := operationPlanStepRepeatIndex(step)
		if index <= 0 {
			index = 1
		}
		if index > completed {
			completed = index
		}
	}
	if status == operationPlanStepStatusCompleted &&
		strings.EqualFold(strings.TrimSpace(stringFromAny(invocation["kind"])), "tool_governance") &&
		completed > 0 {
		return completed
	}
	return completed + 1
}

func operationPlanInvocationOccurrenceKey(value map[string]interface{}) string {
	skillID := strings.ToLower(strings.TrimSpace(stringFromAny(value["skill_id"])))
	toolName := strings.ToLower(strings.TrimSpace(stringFromAny(value["tool_name"])))
	if skillID == "" || toolName == "" {
		return ""
	}
	return skillID + "/" + toolName
}

func operationPlanStepOccurrenceMatchesInvocation(step map[string]interface{}, occurrence int) bool {
	if occurrence <= 0 {
		return true
	}
	repeatIndex := operationPlanStepRepeatIndex(step)
	if repeatIndex <= 0 {
		return true
	}
	return repeatIndex == occurrence
}

func operationPlanStepRepeatIndex(step map[string]interface{}) int {
	if !operationPlanStepIsRepeatedToolStep(step) {
		return 0
	}
	id := strings.TrimSpace(stringFromAny(step["id"]))
	if id == "" {
		return 0
	}
	hash := strings.LastIndex(id, "#")
	if hash < 0 || hash == len(id)-1 {
		return 0
	}
	index, err := strconv.Atoi(id[hash+1:])
	if err != nil || index < 2 {
		return 0
	}
	return index
}

func operationPlanStepIsRepeatedToolStep(step map[string]interface{}) bool {
	return strings.TrimSpace(stringFromAny(step["repeat_of"])) != ""
}

func operationPlanSetStepFromInvocation(step map[string]interface{}, stepStatus map[string]interface{}, status string, invocation map[string]interface{}) bool {
	id := strings.TrimSpace(stringFromAny(step["id"]))
	if id == "" {
		return false
	}
	trackingChanged := false
	if status == operationPlanStepStatusCompleted {
		var trackedStatus string
		trackedStatus, trackingChanged = operationPlanTrackExpectedUpdatedFields(step, invocation)
		if trackedStatus != "" {
			status = trackedStatus
		}
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
	if current == status && !trackingChanged {
		if status == operationPlanStepStatusPending && operationPlanStepIsRepeatedToolStep(step) {
			return operationPlanUpdateStepInvocationMarker(step, stepStatus, id, status, invocation)
		}
		return false
	}
	step["status"] = status
	stepStatus[id] = status
	operationPlanUpdateStepInvocationMarker(step, stepStatus, id, status, invocation)
	if errText := operationPlanInvocationError(invocation); errText != "" {
		step["error"] = errText
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

func operationPlanUpdateStepInvocationMarker(step map[string]interface{}, stepStatus map[string]interface{}, id string, status string, invocation map[string]interface{}) bool {
	changed := false
	if invocationID := operationPlanInvocationPlanID(invocation); invocationID != "" {
		if strings.TrimSpace(stringFromAny(step["last_invocation_id"])) != invocationID {
			step["last_invocation_id"] = invocationID
			changed = true
		}
		kind := strings.TrimSpace(stringFromAny(invocation["kind"]))
		if strings.TrimSpace(stringFromAny(step["last_invocation_kind"])) != kind {
			step["last_invocation_kind"] = kind
			changed = true
		}
	}
	if sequence := operationPlanInvocationSequence(invocation); sequence > 0 {
		if intValueFromAny(step["last_invocation_sequence"]) != sequence {
			step["last_invocation_sequence"] = sequence
			changed = true
		}
	}
	if stepStatus != nil && id != "" && strings.TrimSpace(stringFromAny(stepStatus[id])) != status {
		stepStatus[id] = status
		changed = true
	}
	return changed
}

func operationPlanInvocationSequence(invocation map[string]interface{}) int {
	for _, key := range []string{"runtime_id", "action_id", "call_id"} {
		sequence := operationPlanSequenceFromIdentifier(stringFromAny(invocation[key]))
		if sequence > 0 {
			return sequence
		}
	}
	return operationPlanSequenceFromIdentifier(operationPlanInvocationPlanID(invocation))
}

func operationPlanSequenceFromIdentifier(identifier string) int {
	identifier = strings.TrimSpace(identifier)
	if identifier == "" {
		return 0
	}
	hash := strings.LastIndex(identifier, "#")
	if hash < 0 || hash == len(identifier)-1 {
		return 0
	}
	sequence, err := strconv.Atoi(identifier[hash+1:])
	if err != nil || sequence <= 0 {
		return 0
	}
	return sequence
}

func operationPlanTrackExpectedUpdatedFields(step map[string]interface{}, invocation map[string]interface{}) (string, bool) {
	expected := operationPlanNormalizedAgentConfigFieldsFromAny(step[operationPlanExpectedUpdatedFieldsKey])
	if len(expected) == 0 || !operationPlanInvocationIsAgentConfigUpdate(invocation) {
		return "", false
	}
	result := mapFromOperationContext(invocation["result"])
	if len(result) == 0 {
		return operationPlanStepStatusPending, false
	}
	resultFields := operationPlanAgentConfigFieldsFromResult(result)
	expectedActions := operationPlanAgentConfigBindingActionsFromAny(step[operationPlanExpectedBindingActionsKey])
	actualActions := operationPlanAgentConfigBindingActionsFromResult(result)
	var actionMismatches []string
	if len(expectedActions) > 0 {
		resultFields, actionMismatches = operationPlanFilterAgentConfigFieldsByExpectedActions(resultFields, expectedActions, actualActions, result)
	}
	completed := operationPlanNormalizedAgentConfigFieldsFromAny(step["completed_updated_fields"])
	beforeKey := strings.Join(completed, ",")
	completed = appendOperationPlanFields(completed, resultFields...)
	missing := missingOperationPlanFields(expected, completed)

	completedActions := operationPlanAgentConfigBindingActionsFromAny(step["completed_binding_actions"])
	if completedActions == nil {
		completedActions = map[string]string{}
	}
	actionBeforeKey := operationPlanEncodeAgentConfigBindingActions(completedActions)
	for field, expectedAction := range expectedActions {
		if action, ok := actualActions[field]; ok && operationPlanBindingActionMatches(expectedAction, action) {
			completedActions[field] = expectedAction
			continue
		}
		if operationPlanAgentConfigBindingFinalStateSatisfiesAction(result, field, expectedAction) {
			completedActions[field] = expectedAction
		}
	}

	changed := beforeKey != strings.Join(completed, ",") ||
		actionBeforeKey != operationPlanEncodeAgentConfigBindingActions(completedActions)
	if len(completed) > 0 {
		step["completed_updated_fields"] = completed
	}
	if len(completedActions) > 0 {
		step["completed_binding_actions"] = completedActions
	}
	if len(missing) > 0 {
		step["missing_updated_fields"] = missing
		if len(actionMismatches) > 0 {
			step["binding_action_mismatch"] = actionMismatches
			step["evidence_gap"] = "missing requested agent config fields or binding actions: " + strings.Join(missing, ", ")
		} else {
			delete(step, "binding_action_mismatch")
			step["evidence_gap"] = "missing requested agent config fields: " + strings.Join(missing, ", ")
		}
		return operationPlanStepStatusPending, true
	}
	delete(step, "missing_updated_fields")
	delete(step, "binding_action_mismatch")
	delete(step, "evidence_gap")
	return operationPlanStepStatusCompleted, changed
}

func operationPlanInvocationIsAgentConfigUpdate(invocation map[string]interface{}) bool {
	return strings.EqualFold(strings.TrimSpace(stringFromAny(invocation["skill_id"])), skills.SkillAgentManagement) &&
		strings.EqualFold(strings.TrimSpace(stringFromAny(invocation["tool_name"])), "update_agent_config")
}

func operationPlanAgentConfigFieldsFromResult(result map[string]interface{}) []string {
	fields := []string{}
	for _, field := range operationPlanStringListFromAny(result["updated_fields"]) {
		if canonical := operationPlanAgentConfigCanonicalField(field); canonical != "" {
			fields = appendUniqueStrings(fields, canonical)
		}
	}
	for _, field := range operationPlanStringListFromAny(result["satisfied_fields"]) {
		if canonical := operationPlanAgentConfigCanonicalField(field); canonical != "" {
			fields = appendUniqueStrings(fields, canonical)
		}
	}
	for _, change := range mapSliceFromAny(result["config_changes"]) {
		if canonical := operationPlanAgentConfigCanonicalField(firstNonEmptyString(change["field"], change["binding_kind"])); canonical != "" {
			fields = appendUniqueStrings(fields, canonical)
		}
	}
	for _, change := range mapSliceFromAny(result["binding_changes"]) {
		if canonical := operationPlanAgentConfigCanonicalField(firstNonEmptyString(change["field"], change["binding_kind"])); canonical != "" {
			fields = appendUniqueStrings(fields, canonical)
		}
	}
	for _, state := range mapSliceFromAny(result["binding_final_states"]) {
		if canonical := operationPlanAgentConfigCanonicalField(firstNonEmptyString(state["field"], state["binding_kind"])); canonical != "" {
			fields = appendUniqueStrings(fields, canonical)
		}
	}
	return fields
}

func operationPlanAgentConfigBindingActionsFromResult(result map[string]interface{}) map[string]string {
	actions := map[string]string{}
	add := func(field string, action string) {
		canonicalField := operationPlanAgentConfigCanonicalField(field)
		canonicalAction := operationPlanCanonicalAgentConfigBindingAction(action)
		if canonicalField == "" || canonicalAction == "" {
			return
		}
		actions[canonicalField] = canonicalAction
	}
	for _, change := range mapSliceFromAny(result["binding_changes"]) {
		add(firstNonEmptyString(change["field"], change["binding_kind"]), firstNonEmptyString(change["change_action"], change["action"]))
	}
	for _, change := range mapSliceFromAny(result["config_changes"]) {
		add(firstNonEmptyString(change["field"], change["binding_kind"]), firstNonEmptyString(change["change_action"], change["action"]))
	}
	add(firstNonEmptyString(result["field"], result["binding_kind"]), firstNonEmptyString(result["change_action"], result["action"]))
	return actions
}

func operationPlanFilterAgentConfigFieldsByExpectedActions(fields []string, expectedActions map[string]string, actualActions map[string]string, result map[string]interface{}) ([]string, []string) {
	if len(expectedActions) == 0 {
		return fields, nil
	}
	filtered := []string{}
	mismatches := []string{}
	for _, field := range fields {
		canonicalField := operationPlanAgentConfigCanonicalField(field)
		if canonicalField == "" {
			continue
		}
		expectedAction := strings.TrimSpace(expectedActions[canonicalField])
		if expectedAction == "" {
			filtered = appendUniqueStrings(filtered, canonicalField)
			continue
		}
		actualAction := strings.TrimSpace(actualActions[canonicalField])
		if operationPlanBindingActionMatches(expectedAction, actualAction) {
			filtered = appendUniqueStrings(filtered, canonicalField)
			continue
		}
		if actualAction == "" {
			if operationPlanAgentConfigBindingFinalStateSatisfiesAction(result, canonicalField, expectedAction) {
				filtered = appendUniqueStrings(filtered, canonicalField)
				continue
			}
			mismatches = appendUniqueStrings(mismatches, canonicalField+":missing_action,want:"+expectedAction)
		} else {
			mismatches = appendUniqueStrings(mismatches, canonicalField+":got:"+actualAction+",want:"+expectedAction)
		}
	}
	return filtered, mismatches
}

func operationPlanAgentConfigBindingFinalStateSatisfiesAction(result map[string]interface{}, field string, expectedAction string) bool {
	field = operationPlanAgentConfigCanonicalField(field)
	expectedAction = operationPlanCanonicalAgentConfigBindingAction(expectedAction)
	if len(result) == 0 || field == "" || expectedAction == "" {
		return false
	}
	for _, state := range operationPlanAgentConfigBindingFinalStatesFromResult(result) {
		if operationPlanAgentConfigCanonicalField(firstNonEmptyString(state["field"], state["binding_kind"])) != field {
			continue
		}
		countValue, ok := state["final_resource_count"]
		if !ok || countValue == nil {
			continue
		}
		count := intValueFromAny(countValue)
		switch expectedAction {
		case "unbind":
			return count == 0
		case "bind":
			return count > 0
		}
	}
	return false
}

func operationPlanAgentConfigBindingFinalStatesFromResult(result map[string]interface{}) []map[string]interface{} {
	states := mapSliceFromAny(result["binding_final_states"])
	if len(states) > 0 {
		return states
	}
	field := operationPlanAgentConfigCanonicalField(firstNonEmptyString(result["field"], result["binding_kind"]))
	if field == "" {
		return nil
	}
	if _, ok := result["final_resource_count"]; !ok {
		return nil
	}
	return []map[string]interface{}{{
		"field":                field,
		"binding_kind":         firstNonEmptyString(result["binding_kind"], result["field"]),
		"final_resource_count": result["final_resource_count"],
		"final_resource_names": result["final_resource_names"],
	}}
}

func operationPlanAgentConfigBindingActionsFromAny(value interface{}) map[string]string {
	out := map[string]string{}
	add := func(field string, action string) {
		canonicalField := operationPlanAgentConfigCanonicalField(field)
		canonicalAction := operationPlanCanonicalAgentConfigBindingAction(action)
		if canonicalField == "" || canonicalAction == "" {
			return
		}
		out[canonicalField] = canonicalAction
	}
	switch typed := value.(type) {
	case nil:
		return nil
	case string:
		for _, item := range strings.Split(typed, ",") {
			item = strings.TrimSpace(item)
			if item == "" {
				continue
			}
			parts := strings.SplitN(item, ":", 2)
			if len(parts) != 2 {
				continue
			}
			add(parts[0], parts[1])
		}
	case map[string]interface{}:
		for field, action := range typed {
			add(field, stringFromAny(action))
		}
	case map[string]string:
		for field, action := range typed {
			add(field, action)
		}
	default:
		for _, item := range stringSliceFromAny(value) {
			parts := strings.SplitN(strings.TrimSpace(item), ":", 2)
			if len(parts) == 2 {
				add(parts[0], parts[1])
			}
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func operationPlanEncodeAgentConfigBindingActions(actions map[string]string) string {
	if len(actions) == 0 {
		return ""
	}
	fields := []string{
		"enabled_skill_ids",
		"knowledge_dataset_ids",
		"database_bindings",
		"workflow_bindings",
	}
	parts := []string{}
	for _, field := range fields {
		if action := operationPlanCanonicalAgentConfigBindingAction(actions[field]); action != "" {
			parts = append(parts, field+":"+action)
		}
	}
	return strings.Join(parts, ",")
}

func operationPlanCanonicalAgentConfigBindingAction(action string) string {
	switch strings.ToLower(strings.TrimSpace(action)) {
	case "bind", "add", "enable", "associate":
		return "bind"
	case "unbind", "remove", "delete", "disable", "detach", "clear":
		return "unbind"
	case "replace", "switch":
		return "replace"
	default:
		return ""
	}
}

func operationPlanBindingActionMatches(expected string, actual string) bool {
	expected = operationPlanCanonicalAgentConfigBindingAction(expected)
	actual = operationPlanCanonicalAgentConfigBindingAction(actual)
	return expected != "" && actual != "" && expected == actual
}

func operationPlanNormalizedAgentConfigFieldsFromAny(value interface{}) []string {
	fields := []string{}
	for _, field := range operationPlanStringListFromAny(value) {
		if canonical := operationPlanAgentConfigCanonicalField(field); canonical != "" {
			fields = appendUniqueStrings(fields, canonical)
		}
	}
	return fields
}

func operationPlanStringListFromAny(value interface{}) []string {
	switch typed := value.(type) {
	case nil:
		return nil
	case string:
		out := []string{}
		for _, item := range strings.Split(typed, ",") {
			item = strings.TrimSpace(item)
			if item != "" {
				out = appendUniqueStrings(out, item)
			}
		}
		return out
	default:
		return stringSliceFromAny(value)
	}
}

func operationPlanAgentConfigCanonicalField(field string) string {
	switch strings.TrimSpace(field) {
	case "system_prompt":
		return "system_prompt"
	case "model", "model_provider":
		return "model"
	case "model_parameters":
		return "model_parameters"
	case "enabled_skill_ids", "add_enabled_skill_ids", "remove_enabled_skill_ids", "agent_skill":
		return "enabled_skill_ids"
	case "agent_memory_enabled":
		return "agent_memory_enabled"
	case "file_upload_enabled":
		return "file_upload_enabled"
	case "home_title":
		return "home_title"
	case "input_placeholder":
		return "input_placeholder"
	case "theme_color":
		return "theme_color"
	case "suggested_questions":
		return "suggested_questions"
	case "knowledge_dataset_ids", "dataset_ids", "add_knowledge_dataset_ids", "remove_knowledge_dataset_ids", "knowledge_base":
		return "knowledge_dataset_ids"
	case "knowledge_retrieval_config":
		return "knowledge_retrieval_config"
	case "database_bindings", "add_database_bindings", "remove_database_bindings", "database_table":
		return "database_bindings"
	case "workflow_bindings", "add_workflow_bindings", "remove_workflow_bindings", "workflow":
		return "workflow_bindings"
	default:
		return ""
	}
}

func appendOperationPlanFields(current []string, additions ...string) []string {
	out := append([]string(nil), current...)
	for _, field := range additions {
		field = strings.TrimSpace(field)
		if field != "" {
			out = appendUniqueStrings(out, field)
		}
	}
	return out
}

func missingOperationPlanFields(expected []string, completed []string) []string {
	completedSet := map[string]struct{}{}
	for _, field := range completed {
		field = strings.TrimSpace(field)
		if field != "" {
			completedSet[field] = struct{}{}
		}
	}
	missing := []string{}
	for _, field := range expected {
		field = strings.TrimSpace(field)
		if field == "" {
			continue
		}
		if _, ok := completedSet[field]; !ok {
			missing = appendUniqueStrings(missing, field)
		}
	}
	return missing
}

func operationPlanStepWaitForSatisfied(step map[string]interface{}, steps []map[string]interface{}, stepStatus map[string]interface{}, invocation map[string]interface{}, appliedStepIDs map[string]bool) bool {
	waitFor := strings.TrimSpace(stringFromAny(step["wait_for"]))
	if waitFor == "" || strings.EqualFold(waitFor, "continue") {
		return true
	}
	if stepStatus == nil {
		return false
	}
	status := operationPlanNormalizeStepStatus(stringFromAny(stepStatus[waitFor]))
	if status != operationPlanStepStatusCompleted {
		return false
	}
	return !operationPlanInvocationAlreadySatisfiesSiblingStep(step, steps, stepStatus, invocation, appliedStepIDs)
}

func operationPlanInvocationAlreadySatisfiesSiblingStep(step map[string]interface{}, steps []map[string]interface{}, stepStatus map[string]interface{}, invocation map[string]interface{}, appliedStepIDs map[string]bool) bool {
	invocationID := operationPlanInvocationPlanID(invocation)
	if invocationID == "" {
		return false
	}
	stepID := strings.TrimSpace(stringFromAny(step["id"]))
	stepSkill := strings.TrimSpace(stringFromAny(step["skill_id"]))
	stepTool := strings.TrimSpace(stringFromAny(step["tool_name"]))
	if stepID == "" || stepSkill == "" || stepTool == "" {
		return false
	}
	for _, other := range steps {
		if strings.TrimSpace(stringFromAny(other["id"])) == stepID {
			continue
		}
		if !strings.EqualFold(strings.TrimSpace(stringFromAny(other["skill_id"])), stepSkill) ||
			!strings.EqualFold(strings.TrimSpace(stringFromAny(other["tool_name"])), stepTool) {
			continue
		}
		otherID := strings.TrimSpace(stringFromAny(other["id"]))
		if operationPlanNormalizeStepStatus(firstNonEmptyString(other["status"], stepStatus[otherID])) != operationPlanStepStatusCompleted {
			continue
		}
		if strings.TrimSpace(stringFromAny(other["last_invocation_id"])) == invocationID {
			if operationPlanStepIsPostUpdateAgentRead(step) && appliedStepIDs[otherID] {
				continue
			}
			return true
		}
	}
	return false
}

func operationPlanStepIsPostUpdateAgentConfigRead(step map[string]interface{}) bool {
	if len(step) == 0 {
		return false
	}
	if strings.EqualFold(strings.TrimSpace(stringFromAny(step["id"])), operationPlanPostUpdateAgentConfigReadStepID()) {
		return true
	}
	return strings.EqualFold(strings.TrimSpace(stringFromAny(step["phase"])), "post_update_verification") &&
		strings.EqualFold(strings.TrimSpace(stringFromAny(step["skill_id"])), skills.SkillAgentManagement) &&
		strings.EqualFold(strings.TrimSpace(stringFromAny(step["tool_name"])), "get_agent_config")
}

func operationPlanStepIsPostUpdateAgentIdentityRead(step map[string]interface{}) bool {
	if len(step) == 0 {
		return false
	}
	if strings.EqualFold(strings.TrimSpace(stringFromAny(step["id"])), operationPlanPostUpdateAgentIdentityReadStepID()) {
		return true
	}
	return strings.EqualFold(strings.TrimSpace(stringFromAny(step["phase"])), "post_update_verification") &&
		strings.EqualFold(strings.TrimSpace(stringFromAny(step["skill_id"])), skills.SkillAgentManagement) &&
		strings.EqualFold(strings.TrimSpace(stringFromAny(step["tool_name"])), "get_agent")
}

func operationPlanStepIsPostUpdateAgentRead(step map[string]interface{}) bool {
	return operationPlanStepIsPostUpdateAgentConfigRead(step) ||
		operationPlanStepIsPostUpdateAgentIdentityRead(step)
}

func operationPlanInvocationError(invocation map[string]interface{}) string {
	if len(invocation) == 0 {
		return ""
	}
	if errText := strings.TrimSpace(stringFromAny(invocation["error"])); errText != "" {
		return compactForPrompt(errText, 500)
	}
	result := mapFromOperationContext(invocation["result"])
	if errText := strings.TrimSpace(stringFromAny(result["error"])); errText != "" {
		return compactForPrompt(errText, 500)
	}
	return ""
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

func operationPlanCompactProgressStepRecords(value interface{}, limit int) []interface{} {
	steps := mapSliceFromAny(value)
	if len(steps) == 0 || limit <= 0 {
		return nil
	}
	out := make([]interface{}, 0, minInt(len(steps), limit))
	for _, step := range steps {
		if len(out) >= limit {
			break
		}
		compact := map[string]interface{}{}
		for _, key := range []string{
			"id",
			"status",
			"title",
			"skill_id",
			"tool_name",
			"role",
			"wait_for",
			"reason",
			"error",
			"last_invocation_id",
			"last_invocation_kind",
		} {
			if value := strings.TrimSpace(stringFromAny(step[key])); value != "" {
				compact[key] = compactForPrompt(value, 240)
			}
		}
		if target := mapFromOperationContext(step["asset_target"]); len(target) > 0 {
			compact["asset_target"] = target
		}
		if group := mapFromOperationContext(step["operation_group"]); len(group) > 0 {
			compact["operation_group"] = operationPlanCompactOperationGroup(group)
		}
		if targetSet := operationPlanCompactOperationItems(step["target_set"], 12); len(targetSet) > 0 {
			compact["target_set"] = targetSet
		}
		if itemSteps := operationPlanCompactOperationItems(step["item_steps"], 12); len(itemSteps) > 0 {
			compact["item_steps"] = itemSteps
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
	pendingOverride := ""
	if operationPlanRequiresManagedFileSave(plan, steps) && len(unsavedFiles) > 0 {
		pendingOverride = "save_remaining_generated_files_to_file_management"
	}
	applyOperationPlanProgress(plan, steps, stepStatus, pendingOverride, "")
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
	if applyOperationPlanMissingAgentTargetFromListEvidence(metadata, plan, steps, stepStatus) {
		metadata["operation_plan"] = plan
		return
	}
	if operationPlanApplyMissingAgentSkillCandidateNoop(plan, steps, stepStatus, mapSliceFromAny(metadata["skill_invocations"])) {
		metadata["operation_plan"] = plan
		return
	}

	for _, step := range steps {
		if !operationPlanStepBlocksCompletion(step) {
			continue
		}
		status := operationPlanNormalizeStepStatus(firstNonEmptyString(step["status"], stepStatus[stringFromAny(step["id"])]))
		if status == operationPlanStepStatusFailed {
			applyOperationPlanProgress(plan, steps, stepStatus, "none", operationPlanStatusFailed)
			metadata["operation_plan"] = plan
			return
		}
		if status == operationPlanStepStatusCompleted {
			continue
		}
		if operationPlanStepRequiresRuntimeAction(step) {
			applyOperationPlanProgress(plan, steps, stepStatus, "", "")
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

	applyOperationPlanProgress(plan, steps, stepStatus, "", "")
	metadata["operation_plan"] = plan
}

func applyOperationPlanCompletionVerificationResult(metadata map[string]interface{}, status string, reason string, missingSteps []string, unsupportedClaims []string, nextActionHint string) {
	if len(metadata) == 0 {
		return
	}
	plan := mapFromOperationContext(metadata["operation_plan"])
	if len(plan) == 0 {
		return
	}

	verification := map[string]interface{}{}
	status = strings.ToLower(strings.TrimSpace(status))
	if status == "" {
		status = operationPlanStatusFailed
	}
	verification["status"] = status
	if reason = strings.TrimSpace(reason); reason != "" {
		verification["reason"] = truncateRunes(reason, 500)
	}
	if missing := compactCompletionVerificationStringList(missingSteps, 12, 160); len(missing) > 0 {
		verification["missing_steps"] = missing
	}
	if claims := compactCompletionVerificationStringList(unsupportedClaims, 8, 160); len(claims) > 0 {
		verification["unsupported_claims"] = claims
	}
	if nextActionHint = strings.TrimSpace(nextActionHint); nextActionHint != "" {
		verification["next_action_hint"] = truncateRunes(nextActionHint, 240)
	}
	plan["completion_verification"] = verification

	steps := mapSliceFromAny(plan["steps"])
	if len(steps) == 0 {
		if !strings.EqualFold(strings.TrimSpace(stringFromAny(plan["status"])), operationPlanStatusCompleted) {
			plan["status"] = operationPlanStatusFailed
			plan["pending_next_action"] = "none"
		}
		metadata["operation_plan"] = plan
		return
	}
	stepStatus := mapFromOperationContext(plan["step_status"])
	if stepStatus == nil {
		stepStatus = map[string]interface{}{}
	}

	terminalFailure := operationPlanCompletionVerificationTerminalFailure(status)
	touchedPendingStep := false
	for _, step := range steps {
		if !operationPlanStepBlocksCompletion(step) {
			continue
		}
		id := strings.TrimSpace(stringFromAny(step["id"]))
		if id == "" {
			continue
		}
		current := operationPlanNormalizeStepStatus(firstNonEmptyString(step["status"], stepStatus[id]))
		if current == operationPlanStepStatusCompleted || current == operationPlanStepStatusFailed {
			continue
		}
		if !operationPlanStepRequiresRuntimeAction(step) {
			continue
		}
		touchedPendingStep = true
		if terminalFailure {
			operationPlanSetStepStatus(steps, stepStatus, id, operationPlanStepStatusFailed)
			step["error"] = completionVerificationPlanStepError(status, reason)
			continue
		}
		operationPlanSetStepStatus(steps, stepStatus, id, operationPlanStepStatusPending)
		delete(step, "error")
	}
	if terminalFailure && (touchedPendingStep || !strings.EqualFold(strings.TrimSpace(stringFromAny(plan["status"])), operationPlanStatusCompleted)) {
		applyOperationPlanProgress(plan, steps, stepStatus, "none", operationPlanStatusFailed)
	} else if touchedPendingStep {
		applyOperationPlanProgress(plan, steps, stepStatus, "", operationPlanStatusRunning)
	} else {
		applyOperationPlanProgress(plan, steps, stepStatus, "", "")
	}
	metadata["operation_plan"] = plan
}

func operationPlanCompletionVerificationTerminalFailure(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "failed", "error":
		return true
	default:
		return false
	}
}

func completionVerificationPlanStepError(status string, reason string) string {
	status = strings.TrimSpace(status)
	reason = strings.TrimSpace(reason)
	switch {
	case reason != "":
		return truncateRunes("completion verification stopped: "+reason, 500)
	case status != "":
		return "completion verification stopped with status: " + status
	default:
		return "completion verification stopped before this step had execution evidence"
	}
}

func compactCompletionVerificationStringList(values []string, limit int, runeLimit int) []string {
	if limit <= 0 || runeLimit <= 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		value = truncateRunes(value, runeLimit)
		key := strings.ToLower(value)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, value)
		if len(out) >= limit {
			break
		}
	}
	return out
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
			(operationPlanValuePresent(result,
				"updated_fields",
				"satisfied_fields",
				"config_changes",
				"binding_changes",
				"binding_final_states",
				"binding_kind",
				"resource_count",
				"resource_names",
				"final_resource_count",
				"final_resource_names",
			) ||
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
	group := mapFromOperationContext(result["operation_group"])
	items := mapSliceFromAny(result["item_results"])
	if len(items) == 0 {
		items = mapSliceFromAny(group["item_results"])
	}
	if len(items) == 0 {
		return false
	}
	targetCount := firstPositiveIntValue(result["target_count"], group["target_count"])
	counted := 0
	for _, item := range items {
		status := strings.ToLower(strings.TrimSpace(stringFromAny(item["status"])))
		switch status {
		case "succeeded", "success", "completed", "failed", "skipped", "rejected":
			counted++
		}
	}
	if targetCount > 0 {
		return counted >= targetCount
	}
	return counted == len(items)
}

func firstPositiveIntValue(values ...interface{}) int {
	for _, value := range values {
		if parsed := intValueFromAny(value); parsed > 0 {
			return parsed
		}
	}
	return 0
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
		"requested_fields",
		"satisfied_fields",
		"updated_fields",
		"model_provider",
		"model",
		"agent_memory_enabled",
		"file_upload",
		"file_upload_enabled",
		"home_title",
		"input_placeholder",
		"theme_color",
		"suggested_questions",
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
		"binding_final_states",
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
	if operationPlanStepIsSkillDeclaration(step) &&
		operationPlanNormalizeStepStatus(stringFromAny(step["status"])) != operationPlanStepStatusFailed {
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
