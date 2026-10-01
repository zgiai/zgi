// Package runtimeauth resolves product-surface authorization into typed,
// non-secret Connector dispatch identities. It never receives provider input,
// opens a credential, or invokes a provider.
package runtimeauth

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/bindings"
	"github.com/zgiai/zgi/api/internal/modules/connectors/catalog"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	connectordispatch "github.com/zgiai/zgi/api/internal/modules/connectors/dispatch"
)

// TargetBindingLister must retain the host edit-authority check performed by
// bindings.Service. Reading the repository directly would make a persisted row
// look usable after the owner loses access to its conversation.
type TargetBindingLister interface {
	ListTargetBindings(context.Context, uuid.UUID, uuid.UUID, bindings.TargetIdentity) ([]core.TargetBinding, error)
}

type ConnectionReader interface {
	GetConnection(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) (core.PersonalConnection, error)
}

type DefinitionCatalog interface {
	Get(string) (catalog.Definition, bool)
}

type ChatCapabilityRequest struct {
	OrganizationID uuid.UUID
	OwnerAccountID uuid.UUID
	ConversationID string
}

// ChatCapability contains only the safe catalog and stable relationship
// identities required to expose currently usable operations. A Connection's
// encrypted credential envelope is deliberately not retained here.
type ChatCapability struct {
	BindingID    uuid.UUID
	ConnectionID uuid.UUID
	ConnectorID  core.Identifier
	Definition   catalog.Definition
	Operations   []catalog.Operation
}

type ChatCapabilitySnapshot struct {
	Capabilities []ChatCapability
}

func (snapshot ChatCapabilitySnapshot) HasOperations() bool {
	for _, capability := range snapshot.Capabilities {
		if len(capability.Operations) > 0 {
			return true
		}
	}
	return false
}

// ChatRuntimeResolver is the dependency accepted by AI Chat. Implementations
// must resolve both discovery and invocation from the same authoritative data.
type ChatRuntimeResolver interface {
	connectordispatch.IdentityResolver
	ResolveChatCapabilities(context.Context, ChatCapabilityRequest) (ChatCapabilitySnapshot, error)
}

type ChatResolver struct {
	bindings    TargetBindingLister
	connections ConnectionReader
	catalog     DefinitionCatalog
}

var _ ChatRuntimeResolver = (*ChatResolver)(nil)

func NewChatResolver(
	bindingLister TargetBindingLister,
	connectionReader ConnectionReader,
	definitionCatalog DefinitionCatalog,
) (*ChatResolver, error) {
	if bindingLister == nil || connectionReader == nil || definitionCatalog == nil {
		return nil, core.NewError(core.ErrorCodeInternal, false, nil)
	}
	return &ChatResolver{
		bindings: bindingLister, connections: connectionReader, catalog: definitionCatalog,
	}, nil
}

// ResolveChatCapabilities returns only operations backed by a current active
// Target Binding and a ready owner-scoped Personal Connection.
func (resolver *ChatResolver) ResolveChatCapabilities(
	ctx context.Context,
	request ChatCapabilityRequest,
) (ChatCapabilitySnapshot, error) {
	if resolver == nil || resolver.bindings == nil || resolver.connections == nil || resolver.catalog == nil {
		return ChatCapabilitySnapshot{}, core.NewError(core.ErrorCodeInternal, false, nil)
	}
	conversationID := strings.TrimSpace(request.ConversationID)
	if request.OrganizationID == uuid.Nil || request.OwnerAccountID == uuid.Nil ||
		conversationID == "" || conversationID != request.ConversationID {
		return ChatCapabilitySnapshot{}, core.NewError(core.ErrorCodeInvalidInput, false, nil)
	}
	if parsed, err := uuid.Parse(conversationID); err != nil || parsed == uuid.Nil {
		return ChatCapabilitySnapshot{}, core.NewError(core.ErrorCodeInvalidInput, false, nil)
	}
	return resolver.resolveTargetCapabilities(
		ctx,
		request.OrganizationID,
		request.OwnerAccountID,
		core.TargetRef{Type: core.TargetTypeChat, ID: conversationID},
		catalog.SurfaceChat,
	)
}

