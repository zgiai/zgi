package connectortools

import (
	"encoding/json"

	"github.com/google/uuid"
)

// InvocationEvidence reads only the host-authored Connector trace. Provider
// output cannot opt an ordinary tool into this protocol.
type InvocationEvidence struct {
	Operation OperationRef
	OutcomeID uuid.UUID
	Status    string
}

func Evidence(invocation map[string]any) (InvocationEvidence, bool) {
	var evidence InvocationEvidence
	if invocation["runtime_source"] != "connector" || invocation["kind"] != "tool_call" {
		return evidence, false
	}
	evidence.Status = "unknown"
	var result struct {
		Operation     OperationRef `json:"operation"`
		State         string       `json:"state"`
		OutcomeID     uuid.UUID    `json:"outcome_id"`
		OutcomeStatus string       `json:"outcome_status"`
	}
	encoded, err := json.Marshal(invocation["result"])
	if err != nil || json.Unmarshal(encoded, &result) != nil || !result.Operation.Valid() {
		return evidence, true
	}
	evidence.Operation, evidence.OutcomeID = result.Operation, result.OutcomeID
	switch result.State {
	case "requires_confirmation":
		evidence.Status = "pending"
	case "cancelled":
		evidence.Status = "cancelled"
	case "blocked":
		evidence.Status = "unavailable"
	case "completed":
		if result.OutcomeID != uuid.Nil && (result.OutcomeStatus == "successful" || result.OutcomeStatus == "failed") {
			evidence.Status = result.OutcomeStatus
		}
	}
	return evidence, true
}

// Key excludes presentation labels; all execution coordinates remain pinned.
func (e InvocationEvidence) Key() string {
	ref := e.Operation
	ref.ActionStepID, ref.PlanPhaseID = "", ""
	ref.Target.Scope = ref.Target.EffectiveScope()
	encoded, _ := json.Marshal(ref)
	return string(encoded)
}
