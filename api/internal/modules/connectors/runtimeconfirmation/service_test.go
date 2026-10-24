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
	if result.Presentation == nil || result.Presentation.InvocationID == "" || result.Presentation.DisplayStatus != "successful" {
		t.Fatalf("missing safe terminal presentation = %#v", result.Presentation)
	}
	if result.Authorization == nil || result.Authorization.ConnectionID != store.continuation.Frozen.Request.ConnectionID ||
		result.Authorization.BindingID != store.continuation.Frozen.Request.BindingID ||
		result.Authorization.OperationID != store.continuation.Frozen.Request.OperationID {
		t.Fatalf("authorization snapshot = %#v", result.Authorization)
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
	if duplicate.Presentation == nil || duplicate.Presentation.DisplayStatus != "successful" {
		t.Fatalf("duplicate lost terminal presentation = %#v", duplicate.Presentation)
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

// runtimeConfirmationDigitalHumanHost is the durable waiting-run expectation a
// test guard compares against the server-restored guard request.
type runtimeConfirmationDigitalHumanHost struct {
	organizationID uuid.UUID
	ownerAccountID uuid.UUID
	digitalHumanID uuid.UUID
	versionID      uuid.UUID
	runID          uuid.UUID
	conversationID uuid.UUID
	messageID      uuid.UUID
	receipt        execution.RouteClaimReceipt
	canceled       bool
}

type runtimeConfirmationDigitalHumanGuardSpy struct {
	mu       sync.Mutex
	host     runtimeConfirmationDigitalHumanHost
	calls    int
	requests []DigitalHumanContinuationGuardRequest
}

func (guard *runtimeConfirmationDigitalHumanGuardSpy) AuthorizeDecider(
	_ context.Context,
	request DigitalHumanContinuationGuardRequest,
) error {
	guard.mu.Lock()
	defer guard.mu.Unlock()
	guard.requests = append(guard.requests, request)
	if request.RunID != guard.host.runID || request.DeciderID == uuid.Nil {
		return core.NewError(core.ErrorCodeConflict, false, nil)
	}
	return nil
}

func (guard *runtimeConfirmationDigitalHumanGuardSpy) AuthorizeContinuation(
	_ context.Context,
	request DigitalHumanContinuationGuardRequest,
) error {
	guard.mu.Lock()
	defer guard.mu.Unlock()
	guard.calls++
	guard.requests = append(guard.requests, request)
	host := guard.host
	if host.canceled || request.OrganizationID != host.organizationID ||
		request.OwnerAccountID != host.ownerAccountID ||
		request.DigitalHumanID != host.digitalHumanID ||
		request.DigitalHumanVersionID != host.versionID ||
		request.RunID != host.runID ||
		request.ConversationID != host.conversationID ||
		request.MessageID != host.messageID ||
		request.RouteReceipt != host.receipt {
		return core.NewError(core.ErrorCodeConflict, false, nil)
	}
	return nil
}

func newDigitalHumanRuntimeConfirmationMemoryStore() (
	*runtimeConfirmationMemoryStore,
	Command,
	runtimeConfirmationDigitalHumanHost,
) {
	organizationID, ownerID := uuid.New(), uuid.New()
	receipt := execution.RouteClaimReceipt(uuid.New().String())
	digitalHumanID, versionID := uuid.New(), uuid.New()
	conversationID, messageID, runID := uuid.New(), uuid.New(), uuid.New()
	target := core.TargetRef{
		Type: core.TargetTypeDigitalHuman, ID: digitalHumanID.String(),
		Scope: core.ScopePublished, VersionID: versionID.String(),
		BindingInstanceID: uuid.NewString(),
	}
	frozen := execution.FrozenConfirmationRequest{
		RouteReceipt: receipt,
		Request: execution.ExecutionRequest{
			ConversationID: conversationID.String(),
			OrganizationID: organizationID, OwnerAccountID: ownerID, ActingPrincipalID: ownerID,
			BindingID: uuid.New(), ConnectionID: uuid.New(), ConnectorID: core.Identifier("github"),
			Target: target, OperationID: core.Identifier("create_issue"),
			LogicalOperationID: "logical-operation-1", CorrelationID: "connector-correlation-" + messageID.String(),
			Interactive: true, Input: map[string]any{"query": runtimeConfirmationSecretSentinel},
		},
		Origin: execution.ConfirmationOrigin{
			Surface: core.TargetTypeDigitalHuman, OriginVersion: 2,
			DigitalHumanID: digitalHumanID.String(), ConversationID: conversationID.String(),
			MessageID: messageID.String(), RunID: runID.String(),
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
		}, runtimeConfirmationDigitalHumanHost{
			organizationID: organizationID, ownerAccountID: ownerID,
			digitalHumanID: digitalHumanID, versionID: versionID, runID: runID,
			conversationID: conversationID, messageID: messageID, receipt: receipt,
		}
}

func TestServiceContinueDigitalHumanGuardAllowsExactWaitingRun(t *testing.T) {
	store, command, host := newDigitalHumanRuntimeConfirmationMemoryStore()
	decisionID, outcomeID := uuid.New(), uuid.New()
	router := &runtimeConfirmationRouterSpy{
		result: completedExecutionResult(decisionID, outcomeID, "provider-value"),
	}
	guard := &runtimeConfirmationDigitalHumanGuardSpy{host: host}
	service := mustRuntimeConfirmationService(t, store, router).
		WithDigitalHumanContinuationGuard(guard)

	result, err := service.Continue(context.Background(), command)
	if err != nil {
		t.Fatalf("Continue() error = %v", err)
	}
	if result.State != StateCompleted || result.DecisionID != decisionID {
		t.Fatalf("Continue() = %#v", result)
	}
	if guard.calls != 1 || router.calls != 1 {
		t.Fatalf("guard calls = %d, router calls = %d, want 1/1", guard.calls, router.calls)
	}
	request := guard.requests[0]
	if request.OrganizationID != host.organizationID || request.OwnerAccountID != host.ownerAccountID ||
		request.DigitalHumanID != host.digitalHumanID ||
		request.DigitalHumanVersionID != host.versionID || request.RunID != host.runID ||
		request.ConversationID != host.conversationID || request.MessageID != host.messageID ||
		request.RouteReceipt != host.receipt {
		t.Fatalf("guard request = %#v, want exact durable host identity", request)
	}
}

func TestServiceContinueDigitalHumanGuardDeniesStaleStateBeforeRouter(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*runtimeConfirmationDigitalHumanHost)
	}{
		{"canceled run", func(host *runtimeConfirmationDigitalHumanHost) { host.canceled = true }},
		{"wrong owner", func(host *runtimeConfirmationDigitalHumanHost) { host.ownerAccountID = uuid.New() }},
		{"wrong version", func(host *runtimeConfirmationDigitalHumanHost) { host.versionID = uuid.New() }},
		{"wrong receipt", func(host *runtimeConfirmationDigitalHumanHost) {
			host.receipt = execution.RouteClaimReceipt(uuid.New().String())
		}},
		{"wrong host", func(host *runtimeConfirmationDigitalHumanHost) { host.digitalHumanID = uuid.New() }},
		{"wrong message", func(host *runtimeConfirmationDigitalHumanHost) { host.messageID = uuid.New() }},
		{"wrong conversation", func(host *runtimeConfirmationDigitalHumanHost) { host.conversationID = uuid.New() }},
		{"wrong run", func(host *runtimeConfirmationDigitalHumanHost) { host.runID = uuid.New() }},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			store, command, host := newDigitalHumanRuntimeConfirmationMemoryStore()
			testCase.mutate(&host)
			router := &runtimeConfirmationRouterSpy{}
			guard := &runtimeConfirmationDigitalHumanGuardSpy{host: host}
			service := mustRuntimeConfirmationService(t, store, router).
				WithDigitalHumanContinuationGuard(guard)

			if _, err := service.Continue(context.Background(), command); !core.IsErrorCode(err, core.ErrorCodeConflict) {
				t.Fatalf("Continue() error = %v, want conflict", err)
			}
			if router.calls != 0 || store.recordCalls != 0 {
				t.Fatalf("%s routed=%d recorded=%d, want 0/0", testCase.name, router.calls, store.recordCalls)
			}
			if guard.calls != 1 {
				t.Fatalf("guard calls = %d, want 1", guard.calls)
			}
		})
	}
}

