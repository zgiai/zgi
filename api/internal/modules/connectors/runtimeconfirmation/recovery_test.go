package runtimeconfirmation

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	connectoraudit "github.com/zgiai/zgi/api/internal/modules/connectors/audit"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"github.com/zgiai/zgi/api/internal/modules/connectors/execution"
)

func TestAuditTerminalRecovererProjectsDurableConfirmedOutcomes(t *testing.T) {
	tests := []struct {
		name           string
		status         core.OutcomeStatus
		diagnosticCode string
		wantState      execution.ConfirmationContinuationState
		wantDiagnostic string
	}{
		{
			name: "completed", status: core.OutcomeStatusSuccessful,
			wantState: execution.ConfirmationContinuationCompleted,
		},
		{
			name: "failed", status: core.OutcomeStatusFailed,
			diagnosticCode: string(core.ErrorCodeProviderUnavailable),
			wantState:      execution.ConfirmationContinuationFailed,
			wantDiagnostic: string(core.ErrorCodeProviderUnavailable),
		},
		{
			name: "failed diagnostic is sanitized", status: core.OutcomeStatusFailed,
			diagnosticCode: "provider said secret detail",
			wantState:      execution.ConfirmationContinuationFailed,
			wantDiagnostic: string(core.ErrorCodeInternal),
		},
		{
			name: "unknown", status: core.OutcomeStatusUnknown,
			diagnosticCode: string(core.ErrorCodeOutcomeUnknown),
			wantState:      execution.ConfirmationContinuationUnknown,
			wantDiagnostic: string(core.ErrorCodeOutcomeUnknown),
		},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			request, decisions, outcome := newAuditRecoveryFixture(true)
			outcome.Status = testCase.status
			outcome.DiagnosticCode = testCase.diagnosticCode
			reader := &auditRecoveryReaderStub{decisions: decisions, outcome: outcome}
			recoverer, err := NewAuditTerminalRecoverer(reader)
			if err != nil {
				t.Fatalf("NewAuditTerminalRecoverer() error = %v", err)
			}

			terminal, err := recoverer.RecoverTerminal(context.Background(), request)
			if err != nil {
				t.Fatalf("RecoverTerminal() error = %v", err)
			}
			if terminal.State != testCase.wantState || terminal.DecisionID != outcome.DecisionID ||
				terminal.OutcomeID != outcome.ID || terminal.DiagnosticCode != testCase.wantDiagnostic {
				t.Fatalf("RecoverTerminal() = %#v", terminal)
			}
			if reader.listCalls != 1 || reader.outcomeCalls != 1 {
				t.Fatalf("reader calls = %d/%d, want 1/1", reader.listCalls, reader.outcomeCalls)
			}
		})
	}
}

func TestAuditTerminalRecovererProjectsDurableCancellationWithoutOutcomeLookup(t *testing.T) {
	request, decisions, _ := newAuditRecoveryFixture(false)
	reader := &auditRecoveryReaderStub{decisions: decisions}
	recoverer, err := NewAuditTerminalRecoverer(reader)
	if err != nil {
		t.Fatalf("NewAuditTerminalRecoverer() error = %v", err)
	}

	terminal, err := recoverer.RecoverTerminal(context.Background(), request)
	if err != nil {
		t.Fatalf("RecoverTerminal() error = %v", err)
	}
	cancelled := decisions[1]
	if terminal.State != execution.ConfirmationContinuationCancelled ||
		terminal.DecisionID != cancelled.ID || terminal.OutcomeID != uuid.Nil || terminal.DiagnosticCode != "" {
		t.Fatalf("RecoverTerminal() = %#v", terminal)
	}
	if reader.outcomeCalls != 0 {
		t.Fatalf("cancel recovery performed %d outcome lookups", reader.outcomeCalls)
	}
}

func TestAuditTerminalRecovererRejectsMismatchedDurableIdentity(t *testing.T) {
	t.Run("decision", func(t *testing.T) {
		request, decisions, outcome := newAuditRecoveryFixture(true)
		decisions[0].ReceiptID = uuid.NewString()
		reader := &auditRecoveryReaderStub{decisions: decisions, outcome: outcome}
		recoverer, err := NewAuditTerminalRecoverer(reader)
		if err != nil {
			t.Fatalf("NewAuditTerminalRecoverer() error = %v", err)
		}

		if _, err = recoverer.RecoverTerminal(context.Background(), request); !core.IsErrorCode(err, core.ErrorCodeConflict) {
			t.Fatalf("RecoverTerminal() error = %v, want conflict", err)
		}
		if reader.outcomeCalls != 0 {
			t.Fatalf("mismatched decision performed %d outcome lookups", reader.outcomeCalls)
		}
	})

	t.Run("outcome", func(t *testing.T) {
		request, decisions, outcome := newAuditRecoveryFixture(true)
		outcome.ConnectionID = uuid.New()
		reader := &auditRecoveryReaderStub{decisions: decisions, outcome: outcome}
		recoverer, err := NewAuditTerminalRecoverer(reader)
		if err != nil {
			t.Fatalf("NewAuditTerminalRecoverer() error = %v", err)
		}

		if _, err = recoverer.RecoverTerminal(context.Background(), request); !core.IsErrorCode(err, core.ErrorCodeInternal) {
			t.Fatalf("RecoverTerminal() error = %v, want internal", err)
		}
	})
}

