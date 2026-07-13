package service

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"

	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	"github.com/zgiai/zgi/api/internal/capabilities/chatruntime/skillloop"
	"github.com/zgiai/zgi/api/internal/modules/skills"
)

const (
	operationPlanVersion = "operation_plan.v2"

	operationPlanStatusRunning   = "running"
	operationPlanStatusCompleted = "completed"
	operationPlanStatusFailed    = "failed"

	operationPlanStepStatusPending   = "pending"
	operationPlanStepStatusCompleted = "completed"
	operationPlanStepStatusFailed    = "failed"

	operationPlanExpectedUpdatedFieldsKey  = "expected_updated_fields"
	operationPlanExpectedBindingActionsKey = "expected_binding_actions"
	operationPlanConfigGoalKey             = "config_goal"
	operationPlanEvidenceStateKey          = "evidence_state"
	operationPlanEvidenceLedgerKey         = "evidence_ledger"

	operationPlanCandidateSelectionPolicyKey           = "candidate_selection_policy"
	operationPlanCandidateSelectionAtMostOnePerField   = "at_most_one_per_binding_field"
	operationPlanCandidateSelectionPolicyDetailKey     = "candidate_selection_policy_detail"
	operationPlanCandidateSelectionAtMostOneDetailText = "choose at most one current-workspace candidate for each requested binding field"
)

func operationPlanFromTurnStrategy(taskID string, parts *chatRequestParts, strategy *AIChatTurnStrategy) map[string]interface{} {
	if parts == nil || strategy == nil {
		return nil
	}
	originalGoal := truncateRunes(strings.TrimSpace(parts.Query), 500)
	if partsRequestsContinuationWithFallback(parts, "") {
		if goal := recentOperationPlanOriginalGoal(parts); goal != "" {
			originalGoal = truncateRunes(goal, 500)
		}
	}
	phases := operationPlanPhasesFromTurnStrategy(strategy)
	if len(phases) == 0 && originalGoal != "" {
		phases = []map[string]interface{}{{
			"id":     "phase-1",
			"step":   originalGoal,
			"title":  originalGoal,
			"status": operationPlanStepStatusPending,
		}}
	}
	if len(phases) == 0 {
		return nil
	}
	plan := map[string]interface{}{
		"version":                          operationPlanVersion,
		"task_id":                          strings.TrimSpace(taskID),
		"original_user_goal":               originalGoal,
		"surface":                          normalizeAIChatSurface(parts.Surface),
		"intent":                           strings.TrimSpace(strategy.Intent),
		"status":                           operationPlanStatusRunning,
		"phases":                           mapsToInterfaceSlice(phases),
		"risk_level":                       operationPlanRiskLevel(strategy),
		"approval":                         operationPlanApprovalPolicy(strategy),
		"tool_choice_mode":                 aiChatTurnToolChoiceModelDecides,
		"planning_mode":                    "phase_only_model_decides",
		"success_criteria":                 operationPlanSuccessCriteriaFromTurnStrategy(strategy),
		"plan_sync_status":                 "current",
		"evidence_revision":                0,
		"evidence_revision_at_plan_update": 0,
		"evidence_sequence_at_plan_update": 0,
		"evidence_after_last_plan_update":  0,
		"derived_from":                     "turn_strategy",
	}
	if contract := operationPlanTaskContractFromTurnStrategy(strategy); len(contract) > 0 {
		plan["task_contract"] = contract
	}
	if taskType := strings.TrimSpace(strategy.TaskType); taskType != "" {
		plan["task_type"] = taskType
	}
	if len(strategy.PhaseGoals) > 0 {
		plan["phase_goals"] = compactStringSliceForPrompt(strategy.PhaseGoals, 8, 180)
	}
	if len(strategy.EvidenceRequired) > 0 {
		plan["evidence_required"] = compactStringSliceForPrompt(strategy.EvidenceRequired, 10, 180)
	}
	if len(strategy.RecommendedCapabilities) > 0 {
		plan["recommended_capabilities"] = compactStringSliceForPrompt(strategy.RecommendedCapabilities, 10, 160)
	}
	if goals := agentCapabilityGoalsToMaps(strategy.CapabilityGoals); len(goals) > 0 {
		plan["capability_goals"] = mapsToInterfaceSlice(goals)
	}
	if strategy.NeedsExactAgentRuntime {
		plan["needs_exact_agent_runtime"] = true
	}
	if strategy.CurrentContextMaySummary {
		plan["current_context_may_be_summary"] = true
	}
	if strategy.OpenCreatedAgentDetail {
		plan["open_created_agent_detail"] = true
	}
	if source := strings.TrimSpace(strategy.Source); source != "" {
		plan["strategy_source"] = source
	}
	if reason := strings.TrimSpace(strategy.SourceReason); reason != "" {
		plan["strategy_source_reason"] = reason
	}
	if strings.TrimSpace(strategy.CurrentPage) != "" {
		plan["current_page"] = strings.TrimSpace(strategy.CurrentPage)
	}
	if pageEvidence := operationPlanCompactPageEvidence(skillLoopCompletionPageContextEvidence(parts)); len(pageEvidence) > 0 {
		plan["page_evidence"] = pageEvidence
		plan["current_page_evidence"] = pageEvidence
	}
	return plan
}

