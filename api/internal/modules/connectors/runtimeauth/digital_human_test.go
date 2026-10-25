package runtimeauth_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/bindings"
	"github.com/zgiai/zgi/api/internal/modules/connectors/catalog"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	connectordispatch "github.com/zgiai/zgi/api/internal/modules/connectors/dispatch"
	"github.com/zgiai/zgi/api/internal/modules/connectors/runtimeauth"
)

func TestDelegatedDigitalHumanCapabilitiesRequireRunAuthorization(t *testing.T) {
	harness := newDigitalHumanHarness(t)
	versionID := uuid.NewString()
	binding := harness.addBinding(t, core.ScopePublished, versionID)
	binding.Constraint.HostDelegation = &core.HostDelegation{GrantedBy: harness.ownerID, Audiences: []core.HostAudience{core.HostAudienceWorkspace}}
	harness.bindings.items[len(harness.bindings.items)-1] = binding
	candidate := &delegatedCandidateSpy{bindings: []core.TargetBinding{binding}}
	request := runtimeauth.DelegatedHostRequest{OrganizationID: harness.organizationID, RequesterID: uuid.New(), RunID: uuid.New(), Target: binding.Target, Audience: core.HostAudienceWorkspace}
	if _, err := harness.resolver.ResolveDelegatedCapabilities(t.Context(), request); !core.IsErrorCode(err, core.ErrorCodeAccessDenied) || candidate.calls != 0 {
		t.Fatalf("missing run authorizer exposed host grant: %v", err)
	}
	verifier := &delegatedRunAuthorizerSpy{err: core.NewError(core.ErrorCodeAccessDenied, false, nil)}
	harness.resolver.WithDelegatedHostAccess(candidate, verifier)
	if _, err := harness.resolver.ResolveDelegatedCapabilities(t.Context(), request); !core.IsErrorCode(err, core.ErrorCodeAccessDenied) || harness.bindings.calls != 0 {
		t.Fatalf("rejected run reached owner binding: %v", err)
	}
	verifier.err = nil
	ungranted := binding
	constraint := *binding.Constraint
	constraint.HostDelegation = nil
	ungranted.Constraint = &constraint
	candidate.bindings = []core.TargetBinding{ungranted}
	verifier.last = runtimeauth.DelegatedHostRequest{}
	if _, err := harness.resolver.ResolveDelegatedCapabilities(t.Context(), request); !core.IsErrorCode(err, core.ErrorCodeAccessDenied) || verifier.last.BindingID != uuid.Nil {
		t.Fatalf("owner-only candidate reached run authorizer: %v, proof=%#v", err, verifier.last)
	}
	candidate.bindings = []core.TargetBinding{binding}
	snapshot, err := harness.resolver.ResolveDelegatedCapabilities(t.Context(), request)
	if err != nil || len(snapshot.Capabilities) != 1 || snapshot.Capabilities[0].BindingID != binding.ID || snapshot.Capabilities[0].OwnerAccountID != harness.ownerID || verifier.last.BindingID != binding.ID || verifier.last.RequesterID != request.RequesterID {
		t.Fatalf("authorized host grant = %#v, proof=%#v, error=%v", snapshot, verifier.last, err)
	}
}

func TestDelegatedAgentCapabilitiesUseOnlyAuthorizedPublishedBinding(t *testing.T) {
	harness := newDigitalHumanHarness(t)
	binding := harness.binding
	binding.ID = uuid.New()
	binding.Target = core.TargetRef{Type: core.TargetTypeAgent, ID: uuid.NewString(), Scope: core.ScopePublished, VersionID: uuid.NewString()}
	binding.Constraint = core.OwnerConstraint(harness.ownerID, harness.operations)
	binding.Constraint.TargetBoundaries = []core.TargetBoundary{{Type: binding.Target.Type, ID: binding.Target.ID, Scope: string(binding.Target.Scope), VersionID: binding.Target.VersionID}}
	binding.Constraint.HostDelegation = &core.HostDelegation{GrantedBy: harness.ownerID, Audiences: []core.HostAudience{core.HostAudienceWorkspace}}
	harness.bindings.items = append(harness.bindings.items, binding)
	candidate := &delegatedCandidateSpy{bindings: []core.TargetBinding{binding}}
	verifier := &delegatedRunAuthorizerSpy{}
	resolver, err := runtimeauth.NewAgentResolver(harness.bindings, harness.connections, definitionMap{harness.definition.ID: harness.definition})
	if err != nil {
		t.Fatal(err)
	}
	resolver.WithDelegatedHostAccess(candidate, verifier)
	request := runtimeauth.DelegatedHostRequest{OrganizationID: harness.organizationID, RequesterID: uuid.New(), ConversationID: uuid.NewString(), Target: binding.Target, Audience: core.HostAudienceWorkspace}
	snapshot, err := resolver.ResolveDelegatedCapabilities(t.Context(), request)
	if err != nil || len(snapshot.Capabilities) != 1 || snapshot.Capabilities[0].BindingID != binding.ID || snapshot.Capabilities[0].OwnerAccountID != harness.ownerID {
		t.Fatalf("authorized Agent host grant = %#v, error=%v", snapshot, err)
	}
}

