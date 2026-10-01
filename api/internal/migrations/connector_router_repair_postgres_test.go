package migrations

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/audit"
	"github.com/zgiai/zgi/api/internal/modules/connectors/catalog"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"github.com/zgiai/zgi/api/internal/modules/connectors/execution"
	"github.com/zgiai/zgi/api/internal/modules/connectors/runtimeconfirmation"
	appcatalog "github.com/zgiai/zgi/api/pkg/apperror/catalog"
	"github.com/zgiai/zgi/api/pkg/apperror/transport"
)

// Only the external boundaries are fixtures. Claim, execution, audit, locks,
// indexes, and state triggers all run through the production implementations.
type repairRouterFixture struct {
	binding           core.TargetBinding
	connection        core.PersonalConnection
	calls             atomic.Int32
	credentialFailure atomic.Bool
	providerUnknown   bool
	entered, release  chan struct{}
}

func (f *repairRouterFixture) GetBinding(_ context.Context, org, owner, id uuid.UUID) (core.TargetBinding, error) {
	if org != f.binding.OrganizationID || owner != f.binding.OwnerAccountID || id != f.binding.ID {
		return core.TargetBinding{}, core.NewError(core.ErrorCodeAccessDenied, false, nil)
	}
	return f.binding, nil
}
func (f *repairRouterFixture) GetConnection(_ context.Context, org, owner, id uuid.UUID) (core.PersonalConnection, error) {
	if org != f.connection.OrganizationID || owner != f.connection.OwnerAccountID || id != f.connection.ID {
		return core.PersonalConnection{}, core.NewError(core.ErrorCodeAccessDenied, false, nil)
	}
	return f.connection, nil
}
func (f *repairRouterFixture) AuthorizeTarget(_ context.Context, r execution.TargetAuthorizationRequest) error {
	if r.OwnerAccountID != r.ActingPrincipalID || !r.Binding.Target.SameIdentity(f.binding.Target) {
		return core.NewError(core.ErrorCodeAccessDenied, false, nil)
	}
	return nil
}
func (f *repairRouterFixture) ResolveRisk(_ context.Context, r execution.RiskRequest) (execution.RiskClassification, error) {
	if r.OperationID == "feishu.message.send" {
		return execution.RiskHigh, nil
	}
	return execution.RiskTrustedLow, nil
}
func (f *repairRouterFixture) ResolveCredentials(context.Context, execution.CredentialResolutionRequest) (map[string]string, error) {
	if f.credentialFailure.Load() {
		return nil, core.NewError(core.ErrorCodeConflict, false, nil)
	}
	return map[string]string{"token": "isolated-provider-fixture"}, nil
}
func (f *repairRouterFixture) Invoke(ctx context.Context, _ execution.ProviderInvocation) (execution.ProviderInvocationResult, error) {
	f.calls.Add(1)
	if f.entered != nil {
		f.entered <- struct{}{}
		select {
		case <-f.release:
		case <-ctx.Done():
			return execution.ProviderInvocationResult{}, ctx.Err()
		}
	}
	if f.providerUnknown {
		return execution.ProviderInvocationResult{}, errors.New("external request outcome unavailable")
	}
	return execution.ProviderInvocationResult{Status: core.OutcomeStatusSuccessful, Value: "fixture-only"}, nil
}