func operationPlanTaskContractFromTurnStrategy(strategy *AIChatTurnStrategy) map[string]interface{} {
	if strategy == nil {
		return nil
	}
	intentLabel := strings.TrimSpace(firstNonEmptyString(strategy.CompatibilityIntent, strategy.Intent))
	contract := map[string]interface{}{
		"source":        strings.TrimSpace(strategy.Source),
		"intent_label":  intentLabel,
		"compatibility": "intent_label_is_for_routing_compatibility_only",
		"tool_choice":   "model_decides_from_enabled_tools_and_latest_evidence",
	}
	if executionIntent := strings.TrimSpace(strategy.Intent); executionIntent != "" && !strings.EqualFold(executionIntent, intentLabel) {
		contract["execution_intent"] = executionIntent
	}
	if reason := strings.TrimSpace(strategy.SourceReason); reason != "" {
		contract["source_reason"] = reason
	}
	if taskType := strings.TrimSpace(strategy.TaskType); taskType != "" {
		contract["task_type"] = taskType
	}
	if targetPage := strings.TrimSpace(strategy.TargetPage); targetPage != "" {
		contract["target_page"] = targetPage
	}
	contract["route_required"] = strategy.RouteRequired
	if strategy.LowConfidence {
		contract["low_confidence"] = true
	}
	if len(strategy.PhaseGoals) > 0 {
		contract["phases"] = compactStringSliceForPrompt(strategy.PhaseGoals, 8, 180)
	}
	if len(strategy.EvidenceRequired) > 0 {
		contract["evidence_required"] = compactStringSliceForPrompt(strategy.EvidenceRequired, 10, 180)
	}
	if len(strategy.RecommendedCapabilities) > 0 {
		contract["recommended_capabilities"] = compactStringSliceForPrompt(strategy.RecommendedCapabilities, 10, 160)
	}
	if len(strategy.SuccessCriteria) > 0 {
		contract["completion_criteria"] = compactStringSliceForPrompt(strategy.SuccessCriteria, 8, 240)
	}
	if strategy.NeedsExactAgentRuntime {
		contract["needs_exact_agent_runtime"] = true
	}
	if strategy.CurrentContextMaySummary {
		contract["current_context_may_be_summary"] = true
	}
	if strategy.OpenCreatedAgentDetail {
		contract["open_created_agent_detail"] = true
	}
	if effect := strings.TrimSpace(strategy.AssetEffect); effect != "" {
		contract["asset_effect"] = effect
	}
	if risk := strings.TrimSpace(strategy.AssetRisk); risk != "" {
		contract["asset_risk"] = risk
	}
	if approval := strings.TrimSpace(strategy.Approval); approval != "" {
		contract["approval"] = approval
	}
	if len(strategy.CapabilityGoals) > 0 {
		contract["capability_goals"] = agentCapabilityGoalsToMaps(strategy.CapabilityGoals)
	}
	return contract
}

func operationPlanSuccessCriteriaFromTurnStrategy(strategy *AIChatTurnStrategy) []string {
	if strategy == nil {
		return nil
	}
	criteria := append([]string(nil), strategy.SuccessCriteria...)
	if !aiChatTurnStrategyModelDecidesTools(strategy) {
		return criteria
	}
	out := make([]string, 0, len(criteria)+3)
	for _, value := range criteria {
		value = strings.TrimSpace(value)
		if value == "" || strings.HasPrefix(strings.ToLower(value), "complete pending plan step:") {
			continue
		}
		out = appendUniqueStrings(out, value)
	}
	if len(out) == 0 {
		out = append(out,
			"continue the user goal from current tool, page, and client-action evidence",
			"choose enabled tools from latest evidence instead of replaying a fixed tool script",
			"verify the final answer against actual tool results or refreshed page context",
		)
	}
	return out
}

func operationPlanRiskLevel(strategy *AIChatTurnStrategy) string {
	if strategy == nil {
		return ""
	}
	return strings.TrimSpace(strategy.AssetRisk)
}

func operationPlanPhasesFromTurnStrategy(strategy *AIChatTurnStrategy) []map[string]interface{} {
	if strategy == nil {
		return nil
	}
	if len(strategy.PhaseGoals) > 0 {
		phases := make([]map[string]interface{}, 0, len(strategy.PhaseGoals))
		for idx, goal := range compactStringSliceForPrompt(strategy.PhaseGoals, 8, 180) {
			phase := map[string]interface{}{
				"id":     fmt.Sprintf("phase-%d", idx+1),
				"step":   goal,
				"title":  goal,
				"status": operationPlanStepStatusPending,
			}
			phases = append(phases, phase)
		}
		return phases
	}
	return nil
}

func operationPlanApprovalPolicy(strategy *AIChatTurnStrategy) string {
	if strategy == nil {
		return ""
	}
	return strings.TrimSpace(strategy.Approval)
}

func applyRecentOperationPlansFromBranch(parts *chatRequestParts, branch []*runtimemodel.Message) {
	if parts == nil || len(parts.RecentOperationPlans) > 0 {
		return
	}
	plans := recentContinuationOperationPlans(branch, recentContinuationTurnLimit)
	parts.RecentOperationPlans = plans
}

