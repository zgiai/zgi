package runtimeconfirmation

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/catalog"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"github.com/zgiai/zgi/api/internal/modules/connectors/execution"
)

const runtimeConfirmationSecretSentinel = "runtime-confirmation-secret-sentinel"

func TestServiceContinueRestoresFrozenRequestAndPersistsCompletedTerminal(t *testing.T) {
	store, command := newRuntimeConfirmationMemoryStore()
	decisionID, outcomeID := uuid.New(), uuid.New()
	router := &runtimeConfirmationRouterSpy{
		result: completedExecutionResult(decisionID, outcomeID, map[string]any{
			"provider_value": runtimeConfirmationSecretSentinel,
		}),
	}
	service := mustRuntimeConfirmationService(t, store, router)

	result, err := service.Continue(context.Background(), command)
	if err != nil {
		t.Fatalf("Continue() error = %v", err)
	}
	if result.State != StateCompleted || result.DecisionID != decisionID || result.ErrorCode != "" {
		t.Fatalf("Continue() = %#v", result)
	}
	if result.Origin != store.continuation.Frozen.Origin || result.ExecutionResult == nil ||
		result.ExecutionResult.Outcome.ID != outcomeID {
		t.Fatalf("trusted continuation result = %#v", result)
	}
	frozenRequest := store.continuation.Frozen.Request
	if result.RequestIdentity.OrganizationID != frozenRequest.OrganizationID ||
		result.RequestIdentity.OwnerAccountID != frozenRequest.OwnerAccountID ||
		result.RequestIdentity.ActingPrincipalID != frozenRequest.ActingPrincipalID ||
		result.RequestIdentity.BindingID != frozenRequest.BindingID ||
		result.RequestIdentity.ConnectionID != frozenRequest.ConnectionID ||
		result.RequestIdentity.ConnectorID != frozenRequest.ConnectorID ||
		result.RequestIdentity.Target != frozenRequest.Target ||
		result.RequestIdentity.OperationID != frozenRequest.OperationID ||
		result.RequestIdentity.LogicalOperationID != frozenRequest.LogicalOperationID ||
		result.RequestIdentity.CorrelationID != frozenRequest.CorrelationID ||
		result.RequestIdentity.Interactive != frozenRequest.Interactive {
		t.Fatalf("request identity = %#v, want frozen authority tuple", result.RequestIdentity)
	}
	if router.calls != 1 || len(router.requests) != 1 {
		t.Fatalf("Router calls = %d, requests = %d", router.calls, len(router.requests))
	}
	routed := router.requests[0]
	if routed.Confirmation == nil || routed.Confirmation.RouteReceipt != command.RouteReceipt ||
		routed.Confirmation.Decision != execution.ConfirmationDecisionConfirmed {
		t.Fatalf("Router confirmation = %#v", routed.Confirmation)
	}
	if routed.AcceptedOperation == nil ||
		routed.AcceptedOperation.ContractDigest != store.continuation.Frozen.AcceptedOperation.ContractDigest ||
		routed.AcceptedOperation.PluginVersion != store.continuation.Frozen.AcceptedOperation.PluginVersion {
		t.Fatalf("Router accepted operation = %#v", routed.AcceptedOperation)
	}
	input, ok := routed.Input.(map[string]any)
	if !ok || input["query"] != runtimeConfirmationSecretSentinel {
		t.Fatalf("Router input = %#v", routed.Input)
	}
	if store.continuation.State != execution.ConfirmationContinuationCompleted ||
		store.continuation.DecisionID != decisionID || store.continuation.OutcomeID != outcomeID {
		t.Fatalf("stored continuation = %#v", store.continuation)
	}

	serialized, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("json.Marshal(Result) error = %v", err)
	}
	for _, forbidden := range []string{
		runtimeConfirmationSecretSentinel,
		store.continuation.Frozen.Origin.ConversationID,
		`"ExecutionResult"`,
		`"Origin"`,
		`"RequestIdentity"`,
	} {
		if strings.Contains(string(serialized), forbidden) {
			t.Fatalf("serialized Result exposed %q: %s", forbidden, serialized)
		}
	}
}

