package handler

import (
	"errors"

	llmerrors "github.com/zgiai/zgi/api/internal/modules/llm/errors"
	"github.com/zgiai/zgi/api/internal/modules/llm/gateway"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/pkg/apperror"
)

// applicationCode maps a gateway/domain error to the stable cataloged identity
// that Pre exposes to clients. It is the single classification point shared by
// the legacy protocol classifier and the message projector, so the wire code
// and the projected code can never disagree.
//
// Ordering matters: the four quota causes carried by BillingUserError must be
// resolved before the generic DomainErrBillingFailed that BillingUserError
// wraps; otherwise every cause collapses into a system billing failure.
func applicationCode(err error) (apperror.Code, bool) {
	if err == nil {
		return apperror.Code{}, false
	}
	if appErr, ok := apperror.As(err); ok {
		return appErr.Code(), true
	}

	var userErr *gateway.BillingUserError
	if errors.As(err, &userErr) && userErr != nil {
		switch userErr.Kind {
		case gateway.BillingUserErrorKindOrganizationBalanceInsufficient:
			return llmerrors.AppCodeBillingOrganizationBalanceInsufficient, true
		case gateway.BillingUserErrorKindPrivateChannelBalanceInsufficient:
			return llmerrors.AppCodeBillingPrivateChannelBalanceInsufficient, true
		case gateway.BillingUserErrorKindWorkspaceQuotaInsufficient:
			return llmerrors.AppCodeBillingWorkspaceQuotaInsufficient, true
		case gateway.BillingUserErrorKindAPIKeyQuotaInsufficient:
			return llmerrors.AppCodeQuotaExceeded, true
		case gateway.BillingUserErrorKindModelPricingNotConfigured:
			return llmerrors.AppCodeBillingModelPricingNotConfigured, true
		}
	}

	switch {
	case errors.Is(err, gateway.ErrPricingNotConfigured):
		return llmerrors.AppCodeBillingModelPricingNotConfigured, true
	case errors.Is(err, gateway.ErrInvalidRequest), errors.Is(err, adapter.ErrInvalidRequest),
		errors.Is(err, gateway.ErrMissingModel), errors.Is(err, gateway.ErrMissingMessages),
		errors.Is(err, gateway.ErrEmptyMessages),
		errors.Is(err, llmerrors.DomainErrMissingModel), errors.Is(err, llmerrors.DomainErrMissingMessages),
		errors.Is(err, llmerrors.DomainErrInvalidMessages), errors.Is(err, llmerrors.DomainErrInvalidTemperature),
		errors.Is(err, llmerrors.DomainErrInvalidMaxTokens):
		return llmerrors.AppCodeRequestInvalid, true
	case errors.Is(err, llmerrors.DomainErrModelNotAuthorized):
		return llmerrors.AppCodeModelForbidden, true
	case errors.Is(err, llmerrors.DomainErrModelNotFound), errors.Is(err, gateway.ErrModelNotFound),
		errors.Is(err, gateway.ErrModelNotActive), errors.Is(err, adapter.ErrModelNotFound):
		return llmerrors.AppCodeModelNotFound, true
	case errors.Is(err, llmerrors.DomainErrRouteNotFound):
		return llmerrors.AppCodeRouteNotFound, true
	case errors.Is(err, llmerrors.DomainErrProviderNotFound), errors.Is(err, gateway.ErrProviderNotFound):
		return llmerrors.AppCodeProviderNotFound, true
	case errors.Is(err, llmerrors.DomainErrChannelNotFound):
		return llmerrors.AppCodeChannelNotFound, true
	case errors.Is(err, gateway.ErrNoProviderAvailable), errors.Is(err, llmerrors.DomainErrNoProviderAvailable):
		return llmerrors.AppCodeNoProviderAvailable, true
	case errors.Is(err, gateway.ErrProviderUnavailable), errors.Is(err, llmerrors.DomainErrUpstreamUnavailable):
		return llmerrors.AppCodeProviderUnavailable, true
	case errors.Is(err, llmerrors.DomainErrPrivateChannelUpstreamUnavailable):
		return llmerrors.AppCodePrivateChannelUnavailable, true
	case errors.Is(err, llmerrors.DomainErrRateLimitExceeded), errors.Is(err, adapter.ErrRateLimited):
		return llmerrors.AppCodeRateLimitExceeded, true
	case errors.Is(err, gateway.ErrInsufficientQuota), errors.Is(err, adapter.ErrQuotaExhausted):
		return llmerrors.AppCodeQuotaExceeded, true
	case errors.Is(err, gateway.ErrInsufficientBalance), errors.Is(err, llmerrors.DomainErrInsufficientBalance),
		errors.Is(err, adapter.ErrInsufficientBalance), errors.Is(err, gateway.ErrBalanceNotFound):
		return llmerrors.AppCodeBalanceInsufficient, true
	case errors.Is(err, llmerrors.DomainErrInvalidAPIKey):
		return llmerrors.AppCodeAPIKeyInvalid, true
	case errors.Is(err, llmerrors.DomainErrAPIKeyDisabled):
		return llmerrors.AppCodeAPIKeyInactive, true
	case errors.Is(err, gateway.ErrExchangeRateUnavailable):
		return llmerrors.AppCodeBillingExchangeRateUnavailable, true
	case errors.Is(err, gateway.ErrBillingPreDeductFailed):
		return llmerrors.AppCodeBillingPredeductFailed, true
	case errors.Is(err, gateway.ErrBillingSettleFailed):
		return llmerrors.AppCodeBillingSettleFailed, true
	case errors.Is(err, adapter.ErrAuthFailed):
		return llmerrors.AppCodeProviderAuthFailed, true
	case errors.Is(err, llmerrors.DomainErrUpstreamFailed):
		return llmerrors.AppCodeInvocationFailed, true
	case isGatewayProviderTimeout(err):
		return llmerrors.AppCodeProviderTimeout, true
	}
	return apperror.Code{}, false
}

// applicationErrorFor returns the cataloged application error for a gateway
// error. It preserves an already-wrapped apperror so operation, params, and
// cause survive projection; otherwise it wraps the concrete cause with the
// mapped code and a stable operation name.
func applicationErrorFor(err error) (error, bool) {
	if err == nil {
		return nil, false
	}
	if appErr, ok := apperror.As(err); ok {
		return appErr, true
	}
	code, supported := applicationCode(err)
	if !supported {
		return err, false
	}
	return apperror.Wrap(err, code, apperror.WithOperation("gateway.protocol_error")), true
}
