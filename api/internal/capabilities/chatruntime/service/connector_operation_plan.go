package service

import (
	"strings"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/connectortools"
)

func connectorPlanInvocationStatus(invocation map[string]any) (string, bool) {
	evidence, native := connectortools.Evidence(invocation)
	if !native {
		return "", false
	}
	switch evidence.Status {
	case "successful":
		return operationPlanStepStatusCompleted, true
	case "pending":
		return operationPlanStepStatusPending, true
	default:
		return operationPlanStepStatusFailed, true
	}
}

func connectorPlanInvocationPhase(plan, invocation map[string]any) (int, connectortools.InvocationEvidence) {
	evidence, native := connectortools.Evidence(invocation)
	stepID, err := uuid.Parse(evidence.Operation.ActionStepID)
	if !native || !evidence.Operation.Valid() || err != nil || stepID == uuid.Nil || evidence.Operation.PlanPhaseID == "" {
		return -1, evidence
	}
	match := -1
	for index, phase := range mapSliceFromAny(plan["phases"]) {
		expected := mapFromOperationContext(mapFromOperationContext(phase["expected_action"])["connector_operation"])
		if phase["id"] != evidence.Operation.PlanPhaseID || expected["connector_id"] != string(evidence.Operation.ConnectorID) || expected["operation_id"] != string(evidence.Operation.OperationID) {
			continue
		}
		if match >= 0 {
			return -1, evidence
		}
		match = index
	}
	return match, evidence
}

func connectorPlanEffects(plan, invocation map[string]any) []map[string]any {
	index, evidence := connectorPlanInvocationPhase(plan, invocation)
	if index < 0 || evidence.Status != "successful" {
		return nil
	}
	phase := mapSliceFromAny(plan["phases"])[index]
	phaseID := evidence.Operation.PlanPhaseID
	outcomeID := strings.TrimSpace(stringFromAny(phase["outcome_id"]))
	// Bind every native phase of this outcome, including not-yet-executed
	// phases. One successful operation cannot satisfy a multi-step outcome.
	for _, outcome := range mapSliceFromAny(plan[operationPlanOutcomesKey]) {
		if outcomeID == "" || outcome["id"] != outcomeID {
			continue
		}
		mode := stringFromAny(outcome["verification_mode"])
		if mode != "" && mode != "runtime_effects" && mode != "runtime_unverified" {
			continue
		}
		acceptance := copyStringAnyMap(mapFromOperationContext(outcome["acceptance"]))
		if acceptance == nil {
			acceptance = map[string]any{}
		}
		specs := []map[string]any{}
		for _, spec := range mapSliceFromAny(acceptance["effects"]) {
			if spec["type"] != "connector.operation.completed" {
				specs = append(specs, spec)
			}
		}
		for _, candidate := range mapSliceFromAny(plan["phases"]) {
			if candidate["outcome_id"] == outcomeID && len(mapFromOperationContext(mapFromOperationContext(candidate["expected_action"])["connector_operation"])) > 0 {
				specs = append(specs, map[string]any{"type": "connector.operation.completed", "resource_type": "connector_operation", "resource_id": candidate["id"]})
			}
		}
		acceptance["mode"], acceptance["effects"] = "all", mapsToInterfaceSlice(specs)
		outcome["acceptance"], outcome["verification_mode"] = acceptance, "runtime_effects"
	}
	return []map[string]any{{"id": "connector-outcome:" + evidence.OutcomeID.String(), "type": "connector.operation.completed", "resource_type": "connector_operation", "resource_id": phaseID, "phase_id": phaseID, "source_action": operationPlanInvocationPlanID(invocation), "status": operationPlanStepStatusCompleted, "operation": evidence.Operation, "outcome_id": evidence.OutcomeID.String(), "created_at": operationPlanInvocationCreatedAt(invocation)}}
}
