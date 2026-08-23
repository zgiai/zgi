package service

import (
	"strings"
	"time"

	"github.com/zgiai/zgi/api/internal/capabilities/toolgovernance"
	"github.com/zgiai/zgi/api/internal/modules/skills"
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
	requestedPhaseID := strings.TrimSpace(stringFromAny(event["plan_phase_id"]))
	match := governedInvocationPlanPhaseIndex(phases, frozen, requestedPhaseID, completionIntent)
	if match < 0 {
		return metadata
	}
	expected := mapFromOperationContext(phases[match]["expected_action"])
	projectedExternalAction := operationPlanExpectedActionIsServerProjected(expected)
	if projectedExternalAction && (requestedPhaseID == "" ||
		strings.TrimSpace(stringFromAny(phases[match][operationPlanServerProjectedEpochKey])) == "" ||
		strings.TrimSpace(stringFromAny(expected[operationPlanServerProjectedBindingKey])) == "" ||
		strings.TrimSpace(stringFromAny(frozen.Arguments["connection_id"])) == "") {
		// A projected mutation must be bound to a concrete server phase instance.
		// A unique-looking model phase without the server epoch is not sufficient.
		return metadata
	}
	binding := governedInvocationPlanBinding(frozen)
	if completionIntent != "" {
		binding["completion_intent"] = completionIntent
	}
	if phaseID := strings.TrimSpace(stringFromAny(phases[match]["id"])); phaseID != "" {
		binding["phase_id"] = phaseID
	}
	if projectedExternalAction {
		binding["projected_external_action"] = true
		binding[operationPlanServerProjectedEpochKey] = strings.TrimSpace(stringFromAny(phases[match][operationPlanServerProjectedEpochKey]))
		binding[operationPlanServerProjectedBindingKey] = strings.TrimSpace(stringFromAny(expected[operationPlanServerProjectedBindingKey]))
		if targetArguments := copyStringAnyMap(mapFromOperationContext(expected["target_arguments"])); len(targetArguments) > 0 {
			binding["target_arguments"] = targetArguments
		}
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
	if operationPlanExpectedActionIsServerProjected(expected) {
		return governedProjectedInvocationExpectedActionMatches(expected, frozen)
	}
	return operationPlanExpectedActionMatches(expected, frozen.SkillID, frozen.ToolName, governedInvocationPlanTarget(frozen))
}

func governedProjectedInvocationExpectedActionMatches(expected map[string]interface{}, frozen toolgovernance.FrozenInvocation) bool {
	if !operationPlanExpectedActionIsServerProjected(expected) ||
		!strings.EqualFold(strings.TrimSpace(frozen.SkillID), "external-apps") ||
		!strings.EqualFold(strings.TrimSpace(frozen.ToolName), "execute_action") {
		return false
	}
	expectedTarget := mapFromOperationContext(expected["target"])
	if !strings.EqualFold(strings.TrimSpace(stringFromAny(expectedTarget["integration_id"])), strings.TrimSpace(stringFromAny(frozen.Arguments["integration_id"]))) ||
		!strings.EqualFold(strings.TrimSpace(stringFromAny(expectedTarget["action_id"])), strings.TrimSpace(stringFromAny(frozen.Arguments["action_id"]))) {
		return false
	}
	expectedTargetArguments := copyStringAnyMap(mapFromOperationContext(expected["target_arguments"]))
	if expectedTargetArguments == nil {
		expectedTargetArguments = map[string]interface{}{}
	}
	actualTargetArguments := operationPlanProjectedTargetArgumentsFromBusinessArguments(
		expectedTargetArguments,
		mapFromOperationContext(frozen.Arguments["arguments"]),
	)
	return operationPlanExactTargetArgumentsEqual(expectedTargetArguments, actualTargetArguments)
}

func operationPlanProjectedTargetArgumentsFromBusinessArguments(
	expected map[string]interface{},
	businessArguments map[string]interface{},
) map[string]interface{} {
	if len(expected) == 0 {
		return map[string]interface{}{}
	}
	out := make(map[string]interface{}, len(expected))
	for path := range expected {
		current := interface{}(businessArguments)
		for _, segment := range strings.Split(path, ".") {
			container, ok := current.(map[string]interface{})
			if !ok {
				current = nil
				break
			}
			current, ok = container[segment]
			if !ok {
				current = nil
				break
			}
		}
		if current != nil {
			out[path] = current
		}
	}
	return out
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
		matches := expectedText == actualText
		if key == "integration_id" || key == "action_id" || key == "href" || key == "route" {
			matches = strings.EqualFold(expectedText, actualText)
		}
		if !matches {
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
	for _, key := range []string{
		"integration_id", "action_id", "connection_id", "batch_id", "operation_item_id",
		"agent_id", "file_id", "asset_id", "resource_id", "dataset_id", "data_source_id", "table_id", "workflow_id", "binding_id",
	} {
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
	phases := mapSliceFromAny(plan["phases"])
	match := governedInvocationBoundPlanPhaseIndex(phases, frozen)
	if operationPlanHasStructuredOutcomes(plan) {
		if approvedFrozenExternalAction(frozen) && match >= 0 {
			invocation := governedProjectedExternalActionCompletionInvocation(phases[match], frozen)
			operationPlanRecordActionAttempt(plan, invocation, operationPlanStepStatusCompleted)
			operationPlanRecordInvocationEffects(plan, invocation, operationPlanStepStatusCompleted)
		}
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

func governedInvocationBoundPlanPhaseIndex(phases []map[string]interface{}, frozen toolgovernance.FrozenInvocation) int {
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
		if projected, _ := binding["projected_external_action"].(bool); projected &&
			!governedProjectedInvocationBindingMatchesPhase(binding, phase, frozen) {
			continue
		}
		if match >= 0 {
			return -1
		}
		match = index
	}
	return match
}

func governedProjectedInvocationBindingMatchesPhase(
	binding map[string]interface{},
	phase map[string]interface{},
	frozen toolgovernance.FrozenInvocation,
) bool {
	projected, _ := binding["projected_external_action"].(bool)
	if !projected || !operationPlanExpectedActionIsServerProjected(mapFromOperationContext(phase["expected_action"])) ||
		!governedProjectedInvocationExpectedActionMatches(mapFromOperationContext(phase["expected_action"]), frozen) {
		return false
	}
	phaseEpoch := strings.TrimSpace(stringFromAny(phase[operationPlanServerProjectedEpochKey]))
	bindingEpoch := strings.TrimSpace(stringFromAny(binding[operationPlanServerProjectedEpochKey]))
	if phaseEpoch == "" || bindingEpoch == "" || phaseEpoch != bindingEpoch {
		return false
	}
	expectedFingerprint := strings.TrimSpace(stringFromAny(mapFromOperationContext(phase["expected_action"])[operationPlanServerProjectedBindingKey]))
	bindingFingerprint := strings.TrimSpace(stringFromAny(binding[operationPlanServerProjectedBindingKey]))
	if expectedFingerprint == "" || bindingFingerprint == "" || expectedFingerprint != bindingFingerprint {
		return false
	}
	bindingTarget := mapFromOperationContext(binding["target"])
	if !strings.EqualFold(strings.TrimSpace(stringFromAny(bindingTarget["integration_id"])), strings.TrimSpace(stringFromAny(frozen.Arguments["integration_id"]))) ||
		!strings.EqualFold(strings.TrimSpace(stringFromAny(bindingTarget["action_id"])), strings.TrimSpace(stringFromAny(frozen.Arguments["action_id"]))) {
		return false
	}
	connectionID := strings.TrimSpace(stringFromAny(frozen.Arguments["connection_id"]))
	if connectionID == "" || strings.TrimSpace(stringFromAny(bindingTarget["connection_id"])) != connectionID {
		return false
	}
	expected := mapFromOperationContext(phase["expected_action"])
	expectedTargetArguments := copyStringAnyMap(mapFromOperationContext(expected["target_arguments"]))
	if expectedTargetArguments == nil {
		expectedTargetArguments = map[string]interface{}{}
	}
	return operationPlanExactTargetArgumentsEqual(expectedTargetArguments, mapFromOperationContext(binding["target_arguments"]))
}

func approvedFrozenProjectedExternalActionOperationItemID(
	metadata map[string]interface{},
	frozen toolgovernance.FrozenInvocation,
) string {
	if !approvedFrozenExternalAction(frozen) {
		return ""
	}
	plan := mapFromOperationContext(metadata["operation_plan"])
	phases := mapSliceFromAny(plan["phases"])
	match := -1
	for index, phase := range phases {
		binding := mapFromOperationContext(phase[operationPlanRuntimeBindingKey])
		if !governedInvocationPlanBindingMatches(binding, frozen) ||
			strings.TrimSpace(stringFromAny(binding["phase_id"])) != strings.TrimSpace(stringFromAny(phase["id"])) ||
			!governedProjectedInvocationBindingMatchesPhase(binding, phase, frozen) {
			continue
		}
		if match >= 0 {
			return ""
		}
		match = index
	}
	if match < 0 {
		return ""
	}
	phase := phases[match]
	binding := mapFromOperationContext(phase[operationPlanRuntimeBindingKey])
	return skills.ProjectedExternalActionOperationItemID(
		strings.TrimSpace(stringFromAny(phase["id"])),
		strings.TrimSpace(stringFromAny(binding[operationPlanServerProjectedEpochKey])),
		strings.TrimSpace(stringFromAny(binding[operationPlanServerProjectedBindingKey])),
		strings.TrimSpace(stringFromAny(frozen.Arguments["integration_id"])),
		strings.TrimSpace(stringFromAny(frozen.Arguments["action_id"])),
		strings.TrimSpace(stringFromAny(frozen.Arguments["connection_id"])),
	)
}

func governedProjectedExternalActionCompletionInvocation(
	phase map[string]interface{},
	frozen toolgovernance.FrozenInvocation,
) map[string]interface{} {
	binding := mapFromOperationContext(phase[operationPlanRuntimeBindingKey])
	arguments := copyStringAnyMap(frozen.Arguments)
	if arguments == nil {
		arguments = map[string]interface{}{}
	}
	arguments["plan_phase_id"] = strings.TrimSpace(stringFromAny(phase["id"]))
	arguments[operationPlanServerProjectedEpochKey] = strings.TrimSpace(stringFromAny(binding[operationPlanServerProjectedEpochKey]))
	arguments[operationPlanServerProjectedBindingKey] = strings.TrimSpace(stringFromAny(binding[operationPlanServerProjectedBindingKey]))
	arguments["operation_plan_target"] = copyStringAnyMap(mapFromOperationContext(binding["target_arguments"]))
	return map[string]interface{}{
		"kind":       "tool_call",
		"status":     "success",
		"runtime_id": strings.TrimSpace(frozen.ID),
		"skill_id":   strings.TrimSpace(frozen.SkillID),
		"tool_name":  strings.TrimSpace(frozen.ToolName),
		"arguments":  arguments,
		"result": map[string]interface{}{
			"operation_status":           "completed",
			"provider_success_confirmed": true,
			"integration_id":             strings.TrimSpace(stringFromAny(frozen.Arguments["integration_id"])),
			"action_id":                  strings.TrimSpace(stringFromAny(frozen.Arguments["action_id"])),
			"connection_id":              strings.TrimSpace(stringFromAny(frozen.Arguments["connection_id"])),
		},
	}
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
