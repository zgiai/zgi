package service

import (
	"strings"
	"time"

	"github.com/zgiai/zgi/api/internal/capabilities/toolgovernance"
)

const (
	operationPlanRuntimeBindingKey            = "runtime_binding"
	governedCompletionIntentFinalizeIfSuccess = "finalize_if_success"
)

func bindPendingGovernedInvocationToOperationPlan(metadata map[string]interface{}, event map[string]interface{}) map[string]interface{} {
	frozen, ok, err := toolGovernanceFrozenInvocationFromEvent(event)
	if err != nil || !ok {
		return metadata
	}
	plan := copyStringAnyMap(mapFromOperationContext(metadata["operation_plan"]))
	if len(plan) == 0 {
		return metadata
	}
	phases := mapSliceFromAny(plan["phases"])
	completionIntent := normalizeGovernedCompletionIntent(event["completion_intent"])
	match := governedInvocationPlanPhaseIndex(phases, frozen, strings.TrimSpace(stringFromAny(event["plan_phase_id"])), completionIntent)
	if match < 0 {
		return metadata
	}
	binding := governedInvocationPlanBinding(frozen)
	if completionIntent != "" {
		binding["completion_intent"] = completionIntent
	}
	if phaseID := strings.TrimSpace(stringFromAny(phases[match]["id"])); phaseID != "" {
		binding["phase_id"] = phaseID
	}
	phases[match][operationPlanRuntimeBindingKey] = binding
	plan["phases"] = mapsToInterfaceSlice(phases)
	next := copyStringAnyMap(metadata)
	next["operation_plan"] = plan
	return next
}

func governedInvocationPlanPhaseIndex(phases []map[string]interface{}, frozen toolgovernance.FrozenInvocation, requestedPhaseID string, completionIntent string) int {
	requestedPhaseID = strings.TrimSpace(requestedPhaseID)
	if requestedPhaseID != "" {
		for index, phase := range phases {
			switch strings.ToLower(strings.TrimSpace(stringFromAny(phase["status"]))) {
			case operationPlanStepStatusCompleted, "skipped", operationPlanStepStatusFailed:
				continue
			}
			if requestedPhaseID != strings.TrimSpace(stringFromAny(phase["id"])) {
				continue
			}
			expected := mapFromOperationContext(phase["expected_action"])
			if len(expected) == 0 || governedInvocationExpectedActionMatches(expected, frozen) {
				return index
			}
			return -1
		}
		return -1
	}
	match := -1
	for index, phase := range phases {
		switch strings.ToLower(strings.TrimSpace(stringFromAny(phase["status"]))) {
		case operationPlanStepStatusCompleted, "skipped", operationPlanStepStatusFailed:
			continue
		}
		if !governedInvocationExpectedActionMatches(mapFromOperationContext(phase["expected_action"]), frozen) {
			continue
		}
		if match >= 0 {
			return -1
		}
		match = index
	}
	if match < 0 && completionIntent == governedCompletionIntentFinalizeIfSuccess {
		for index, phase := range phases {
			if !governedCompletionIntentPhaseCandidate(phase) {
				continue
			}
			if match >= 0 {
				return -1
			}
			match = index
		}
	}
	return match
}

func normalizeGovernedCompletionIntent(value interface{}) string {
	if strings.EqualFold(strings.TrimSpace(stringFromAny(value)), governedCompletionIntentFinalizeIfSuccess) {
		return governedCompletionIntentFinalizeIfSuccess
	}
	return ""
}

func governedCompletionIntentPhaseCandidate(phase map[string]interface{}) bool {
	switch strings.ToLower(strings.TrimSpace(stringFromAny(phase["status"]))) {
	case "pending", "in_progress", "running":
	default:
		return false
	}
	return len(mapFromOperationContext(phase["expected_action"])) == 0 &&
		strings.EqualFold(strings.TrimSpace(stringFromAny(phase["verification_mode"])), "model_reconciliation")
}

func governedInvocationExpectedActionMatches(expected map[string]interface{}, frozen toolgovernance.FrozenInvocation) bool {
	return operationPlanExpectedActionMatches(expected, frozen.SkillID, frozen.ToolName, governedInvocationPlanTarget(frozen))
}