func TestDelegatedAgentCapabilitiesRejectAmbiguousOwners(t *testing.T) {
	harness := newDigitalHumanHarness(t)
	target := core.TargetRef{Type: core.TargetTypeAgent, ID: uuid.NewString(), Scope: core.ScopePublished, VersionID: uuid.NewString()}
	ownerIDs := []uuid.UUID{harness.ownerID, uuid.New()}
	candidates := make([]core.TargetBinding, 0, 2)
	for _, ownerID := range ownerIDs {
		binding := harness.binding
		binding.ID, binding.OwnerAccountID, binding.Target = uuid.New(), ownerID, target
		binding.Constraint = core.OwnerConstraint(ownerID, harness.operations)
		binding.Constraint.TargetBoundaries = []core.TargetBoundary{{Type: target.Type, ID: target.ID, Scope: string(target.Scope), VersionID: target.VersionID}}
		binding.Constraint.HostDelegation = &core.HostDelegation{GrantedBy: ownerID, Audiences: []core.HostAudience{core.HostAudienceWorkspace}}
		connection := digitalHumanConnection(harness.organizationID, ownerID, harness.definition, harness.operations)
		binding.ConnectionID = connection.ID
		harness.connections.connections[connection.ID] = connection
		harness.bindings.items = append(harness.bindings.items, binding)
		candidates = append(candidates, binding)
	}
	resolver, err := runtimeauth.NewAgentResolver(harness.bindings, harness.connections, definitionMap{harness.definition.ID: harness.definition})
	if err != nil {
		t.Fatal(err)
	}
	resolver.WithDelegatedHostAccess(&delegatedCandidateSpy{bindings: candidates}, &delegatedRunAuthorizerSpy{})
	request := runtimeauth.DelegatedHostRequest{OrganizationID: harness.organizationID, RequesterID: uuid.New(), ConversationID: uuid.NewString(), Target: target, Audience: core.HostAudienceWorkspace}
	if _, err := resolver.ResolveDelegatedCapabilities(t.Context(), request); !core.IsErrorCode(err, core.ErrorCodeConflict) {
		t.Fatalf("different Agent binding owners were collapsed: %v", err)
	}
}

type delegatedCandidateSpy struct {
	bindings []core.TargetBinding
	calls    int
}

func (spy *delegatedCandidateSpy) ListPublishedHostDelegations(_ context.Context, _ uuid.UUID, _ core.TargetRef, _ core.HostAudience) ([]core.TargetBinding, error) {
	spy.calls++
	return spy.bindings, nil
}

type delegatedRunAuthorizerSpy struct {
	last runtimeauth.DelegatedHostRequest
	err  error
}

func (spy *delegatedRunAuthorizerSpy) AuthorizeHostUse(_ context.Context, request runtimeauth.DelegatedHostRequest) error {
	spy.last = request
	return spy.err
}

