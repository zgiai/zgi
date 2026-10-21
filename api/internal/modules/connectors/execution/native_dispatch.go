package execution

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
)

// InvocationSnapshot is immutable evidence captured before any submission.
// Digests contain no input or credentials. Versions are checked again at the
// provider boundary and when a confirmation resumes.
type InvocationSnapshot struct {
	InputDigest       string
	ContractDigest    string
	BindingRevision   int
	CredentialVersion int
}

type InvocationClaim struct {
	Identity RouteResolutionRequest
	Snapshot InvocationSnapshot
}

type InvocationStore interface {
	Claim(context.Context, InvocationClaim) (RouteClaimReceipt, error)
	Resume(context.Context, InvocationClaim, RouteClaimReceipt) error
}

type NativeExecutionService interface {
	PrepareInvocation(context.Context, ExecutionRequest) (InvocationSnapshot, error)
	InvokeConnector(context.Context, PinnedExecution) (ExecutionResult, error)
}

// NativeRouter has one provider entry point and one persistent claim. It has
// no ownership lookup, rollout state, alternate path or retry mechanism.
type NativeRouter struct {
	store   InvocationStore
	service NativeExecutionService
}

func NewNativeRouter(store InvocationStore, service NativeExecutionService) (*NativeRouter, error) {
	if store == nil || service == nil {
		return nil, connectorError(core.ErrorCodeInternal, false)
	}
	return &NativeRouter{store: store, service: service}, nil
}

func (router *NativeRouter) Route(ctx context.Context, request ExecutionRequest) (ExecutionResult, error) {
	if router == nil || router.store == nil || router.service == nil || ctx == nil {
		return ExecutionResult{}, connectorError(core.ErrorCodeInternal, false)
	}
	if err := validateExecutionRequest(request); err != nil {
		return UnresolvedResult(core.ErrorCodeOf(err)), err
	}
	// Own a normalized input before hashing it; caller mutation cannot change
	// the request after its claim is persisted.
	encoded, err := json.Marshal(request.Input)
	if err != nil || len(encoded) > 256*1024 {
		return ExecutionResult{}, connectorError(core.ErrorCodeInvalidInput, false)
	}
	defer clear(encoded)
	if json.Unmarshal(encoded, &request.Input) != nil {
		return ExecutionResult{}, connectorError(core.ErrorCodeInvalidInput, false)
	}
	snapshot, err := router.service.PrepareInvocation(ctx, request)
	if err != nil {
		return UnresolvedResult(core.ErrorCodeOf(err)), err
	}
	claim := InvocationClaim{Identity: routeResolutionRequest(request), Snapshot: snapshot}
	var receipt RouteClaimReceipt
	if request.Confirmation == nil {
		receipt, err = router.store.Claim(ctx, claim)
	} else {
		receipt = request.Confirmation.RouteReceipt
		err = router.store.Resume(ctx, claim, receipt)
	}
	if err != nil {
		return router.recoverInvocation(ctx, request, claim, err)
	}
	if !validReceipt(receipt) {
		return ExecutionResult{}, connectorError(core.ErrorCodeInternal, false)
	}
	return router.service.InvokeConnector(ctx, PinnedExecution{Request: request, Receipt: receipt, Path: PathConnectorNative,
		Snapshot: &snapshot, freshClaim: request.Confirmation == nil})
}

func (router *NativeRouter) recoverInvocation(ctx context.Context, request ExecutionRequest, claim InvocationClaim, cause error) (ExecutionResult, error) {
	lookup, ok := router.store.(interface {
		Lookup(context.Context, InvocationClaim) (RouteClaimReceipt, error)
	})
	recoverer, recoverOK := router.service.(interface {
		RecoverInvocation(context.Context, PinnedExecution) (ExecutionResult, error)
	})
	if ok && recoverOK {
		if receipt, err := lookup.Lookup(ctx, claim); err == nil {
			return recoverer.RecoverInvocation(ctx, PinnedExecution{Request: request, Receipt: receipt, Path: PathConnectorNative, Snapshot: &claim.Snapshot})
		}
	}
	return UnresolvedResult(core.ErrorCodeOf(cause)), preserveError(cause, core.ErrorCodeConflict)
}

