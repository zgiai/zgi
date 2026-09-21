// Package ratingcore is the dependency-free billing calculator shared by
// Console (source of truth) and Pre (byte-for-byte mirror).
//
// Purity contract: this package must never import database, transport, logging,
// configuration, wallet, state-machine, or application packages. Its only
// import is exact decimal arithmetic (github.com/shopspring/decimal). It has no
// process, network, clock, or filesystem side effects.
//
// The source lives at zgi-console/api/internal/modules/billing/ratingcore and
// is copied wholesale to zgi-pre/api/internal/modules/llm/gateway/ratingcore.
// Never edit the Pre copy by hand; run the sync target instead.
package ratingcore

import "fmt"

// Stable, cataloged error codes. These values are frozen contract and are
// registered in error-catalog-v1.json / 错误码表-v1.md. Do not invent codes
// here without registering them in the contract first.
const (
	// CodePricingNotConfigured means no price matched the requested
	// operation/meter/unit combination. It must never fall back to another
	// combination (see the image-token vs per-image rule).
	CodePricingNotConfigured = "billing.model.pricing_not_configured"
	// CodeExchangeRateUnavailable means a required exchange rate is missing or
	// invalid. Missing rates are fail-closed: never substitute 1 or a current
	// rate.
	CodeExchangeRateUnavailable = "billing.exchange_rate.unavailable"
	// CodeRequestInvalid means the usage or request shape cannot be rated.
	CodeRequestInvalid = "billing.request.invalid"
)

// Error is the stable error type returned by Quote and Resolve. It mirrors the
// catalog fields Pre and Console already project at their boundaries.
type Error struct {
	Code       string
	HTTPStatus int
	Retryable  bool
	Message    string
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	if e.Message == "" {
		return e.Code
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// Is reports whether target carries the same stable code, so callers can use
// errors.Is(err, &ratingcore.Error{Code: ...}) without importing sentinels.
func (e *Error) Is(target error) bool {
	other, ok := target.(*Error)
	return ok && other != nil && e != nil && other.Code == e.Code
}

func pricingError(message string) error {
	return &Error{Code: CodePricingNotConfigured, HTTPStatus: 402, Retryable: false, Message: message}
}

func exchangeRateError(message string) error {
	return &Error{Code: CodeExchangeRateUnavailable, HTTPStatus: 503, Retryable: false, Message: message}
}

func invalidRequestError(message string) error {
	return &Error{Code: CodeRequestInvalid, HTTPStatus: 400, Retryable: false, Message: message}
}