func TestAuditTerminalRecovererConvergesMissingSubmittedOutcomeToDurableUnknown(t *testing.T) {
	request, decisions, _ := newAuditRecoveryFixture(true)
	reader := &auditRecoveryReaderStub{
		decisions:  decisions,
		outcomeErr: core.NewError(core.ErrorCodeNotFound, false, nil),
	}
	recoverer, err := NewAuditTerminalRecoverer(reader)
	if err != nil {
		t.Fatalf("NewAuditTerminalRecoverer() error = %v", err)
	}

	terminal, err := recoverer.RecoverTerminal(context.Background(), request)
	if err != nil {
		t.Fatalf("RecoverTerminal() error = %v", err)
	}
	if terminal.State != execution.ConfirmationContinuationUnknown ||
		terminal.DecisionID != decisions[1].ID || terminal.OutcomeID == uuid.Nil ||
		terminal.DiagnosticCode != string(core.ErrorCodeOutcomeUnknown) {
		t.Fatalf("RecoverTerminal() = %#v", terminal)
	}
	if reader.convergeCalls != 1 || reader.outcome.ID != terminal.OutcomeID ||
		reader.outcome.Status != core.OutcomeStatusUnknown {
		t.Fatalf("durable convergence = calls %d, outcome %#v", reader.convergeCalls, reader.outcome)
	}
	again, err := recoverer.RecoverTerminal(context.Background(), request)
	if err != nil || again != terminal || reader.convergeCalls != 1 {
		t.Fatalf("duplicate recovery = %#v, %v; convergence calls=%d", again, err, reader.convergeCalls)
	}
}

func TestAuditTerminalRecovererWaitsForDispatchLeaseBeforeConvergingCrashToUnknown(t *testing.T) {
	request, decisions, _ := newAuditRecoveryFixture(true)
	createdAt := time.Date(2026, 9, 2, 10, 0, 0, 0, time.UTC)
	leaseExpiresAt := createdAt.Add(2 * time.Minute)
	decisions[1].CreatedAt = createdAt
	decisions[1].Details["execution_lease_expires_at"] = leaseExpiresAt.Format(time.RFC3339Nano)
	reader := &auditRecoveryReaderStub{
		decisions:  decisions,
		outcomeErr: core.NewError(core.ErrorCodeNotFound, false, nil),
	}
	recoverer, err := NewAuditTerminalRecoverer(reader)
	if err != nil {
		t.Fatal(err)
	}
	now := createdAt.Add(time.Minute)
	recoverer.now = func() time.Time { return now }

	if _, err = recoverer.RecoverTerminal(context.Background(), request); !core.IsErrorCode(err, core.ErrorCodeConflict) {
		t.Fatalf("active lease recovery error = %v, want conflict", err)
	}
	if reader.convergeCalls != 0 || reader.outcome.ID != uuid.Nil {
		t.Fatalf("active lease converged outcome = calls %d, %#v", reader.convergeCalls, reader.outcome)
	}

	now = leaseExpiresAt
	terminal, err := recoverer.RecoverTerminal(context.Background(), request)
	if err != nil || terminal.State != execution.ConfirmationContinuationUnknown ||
		terminal.DecisionID != decisions[1].ID || terminal.OutcomeID == uuid.Nil {
		t.Fatalf("expired lease recovery = %#v, %v", terminal, err)
	}
	if reader.convergeCalls != 1 || reader.outcome.ID != terminal.OutcomeID ||
		reader.outcome.CreatedAt != leaseExpiresAt {
		t.Fatalf("expired lease convergence = calls %d, %#v", reader.convergeCalls, reader.outcome)
	}
	again, err := recoverer.RecoverTerminal(context.Background(), request)
	if err != nil || again != terminal || reader.convergeCalls != 1 {
		t.Fatalf("duplicate expired recovery = %#v, %v; calls=%d", again, err, reader.convergeCalls)
	}
}

