package execution

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
)

type memoryInvocationStore struct {
	claim   InvocationClaim
	receipt RouteClaimReceipt
	claims  int
}

func (store *memoryInvocationStore) Claim(_ context.Context, claim InvocationClaim) (RouteClaimReceipt, error) {
	store.claims++
	if store.receipt != "" {
		return "", connectorError(core.ErrorCodeConflict, false)
	}
	store.claim = claim
	store.receipt = RouteClaimReceipt(uuid.NewString())
	return store.receipt, nil
}
func (store *memoryInvocationStore) Resume(_ context.Context, claim InvocationClaim, receipt RouteClaimReceipt) error {
	if store.claim != claim || store.receipt != receipt {
		return connectorError(core.ErrorCodeConflict, false)
	}
	return nil
}

func TestNativeRouterNeverResubmitsUnknownInvocation(t *testing.T) {
	h := newExecutionHarness(t)
	h.provider.result = ProviderInvocationResult{Status: core.OutcomeStatusUnknown}
	store := &memoryInvocationStore{}
	router, _ := NewNativeRouter(store, h.service)
	result, _ := router.Route(context.Background(), h.pinned.Request)
	if result.Outcome.Status != core.OutcomeStatusUnknown || h.provider.calls != 1 {
		t.Fatal("unknown was not recorded")
	}
	_, err := router.Route(context.Background(), h.pinned.Request)
	if !core.IsErrorCode(err, core.ErrorCodeConflict) || h.provider.calls != 1 {
		t.Fatal("unknown was replayed")
	}
	if store.claim.Snapshot.BindingRevision != h.bindings.binding.Revision || len(store.claim.Snapshot.InputDigest) != 64 {
		t.Fatal("claim evidence missing")
	}
}

func TestNativeConfirmationRejectsChangedInputCredentialAndBinding(t *testing.T) {
	for _, change := range []string{"input", "credential", "binding"} {
		t.Run(change, func(t *testing.T) {
			h := newExecutionHarness(t)
			h.risk.classification = RiskHigh
			store := &memoryInvocationStore{}
			router, _ := NewNativeRouter(store, h.service)
			pending, err := router.Route(context.Background(), h.pinned.Request)
			if err != nil || pending.Confirmation == nil {
				t.Fatalf("no confirmation: %v", err)
			}
			resume := h.pinned.Request
			resume.AcceptedOperation = pending.Confirmation.AcceptedOperation
			resume.Confirmation = &RuntimeConfirmationCommand{RouteReceipt: store.receipt, Decision: ConfirmationDecisionConfirmed}
			switch change {
			case "input":
				resume.Input = map[string]any{"changed": true}
			case "credential":
				h.connections.connection.CredentialVersion++
			case "binding":
				h.bindings.binding.Revision++
			}
			_, err = router.Route(context.Background(), resume)
			if !core.IsErrorCode(err, core.ErrorCodeConflict) || h.provider.calls != 0 || store.claims != 1 {
				t.Fatalf("changed %s resumed: %v", change, err)
			}
		})
	}
}

func TestNativeExecutionDeniesMissingAndNarrowedConstraintsBeforeCredentials(t *testing.T) {
	for _, layer := range []string{"connection_missing", "binding_missing", "connection_denied", "binding_expired"} {
		t.Run(layer, func(t *testing.T) {
			h := newExecutionHarness(t)
			switch layer {
			case "connection_missing":
				h.connections.connection.Constraint = nil
			case "binding_missing":
				h.bindings.binding.Constraint = nil
			case "connection_denied":
				h.connections.connection.Constraint.DenyReasons = []string{"source_disabled"}
			case "binding_expired":
				now := h.service.now()
				h.bindings.binding.Constraint.ExpiresAt = &now
			}
			router, _ := NewNativeRouter(&memoryInvocationStore{}, h.service)
			_, err := router.Route(context.Background(), h.pinned.Request)
			if !core.IsErrorCode(err, core.ErrorCodeAccessDenied) || h.provider.calls != 0 || h.credentials.calls != 0 {
				t.Fatalf("constraint bypassed: %v", err)
			}
		})
	}
}