func TestServiceContinueDuplicateAndConflictingDecisionsDoNotRouteAgain(t *testing.T) {
	store, command := newRuntimeConfirmationMemoryStore()
	decisionID, outcomeID := uuid.New(), uuid.New()
	router := &runtimeConfirmationRouterSpy{result: completedExecutionResult(decisionID, outcomeID, "provider-value")}
	service := mustRuntimeConfirmationService(t, store, router)

	first, err := service.Continue(context.Background(), command)
	if err != nil || first.State != StateCompleted {
		t.Fatalf("first Continue() = %#v, %v", first, err)
	}
	duplicate, err := service.Continue(context.Background(), command)
	if err != nil || duplicate.State != StateCompleted || duplicate.DecisionID != decisionID {
		t.Fatalf("duplicate Continue() = %#v, %v", duplicate, err)
	}
	if duplicate.ExecutionResult != nil {
		t.Fatalf("duplicate unexpectedly returned live execution = %#v", duplicate.ExecutionResult)
	}
	if duplicate.RequestIdentity != first.RequestIdentity {
		t.Fatalf("duplicate request identity = %#v, want %#v", duplicate.RequestIdentity, first.RequestIdentity)
	}
	conflicting := command
	conflicting.Decision = execution.ConfirmationDecisionCancelled
	if _, err = service.Continue(context.Background(), conflicting); !core.IsErrorCode(err, core.ErrorCodeConflict) {
		t.Fatalf("conflicting Continue() error = %v, want conflict", err)
	}
	if router.calls != 1 {
		t.Fatalf("Router calls = %d, want 1", router.calls)
	}
}

func TestServiceContinueCancellationIsTerminalWithoutProviderOutput(t *testing.T) {
	store, command := newRuntimeConfirmationMemoryStore()
	command.Decision = execution.ConfirmationDecisionCancelled
	decisionID := uuid.New()
	router := &runtimeConfirmationRouterSpy{result: execution.ExecutionResult{
		State: execution.ExecutionStateCancelled, DecisionID: decisionID,
	}}
	service := mustRuntimeConfirmationService(t, store, router)

	result, err := service.Continue(context.Background(), command)
	if err != nil {
		t.Fatalf("Continue(cancel) error = %v", err)
	}
	if result.State != StateCancelled || result.DecisionID != decisionID || result.ErrorCode != "" {
		t.Fatalf("Continue(cancel) = %#v", result)
	}
	if store.continuation.State != execution.ConfirmationContinuationCancelled {
		t.Fatalf("stored state = %q", store.continuation.State)
	}
}

func TestServiceContinueCancellationFailureRemainsPending(t *testing.T) {
	store, command := newRuntimeConfirmationMemoryStore()
	command.Decision = execution.ConfirmationDecisionCancelled
	router := &runtimeConfirmationRouterSpy{
		err: core.NewError(core.ErrorCodeTargetNotReady, false, nil),
	}
	service := mustRuntimeConfirmationService(t, store, router)

	if _, err := service.Continue(context.Background(), command); !core.IsErrorCode(err, core.ErrorCodeTargetNotReady) {
		t.Fatalf("Continue(cancel) error = %v, want target not ready", err)
	}
	if store.continuation.State != execution.ConfirmationContinuationPending || store.recordCalls != 0 {
		t.Fatalf("continuation after failed cancellation = %#v", store.continuation)
	}
}