func (service *Service) PrepareInvocation(ctx context.Context, request ExecutionRequest) (InvocationSnapshot, error) {
	if err := service.available(); err != nil {
		return InvocationSnapshot{}, err
	}
	if validateExecutionRequest(request) != nil || request.ActingPrincipalID != request.OwnerAccountID {
		return InvocationSnapshot{}, connectorError(core.ErrorCodeAccessDenied, false)
	}
	binding, err := service.bindings.GetBinding(ctx, request.OrganizationID, request.OwnerAccountID, request.BindingID)
	if err != nil {
		return InvocationSnapshot{}, preserveError(err, core.ErrorCodeTargetNotReady)
	}
	if err = authorizeBinding(request, binding); err != nil {
		return InvocationSnapshot{}, err
	}
	connection, err := service.connections.GetConnection(ctx, request.OrganizationID, request.OwnerAccountID, request.ConnectionID)
	if err != nil {
		return InvocationSnapshot{}, preserveError(err, core.ErrorCodeConnectionNotReady)
	}
	if err = authorizeConnection(request, binding, connection); err != nil {
		return InvocationSnapshot{}, err
	}
	now := service.now().UTC()
	// The historical Connection constraint predates the DigitalHuman host
	// boundary, so checking each layer independently rejects every published
	// DigitalHuman target even when an owner-created Target Binding pins the
	// exact host. core.ConstraintsAllow preserves that historical Connection
	// boundary while letting the exact persisted binding supply the newly added
	// host boundary; owner, operation, deny and expiry restrictions still apply
	// to both layers. The projection only applies to an exact DigitalHuman
	// target, so Chat, Agent and Workflow keep their existing behavior.
	if !core.ConstraintsAllow(connection.Constraint, binding.Constraint, request.OwnerAccountID, request.OperationID, request.Target, now) {
		return InvocationSnapshot{}, connectorError(core.ErrorCodeAccessDenied, false)
	}
	accepted, ok := acceptedCatalogOperation(service.catalog, request)
	if !ok || authorizeDefinition(request, connection, accepted.Definition) != nil {
		return InvocationSnapshot{}, connectorError(core.ErrorCodeConnectorUnavailable, false)
	}
	if err = service.targetAuthorizer.AuthorizeTarget(ctx, TargetAuthorizationRequest{OrganizationID: request.OrganizationID, OwnerAccountID: request.OwnerAccountID, ActingPrincipalID: request.ActingPrincipalID, Binding: binding, Connection: connection, OperationID: request.OperationID, Interactive: request.Interactive}); err != nil {
		return InvocationSnapshot{}, preserveError(err, core.ErrorCodeAccessDenied)
	}
	digest, err := invocationInputDigest(request.Input)
	if err != nil {
		return InvocationSnapshot{}, err
	}
	return InvocationSnapshot{InputDigest: digest, ContractDigest: accepted.ContractDigest, BindingRevision: binding.Revision, CredentialVersion: connection.CredentialVersion}, nil
}

func invocationInputDigest(input any) (string, error) {
	data, err := json.Marshal(input)
	if err != nil {
		return "", connectorError(core.ErrorCodeInvalidInput, false)
	}
	defer clear(data)
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:]), nil
}

func (claim InvocationClaim) valid() bool {
	return validateRouteResolutionRequest(claim.Identity) == nil && claim.Identity.ActingPrincipalID == claim.Identity.OwnerAccountID &&
		validInvocationDigest(claim.Snapshot.InputDigest) && validInvocationDigest(claim.Snapshot.ContractDigest) && claim.Snapshot.BindingRevision > 0 && claim.Snapshot.CredentialVersion > 0
}

func validInvocationDigest(value string) bool {
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == 32 && hex.EncodeToString(decoded) == value
}