// TestDigitalHumanResolverAllowsMultipleInstancesOfOneConnector proves that a
// DigitalHuman host may bind the same Connector more than once. Chat, Agent and
// Workflow intentionally keep a single active binding per Connector, so this
// also guards against regressing their duplicate-binding conflict.
func TestDigitalHumanResolverAllowsMultipleInstancesOfOneConnector(t *testing.T) {
	t.Parallel()

	harness := newDigitalHumanHarness(t)
	first := harness.binding
	second := harness.addBinding(t, core.ScopeDraft, "")

	snapshot, err := harness.resolver.ResolveDigitalHumanCapabilities(
		context.Background(), harness.capabilityRequest(core.ScopeDraft, ""))
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Capabilities) != 2 {
		t.Fatalf("capabilities = %#v, want two binding instances", snapshot.Capabilities)
	}
	byInstance := capabilityBindings(snapshot)
	if len(byInstance) != 2 {
		t.Fatalf("binding instances = %#v, want two distinct instances", byInstance)
	}
	if byInstance[first.Target.BindingInstanceID] != first.ID || byInstance[second.Target.BindingInstanceID] != second.ID {
		t.Fatalf("projected instances = %#v, want first=%s second=%s", byInstance, first.ID, second.ID)
	}
	for _, capability := range snapshot.Capabilities {
		if capability.ConnectorID != core.Identifier(harness.definition.ID) {
			t.Fatalf("connector = %q, want %q", capability.ConnectorID, harness.definition.ID)
		}
		if capability.Operations[0].ID != harness.definition.Operations[0].ID {
			t.Fatalf("operations = %#v, want the admitted operation", capability.Operations)
		}
	}
	if snapshot.Capabilities[0].Target.BindingInstanceID > snapshot.Capabilities[1].Target.BindingInstanceID {
		t.Fatalf("capabilities are not ordered by binding instance: %#v", snapshot.Capabilities)
	}
	if harness.connections.calls != 2 {
		t.Fatalf("connection reads = %d, want one authoritative read per instance", harness.connections.calls)
	}
	if harness.bindings.lastTarget.Type != core.TargetTypeDigitalHuman || harness.bindings.lastTarget.ID != harness.digitalHumanID ||
		harness.bindings.lastTarget.Scope != core.ScopeDraft || harness.bindings.lastTarget.VersionID != "" {
		t.Fatalf("binding lookup identity = %#v", harness.bindings.lastTarget)
	}
}

// TestDigitalHumanResolverKeepsVersionsAndOwnersSeparate proves that a binding
// for another scope/version, host or owner is never projected as authority.
func TestDigitalHumanResolverKeepsVersionsAndOwnersSeparate(t *testing.T) {
	t.Parallel()

	t.Run("draft and published stay separate", func(t *testing.T) {
		harness := newDigitalHumanHarness(t)
		draft := harness.binding
		versionID := uuid.NewString()
		published := harness.addBinding(t, core.ScopePublished, versionID)

		draftSnapshot, err := harness.resolver.ResolveDigitalHumanCapabilities(
			context.Background(), harness.capabilityRequest(core.ScopeDraft, ""))
		if err != nil {
			t.Fatal(err)
		}
		if len(draftSnapshot.Capabilities) != 1 || draftSnapshot.Capabilities[0].BindingID != draft.ID {
			t.Fatalf("draft projection = %#v, want only the draft binding", draftSnapshot.Capabilities)
		}

		publishedSnapshot, err := harness.resolver.ResolveDigitalHumanCapabilities(
			context.Background(), harness.capabilityRequest(core.ScopePublished, versionID))
		if err != nil {
			t.Fatal(err)
		}
		if len(publishedSnapshot.Capabilities) != 1 || publishedSnapshot.Capabilities[0].BindingID != published.ID {
			t.Fatalf("published projection = %#v, want only the published binding", publishedSnapshot.Capabilities)
		}

		unknown, err := harness.resolver.ResolveDigitalHumanCapabilities(
			context.Background(), harness.capabilityRequest(core.ScopePublished, uuid.NewString()))
		if err != nil || unknown.HasOperations() {
			t.Fatalf("unknown published version = %#v, err = %v", unknown, err)
		}
	})

	t.Run("hostile lister mismatch fails closed", func(t *testing.T) {
		for _, test := range []struct {
			name   string
			mutate func(*core.TargetBinding)
		}{
			{"foreign owner", func(binding *core.TargetBinding) { binding.OwnerAccountID = uuid.New() }},
			{"foreign host", func(binding *core.TargetBinding) { binding.Target.ID = uuid.NewString() }},
			{"foreign version", func(binding *core.TargetBinding) {
				binding.Target.Scope = core.ScopePublished
				binding.Target.VersionID = uuid.NewString()
			}},
		} {
			t.Run(test.name, func(t *testing.T) {
				harness := newDigitalHumanHarness(t)
				test.mutate(&harness.bindings.items[0])
				// A compromised lister bypasses the service query filter so the
				// resolver's own identity checks are the only remaining guard.
				harness.bindings.raw = true

				if _, err := harness.resolver.ResolveDigitalHumanCapabilities(
					context.Background(), harness.capabilityRequest(core.ScopeDraft, "")); !core.IsErrorCode(err, core.ErrorCodeInternal) {
					t.Fatalf("mismatched binding error = %v, want connector_internal", err)
				}
				if harness.connections.calls != 0 {
					t.Fatalf("connection reads = %d, want none for a mismatched binding", harness.connections.calls)
				}
			})
		}
	})
}

