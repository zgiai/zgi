package handler

import (
	"testing"

	llmerrors "github.com/zgiai/zgi/api/internal/modules/llm/errors"
	"github.com/zgiai/zgi/api/internal/modules/llm/gateway"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

// TestRawClassifierAgreesWithTheLedger feeds every classification input to the
// raw outward classifier and to the ledger classifier. The projected variant is
// covered by TestPublishedErrorTripleMatchesCatalogAndLedger, but handlers that
// answer before a projector is attached (or that own no projector at all) reach
// classifyProtocolError directly, so the two must agree on their own.
//
// This is the regression gate for the divergence that made the wire answer a
// retryable 502 llm.invocation.failed for outcomes the ledger recorded as
// llm.balance.insufficient or llm.provider.auth_failed.
func TestRawClassifierAgreesWithTheLedger(t *testing.T) {
	inputs := map[string]error{
		"balance not found":        gateway.ErrBalanceNotFound,
		"billing failed":           gateway.ErrBillingFailed,
		"billing lane mismatch":    gateway.ErrBillingLaneMismatch,
		"pricing calculation":      gateway.ErrPricingCalculationFailed,
		"exchange rate":            gateway.ErrExchangeRateUnavailable,
		"no provider":              gateway.ErrNoProviderAvailable,
		"provider unavailable":     gateway.ErrProviderUnavailable,
		"pricing not configured":   gateway.ErrPricingNotConfigured,
		"model not found":          gateway.ErrModelNotFound,
		"model not authorized":     gateway.ErrModelNotAuthorized,
		"model not active":         gateway.ErrModelNotActive,
		"invalid request":          gateway.ErrInvalidRequest,
		"missing model":            gateway.ErrMissingModel,
		"missing messages":         gateway.ErrMissingMessages,
		"empty messages":           gateway.ErrEmptyMessages,
		"provider not found":       gateway.ErrProviderNotFound,
		"invalid api key":          gateway.ErrInvalidAPIKey,
		"api key not found":        gateway.ErrAPIKeyNotFound,
		"api key expired":          gateway.ErrAPIKeyExpired,
		"api key inactive":         gateway.ErrAPIKeyInactive,
		"insufficient quota":       gateway.ErrInsufficientQuota,
		"insufficient balance":     gateway.ErrInsufficientBalance,
		"billing prededuct":        gateway.ErrBillingPreDeductFailed,
		"billing settle":           gateway.ErrBillingSettleFailed,
		"adapter upstream":         adapter.ErrUpstreamError,
		"adapter auth":             adapter.ErrAuthFailed,
		"adapter rate limited":     adapter.ErrRateLimited,
		"adapter platform down":    adapter.ErrPlatformChannelUnavailable,
		"adapter proxy":            adapter.ErrProxyError,
		"adapter billing unavail":  adapter.ErrBillingUnavailable,
		"adapter quota":            adapter.ErrQuotaExhausted,
		"adapter insufficient bal": adapter.ErrInsufficientBalance,
		"adapter model not found":  adapter.ErrModelNotFound,
		"adapter timeout":          adapter.ErrTimeout,
		"adapter invalid request":  adapter.ErrInvalidRequest,
		"adapter content policy":   adapter.ErrContentPolicyViolation,
		"adapter capability":       adapter.ErrCapabilityUnsupported,
		"adapter no provider":      adapter.ErrNoProviderAvailable,
		"adapter own rate limit":   adapter.ErrRateLimitExceeded,
		"adapter pricing":          adapter.ErrPricingNotConfigured,
		"adapter exchange rate":    adapter.ErrExchangeRateUnavailable,
		"adapter settle failed":    adapter.ErrBillingSettleFailed,
		"domain route not found":   llmerrors.DomainErrRouteNotFound,
		"domain insufficient bal":  llmerrors.DomainErrInsufficientBalance,
		"domain upstream failed":   llmerrors.DomainErrUpstreamFailed,
		"domain rate limit":        llmerrors.DomainErrRateLimitExceeded,
		"domain provider not fnd":  llmerrors.DomainErrProviderNotFound,
		"domain channel not fnd":   llmerrors.DomainErrChannelNotFound,
		"domain invalid api key":   llmerrors.DomainErrInvalidAPIKey,
		"domain api key disabled":  llmerrors.DomainErrAPIKeyDisabled,
		"domain model not found":   llmerrors.DomainErrModelNotFound,
		"domain model not authz":   llmerrors.DomainErrModelNotAuthorized,
		"domain private channel":   llmerrors.DomainErrPrivateChannelUpstreamUnavailable,
		"domain upstream timeout":  llmerrors.DomainErrUpstreamTimeout,
	}
	for name, err := range inputs {
		t.Run(name, func(t *testing.T) {
			wire := classifyProtocolError(err)
			ledger, ok := gateway.ApplicationCodeForError(err)
			if !ok {
				t.Fatalf("ledger did not classify %v", err)
			}
			if wire.appCode != ledger.String() {
				t.Fatalf("wire code %q, ledger code %q", wire.appCode, ledger.String())
			}
		})
	}
}
