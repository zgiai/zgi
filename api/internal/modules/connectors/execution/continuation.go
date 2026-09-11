package execution

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/catalog"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
)

const maxContinuationPayloadEnvelopeBytes = 1 << 20

// ConfirmationOrigin identifies the durable host state that must be resumed
// after a Connector confirmation. These references are non-secret and are
// stored separately from the protected provider input so runtimes can find
// their own paused work without opening the payload.
type ConfirmationOrigin struct {
	Surface        core.TargetType `json:"surface"`
	ConversationID string          `json:"conversation_id,omitempty"`
	MessageID      string          `json:"message_id,omitempty"`
	AgentID        string          `json:"agent_id,omitempty"`
	WorkflowID     string          `json:"workflow_id,omitempty"`
	RunID          string          `json:"run_id,omitempty"`
	NodeID         string          `json:"node_id,omitempty"`
}

// FrozenConfirmationRequest is the server-authoritative operation snapshot.
// The request includes Input in memory for the payload protector, while its
// public JSON representation deliberately omits Input. A continuation client
// submits only RuntimeConfirmationCommand, never this snapshot.
type FrozenConfirmationRequest struct {
	Preview           *ConfirmationPreview    `json:"-"`
	RouteReceipt      RouteClaimReceipt       `json:"route_receipt"`
	Request           ExecutionRequest        `json:"request"`
	Origin            ConfirmationOrigin      `json:"origin"`
	AcceptedOperation catalog.PinnedOperation `json:"-"`
}

// MarshalJSON keeps the non-secret execution identity observable while making
// Input impossible to expose through an accidental public serialization.
func (request FrozenConfirmationRequest) MarshalJSON() ([]byte, error) {
	type safeExecutionRequest struct {
		OrganizationID     uuid.UUID       `json:"organization_id"`
		OwnerAccountID     uuid.UUID       `json:"owner_account_id"`
		ActingPrincipalID  uuid.UUID       `json:"acting_principal_id"`
		BindingID          uuid.UUID       `json:"binding_id"`
		ConnectionID       uuid.UUID       `json:"connection_id"`
		ConnectorID        core.Identifier `json:"connector_id"`
		Target             core.TargetRef  `json:"target"`
		OperationID        core.Identifier `json:"operation_id"`
		LogicalOperationID string          `json:"logical_operation_id"`
		CorrelationID      string          `json:"correlation_id"`
		Interactive        bool            `json:"interactive"`
	}
	type safeFrozenRequest struct {
		RouteReceipt RouteClaimReceipt    `json:"route_receipt"`
		Request      safeExecutionRequest `json:"request"`
		Origin       ConfirmationOrigin   `json:"origin"`
	}
	return json.Marshal(safeFrozenRequest{
		RouteReceipt: request.RouteReceipt,
		Request: safeExecutionRequest{
			OrganizationID: request.Request.OrganizationID, OwnerAccountID: request.Request.OwnerAccountID,
			ActingPrincipalID: request.Request.ActingPrincipalID, BindingID: request.Request.BindingID,
			ConnectionID: request.Request.ConnectionID, ConnectorID: request.Request.ConnectorID,
			Target: request.Request.Target, OperationID: request.Request.OperationID,
			LogicalOperationID: request.Request.LogicalOperationID, CorrelationID: request.Request.CorrelationID,
			Interactive: request.Request.Interactive,
		},
		Origin: request.Origin,
	})
}

// ContinuationPayloadEnvelope is an opaque, authenticated ciphertext produced
// outside this package. Persistence never parses or constructs it.
type ContinuationPayloadEnvelope string

