// Package dispatch adapts product-surface invocation identities to the single
// Connector execution Router. It is intentionally transport-neutral: HTTP,
// chat, Agent, and Workflow code may carry the typed identity, but only this
// adapter constructs an execution request.
package dispatch

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/toolgovernance"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"github.com/zgiai/zgi/api/internal/modules/connectors/execution"
)

// Identity is the complete non-secret identity of one Connector operation.
// Input is deliberately kept on Invocation so identities can be logged or
// compared without accidentally retaining operation content.
type Identity struct {
	ConversationID     string
	OrganizationID     uuid.UUID
	OwnerAccountID     uuid.UUID
	ActingPrincipalID  uuid.UUID
	BindingID          uuid.UUID
	ConnectionID       uuid.UUID
	ConnectorID        core.Identifier
	Target             core.TargetRef
	OperationID        core.Identifier
	LogicalOperationID string
	CorrelationID      string
	Interactive        bool
	Confirmation       *execution.RuntimeConfirmationCommand
	ActionStepID       string
	PlanPhaseID        string
}

// IdentityResolutionRequest is the complete non-secret authorization input a
// product surface supplies immediately before one Connector-native Tool call.
// Resolvers must re-read the authoritative Target Binding and Connection;
// request metadata, UI preferences, and model-authored arguments are never
// sufficient authorization.
type IdentityResolutionRequest struct {
	ConversationID     string
	OrganizationID     uuid.UUID
	OwnerAccountID     uuid.UUID
	ActingPrincipalID  uuid.UUID
	ConnectorID        core.Identifier
	Target             core.TargetRef
	OperationID        core.Identifier
	LogicalOperationID string
	CorrelationID      string
	Interactive        bool
	Confirmation       *execution.RuntimeConfirmationCommand
}

// IdentityResolver resolves a current, owner-scoped Connector identity for a
// single surface invocation. It is intentionally narrower than Router: it
// cannot receive provider input or execute an operation.
type IdentityResolver interface {
	ResolveConnectorIdentity(context.Context, IdentityResolutionRequest) (Identity, error)
}

// Invocation adds the opaque provider input to one typed surface identity.
// Neither this package nor the Router persists Input in route claims.
type Invocation struct {
	Identity      Identity
	SessionGrants []toolgovernance.SessionGrant `json:"-"`
	Input         any                           `json:"-"`
}

// Router is the only execution dependency admitted by the surface adapter.
// In production it is implemented by execution.Router.
type Router interface {
	Route(context.Context, execution.ExecutionRequest) (execution.ExecutionResult, error)
}

// Dispatcher is the narrow contract used by Connector-native Tool providers.
type Dispatcher interface {
	Dispatch(context.Context, Invocation) (execution.ExecutionResult, error)
}

// Adapter converts one surface identity and invokes the Router exactly once.
// It never imports or probes the legacy Executor.
type Adapter struct {
	router Router
}

func New(router Router) (*Adapter, error) {
	if router == nil {
		return nil, core.NewError(core.ErrorCodeInternal, false, nil)
	}
	return &Adapter{router: router}, nil
}

func (adapter *Adapter) Dispatch(
	ctx context.Context,
	invocation Invocation,
) (execution.ExecutionResult, error) {
	if adapter == nil || adapter.router == nil {
		return execution.ExecutionResult{}, core.NewError(core.ErrorCodeInternal, false, nil)
	}
	if err := validateIdentity(invocation.Identity); err != nil {
		return execution.ExecutionResult{}, err
	}

	identity := invocation.Identity
	return adapter.router.Route(ctx, execution.ExecutionRequest{
		ConversationID: identity.ConversationID,
		OrganizationID: identity.OrganizationID, OwnerAccountID: identity.OwnerAccountID,
		ActingPrincipalID: identity.ActingPrincipalID, BindingID: identity.BindingID,
		ConnectionID: identity.ConnectionID, ConnectorID: identity.ConnectorID,
		Target: identity.Target, OperationID: identity.OperationID,
		LogicalOperationID: identity.LogicalOperationID, CorrelationID: identity.CorrelationID,
		Interactive: identity.Interactive, Confirmation: cloneConfirmation(identity.Confirmation),
		SessionGrants: invocation.SessionGrants,
		Input:         invocation.Input,
	})
}

func validateIdentity(identity Identity) error {
	if identity.OrganizationID == uuid.Nil || identity.OwnerAccountID == uuid.Nil ||
		identity.ActingPrincipalID == uuid.Nil || identity.OwnerAccountID != identity.ActingPrincipalID ||
		identity.BindingID == uuid.Nil || identity.ConnectionID == uuid.Nil ||
		!identity.ConnectorID.Valid() || !identity.OperationID.Valid() ||
		identity.Target.Validate() != nil || !validOpaque(identity.Target.ID, 255) ||
		(identity.Target.BindingInstanceID != "" && !validOpaque(identity.Target.BindingInstanceID, 255)) ||
		!validOpaque(identity.LogicalOperationID, 128) || !validOpaque(identity.CorrelationID, 128) ||
		(identity.Confirmation != nil && !identity.Interactive) {
		return core.NewError(core.ErrorCodeInvalidInput, false, nil)
	}
	return nil
}

func validOpaque(value string, maximum int) bool {
	return value != "" && len(value) <= maximum && value == strings.TrimSpace(value) &&
		!strings.ContainsAny(value, "\x00\r\n")
}

func cloneConfirmation(
	confirmation *execution.RuntimeConfirmationCommand,
) *execution.RuntimeConfirmationCommand {
	if confirmation == nil {
		return nil
	}
	cloned := *confirmation
	return &cloned
}

type identityContextKey struct{}

// WithIdentity carries a typed Connector identity through the generic Tool
// interface. Input remains the Tool method argument and is never put in the
// context.
func WithIdentity(ctx context.Context, identity Identity) context.Context {
	return context.WithValue(ctx, identityContextKey{}, identity)
}

// IdentityFromContext returns a defensive copy of the typed identity.
func IdentityFromContext(ctx context.Context) (Identity, bool) {
	if ctx == nil {
		return Identity{}, false
	}
	identity, ok := ctx.Value(identityContextKey{}).(Identity)
	if !ok {
		return Identity{}, false
	}
	identity.Confirmation = cloneConfirmation(identity.Confirmation)
	return identity, true
}
