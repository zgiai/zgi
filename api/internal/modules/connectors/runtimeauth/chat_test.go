package runtimeauth_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/bindings"
	"github.com/zgiai/zgi/api/internal/modules/connectors/catalog"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	connectordispatch "github.com/zgiai/zgi/api/internal/modules/connectors/dispatch"
	"github.com/zgiai/zgi/api/internal/modules/connectors/runtimeauth"
)

func TestChatResolverProjectsOnlyCurrentUsableNativeOperations(t *testing.T) {
	t.Parallel()

	harness := newChatResolverHarness(t)
	unavailableID := core.Identifier(harness.definition.Operations[1].ID)
	harness.connection.UsableOperationIDs = []core.Identifier{
		core.Identifier(harness.definition.Operations[0].ID),
		unavailableID,
	}
	harness.connection.UnavailableOperations[unavailableID] = "provider_scope_missing"
	harness.bindings.items = append(harness.bindings.items,
		core.TargetBinding{
			ID: uuid.New(), OrganizationID: harness.organizationID, OwnerAccountID: harness.ownerID,
			ConnectionID: uuid.New(), ConnectorID: core.Identifier(harness.definition.ID),
			Target: core.TargetRef{Type: core.TargetTypeChat, ID: harness.conversationID},
			Status: core.BindingStatusNeedsReplacement,
		},
		core.TargetBinding{
			ID: uuid.New(), OrganizationID: harness.organizationID, OwnerAccountID: harness.ownerID,
			ConnectionID: uuid.New(), ConnectorID: core.Identifier("disabled-connector"),
			Target: core.TargetRef{Type: core.TargetTypeChat, ID: harness.conversationID},
			Status: core.BindingStatusDisabled,
		},
	)

	snapshot, err := harness.resolver.ResolveChatCapabilities(context.Background(), runtimeauth.ChatCapabilityRequest{
		OrganizationID: harness.organizationID,
		OwnerAccountID: harness.ownerID,
		ConversationID: harness.conversationID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Capabilities) != 1 || len(snapshot.Capabilities[0].Operations) != 1 {
		t.Fatalf("capability snapshot = %#v", snapshot)
	}
	capability := snapshot.Capabilities[0]
	if capability.BindingID != harness.binding.ID || capability.ConnectionID != harness.connection.ID ||
		capability.ConnectorID.String() != harness.definition.ID ||
		capability.Operations[0].ID != harness.definition.Operations[0].ID {
		t.Fatalf("projected capability = %#v", capability)
	}
	if harness.connections.calls != 1 {
		t.Fatalf("connection reads = %d, want only the native active binding", harness.connections.calls)
	}
}

func TestChatResolverNonReadyAndUnavailableBindingsNeverAuthorizeRuntime(t *testing.T) {
	t.Parallel()

	t.Run("non-ready", func(t *testing.T) {
		harness := newChatResolverHarness(t)
		harness.connection.State = core.ReadinessReconnectRequired
		snapshot, err := harness.resolver.ResolveChatCapabilities(context.Background(), runtimeauth.ChatCapabilityRequest{
			OrganizationID: harness.organizationID, OwnerAccountID: harness.ownerID,
			ConversationID: harness.conversationID,
		})
		if err != nil || snapshot.HasOperations() {
			t.Fatalf("non-ready snapshot = %#v, err = %v", snapshot, err)
		}
		_, err = harness.resolver.ResolveConnectorIdentity(context.Background(), harness.identityRequest())
		if !core.IsErrorCode(err, core.ErrorCodeTargetNotReady) {
			t.Fatalf("identity error = %v, want target_not_ready", err)
		}
	})

	t.Run("needs replacement", func(t *testing.T) {
		harness := newChatResolverHarness(t)
		harness.bindings.items[0].Status = core.BindingStatusNeedsReplacement
		snapshot, err := harness.resolver.ResolveChatCapabilities(context.Background(), runtimeauth.ChatCapabilityRequest{
			OrganizationID: harness.organizationID, OwnerAccountID: harness.ownerID,
			ConversationID: harness.conversationID,
		})
		if err != nil || snapshot.HasOperations() || harness.connections.calls != 0 {
			t.Fatalf("unavailable snapshot = %#v, reads = %d, err = %v", snapshot, harness.connections.calls, err)
		}
	})
}

func TestChatResolverNeverFallsBackToAnotherReadyConnection(t *testing.T) {
	t.Parallel()

	harness := newChatResolverHarness(t)
	boundConnectionID := harness.binding.ConnectionID
	alternate := *harness.connection
	alternate.ID = uuid.New()
	alternate.State = core.ReadinessReady
	harness.connections.connection = &alternate

	_, err := harness.resolver.ResolveChatCapabilities(context.Background(), runtimeauth.ChatCapabilityRequest{
		OrganizationID: harness.organizationID,
		OwnerAccountID: harness.ownerID,
		ConversationID: harness.conversationID,
	})
	if !core.IsErrorCode(err, core.ErrorCodeNotFound) {
		t.Fatalf("missing bound connection error = %v, want not_found", err)
	}
	if len(harness.connections.requestedIDs) != 1 || harness.connections.requestedIDs[0] != boundConnectionID {
		t.Fatalf("connection reads = %v, want only bound connection %s", harness.connections.requestedIDs, boundConnectionID)
	}
}

func TestChatResolverReauthorizesTypedIdentityAtInvocation(t *testing.T) {
	t.Parallel()

	harness := newChatResolverHarness(t)
	request := harness.identityRequest()
	identity, err := harness.resolver.ResolveConnectorIdentity(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if identity.OrganizationID != request.OrganizationID || identity.OwnerAccountID != request.OwnerAccountID ||
		identity.ActingPrincipalID != request.ActingPrincipalID || identity.BindingID != harness.binding.ID ||
		identity.ConnectionID != harness.connection.ID || identity.ConnectorID != request.ConnectorID ||
		identity.OperationID != request.OperationID || identity.Target != request.Target ||
		identity.LogicalOperationID != request.LogicalOperationID || identity.CorrelationID != request.CorrelationID ||
		!identity.Interactive {
		t.Fatalf("resolved identity = %#v, request = %#v", identity, request)
	}

	// A detach between model discovery and the actual Tool call is observed by
	// the second authoritative read and cannot reuse the earlier snapshot.
	harness.bindings.items[0].Status = core.BindingStatusDisabled
	if _, err := harness.resolver.ResolveConnectorIdentity(context.Background(), request); !core.IsErrorCode(err, core.ErrorCodeTargetNotReady) {
		t.Fatalf("detached identity error = %v, want target_not_ready", err)
	}
	if harness.bindings.calls != 2 {
		t.Fatalf("binding reads = %d, want one per invocation boundary", harness.bindings.calls)
	}
}

func TestChatResolverRejectsForeignOwnerProjection(t *testing.T) {
	t.Parallel()

	harness := newChatResolverHarness(t)
	harness.bindings.items[0].OwnerAccountID = uuid.New()
	_, err := harness.resolver.ResolveChatCapabilities(context.Background(), runtimeauth.ChatCapabilityRequest{
		OrganizationID: harness.organizationID, OwnerAccountID: harness.ownerID,
		ConversationID: harness.conversationID,
	})
	if !core.IsErrorCode(err, core.ErrorCodeInternal) || harness.connections.calls != 0 {
		t.Fatalf("foreign-owner projection error = %v, connection reads = %d", err, harness.connections.calls)
	}
}

func TestChatResolverRejectsBindingForAnotherTarget(t *testing.T) {
	t.Parallel()

	harness := newChatResolverHarness(t)
	harness.bindings.items[0].Target.ID = uuid.NewString()
	_, err := harness.resolver.ResolveChatCapabilities(context.Background(), runtimeauth.ChatCapabilityRequest{
		OrganizationID: harness.organizationID, OwnerAccountID: harness.ownerID,
		ConversationID: harness.conversationID,
	})
	if !core.IsErrorCode(err, core.ErrorCodeInternal) || harness.connections.calls != 0 {
		t.Fatalf("foreign-target projection error = %v, connection reads = %d", err, harness.connections.calls)
	}
}

func TestChatResolverRejectsDuplicateConnectorBindings(t *testing.T) {
	t.Parallel()

	harness := newChatResolverHarness(t)
	duplicate := harness.binding
	duplicate.ID = uuid.New()
	duplicate.ConnectionID = uuid.New()
	harness.bindings.items = append(harness.bindings.items, duplicate)

	_, err := harness.resolver.ResolveChatCapabilities(context.Background(), runtimeauth.ChatCapabilityRequest{
		OrganizationID: harness.organizationID, OwnerAccountID: harness.ownerID,
		ConversationID: harness.conversationID,
	})
	if !core.IsErrorCode(err, core.ErrorCodeConflict) {
		t.Fatalf("duplicate binding error = %v, want conflict", err)
	}
}

type chatResolverHarness struct {
	organizationID uuid.UUID
	ownerID        uuid.UUID
	conversationID string
	definition     catalog.Definition
	binding        core.TargetBinding
	connection     *core.PersonalConnection
	bindings       *bindingListerSpy
	connections    *connectionReaderSpy
	resolver       *runtimeauth.ChatResolver
}

func newChatResolverHarness(t *testing.T) *chatResolverHarness {
	t.Helper()
	definitions, err := catalog.BuiltinDefinitions()
	if err != nil {
		t.Fatal(err)
	}
	var definition catalog.Definition
	for _, candidate := range definitions {
		if candidate.ID == "github" {
			definition = candidate
			break
		}
	}
	if definition.ID == "" || len(definition.Operations) < 2 {
		t.Fatal("GitHub Connector test definition is incomplete")
	}
	organizationID := uuid.New()
	ownerID := uuid.New()
	conversationID := uuid.NewString()
	binding := core.TargetBinding{Constraint: bindings.NewBindingConstraint(ownerID, core.Identifier(definition.ID)),
		ID: uuid.New(), OrganizationID: organizationID, OwnerAccountID: ownerID,
		ConnectionID: uuid.New(), ConnectorID: core.Identifier(definition.ID),
		Target: core.TargetRef{Type: core.TargetTypeChat, ID: conversationID},
		Status: core.BindingStatusActive,
	}
	connection := core.PersonalConnection{Constraint: bindings.NewBindingConstraint(ownerID, core.Identifier(definition.ID)),
		ID: binding.ConnectionID, OrganizationID: organizationID, OwnerAccountID: ownerID,
		ConnectorID: core.Identifier(definition.ID), State: core.ReadinessReady,
		UsableOperationIDs:    []core.Identifier{core.Identifier(definition.Operations[0].ID)},
		UnavailableOperations: map[core.Identifier]string{},
	}
	bindingReader := &bindingListerSpy{items: []core.TargetBinding{binding}}
	connectionReader := &connectionReaderSpy{connection: &connection}
	resolver, err := runtimeauth.NewChatResolver(bindingReader, connectionReader, definitionMap{definition.ID: definition})
	if err != nil {
		t.Fatal(err)
	}
	return &chatResolverHarness{
		organizationID: organizationID, ownerID: ownerID, conversationID: conversationID,
		definition: definition, binding: binding, connection: &connection,
		bindings: bindingReader, connections: connectionReader, resolver: resolver,
	}
}

func (harness *chatResolverHarness) identityRequest() connectordispatch.IdentityResolutionRequest {
	return connectordispatch.IdentityResolutionRequest{
		OrganizationID: harness.organizationID, OwnerAccountID: harness.ownerID,
		ActingPrincipalID: harness.ownerID, ConnectorID: core.Identifier(harness.definition.ID),
		Target:             core.TargetRef{Type: core.TargetTypeChat, ID: harness.conversationID},
		OperationID:        core.Identifier(harness.definition.Operations[0].ID),
		LogicalOperationID: "logical-operation", CorrelationID: "correlation", Interactive: true,
	}
}

type bindingListerSpy struct {
	items []core.TargetBinding
	calls int
}

func (spy *bindingListerSpy) ListTargetBindings(
	_ context.Context,
	organizationID uuid.UUID,
	ownerID uuid.UUID,
	target bindings.TargetIdentity,
) ([]core.TargetBinding, error) {
	spy.calls++
	if organizationID == uuid.Nil || ownerID == uuid.Nil || target.Type != core.TargetTypeChat || target.ID == "" {
		return nil, core.NewError(core.ErrorCodeInvalidInput, false, nil)
	}
	return append([]core.TargetBinding(nil), spy.items...), nil
}

type connectionReaderSpy struct {
	connection   *core.PersonalConnection
	calls        int
	requestedIDs []uuid.UUID
}

func (spy *connectionReaderSpy) GetConnection(
	_ context.Context,
	organizationID uuid.UUID,
	ownerID uuid.UUID,
	connectionID uuid.UUID,
) (core.PersonalConnection, error) {
	spy.calls++
	spy.requestedIDs = append(spy.requestedIDs, connectionID)
	if spy.connection == nil || spy.connection.OrganizationID != organizationID ||
		spy.connection.OwnerAccountID != ownerID || spy.connection.ID != connectionID {
		return core.PersonalConnection{}, core.NewError(core.ErrorCodeNotFound, false, nil)
	}
	return *spy.connection, nil
}

type definitionMap map[string]catalog.Definition

func (definitions definitionMap) Get(id string) (catalog.Definition, bool) {
	definition, ok := definitions[id]
	return definition, ok
}