func TestServiceContinueDigitalHumanWithoutGuardFailsClosed(t *testing.T) {
	store, command, _ := newDigitalHumanRuntimeConfirmationMemoryStore()
	router := &runtimeConfirmationRouterSpy{}
	service := mustRuntimeConfirmationService(t, store, router)

	if _, err := service.Continue(context.Background(), command); !core.IsErrorCode(err, core.ErrorCodeInternal) {
		t.Fatalf("Continue(no guard) error = %v, want internal fail-closed", err)
	}
	if router.calls != 0 || store.recordCalls != 0 {
		t.Fatalf("unguarded DigitalHuman continuation routed=%d recorded=%d", router.calls, store.recordCalls)
	}
}

func TestServiceContinueDigitalHumanGuardDenialNeverWritesTerminal(t *testing.T) {
	store, command, host := newDigitalHumanRuntimeConfirmationMemoryStore()
	host.canceled = true
	router := &runtimeConfirmationRouterSpy{}
	guard := &runtimeConfirmationDigitalHumanGuardSpy{host: host}
	service := mustRuntimeConfirmationService(t, store, router).
		WithDigitalHumanContinuationGuard(guard)

	if _, err := service.Continue(context.Background(), command); !core.IsErrorCode(err, core.ErrorCodeConflict) {
		t.Fatalf("Continue() error = %v, want conflict", err)
	}
	if store.continuation.State != execution.ConfirmationContinuationPending || store.recordCalls != 0 {
		t.Fatalf("denied continuation was mutated: %#v", store.continuation)
	}
}

