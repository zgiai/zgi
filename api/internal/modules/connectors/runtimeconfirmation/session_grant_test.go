package runtimeconfirmation

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"github.com/zgiai/zgi/api/internal/modules/connectors/execution"
)

// delegatedRuntimeConfirmationStore lets a decider who is not the owner load
// the continuation by receipt, as the durable store does for a DigitalHuman
// run's requester.
type delegatedRuntimeConfirmationStore struct {
	*runtimeConfirmationMemoryStore
}

func (store delegatedRuntimeConfirmationStore) LoadByReceipt(
	_ context.Context,
	organizationID uuid.UUID,
	receipt execution.RouteClaimReceipt,
) (execution.ConfirmationContinuation, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	if organizationID != store.organizationID || receipt != store.receipt {
		return execution.ConfirmationContinuation{}, core.NewError(core.ErrorCodeNotFound, false, nil)
	}
	return store.continuation, nil
}

func TestServiceContinueDigitalHumanSessionGrantNamesReasonOnlyToRequester(t *testing.T) {
	store, command, host := newDigitalHumanRuntimeConfirmationMemoryStore()
	router := &runtimeConfirmationRouterSpy{}
	guard := &runtimeConfirmationDigitalHumanGuardSpy{host: host}
	service := mustRuntimeConfirmationService(t, delegatedRuntimeConfirmationStore{store}, router).
		WithDigitalHumanContinuationGuard(guard)
	command.OwnerAccountID = uuid.New()
	command.RememberForSession = true

	if _, err := service.Continue(context.Background(), command); !core.IsErrorCode(err, core.ErrorCodeSessionGrantUnsupported) {
		t.Fatalf("Continue(requester remember) error = %v, want session grant unsupported", err)
	}

	// A decider the run does not belong to learns nothing about its surface.
	guard.host.runID = uuid.New()
	if _, err := service.Continue(context.Background(), command); !core.IsErrorCode(err, core.ErrorCodeAccessDenied) {
		t.Fatalf("Continue(stranger remember) error = %v, want access denied", err)
	}
	if guard.calls != 0 || router.calls != 0 || store.recordCalls != 0 {
		t.Fatalf("remember reached guard=%d router=%d record=%d, want 0/0/0",
			guard.calls, router.calls, store.recordCalls)
	}
	if store.continuation.State != execution.ConfirmationContinuationPending {
		t.Fatalf("remember mutated continuation: %#v", store.continuation)
	}
}

func TestServiceContinueDigitalHumanConfirmsOnceAfterSessionGrantRejection(t *testing.T) {
	store, command, host := newDigitalHumanRuntimeConfirmationMemoryStore()
	decisionID, outcomeID := uuid.New(), uuid.New()
	router := &runtimeConfirmationRouterSpy{
		result: completedExecutionResult(decisionID, outcomeID, "provider-value"),
	}
	guard := &runtimeConfirmationDigitalHumanGuardSpy{host: host}
	service := mustRuntimeConfirmationService(t, store, router).
		WithDigitalHumanContinuationGuard(guard)

	command.RememberForSession = true
	if _, err := service.Continue(context.Background(), command); !core.IsErrorCode(err, core.ErrorCodeSessionGrantUnsupported) {
		t.Fatalf("Continue(remember) error = %v, want session grant unsupported", err)
	}
	command.RememberForSession = false
	result, err := service.Continue(context.Background(), command)
	if err != nil || result.State != StateCompleted || result.DecisionID != decisionID {
		t.Fatalf("Continue(one-shot retry) = %#v, %v", result, err)
	}
	if router.calls != 1 {
		t.Fatalf("router calls = %d, want 1", router.calls)
	}
}

func TestReadStatusMarksDigitalHumanConfirmationOneShot(t *testing.T) {
	store, command, _ := newDigitalHumanRuntimeConfirmationMemoryStore()
	service := mustRuntimeConfirmationService(t, store, &runtimeConfirmationRouterSpy{})
	query := StatusQuery{
		OrganizationID: command.OrganizationID, OwnerAccountID: command.OwnerAccountID,
		RouteReceipt: command.RouteReceipt, PresentationVersion: 1,
	}
	result, err := service.ReadStatus(t.Context(), query)
	if err != nil || result.Presentation == nil || !result.Presentation.OneShot {
		t.Fatalf("ReadStatus(DigitalHuman) = %#v, %v, want one-shot presentation", result, err)
	}

	for _, surface := range []core.TargetType{core.TargetTypeChat, core.TargetTypeAgent, core.TargetTypeWorkflow} {
		store, command := newRuntimeConfirmationMemoryStore()
		store.continuation.Frozen.Origin.Surface = surface
		service := mustRuntimeConfirmationService(t, store, &runtimeConfirmationRouterSpy{})
		query := StatusQuery{
			OrganizationID: command.OrganizationID, OwnerAccountID: command.OwnerAccountID,
			RouteReceipt: command.RouteReceipt, PresentationVersion: 1,
		}
		result, err := service.ReadStatus(t.Context(), query)
		if err != nil || result.Presentation == nil || result.Presentation.OneShot {
			t.Fatalf("ReadStatus(%s) = %#v, %v, want a session-capable presentation", surface, result, err)
		}
	}
}