func operationPlanAgentResourceBindingFieldForCapability(capabilityID string) string {
	descriptor, ok := agentManagementBindingCapabilityDescriptorForCapability(capabilityID)
	if !ok {
		return ""
	}
	return operationPlanAgentConfigCanonicalField(descriptor.field)
}

func operationPlanIsTerminalFailure(plan map[string]interface{}) bool {
	switch strings.ToLower(strings.TrimSpace(stringFromAny(plan["status"]))) {
	case operationPlanStatusFailed, "error", "rejected", "blocked":
		return true
	default:
		return false
	}
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
	if parts == nil || strategy == nil || !partsRequestsContinuationWithFallback(parts, "") {
		return strategy
	}
	plan := firstIncompleteRecentOperationPlan(parts)
	if len(plan) == 0 {
		return strategy
	}
	strategy.PhaseGoals = appendUniqueStrings(strategy.PhaseGoals, operationPlanAdvisoryContinuationPhases(plan, 8)...)
	strategy.RecommendedCapabilities = appendUniqueStrings(strategy.RecommendedCapabilities, stringSliceFromAny(plan["recommended_capabilities"])...)
	if goals := agentCapabilityGoalsFromOperationPlan(plan); len(goals) > 0 {
		strategy.CapabilityGoals = appendAgentCapabilityGoals(strategy.CapabilityGoals, goals...)
	}
	return strategy
}

func operationPlanAdvisoryContinuationPhases(plan map[string]interface{}, limit int) []string {
	if len(plan) == 0 || limit <= 0 {
		return nil
	}
	collect := func(records []map[string]interface{}) []string {
		out := []string{}
		for _, record := range records {
			status := strings.ToLower(strings.TrimSpace(stringFromAny(record["status"])))
			if status == operationPlanStepStatusCompleted || status == "skipped" {
				continue
			}
			text := strings.TrimSpace(firstNonEmptyString(record["step"], record["title"], record["goal"], record["description"]))
			if text == "" {
				continue
			}
			out = appendUniqueStrings(out, compactForPrompt(text, 240))
			if len(out) >= limit {
				break
			}
		}
		return out
	}
	if phases := collect(mapSliceFromAny(plan["phases"])); len(phases) > 0 {
		return phases
	}
	if structured := mapFromOperationContext(plan["structured_plan"]); len(structured) > 0 {
		if phases := collect(mapSliceFromAny(structured["operations"])); len(phases) > 0 {
			return phases
		}
	}
	return collect(mapSliceFromAny(plan["steps"]))
}

func operationPlanCompletionVerificationPassStatus(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "pass", "passed", "completed", "complete", "success", "succeeded", "ok":
		return true
	default:
		return false
	}
}

