package runtimeauth

import (
	"context"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/catalog"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	connectordispatch "github.com/zgiai/zgi/api/internal/modules/connectors/dispatch"
)

// digitalHumanTargetInstance is the validation sentinel that
// core.TargetRef.Validate requires for a DigitalHuman target. A DigitalHuman
// host is identified by organization, owner, host id and scope/version only;
// its concrete resource binding instances are enumerated per binding by the
// shared resolver and are never pinned or supplied by the caller.
const digitalHumanTargetInstance = "00000000-0000-0000-0000-000000000001"

// DigitalHumanCapabilityRequest identifies the owner-scoped DigitalHuman host
// whose native Connector operations may be projected into a DigitalHuman run.
// Persisted host resource rows are deliberately not authorization.
type DigitalHumanCapabilityRequest struct {
	OrganizationID uuid.UUID
	OwnerAccountID uuid.UUID
	DigitalHumanID string
	Scope          core.TargetScope
	VersionID      string
}

// DigitalHumanRuntimeResolver resolves both discovery and invocation from the
// same authoritative Target Bindings and Personal Connections.
type DigitalHumanRuntimeResolver interface {
	connectordispatch.IdentityResolver
	ResolveDigitalHumanCapabilities(context.Context, DigitalHumanCapabilityRequest) (ChatCapabilitySnapshot, error)
}

type DelegatedDigitalHumanRuntimeResolver interface {
	ResolveDelegatedCapabilities(context.Context, DelegatedHostRequest) (ChatCapabilitySnapshot, error)
}

// DigitalHumanResolver keeps the host type, its exact scope/version and every
// resource binding instance intact. The injected lister owns current host
// permission checks, so a persisted draft row cannot outlive owner loss.
type DigitalHumanResolver struct {
	base       *ChatResolver
	candidates HostGrantCandidateReader
	authorizer DelegatedRunAuthorizer
}

var _ DigitalHumanRuntimeResolver = (*DigitalHumanResolver)(nil)

func NewDigitalHumanResolver(
	bindingLister TargetBindingLister,
	connectionReader ConnectionReader,
	definitionCatalog DefinitionCatalog,
) (*DigitalHumanResolver, error) {
	base, err := NewChatResolver(bindingLister, connectionReader, definitionCatalog)
	if err != nil {
		return nil, err
	}
	return &DigitalHumanResolver{base: base}, nil
}

func (resolver *DigitalHumanResolver) WithDelegatedHostAccess(candidates HostGrantCandidateReader, authorizer DelegatedRunAuthorizer) *DigitalHumanResolver {
	if resolver != nil {
		resolver.candidates, resolver.authorizer = candidates, authorizer
	}
	return resolver
}

func (resolver *DigitalHumanResolver) ResolveDelegatedCapabilities(ctx context.Context, request DelegatedHostRequest) (ChatCapabilitySnapshot, error) {
	if resolver == nil || resolver.base == nil || request.Target.Type != core.TargetTypeDigitalHuman {
		return ChatCapabilitySnapshot{}, core.NewError(core.ErrorCodeAccessDenied, false, nil)
	}
	return resolver.base.resolveDelegatedHostCapabilities(ctx, request, resolver.candidates, resolver.authorizer, catalog.SurfaceDigitalHuman)
}

// ResolveDigitalHumanCapabilities returns only operations backed by a current
// active DigitalHuman Target Binding and a ready owner-scoped Personal
// Connection. The shared resolver compares each persisted binding instance
// against the requested host and version, so one Connector may be bound more
// than once while a binding from another scope, version or owner is rejected.
func (resolver *DigitalHumanResolver) ResolveDigitalHumanCapabilities(
	ctx context.Context,
	request DigitalHumanCapabilityRequest,
) (ChatCapabilitySnapshot, error) {
	if resolver == nil || resolver.base == nil {
		return ChatCapabilitySnapshot{}, core.NewError(core.ErrorCodeInternal, false, nil)
	}
	return resolver.base.resolveTargetCapabilities(
		ctx,
		request.OrganizationID,
		request.OwnerAccountID,
		core.TargetRef{
			Type: core.TargetTypeDigitalHuman, ID: request.DigitalHumanID,
			Scope: request.Scope, VersionID: request.VersionID,
			BindingInstanceID: digitalHumanTargetInstance,
		},
		catalog.SurfaceDigitalHuman,
	)
}

// ResolveConnectorIdentity re-reads the same capability snapshot immediately
// before dispatch and selects the exact persisted binding instance named by the
// request. A stale discovery snapshot therefore cannot outlive detach, owner
// loss, operation disablement, or a replacement of the host version.
func (resolver *DigitalHumanResolver) ResolveConnectorIdentity(
	ctx context.Context,
	request connectordispatch.IdentityResolutionRequest,
) (connectordispatch.Identity, error) {
	if request.Target.Type != core.TargetTypeDigitalHuman || request.Target.Validate() != nil ||
		request.OrganizationID == uuid.Nil || request.OwnerAccountID == uuid.Nil ||
		request.ActingPrincipalID != request.OwnerAccountID || !request.ConnectorID.Valid() ||
		!request.OperationID.Valid() || request.LogicalOperationID == "" || request.CorrelationID == "" {
		return connectordispatch.Identity{}, core.NewError(core.ErrorCodeInvalidInput, false, nil)
	}
	if request.Delegation != nil {
		return resolver.base.resolveDelegatedIdentity(ctx, request, resolver.candidates, resolver.authorizer, catalog.SurfaceDigitalHuman, digitalHumanTargetInstance)
	}
	snapshot, err := resolver.ResolveDigitalHumanCapabilities(ctx, DigitalHumanCapabilityRequest{
		OrganizationID: request.OrganizationID, OwnerAccountID: request.OwnerAccountID,
		DigitalHumanID: request.Target.ID, Scope: request.Target.Scope, VersionID: request.Target.VersionID,
	})
	if err != nil {
		return connectordispatch.Identity{}, err
	}
	for _, capability := range snapshot.Capabilities {
		if capability.ConnectorID != request.ConnectorID || !capability.Target.SameIdentity(request.Target) {
			continue
		}
		for _, operation := range capability.Operations {
			if operation.ID != request.OperationID.String() {
				continue
			}
			return connectordispatch.Identity{
				ConversationID: request.ConversationID,
				OrganizationID: request.OrganizationID, OwnerAccountID: request.OwnerAccountID,
				ActingPrincipalID: request.ActingPrincipalID, BindingID: capability.BindingID,
				ConnectionID: capability.ConnectionID, ConnectorID: capability.ConnectorID,
				Target: request.Target, OperationID: request.OperationID,
				LogicalOperationID: request.LogicalOperationID, CorrelationID: request.CorrelationID,
				Interactive: request.Interactive, Confirmation: request.Confirmation,
			}, nil
		}
	}
	return connectordispatch.Identity{}, core.NewError(core.ErrorCodeTargetNotReady, false, nil)
}