func operationPlanExpectedActionMatches(expected map[string]interface{}, skillID string, toolName string, actualTarget map[string]interface{}) bool {
	if len(expected) == 0 ||
		!strings.EqualFold(strings.TrimSpace(stringFromAny(expected["skill_id"])), strings.TrimSpace(skillID)) ||
		!strings.EqualFold(strings.TrimSpace(stringFromAny(expected["tool_name"])), strings.TrimSpace(toolName)) {
		return false
	}
	expectedTarget := mapFromOperationContext(expected["target"])
	if len(expectedTarget) == 0 {
		return true
	}
	for key, expectedValue := range expectedTarget {
		expectedText := normalizeConsoleNavigationGuardHref(strings.TrimSpace(stringFromAny(expectedValue)))
		actualText := normalizeConsoleNavigationGuardHref(strings.TrimSpace(stringFromAny(actualTarget[key])))
		if key != "href" && key != "route" {
			expectedText = strings.TrimSpace(stringFromAny(expectedValue))
			actualText = strings.TrimSpace(stringFromAny(actualTarget[key]))
		}
		if !strings.EqualFold(expectedText, actualText) {
			return false
		}
	}
	return true
}

func governedInvocationPlanBinding(frozen toolgovernance.FrozenInvocation) map[string]interface{} {
	binding := map[string]interface{}{
		"type":                 "tool_governance",
		"skill_id":             strings.TrimSpace(frozen.SkillID),
		"tool_name":            strings.TrimSpace(frozen.ToolName),
		"frozen_invocation_id": strings.TrimSpace(frozen.ID),
		"idempotency_key":      strings.TrimSpace(frozen.IdempotencyKey),
		"correlation_id":       strings.TrimSpace(frozen.CorrelationID),
	}
	if target := governedInvocationPlanTarget(frozen); len(target) > 0 {
		binding["target"] = target
	}
	return binding
}

func governedInvocationPlanTarget(frozen toolgovernance.FrozenInvocation) map[string]interface{} {
	target := map[string]interface{}{}
	for _, key := range []string{"agent_id", "file_id", "asset_id", "resource_id", "dataset_id", "data_source_id", "table_id", "workflow_id", "binding_id"} {
		if value := strings.TrimSpace(stringFromAny(frozen.Arguments[key])); value != "" {
			target[key] = value
		}
	}
	assets := frozen.Assets
	if len(assets) == 0 {
		assets = frozen.ExpectedAssets
	}
	if len(assets) == 1 {
		if value := strings.TrimSpace(assets[0].ID); value != "" {
			target["asset_id"] = value
		}
		if value := strings.TrimSpace(assets[0].Type); value != "" {
			target["asset_type"] = value
		}
	}
	return target
}