func TestServiceContinueNeverStoresRouterIDsWithoutDurableAuditEvidence(t *testing.T) {
	store, command := newRuntimeConfirmationMemoryStore()
	decisionID, outcomeID := uuid.New(), uuid.New()
	router := &runtimeConfirmationRouterSpy{
		result: completedExecutionResult(decisionID, outcomeID, "provider-value"),
	}
	recoverer := &runtimeConfirmationTerminalRecovererSpy{
		err: core.NewError(core.ErrorCodeNotFound, false, nil),
	}
	service, err := NewService(store, router, recoverer)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	if _, err = service.Continue(context.Background(), command); !core.IsErrorCode(err, core.ErrorCodeNotFound) {
		t.Fatalf("Continue() error = %v, want missing durable evidence", err)
	}
	if store.continuation.State != execution.ConfirmationContinuationPending || store.recordCalls != 0 ||
		store.continuation.DecisionID != uuid.Nil || store.continuation.OutcomeID != uuid.Nil {
		t.Fatalf("unverified terminal IDs were stored: %#v", store.continuation)
	}
}

func TestServiceContinueProjectsProviderFailedAndUnknownWithoutValues(t *testing.T) {
	tests := []struct {
		name          string
		status        core.OutcomeStatus
		routeErr      error
		wantState     State
		wantErrorCode core.ErrorCode
		wantStored    execution.ConfirmationContinuationState
	}{
		{
			name: "failed", status: core.OutcomeStatusFailed,
			routeErr:  core.NewError(core.ErrorCodeProviderUnavailable, true, nil),
			wantState: StateFailed, wantErrorCode: core.ErrorCodeProviderUnavailable,
			wantStored: execution.ConfirmationContinuationFailed,
		},
		{
			name: "unknown", status: core.OutcomeStatusUnknown,
			routeErr:  core.NewError(core.ErrorCodeOutcomeUnknown, false, nil),
			wantState: StateUnknown, wantErrorCode: core.ErrorCodeOutcomeUnknown,
			wantStored: execution.ConfirmationContinuationUnknown,
		},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			store, command := newRuntimeConfirmationMemoryStore()
			decisionID, outcomeID := uuid.New(), uuid.New()
			router := &runtimeConfirmationRouterSpy{
				result: completedExecutionResult(decisionID, outcomeID, runtimeConfirmationSecretSentinel),
				err:    testCase.routeErr,
			}
			router.result.Outcome.Status = testCase.status
			service := mustRuntimeConfirmationService(t, store, router)

			result, err := service.Continue(context.Background(), command)
			if err != nil {
				t.Fatalf("Continue() error = %v", err)
			}
			if result.State != testCase.wantState || result.ErrorCode != testCase.wantErrorCode ||
				result.DecisionID != decisionID {
				t.Fatalf("Continue() = %#v", result)
			}
			if store.continuation.State != testCase.wantStored ||
				store.continuation.DiagnosticCode != string(testCase.wantErrorCode) {
				t.Fatalf("stored continuation = %#v", store.continuation)
			}
		})
	}
}

func TestServiceContinuePreProviderFailureBecomesIdempotentTerminal(t *testing.T) {
	store, command := newRuntimeConfirmationMemoryStore()
	router := &runtimeConfirmationRouterSpy{
		err: core.NewError(core.ErrorCodeConnectionNotReady, false, nil),
	}
	service := mustRuntimeConfirmationService(t, store, router)

	first, err := service.Continue(context.Background(), command)
	if err != nil || first.State != StateFailed || first.ErrorCode != core.ErrorCodeConnectionNotReady {
		t.Fatalf("first Continue() = %#v, %v", first, err)
	}
	second, err := service.Continue(context.Background(), command)
	if err != nil || second.State != StateFailed || second.ErrorCode != core.ErrorCodeConnectionNotReady {
		t.Fatalf("second Continue() = %#v, %v", second, err)
	}
	if router.calls != 1 {
		t.Fatalf("Router calls = %d, want 1", router.calls)
	}
}

