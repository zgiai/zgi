package skillloop

import (
	"strings"

	"github.com/zgiai/zgi/api/internal/modules/skills"
)

const operationPlanPhaseMismatchCode = "operation_plan_phase_mismatch"
const operationPlanPhaseSourceKey = "phase_source"
const operationPlanPhaseSourceServerOutcome = "server_outcome"
const operationPlanPhaseSourceModelAdvisory = "model_advisory"

func resolveOperationPlanPhaseForSkillCall(
	runtimeState map[string]interface{},
	requestedPhaseID string,
	skillID string,
	toolName string,
	arguments map[string]interface{},
) (string, bool, error) {
	plan := evidenceMapFromAny(runtimeState["operation_plan"])
	phases := evidenceMapsFromAny(plan["phases"])
	if len(phases) == 0 {
		return strings.TrimSpace(requestedPhaseID), false, nil
	}

	requestedPhaseID = strings.TrimSpace(requestedPhaseID)
	if requestedPhaseID != "" {
		for _, phase := range phases {
			if !operationPlanPhaseOpenForToolCall(phase) {
				continue
			}
			phaseID := strings.TrimSpace(evidenceStringFromAny(phase["id"]))
			if strings.EqualFold(phaseID, requestedPhaseID) {
				// A phase ID is an evidence-association hint for ordinary tools, not
				// an authorization boundary. The runtime records the exact call and
				// only completes the phase after a successful matching outcome. Tool
				// Governance separately freezes side-effecting calls that need exact
				// approval binding.
				return phaseID, true, nil
			}
		}
		// Stale or unknown phase IDs must not block a safe prerequisite call.
		// Drop the association and keep the concrete execution in the ledger.
		return "", false, nil
	}

	matches := make([]string, 0, 1)
	for _, phase := range phases {
		if !operationPlanPhaseOpenForToolCall(phase) {
			continue
		}
		phaseID := strings.TrimSpace(evidenceStringFromAny(phase["id"]))
		expected := evidenceMapFromAny(phase["expected_action"])
		if len(expected) == 0 {
			continue
		}
		if operationPlanExpectedActionMatchesSkillCall(expected, skillID, toolName, arguments) {
			matches = append(matches, phaseID)
		}
	}
	if len(matches) == 1 && strings.TrimSpace(matches[0]) != "" {
		return matches[0], true, nil
	}
	if len(matches) > 1 {
		current := operationPlanCurrentOpenPhaseID(phases)
		for _, phase := range phases {
			if phase["id"] != current || len(connectorPhaseOperation(phase)) == 0 {
				continue
			}
			for _, match := range matches {
				if match == current {
					return current, true, nil
				}
			}
		}
	}
	return "", false, nil
}

func operationPlanCurrentOpenPhaseID(phases []map[string]interface{}) string {
	firstOpen := ""
	inProgress := ""
	for _, phase := range phases {
		if !operationPlanPhaseOpenForToolCall(phase) {
			continue
		}
		phaseID := strings.TrimSpace(evidenceStringFromAny(phase["id"]))
		if phaseID == "" {
			continue
		}
		if firstOpen == "" {
			firstOpen = phaseID
		}
		if strings.EqualFold(strings.TrimSpace(evidenceStringFromAny(phase["status"])), "in_progress") ||
			strings.EqualFold(strings.TrimSpace(evidenceStringFromAny(phase["status"])), "running") {
			if inProgress != "" {
				return ""
			}
			inProgress = phaseID
		}
	}
	if inProgress != "" {
		return inProgress
	}
	return firstOpen
}

func operationPlanPhaseOpenForToolCall(phase map[string]interface{}) bool {
	switch strings.ToLower(strings.TrimSpace(evidenceStringFromAny(phase["status"]))) {
	case "pending", "in_progress", "running":
		return true
	default:
		return false
	}
}

