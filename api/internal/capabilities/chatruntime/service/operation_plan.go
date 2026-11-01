package service

import (
	"reflect"
	"strconv"
	"strings"

	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	"github.com/zgiai/zgi/api/internal/capabilities/chatruntime/skillloop"
	"github.com/zgiai/zgi/api/internal/modules/skills"
)

const (
	operationPlanVersion = "operation_plan.v2"
	// operationPlanChecklistVersion marks a plan whose steps are the model's own
	// checklist (ADR 0049); older plans carry runtime phases.
	operationPlanChecklistVersion = "operation_plan.v3"

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
	operationPlanOutcomesKey               = "outcomes"
	operationPlanActionAttemptsKey         = "action_attempts"
	operationPlanEffectLedgerKey           = "effect_ledger"
	operationPlanPhaseSourceKey            = "phase_source"
	operationPlanPhaseSourceServerOutcome  = "server_outcome"
	operationPlanPhaseSourceModelAdvisory  = "model_advisory"
)

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
	if len(strategy.Outcomes) > 0 {
		contract["outcomes"] = mapsToInterfaceSlice(operationPlanTurnOutcomesToMaps(strategy.Outcomes))
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

func operationPlanTurnOutcomesToMaps(outcomes []AIChatTurnOutcome) []map[string]interface{} {
	if len(outcomes) == 0 {
		return nil
	}
	out := make([]map[string]interface{}, 0, len(outcomes))
	for _, outcome := range outcomes {
		item := map[string]interface{}{
			"id":   strings.TrimSpace(outcome.ID),
			"goal": strings.TrimSpace(outcome.Goal),
		}
		if value := strings.TrimSpace(outcome.TargetResourceType); value != "" {
			item["target_resource_type"] = value
		}
		if value := strings.TrimSpace(outcome.TargetResourceID); value != "" {
			item["target_resource_id"] = value
		}
		if len(outcome.DependsOn) > 0 {
			item["depends_on"] = append([]string(nil), outcome.DependsOn...)
		}
		if len(outcome.Capabilities) > 0 {
			item["capabilities"] = append([]string(nil), outcome.Capabilities...)
		}
		if len(outcome.Constraints) > 0 {
			item["constraints"] = append([]string(nil), outcome.Constraints...)
		}
		if outcome.Required != nil {
			item["required"] = *outcome.Required
		}
		out = append(out, item)
	}
	return out
}

func applyRecentOperationPlansFromBranch(parts *chatRequestParts, branch []*runtimemodel.Message) {
	if parts == nil || len(parts.RecentOperationPlans) > 0 {
		return
	}
	plans := recentContinuationOperationPlans(branch, recentContinuationTurnLimit)
	parts.RecentOperationPlans = plans
}

func operationPlanIsTerminalFailure(plan map[string]interface{}) bool {
	switch strings.ToLower(strings.TrimSpace(stringFromAny(plan["status"]))) {
	case operationPlanStatusFailed, "error", "rejected", "blocked", "canceled", "cancelled", "stopped":
		return true
	default:
		return false
	}
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
	operationPlanStrategyStateSetInterfaceSlice(state, "outcomes", operationPlanCompactOutcomesForPrompt(plan[operationPlanOutcomesKey], 8))
	operationPlanStrategyStateSetInterfaceSlice(state, "capability_goals", operationPlanCompactCapabilityGoals(plan["capability_goals"], 8))
	delete(state, "evidence_ledger")
	if revision := operationPlanCurrentEvidenceRevision(plan); revision > 0 {
		state["evidence_revision"] = revision
	} else {
		delete(state, "evidence_revision")
	}
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
		for _, existing := range ledger {
			if operationPlanEvidenceLedgerHasEntry([]map[string]interface{}{existing}, entry) {
				if id := stringFromAny(entry["invocation_id"]); id != "" && id != stringFromAny(existing["invocation_id"]) {
					existing["invocation_aliases"] = appendUniqueStrings(stringSliceFromAny(existing["invocation_aliases"]), id)
				}
				break
			}
		}
		plan[operationPlanEvidenceLedgerKey] = mapsToInterfaceSlice(ledger)
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

// applyOperationPlanInvocationState records a run's tool calls in the plan's
// ledgers, the authoritative facts later continuations reuse. The calls do not
// advance plan steps: the model owns its checklist (ADR 0049).
func applyOperationPlanInvocationState(metadata map[string]interface{}, invocations []map[string]interface{}) {
	plan := mapFromOperationContext(metadata["operation_plan"])
	var lastActionable map[string]interface{}
	for _, invocation := range invocations {
		if !operationPlanInvocationIsActionable(invocation) {
			continue
		}
		if plan == nil {
			plan = map[string]interface{}{"version": operationPlanVersion}
		}
		invocation = operationPlanInvocationExecutionEvidenceView(invocation)
		status := operationPlanStatusFromInvocation(invocation)
		lastActionable = invocation
		if operationPlanInvocationShouldUpdateCurrentPage(invocation, status) {
			if href := operationPlanInvocationHref(invocation); href != "" {
				plan["current_page"] = href
			}
		}
		operationPlanRecordInvocationEvidence(plan, invocation, status)
		operationPlanRecordActionAttempt(plan, invocation, status)
		operationPlanRecordInvocationEffects(plan, invocation, status)
	}
	if len(plan) == 0 {
		return
	}
	if lastActionable != nil {
		plan["tool_result"] = operationPlanToolResult(lastActionable)
		operationPlanMarkEvidenceCurrent(plan)
	}
	operationPlanSyncStrategyState(plan)
	metadata["operation_plan"] = plan
	applySkillLoopExecutionDiagnostics(metadata, invocations)
}

func operationPlanPhaseOpen(phase map[string]interface{}) bool {
	switch strings.ToLower(strings.TrimSpace(stringFromAny(phase["status"]))) {
	case operationPlanStepStatusPending, "in_progress", "running":
		return true
	default:
		return false
	}
}

// operationPlanInvocationIsExecutionEvidence keeps policy and audit records
// out of execution-state reconciliation. Invocation records persisted before
// the kind discriminator was introduced remain valid compatibility evidence.
func operationPlanInvocationIsExecutionEvidence(invocation map[string]interface{}) bool {
	kind := strings.TrimSpace(stringFromAny(invocation["kind"]))
	return kind == "" || strings.EqualFold(kind, "tool_call")
}

func operationPlanInvocationExecutionEvidenceView(invocation map[string]interface{}) map[string]interface{} {
	if strings.TrimSpace(stringFromAny(invocation["kind"])) != "" ||
		!operationPlanInvocationIsExecutionEvidence(invocation) {
		return invocation
	}
	normalized := copyStringAnyMap(invocation)
	normalized["kind"] = "tool_call"
	return normalized
}

func operationPlanInvocationExpectedActionTarget(invocation map[string]interface{}) map[string]interface{} {
	arguments := mapFromOperationContext(invocation["arguments"])
	target := copyStringAnyMap(mapFromOperationContext(arguments["operation_plan_target"]))
	if target == nil {
		target = map[string]interface{}{}
	}
	for _, key := range []string{"agent_id", "file_id", "asset_id", "resource_id", "dataset_id", "data_source_id", "table_id", "workflow_id", "binding_id", "href", "route"} {
		if _, exists := target[key]; exists {
			// The execution trace stores summarized arguments, while
			// operation_plan_target is captured from the raw authorized call.
			// Never replace the exact target with a redacted type/length summary.
			continue
		}
		value := operationPlanInvocationArgumentValue(arguments, key)
		if value != "" {
			target[key] = value
		}
	}
	return target
}

func operationPlanInvocationArgumentValue(arguments map[string]interface{}, key string) string {
	if value := strings.TrimSpace(stringFromAny(arguments[key])); value != "" {
		return value
	}
	for _, containerKey := range []string{"target", "system_prompt_source", "agent", "file", "resource"} {
		if value := strings.TrimSpace(stringFromAny(mapFromOperationContext(arguments[containerKey])[key])); value != "" {
			return value
		}
	}
	if patch := mapFromOperationContext(arguments["system_prompt_patch"]); len(patch) > 0 {
		if value := strings.TrimSpace(stringFromAny(mapFromOperationContext(patch["source"])[key])); value != "" {
			return value
		}
	}
	return ""
}

func operationPlanAdvanceNextPendingPhase(phases []map[string]interface{}) {
	for _, phase := range phases {
		status := strings.ToLower(strings.TrimSpace(stringFromAny(phase["status"])))
		if status == "in_progress" || status == "running" {
			return
		}
	}
	for _, phase := range phases {
		if strings.EqualFold(strings.TrimSpace(stringFromAny(phase["status"])), operationPlanStepStatusPending) {
			phase["status"] = "in_progress"
			return
		}
	}
}

func operationPlanMarkEvidenceCurrent(plan map[string]interface{}) {
	current := operationPlanCurrentEvidenceRevision(plan)
	plan["evidence_revision"] = current
	plan["evidence_revision_at_plan_update"] = current
	plan["evidence_sequence_at_plan_update"] = current
	plan["evidence_after_last_plan_update"] = 0
	plan["plan_sync_status"] = "current"
	delete(plan, "plan_revision_reason")
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
		// The turn's answer is in. Steps the model left open keep the plan
		// running, so the next turn sees the task as unfinished (ADR 0049).
		if open := operationPlanOpenSteps(plan); len(open) > 0 {
			verification["status"] = "incomplete"
			verification["reason"] = "the turn ended with plan steps still open"
			verification["missing_steps"] = compactCompletionVerificationStringList(open, 12, 160)
			plan["status"] = operationPlanStatusRunning
			plan["pending_next_action"] = "complete_remaining_plan_steps"
		} else {
			plan["status"] = operationPlanStatusCompleted
			plan["pending_next_action"] = "none"
		}
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

func operationPlanPhaseCanCompleteWithFinalAnswer(phase map[string]interface{}) bool {
	// An answer can finish an explicitly non-tool phase, but cannot replace a
	// bound action or an outcome's server-owned runtime acceptance conditions.
	if len(mapFromOperationContext(phase["expected_action"])) > 0 ||
		strings.TrimSpace(stringFromAny(phase["outcome_id"])) != "" {
		return false
	}
	verification := strings.ToLower(strings.TrimSpace(stringFromAny(phase["verification_mode"])))
	if verification != "" && verification != "model_reconciliation" && verification != "final_answer" {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(stringFromAny(phase["completion_mode"]))) {
	case "tool":
		return false
	case "final_answer", "non_tool":
		return true
	default:
		return verification == "model_reconciliation"
	}
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
		metadata["evidence_ledger_ref"] = map[string]interface{}{
			"source":   "operation_plan.evidence_ledger",
			"revision": operationPlanCurrentEvidenceRevision(plan),
			"count":    len(ledger),
		}
		delete(metadata, "evidence_ledger")
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
	case "":
		// An empty kind is the historical tool-call representation. Require a
		// tool identity so unrelated legacy metadata does not become actionable.
		return strings.TrimSpace(stringFromAny(invocation["skill_id"])) != "" ||
			strings.TrimSpace(stringFromAny(invocation["tool_name"])) != ""
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
	if status, native := connectorPlanInvocationStatus(invocation); native {
		return status
	}
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
	if !operationPlanInvocationIsExecutionEvidence(invocation) {
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
	case "replace_agent_memory_blocks", "replace_agent_memory_slots":
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