func TestDigitalHumanResolverNeverAuthorizesNonReadyOrUnadmittedBindings(t *testing.T) {
	t.Parallel()

	t.Run("non-ready connection", func(t *testing.T) {
		harness := newDigitalHumanHarness(t)
		harness.connection.State = core.ReadinessReconnectRequired

		snapshot, err := harness.resolver.ResolveDigitalHumanCapabilities(
			context.Background(), harness.capabilityRequest(core.ScopeDraft, ""))
		if err != nil || snapshot.HasOperations() {
			t.Fatalf("non-ready snapshot = %#v, err = %v", snapshot, err)
		}
		_, err = harness.resolver.ResolveConnectorIdentity(context.Background(),
			harness.identityRequest(harness.binding, harness.definition.Operations[0].ID))
		if !core.IsErrorCode(err, core.ErrorCodeTargetNotReady) {
			t.Fatalf("non-ready identity error = %v, want connector_target_not_ready", err)
		}
	})

	t.Run("connector without the digital human surface", func(t *testing.T) {
		harness := newDigitalHumanHarness(t)
		base := builtinDefinition(t, "github")
		resolver, err := runtimeauth.NewDigitalHumanResolver(
			harness.bindings, harness.connections, definitionMap{base.ID: base})
		if err != nil {
			t.Fatal(err)
		}
		snapshot, err := resolver.ResolveDigitalHumanCapabilities(
			context.Background(), harness.capabilityRequest(core.ScopeDraft, ""))
		if err != nil || snapshot.HasOperations() || harness.connections.calls != 1 {
			t.Fatalf("unadmitted surface snapshot = %#v, reads = %d, err = %v",
				snapshot, harness.connections.calls, err)
		}
	})
}