func newRepairRouter(t *testing.T, target core.TargetRef, wrap func(*audit.GormRepository) execution.DecisionOutcomeRecorder) (*execution.NativeRouter, *repairRouterFixture, execution.ExecutionRequest, *audit.GormRepository) {
	t.Helper()
	db, schema := connectorRepairDatabase(t, "")
	if err := repairConnectorTestSchema(db, schema); err != nil {
		t.Fatal(err)
	}
	definition, err := catalog.FeishuDefinition()
	if err != nil {
		t.Fatal(err)
	}
	catalogue, err := catalog.New(definition)
	if err != nil {
		t.Fatal(err)
	}
	owner, org := uuid.New(), uuid.New()
	r := execution.ExecutionRequest{OrganizationID: org, OwnerAccountID: owner, ActingPrincipalID: owner, BindingID: uuid.New(), ConnectionID: uuid.New(), ConnectorID: "feishu", Target: target, OperationID: "feishu.chat.list", LogicalOperationID: uuid.NewString(), CorrelationID: uuid.NewString(), Interactive: true, Input: map[string]any{}}
	ops := []core.Identifier{"feishu.chat.list", "feishu.bot.get", "feishu.message.send"}
	now := time.Now().UTC()
	f := &repairRouterFixture{
		binding:    core.TargetBinding{Constraint: core.OwnerConstraint(owner, ops), ID: r.BindingID, OrganizationID: org, OwnerAccountID: owner, ConnectionID: r.ConnectionID, ConnectorID: r.ConnectorID, Target: target, Status: core.BindingStatusActive, Revision: 1},
		connection: core.PersonalConnection{Constraint: core.OwnerConstraint(owner, ops), ID: r.ConnectionID, OrganizationID: org, OwnerAccountID: owner, ConnectorID: r.ConnectorID, ProviderID: core.Identifier(definition.ProviderID), DriverID: core.Identifier(definition.DriverID), AuthMethodID: core.Identifier(definition.AuthMethodID), Name: "isolated fixture", EncryptedCredentials: "fixture envelope", CredentialVersion: 1, State: core.ReadinessReady, UsableOperationIDs: ops, UnavailableOperations: map[core.Identifier]string{}, Revision: 1, LastVerifiedAt: now, CreatedAt: now, UpdatedAt: now},
	}
	definitions := append(appcatalog.DefaultDefinitions(), core.CatalogDefinitions()...)
	productCatalog, err := appcatalog.New(appcatalog.LocaleEnglishUS, appcatalog.CodeInternal, definitions...)
	if err != nil {
		t.Fatal(err)
	}
	projector, err := transport.NewProjector(productCatalog)
	if err != nil {
		t.Fatal(err)
	}
	repo := audit.NewGormRepository(db)
	var recorder execution.DecisionOutcomeRecorder = repo
	if wrap != nil {
		recorder = wrap(repo)
	}
	service, err := execution.NewService(catalogue, f, f, f, f, f, recorder, f, projector)
	if err != nil {
		t.Fatal(err)
	}
	router, err := execution.NewNativeRouter(execution.NewGormInvocationStore(db), service)
	if err != nil {
		t.Fatal(err)
	}
	return router, f, r, repo
}

func TestConnectorRouterSameTurnPostgres(t *testing.T) {
	for _, target := range []core.TargetRef{{Type: core.TargetTypeChat, ID: "chat"}, {Type: core.TargetTypeAgent, ID: "agent"}, {Type: core.TargetTypeWorkflow, ID: "workflow", Scope: core.ScopeDraft, BindingInstanceID: "node-binding"}} {
		t.Run(string(target.Type), func(t *testing.T) {
			router, f, r, repo := newRepairRouter(t, target, nil)
			ctx := execution.WithPresentationLocale(context.Background(), "zh-Hans")
			for _, op := range []core.Identifier{"feishu.chat.list", "feishu.bot.get", "feishu.bot.get"} {
				r.OperationID, r.LogicalOperationID = op, uuid.NewString()
				result, err := router.Route(ctx, r)
				if err != nil || result.Outcome.Status != core.OutcomeStatusSuccessful || result.Display == nil || result.Display.Name == "" {
					t.Fatalf("%s: state=%s err=%v", op, result.State, err)
				}
			}
			result, err := router.Route(ctx, r)
			if err != nil || result.Outcome.Status != core.OutcomeStatusSuccessful || f.calls.Load() != 3 {
				t.Fatalf("duplicate replay: %v calls=%d", err, f.calls.Load())
			}
			r.OperationID, r.LogicalOperationID = "feishu.message.send", uuid.NewString()
			pending, err := router.Route(ctx, r)
			if err != nil || pending.Confirmation == nil || f.calls.Load() != 3 {
				t.Fatalf("send must await confirmation: %v", err)
			}
			r.AcceptedOperation = pending.Confirmation.AcceptedOperation
			r.Confirmation = &execution.RuntimeConfirmationCommand{RouteReceipt: pending.Confirmation.RouteReceipt, Decision: execution.ConfirmationDecisionConfirmed}
			result, err = router.Route(ctx, r)
			if err != nil || result.Outcome.Status != core.OutcomeStatusSuccessful || f.calls.Load() != 4 {
				t.Fatalf("confirmed send: %v calls=%d", err, f.calls.Load())
			}
			// Enough unrelated same-correlation decisions to exceed the former
			// latest-100 scan. Exact identity recovery must still find this send.
			send := r
			r.Confirmation, r.AcceptedOperation = nil, nil
			r.OperationID = "feishu.bot.get"
			for i := 0; i < 105; i++ {
				r.LogicalOperationID = uuid.NewString()
				if _, err = router.Route(ctx, r); err != nil {
					t.Fatal(err)
				}
			}
			recoverer, err := runtimeconfirmation.NewAuditTerminalRecoverer(repo)
			if err != nil {
				t.Fatal(err)
			}
			terminal, err := recoverer.RecoverTerminal(ctx, runtimeconfirmation.TerminalRecoveryRequest{OrganizationID: send.OrganizationID, OwnerAccountID: send.OwnerAccountID, ActingPrincipalID: send.ActingPrincipalID, ConnectionID: send.ConnectionID, ConnectorID: send.ConnectorID, Target: send.Target, OperationID: send.OperationID, LogicalOperationID: send.LogicalOperationID, CorrelationID: send.CorrelationID, RouteReceipt: send.Confirmation.RouteReceipt, Decision: send.Confirmation.Decision})
			if err != nil || terminal.State != execution.ConfirmationContinuationCompleted || terminal.OutcomeID != result.Outcome.ID {
				t.Fatalf("precise old receipt recovery: %+v %v", terminal, err)
			}
		})
	}
}