func operationPlanExpectedActionMatchesSkillCall(expected map[string]interface{}, skillID string, toolName string, arguments map[string]interface{}) bool {
	if len(expected) == 0 ||
		!strings.EqualFold(strings.TrimSpace(evidenceStringFromAny(expected["skill_id"])), strings.TrimSpace(skillID)) ||
		!strings.EqualFold(strings.TrimSpace(evidenceStringFromAny(expected["tool_name"])), strings.TrimSpace(toolName)) {
		return false
	}
	for key, expectedValue := range evidenceMapFromAny(expected["target"]) {
		expectedText := normalizeOperationPlanTargetValue(key, evidenceStringFromAny(expectedValue))
		actualText := normalizeOperationPlanTargetValue(key, operationPlanSkillCallTargetValue(arguments, key))
		if expectedText == "" || !strings.EqualFold(expectedText, actualText) {
			return false
		}
	}
	for path, expectedValue := range evidenceMapFromAny(expected["target_arguments"]) {
		expectedText := strings.TrimSpace(evidenceStringFromAny(expectedValue))
		actualText := operationPlanArgumentPathValue(evidenceMapFromAny(arguments["arguments"]), path)
		if actualText == "" {
			actualText = operationPlanArgumentPathValue(arguments, path)
		}
		if expectedText == "" || expectedText != strings.TrimSpace(actualText) {
			return false
		}
	}
	return true
}

func operationPlanArgumentPathValue(arguments map[string]interface{}, path string) string {
	path = strings.TrimSpace(path)
	if len(arguments) == 0 || path == "" {
		return ""
	}
	current := interface{}(arguments)
	for _, segment := range strings.Split(path, ".") {
		segment = strings.TrimSpace(segment)
		if segment == "" {
			return ""
		}
		mapped := evidenceMapFromAny(current)
		if len(mapped) == 0 {
			return ""
		}
		current = mapped[segment]
	}
	return strings.TrimSpace(evidenceStringFromAny(current))
}

func operationPlanSkillCallTargetValue(arguments map[string]interface{}, key string) string {
	if value := strings.TrimSpace(evidenceStringFromAny(arguments[key])); value != "" {
		return value
	}
	for _, containerKey := range []string{"arguments", "target", "system_prompt_source", "agent", "file", "resource"} {
		if value := strings.TrimSpace(evidenceStringFromAny(evidenceMapFromAny(arguments[containerKey])[key])); value != "" {
			return value
		}
	}
	if patch := evidenceMapFromAny(arguments["system_prompt_patch"]); len(patch) > 0 {
		if value := strings.TrimSpace(evidenceStringFromAny(evidenceMapFromAny(patch["source"])[key])); value != "" {
			return value
		}
	}
	return ""
}

func operationPlanPhaseServerClassifiedNonTool(phase map[string]interface{}, runtimeState map[string]interface{}) bool {
	outcome := operationPlanOutcomeForPhase(phase, runtimeState)
	if len(outcome) > 0 {
		// A linked server Outcome owns its verification policy. Model-authored
		// completion_mode/verification_mode values on the phase cannot reclassify
		// a runtime-effect Outcome as final-answer-only.
		switch strings.ToLower(strings.TrimSpace(evidenceStringFromAny(outcome["verification_mode"]))) {
		case "final_answer", "non_tool":
			return true
		default:
			return false
		}
	}
	switch strings.ToLower(strings.TrimSpace(firstNonEmptyString(
		phase["verification_mode"],
		phase["completion_mode"],
	))) {
	case "final_answer", "non_tool":
		return true
	default:
		return false
	}
}