func TestServiceContinueRecoversDurableOutcomeAfterTerminalWriteFailureWithoutResubmission(t *testing.T) {
	store, command := newRuntimeConfirmationMemoryStore()
	store.recordErrorOnce = core.NewError(core.ErrorCodeInternal, false, nil)
	decisionID, outcomeID := uuid.New(), uuid.New()
	terminal := execution.ConfirmationTerminal{
		State: execution.ConfirmationContinuationCompleted, DecisionID: decisionID, OutcomeID: outcomeID,
	}
	router := &runtimeConfirmationRecoveryRouter{
		result: completedExecutionResult(decisionID, outcomeID, "provider-value"),
	}
	recoverer := &runtimeConfirmationTerminalRecovererSpy{terminal: terminal}
	service, err := NewService(store, router, recoverer)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	if _, err = service.Continue(context.Background(), command); !core.IsErrorCode(err, core.ErrorCodeInternal) {
		t.Fatalf("first Continue() error = %v, want terminal persistence failure", err)
	}
	if store.continuation.State != execution.ConfirmationContinuationPending {
		t.Fatalf("continuation after terminal failure = %#v", store.continuation)
	}

	result, err := service.Continue(context.Background(), command)
	if err != nil {
		t.Fatalf("recovered Continue() error = %v", err)
	}
	if result.State != StateCompleted || result.DecisionID != decisionID || result.ExecutionResult != nil {
		t.Fatalf("recovered Continue() = %#v", result)
	}
	if store.continuation.State != execution.ConfirmationContinuationCompleted ||
		store.continuation.DecisionID != decisionID || store.continuation.OutcomeID != outcomeID {
		t.Fatalf("recovered continuation = %#v", store.continuation)
	}
	if router.providerCalls.Load() != 1 || router.routeCalls.Load() != 2 {
		t.Fatalf("provider/router calls = %d/%d, want 1/2", router.providerCalls.Load(), router.routeCalls.Load())
	}
	if recoverer.calls != 2 || len(recoverer.requests) != 2 {
		t.Fatalf("recoverer calls/requests = %d/%d", recoverer.calls, len(recoverer.requests))
	}
	recoveryRequest := recoverer.requests[1]
	frozenRequest := store.continuation.Frozen.Request
	if recoveryRequest.OrganizationID != frozenRequest.OrganizationID ||
		recoveryRequest.OwnerAccountID != frozenRequest.OwnerAccountID ||
		recoveryRequest.ActingPrincipalID != frozenRequest.ActingPrincipalID ||
		recoveryRequest.ConnectionID != frozenRequest.ConnectionID ||
		recoveryRequest.ConnectorID != frozenRequest.ConnectorID ||
		recoveryRequest.Target != frozenRequest.Target ||
		recoveryRequest.OperationID != frozenRequest.OperationID ||
		recoveryRequest.LogicalOperationID != frozenRequest.LogicalOperationID ||
		recoveryRequest.CorrelationID != frozenRequest.CorrelationID ||
		recoveryRequest.RouteReceipt != command.RouteReceipt ||
		recoveryRequest.Decision != command.Decision {
		t.Fatalf("recovery request did not preserve frozen identity: %#v", recoveryRequest)
	}
}

func TestServiceContinueOwnerMismatchNeverRoutes(t *testing.T) {
	store, command := newRuntimeConfirmationMemoryStore()
	router := &runtimeConfirmationRouterSpy{}
	service := mustRuntimeConfirmationService(t, store, router)
	command.OwnerAccountID = uuid.New()

	if _, err := service.Continue(context.Background(), command); !core.IsErrorCode(err, core.ErrorCodeAccessDenied) {
		t.Fatalf("Continue(owner mismatch) error = %v, want access denied", err)
	}
	if router.calls != 0 || store.recordCalls != 0 {
		t.Fatalf("owner mismatch routed=%d recorded=%d", router.calls, store.recordCalls)
	}
}