func TestServiceContinueNonDigitalHumanOriginsNeverConsultGuard(t *testing.T) {
	for _, surface := range []core.TargetType{
		core.TargetTypeChat, core.TargetTypeAgent, core.TargetTypeWorkflow,
	} {
		t.Run(string(surface), func(t *testing.T) {
			store, command := newRuntimeConfirmationMemoryStore()
			store.continuation.Frozen.Origin.Surface = surface
			decisionID, outcomeID := uuid.New(), uuid.New()
			router := &runtimeConfirmationRouterSpy{
				result: completedExecutionResult(decisionID, outcomeID, "provider-value"),
			}
			guard := &runtimeConfirmationDigitalHumanGuardSpy{}
			service := mustRuntimeConfirmationService(t, store, router).
				WithDigitalHumanContinuationGuard(guard)

			result, err := service.Continue(context.Background(), command)
			if err != nil || result.State != StateCompleted {
				t.Fatalf("Continue(%s) = %#v, %v", surface, result, err)
			}
			if guard.calls != 0 {
				t.Fatalf("%s origin consulted the DigitalHuman guard %d times", surface, guard.calls)
			}
			if router.calls != 1 {
				t.Fatalf("%s origin router calls = %d, want 1", surface, router.calls)
			}
		})
	}
}

func TestServiceContinueDigitalHumanRejectsRememberForSessionBeforeRouting(t *testing.T) {
	store, command, host := newDigitalHumanRuntimeConfirmationMemoryStore()
	command.RememberForSession = true
	router := &runtimeConfirmationRouterSpy{}
	guard := &runtimeConfirmationDigitalHumanGuardSpy{host: host}
	service := mustRuntimeConfirmationService(t, store, router).
		WithDigitalHumanContinuationGuard(guard)

	if _, err := service.Continue(context.Background(), command); !core.IsErrorCode(err, core.ErrorCodeAccessDenied) {
		t.Fatalf("Continue(DigitalHuman remember) error = %v, want access denied", err)
	}
	if guard.calls != 0 || router.calls != 0 || store.recordCalls != 0 {
		t.Fatalf(
			"forged remember reached guard=%d router=%d record=%d, want 0/0/0",
			guard.calls, router.calls, store.recordCalls,
		)
	}
	if store.continuation.State != execution.ConfirmationContinuationPending {
		t.Fatalf("forged remember mutated continuation: %#v", store.continuation)
	}
}

func TestServiceContinueDigitalHumanRejectsRememberForSessionInTerminalState(t *testing.T) {
	store, command, _ := newDigitalHumanRuntimeConfirmationMemoryStore()
	store.continuation.State = execution.ConfirmationContinuationCompleted
	store.continuation.DecisionID = uuid.New()
	store.continuation.OutcomeID = uuid.New()
	command.RememberForSession = true
	router := &runtimeConfirmationRouterSpy{}
	service := mustRuntimeConfirmationService(t, store, router)

	if _, err := service.Continue(context.Background(), command); !core.IsErrorCode(err, core.ErrorCodeAccessDenied) {
		t.Fatalf("Continue(DigitalHuman terminal remember) error = %v, want access denied", err)
	}
	if router.calls != 0 || store.recordCalls != 0 {
		t.Fatalf("terminal forged remember routed=%d recorded=%d, want 0/0", router.calls, store.recordCalls)
	}
}

