package skillloop

import (
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/connectortools"
)

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