func newAuditRecoveryFixture(
	confirmed bool,
) (TerminalRecoveryRequest, []core.LifecycleDecision, core.OperationOutcome) {
	organizationID, ownerAccountID, connectionID := uuid.New(), uuid.New(), uuid.New()
	target := core.TargetRef{Type: core.TargetTypeChat, ID: "conversation-1"}
	request := TerminalRecoveryRequest{
		OrganizationID: organizationID, OwnerAccountID: ownerAccountID, ActingPrincipalID: ownerAccountID,
		ConnectionID: connectionID, ConnectorID: core.Identifier("github"), Target: target,
		OperationID: core.Identifier("create_issue"), LogicalOperationID: "logical-operation-1",
		CorrelationID: "correlation-1", RouteReceipt: execution.RouteClaimReceipt(uuid.NewString()),
		Decision: execution.ConfirmationDecisionConfirmed,
	}
	finalization := auditRecoveryDecision(request, uuid.New(), "confirmation_finalization", core.DecisionStatusSuccessful, confirmed)
	if !confirmed {
		request.Decision = execution.ConfirmationDecisionCancelled
		cancelled := auditRecoveryDecision(request, uuid.New(), "confirmation", core.DecisionStatusCancelled, false)
		return request, []core.LifecycleDecision{finalization, cancelled}, core.OperationOutcome{}
	}
	dispatch := auditRecoveryDecision(request, uuid.New(), "dispatch", core.DecisionStatusSubmitted, true)
	outcome := core.OperationOutcome{
		ID: uuid.New(), DecisionID: dispatch.ID, OrganizationID: organizationID,
		OwnerAccountID: ownerAccountID, ActingPrincipalID: ownerAccountID,
		ConnectorID: request.ConnectorID, ConnectionID: connectionID, Target: target,
		OperationID: request.OperationID, LogicalOperationID: request.LogicalOperationID,
		Status: core.OutcomeStatusSuccessful, CreatedAt: time.Now().UTC(),
	}
	return request, []core.LifecycleDecision{finalization, dispatch}, outcome
}

func auditRecoveryDecision(
	request TerminalRecoveryRequest,
	id uuid.UUID,
	decision string,
	status core.DecisionStatus,
	confirmed bool,
) core.LifecycleDecision {
	target := request.Target
	createdAt := time.Now().UTC()
	details := map[string]any{
		"execution_path": string(execution.PathConnectorNative), "interactive": true,
		"confirmation": confirmed,
	}
	if decision == "dispatch" && status == core.DecisionStatusSubmitted {
		createdAt = createdAt.Add(-2 * time.Minute)
		details["execution_lease_expires_at"] = createdAt.Add(time.Minute).Format(time.RFC3339Nano)
	}
	return core.LifecycleDecision{
		ID: id, OrganizationID: request.OrganizationID, OwnerAccountID: request.OwnerAccountID,
		ActingPrincipalID: request.ActingPrincipalID, ConnectorID: request.ConnectorID,
		ConnectionID: request.ConnectionID, Target: &target, OperationID: request.OperationID,
		LogicalOperationID: request.LogicalOperationID, Decision: decision, Status: status,
		CorrelationID: request.CorrelationID, ReceiptID: string(request.RouteReceipt),
		Details: details, CreatedAt: createdAt,
	}
}

type auditRecoveryReaderStub struct {
	decisions     []core.LifecycleDecision
	outcome       core.OperationOutcome
	listErr       error
	outcomeErr    error
	listCalls     int
	outcomeCalls  int
	convergeCalls int
}

func (reader *auditRecoveryReaderStub) ListDecisions(
	_ context.Context,
	_, _ uuid.UUID,
	_ connectoraudit.DecisionListFilter,
) ([]core.LifecycleDecision, error) {
	reader.listCalls++
	return reader.decisions, reader.listErr
}

func (reader *auditRecoveryReaderStub) GetOutcomeByDecision(
	_ context.Context,
	_, _, _ uuid.UUID,
) (core.OperationOutcome, error) {
	reader.outcomeCalls++
	return reader.outcome, reader.outcomeErr
}

func (reader *auditRecoveryReaderStub) ConvergeOutcome(
	_ context.Context,
	unknown core.OperationOutcome,
) (core.OperationOutcome, error) {
	reader.convergeCalls++
	if reader.outcomeErr != nil && !core.IsErrorCode(reader.outcomeErr, core.ErrorCodeNotFound) {
		return core.OperationOutcome{}, reader.outcomeErr
	}
	if reader.outcome.ID != uuid.Nil {
		return reader.outcome, nil
	}
	reader.outcome = unknown
	reader.outcomeErr = nil
	return unknown, nil
}