// ContinuationPayloadProtector seals and opens the complete frozen request.
// Implementations must bind authenticated encryption to both organizationID
// and receipt so an envelope cannot be replayed across tenants or operations.
// FrozenConfirmationRequest.MarshalJSON intentionally omits Input, so a
// protector must explicitly encode Request.Input inside its encrypted
// plaintext and restore it on Open; marshaling the public value is not enough.
type ContinuationPayloadProtector interface {
	Seal(context.Context, uuid.UUID, RouteClaimReceipt, FrozenConfirmationRequest) (ContinuationPayloadEnvelope, error)
	Open(context.Context, uuid.UUID, RouteClaimReceipt, ContinuationPayloadEnvelope) (FrozenConfirmationRequest, error)
}

type ConfirmationContinuationState string

const (
	ConfirmationContinuationPending   ConfirmationContinuationState = "pending"
	ConfirmationContinuationCancelled ConfirmationContinuationState = "cancelled"
	ConfirmationContinuationCompleted ConfirmationContinuationState = "completed"
	ConfirmationContinuationFailed    ConfirmationContinuationState = "failed"
	ConfirmationContinuationUnknown   ConfirmationContinuationState = "unknown"
)

func (state ConfirmationContinuationState) valid() bool {
	switch state {
	case ConfirmationContinuationPending, ConfirmationContinuationCancelled,
		ConfirmationContinuationCompleted, ConfirmationContinuationFailed,
		ConfirmationContinuationUnknown:
		return true
	default:
		return false
	}
}

func (state ConfirmationContinuationState) terminal() bool {
	return state.valid() && state != ConfirmationContinuationPending
}

// ConfirmationTerminal is the safe terminal projection stored after the
// pending continuation has been consumed. It contains no provider value,
// request input, credential, or unstructured error text.
type ConfirmationTerminal struct {
	State          ConfirmationContinuationState `json:"state"`
	DecisionID     uuid.UUID                     `json:"decision_id,omitempty,omitzero"`
	OutcomeID      uuid.UUID                     `json:"outcome_id,omitempty,omitzero"`
	DiagnosticCode string                        `json:"diagnostic_code,omitempty"`
}

// ConfirmationContinuation is returned only after its envelope has been
// opened and checked against the immutable row identity. Frozen.Request.Input
// remains available to trusted runtime code but is excluded from JSON.
type ConfirmationContinuation struct {
	Frozen         FrozenConfirmationRequest     `json:"frozen"`
	State          ConfirmationContinuationState `json:"state"`
	DecisionID     uuid.UUID                     `json:"decision_id,omitempty,omitzero"`
	OutcomeID      uuid.UUID                     `json:"outcome_id,omitempty,omitzero"`
	DiagnosticCode string                        `json:"diagnostic_code,omitempty"`
	CreatedAt      time.Time                     `json:"created_at"`
	UpdatedAt      time.Time                     `json:"updated_at"`
	TerminalAt     time.Time                     `json:"terminal_at,omitempty,omitzero"`
}

// ConfirmationContinuationStore is transport-neutral. HTTP, SSE, agent, and
// workflow runtimes share this same freeze/load/CAS boundary.
type ConfirmationContinuationStore interface {
	Freeze(context.Context, FrozenConfirmationRequest) (ConfirmationContinuation, error)
	Load(context.Context, uuid.UUID, uuid.UUID, RouteClaimReceipt) (ConfirmationContinuation, error)
	RecordTerminal(context.Context, uuid.UUID, uuid.UUID, RouteClaimReceipt, ConfirmationTerminal) (ConfirmationContinuation, error)
}

func validateFrozenConfirmationRequest(frozen FrozenConfirmationRequest) error {
	request := frozen.Request
	if !validUUIDReceipt(frozen.RouteReceipt) || request.Confirmation != nil || !request.Interactive ||
		request.ActingPrincipalID != request.OwnerAccountID || validateExecutionRequest(request) != nil ||
		!validAcceptedOperationSnapshot(
			frozen.AcceptedOperation, request.ConnectorID, request.OperationID,
		) ||
		validateConfirmationOrigin(frozen.Origin, request.Target) != nil {
		return connectorError(core.ErrorCodeInvalidInput, false)
	}
	// Payload protectors commonly use JSON internally. Reject a value that
	// cannot be represented before any row is claimed, without retaining the
	// plaintext bytes or including the marshaling error in a public error.
	if _, err := json.Marshal(request.Input); err != nil {
		return connectorError(core.ErrorCodeInvalidInput, false)
	}
	return nil
}