func TestServiceContinueConcurrentConfirmationCallsProviderAtMostOnce(t *testing.T) {
	store, command := newRuntimeConfirmationMemoryStore()
	store.waitForInitialLoads = true
	decisionID, outcomeID := uuid.New(), uuid.New()
	router := &runtimeConfirmationCASRouter{
		store:  store,
		result: completedExecutionResult(decisionID, outcomeID, "provider-value"),
	}
	service := mustRuntimeConfirmationService(t, store, router)

	type callResult struct {
		result Result
		err    error
	}
	results := make(chan callResult, 2)
	for range 2 {
		go func() {
			result, err := service.Continue(context.Background(), command)
			results <- callResult{result: result, err: err}
		}()
	}
	for range 2 {
		call := <-results
		if call.err != nil || call.result.State != StateCompleted || call.result.DecisionID != decisionID {
			t.Fatalf("concurrent Continue() = %#v, %v", call.result, call.err)
		}
	}
	if router.providerCalls.Load() != 1 {
		t.Fatalf("provider calls = %d, want 1", router.providerCalls.Load())
	}
	if router.routeCalls.Load() != 2 {
		t.Fatalf("Router calls = %d, want both concurrent requests to reach CAS", router.routeCalls.Load())
	}
}

func TestServiceContinueConcurrentConflictingDecisionsHaveOneWinner(t *testing.T) {
	store, confirmed := newRuntimeConfirmationMemoryStore()
	store.waitForInitialLoads = true
	cancelled := confirmed
	cancelled.Decision = execution.ConfirmationDecisionCancelled
	confirmedDecisionID, outcomeID := uuid.New(), uuid.New()
	cancelledDecisionID := uuid.New()
	router := &runtimeConfirmationCASRouter{
		store:               store,
		result:              completedExecutionResult(confirmedDecisionID, outcomeID, "provider-value"),
		cancelledDecisionID: cancelledDecisionID,
	}
	service := mustRuntimeConfirmationService(t, store, router)

	type callResult struct {
		decision execution.ConfirmationDecision
		result   Result
		err      error
	}
	results := make(chan callResult, 2)
	for _, command := range []Command{confirmed, cancelled} {
		command := command
		go func() {
			result, err := service.Continue(context.Background(), command)
			results <- callResult{decision: command.Decision, result: result, err: err}
		}()
	}
	winners, conflicts := 0, 0
	for range 2 {
		call := <-results
		if call.err != nil {
			if !core.IsErrorCode(call.err, core.ErrorCodeConflict) {
				t.Fatalf("concurrent %q error = %v", call.decision, call.err)
			}
			conflicts++
			continue
		}
		winners++
		if call.decision == execution.ConfirmationDecisionConfirmed && call.result.State != StateCompleted {
			t.Fatalf("confirmed winner = %#v", call.result)
		}
		if call.decision == execution.ConfirmationDecisionCancelled && call.result.State != StateCancelled {
			t.Fatalf("cancelled winner = %#v", call.result)
		}
	}
	if winners != 1 || conflicts != 1 {
		t.Fatalf("winners=%d conflicts=%d", winners, conflicts)
	}
	if router.providerCalls.Load() > 1 {
		t.Fatalf("provider calls = %d, want at most 1", router.providerCalls.Load())
	}
	if store.continuation.State == execution.ConfirmationContinuationPending {
		t.Fatalf("continuation remained pending: %#v", store.continuation)
	}
}

func TestServiceContinueRejectsInvalidClientCommandBeforeStore(t *testing.T) {
	store, command := newRuntimeConfirmationMemoryStore()
	router := &runtimeConfirmationRouterSpy{}
	service := mustRuntimeConfirmationService(t, store, router)
	tests := []Command{
		{},
		{
			OrganizationID: command.OrganizationID, OwnerAccountID: command.OwnerAccountID,
			RouteReceipt: command.RouteReceipt, Decision: execution.ConfirmationDecision("remember"),
		},
		{
			OrganizationID: command.OrganizationID, OwnerAccountID: command.OwnerAccountID,
			RouteReceipt: execution.RouteClaimReceipt("forged"), Decision: command.Decision,
		},
	}
	for _, invalid := range tests {
		if _, err := service.Continue(context.Background(), invalid); !core.IsErrorCode(err, core.ErrorCodeInvalidInput) {
			t.Fatalf("Continue(%#v) error = %v, want invalid input", invalid, err)
		}
	}
	if store.loadCalls != 0 || router.calls != 0 {
		t.Fatalf("invalid commands loaded=%d routed=%d", store.loadCalls, router.calls)
	}
}

