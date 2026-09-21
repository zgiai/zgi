package execution

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/catalog"
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

// publishedDigitalHumanTarget is the most restrictive DigitalHuman host
// identity: a published version pinned by an exact UUID plus its binding
// instance. A historical OwnerConstraint never carried this boundary.
func publishedDigitalHumanTarget() core.TargetRef {
	return core.TargetRef{
		Type: core.TargetTypeDigitalHuman, ID: uuid.NewString(), Scope: core.ScopePublished,
		VersionID: uuid.NewString(), BindingInstanceID: uuid.NewString(),
	}
}

// digitalHumanExactBindingConstraint copies the owner and operation
// restrictions of a historical OwnerConstraint and replaces only the target
// boundary with the exact published host identity. It models an owner-created
// DigitalHuman Target Binding.
func digitalHumanExactBindingConstraint(owner uuid.UUID, operation core.Identifier, target core.TargetRef) *core.AccessConstraint {
	constraint := core.OwnerConstraint(owner, []core.Identifier{operation})
	constraint.TargetBoundaries = []core.TargetBoundary{{
		Type: target.Type, ID: target.ID, Scope: string(target.Scope), VersionID: target.VersionID,
	}}
	return constraint
}

// newDigitalHumanExecutionHarness prepares an owner-scoped published
// DigitalHuman request whose binding pins the exact host while the Connection
// still carries the historical OwnerConstraint without a DigitalHuman
// boundary. The catalog holds an admitted V4 DigitalHuman definition so the
// only remaining gate is the surface eligibility check in authorizeDefinition.
func newDigitalHumanExecutionHarness(t *testing.T) (*executionHarness, ExecutionRequest) {
	t.Helper()
	request := validExecutionRequest()
	request.Target = publishedDigitalHumanTarget()
	harness := newExecutionHarnessForRequest(t, request)
	harness.bindings.binding.Constraint = digitalHumanExactBindingConstraint(
		request.OwnerAccountID, request.OperationID, request.Target,
	)
	harness.connections.connection.Constraint = core.OwnerConstraint(
		request.OwnerAccountID, []core.Identifier{request.OperationID},
	)
	definition, err := catalog.WithDigitalHumanSurface(validDefinition(), "1.0.1")
	if err != nil {
		t.Fatalf("WithDigitalHumanSurface() error = %v", err)
	}
	harness.service.catalog = &definitionCatalogStub{definition: definition}
	return harness, request
}

func TestPrepareInvocationAdmitsPublishedDigitalHumanExactBinding(t *testing.T) {
	harness, request := newDigitalHumanExecutionHarness(t)
	snapshot, err := harness.service.PrepareInvocation(context.Background(), request)

	// The DigitalHuman surface gate (surfaceForTarget) is owned by a separate
	// change. While it is closed, preparation must still clear the
	// owner/operation/target constraint boundary and stop only at catalog
	// surface eligibility; before this correction the same request failed here
	// with AccessDenied from the Connection constraint.
	if surfaceForTarget(request.Target.Type) != catalog.SurfaceDigitalHuman {
		if !core.IsErrorCode(err, core.ErrorCodeConnectorUnavailable) {
			t.Fatalf("PrepareInvocation() error = %v, want the catalog surface rejection while the DigitalHuman surface gate is closed", err)
		}
		return
	}
	if err != nil {
		t.Fatalf("PrepareInvocation() error = %v", err)
	}
	if !validInvocationDigest(snapshot.InputDigest) || !validInvocationDigest(snapshot.ContractDigest) ||
		snapshot.BindingRevision != harness.bindings.binding.Revision ||
		snapshot.CredentialVersion != harness.connections.connection.CredentialVersion {
		t.Fatalf("snapshot = %#v", snapshot)
	}
	if harness.risk.calls != 0 || harness.credentials.calls != 0 || harness.provider.calls != 0 {
		t.Fatalf("preparation reached risk/credentials/provider = %d/%d/%d",
			harness.risk.calls, harness.credentials.calls, harness.provider.calls)
	}
}