func TestServiceContinueDigitalHumanOneShotStillRoutes(t *testing.T) {
	store, command, host := newDigitalHumanRuntimeConfirmationMemoryStore()
	command.RememberForSession = false
	decisionID, outcomeID := uuid.New(), uuid.New()
	router := &runtimeConfirmationRouterSpy{
		result: completedExecutionResult(decisionID, outcomeID, "provider-value"),
	}
	guard := &runtimeConfirmationDigitalHumanGuardSpy{host: host}
	service := mustRuntimeConfirmationService(t, store, router).
		WithDigitalHumanContinuationGuard(guard)

	result, err := service.Continue(context.Background(), command)
	if err != nil || result.State != StateCompleted || result.DecisionID != decisionID {
		t.Fatalf("Continue(DigitalHuman one-shot) = %#v, %v", result, err)
	}
	if guard.calls != 1 || router.calls != 1 {
		t.Fatalf("guard/router calls = %d/%d, want 1/1", guard.calls, router.calls)
	}
}

func TestServiceContinueNonDigitalHumanRememberForSessionStillRoutes(t *testing.T) {
	for _, surface := range []core.TargetType{
		core.TargetTypeChat, core.TargetTypeAgent, core.TargetTypeWorkflow,
	} {
		t.Run(string(surface), func(t *testing.T) {
			store, command := newRuntimeConfirmationMemoryStore()
			store.continuation.Frozen.Origin.Surface = surface
			command.RememberForSession = true
			decisionID, outcomeID := uuid.New(), uuid.New()
			router := &runtimeConfirmationRouterSpy{
				result: completedExecutionResult(decisionID, outcomeID, "provider-value"),
			}
			service := mustRuntimeConfirmationService(t, store, router)

			result, err := service.Continue(context.Background(), command)
			if err != nil || result.State != StateCompleted || result.DecisionID != decisionID {
				t.Fatalf("Continue(%s remember) = %#v, %v", surface, result, err)
			}
			if router.calls != 1 {
				t.Fatalf("%s remember router calls = %d, want 1", surface, router.calls)
			}
		})
	}
}

func TestFallbackPresentationIsSafeLocalizedAndNotRedacted(t *testing.T) {
	request := execution.ExecutionRequest{
		ConnectorID:        core.Identifier("github"),
		OperationID:        core.Identifier("create_issue"),
		LogicalOperationID: "logical-operation-1",
	}
	operation := runtimeConfirmationAcceptedOperation()
	operation.Operation.NameI18n = catalog.LocalizedText{"en-US": "Create issue", "zh-Hans": "创建 Issue"}

	en := fallbackPresentation(execution.WithPresentationLocale(context.Background(), "en-US"), request, operation, StateCompleted, "")
	if en.Redacted || en.DisplayStatus != "successful" || en.Title != "Create issue" ||
		en.Summary != "The Connector operation completed successfully." {
		t.Fatalf("en completed fallback = %#v", en)
	}

	zh := fallbackPresentation(execution.WithPresentationLocale(context.Background(), "zh-Hans"), request, operation, StateCompleted, "")
	if zh.Redacted || zh.Title != "创建 Issue" || zh.Summary != "连接器操作已成功完成。" {
		t.Fatalf("zh completed fallback = %#v", zh)
	}

	failed := fallbackPresentation(execution.WithPresentationLocale(context.Background(), "zh-Hans"), request, operation, StateFailed, core.ErrorCodeMailSubmissionRejected)
	if failed.Redacted || failed.DisplayStatus != "failed" ||
		failed.ErrorCode != string(core.ErrorCodeMailSubmissionRejected) ||
		failed.Summary != "邮件服务器拒绝了本次提交，请检查邮件服务状态后重试。" {
		t.Fatalf("failed fallback = %#v", failed)
	}

	unknown := fallbackPresentation(context.Background(), request, operation, StateUnknown, core.ErrorCodeOutcomeUnknown)
	if unknown.Redacted || unknown.DisplayStatus != "unknown" ||
		unknown.ErrorCode != string(core.ErrorCodeOutcomeUnknown) || unknown.Summary == "" {
		t.Fatalf("unknown fallback = %#v", unknown)
	}

	cancelled := fallbackPresentation(context.Background(), request, operation, StateCancelled, "")
	if cancelled.Redacted || cancelled.DisplayStatus != "cancelled" || cancelled.Summary != "" {
		t.Fatalf("cancelled fallback = %#v", cancelled)
	}
}