func completedExecutionResult(decisionID, outcomeID uuid.UUID, value any) execution.ExecutionResult {
	return execution.ExecutionResult{
		State:      execution.ExecutionStateCompleted,
		DecisionID: decisionID,
		Outcome: core.OperationOutcome{
			ID: outcomeID, DecisionID: decisionID, Status: core.OutcomeStatusSuccessful,
		},
		Value: value,
	}
}

func mustRuntimeConfirmationService(
	t *testing.T,
	store execution.ConfirmationContinuationStore,
	router ExecutionRouter,
) *Service {
	t.Helper()
	recoverer, ok := router.(TerminalRecoverer)
	if !ok {
		t.Fatalf("test router %T does not provide durable terminal evidence", router)
	}
	service, err := NewService(store, router, recoverer)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	return service
}

type runtimeConfirmationMemoryStore struct {
	mu                  sync.Mutex
	organizationID      uuid.UUID
	ownerAccountID      uuid.UUID
	receipt             execution.RouteClaimReceipt
	continuation        execution.ConfirmationContinuation
	loadCalls           int
	recordCalls         int
	waitForInitialLoads bool
	initialLoads        int
	initialLoadsReady   chan struct{}
	initialLoadsOnce    sync.Once
	terminalReady       chan struct{}
	terminalOnce        sync.Once
	recordErrorOnce     error
}

func newRuntimeConfirmationMemoryStore() (*runtimeConfirmationMemoryStore, Command) {
	organizationID, ownerID := uuid.New(), uuid.New()
	receipt := execution.RouteClaimReceipt(uuid.New().String())
	target := core.TargetRef{Type: core.TargetTypeChat, ID: "conversation-1"}
	frozen := execution.FrozenConfirmationRequest{
		RouteReceipt: receipt,
		Request: execution.ExecutionRequest{
			OrganizationID: organizationID, OwnerAccountID: ownerID, ActingPrincipalID: ownerID,
			BindingID: uuid.New(), ConnectionID: uuid.New(), ConnectorID: core.Identifier("github"),
			Target: target, OperationID: core.Identifier("create_issue"),
			LogicalOperationID: "logical-operation-1", CorrelationID: "correlation-1",
			Interactive: true, Input: map[string]any{"query": runtimeConfirmationSecretSentinel},
		},
		Origin: execution.ConfirmationOrigin{
			Surface: core.TargetTypeChat, ConversationID: target.ID, MessageID: "message-1",
		},
		AcceptedOperation: runtimeConfirmationAcceptedOperation(),
	}
	store := &runtimeConfirmationMemoryStore{
		organizationID: organizationID, ownerAccountID: ownerID, receipt: receipt,
		continuation: execution.ConfirmationContinuation{
			Frozen: frozen, State: execution.ConfirmationContinuationPending,
			CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
		},
		initialLoadsReady: make(chan struct{}), terminalReady: make(chan struct{}),
	}
	return store, Command{
		OrganizationID: organizationID, OwnerAccountID: ownerID, RouteReceipt: receipt,
		Decision: execution.ConfirmationDecisionConfirmed,
	}
}