func operationPlanCompactEvidenceLedger(value interface{}, limit int) []map[string]interface{} {
	if limit <= 0 {
		return nil
	}
	ledger := mapSliceFromAny(value)
	if len(ledger) == 0 {
		return nil
	}
	if len(ledger) > limit {
		ledger = ledger[len(ledger)-limit:]
	}
	out := make([]map[string]interface{}, 0, len(ledger))
	for _, entry := range ledger {
		compact := map[string]interface{}{}
		if keys := stringSliceFromAny(entry["keys"]); len(keys) > 0 {
			compact["keys"] = keys
		}
		for _, key := range []string{"skill_id", "tool_name", "kind", "status", "invocation_id"} {
			if value := strings.TrimSpace(stringFromAny(entry[key])); value != "" {
				compact[key] = compactForPrompt(value, 300)
			}
		}
		if sequence := intValueFromAny(entry["sequence"]); sequence > 0 {
			compact["sequence"] = sequence
		}
		if revision := intValueFromAny(entry["ledger_revision"]); revision > 0 {
			compact["ledger_revision"] = revision
		}
		if target := operationPlanCompactEvidenceTarget(mapFromOperationContext(entry["target"])); len(target) > 0 {
			compact["target"] = target
		}
		if summary := operationPlanCompactEvidenceResultSummary(mapFromOperationContext(entry["result_summary"])); len(summary) > 0 {
			compact["result_summary"] = summary
		}
		if facts := mapFromOperationContext(entry["result_facts"]); len(facts) > 0 {
			if compactFacts := operationPlanCompactEvidenceResultFacts(facts); len(compactFacts) > 0 {
				compact["result_facts"] = compactFacts
			}
		}
		if len(compact) > 0 {
			out = append(out, compact)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
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
	operationPlanStrategyStateSetString(state, "task_type", stringFromAny(plan["task_type"]))
	operationPlanStrategyStateSetString(state, "current_page", stringFromAny(plan["current_page"]))
	operationPlanStrategyStateSetString(state, "risk_level", stringFromAny(plan["risk_level"]))
	operationPlanStrategyStateSetString(state, "approval", stringFromAny(plan["approval"]))
	if value, ok := plan["approval_required"].(bool); ok {
		state["approval_required"] = value
	} else {
		delete(state, "approval_required")
	}
	operationPlanStrategyStateSetStringSlice(state, "success_criteria", stringSliceFromAny(plan["success_criteria"]))
	operationPlanStrategyStateSetStringSlice(state, "phase_goals", stringSliceFromAny(plan["phase_goals"]))
	operationPlanStrategyStateSetStringSlice(state, "evidence_required", stringSliceFromAny(plan["evidence_required"]))
	operationPlanStrategyStateSetStringSlice(state, "recommended_capabilities", stringSliceFromAny(plan["recommended_capabilities"]))
	if value, ok := plan["needs_exact_agent_runtime"].(bool); ok {
		state["needs_exact_agent_runtime"] = value
	} else {
		delete(state, "needs_exact_agent_runtime")
	}
	if value, ok := plan["current_context_may_be_summary"].(bool); ok {
		state["current_context_may_be_summary"] = value
	} else {
		delete(state, "current_context_may_be_summary")
	}
	delete(state, "target_resource")
	if pageEvidence := operationPlanCompactPageEvidence(mapFromOperationContext(firstNonNil(plan["current_page_evidence"], plan["page_evidence"]))); len(pageEvidence) > 0 {
		state["current_page_evidence"] = pageEvidence
	} else if currentPage := strings.TrimSpace(stringFromAny(plan["current_page"])); currentPage != "" {
		state["current_page_evidence"] = map[string]interface{}{"current_page": compactForPrompt(currentPage, 300)}
	} else {
		delete(state, "current_page_evidence")
	}
	delete(state, "plan_steps")
	delete(state, "structured_plan")
	operationPlanStrategyStateSetInterfaceSlice(state, "phases", operationPlanCompactPhasesForPrompt(plan["phases"], 8))
	operationPlanStrategyStateSetInterfaceSlice(state, "capability_goals", operationPlanCompactCapabilityGoals(plan["capability_goals"], 8))
	operationPlanStrategyStateSetInterfaceSlice(state, "evidence_ledger", mapsToInterfaceSlice(operationPlanCompactEvidenceLedger(plan[operationPlanEvidenceLedgerKey], 12)))
	for _, key := range []string{
		"pending_next_action", "approval_actions", "completion_criteria", "completed_steps", "failed_steps",
		"plan_deviations", "blocked_deviations", "last_plan_deviation", "last_blocked_deviation",
		"completed_step_count", "failed_step_count", "plan_deviation_count", "blocked_deviation_count",
	} {
		delete(state, key)
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
	ok, source := operationPlanExplicitReadOnlyAgentCandidateLookup(plan)
	if !ok {
		return
	}
	if source != "" {
		plan["read_only_candidate_lookup_source"] = source
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

func operationPlanExplicitReadOnlyAgentCandidateLookup(plan map[string]interface{}) (bool, string) {
	if len(plan) == 0 {
		return false, ""
	}
	if goals := agentCapabilityGoalsFromOperationPlan(plan); len(goals) > 0 {
		return agentCapabilityGoalsAreExplicitReadOnly(goals), "capability_goals"
	}
	return false, ""
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
		if !operationPlanStepBlocksCompletion(step) || !operationPlanStepRequiresStrictRuntimeStateSnapshot(step) {
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
		skillLoopToolLooksReadOnly(skills.SkillAgentManagement, toolName)
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
	if agentID != "" && consoleNavigationLoadedHrefMatchesTarget(consoleAgentDetailHref(agentID), target) {
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
		if rawStatus, ok := stepStatus[id]; ok {
			status := operationPlanNormalizeStepStatus(stringFromAny(rawStatus))
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

func operationPlanRecordInvocationEvidence(plan map[string]interface{}, invocation map[string]interface{}, status string) bool {
	if len(plan) == 0 || status != operationPlanStepStatusCompleted {
		return false
	}
	keys := operationPlanInvocationEvidenceKeys(invocation, status)
	if len(keys) == 0 {
		return false
	}
	operationPlanAppendEvidenceLedgerEntry(plan, invocation, keys)
	evidenceState := mapFromOperationContext(plan[operationPlanEvidenceStateKey])
	if evidenceState == nil {
		evidenceState = map[string]interface{}{}
	}
	changed := false
	for _, key := range keys {
		if strings.TrimSpace(stringFromAny(evidenceState[key])) == operationPlanStepStatusCompleted {
			continue
		}
		evidenceState[key] = operationPlanStepStatusCompleted
		changed = true
	}
	if changed {
		plan[operationPlanEvidenceStateKey] = evidenceState
	}
	return changed
}

func operationPlanAppendEvidenceLedgerEntry(plan map[string]interface{}, invocation map[string]interface{}, keys []string) {
	if len(plan) == 0 || len(keys) == 0 {
		return
	}
	entry := map[string]interface{}{
		"keys":      append([]string(nil), keys...),
		"skill_id":  strings.TrimSpace(stringFromAny(invocation["skill_id"])),
		"tool_name": strings.TrimSpace(stringFromAny(invocation["tool_name"])),
		"kind":      strings.TrimSpace(stringFromAny(invocation["kind"])),
		"status":    operationPlanStepStatusCompleted,
	}
	if invocationID := operationPlanInvocationPlanID(invocation); invocationID != "" {
		entry["invocation_id"] = invocationID
	}
	if sequence := operationPlanInvocationSequence(invocation); sequence > 0 {
		entry["sequence"] = sequence
	}
	if target := operationPlanEvidenceLedgerTarget(invocation); len(target) > 0 {
		entry["target"] = target
	}
	if summary := operationPlanEvidenceLedgerResultSummary(invocation); len(summary) > 0 {
		entry["result_summary"] = summary
	}
	if facts := operationPlanEvidenceLedgerResultFacts(invocation); len(facts) > 0 {
		entry["result_facts"] = facts
	}
	ledger := mapSliceFromAny(plan[operationPlanEvidenceLedgerKey])
	if operationPlanEvidenceLedgerHasEntry(ledger, entry) {
		return
	}
	revision := operationPlanCurrentEvidenceRevision(plan) + 1
	entry["ledger_revision"] = revision
	plan["evidence_revision"] = revision
	ledger = append(ledger, entry)
	if len(ledger) > 50 {
		ledger = ledger[len(ledger)-50:]
	}
	ledger = operationPlanAnnotateEvidenceLedger(ledger)
	plan[operationPlanEvidenceLedgerKey] = mapsToInterfaceSlice(ledger)
}

func operationPlanEvidenceLedgerHasEntry(ledger []map[string]interface{}, entry map[string]interface{}) bool {
	invocationID := strings.TrimSpace(stringFromAny(entry["invocation_id"]))
	for _, existing := range ledger {
		if invocationID != "" &&
			strings.TrimSpace(stringFromAny(existing["invocation_id"])) == invocationID &&
			strings.TrimSpace(stringFromAny(existing["skill_id"])) == strings.TrimSpace(stringFromAny(entry["skill_id"])) &&
			strings.TrimSpace(stringFromAny(existing["tool_name"])) == strings.TrimSpace(stringFromAny(entry["tool_name"])) &&
			strings.TrimSpace(stringFromAny(existing["kind"])) == strings.TrimSpace(stringFromAny(entry["kind"])) &&
			sameStringSet(stringSliceFromAny(existing["keys"]), stringSliceFromAny(entry["keys"])) {
			return true
		}
		if !strings.EqualFold(strings.TrimSpace(stringFromAny(existing["skill_id"])), strings.TrimSpace(stringFromAny(entry["skill_id"]))) ||
			!strings.EqualFold(strings.TrimSpace(stringFromAny(existing["tool_name"])), strings.TrimSpace(stringFromAny(entry["tool_name"]))) ||
			!strings.EqualFold(strings.TrimSpace(stringFromAny(existing["kind"])), strings.TrimSpace(stringFromAny(entry["kind"]))) {
			continue
		}
		if !reflect.DeepEqual(mapFromOperationContext(existing["target"]), mapFromOperationContext(entry["target"])) {
			continue
		}
		if !reflect.DeepEqual(mapFromOperationContext(existing["result_facts"]), mapFromOperationContext(entry["result_facts"])) {
			continue
		}
		if !reflect.DeepEqual(mapFromOperationContext(existing["result_summary"]), mapFromOperationContext(entry["result_summary"])) {
			continue
		}
		return true
	}
	return false
}

func sameStringSet(left []string, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	counts := make(map[string]int, len(left))
	for _, value := range left {
		normalized := strings.TrimSpace(value)
		counts[normalized]++
	}
	for _, value := range right {
		normalized := strings.TrimSpace(value)
		if counts[normalized] == 0 {
			return false
		}
		counts[normalized]--
		if counts[normalized] == 0 {
			delete(counts, normalized)
		}
	}
	return len(counts) == 0
}

func operationPlanInvocationEvidenceKeys(invocation map[string]interface{}, status string) []string {
	if len(invocation) == 0 || status != operationPlanStepStatusCompleted {
		return nil
	}
	skillID := strings.TrimSpace(stringFromAny(invocation["skill_id"]))
	toolName := strings.TrimSpace(stringFromAny(invocation["tool_name"]))
	keys := []string{}
	if operationPlanInvocationIsConsoleRouteNavigation(invocation) &&
		strings.EqualFold(strings.TrimSpace(stringFromAny(invocation["kind"])), "client_action") {
		if key := operationPlanPageEvidenceKey(operationPlanInvocationHref(invocation)); key != "" {
			keys = appendUniqueStrings(keys, key)
		}
	}
	if exact := operationPlanToolEvidenceKey(skillID, toolName); exact != "" {
		keys = appendUniqueStrings(keys, exact)
	}
	keys = appendUniqueStrings(keys, operationPlanSemanticEvidenceKeys(skillID, toolName)...)
	if strings.EqualFold(skillID, skills.SkillFileReader) && strings.EqualFold(toolName, "read_file") {
		keys = appendUniqueStrings(keys, "file:list")
	}
	return keys
}

func operationPlanToolEvidenceKey(skillID, toolName string) string {
	skillID = strings.ToLower(strings.TrimSpace(skillID))
	toolName = strings.ToLower(strings.TrimSpace(toolName))
	if skillID == "" || toolName == "" {
		return ""
	}
	return "tool:" + skillID + "/" + toolName
}

func operationPlanPageEvidenceKey(href string) string {
	href = normalizeConsoleNavigationGuardHref(href)
	if href == "" {
		return ""
	}
	return "page:" + href
}

func operationPlanSemanticEvidenceKeys(skillID, toolName string) []string {
	skillID = strings.TrimSpace(skillID)
	toolName = strings.TrimSpace(toolName)
	switch {
	case strings.EqualFold(skillID, skills.SkillFileReader) && strings.EqualFold(toolName, "list_visible_files"):
		return []string{"file:list"}
	case strings.EqualFold(skillID, skills.SkillFileReader) && strings.EqualFold(toolName, "read_file"):
		return []string{"file:read"}
	case strings.EqualFold(skillID, skills.SkillAgentManagement):
		return operationPlanAgentManagementEvidenceKeys(toolName)
	default:
		return nil
	}
}

func operationPlanAgentManagementEvidenceKeys(toolName string) []string {
	switch strings.TrimSpace(toolName) {
	case "list_agents":
		return []string{"agent:list"}
	case "get_agent":
		return []string{"agent:identity"}
	case "get_agent_config":
		return []string{"agent:config"}
	case "list_available_models":
		return []string{"agent:model_candidates"}
	case "list_agent_skill_candidates":
		return []string{"agent:skill_candidates"}
	case "list_agent_knowledge_candidates":
		return []string{"agent:knowledge_candidates"}
	case "list_agent_database_candidates":
		return []string{"agent:database_candidates"}
	case "list_agent_database_tables":
		return []string{"agent:database_table_candidates"}
	case "list_agent_workflow_binding_candidates":
		return []string{"agent:workflow_candidates"}
	default:
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
		if !operationPlanStepRequiresStrictRuntimeStateSnapshot(step) {
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

func operationPlanStepRequiresStrictRuntimeStateSnapshot(step map[string]interface{}) bool {
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
	return skillLoopToolLooksAssetMutation(stringFromAny(step["skill_id"]), toolName)
}

func operationPlanToolStepAssetTarget(skillID, toolName string) map[string]interface{} {
	switch {
	case isKnownArtifactGeneratorToolCall(skillID, toolName):
		return map[string]interface{}{"effect": "create_temporary_artifact"}
	}
	if target := operationPlanToolStepAssetTargetFromGovernance(skillID, toolName); len(target) > 0 {
		operationPlanEnrichToolStepAssetTarget(target, skillID, toolName)
		return target
	}
	return nil
}

func operationPlanToolStepAssetTargetFromGovernance(skillID, toolName string) map[string]interface{} {
	manifest, ok := skills.SystemSkillToolGovernanceManifest(skillID, toolName)
	if !ok {
		return nil
	}
	target := map[string]interface{}{}
	if strings.TrimSpace(string(manifest.Effect)) != "" {
		target["effect"] = strings.TrimSpace(string(manifest.Effect))
	}
	if strings.TrimSpace(manifest.AssetType) != "" {
		target["asset_type"] = strings.TrimSpace(manifest.AssetType)
	}
	if len(target) == 0 {
		return nil
	}
	return target
}

func operationPlanEnrichToolStepAssetTarget(target map[string]interface{}, skillID, toolName string) {
	if target == nil {
		return
	}
	switch {
	case isFileManagerSaveToolCall(skillID, toolName):
		target["target"] = "file_management"
	case strings.EqualFold(strings.TrimSpace(skillID), skills.SkillAgentManagement) &&
		strings.EqualFold(strings.TrimSpace(toolName), "delete_agents"):
		target["operation_mode"] = "batch"
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
	var lastActionable map[string]interface{}
	for _, invocation := range invocations {
		if !operationPlanInvocationIsActionable(invocation) {
			continue
		}
		status := operationPlanStatusFromInvocation(invocation)
		lastActionable = invocation
		if operationPlanInvocationShouldUpdateCurrentPage(invocation, status) {
			if href := operationPlanInvocationHref(invocation); href != "" {
				plan["current_page"] = href
			}
		}
		operationPlanRecordInvocationEvidence(plan, invocation, status)
	}

	if lastActionable != nil {
		plan["tool_result"] = operationPlanToolResult(lastActionable)
	}
	operationPlanRefreshSyncStatus(plan)
	operationPlanSyncStrategyState(plan)
	metadata["operation_plan"] = plan
}

func operationPlanCurrentEvidenceRevision(plan map[string]interface{}) int {
	if revision := intValueFromAny(plan["evidence_revision"]); revision > 0 {
		return revision
	}
	ledger := mapSliceFromAny(plan[operationPlanEvidenceLedgerKey])
	latest := 0
	for index, entry := range ledger {
		revision := intValueFromAny(entry["ledger_revision"])
		if revision <= 0 {
			revision = index + 1
			if legacySequence := intValueFromAny(entry["sequence"]); legacySequence > revision {
				revision = legacySequence
			}
		}
		if revision > latest {
			latest = revision
		}
	}
	return latest
}

func operationPlanRefreshSyncStatus(plan map[string]interface{}) {
	if len(plan) == 0 {
		return
	}
	current := operationPlanCurrentEvidenceRevision(plan)
	plan["evidence_revision"] = current
	baseline := intValueFromAny(plan["evidence_revision_at_plan_update"])
	if _, ok := plan["evidence_revision_at_plan_update"]; !ok {
		baseline = intValueFromAny(plan["evidence_sequence_at_plan_update"])
	}
	if baseline < 0 {
		baseline = 0
	}
	after := current - baseline
	if after < 0 {
		after = 0
	}
	plan["evidence_after_last_plan_update"] = after
	if after > 0 {
		plan["plan_sync_status"] = "stale"
	}
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

func operationPlanEncodeAgentConfigBindingActions(actions map[string]string) string {
	if len(actions) == 0 {
		return ""
	}
	parts := []string{}
	for _, field := range []string{"enabled_skill_ids", "knowledge_dataset_ids", "database_bindings", "workflow_bindings"} {
		if action := operationPlanCanonicalAgentConfigBindingAction(actions[field]); action != "" {
			parts = append(parts, field+":"+action)
		}
	}
	return strings.Join(parts, ",")
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
	if text, ok := value.(string); ok {
		return compactStringSliceForPrompt(strings.Split(text, ","), 32, 120)
	}
	return stringSliceFromAny(value)
}

func operationPlanAgentConfigCanonicalField(field string) string {
	descriptor, ok := agentManagementConfigFieldDescriptorForAlias(field)
	if !ok {
		return ""
	}
	return descriptor.field
}

func operationPlanInvocationIsAssetMutation(invocation map[string]interface{}) bool {
	if len(invocation) == 0 {
		return false
	}
	skillID := strings.TrimSpace(stringFromAny(invocation["skill_id"]))
	toolName := strings.TrimSpace(stringFromAny(invocation["tool_name"]))
	target := operationPlanToolStepAssetTarget(skillID, toolName)
	effect := strings.ToLower(strings.TrimSpace(stringFromAny(target["effect"])))
	if effect != "" {
		return effect != "read"
	}
	return skillLoopToolLooksAssetMutation(skillID, toolName)
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
	assetFiles := make([]interface{}, 0, minInt(len(files), 8))
	logicalAssets := map[string]struct{}{}
	temporaryCount := 0
	successfulSaveCalls := successfulMetadataToolCalls(metadata, skills.SkillFileManager, "save_file_to_management")
	managedCount := len(successfulSaveCalls)
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
	}
	if len(assetFiles) == 0 {
		return
	}
	logicalAssetCount := len(logicalAssets)
	if logicalAssetCount == 0 {
		logicalAssetCount = len(assetFiles)
	}
	unsavedFiles := compactUnsavedOperationPlanGeneratedFiles(files, successfulSaveCalls)

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

func finalizeOperationPlanForCompletedResult(metadata map[string]interface{}) {
	plan := mapFromOperationContext(metadata["operation_plan"])
	if len(plan) == 0 {
		return
	}
	if verification := mapFromOperationContext(plan["completion_verification"]); len(verification) == 0 {
		plan["completion_verification"] = map[string]interface{}{
			"status": "pass",
			"source": "main_model_final",
			"reason": "main model submitted the final answer with no protocol blockers",
		}
	}
	plan["status"] = operationPlanStatusCompleted
	plan["pending_next_action"] = "none"
	operationPlanSyncStrategyState(plan)
	metadata["operation_plan"] = plan
	syncOperationPlanCompletionMetadata(metadata)
}

func applyOperationPlanTerminalCompletionResult(metadata map[string]interface{}, status string, reason string, missingSteps []string, unsupportedClaims []string, nextActionHint string) {
	applyOperationPlanTerminalCompletionResultWithSource(metadata, status, "", reason, missingSteps, unsupportedClaims, nextActionHint)
}

func applyOperationPlanTerminalCompletionResultWithSource(metadata map[string]interface{}, status string, source string, reason string, missingSteps []string, unsupportedClaims []string, nextActionHint string) {
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
	if source = strings.TrimSpace(source); source != "" {
		verification["source"] = truncateRunes(source, 120)
	}
	if operationPlanCompletionVerificationPassStatus(status) && operationPlanCompletionVerificationReasonLooksStaleFailure(reason) {
		reason = "latest evidence satisfies requested operation"
	}
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
	switch {
	case operationPlanCompletionVerificationPassStatus(status):
		plan["status"] = operationPlanStatusCompleted
		plan["pending_next_action"] = "none"
	case operationPlanCompletionVerificationTerminalFailure(status):
		plan["status"] = operationPlanStatusFailed
		plan["pending_next_action"] = "none"
	default:
		plan["status"] = operationPlanStatusRunning
		if nextActionHint != "" {
			plan["pending_next_action"] = truncateRunes(nextActionHint, 240)
		}
	}
	operationPlanSyncStrategyState(plan)
	metadata["operation_plan"] = plan
	syncOperationPlanCompletionMetadata(metadata)
}

func syncOperationPlanCompletionMetadata(metadata map[string]interface{}) {
	if len(metadata) == 0 {
		return
	}
	plan := mapFromOperationContext(metadata["operation_plan"])
	if len(plan) == 0 {
		return
	}
	if ledger := operationPlanCompactEvidenceLedger(plan[operationPlanEvidenceLedgerKey], 50); len(ledger) > 0 {
		metadata["evidence_ledger"] = mapsToInterfaceSlice(ledger)
	}
	verification := mapFromOperationContext(plan["completion_verification"])
	if len(verification) > 0 {
		metadata["completion_verification"] = verification
	}
	if !strings.EqualFold(strings.TrimSpace(stringFromAny(plan["status"])), operationPlanStatusCompleted) {
		return
	}
	if len(verification) > 0 && !operationPlanCompletionVerificationPassStatus(stringFromAny(verification["status"])) {
		return
	}
	summary := mapFromOperationContext(metadata["operation_result_summary"])
	if len(summary) == 0 {
		summary = map[string]interface{}{}
	}
	summary["status"] = operationPlanStatusCompleted
	summary["plan_status"] = operationPlanStatusCompleted
	summary["pending_next_action"] = "none"
	metadata["operation_result_summary"] = summary
}

func operationPlanCompletionVerificationTerminalFailure(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "failed", "error":
		return true
	default:
		return false
	}
}

func operationPlanCompletionVerificationReasonLooksStaleFailure(reason string) bool {
	reason = strings.ToLower(strings.TrimSpace(reason))
	if reason == "" {
		return false
	}
	for _, marker := range []string{
		"缺少工具结果支持",
		"不能可靠确认",
		"不能确认",
		"后校验发现",
		"failed",
		"failure",
		"unsupported",
		"missing tool",
		"missing evidence",
		"not verified",
	} {
		if strings.Contains(reason, marker) {
			return true
		}
	}
	return false
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

func compactUnsavedOperationPlanGeneratedFiles(files []map[string]interface{}, successfulSaveCalls []skillloop.SkillToolCallRef) []map[string]interface{} {
	if len(files) == 0 {
		return nil
	}
	savedSourceIDs := map[string]struct{}{}
	for _, call := range successfulSaveCalls {
		if sourceID := fileManagerSaveToolFileID(call); sourceID != "" {
			savedSourceIDs[sourceID] = struct{}{}
		}
	}
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
	kind := strings.TrimSpace(stringFromAny(invocation["kind"]))
	status := operationPlanNormalizeStepStatus(stringFromAny(invocation["status"]))
	if kind == "tool_governance" && status == operationPlanStepStatusCompleted {
		return operationPlanStepStatusPending
	}
	if status != operationPlanStepStatusCompleted {
		return status
	}
	if operationPlanInvocationResultSignalsFailure(invocation) {
		return operationPlanStepStatusFailed
	}
	if !operationPlanInvocationHasRuntimeStateSnapshot(invocation) {
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

func operationPlanInvocationHasRuntimeStateSnapshot(invocation map[string]interface{}) bool {
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

func operationPlanInvocationShouldUpdateCurrentPage(invocation map[string]interface{}, status string) bool {
	if status != operationPlanStepStatusCompleted {
		return false
	}
	if strings.TrimSpace(stringFromAny(invocation["kind"])) != "client_action" {
		return false
	}
	return operationPlanInvocationIsConsoleRouteNavigation(invocation)
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
		"opening_statement",
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
	operationPlanAddAgentConfigReferenceSamples(result, payload)
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
		"content_returned_chars",
		"content_truncated",
		"content_value_preview",
		"content_value_source",
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

func operationPlanAddAgentConfigReferenceSamples(result map[string]interface{}, payload map[string]interface{}) {
	if len(result) == 0 || len(payload) == 0 {
		return
	}
	config := mapFromOperationContext(payload["config"])
	operationPlanAddCollectionRefs(result, "enabled_skill_refs", payload["enabled_skill_ids"], config["enabled_skill_ids"])
	operationPlanAddCollectionRefs(result, "knowledge_dataset_refs", payload["knowledge_dataset_ids"], config["knowledge_dataset_ids"])
	operationPlanAddCollectionRefs(result, "database_binding_refs", payload["database_bindings"], config["database_bindings"])
	operationPlanAddCollectionRefs(result, "workflow_binding_refs", payload["workflow_bindings"], config["workflow_bindings"])
}

func operationPlanAddCollectionRefs(result map[string]interface{}, outKey string, values ...interface{}) {
	if len(result) == 0 || strings.TrimSpace(outKey) == "" {
		return
	}
	if _, exists := result[outKey]; exists {
		return
	}
	refs := operationPlanCollectionReferenceSamples(values...)
	if len(refs) == 0 {
		return
	}
	result[outKey] = refs
}

func operationPlanCollectionReferenceSamples(values ...interface{}) []string {
	const maxRefs = 8
	refs := make([]string, 0, maxRefs)
	seen := map[string]struct{}{}
	add := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		value = truncateRunes(value, 96)
		key := strings.ToLower(value)
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		refs = append(refs, value)
	}
	for _, value := range values {
		for _, text := range sanitizedStringListArgumentValue(value) {
			if len(refs) >= maxRefs {
				return refs
			}
			add(text)
		}
		for _, item := range mapSliceFromAny(value) {
			if len(refs) >= maxRefs {
				return refs
			}
			add(firstNonEmptyString(
				item["id"],
				item["skill_id"],
				item["dataset_id"],
				item["database_table_id"],
				item["table_id"],
				item["workflow_id"],
				item["name"],
				item["label"],
				item["title"],
				item["display_name"],
			))
		}
	}
	return refs
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
