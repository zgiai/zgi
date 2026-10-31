package service

import (
	"strings"
)

const (
	operationPlanRuntimeBindingKey            = "runtime_binding"
	governedCompletionIntentFinalizeIfSuccess = "finalize_if_success"
)

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

func operationPlanPhasesTerminal(plan map[string]interface{}, phases []map[string]interface{}) bool {
	if len(phases) == 0 {
		return false
	}
	for _, phase := range phases {
		if !operationPlanPhaseBlocksTerminal(plan, phase) {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(stringFromAny(phase["status"]))) {
		case operationPlanStepStatusCompleted, "skipped":
		default:
			return false
		}
	}
	return true
}

func operationPlanPhaseBlocksTerminal(plan map[string]interface{}, phase map[string]interface{}) bool {
	switch strings.ToLower(strings.TrimSpace(stringFromAny(phase[operationPlanPhaseSourceKey]))) {
	case operationPlanPhaseSourceModelAdvisory:
		return false
	case operationPlanPhaseSourceServerOutcome:
		// The source label is not an attestation. Only a phase linked to an
		// Outcome currently present in the server-owned plan may block terminal
		// completion, and the Outcome owns the required/optional decision.
		outcome := operationPlanPhaseLinkedOutcome(plan, phase)
		if len(outcome) == 0 {
			return false
		}
		if required, exists := outcome["required"]; exists {
			return operationPlanBoolValue(required)
		}
		return true
	default:
		if strings.TrimSpace(stringFromAny(phase[operationPlanPhaseSourceKey])) != "" {
			return false
		}
		// Historical phases predate phase_source. Preserve a valid Outcome link
		// first, then fall back to their explicit required flag. New model phases
		// are normalized with model_advisory before reaching persistence.
		if outcome := operationPlanPhaseLinkedOutcome(plan, phase); len(outcome) > 0 {
			if required, exists := outcome["required"]; exists {
				return operationPlanBoolValue(required)
			}
			return true
		}
		if required, exists := phase["required"]; exists {
			return operationPlanBoolValue(required)
		}
		return true
	}
}

func operationPlanPhaseLinkedOutcome(plan map[string]interface{}, phase map[string]interface{}) map[string]interface{} {
	outcomeID := strings.TrimSpace(stringFromAny(phase["outcome_id"]))
	if outcomeID == "" {
		return nil
	}
	for _, outcome := range mapSliceFromAny(plan[operationPlanOutcomesKey]) {
		if strings.EqualFold(strings.TrimSpace(stringFromAny(outcome["id"])), outcomeID) {
			return outcome
		}
	}
	return nil
}