func runtimeConfirmationAcceptedOperation() catalog.PinnedOperation {
	definition := catalog.Definition{
		ID: "github", PluginID: "github", PluginVersion: "1.0.0",
		SourceDialect: catalog.SourceDialectAgentPluginsV1, SourceSchemaVersion: catalog.SourceSchemaVersion1,
		ProviderID: "github", DriverID: "github_api", AuthMethodID: "personal_access_token",
		Name: "GitHub", Description: "GitHub Connector", Classification: catalog.ClassificationConnector,
		CapabilityTypes: []catalog.CapabilityType{catalog.CapabilityTypeAction},
		SetupMethod:     catalog.SetupMethodKey, Ownership: catalog.OwnershipPersonal,
		Surfaces:   catalog.FirstReleaseSurfaces(),
		KeyFields:  []catalog.KeyField{{Name: "token", Label: "Token", Required: true, Secret: true}},
		Operations: []catalog.Operation{{ID: "create_issue", ToolName: "create_issue", Name: "Create issue"}},
	}
	digest, err := catalog.DefinitionContractDigest(definition)
	if err != nil {
		panic(err)
	}
	return catalog.PinnedOperation{
		ConnectorID: definition.ID, PluginID: definition.PluginID, PluginVersion: definition.PluginVersion,
		ContractDigest: digest, Definition: definition, Operation: definition.Operations[0],
	}
}

func (store *runtimeConfirmationMemoryStore) Freeze(
	context.Context,
	execution.FrozenConfirmationRequest,
) (execution.ConfirmationContinuation, error) {
	return execution.ConfirmationContinuation{}, core.NewError(core.ErrorCodeInternal, false, nil)
}

func (store *runtimeConfirmationMemoryStore) Load(
	_ context.Context,
	organizationID, ownerAccountID uuid.UUID,
	receipt execution.RouteClaimReceipt,
) (execution.ConfirmationContinuation, error) {
	store.mu.Lock()
	store.loadCalls++
	if organizationID != store.organizationID || ownerAccountID != store.ownerAccountID || receipt != store.receipt {
		store.mu.Unlock()
		return execution.ConfirmationContinuation{}, core.NewError(core.ErrorCodeAccessDenied, false, nil)
	}
	wait := store.waitForInitialLoads && store.continuation.State == execution.ConfirmationContinuationPending
	if wait {
		store.initialLoads++
		if store.initialLoads >= 2 {
			store.initialLoadsOnce.Do(func() { close(store.initialLoadsReady) })
		}
	}
	continuation := store.continuation
	store.mu.Unlock()
	if wait {
		<-store.initialLoadsReady
	}
	return continuation, nil
}

func (store *runtimeConfirmationMemoryStore) RecordTerminal(
	_ context.Context,
	organizationID, ownerAccountID uuid.UUID,
	receipt execution.RouteClaimReceipt,
	terminal execution.ConfirmationTerminal,
) (execution.ConfirmationContinuation, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.recordCalls++
	if organizationID != store.organizationID || ownerAccountID != store.ownerAccountID || receipt != store.receipt {
		return execution.ConfirmationContinuation{}, core.NewError(core.ErrorCodeAccessDenied, false, nil)
	}
	if store.recordErrorOnce != nil {
		err := store.recordErrorOnce
		store.recordErrorOnce = nil
		return execution.ConfirmationContinuation{}, err
	}
	if store.continuation.State != execution.ConfirmationContinuationPending {
		if store.continuation.State == terminal.State && store.continuation.DecisionID == terminal.DecisionID &&
			store.continuation.OutcomeID == terminal.OutcomeID &&
			store.continuation.DiagnosticCode == terminal.DiagnosticCode {
			return store.continuation, nil
		}
		return execution.ConfirmationContinuation{}, core.NewError(core.ErrorCodeConflict, false, nil)
	}
	store.continuation.State = terminal.State
	store.continuation.DecisionID = terminal.DecisionID
	store.continuation.OutcomeID = terminal.OutcomeID
	store.continuation.DiagnosticCode = terminal.DiagnosticCode
	store.continuation.UpdatedAt = time.Now().UTC()
	store.continuation.TerminalAt = store.continuation.UpdatedAt
	store.terminalOnce.Do(func() { close(store.terminalReady) })
	return store.continuation, nil
}

type runtimeConfirmationRouterSpy struct {
	mu       sync.Mutex
	result   execution.ExecutionResult
	err      error
	calls    int
	requests []execution.ExecutionRequest
}

