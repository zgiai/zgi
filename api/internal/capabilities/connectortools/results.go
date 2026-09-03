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
	Operation      *OperationRef                  `json:"operation,omitempty"`
	Presentation   *OperationPresentation         `json:"presentation,omitempty"`
	State          execution.ExecutionState       `json:"state"`
	Confirmation   *execution.RuntimeConfirmation `json:"confirmation,omitempty"`
	DecisionID     uuid.UUID                      `json:"decision_id,omitempty,omitzero"`
	OutcomeID      uuid.UUID                      `json:"outcome_id,omitempty,omitzero"`
	OutcomeStatus  core.OutcomeStatus             `json:"outcome_status,omitempty"`
	DiagnosticCode string                         `json:"diagnostic_code,omitempty"`
}

func Messages(result execution.ExecutionResult) []tools.ToolInvokeMessage {
	metadata := ResultMetadata{State: result.State, Confirmation: result.Confirmation, DecisionID: result.DecisionID, OutcomeID: result.Outcome.ID, OutcomeStatus: result.Outcome.Status, DiagnosticCode: result.Outcome.DiagnosticCode}
	data := map[string]any{"state": string(result.State)}
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