func operationPlanPhaseRequiredByServer(phase map[string]interface{}, runtimeState map[string]interface{}) bool {
	if len(connectorPhaseOperation(phase)) > 0 {
		return true
	}
	source := strings.ToLower(strings.TrimSpace(evidenceStringFromAny(phase[operationPlanPhaseSourceKey])))
	switch source {
	case operationPlanPhaseSourceModelAdvisory:
		// A model-authored progress item is useful context, but it is not a
		// server acceptance contract even if the model also supplied required.
		return false
	case operationPlanPhaseSourceServerOutcome:
		outcome := operationPlanOutcomeForPhase(phase, runtimeState)
		if len(outcome) == 0 {
			return false
		}
		if required, exists := outcome["required"].(bool); exists {
			return required
		}
		return true
	default:
		if source != "" {
			return false
		}
	}

	// Backward compatibility: an existing valid Outcome linkage or an
	// explicitly classified legacy phase retains its historical authority.
	// A new unclassified model phase no longer becomes required by omission.
	if outcome := operationPlanOutcomeForPhase(phase, runtimeState); len(outcome) > 0 {
		if required, exists := outcome["required"].(bool); exists {
			return required
		}
		return true
	}
	if required, exists := phase["required"].(bool); exists {
		return required
	}
	return false
}

// operationPlanPhaseHasServerAuthority requires a current server-owned Outcome.
// Model phase labels alone cannot create an acceptance contract.
func operationPlanPhaseHasServerAuthority(phase map[string]interface{}, runtimeState map[string]interface{}) bool {
	switch strings.ToLower(strings.TrimSpace(evidenceStringFromAny(phase[operationPlanPhaseSourceKey]))) {
	case operationPlanPhaseSourceModelAdvisory:
		return false
	case operationPlanPhaseSourceServerOutcome:
		return len(operationPlanOutcomeForPhase(phase, runtimeState)) > 0
	default:
		return len(operationPlanOutcomeForPhase(phase, runtimeState)) > 0
	}
}

func operationPlanOutcomeForPhase(phase map[string]interface{}, runtimeState map[string]interface{}) map[string]interface{} {
	outcomeID := strings.TrimSpace(evidenceStringFromAny(phase["outcome_id"]))
	if outcomeID == "" {
		return nil
	}
	for _, outcome := range evidenceMapsFromAny(evidenceMapFromAny(runtimeState["operation_plan"])["outcomes"]) {
		if strings.EqualFold(strings.TrimSpace(evidenceStringFromAny(outcome["id"])), outcomeID) {
			return outcome
		}
	}
	return nil
}

func operationPlanSkillCallTarget(arguments map[string]interface{}) map[string]interface{} {
	target := map[string]interface{}{}
	for _, key := range []string{
		"agent_id", "file_id", "asset_id", "resource_id", "dataset_id",
		"data_source_id", "table_id", "workflow_id", "binding_id", "href", "route",
	} {
		if value := operationPlanSkillCallTargetValue(arguments, key); value != "" {
			target[key] = value
		}
	}
	if len(target) == 0 {
		return nil
	}
	return target
}

func normalizeOperationPlanTargetValue(key string, value string) string {
	value = strings.TrimSpace(value)
	if key == "href" || key == "route" {
		if value != "/" {
			value = strings.TrimRight(value, "/")
		}
	}
	return value
}

func operationPlanPhaseMismatchStep(callID string, skillID string, toolName string, arguments map[string]interface{}, err error) skillStepResult {
	trace := plannerFeedbackTrace(skillID, toolName, err)
	trace.Status = "blocked"
	trace.Arguments = map[string]interface{}{
		"code":         operationPlanPhaseMismatchCode,
		"call_id":      strings.TrimSpace(callID),
		"skill_id":     strings.TrimSpace(skillID),
		"tool_name":    strings.TrimSpace(toolName),
		"next_step":    "retry_with_matching_plan_phase",
		"tool_summary": summarizeSkillToolArguments(skillID, toolName, arguments),
	}
	payload := map[string]interface{}{
		"status":      "blocked",
		"code":        operationPlanPhaseMismatchCode,
		"error":       err.Error(),
		"recoverable": true,
		"next_action": "Use the current operation_plan phase whose expected_action exactly matches this skill, tool, and target. Pass its id as plan_phase_id; update the plan first if no unique phase matches.",
	}
	return recoverableSkillStep(trace, skills.ToolResultMessage(callID, payload), false, false)
}