func (router *runtimeConfirmationRouterSpy) Route(
	_ context.Context,
	request execution.ExecutionRequest,
) (execution.ExecutionResult, error) {
	router.mu.Lock()
	defer router.mu.Unlock()
	router.calls++
	router.requests = append(router.requests, request)
	return router.result, router.err
}

func (router *runtimeConfirmationRouterSpy) RecoverTerminal(
	_ context.Context,
	_ TerminalRecoveryRequest,
) (execution.ConfirmationTerminal, error) {
	router.mu.Lock()
	defer router.mu.Unlock()
	return terminalFromExecution(router.result, router.err), nil
}

type runtimeConfirmationCASRouter struct {
	store               *runtimeConfirmationMemoryStore
	result              execution.ExecutionResult
	cancelledDecisionID uuid.UUID
	claimed             atomic.Bool
	routeCalls          atomic.Int32
	providerCalls       atomic.Int32
}

type runtimeConfirmationRecoveryRouter struct {
	result        execution.ExecutionResult
	claimed       atomic.Bool
	routeCalls    atomic.Int32
	providerCalls atomic.Int32
}

func (router *runtimeConfirmationRecoveryRouter) Route(
	_ context.Context,
	_ execution.ExecutionRequest,
) (execution.ExecutionResult, error) {
	router.routeCalls.Add(1)
	if router.claimed.CompareAndSwap(false, true) {
		router.providerCalls.Add(1)
		return router.result, nil
	}
	return execution.ExecutionResult{}, core.NewError(core.ErrorCodeConflict, false, nil)
}

func (router *runtimeConfirmationRecoveryRouter) RecoverTerminal(
	_ context.Context,
	_ TerminalRecoveryRequest,
) (execution.ConfirmationTerminal, error) {
	return terminalFromExecution(router.result, nil), nil
}

type runtimeConfirmationTerminalRecovererSpy struct {
	terminal execution.ConfirmationTerminal
	err      error
	calls    int
	requests []TerminalRecoveryRequest
}

func (recoverer *runtimeConfirmationTerminalRecovererSpy) RecoverTerminal(
	_ context.Context,
	request TerminalRecoveryRequest,
) (execution.ConfirmationTerminal, error) {
	recoverer.calls++
	recoverer.requests = append(recoverer.requests, request)
	return recoverer.terminal, recoverer.err
}

func (router *runtimeConfirmationCASRouter) Route(
	_ context.Context,
	request execution.ExecutionRequest,
) (execution.ExecutionResult, error) {
	router.routeCalls.Add(1)
	if router.claimed.CompareAndSwap(false, true) {
		if request.Confirmation != nil &&
			request.Confirmation.Decision == execution.ConfirmationDecisionCancelled {
			decisionID := router.cancelledDecisionID
			if decisionID == uuid.Nil {
				decisionID = uuid.New()
			}
			return execution.ExecutionResult{
				State: execution.ExecutionStateCancelled, DecisionID: decisionID,
			}, nil
		}
		router.providerCalls.Add(1)
		return router.result, nil
	}
	<-router.store.terminalReady
	return execution.ExecutionResult{}, core.NewError(core.ErrorCodeConflict, false, nil)
}

func (router *runtimeConfirmationCASRouter) RecoverTerminal(
	_ context.Context,
	request TerminalRecoveryRequest,
) (execution.ConfirmationTerminal, error) {
	if request.Decision == execution.ConfirmationDecisionCancelled {
		decisionID := router.cancelledDecisionID
		if decisionID == uuid.Nil {
			return execution.ConfirmationTerminal{}, core.NewError(core.ErrorCodeInternal, false, nil)
		}
		return execution.ConfirmationTerminal{
			State: execution.ConfirmationContinuationCancelled, DecisionID: decisionID,
		}, nil
	}
	return terminalFromExecution(router.result, nil), nil
}
