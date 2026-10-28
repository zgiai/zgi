package connectortools

import (
	"encoding/json"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/execution"
)

// InvocationEvidence reads only the host-authored Connector trace. Provider
// output cannot opt an ordinary tool into this protocol.
type InvocationEvidence struct {
	Operation OperationRef
	OutcomeID uuid.UUID
	Status    string
	Title     string
	Summary   string
	// ErrorCode is the safe Connector error code of a terminal that did not
	// succeed, from the host presentation or the diagnostic code.
	ErrorCode string
}

func Evidence(invocation map[string]any) (InvocationEvidence, bool) {
	var evidence InvocationEvidence
	if invocation["runtime_source"] != "connector" || invocation["kind"] != "tool_call" {
		return evidence, false
	}
	evidence.Status = "unknown"
	var result struct {
		Operation         OperationRef           `json:"operation"`
		State             string                 `json:"state"`
		OutcomeID         uuid.UUID              `json:"outcome_id"`
		OutcomeStatus     string                 `json:"outcome_status"`
		DiagnosticCode    string                 `json:"diagnostic_code"`
		ExecutionEvidence *ExecutionEvidence     `json:"execution_evidence"`
		Presentation      *OperationPresentation `json:"connector_presentation"`
		Confirmation      json.RawMessage        `json:"confirmation"`
	}
	encoded, err := json.Marshal(invocation["result"])
	if err != nil || json.Unmarshal(encoded, &result) != nil || !result.Operation.Valid() {
		return evidence, true
	}
	evidence.Operation, evidence.OutcomeID = result.Operation, result.OutcomeID
	if p := result.Presentation; p != nil && p.SchemaVersion == 1 && p.SourceKind == "native" && p.InvocationID == result.Operation.LogicalOperationID && p.ConnectorID == result.Operation.ConnectorID && p.OperationID == result.Operation.OperationID {
		evidence.Title, _ = boundedPresentationText(p.Title, 120)
		evidence.Summary, _ = boundedPresentationText(p.Summary, 800)
		evidence.ErrorCode, _ = boundedPresentationText(p.ErrorCode, 128)
	}
	if evidence.ErrorCode == "" {
		evidence.ErrorCode, _ = boundedPresentationText(result.DiagnosticCode, 128)
	}
	switch result.State {
	case "rejected", "unresolved":
		if result.OutcomeID == uuid.Nil && result.OutcomeStatus == "" &&
			(len(result.Confirmation) == 0 || string(result.Confirmation) == "null") && result.ExecutionEvidence.ValidFor(execution.ExecutionState(result.State)) {
			if result.ExecutionEvidence.Disposition == execution.DispatchNotSubmitted {
				evidence.Status = "rejected"
			}
			if result.ExecutionEvidence.Disposition == execution.DispatchInProgress {
				evidence.Status = "in_progress"
			}
		}
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
