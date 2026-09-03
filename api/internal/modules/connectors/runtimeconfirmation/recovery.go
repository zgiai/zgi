package runtimeconfirmation

import (
	"context"
	"time"

	"github.com/google/uuid"
	connectoraudit "github.com/zgiai/zgi/api/internal/modules/connectors/audit"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"github.com/zgiai/zgi/api/internal/modules/connectors/execution"
)

// TerminalRecoveryRequest deliberately excludes provider input, credentials,
// and provider values. Only immutable identities may be used for recovery.
type TerminalRecoveryRequest struct {
	OrganizationID     uuid.UUID
	OwnerAccountID     uuid.UUID
	ActingPrincipalID  uuid.UUID
	ConnectionID       uuid.UUID
	ConnectorID        core.Identifier
	Target             core.TargetRef
	OperationID        core.Identifier
	LogicalOperationID string
	CorrelationID      string
	RouteReceipt       execution.RouteClaimReceipt
	Decision           execution.ConfirmationDecision
}

type decisionOutcomeReader interface {
	ListDecisions(
		context.Context,
		uuid.UUID,
		uuid.UUID,
		connectoraudit.DecisionListFilter,
	) ([]core.LifecycleDecision, error)
	GetOutcomeByDecision(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) (core.OperationOutcome, error)
	ConvergeOutcome(context.Context, core.OperationOutcome) (core.OperationOutcome, error)
}

// AuditTerminalRecoverer converges a pending continuation from the immutable
// confirmation/dispatch evidence written by Connector execution. It never
// decrypts credentials and never has a provider dependency.
type AuditTerminalRecoverer struct {
	reader decisionOutcomeReader
	now    func() time.Time
}

func NewAuditTerminalRecoverer(reader decisionOutcomeReader) (*AuditTerminalRecoverer, error) {
	if reader == nil {
		return nil, core.NewError(core.ErrorCodeInternal, false, nil)
	}
	return &AuditTerminalRecoverer{
		reader: reader,
		now:    func() time.Time { return time.Now().UTC() },
	}, nil
}

func (recoverer *AuditTerminalRecoverer) RecoverTerminal(
	ctx context.Context,
	request TerminalRecoveryRequest,
) (execution.ConfirmationTerminal, error) {
	if recoverer == nil || recoverer.reader == nil || recoverer.now == nil || !validTerminalRecoveryRequest(request) {
		return execution.ConfirmationTerminal{}, core.NewError(core.ErrorCodeInvalidInput, false, nil)
	}
	decisions, err := recoverer.reader.ListDecisions(
		ctx,
		request.OrganizationID,
		request.OwnerAccountID,
		connectoraudit.DecisionListFilter{
			ConnectorID: request.ConnectorID, ConnectionID: request.ConnectionID,
			CorrelationID: request.CorrelationID, Limit: 100,
		},
	)
	if err != nil {
		return execution.ConfirmationTerminal{}, err
	}
	confirmed := request.Decision == execution.ConfirmationDecisionConfirmed
	finalization, ok := uniqueRecoveryDecision(decisions, request, "confirmation_finalization", core.DecisionStatusSuccessful)
	if !ok || finalization.Details["confirmation"] != confirmed {
		return execution.ConfirmationTerminal{}, core.NewError(core.ErrorCodeConflict, false, nil)
	}
	if !confirmed {
		cancelled, ok := uniqueRecoveryDecision(decisions, request, "confirmation", core.DecisionStatusCancelled)
		if !ok || cancelled.Details["execution_path"] != finalization.Details["execution_path"] {
			return execution.ConfirmationTerminal{}, core.NewError(core.ErrorCodeConflict, false, nil)
		}
		return execution.ConfirmationTerminal{
			State: execution.ConfirmationContinuationCancelled, DecisionID: cancelled.ID,
		}, nil
	}

	dispatch, ok := uniqueRecoveryDecision(decisions, request, "dispatch", core.DecisionStatusSubmitted)
	if !ok || dispatch.Details["execution_path"] != finalization.Details["execution_path"] {
		return execution.ConfirmationTerminal{}, core.NewError(core.ErrorCodeConflict, false, nil)
	}
	leaseExpiresAt, ok := recoveryExecutionLease(dispatch)
	if !ok {
		return execution.ConfirmationTerminal{}, core.NewError(core.ErrorCodeInternal, false, nil)
	}
	outcome, err := recoverer.reader.GetOutcomeByDecision(
		ctx,
		request.OrganizationID,
		request.OwnerAccountID,
		dispatch.ID,
	)
	if core.IsErrorCode(err, core.ErrorCodeNotFound) {
		now := recoverer.now().UTC()
		if now.IsZero() {
			return execution.ConfirmationTerminal{}, core.NewError(core.ErrorCodeInternal, false, nil)
		}
		if now.Before(leaseExpiresAt) {
			return execution.ConfirmationTerminal{}, core.NewError(core.ErrorCodeConflict, false, nil)
		}
		outcome, err = recoverer.reader.ConvergeOutcome(
			ctx,
			unknownRecoveryOutcome(dispatch, request, now),
		)
	}
	if err != nil {
		return execution.ConfirmationTerminal{}, err
	}
	if !recoveryOutcomeMatches(outcome, dispatch, request) {
		return execution.ConfirmationTerminal{}, core.NewError(core.ErrorCodeInternal, false, nil)
	}
	terminal := execution.ConfirmationTerminal{DecisionID: dispatch.ID, OutcomeID: outcome.ID}
	switch outcome.Status {
	case core.OutcomeStatusSuccessful:
		terminal.State = execution.ConfirmationContinuationCompleted
	case core.OutcomeStatusFailed:
		terminal.State = execution.ConfirmationContinuationFailed
		terminal.DiagnosticCode = string(safeStoredErrorCode(outcome.DiagnosticCode, core.ErrorCodeInternal))
	case core.OutcomeStatusUnknown:
		terminal.State = execution.ConfirmationContinuationUnknown
		terminal.DiagnosticCode = string(core.ErrorCodeOutcomeUnknown)
	default:
		return execution.ConfirmationTerminal{}, core.NewError(core.ErrorCodeInternal, false, nil)
	}
	return terminal, nil
}