// TestDigitalHumanResolverIdentitySelectsTheExactBindingInstance proves that
// dispatch re-reads the authoritative bindings and matches the concrete
// instance, Connector and operation carried by the invocation.
func TestDigitalHumanResolverIdentitySelectsTheExactBindingInstance(t *testing.T) {
	t.Parallel()

	harness := newDigitalHumanHarness(t)
	harness.addBinding(t, core.ScopeDraft, "")
	target := harness.binding
	request := harness.identityRequest(target, harness.definition.Operations[0].ID)

	identity, err := harness.resolver.ResolveConnectorIdentity(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if identity.OrganizationID != request.OrganizationID || identity.OwnerAccountID != request.OwnerAccountID ||
		identity.ActingPrincipalID != request.ActingPrincipalID || identity.BindingID != target.ID ||
		identity.ConnectionID != target.ConnectionID || identity.ConnectorID != request.ConnectorID ||
		identity.OperationID != request.OperationID || identity.Target != request.Target ||
		identity.LogicalOperationID != request.LogicalOperationID || identity.CorrelationID != request.CorrelationID {
		t.Fatalf("resolved identity = %#v, request = %#v", identity, request)
	}

	// A detach between discovery and execution must be observed by the second
	// authoritative read and cannot reuse the earlier snapshot.
	harness.bindings.items[1].Status = core.BindingStatusDisabled
	if _, err := harness.resolver.ResolveConnectorIdentity(context.Background(), request); !core.IsErrorCode(err, core.ErrorCodeTargetNotReady) {
		t.Fatalf("detached instance error = %v, want connector_target_not_ready", err)
	}

	unknown := request
	unknown.Target.BindingInstanceID = uuid.NewString()
	if _, err := harness.resolver.ResolveConnectorIdentity(context.Background(), unknown); !core.IsErrorCode(err, core.ErrorCodeTargetNotReady) {
		t.Fatalf("unknown instance error = %v, want connector_target_not_ready", err)
	}

	wrongConnector := request
	wrongConnector.ConnectorID = core.Identifier("exa")
	if _, err := harness.resolver.ResolveConnectorIdentity(context.Background(), wrongConnector); !core.IsErrorCode(err, core.ErrorCodeTargetNotReady) {
		t.Fatalf("unknown connector error = %v, want connector_target_not_ready", err)
	}

	if harness.bindings.calls != 4 {
		t.Fatalf("binding reads = %d, want one per invocation boundary", harness.bindings.calls)
	}
}

func TestDigitalHumanResolverRejectsInvalidCapabilityRequests(t *testing.T) {
	t.Parallel()

	harness := newDigitalHumanHarness(t)
	for _, test := range []struct {
		name   string
		mutate func(*runtimeauth.DigitalHumanCapabilityRequest)
	}{
		{"nil organization", func(request *runtimeauth.DigitalHumanCapabilityRequest) { request.OrganizationID = uuid.Nil }},
		{"nil owner", func(request *runtimeauth.DigitalHumanCapabilityRequest) { request.OwnerAccountID = uuid.Nil }},
		{"empty host", func(request *runtimeauth.DigitalHumanCapabilityRequest) { request.DigitalHumanID = "" }},
		{"live scope", func(request *runtimeauth.DigitalHumanCapabilityRequest) { request.Scope = core.ScopeLive }},
		{"published without version", func(request *runtimeauth.DigitalHumanCapabilityRequest) {
			request.Scope = core.ScopePublished
		}},
		{"published with malformed version", func(request *runtimeauth.DigitalHumanCapabilityRequest) {
			request.Scope, request.VersionID = core.ScopePublished, "not-a-version"
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := harness.capabilityRequest(core.ScopeDraft, "")
			test.mutate(&request)
			if _, err := harness.resolver.ResolveDigitalHumanCapabilities(context.Background(), request); !core.IsErrorCode(err, core.ErrorCodeInvalidInput) {
				t.Fatalf("capability error = %v, want connector_invalid_input", err)
			}
			if harness.connections.calls != 0 || harness.bindings.calls != 0 {
				t.Fatal("invalid request reached a dependency")
			}
		})
	}
}

func TestDigitalHumanResolverRejectsInvalidIdentityRequests(t *testing.T) {
	t.Parallel()

	harness := newDigitalHumanHarness(t)
	valid := harness.identityRequest(harness.binding, harness.definition.Operations[0].ID)
	for _, test := range []struct {
		name   string
		mutate func(*connectordispatch.IdentityResolutionRequest)
	}{
		{"chat target", func(request *connectordispatch.IdentityResolutionRequest) {
			request.Target = core.TargetRef{Type: core.TargetTypeChat, ID: uuid.NewString()}
		}},
		{"missing binding instance", func(request *connectordispatch.IdentityResolutionRequest) {
			request.Target.BindingInstanceID = ""
		}},
		{"foreign acting principal", func(request *connectordispatch.IdentityResolutionRequest) {
			request.ActingPrincipalID = uuid.New()
		}},
		{"invalid connector", func(request *connectordispatch.IdentityResolutionRequest) { request.ConnectorID = "" }},
		{"invalid operation", func(request *connectordispatch.IdentityResolutionRequest) { request.OperationID = "" }},
		{"empty logical operation", func(request *connectordispatch.IdentityResolutionRequest) { request.LogicalOperationID = "" }},
		{"empty correlation", func(request *connectordispatch.IdentityResolutionRequest) { request.CorrelationID = "" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := valid
			test.mutate(&request)
			if _, err := harness.resolver.ResolveConnectorIdentity(context.Background(), request); !core.IsErrorCode(err, core.ErrorCodeInvalidInput) {
				t.Fatalf("identity error = %v, want connector_invalid_input", err)
			}
		})
	}
	if harness.bindings.calls != 0 || harness.connections.calls != 0 {
		t.Fatal("invalid identity request reached a dependency")
	}
}

// TestDigitalHumanResolverProjectsOnlyTheConnectionDisplayLabel proves that the
// model-visible capability carries only the owner-scoped display name, never the
// credential envelope or other private connection metadata.
func TestDigitalHumanResolverProjectsOnlyTheConnectionDisplayLabel(t *testing.T) {
	t.Parallel()

	const (
		label                = "Team GitHub"
		credentialSentinel   = "opaque-credential-envelope-sentinel"
		externalIDSentinel   = "private-external-account-sentinel"
		externalNameSentinel = "private-external-display-name-sentinel"
	)
	harness := newDigitalHumanHarness(t)
	harness.connection.Name = label
	harness.connection.EncryptedCredentials = credentialSentinel
	harness.connection.ExternalAccountID = stringPointer(externalIDSentinel)
	harness.connection.ExternalDisplayName = stringPointer(externalNameSentinel)

	snapshot, err := harness.resolver.ResolveDigitalHumanCapabilities(
		context.Background(), harness.capabilityRequest(core.ScopeDraft, ""))
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Capabilities) != 1 {
		t.Fatalf("capabilities = %#v", snapshot.Capabilities)
	}
	capability := snapshot.Capabilities[0]
	if capability.ConnectionLabel != label || capability.ConnectionID != harness.connection.ID ||
		capability.BindingID != harness.binding.ID {
		t.Fatalf("capability = %#v, want label %q for connection %s", capability, label, harness.connection.ID)
	}
	rendered := fmt.Sprintf("%#v", snapshot)
	for _, forbidden := range []string{credentialSentinel, externalIDSentinel, externalNameSentinel} {
		if strings.Contains(rendered, forbidden) {
			t.Fatalf("capability snapshot leaked private connection data %q", forbidden)
		}
	}
}