func (resolver *ChatResolver) resolveTargetCapabilities(
	ctx context.Context,
	organizationID uuid.UUID,
	ownerAccountID uuid.UUID,
	target core.TargetRef,
	surface catalog.Surface,
) (ChatCapabilitySnapshot, error) {
	if resolver == nil || resolver.bindings == nil || resolver.connections == nil || resolver.catalog == nil ||
		organizationID == uuid.Nil || ownerAccountID == uuid.Nil || target.Validate() != nil {
		return ChatCapabilitySnapshot{}, core.NewError(core.ErrorCodeInvalidInput, false, nil)
	}

	items, err := resolver.bindings.ListTargetBindings(
		ctx,
		organizationID,
		ownerAccountID,
		bindings.TargetIdentity{Type: target.Type, ID: target.ID, Scope: target.Scope, VersionID: target.VersionID},
	)
	if err != nil {
		return ChatCapabilitySnapshot{}, err
	}

	capabilities := make([]ChatCapability, 0, len(items))
	seenConnectors := make(map[core.Identifier]struct{}, len(items))
	for _, binding := range items {
		if binding.Status != core.BindingStatusActive {
			continue
		}
		if !binding.Target.SameIdentity(target) ||
			binding.OrganizationID != organizationID || binding.OwnerAccountID != ownerAccountID {
			return ChatCapabilitySnapshot{}, core.NewError(core.ErrorCodeInternal, false, nil)
		}
		if _, duplicate := seenConnectors[binding.ConnectorID]; duplicate {
			return ChatCapabilitySnapshot{}, core.NewError(core.ErrorCodeConflict, false, nil)
		}

		connection, err := resolver.connections.GetConnection(
			ctx, organizationID, ownerAccountID, binding.ConnectionID,
		)
		if err != nil {
			return ChatCapabilitySnapshot{}, err
		}
		if connection.ID != binding.ConnectionID || connection.OrganizationID != organizationID ||
			connection.OwnerAccountID != ownerAccountID || connection.ConnectorID != binding.ConnectorID {
			return ChatCapabilitySnapshot{}, core.NewError(core.ErrorCodeInternal, false, nil)
		}
		if connection.State != core.ReadinessReady {
			continue
		}

		definition, ok := resolver.catalog.Get(binding.ConnectorID.String())
		if !ok || catalog.Validate(definition) != nil || !definition.Supports(surface) {
			continue
		}
		operations := permittedOperations(definition, connection, binding.Constraint, ownerAccountID, target, time.Now())
		if len(operations) == 0 {
			continue
		}
		seenConnectors[binding.ConnectorID] = struct{}{}
		capabilities = append(capabilities, ChatCapability{
			BindingID: binding.ID, ConnectionID: binding.ConnectionID,
			ConnectorID: binding.ConnectorID, Definition: definition, Operations: operations,
		})
	}
	sort.Slice(capabilities, func(i, j int) bool {
		return capabilities[i].ConnectorID.String() < capabilities[j].ConnectorID.String()
	})
	return ChatCapabilitySnapshot{Capabilities: capabilities}, nil
}

// Both first-chat initialization and ordinary discovery use this exact filter;
// ready metadata alone must never turn a disabled operation into authority.
func permittedOperations(definition catalog.Definition, connection core.PersonalConnection, bindingConstraint *core.AccessConstraint, owner uuid.UUID, target core.TargetRef, now time.Time) []catalog.Operation {
	usable := make(map[string]struct{}, len(connection.UsableOperationIDs))
	for _, operationID := range connection.UsableOperationIDs {
		if operationID.Valid() {
			usable[operationID.String()] = struct{}{}
		}
	}
	operations := make([]catalog.Operation, 0, len(definition.Operations))
	for _, operation := range definition.Operations {
		if _, allowed := usable[operation.ID]; !allowed {
			continue
		}
		if _, unavailable := connection.UnavailableOperations[core.Identifier(operation.ID)]; unavailable {
			continue
		}
		if !connection.Constraint.Allows(owner, core.Identifier(operation.ID), target, now) || !bindingConstraint.Allows(owner, core.Identifier(operation.ID), target, now) {
			continue
		}
		operations = append(operations, operation)
	}
	return operations
}

// ResolveConnectorIdentity re-reads the same capability snapshot immediately
// before dispatch. A stale schema exposed earlier in the turn therefore cannot
// outlive detach, reconnect failure, owner loss, or operation disablement.
func (resolver *ChatResolver) ResolveConnectorIdentity(
	ctx context.Context,
	request connectordispatch.IdentityResolutionRequest,
) (connectordispatch.Identity, error) {
	if request.Target.Type != core.TargetTypeChat || request.Target.BindingInstanceID != "" ||
		request.OrganizationID == uuid.Nil || request.OwnerAccountID == uuid.Nil ||
		request.ActingPrincipalID != request.OwnerAccountID || !request.ConnectorID.Valid() ||
		!request.OperationID.Valid() || strings.TrimSpace(request.LogicalOperationID) == "" ||
		strings.TrimSpace(request.CorrelationID) == "" || !request.Interactive {
		return connectordispatch.Identity{}, core.NewError(core.ErrorCodeInvalidInput, false, nil)
	}
	snapshot, err := resolver.ResolveChatCapabilities(ctx, ChatCapabilityRequest{
		OrganizationID: request.OrganizationID,
		OwnerAccountID: request.OwnerAccountID,
		ConversationID: request.Target.ID,
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
			identity := connectordispatch.Identity{
				ConversationID: request.ConversationID,
				OrganizationID: request.OrganizationID, OwnerAccountID: request.OwnerAccountID,
				ActingPrincipalID: request.ActingPrincipalID, BindingID: capability.BindingID,
				ConnectionID: capability.ConnectionID, ConnectorID: capability.ConnectorID,
				Target: request.Target, OperationID: request.OperationID,
				LogicalOperationID: request.LogicalOperationID, CorrelationID: request.CorrelationID,
				Interactive: request.Interactive, Confirmation: request.Confirmation,
			}
			return identity, nil
		}
	}
	return connectordispatch.Identity{}, core.NewError(core.ErrorCodeTargetNotReady, false, nil)
}