func recoveryExecutionLease(dispatch core.LifecycleDecision) (time.Time, bool) {
	value, ok := dispatch.Details["execution_lease_expires_at"].(string)
	if !ok || value == "" {
		return time.Time{}, false
	}
	leaseExpiresAt, err := time.Parse(time.RFC3339Nano, value)
	return leaseExpiresAt.UTC(), err == nil && leaseExpiresAt.After(dispatch.CreatedAt)
}

func unknownRecoveryOutcome(
	dispatch core.LifecycleDecision,
	request TerminalRecoveryRequest,
	createdAt time.Time,
) core.OperationOutcome {
	return core.OperationOutcome{
		ID: uuid.NewSHA1(
			uuid.NameSpaceOID,
			[]byte("connector-runtime-confirmation-outcome/"+dispatch.ID.String()),
		),
		DecisionID: dispatch.ID, OrganizationID: request.OrganizationID,
		OwnerAccountID: request.OwnerAccountID, ActingPrincipalID: request.ActingPrincipalID,
		ConnectorID: request.ConnectorID, ConnectionID: request.ConnectionID,
		Target: request.Target, OperationID: request.OperationID,
		LogicalOperationID: request.LogicalOperationID,
		Status:             core.OutcomeStatusUnknown,
		DiagnosticCode:     string(core.ErrorCodeOutcomeUnknown),
		CreatedAt:          createdAt.UTC(),
	}
}

func validTerminalRecoveryRequest(request TerminalRecoveryRequest) bool {
	return request.OrganizationID != uuid.Nil && request.OwnerAccountID != uuid.Nil &&
		request.ActingPrincipalID == request.OwnerAccountID && request.ConnectionID != uuid.Nil &&
		request.ConnectorID.Valid() && request.Target.Validate() == nil && request.OperationID.Valid() &&
		request.LogicalOperationID != "" && request.CorrelationID != "" && request.RouteReceipt != "" &&
		(request.Decision == execution.ConfirmationDecisionConfirmed ||
			request.Decision == execution.ConfirmationDecisionCancelled)
}

func uniqueRecoveryDecision(
	decisions []core.LifecycleDecision,
	request TerminalRecoveryRequest,
	name string,
	status core.DecisionStatus,
) (core.LifecycleDecision, bool) {
	var matched core.LifecycleDecision
	found := false
	for _, decision := range decisions {
		if decision.Decision != name || decision.Status != status || !recoveryDecisionMatches(decision, request) {
			continue
		}
		if found {
			return core.LifecycleDecision{}, false
		}
		matched, found = decision, true
	}
	return matched, found
}

func recoveryDecisionMatches(decision core.LifecycleDecision, request TerminalRecoveryRequest) bool {
	return decision.ID != uuid.Nil && decision.OrganizationID == request.OrganizationID &&
		decision.OwnerAccountID == request.OwnerAccountID &&
		decision.ActingPrincipalID == request.ActingPrincipalID &&
		decision.ConnectorID == request.ConnectorID && decision.ConnectionID == request.ConnectionID &&
		decision.Target != nil && decision.Target.SameIdentity(request.Target) &&
		decision.OperationID == request.OperationID &&
		decision.LogicalOperationID == request.LogicalOperationID &&
		decision.CorrelationID == request.CorrelationID &&
		decision.ReceiptID == string(request.RouteReceipt) &&
		decision.Details["execution_path"] == string(execution.PathConnectorNative) &&
		decision.Details["interactive"] == true
}

func recoveryOutcomeMatches(
	outcome core.OperationOutcome,
	dispatch core.LifecycleDecision,
	request TerminalRecoveryRequest,
) bool {
	return outcome.ID != uuid.Nil && outcome.DecisionID == dispatch.ID &&
		outcome.OrganizationID == request.OrganizationID && outcome.OwnerAccountID == request.OwnerAccountID &&
		outcome.ActingPrincipalID == request.ActingPrincipalID && outcome.ConnectorID == request.ConnectorID &&
		outcome.ConnectionID == request.ConnectionID && outcome.Target.SameIdentity(request.Target) &&
		outcome.OperationID == request.OperationID && outcome.LogicalOperationID == request.LogicalOperationID
}