func stringPointer(value string) *string { return &value }

type digitalHumanHarness struct {
	organizationID uuid.UUID
	ownerID        uuid.UUID
	digitalHumanID string
	definition     catalog.Definition
	operations     []core.Identifier
	binding        core.TargetBinding
	connection     *core.PersonalConnection
	bindings       *digitalHumanBindingListerSpy
	connections    *digitalHumanConnectionReaderSpy
	resolver       *runtimeauth.DigitalHumanResolver
}

func newDigitalHumanHarness(t *testing.T) *digitalHumanHarness {
	t.Helper()

	definition := digitalHumanDefinition(t)
	organizationID := uuid.New()
	ownerID := uuid.New()
	operations := []core.Identifier{core.Identifier(definition.Operations[0].ID)}
	lister := &digitalHumanBindingListerSpy{}
	connections := &digitalHumanConnectionReaderSpy{connections: map[uuid.UUID]*core.PersonalConnection{}}
	resolver, err := runtimeauth.NewDigitalHumanResolver(
		lister, connections, definitionMap{definition.ID: definition})
	if err != nil {
		t.Fatal(err)
	}
	harness := &digitalHumanHarness{
		organizationID: organizationID, ownerID: ownerID, digitalHumanID: uuid.NewString(),
		definition: definition, operations: operations,
		bindings: lister, connections: connections, resolver: resolver,
	}
	harness.addBinding(t, core.ScopeDraft, "")
	return harness
}

// addBinding appends one independent resource binding instance of the same
// Connector, each with its own ready owner-scoped Personal Connection.
func (harness *digitalHumanHarness) addBinding(t *testing.T, scope core.TargetScope, versionID string) core.TargetBinding {
	t.Helper()

	target := core.TargetRef{
		Type: core.TargetTypeDigitalHuman, ID: harness.digitalHumanID,
		Scope: scope, VersionID: versionID, BindingInstanceID: uuid.NewString(),
	}
	connection := digitalHumanConnection(harness.organizationID, harness.ownerID, harness.definition, harness.operations)
	binding := core.TargetBinding{
		Constraint: digitalHumanBindingConstraint(harness.ownerID, target, harness.operations),
		ID:         uuid.New(), OrganizationID: harness.organizationID, OwnerAccountID: harness.ownerID,
		ConnectionID: connection.ID, ConnectorID: core.Identifier(harness.definition.ID),
		Target: target, Status: core.BindingStatusActive,
	}
	harness.bindings.items = append(harness.bindings.items, binding)
	harness.connections.connections[connection.ID] = connection
	harness.binding = binding
	harness.connection = connection
	return binding
}

func (harness *digitalHumanHarness) capabilityRequest(scope core.TargetScope, versionID string) runtimeauth.DigitalHumanCapabilityRequest {
	return runtimeauth.DigitalHumanCapabilityRequest{
		OrganizationID: harness.organizationID, OwnerAccountID: harness.ownerID,
		DigitalHumanID: harness.digitalHumanID, Scope: scope, VersionID: versionID,
	}
}

func (harness *digitalHumanHarness) identityRequest(binding core.TargetBinding, operationID string) connectordispatch.IdentityResolutionRequest {
	return connectordispatch.IdentityResolutionRequest{
		OrganizationID: harness.organizationID, OwnerAccountID: harness.ownerID,
		ActingPrincipalID: harness.ownerID, ConnectorID: binding.ConnectorID,
		Target: binding.Target, OperationID: core.Identifier(operationID),
		LogicalOperationID: "logical-operation", CorrelationID: "correlation",
	}
}

