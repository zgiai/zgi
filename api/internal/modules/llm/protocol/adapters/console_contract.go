package adapter

import (
	"encoding/json"
	"strings"
)

// Console (the platform channel) answers failures with the same frozen public
// contract Pre exposes: a top-level error_code carrying the stable catalog
// identity, plus the legacy protocol shape nested under error. Pre has to
// translate those identities back into its own sentinels. Without this table
// every platform-channel outcome collapses into a retryable
// llm.invocation.failed, so the client loses the cause and is told to retry
// things that can never succeed (an upstream that rejected the platform
// credential, an unpriced model, an exhausted balance).
const (
	ConsoleCodeProviderNoneAvailable          = "llm.provider.none_available"
	ConsoleCodeProviderUnavailable            = "llm.provider.unavailable"
	ConsoleCodeProviderTimeout                = "llm.provider.timeout"
	ConsoleCodeProviderAuthFailed             = "llm.provider.auth_failed"
	ConsoleCodeProviderRateLimited            = "llm.provider.rate_limited"
	ConsoleCodeRateLimitExceeded              = "llm.rate_limit.exceeded"
	ConsoleCodeQuotaExceeded                  = "llm.quota.exceeded"
	ConsoleCodeBalanceInsufficient            = "llm.balance.insufficient"
	ConsoleCodeModelNotFound                  = "llm.model.not_found"
	ConsoleCodeRequestInvalid                 = "llm.request.invalid"
	ConsoleCodeBillingPricingNotConfigured    = "billing.model.pricing_not_configured"
	ConsoleCodeBillingExchangeRateUnavailable = "billing.exchange_rate.unavailable"
	ConsoleCodeBillingSettleFailed            = "billing.settle.failed"
)

// consoleSentinelByCatalogCode maps Console's cataloged identity (the
// top-level error_code) onto the adapter sentinel with the same meaning.
var consoleSentinelByCatalogCode = map[string]error{
	ConsoleCodeProviderNoneAvailable:          ErrNoProviderAvailable,
	ConsoleCodeProviderUnavailable:            ErrPlatformChannelUnavailable,
	ConsoleCodeProviderTimeout:                ErrTimeout,
	ConsoleCodeProviderAuthFailed:             ErrAuthFailed,
	ConsoleCodeProviderRateLimited:            ErrRateLimited,
	ConsoleCodeRateLimitExceeded:              ErrRateLimitExceeded,
	ConsoleCodeQuotaExceeded:                  ErrQuotaExhausted,
	ConsoleCodeBalanceInsufficient:            ErrInsufficientBalance,
	ConsoleCodeModelNotFound:                  ErrModelNotFound,
	ConsoleCodeRequestInvalid:                 ErrInvalidRequest,
	ConsoleCodeBillingPricingNotConfigured:    ErrPricingNotConfigured,
	ConsoleCodeBillingExchangeRateUnavailable: ErrExchangeRateUnavailable,
	ConsoleCodeBillingSettleFailed:            ErrBillingSettleFailed,
}

// consoleSentinelByLegacyCode maps the legacy protocol codes Console nests
// under error.code. It exists for responses that predate the catalog envelope,
// so the fallback never re-flattens them.
//
// Keep this list to codes only Console emits. Real providers share much of the
// OpenAI vocabulary (model_not_found, insufficient_balance, and so on), and
// resolving those here would override the status-based classification that
// already handles them correctly.
var consoleSentinelByLegacyCode = map[string]error{
	"no_available_provider":        ErrNoProviderAvailable,
	"platform_channel_unavailable": ErrPlatformChannelUnavailable,
	"upstream_timeout":             ErrTimeout,
	"upstream_auth_failed":         ErrAuthFailed,
	"pricing_not_configured":       ErrPricingNotConfigured,
	"exchange_rate_unavailable":    ErrExchangeRateUnavailable,
	"unsupported_protocol":         ErrCapabilityUnsupported,
	"unsupported_capability":       ErrCapabilityUnsupported,
}

// SentinelForConsoleErrorCode resolves a Console cataloged error_code, falling
// back to the legacy nested error.code. The catalog identity always wins: a
// response may carry both, and the nested value is only a compatibility echo.
func SentinelForConsoleErrorCode(code string) (error, bool) {
	trimmed := strings.TrimSpace(code)
	if trimmed == "" {
		return nil, false
	}
	if sentinel, ok := consoleSentinelByCatalogCode[trimmed]; ok {
		return sentinel, true
	}
	sentinel, ok := consoleSentinelByLegacyCode[trimmed]
	return sentinel, ok
}

// ConsoleSentinelFromBody resolves the Console contract identity carried by an
// error response body. The retry loop uses it so a 5xx that has exhausted its
// retries is still reported as the platform channel's own outcome instead of a
// bare upstream failure: retrying must not erase what went wrong.
func ConsoleSentinelFromBody(body []byte) (error, bool) {
	if len(body) == 0 {
		return nil, false
	}
	var payload struct {
		ErrorCode string `json:"error_code"`
		Code      string `json:"code"`
		Error     struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, false
	}
	for _, candidate := range []string{payload.ErrorCode, payload.Code, payload.Error.Code} {
		if sentinel, ok := SentinelForConsoleErrorCode(candidate); ok {
			return sentinel, true
		}
	}
	return nil, false
}