func completeBoundGovernedInvocationOperationPlan(metadata map[string]interface{}, frozen toolgovernance.FrozenInvocation) (map[string]interface{}, bool) {
	plan := copyStringAnyMap(mapFromOperationContext(metadata["operation_plan"]))
	if len(plan) == 0 {
		return metadata, false
	}
	if operationPlanHasStructuredOutcomes(plan) {
		operationPlanReconcileOutcomes(plan)
		terminal := operationPlanOutcomesTerminal(mapSliceFromAny(plan[operationPlanOutcomesKey]))
		operationPlanSyncStrategyState(plan)
		next := copyStringAnyMap(metadata)
		next["operation_plan"] = plan
		if terminal {
			summary := copyStringAnyMap(mapFromOperationContext(next["operation_result_summary"]))
			if summary == nil {
				summary = map[string]interface{}{}
			}
			summary["status"] = operationPlanStatusCompleted
			summary["plan_status"] = operationPlanStatusCompleted
			summary["pending_next_action"] = "none"
			summary["updated_at"] = time.Now().UTC().Format(time.RFC3339)
			next["operation_result_summary"] = summary
		}
		return next, terminal
	}
	phases := mapSliceFromAny(plan["phases"])
	match := -1
	for index, phase := range phases {
		binding := mapFromOperationContext(phase[operationPlanRuntimeBindingKey])
		if !governedInvocationPlanBindingMatches(binding, frozen) {
			continue
		}
		if phaseID := strings.TrimSpace(stringFromAny(binding["phase_id"])); phaseID != "" && phaseID != strings.TrimSpace(stringFromAny(phase["id"])) {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(stringFromAny(phase["status"]))) {
		case "pending", "in_progress", "running":
		default:
			continue
		}
		if match >= 0 {
			return metadata, false
		}
		match = index
	}
	if match < 0 {
		return metadata, false
	}
	if len(mapFromOperationContext(phases[match]["expected_action"])) == 0 {
		binding := mapFromOperationContext(phases[match][operationPlanRuntimeBindingKey])
		if normalizeGovernedCompletionIntent(binding["completion_intent"]) != governedCompletionIntentFinalizeIfSuccess ||
			!strings.EqualFold(strings.TrimSpace(stringFromAny(phases[match]["verification_mode"])), "model_reconciliation") {
			// The frozen call proves which action was approved and executed, but an
			// unstructured phase still has no machine-checkable acceptance fact.
			// Only the model's explicit final-action declaration, frozen into the
			// runtime binding before approval, may close an advisory reconciliation
			// phase after the exact invocation succeeds.
			return metadata, false
		}
	}
	phases[match]["status"] = operationPlanStepStatusCompleted
	phases[match]["completed_at"] = time.Now().UTC().Format(time.RFC3339)
	refs := stringSliceFromAny(phases[match]["evidence_refs"])
	refs = appendUniqueStrings(refs, operationPlanToolEvidenceKey(frozen.SkillID, frozen.ToolName))
	if id := strings.TrimSpace(frozen.ID); id != "" {
		refs = appendUniqueStrings(refs, "invocation_id:"+id)
	}
	phases[match]["evidence_refs"] = refs
	operationPlanAdvanceNextPendingPhase(phases)
	plan["phases"] = mapsToInterfaceSlice(phases)
	operationPlanMarkEvidenceCurrent(plan)
	terminal := operationPlanPhasesTerminal(phases)
	if terminal {
		plan["status"] = operationPlanStatusCompleted
		plan["pending_next_action"] = "none"
	}
	operationPlanSyncStrategyState(plan)
	next := copyStringAnyMap(metadata)
	next["operation_plan"] = plan
	if terminal {
		summary := copyStringAnyMap(mapFromOperationContext(next["operation_result_summary"]))
		if summary == nil {
			summary = map[string]interface{}{}
		}
		summary["status"] = operationPlanStatusCompleted
		summary["plan_status"] = operationPlanStatusCompleted
		summary["pending_next_action"] = "none"
		summary["updated_at"] = time.Now().UTC().Format(time.RFC3339)
		next["operation_result_summary"] = summary
	}
	return next, terminal
}

func governedInvocationPlanBindingMatches(binding map[string]interface{}, frozen toolgovernance.FrozenInvocation) bool {
	if len(binding) == 0 || !strings.EqualFold(strings.TrimSpace(stringFromAny(binding["type"])), "tool_governance") {
		return false
	}
	if !strings.EqualFold(strings.TrimSpace(stringFromAny(binding["skill_id"])), strings.TrimSpace(frozen.SkillID)) ||
		!strings.EqualFold(strings.TrimSpace(stringFromAny(binding["tool_name"])), strings.TrimSpace(frozen.ToolName)) {
		return false
	}
	if id := strings.TrimSpace(stringFromAny(binding["frozen_invocation_id"])); id != "" {
		return id == strings.TrimSpace(frozen.ID)
	}
	if key := strings.TrimSpace(stringFromAny(binding["idempotency_key"])); key != "" {
		return key == strings.TrimSpace(frozen.IdempotencyKey)
	}
	return false
}

func operationPlanPhasesTerminal(phases []map[string]interface{}) bool {
	if len(phases) == 0 {
		return false
	}
	for _, phase := range phases {
		switch strings.ToLower(strings.TrimSpace(stringFromAny(phase["status"]))) {
		case operationPlanStepStatusCompleted, "skipped":
		default:
			return false
		}
	}
	return true
}
