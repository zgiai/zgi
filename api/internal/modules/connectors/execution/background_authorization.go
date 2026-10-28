package execution

import (
	"context"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/serverops"
	"github.com/zgiai/zgi/api/internal/modules/connectors/catalog"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
)

type backgroundIdentityContextKey struct{}

// BackgroundIdentity is server-authored provenance for one noninteractive
// DigitalHuman invocation. It is carried in context rather than request/input
// so tool arguments and model output cannot manufacture it.
type BackgroundIdentity struct {
	RunID     uuid.UUID
	MessageID uuid.UUID
}

func WithBackgroundIdentity(ctx context.Context, runID, messageID uuid.UUID) context.Context {
	return context.WithValue(ctx, backgroundIdentityContextKey{}, BackgroundIdentity{RunID: runID, MessageID: messageID})
}

func BackgroundIdentityFromContext(ctx context.Context) (BackgroundIdentity, bool) {
	identity, ok := ctx.Value(backgroundIdentityContextKey{}).(BackgroundIdentity)
	return identity, ok && identity.RunID != uuid.Nil && identity.MessageID != uuid.Nil
}

// BackgroundAuthorizer is an injected server boundary, never an argument in a
// tool request. It must restore live task/employee authority and atomically
// consume an owner-issued bounded grant before returning true.
type BackgroundAuthorizer interface {
	AuthorizeBackground(context.Context, BackgroundAuthorizationRequest) (bool, error)
}
type BackgroundAuthorizationRequest struct {
	Request        ExecutionRequest
	Binding        core.TargetBinding
	Connection     core.PersonalConnection
	Operation      catalog.Operation
	ContractDigest string
	// Trusted means the binding's owner trusted it for unattended runs (ADR
	// 0028): the live task run and its owner are still restored, but no task
	// grant is consumed.
	Trusted bool
}

// CommandRuleSource returns an organization's server command rules (ADR
// 0029).
type CommandRuleSource interface {
	CommandRules(context.Context, uuid.UUID) ([]serverops.CommandRule, error)
}

// WithCommandRules configures the organization rules every SSH call is checked
// against. Nil checks none.
func (s *Service) WithCommandRules(source CommandRuleSource) *Service {
	s.commandRules = source
	return s
}

// WithBackgroundAuthorizer configures the service at composition time. Nil
// preserves the legacy per-invocation confirmation boundary.
func (s *Service) WithBackgroundAuthorizer(authorizer BackgroundAuthorizer) *Service {
	s.backgroundAuthorizer = authorizer
	return s
}
func (s *Service) authorizeBackground(ctx context.Context, pinned PinnedExecution, binding core.TargetBinding, connection core.PersonalConnection, accepted catalog.PinnedOperation, trusted bool) (bool, error) {
	r := pinned.Request
	if s.backgroundAuthorizer == nil || r.Interactive || r.Confirmation != nil || r.Target.Type != core.TargetTypeDigitalHuman || r.Target.EffectiveScope() != core.ScopePublished {
		return false, nil
	}
	return s.backgroundAuthorizer.AuthorizeBackground(ctx, BackgroundAuthorizationRequest{Request: r, Binding: binding, Connection: connection, Operation: accepted.Operation, ContractDigest: accepted.ContractDigest, Trusted: trusted})
}
