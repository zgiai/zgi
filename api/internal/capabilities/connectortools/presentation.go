package connectortools

import (
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"github.com/zgiai/zgi/api/internal/modules/connectors/dispatch"
	"github.com/zgiai/zgi/api/internal/modules/connectors/execution"
	"github.com/zgiai/zgi/api/internal/modules/tools"
	"strings"
)

// OperationRef is resolved by the host. A presentation never creates authority
// and is not accepted as dispatcher or confirmation input.
type OperationRef struct {
	ConnectorID        core.Identifier `json:"connector_id"`
	OperationID        core.Identifier `json:"operation_id"`
	BindingID          uuid.UUID       `json:"binding_id"`
	ConnectionID       uuid.UUID       `json:"connection_id"`
	Target             core.TargetRef  `json:"target"`
	LogicalOperationID string          `json:"logical_operation_id"`
	ActionStepID       string          `json:"action_step_id,omitempty"`
	PlanPhaseID        string          `json:"plan_phase_id,omitempty"`
}

func (ref OperationRef) Valid() bool {
	return ref.ConnectorID.Valid() && ref.OperationID.Valid() && ref.BindingID != uuid.Nil && ref.ConnectionID != uuid.Nil && ref.Target.Validate() == nil && strings.TrimSpace(ref.LogicalOperationID) != ""
}

// OperationPresentation is a versioned, read-only projection of server facts.
// Summaries deliberately exclude provider content, arguments and receipts.
type OperationPresentation struct {
	SchemaVersion int             `json:"schema_version"`
	SourceKind    string          `json:"source_kind"`
	InvocationID  string          `json:"invocation_id"`
	ConnectorID   core.Identifier `json:"connector_id,omitempty"`
	OperationID   core.Identifier `json:"operation_id,omitempty"`
	DisplayStatus string          `json:"display_status"`
	Redacted      bool            `json:"redacted"`
	Truncated     bool            `json:"truncated"`
}

func MessagesForInvocation(result execution.ExecutionResult, identity dispatch.Identity) []tools.ToolInvokeMessage {
	messages := Messages(result)
	for i := range messages {
		metadata, ok := ResultFromMessage(messages[i])
		if !ok {
			// Dispatch was attempted, but did not return a valid terminal or
			// confirmation shape. Preserve its action identity as unknown so
			// the host cannot send it through generic argument-error retries.
			metadata = ResultMetadata{State: execution.ExecutionStateCompleted, OutcomeStatus: core.OutcomeStatusUnknown}
			messages[i].Data = map[string]any{"state": string(metadata.State)}
		}
		metadata.Operation = &OperationRef{ConnectorID: identity.ConnectorID, OperationID: identity.OperationID, BindingID: identity.BindingID, ConnectionID: identity.ConnectionID, Target: identity.Target, LogicalOperationID: identity.LogicalOperationID}
		metadata.Operation.ActionStepID, metadata.Operation.PlanPhaseID = identity.ActionStepID, identity.PlanPhaseID
		metadata.Presentation = &OperationPresentation{SchemaVersion: 1, SourceKind: "native", InvocationID: identity.LogicalOperationID, ConnectorID: identity.ConnectorID, OperationID: identity.OperationID, DisplayStatus: presentationStatus(metadata), Redacted: true}
		messages[i].Meta[ResultKey] = metadata
	}
	return messages
}

func presentationStatus(result ResultMetadata) string {
	switch result.State {
	case execution.ExecutionStateRequiresConfirmation:
		return "pending"
	case execution.ExecutionStateCancelled:
		return "cancelled"
	case execution.ExecutionStateBlocked:
		return "unavailable"
	case execution.ExecutionStateCompleted:
		if result.OutcomeID != uuid.Nil && result.OutcomeStatus.Valid() {
			return string(result.OutcomeStatus)
		}
	}
	return "unknown"
}
