package skillloop

import (
	"strings"
)

const operationPlanPhaseMismatchCode = "operation_plan_phase_mismatch"
const operationPlanPhaseSourceKey = "phase_source"
const operationPlanPhaseSourceServerOutcome = "server_outcome"
const operationPlanPhaseSourceModelAdvisory = "model_advisory"

func operationPlanPhaseOpenForToolCall(phase map[string]interface{}) bool {
	switch strings.ToLower(strings.TrimSpace(evidenceStringFromAny(phase["status"]))) {
	case "pending", "in_progress", "running":
		return true
	default:
		return false
	}
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
