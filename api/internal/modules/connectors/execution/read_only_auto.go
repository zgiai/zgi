package execution

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/serverops"
	"github.com/zgiai/zgi/api/internal/modules/connectors/catalog"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
)

// readOnlyAutoInvocation reports whether a High-risk call may run as
// trusted-low under ADR 0026: the target is a DigitalHuman, its binding chose
// read_only_auto, the connector is the exact SSH Server Operations built-in,
// and the call with its exact input is proven read-only. Everything else keeps
// its classification, so a change still asks and an unattended run is still
// governed by task authorization.
func readOnlyAutoInvocation(request ExecutionRequest, binding core.TargetBinding, definition catalog.Definition) bool {
	if request.Target.Type != core.TargetTypeDigitalHuman || !binding.Constraint.ReadOnlyAutoAuthorized() ||
		!definition.IsServerOpsSSHCredentials() {
		return false
	}
	input, ok := invocationInputMap(request.Input)
	return ok && serverops.IsReadOnlyInvocation(string(request.OperationID), input)
}

// invocationInputMap returns the call input as the JSON object the provider
// receives.
func invocationInputMap(input any) (map[string]any, bool) {
	if values, ok := input.(map[string]any); ok {
		return values, true
	}
	if input == nil {
		return map[string]any{}, true
	}
	data, err := json.Marshal(input)
	if err != nil {
		return nil, false
	}
	var values map[string]any
	if json.Unmarshal(data, &values) != nil || values == nil {
		return nil, false
	}
	return values, true
}

// unattendedInvocationAllowed reports whether an unattended call may consume
// the owner's task grant at all (ADR 0027). An SSH Server Operations call must
// be proven read-only: a change always needs a person, whatever was granted.
func unattendedInvocationAllowed(request ExecutionRequest, definition catalog.Definition) bool {
	if !definition.IsServerOpsSSHCredentials() {
		return true
	}
	input, ok := invocationInputMap(request.Input)
	return ok && serverops.IsReadOnlyInvocation(string(request.OperationID), input)
}

type presentRequesterContextKey struct{}

// WithPresentRequester records the account of the person present in an
// interactive console conversation. It is server-authored context, never tool
// input, and only a present owner is trusted (ADR 0028).
func WithPresentRequester(ctx context.Context, accountID uuid.UUID) context.Context {
	return context.WithValue(ctx, presentRequesterContextKey{}, accountID)
}

func presentRequester(ctx context.Context) uuid.UUID {
	accountID, _ := ctx.Value(presentRequesterContextKey{}).(uuid.UUID)
	return accountID
}

// trustedAuthorizationFresh reports whether a trusted binding is still pinned
// to the current connection revision, credential, and contract.
func trustedAuthorizationFresh(constraint *core.AccessConstraint, connection core.PersonalConnection, contractDigest string) bool {
	if !constraint.Trusted() || constraint.AuthorizationConnectionRevision < 1 || constraint.AuthorizationCredentialVersion < 1 ||
		constraint.AuthorizationContractDigest == "" {
		return false
	}
	return constraint.AuthorizationConnectionRevision == connection.Revision &&
		constraint.AuthorizationCredentialVersion == connection.CredentialVersion &&
		constraint.AuthorizationContractDigest == contractDigest
}

// trustedOwnerInvocation reports whether an interactive SSH call runs without
// asking because its binding is trusted and its owner is the person present.
func trustedOwnerInvocation(ctx context.Context, request ExecutionRequest, binding core.TargetBinding, connection core.PersonalConnection, definition catalog.Definition, contractDigest string) bool {
	return request.Target.Type == core.TargetTypeDigitalHuman && definition.IsServerOpsSSHCredentials() &&
		presentRequester(ctx) == request.OwnerAccountID && request.OwnerAccountID != uuid.Nil &&
		trustedAuthorizationFresh(binding.Constraint, connection, contractDigest)
}

// trustedUnattendedInvocation reports whether an unattended SSH call may run
// in the owner's task run without a task grant (ADR 0028). The background
// authorizer still restores the live task run and its owner.
func trustedUnattendedInvocation(request ExecutionRequest, binding core.TargetBinding, connection core.PersonalConnection, definition catalog.Definition, contractDigest string) bool {
	return !request.Interactive && request.Target.Type == core.TargetTypeDigitalHuman && definition.IsServerOpsSSHCredentials() &&
		binding.Constraint.TrustedUnattended() && trustedAuthorizationFresh(binding.Constraint, connection, contractDigest)
}

// commandRuleAction matches an SSH call against its organization's command
// rules (ADR 0029). A rule store that cannot be read fails closed.
func (service *Service) commandRuleAction(ctx context.Context, request ExecutionRequest, definition catalog.Definition) (serverops.RuleAction, error) {
	if service.commandRules == nil || !definition.IsServerOpsSSHCredentials() {
		return serverops.RuleNone, nil
	}
	rules, err := service.commandRules.CommandRules(ctx, request.OrganizationID)
	if err != nil {
		return serverops.RuleNone, connectorError(core.ErrorCodeConnectorUnavailable, true)
	}
	input, ok := invocationInputMap(request.Input)
	if !ok {
		return serverops.RuleNone, connectorError(core.ErrorCodeInvalidInput, false)
	}
	return serverops.MatchCommandRules(string(request.OperationID), input, rules), nil
}