// digitalHumanDefinition admits a DigitalHuman extension contract without
// changing the Connector's credential family.
func digitalHumanDefinition(t *testing.T) catalog.Definition {
	t.Helper()

	base := builtinDefinition(t, "github")
	definition, err := catalog.WithDigitalHumanSurface(base, base.PluginVersion+"-digitalhuman.1")
	if err != nil {
		t.Fatal(err)
	}
	if !definition.Supports(catalog.SurfaceDigitalHuman) {
		t.Fatal("digital human definition is missing the host surface")
	}
	return definition
}

func builtinDefinition(t *testing.T, connectorID string) catalog.Definition {
	t.Helper()

	definitions, err := catalog.BuiltinDefinitions()
	if err != nil {
		t.Fatal(err)
	}
	for _, candidate := range definitions {
		if candidate.ID == connectorID {
			if len(candidate.Operations) < 2 {
				t.Fatalf("Connector %q test definition is incomplete", connectorID)
			}
			return candidate
		}
	}
	t.Fatalf("Connector %q test definition is missing", connectorID)
	return catalog.Definition{}
}

func digitalHumanBindingConstraint(owner uuid.UUID, target core.TargetRef, operations []core.Identifier) *core.AccessConstraint {
	constraint := core.OwnerConstraint(owner, operations)
	constraint.TargetBoundaries = []core.TargetBoundary{{
		Type: target.Type, ID: target.ID, Scope: string(target.EffectiveScope()), VersionID: target.VersionID,
	}}
	return constraint
}

// digitalHumanConnection retains only safe metadata. The encrypted envelope
// stands in as an opaque non-secret test value: resolution never opens it.
func digitalHumanConnection(organizationID, ownerID uuid.UUID, definition catalog.Definition, operations []core.Identifier) *core.PersonalConnection {
	return &core.PersonalConnection{
		Constraint:     core.OwnerConstraint(ownerID, operations),
		ID:             uuid.New(),
		OrganizationID: organizationID, OwnerAccountID: ownerID,
		ConnectorID:           core.Identifier(definition.ID),
		ProviderID:            core.Identifier(definition.ProviderID),
		DriverID:              core.Identifier(definition.DriverID),
		AuthMethodID:          core.Identifier(definition.AuthMethodID),
		Name:                  "Digital Human Connector",
		EncryptedCredentials:  "opaque-test-envelope",
		CredentialVersion:     1,
		State:                 core.ReadinessReady,
		UsableOperationIDs:    append([]core.Identifier(nil), operations...),
		UnavailableOperations: map[core.Identifier]string{},
		Revision:              1,
		LastVerifiedAt:        time.Now().UTC(),
	}
}

func capabilityBindings(snapshot runtimeauth.ChatCapabilitySnapshot) map[string]uuid.UUID {
	result := make(map[string]uuid.UUID, len(snapshot.Capabilities))
	for _, capability := range snapshot.Capabilities {
		result[capability.Target.BindingInstanceID] = capability.BindingID
	}
	return result
}

// digitalHumanBindingListerSpy mirrors the organization, owner, host and
// scope/version filtering of bindings.Service.ListTargetBindings. raw mode
// deliberately bypasses that filter so tests can prove the resolver itself
// fails closed on a hostile listing.
type digitalHumanBindingListerSpy struct {
	items      []core.TargetBinding
	calls      int
	lastTarget bindings.TargetIdentity
	raw        bool
}

func (spy *digitalHumanBindingListerSpy) ListTargetBindings(
	_ context.Context,
	organizationID uuid.UUID,
	ownerID uuid.UUID,
	target bindings.TargetIdentity,
) ([]core.TargetBinding, error) {
	spy.calls++
	spy.lastTarget = target
	if organizationID == uuid.Nil || ownerID == uuid.Nil || target.Validate() != nil {
		return nil, core.NewError(core.ErrorCodeInvalidInput, false, nil)
	}
	effective := core.TargetRef{Type: target.Type, Scope: target.Scope}.EffectiveScope()
	matches := []core.TargetBinding{}
	for _, item := range spy.items {
		if !spy.raw && (item.Target.Type != target.Type || item.Target.ID != target.ID ||
			item.OrganizationID != organizationID || item.OwnerAccountID != ownerID ||
			item.Target.EffectiveScope() != effective || item.Target.VersionID != target.VersionID) {
			continue
		}
		matches = append(matches, item)
	}
	return matches, nil
}

