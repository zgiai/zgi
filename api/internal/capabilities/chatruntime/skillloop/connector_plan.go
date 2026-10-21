package skillloop

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/connectortools"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/modules/skills"
)

func nativeCanonicalizeConnectorPlanCall(call adapter.ToolCall, toolSet *skills.NativeToolSet) adapter.ToolCall {
	if toolSet == nil || strings.TrimSpace(call.Function.Name) != skills.MetaToolUpdatePlan {
		return call
	}
	var args map[string]any
	if json.Unmarshal([]byte(call.Function.Arguments), &args) != nil {
		return call
	}
	for _, key := range []string{"plan", "phase_updates"} {
		phases, _ := args[key].([]any)
		for _, raw := range phases {
			phase, _ := raw.(map[string]any)
			expected, _ := phase["expected_action"].(map[string]any)
			if expected == nil {
				continue
			}
			delete(expected, "connector_operation")
			binding, _, exists := nativeToolBindingForCall(toolSet, strings.TrimSpace(evidenceStringFromAny(expected["tool_name"])))
			if !exists {
				// Persisted plans contain canonical names. Resolve them against
				// the current exposed set after aliases have changed.
				for _, candidate := range toolSet.ToolBindings {
					if candidate.SkillID != evidenceStringFromAny(expected["skill_id"]) || candidate.ToolName != evidenceStringFromAny(expected["tool_name"]) || candidate.ConnectorOperation == nil {
						continue
					}
					if exists && *candidate.ConnectorOperation != *binding.ConnectorOperation {
						exists = false
						break
					}
					binding, exists = candidate, true
				}
			}
			if !exists || binding.ConnectorOperation == nil {
				continue
			}
			operation := binding.ConnectorOperation
			if !core.Identifier(operation.ConnectorID).Valid() || !core.Identifier(operation.OperationID).Valid() {
				continue
			}
			expected["skill_id"], expected["tool_name"] = binding.SkillID, binding.ToolName
			expected["connector_operation"] = map[string]any{"connector_id": operation.ConnectorID, "operation_id": operation.OperationID}
		}
	}
	encoded, err := json.Marshal(args)
	if err == nil {
		call.Function.Arguments = string(encoded)
	}
	return call
}

func connectorPhaseOperation(phase map[string]any) map[string]any {
	return evidenceMapFromAny(evidenceMapFromAny(phase["expected_action"])["connector_operation"])
}

func connectorPhaseEvidence(phase map[string]any, evidence map[string]any) (connectortools.InvocationEvidence, bool) {
	op := connectorPhaseOperation(phase)
	var found connectortools.InvocationEvidence
	matched := false
	for _, invocation := range terminalStateGuardInvocations(evidence) {
		current, native := connectortools.Evidence(invocation)
		if !native || !current.Operation.Valid() || current.Operation.PlanPhaseID != evidenceStringFromAny(phase["id"]) || string(current.Operation.ConnectorID) != evidenceStringFromAny(op["connector_id"]) || string(current.Operation.OperationID) != evidenceStringFromAny(op["operation_id"]) {
			continue
		}
		if stepID, err := uuid.Parse(current.Operation.ActionStepID); err != nil || stepID == uuid.Nil || stepID.String() != current.Operation.ActionStepID {
			continue
		}
		if matched && (current.Key() != found.Key() || (found.Status != "pending" && found.Status != current.Status)) {
			found.Status = "unknown"
			return found, true
		}
		found, matched = current, true
	}
	return found, matched
}

func validateConnectorPlanSnapshot(phases []map[string]any, evidence map[string]any) error {
	baseline := evidenceMapsFromAny(evidenceMapFromAny(evidence["operation_plan"])["phases"])
	shape := func(items []map[string]any) string {
		pairs := [][2]string{}
		for _, phase := range items {
			op := connectorPhaseOperation(phase)
			if len(op) > 0 {
				pairs = append(pairs, [2]string{evidenceStringFromAny(op["connector_id"]), evidenceStringFromAny(op["operation_id"])})
			}
		}
		encoded, _ := json.Marshal(pairs)
		return string(encoded)
	}
	for _, invocation := range terminalStateGuardInvocations(evidence) {
		if _, native := connectortools.Evidence(invocation); native && shape(phases) != shape(baseline) {
			return fmt.Errorf("%w: an executed Connector plan cannot add, remove, or reorder operations", ErrInvalidInput)
		}
	}
	for _, phase := range phases {
		if len(connectorPhaseOperation(phase)) == 0 {
			continue
		}
		phase["completion_mode"], phase["required"] = "tool", true
		status := evidenceStringFromAny(phase["status"])
		if status == "completed" || status == "skipped" {
			proof, matched := connectorPhaseEvidence(phase, evidence)
			if !matched || proof.Status != "successful" {
				return fmt.Errorf("%w: Connector phase completion requires its successful native outcome", ErrInvalidInput)
			}
			phase["status"] = "completed"
		}
	}
	return nil
}