func TestConnectorRouterRejectionAndUncertaintyPostgres(t *testing.T) {
	ctx := execution.WithPresentationLocale(context.Background(), "zh-Hans")
	target := core.TargetRef{Type: core.TargetTypeChat, ID: "chat"}
	t.Run("durable rejection", func(t *testing.T) {
		router, f, r, _ := newRepairRouter(t, target, nil)
		f.credentialFailure.Store(true)
		for i := 0; i < 2; i++ {
			result, err := router.Route(ctx, r)
			if !core.IsErrorCode(err, core.ErrorCodeConflict) || result.State != execution.ExecutionStateRejected || !result.Evidence.ValidFor(result.State) || result.DecisionID == uuid.Nil || result.Display.Summary == "" || f.calls.Load() != 0 {
				t.Fatalf("rejection=%+v %v calls=%d", result, err, f.calls.Load())
			}
			f.credentialFailure.Store(false)
		}
	})
	t.Run("provider uncertainty never replays", func(t *testing.T) {
		router, f, r, _ := newRepairRouter(t, target, nil)
		f.providerUnknown = true
		for i := 0; i < 2; i++ {
			result, _ := router.Route(ctx, r)
			if result.Outcome.Status != core.OutcomeStatusUnknown || f.calls.Load() != 1 {
				t.Fatalf("unknown was replayed: %+v calls=%d", result, f.calls.Load())
			}
		}
	})
	t.Run("ambiguous audit commit", func(t *testing.T) {
		router, f, r, _ := newRepairRouter(t, target, func(repo *audit.GormRepository) execution.DecisionOutcomeRecorder { return &repairAmbiguousAudit{repo} })
		for i := 0; i < 2; i++ {
			result, _ := router.Route(ctx, r)
			if result.State != execution.ExecutionStateUnresolved || result.Evidence.Disposition != execution.DispatchInProgress || f.calls.Load() != 0 {
				t.Fatalf("ambiguous write was classified as safe to retry: %+v", result)
			}
		}
	})
	t.Run("rejection audit unavailable preserves barrier", func(t *testing.T) {
		router, f, r, _ := newRepairRouter(t, target, func(repo *audit.GormRepository) execution.DecisionOutcomeRecorder {
			return &repairRejectionUnavailable{repo}
		})
		f.credentialFailure.Store(true)
		result, _ := router.Route(ctx, r)
		if result.State != execution.ExecutionStateRejected || result.DecisionID != uuid.Nil || f.calls.Load() != 0 {
			t.Fatalf("fresh local rejection=%+v", result)
		}
		f.credentialFailure.Store(false)
		result, _ = router.Route(ctx, r)
		if result.State != execution.ExecutionStateUnresolved || result.Evidence.Disposition != execution.DispatchUnknown || f.calls.Load() != 0 {
			t.Fatalf("claim without audit was replayed: %+v", result)
		}
	})
	t.Run("outcome persistence failure never replays", func(t *testing.T) {
		router, f, r, _ := newRepairRouter(t, target, func(repo *audit.GormRepository) execution.DecisionOutcomeRecorder {
			return &repairOutcomeUnavailable{repo}
		})
		for i := 0; i < 2; i++ {
			result, _ := router.Route(ctx, r)
			if result.State != execution.ExecutionStateUnresolved || f.calls.Load() != 1 {
				t.Fatalf("outcome failure=%+v calls=%d", result, f.calls.Load())
			}
		}
	})
	t.Run("active provider duplicate is processing", func(t *testing.T) {
		router, f, r, _ := newRepairRouter(t, target, nil)
		f.entered, f.release = make(chan struct{}, 1), make(chan struct{})
		done := make(chan execution.ExecutionResult, 1)
		go func() { result, _ := router.Route(ctx, r); done <- result }()
		select {
		case <-f.entered:
		case <-time.After(10 * time.Second):
			close(f.release)
			t.Fatal("provider was not reached")
		}
		result, _ := router.Route(ctx, r)
		close(f.release)
		if result.State != execution.ExecutionStateUnresolved || result.Evidence.Disposition != execution.DispatchInProgress || f.calls.Load() != 1 {
			t.Fatalf("active duplicate=%+v", result)
		}
		if finished := <-done; finished.Outcome.Status != core.OutcomeStatusSuccessful {
			t.Fatalf("first execution=%+v", finished)
		}
	})
}