type digitalHumanConnectionReaderSpy struct {
	connections map[uuid.UUID]*core.PersonalConnection
	calls       int
}

func (spy *digitalHumanConnectionReaderSpy) GetConnection(
	_ context.Context,
	organizationID uuid.UUID,
	ownerID uuid.UUID,
	connectionID uuid.UUID,
) (core.PersonalConnection, error) {
	spy.calls++
	connection, ok := spy.connections[connectionID]
	if !ok || connection.OrganizationID != organizationID || connection.OwnerAccountID != ownerID {
		return core.PersonalConnection{}, core.NewError(core.ErrorCodeNotFound, false, nil)
	}
	return *connection, nil
}

func TestDelegatedDigitalHumanIdentityActsWithOwnerCredentialForRequester(t *testing.T) {
	harness := newDigitalHumanHarness(t)
	binding := harness.addBinding(t, core.ScopePublished, uuid.NewString())
	binding.Constraint.HostDelegation = &core.HostDelegation{GrantedBy: harness.ownerID, Audiences: []core.HostAudience{core.HostAudienceAPI}}
	harness.bindings.items[len(harness.bindings.items)-1] = binding
	verifier := &delegatedRunAuthorizerSpy{}
	harness.resolver.WithDelegatedHostAccess(&delegatedCandidateSpy{bindings: []core.TargetBinding{binding}}, verifier)
	requester, runID := uuid.New(), uuid.New()
	request := connectordispatch.IdentityResolutionRequest{
		OrganizationID: harness.organizationID, OwnerAccountID: harness.ownerID, ActingPrincipalID: harness.ownerID,
		ConnectorID: binding.ConnectorID, Target: binding.Target, OperationID: harness.operations[0],
		LogicalOperationID: "logical", CorrelationID: "correlation", Interactive: true,
		Delegation: &connectordispatch.DelegatedUse{RequesterID: requester, RunID: runID, BindingID: binding.ID, Audience: core.HostAudienceAPI},
	}
	identity, err := harness.resolver.ResolveConnectorIdentity(t.Context(), request)
	if err != nil || identity.OwnerAccountID != harness.ownerID || identity.ActingPrincipalID != harness.ownerID ||
		identity.DelegatedRequesterID != requester || identity.BindingID != binding.ID || identity.ConnectionID != binding.ConnectionID {
		t.Fatalf("delegated identity = %#v, %v", identity, err)
	}
	if verifier.last.RequesterID != requester || verifier.last.RunID != runID || verifier.last.BindingID != binding.ID {
		t.Fatalf("the run was not authorized for the requester: %#v", verifier.last)
	}

	for name, mutate := range map[string]func(*connectordispatch.IdentityResolutionRequest){
		"other audience": func(r *connectordispatch.IdentityResolutionRequest) { r.Delegation.Audience = core.HostAudienceChannel },
		"other binding":  func(r *connectordispatch.IdentityResolutionRequest) { r.Delegation.BindingID = uuid.New() },
		"other owner": func(r *connectordispatch.IdentityResolutionRequest) {
			r.OwnerAccountID, r.ActingPrincipalID = uuid.New(), uuid.Nil
		},
		"requester owner": func(r *connectordispatch.IdentityResolutionRequest) { r.Delegation.RequesterID = harness.ownerID },
		"missing binding": func(r *connectordispatch.IdentityResolutionRequest) { r.Delegation.BindingID = uuid.Nil },
	} {
		denied := request
		delegation := *request.Delegation
		denied.Delegation = &delegation
		mutate(&denied)
		if denied.ActingPrincipalID == uuid.Nil {
			denied.ActingPrincipalID = denied.OwnerAccountID
		}
		if _, err := harness.resolver.ResolveConnectorIdentity(t.Context(), denied); err == nil {
			t.Fatalf("%s resolved a delegated identity", name)
		}
	}
	verifier.err = core.NewError(core.ErrorCodeAccessDenied, false, nil)
	if _, err := harness.resolver.ResolveConnectorIdentity(t.Context(), request); !core.IsErrorCode(err, core.ErrorCodeAccessDenied) {
		t.Fatalf("a run the requester does not hold resolved: %v", err)
	}
}
