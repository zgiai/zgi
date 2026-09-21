package connectortools

import (
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"github.com/zgiai/zgi/api/internal/modules/connectors/dispatch"
	"github.com/zgiai/zgi/api/internal/modules/connectors/execution"
	"github.com/zgiai/zgi/api/internal/modules/tools"
	"strings"
	"unicode"
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
	Title         string          `json:"title,omitempty"`
	Summary       string          `json:"summary,omitempty"`
}

func MessagesForInvocation(result execution.ExecutionResult, identity dispatch.Identity, causes ...error) []tools.ToolInvokeMessage {
	if result.State == execution.ExecutionStateCompleted && result.Outcome.ID == uuid.Nil {
		display := result.Display
		result = execution.UnresolvedResult(core.ErrorCodeOutcomeUnknown)
		result.Display = display
	}
	messages := Messages(result)
	for i := range messages {
		metadata, ok := ResultFromMessage(messages[i])
		if !ok {
			// Dispatch was attempted, but did not return a valid terminal or
			// confirmation shape. Preserve its action identity as unknown so
			// the host cannot send it through generic argument-error retries.
			code := core.ErrorCodeOutcomeUnknown
			if len(causes) > 0 && causes[0] != nil {
				code = core.ErrorCodeOf(causes[0])
			}
			unknown := Messages(execution.UnresolvedResult(code))[0]
			metadata, _ = ResultFromMessage(unknown)
			messages[i].Data = unknown.Data
		}
		metadata.Operation = &OperationRef{ConnectorID: identity.ConnectorID, OperationID: identity.OperationID, BindingID: identity.BindingID, ConnectionID: identity.ConnectionID, Target: identity.Target, LogicalOperationID: identity.LogicalOperationID}
		metadata.Operation.ActionStepID, metadata.Operation.PlanPhaseID = identity.ActionStepID, identity.PlanPhaseID
		metadata.Presentation = &OperationPresentation{SchemaVersion: 1, SourceKind: "native", InvocationID: identity.LogicalOperationID, ConnectorID: identity.ConnectorID, OperationID: identity.OperationID, DisplayStatus: presentationStatus(metadata)}
		if result.Display != nil {
			name := result.Display.Name
			if localized := result.Display.NameI18n[result.Display.Locale]; strings.TrimSpace(localized) != "" {
				name = localized
			}
			metadata.Presentation.Title, metadata.Presentation.Truncated = boundedPresentationText(name, 120)
			var summaryTruncated bool
			metadata.Presentation.Summary, summaryTruncated = boundedPresentationText(result.Display.Summary, 800)
			metadata.Presentation.Truncated = metadata.Presentation.Truncated || summaryTruncated
		}
		messages[i].Meta[ResultKey] = metadata
	}
	return messages
}

func presentationStatus(result ResultMetadata) string {
	switch result.State {
	case execution.ExecutionStateRejected:
		return "unavailable"
	case execution.ExecutionStateUnresolved:
		return "unknown"
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

func boundedPresentationText(value string, limit int) (string, bool) {
	value = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || unicode.In(r, unicode.Cf) {
			return -1
		}
		return r
	}, value)
	value = strings.TrimSpace(value)
	units := 0
	for offset, r := range value {
		units++
		if r > 0xffff {
			units++
		}
		if units > limit {
			return value[:offset], true
		}
	}
	return value, false
}
