package gateway

import (
	"errors"
	"net/http"

	llmerrors "github.com/zgiai/zgi/api/internal/modules/llm/errors"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/pkg/apperror"
)

// ApplicationCodeForError maps a gateway/domain error to the stable cataloged
// identity that Pre exposes to clients. It is the single classification point
// shared by the persisted usage event and the protocol response, so the code
// written to the ledger and the code returned to the client can never disagree.
//
// Ordering matters: the four quota causes carried by BillingUserError must be
// resolved before the generic billing failure it wraps; otherwise every cause
// collapses into a system billing failure.
func ApplicationCodeForError(err error) (apperror.Code, bool) {
	if err == nil {
		return apperror.Code{}, false
	}
	if appErr, ok := apperror.As(err); ok {
		return appErr.Code(), true
	}

	var userErr *BillingUserError
	if errors.As(err, &userErr) && userErr != nil {
		switch userErr.Kind {
		case BillingUserErrorKindOrganizationBalanceInsufficient:
			return llmerrors.AppCodeBillingOrganizationBalanceInsufficient, true
		case BillingUserErrorKindPrivateChannelBalanceInsufficient:
			return llmerrors.AppCodeBillingPrivateChannelBalanceInsufficient, true
		case BillingUserErrorKindWorkspaceQuotaInsufficient:
			return llmerrors.AppCodeBillingWorkspaceQuotaInsufficient, true
		case BillingUserErrorKindAPIKeyQuotaInsufficient:
			return llmerrors.AppCodeQuotaExceeded, true
		case BillingUserErrorKindModelPricingNotConfigured:
			return llmerrors.AppCodeBillingModelPricingNotConfigured, true
		}
	}

	switch {
	case errors.Is(err, ErrPricingNotConfigured):
		return llmerrors.AppCodeBillingModelPricingNotConfigured, true
	case errors.Is(err, ErrInvalidRequest), errors.Is(err, adapter.ErrInvalidRequest),
		errors.Is(err, ErrMissingModel), errors.Is(err, ErrMissingMessages),
		errors.Is(err, ErrEmptyMessages),
		errors.Is(err, adapter.ErrContentPolicyViolation),
		errors.Is(err, llmerrors.DomainErrMissingModel), errors.Is(err, llmerrors.DomainErrMissingMessages),
		errors.Is(err, llmerrors.DomainErrInvalidMessages), errors.Is(err, llmerrors.DomainErrInvalidTemperature),
		errors.Is(err, llmerrors.DomainErrInvalidMaxTokens):
		return llmerrors.AppCodeRequestInvalid, true
	case errors.Is(err, llmerrors.DomainErrModelNotAuthorized):
		return llmerrors.AppCodeModelForbidden, true
	case errors.Is(err, llmerrors.DomainErrModelNotFound), errors.Is(err, ErrModelNotFound),
		errors.Is(err, ErrModelNotActive), errors.Is(err, adapter.ErrModelNotFound):
		return llmerrors.AppCodeModelNotFound, true
	case adapter.IsCapabilityUnsupported(err):
		// The same predicate, at the same precedence, as the protocol
		// boundary: an upstream that says the model does not support the
		// request is a request problem on both sides, never an invocation
		// failure in the ledger and a 400 on the wire.
		return llmerrors.AppCodeRequestInvalid, true
	case errors.Is(err, llmerrors.DomainErrRouteNotFound):
		// The model exists but no enabled route can serve it: an availability
		// outcome, not "model not found".
		return llmerrors.AppCodeNoProviderAvailable, true
	case errors.Is(err, llmerrors.DomainErrProviderNotFound), errors.Is(err, ErrProviderNotFound):
		return llmerrors.AppCodeProviderNotFound, true
	case errors.Is(err, llmerrors.DomainErrChannelNotFound):
		return llmerrors.AppCodeChannelNotFound, true
	case errors.Is(err, ErrNoProviderAvailable), errors.Is(err, llmerrors.DomainErrNoProviderAvailable):
		return llmerrors.AppCodeNoProviderAvailable, true
	case errors.Is(err, ErrProviderUnavailable), errors.Is(err, llmerrors.DomainErrUpstreamUnavailable),
		errors.Is(err, adapter.ErrPlatformChannelUnavailable), errors.Is(err, adapter.ErrProxyError),
		errors.Is(err, adapter.ErrBillingUnavailable):
		return llmerrors.AppCodeProviderUnavailable, true
	case errors.Is(err, llmerrors.DomainErrPrivateChannelUpstreamUnavailable):
		return llmerrors.AppCodePrivateChannelUnavailable, true
	case errors.Is(err, llmerrors.DomainErrRateLimitExceeded):
		return llmerrors.AppCodeRateLimitExceeded, true
	case errors.Is(err, adapter.ErrRateLimited):
		// The upstream provider throttled the call, as opposed to this
		// gateway's own limit above.
		return llmerrors.AppCodeProviderRateLimited, true
	case errors.Is(err, ErrInsufficientQuota), errors.Is(err, adapter.ErrQuotaExhausted):
		return llmerrors.AppCodeQuotaExceeded, true
	case errors.Is(err, ErrInsufficientBalance), errors.Is(err, llmerrors.DomainErrInsufficientBalance),
		errors.Is(err, adapter.ErrInsufficientBalance), errors.Is(err, ErrBalanceNotFound):
		return llmerrors.AppCodeBalanceInsufficient, true
	case errors.Is(err, llmerrors.DomainErrInvalidAPIKey):
		return llmerrors.AppCodeAPIKeyInvalid, true
	case errors.Is(err, llmerrors.DomainErrAPIKeyDisabled):
		return llmerrors.AppCodeAPIKeyInactive, true
	case errors.Is(err, ErrExchangeRateUnavailable):
		return llmerrors.AppCodeBillingExchangeRateUnavailable, true
	case errors.Is(err, ErrBillingPreDeductFailed):
		return llmerrors.AppCodeBillingPredeductFailed, true
	case errors.Is(err, ErrBillingSettleFailed):
		return llmerrors.AppCodeBillingSettleFailed, true
	case errors.Is(err, adapter.ErrAuthFailed):
		return llmerrors.AppCodeProviderAuthFailed, true
	case isProviderTimeoutError(err):
		return llmerrors.AppCodeProviderTimeout, true
	case errors.Is(err, llmerrors.DomainErrUpstreamFailed), errors.Is(err, adapter.ErrUpstreamError):
		return llmerrors.AppCodeInvocationFailed, true
	}
	var statusErr *adapter.HTTPStatusError
	if errors.As(err, &statusErr) && statusErr.StatusCode >= 500 {
		return llmerrors.AppCodeInvocationFailed, true
	}
	// Every remaining failure is still projected through the catalog: callers
	// must never fall back to an English diagnostic string on a public response.
	return llmerrors.AppCodeInvocationFailed, true
}

// isProviderTimeoutError mirrors the handler's timeout predicate so the shared
// mapper keeps classifying timeouts as provider timeouts.
func isProviderTimeoutError(err error) bool {
	if errors.Is(err, adapter.ErrTimeout) || errors.Is(err, llmerrors.DomainErrUpstreamTimeout) {
		return true
	}
	var adapterErr *adapter.AdapterError
	if errors.As(err, &adapterErr) && adapterErr.StatusCode == http.StatusGatewayTimeout {
		return true
	}
	var statusErr *adapter.HTTPStatusError
	return errors.As(err, &statusErr) && statusErr.StatusCode == http.StatusGatewayTimeout
}