func validateConfirmationOrigin(origin ConfirmationOrigin, target core.TargetRef) error {
	for _, value := range []string{
		origin.ConversationID, origin.MessageID, origin.AgentID,
		origin.WorkflowID, origin.RunID, origin.NodeID,
	} {
		if value != "" && !validOpaqueText(value, 255) {
			return connectorError(core.ErrorCodeInvalidInput, false)
		}
	}
	switch origin.Surface {
	case core.TargetTypeChat:
		if origin.ConversationID == "" || origin.MessageID == "" ||
			origin.AgentID != "" || origin.WorkflowID != "" || origin.RunID != "" || origin.NodeID != "" ||
			origin.ConversationID != target.ID || target.Type != core.TargetTypeChat {
			return connectorError(core.ErrorCodeInvalidInput, false)
		}
	case core.TargetTypeAgent:
		if origin.ConversationID == "" || origin.MessageID == "" || origin.AgentID == "" ||
			origin.WorkflowID != "" || origin.NodeID != "" || origin.AgentID != target.ID || target.Type != core.TargetTypeAgent {
			return connectorError(core.ErrorCodeInvalidInput, false)
		}
	case core.TargetTypeWorkflow:
		if origin.WorkflowID == "" || origin.RunID == "" || origin.NodeID == "" ||
			origin.ConversationID != "" || origin.MessageID != "" || origin.AgentID != "" ||
			origin.WorkflowID != target.ID || origin.NodeID != target.BindingInstanceID || target.Type != core.TargetTypeWorkflow {
			return connectorError(core.ErrorCodeInvalidInput, false)
		}
	default:
		return connectorError(core.ErrorCodeInvalidInput, false)
	}
	return nil
}

func validateConfirmationTerminal(terminal ConfirmationTerminal) error {
	diagnostic := strings.TrimSpace(terminal.DiagnosticCode)
	if diagnostic != terminal.DiagnosticCode || !terminal.State.terminal() {
		return connectorError(core.ErrorCodeInvalidInput, false)
	}
	switch terminal.State {
	case ConfirmationContinuationCancelled:
		if terminal.DecisionID == uuid.Nil || terminal.OutcomeID != uuid.Nil || diagnostic != "" {
			return connectorError(core.ErrorCodeInvalidInput, false)
		}
	case ConfirmationContinuationCompleted:
		if terminal.DecisionID == uuid.Nil || terminal.OutcomeID == uuid.Nil ||
			(diagnostic != "" && !diagnosticCodePattern.MatchString(diagnostic)) {
			return connectorError(core.ErrorCodeInvalidInput, false)
		}
	case ConfirmationContinuationFailed, ConfirmationContinuationUnknown:
		if !diagnosticCodePattern.MatchString(diagnostic) ||
			(terminal.OutcomeID != uuid.Nil && terminal.DecisionID == uuid.Nil) {
			return connectorError(core.ErrorCodeInvalidInput, false)
		}
	default:
		return connectorError(core.ErrorCodeInvalidInput, false)
	}
	return nil
}

func validUUIDReceipt(receipt RouteClaimReceipt) bool {
	if !validReceipt(receipt) {
		return false
	}
	parsed, err := uuid.Parse(string(receipt))
	return err == nil && parsed != uuid.Nil
}

func validContinuationEnvelope(envelope ContinuationPayloadEnvelope) bool {
	value := string(envelope)
	return value != "" && value == strings.TrimSpace(value) && len(value) <= maxContinuationPayloadEnvelopeBytes &&
		!strings.ContainsAny(value, "\x00\r\n")
}
