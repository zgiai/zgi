package dispatch

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"github.com/zgiai/zgi/api/internal/modules/connectors/execution"
)

type routerSpy struct {
	calls   int
	request execution.ExecutionRequest
	result  execution.ExecutionResult
	err     error
}

func (spy *routerSpy) Route(_ context.Context, request execution.ExecutionRequest) (execution.ExecutionResult, error) {
	spy.calls++
	spy.request = request
	return spy.result, spy.err
}

func TestAdapterDispatchesEachSurfaceToRouterExactlyOnce(t *testing.T) {
	t.Parallel()

	for _, target := range []core.TargetRef{
		{Type: core.TargetTypeChat, ID: uuid.NewString()},
		{Type: core.TargetTypeAgent, ID: uuid.NewString()},
		{Type: core.TargetTypeWorkflow, ID: uuid.NewString(), BindingInstanceID: "tool-node"},
	} {
		target := target
		t.Run(string(target.Type), func(t *testing.T) {
			t.Parallel()
			spy := &routerSpy{result: execution.ExecutionResult{State: execution.ExecutionStateCompleted}}
			adapter, err := New(spy)
			if err != nil {
				t.Fatal(err)
			}
			identity := validIdentity(target)
			input := map[string]interface{}{"query": "bounded"}

			result, err := adapter.Dispatch(context.Background(), Invocation{Identity: identity, Input: input})
			if err != nil || result.State != execution.ExecutionStateCompleted {
				t.Fatalf("Dispatch() = (%#v, %v)", result, err)
			}
			if spy.calls != 1 {
				t.Fatalf("Router calls = %d, want 1", spy.calls)
			}
			if spy.request.OrganizationID != identity.OrganizationID ||
				spy.request.OwnerAccountID != identity.OwnerAccountID ||
				spy.request.ActingPrincipalID != identity.ActingPrincipalID ||
				spy.request.BindingID != identity.BindingID ||
				spy.request.ConnectionID != identity.ConnectionID ||
				spy.request.ConnectorID != identity.ConnectorID ||
				spy.request.Target != identity.Target ||
				spy.request.OperationID != identity.OperationID ||
				spy.request.LogicalOperationID != identity.LogicalOperationID ||
				spy.request.CorrelationID != identity.CorrelationID ||
				spy.request.Interactive != identity.Interactive ||
				!reflect.DeepEqual(spy.request.Input, input) {
				t.Fatalf("Router request = %#v, want identity %#v", spy.request, identity)
			}
		})
	}
}

func TestAdapterFailsClosedWithoutRetryOrFallback(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("router unavailable")
	spy := &routerSpy{err: wantErr}
	adapter, err := New(spy)
	if err != nil {
		t.Fatal(err)
	}
	_, err = adapter.Dispatch(context.Background(), Invocation{
		Identity: validIdentity(core.TargetRef{Type: core.TargetTypeChat, ID: uuid.NewString()}),
	})
	if !errors.Is(err, wantErr) || spy.calls != 1 {
		t.Fatalf("Dispatch() error = %v, Router calls = %d", err, spy.calls)
	}
}

func TestAdapterRejectsInvalidIdentityBeforeRouter(t *testing.T) {
	t.Parallel()

	spy := &routerSpy{}
	adapter, err := New(spy)
	if err != nil {
		t.Fatal(err)
	}
	identity := validIdentity(core.TargetRef{Type: core.TargetTypeAgent, ID: uuid.NewString()})
	identity.ActingPrincipalID = uuid.New()
	_, err = adapter.Dispatch(context.Background(), Invocation{Identity: identity})
	if !core.IsErrorCode(err, core.ErrorCodeInvalidInput) || spy.calls != 0 {
		t.Fatalf("Dispatch() error = %v, Router calls = %d", err, spy.calls)
	}
}

func TestIdentityContextDefensivelyCopiesConfirmation(t *testing.T) {
	t.Parallel()

	identity := validIdentity(core.TargetRef{Type: core.TargetTypeChat, ID: uuid.NewString()})
	identity.Confirmation = &execution.RuntimeConfirmationCommand{
		RouteReceipt: "route-receipt", Decision: execution.ConfirmationDecisionConfirmed,
	}
	ctx := WithIdentity(context.Background(), identity)
	first, ok := IdentityFromContext(ctx)
	if !ok {
		t.Fatal("IdentityFromContext() did not find identity")
	}
	first.Confirmation.Decision = execution.ConfirmationDecisionCancelled
	second, ok := IdentityFromContext(ctx)
	if !ok || second.Confirmation == nil || second.Confirmation.Decision != execution.ConfirmationDecisionConfirmed {
		t.Fatalf("stored confirmation mutated: %#v", second.Confirmation)
	}
}

func validIdentity(target core.TargetRef) Identity {
	ownerID := uuid.New()
	return Identity{
		OrganizationID: uuid.New(), OwnerAccountID: ownerID, ActingPrincipalID: ownerID,
		BindingID: uuid.New(), ConnectionID: uuid.New(), ConnectorID: core.Identifier("github"),
		Target: target, OperationID: core.Identifier("github.user.get"),
		LogicalOperationID: uuid.NewString(), CorrelationID: uuid.NewString(), Interactive: true,
	}
}
