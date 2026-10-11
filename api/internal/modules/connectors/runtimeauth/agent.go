package runtimeauth

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/catalog"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	connectordispatch "github.com/zgiai/zgi/api/internal/modules/connectors/dispatch"
)

// AgentCapabilityRequest identifies the owner-scoped Agent target whose
// native Connector operations may be projected into an interactive Agent
// conversation. Persisted Agent mode JSON is deliberately not authorization.
type AgentCapabilityRequest struct {
	OrganizationID uuid.UUID
	OwnerAccountID uuid.UUID
	AgentID        string
	Scope          core.TargetScope
	VersionID      string
}

// AgentRuntimeResolver reuses the same current binding, connection, and
// catalog checks as AI Chat while retaining an Agent-specific target type.
type AgentRuntimeResolver interface {
	connectordispatch.IdentityResolver
	ResolveAgentCapabilities(context.Context, AgentCapabilityRequest) (ChatCapabilitySnapshot, error)
}

type DelegatedAgentRuntimeResolver interface {
	ResolveDelegatedCapabilities(context.Context, DelegatedHostRequest) (ChatCapabilitySnapshot, error)
}

type AgentResolver struct {
	base       *ChatResolver
	candidates HostGrantCandidateReader
	authorizer DelegatedRunAuthorizer
}

var _ AgentRuntimeResolver = (*AgentResolver)(nil)

func NewAgentResolver(
	bindingLister TargetBindingLister,
	connectionReader ConnectionReader,
	definitionCatalog DefinitionCatalog,
) (*AgentResolver, error) {
	base, err := NewChatResolver(bindingLister, connectionReader, definitionCatalog)
	if err != nil {
		return nil, err
	}
	return &AgentResolver{base: base}, nil
}

func (resolver *AgentResolver) WithDelegatedHostAccess(candidates HostGrantCandidateReader, authorizer DelegatedRunAuthorizer) *AgentResolver {
	if resolver != nil {
		resolver.candidates, resolver.authorizer = candidates, authorizer
	}
	return resolver
}

func (resolver *AgentResolver) ResolveDelegatedCapabilities(ctx context.Context, request DelegatedHostRequest) (ChatCapabilitySnapshot, error) {
	if resolver == nil || resolver.base == nil || request.Target.Type != core.TargetTypeAgent {
		return ChatCapabilitySnapshot{}, core.NewError(core.ErrorCodeAccessDenied, false, nil)
	}
	return resolver.base.resolveDelegatedHostCapabilities(ctx, request, resolver.candidates, resolver.authorizer, catalog.SurfaceAgent)
}

func (resolver *AgentResolver) ResolveAgentCapabilities(
	ctx context.Context,
	request AgentCapabilityRequest,
) (ChatCapabilitySnapshot, error) {
	if resolver == nil || resolver.base == nil {
		return ChatCapabilitySnapshot{}, core.NewError(core.ErrorCodeInternal, false, nil)
	}
	agentID := strings.TrimSpace(request.AgentID)
	if request.OrganizationID == uuid.Nil || request.OwnerAccountID == uuid.Nil ||
		agentID == "" || agentID != request.AgentID {
		return ChatCapabilitySnapshot{}, core.NewError(core.ErrorCodeInvalidInput, false, nil)
	}
	if parsed, err := uuid.Parse(agentID); err != nil || parsed == uuid.Nil {
		return ChatCapabilitySnapshot{}, core.NewError(core.ErrorCodeInvalidInput, false, nil)
	}
	return resolver.base.resolveTargetCapabilities(
		ctx,
		request.OrganizationID,
		request.OwnerAccountID,
		core.TargetRef{Type: core.TargetTypeAgent, ID: agentID, Scope: request.Scope, VersionID: request.VersionID},
		catalog.SurfaceAgent,
	)
}

// ResolveConnectorIdentity is used only by an interactive Agent conversation.
// Unattended Agent jobs must use their own runtime resolver and pass
// Interactive=false so high-risk and unclassified operations remain blocked.
func (resolver *AgentResolver) ResolveConnectorIdentity(
	ctx context.Context,
	request connectordispatch.IdentityResolutionRequest,
) (connectordispatch.Identity, error) {
	if request.Target.Type != core.TargetTypeAgent || request.Target.BindingInstanceID != "" ||
		request.OrganizationID == uuid.Nil || request.OwnerAccountID == uuid.Nil ||
		request.ActingPrincipalID != request.OwnerAccountID || !request.ConnectorID.Valid() ||
		!request.OperationID.Valid() || strings.TrimSpace(request.LogicalOperationID) == "" ||
		strings.TrimSpace(request.CorrelationID) == "" || !request.Interactive {
		return connectordispatch.Identity{}, core.NewError(core.ErrorCodeInvalidInput, false, nil)
	}
	snapshot, err := resolver.ResolveAgentCapabilities(ctx, AgentCapabilityRequest{
		OrganizationID: request.OrganizationID,
		OwnerAccountID: request.OwnerAccountID,
		AgentID:        request.Target.ID, Scope: request.Target.Scope, VersionID: request.Target.VersionID,
	})
	if err != nil {
		return connectordispatch.Identity{}, err
	}
	for _, capability := range snapshot.Capabilities {
		if capability.ConnectorID != request.ConnectorID {
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
