package execution

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/audit"
	"github.com/zgiai/zgi/api/internal/modules/connectors/catalog"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
)

type DispatchDisposition string

const (
	DispatchNotSubmitted DispatchDisposition = "not_submitted"
	DispatchInProgress   DispatchDisposition = "in_progress"
	DispatchUnknown      DispatchDisposition = "unknown"
)

// DispatchEvidence is authored by execution, never decoded from provider JSON.
// Only its version and disposition may enter the ordinary invocation projection.
type DispatchEvidence struct {
	Version        int
	Disposition    DispatchDisposition
	DiagnosticCode core.ErrorCode
}

func (e *DispatchEvidence) ValidFor(state ExecutionState) bool {
	return e != nil && e.Version == 1 && e.DiagnosticCode.Valid() &&
		((state == ExecutionStateRejected && e.Disposition == DispatchNotSubmitted) ||
			(state == ExecutionStateUnresolved && (e.Disposition == DispatchUnknown || e.Disposition == DispatchInProgress)))
}

type OperationDisplay struct {
	Name     string
	NameI18n catalog.LocalizedText
	Locale   string
	Summary  string
}
type dispatchProgress struct {
	submissionAttempted, definitelyNotWritten bool
	display                                   *OperationDisplay
}
type executionRejector interface {
	RejectExecution(context.Context, core.LifecycleDecision) error
}
type executionEvidenceReader interface {
	ListDecisions(context.Context, uuid.UUID, uuid.UUID, audit.DecisionListFilter) ([]core.LifecycleDecision, error)
	GetOutcomeByLogicalOperation(context.Context, uuid.UUID, uuid.UUID, string) (core.OperationOutcome, error)
}

func UnresolvedResult(code core.ErrorCode) ExecutionResult {
	if !code.Valid() {
		code = core.ErrorCodeOutcomeUnknown
	}
	return ExecutionResult{State: ExecutionStateUnresolved, Evidence: &DispatchEvidence{Version: 1, Disposition: DispatchUnknown, DiagnosticCode: code}}
}

func rejectedResult(id uuid.UUID, code core.ErrorCode) ExecutionResult {
	if !code.Valid() {
		code = core.ErrorCodeInternal
	}
	return ExecutionResult{State: ExecutionStateRejected, DecisionID: id, Evidence: &DispatchEvidence{Version: 1, Disposition: DispatchNotSubmitted, DiagnosticCode: code}}
}

// RecoverInvocation has no provider call. Router first verifies the immutable
// claim snapshot; all decision/outcome reads remain organization+owner scoped.
func (service *Service) RecoverInvocation(ctx context.Context, pinned PinnedExecution) (result ExecutionResult, resultErr error) {
	defer func() {
		if accepted, ok := acceptedCatalogOperation(service.catalog, pinned.Request); ok {
			result.Display = &OperationDisplay{Name: accepted.Operation.Name, NameI18n: accepted.Operation.NameI18n}
		}
		result = service.presentResult(ctx, result)
	}()
	unknown := UnresolvedResult(core.ErrorCodeOutcomeUnknown)
	reader, ok := service.audit.(executionEvidenceReader)
	if !ok || ValidatePinnedExecution(pinned, PathConnectorNative) != nil {
		return unknown, connectorError(core.ErrorCodeConflict, false)
	}
	r := pinned.Request
	decisions, err := reader.ListDecisions(ctx, r.OrganizationID, r.OwnerAccountID, audit.DecisionListFilter{ConnectorID: r.ConnectorID, ConnectionID: r.ConnectionID, LogicalOperationID: r.LogicalOperationID, ReceiptID: string(pinned.Receipt)})
	if err != nil {
		return unknown, connectorError(core.ErrorCodeOutcomeUnknown, false)
	}
	var submitted, denied, cancelled *core.LifecycleDecision
	for i := range decisions {
		d := &decisions[i]
		if !invocationDecisionMatches(*d, pinned) {
			return unknown, connectorError(core.ErrorCodeConflict, false)
		}
		switch {
		case d.Decision == "dispatch" && d.Status == core.DecisionStatusSubmitted:
			if submitted != nil {
				return unknown, connectorError(core.ErrorCodeConflict, false)
			}
			submitted = d
		case d.Decision == "authorization" && d.Status == core.DecisionStatusDenied && d.Details["rejection_finalized"] == true:
			denied = d
		case d.Decision == "confirmation" && d.Status == core.DecisionStatusCancelled:
			cancelled = d
		}
	}
	if submitted == nil {
		if denied != nil && cancelled == nil {
			code := core.ErrorCodeAccessDenied
			if stored, ok := denied.Details["diagnostic_code"].(string); ok && core.ErrorCode(stored).Valid() {
				code = core.ErrorCode(stored)
			}
			return rejectedResult(denied.ID, code), connectorError(code, false)
		}
		if cancelled != nil && denied == nil {
			return ExecutionResult{State: ExecutionStateCancelled, DecisionID: cancelled.ID}, nil
		}
		return unknown, connectorError(core.ErrorCodeConflict, false)
	}
	if denied != nil || cancelled != nil {
		return unknown, connectorError(core.ErrorCodeConflict, false)
	}
	unknown.DecisionID = submitted.ID
	outcome, err := reader.GetOutcomeByLogicalOperation(ctx, r.OrganizationID, r.OwnerAccountID, r.LogicalOperationID)
	if err == nil {
		if outcome.Validate() != nil || outcome.DecisionID != submitted.ID || outcome.ActingPrincipalID != r.ActingPrincipalID || outcome.ConnectorID != r.ConnectorID || outcome.ConnectionID != r.ConnectionID || !outcome.Target.SameIdentity(r.Target) || outcome.OperationID != r.OperationID {
			return unknown, connectorError(core.ErrorCodeConflict, false)
		}
		return ExecutionResult{State: ExecutionStateCompleted, DecisionID: submitted.ID, Outcome: outcome}, executionOutcomeError(outcome, core.ErrorCodeInternal, false)
	}
	if core.IsErrorCode(err, core.ErrorCodeNotFound) {
		lease, _ := submitted.Details["execution_lease_expires_at"].(string)
		deadline, parseErr := time.Parse(time.RFC3339Nano, lease)
		if parseErr == nil && deadline.After(submitted.CreatedAt) && service.now().Before(deadline) {
			unknown.Evidence.Disposition = DispatchInProgress
		}
	}
	return unknown, connectorError(core.ErrorCodeOutcomeUnknown, false)
}

func invocationDecisionMatches(d core.LifecycleDecision, pinned PinnedExecution) bool {
	r := pinned.Request
	return d.OrganizationID == r.OrganizationID && d.OwnerAccountID == r.OwnerAccountID && d.ActingPrincipalID == r.ActingPrincipalID && d.ConnectionID == r.ConnectionID && d.ConnectorID == r.ConnectorID && d.OperationID == r.OperationID && d.LogicalOperationID == r.LogicalOperationID && d.Target != nil && d.Target.SameIdentity(r.Target) && d.ReceiptID == string(pinned.Receipt) && d.Details["execution_path"] == string(PathConnectorNative)
}

func (service *Service) recoverAfterFailure(ctx context.Context, pinned PinnedExecution, cause error) (ExecutionResult, error) {
	result, err := service.RecoverInvocation(ctx, pinned)
	if result.State == ExecutionStateUnresolved {
		result.Evidence.DiagnosticCode = core.ErrorCodeOf(cause)
		return result, cause
	}
	return result, err
}
