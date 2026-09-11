package connectortools

import (
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"github.com/zgiai/zgi/api/internal/modules/connectors/execution"
	"github.com/zgiai/zgi/api/internal/modules/tools"
)

const ResultKey = "connector_execution"

// ResultMetadata is a host presentation value authored after native dispatch.
// It never carries operation input or credentials, and JSON cannot manufacture
// this in-process type to control a confirmation.
type ResultMetadata struct {
	ExecutionEvidence *ExecutionEvidence             `json:"execution_evidence,omitempty"`
	Operation         *OperationRef                  `json:"operation,omitempty"`
	Presentation      *OperationPresentation         `json:"presentation,omitempty"`
	State             execution.ExecutionState       `json:"state"`
	Confirmation      *execution.RuntimeConfirmation `json:"confirmation,omitempty"`
	DecisionID        uuid.UUID                      `json:"decision_id,omitempty,omitzero"`
	OutcomeID         uuid.UUID                      `json:"outcome_id,omitempty,omitzero"`
	OutcomeStatus     core.OutcomeStatus             `json:"outcome_status,omitempty"`
	DiagnosticCode    string                         `json:"diagnostic_code,omitempty"`
}

// ExecutionEvidence belongs only to invocation display, never receipt payloads.
type ExecutionEvidence struct {
	Version     int                           `json:"version"`
	Disposition execution.DispatchDisposition `json:"disposition"`
}

func (e *ExecutionEvidence) ValidFor(state execution.ExecutionState) bool {
	return e != nil && e.Version == 1 && ((state == execution.ExecutionStateRejected && e.Disposition == execution.DispatchNotSubmitted) ||
		(state == execution.ExecutionStateUnresolved && (e.Disposition == execution.DispatchInProgress || e.Disposition == execution.DispatchUnknown)))
}

func Messages(result execution.ExecutionResult) []tools.ToolInvokeMessage {
	metadata := ResultMetadata{State: result.State, Confirmation: result.Confirmation, DecisionID: result.DecisionID, OutcomeID: result.Outcome.ID, OutcomeStatus: result.Outcome.Status, DiagnosticCode: result.Outcome.DiagnosticCode}
	if result.Evidence.ValidFor(result.State) {
		metadata.ExecutionEvidence = &ExecutionEvidence{Version: 1, Disposition: result.Evidence.Disposition}
		metadata.DiagnosticCode = string(result.Evidence.DiagnosticCode)
	}
	data := map[string]any{"state": string(result.State)}
	if metadata.ExecutionEvidence != nil {
		data["execution_evidence"] = metadata.ExecutionEvidence
		data["diagnostic_code"] = metadata.DiagnosticCode
	}
	if result.State == execution.ExecutionStateCompleted {
		if value, ok := result.Value.(map[string]any); ok {
			data = value
		} else {
			data = map[string]any{"value": result.Value}
		}
	} else if result.State == execution.ExecutionStateBlocked {
		data["decision_id"] = result.DecisionID.String()
	} else if result.Confirmation != nil {
		data["confirmation"] = result.Confirmation
	}
	return []tools.ToolInvokeMessage{{Type: tools.ToolInvokeMessageTypeJSON, Data: data, Meta: map[string]any{ResultKey: metadata}}}
}

func ResultFromMessage(message tools.ToolInvokeMessage) (ResultMetadata, bool) {
	if message.Type != tools.ToolInvokeMessageTypeJSON {
		return ResultMetadata{}, false
	}
	result, ok := message.Meta[ResultKey].(ResultMetadata)
	if !ok {
		return ResultMetadata{}, false
	}
	switch result.State {
	case execution.ExecutionStateRejected, execution.ExecutionStateUnresolved:
		ok = result.Confirmation == nil && result.OutcomeID == uuid.Nil && result.ExecutionEvidence.ValidFor(result.State)
	case execution.ExecutionStateRequiresConfirmation:
		ok = result.Confirmation != nil && result.OutcomeID == uuid.Nil && result.DecisionID == uuid.Nil
	case execution.ExecutionStateBlocked:
		ok = result.Confirmation == nil && result.DecisionID != uuid.Nil && result.OutcomeID == uuid.Nil
	case execution.ExecutionStateCancelled:
		ok = result.Confirmation == nil && result.OutcomeID == uuid.Nil
	case execution.ExecutionStateCompleted:
		ok = result.Confirmation == nil
	default:
		ok = false
	}
	return result, ok
}