func TestPrepareInvocationRejectsNarrowedDigitalHumanBindingBeforeTargetAuthority(t *testing.T) {
	for _, testCase := range []struct {
		name   string
		mutate func(*executionHarness)
	}{
		{
			name: "missing connection constraint",
			mutate: func(h *executionHarness) {
				h.connections.connection.Constraint = nil
			},
		},
		{
			name: "missing binding constraint",
			mutate: func(h *executionHarness) {
				h.bindings.binding.Constraint = nil
			},
		},
		{
			name: "wrong target",
			mutate: func(h *executionHarness) {
				h.bindings.binding.Constraint.TargetBoundaries[0].ID = uuid.NewString()
			},
		},
		{
			name: "wrong published version",
			mutate: func(h *executionHarness) {
				h.bindings.binding.Constraint.TargetBoundaries[0].VersionID = uuid.NewString()
			},
		},
		{
			name: "wrong binding owner",
			mutate: func(h *executionHarness) {
				h.bindings.binding.Constraint.AudienceOwner = uuid.New()
			},
		},
		{
			name: "wrong connection owner",
			mutate: func(h *executionHarness) {
				h.connections.connection.Constraint.AudienceOwner = uuid.New()
			},
		},
		{
			name: "binding operation not allowed",
			mutate: func(h *executionHarness) {
				h.bindings.binding.Constraint.AllowedOperationIDs = []core.Identifier{"github.list_issues"}
			},
		},
		{
			name: "connection operation not allowed",
			mutate: func(h *executionHarness) {
				h.connections.connection.Constraint.AllowedOperationIDs = []core.Identifier{"github.list_issues"}
			},
		},
		{
			name: "expired binding",
			mutate: func(h *executionHarness) {
				expiresAt := h.service.now().UTC()
				h.bindings.binding.Constraint.ExpiresAt = &expiresAt
			},
		},
		{
			name: "expired connection",
			mutate: func(h *executionHarness) {
				expiresAt := h.service.now().UTC()
				h.connections.connection.Constraint.ExpiresAt = &expiresAt
			},
		},
		{
			name: "denied binding",
			mutate: func(h *executionHarness) {
				h.bindings.binding.Constraint.DenyReasons = []string{"source_disabled"}
			},
		},
		{
			name: "denied connection",
			mutate: func(h *executionHarness) {
				h.connections.connection.Constraint.DenyReasons = []string{"source_disabled"}
			},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			harness, request := newDigitalHumanExecutionHarness(t)
			testCase.mutate(harness)
			if _, err := harness.service.PrepareInvocation(context.Background(), request); !core.IsErrorCode(err, core.ErrorCodeAccessDenied) {
				t.Fatalf("PrepareInvocation() error = %v, want %v", err, core.ErrorCodeAccessDenied)
			}
			if harness.target.calls != 0 || harness.risk.calls != 0 || harness.credentials.calls != 0 ||
				harness.provider.calls != 0 {
				t.Fatalf("rejected preparation reached target/risk/credentials/provider = %d/%d/%d/%d",
					harness.target.calls, harness.risk.calls, harness.credentials.calls, harness.provider.calls)
			}
		})
	}
}

// TestPrepareInvocationKeepsNonDigitalHumanConnectionBoundary pins that the
// owner-created binding may only supply a host boundary for DigitalHuman. A
// binding that allows Chat, Agent, or Workflow must not widen a Connection that
// excludes that target.
func TestPrepareInvocationKeepsNonDigitalHumanConnectionBoundary(t *testing.T) {
	for _, target := range []core.TargetRef{
		{Type: core.TargetTypeChat, ID: "conversation-1"},
		{Type: core.TargetTypeAgent, ID: uuid.NewString(), Scope: core.ScopeDraft},
		{Type: core.TargetTypeWorkflow, ID: uuid.NewString(), Scope: core.ScopeDraft, BindingInstanceID: uuid.NewString()},
	} {
		t.Run(string(target.Type), func(t *testing.T) {
			request := validExecutionRequest()
			request.Target = target
			harness := newExecutionHarnessForRequest(t, request)
			harness.connections.connection.Constraint.TargetBoundaries = []core.TargetBoundary{{
				Type: core.TargetTypeDigitalHuman, ID: "*", Scope: "*", VersionID: "*",
			}}
			if _, err := harness.service.PrepareInvocation(context.Background(), request); !core.IsErrorCode(err, core.ErrorCodeAccessDenied) {
				t.Fatalf("PrepareInvocation() error = %v, want %v", err, core.ErrorCodeAccessDenied)
			}
			if harness.target.calls != 0 {
				t.Fatalf("widened connection reached target authority: %d", harness.target.calls)
			}
		})
	}
}
