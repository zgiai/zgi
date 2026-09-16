package skillloop

import (
	"strings"

	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/modules/skills"
)

// updatePlanToolsForRuntimeState keeps phase_updates out of a request that has
// no phase ledger to patch. Tool schemas can be shared by a runtime/session, so
// clone only the update_plan schema before narrowing it for this model round.
func updatePlanToolsForRuntimeState(input []adapter.Tool, runtimeState map[string]interface{}) []adapter.Tool {
	if len(evidenceMapsFromAny(evidenceMapFromAny(runtimeState["operation_plan"])["phases"])) > 0 {
		return input
	}
	out := append([]adapter.Tool(nil), input...)
	for index := range out {
		if !strings.EqualFold(strings.TrimSpace(out[index].Function.Name), skills.MetaToolUpdatePlan) {
			continue
		}
		parameters := cloneSchemaMap(evidenceMapFromAny(out[index].Function.Parameters))
		properties := evidenceMapFromAny(parameters["properties"])
		delete(properties, "phase_updates")
		parameters["properties"] = properties

		branches := evidenceSliceFromAny(parameters["oneOf"])
		filtered := make([]interface{}, 0, len(branches))
		for _, raw := range branches {
			branch := evidenceMapFromAny(raw)
			containsPhaseUpdates := false
			for _, required := range evidenceStringSliceFromAny(branch["required"]) {
				if strings.EqualFold(strings.TrimSpace(required), "phase_updates") {
					containsPhaseUpdates = true
					break
				}
			}
			if !containsPhaseUpdates {
				filtered = append(filtered, branch)
			}
		}
		parameters["oneOf"] = filtered
		out[index].Function.Parameters = parameters
	}
	return out
}

func cloneSchemaMap(input map[string]interface{}) map[string]interface{} {
	if input == nil {
		return nil
	}
	out := make(map[string]interface{}, len(input))
	for key, value := range input {
		out[key] = cloneSchemaValue(value)
	}
	return out
}

func cloneSchemaValue(value interface{}) interface{} {
	switch typed := value.(type) {
	case map[string]interface{}:
		return cloneSchemaMap(typed)
	case []interface{}:
		out := make([]interface{}, len(typed))
		for index := range typed {
			out[index] = cloneSchemaValue(typed[index])
		}
		return out
	case []string:
		return append([]string(nil), typed...)
	default:
		return typed
	}
}
