package skillloop

import (
	"strings"

	"github.com/zgiai/zgi/api/internal/modules/skills"
)

const operationPlanPhaseMismatchCode = "operation_plan_phase_mismatch"

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
	return true
}

func operationPlanSkillCallTargetValue(arguments map[string]interface{}, key string) string {
	if value := strings.TrimSpace(evidenceStringFromAny(arguments[key])); value != "" {
		return value
	}
	for _, containerKey := range []string{"target", "system_prompt_source", "agent", "file", "resource"} {
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