type repairRejectionUnavailable struct{ *audit.GormRepository }

func (*repairRejectionUnavailable) RejectExecution(context.Context, core.LifecycleDecision) error {
	return errors.New("audit unavailable")
}

type repairOutcomeUnavailable struct{ *audit.GormRepository }

func (*repairOutcomeUnavailable) RecordOutcome(context.Context, core.OperationOutcome) error {
	return errors.New("outcome unavailable")
}
func (*repairOutcomeUnavailable) ConvergeOutcome(context.Context, core.OperationOutcome) (core.OperationOutcome, error) {
	return core.OperationOutcome{}, errors.New("outcome unavailable")
}

type repairAmbiguousAudit struct{ *audit.GormRepository }

func (r *repairAmbiguousAudit) CreateDecision(ctx context.Context, d core.LifecycleDecision) error {
	if err := r.GormRepository.CreateDecision(ctx, d); err != nil {
		return err
	}
	return errors.New("commit response lost")
}

func TestConnectorRouterConcurrentConfirmationPostgres(t *testing.T) {
	for _, cancel := range []bool{false, true} {
		t.Run(map[bool]string{false: "confirm-confirm", true: "confirm-cancel"}[cancel], func(t *testing.T) {
			router, f, r, _ := newRepairRouter(t, core.TargetRef{Type: core.TargetTypeChat, ID: "chat"}, nil)
			r.OperationID = "feishu.message.send"
			pending, err := router.Route(context.Background(), r)
			if err != nil || pending.Confirmation == nil {
				t.Fatal(err)
			}
			r.AcceptedOperation = pending.Confirmation.AcceptedOperation
			r.Confirmation = &execution.RuntimeConfirmationCommand{RouteReceipt: pending.Confirmation.RouteReceipt, Decision: execution.ConfirmationDecisionConfirmed}
			other := r
			if cancel {
				other.Confirmation = &execution.RuntimeConfirmationCommand{RouteReceipt: r.Confirmation.RouteReceipt, Decision: execution.ConfirmationDecisionCancelled}
			}
			start := make(chan struct{})
			var wg sync.WaitGroup
			results := make([]execution.ExecutionResult, 2)
			for i, request := range []execution.ExecutionRequest{r, other} {
				wg.Add(1)
				go func(i int, request execution.ExecutionRequest) {
					defer wg.Done()
					<-start
					results[i], _ = router.Route(context.Background(), request)
				}(i, request)
			}
			close(start)
			wg.Wait()
			if f.calls.Load() > 1 || (!cancel && f.calls.Load() != 1) {
				t.Fatalf("provider calls=%d", f.calls.Load())
			}
			for _, result := range results {
				if result.State == execution.ExecutionStateRejected {
					t.Fatal("concurrent winner misclassified as rejected")
				}
			}
			if cancel && f.calls.Load() == 0 {
				for _, result := range results {
					if result.State != execution.ExecutionStateCancelled {
						t.Fatalf("cancellation winner lost: %+v", results)
					}
				}
			}
		})
	}
}
