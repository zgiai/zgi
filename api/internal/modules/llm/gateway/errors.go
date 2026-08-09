package gateway

import (
	"errors"
	"fmt"

	llmerrors "github.com/zgiai/zgi/api/internal/modules/llm/errors"
)

// Gateway specific errors
var (
	// API Key errors
	ErrInvalidAPIKey     = errors.New("invalid API key")
	ErrAPIKeyExpired     = errors.New("API key expired")
	ErrInsufficientQuota = errors.New("insufficient API key quota")
	ErrAPIKeyInactive    = errors.New("API key is inactive")
	ErrAPIKeyNotFound    = errors.New("API key not found")

	// Model errors
	ErrModelNotFound      = errors.New("model not found")
	ErrModelNotAuthorized = llmerrors.DomainErrModelNotAuthorized
	ErrModelNotActive     = errors.New("model is not active")

	// Provider errors
	ErrProviderNotFound    = errors.New("provider not found")
	ErrProviderUnavailable = errors.New("provider unavailable")
	ErrNoProviderAvailable = errors.New("no provider available for this model")
	ErrProviderCallFailed  = errors.New("provider call failed")

	// Balance errors
	ErrInsufficientBalance = errors.New("insufficient account balance")
	ErrBalanceNotFound     = errors.New("account balance not found")

	// Request errors
	ErrInvalidRequest  = errors.New("invalid request")
	ErrMissingModel    = errors.New("model field is required")
	ErrMissingMessages = errors.New("messages field is required")
	ErrEmptyMessages   = errors.New("messages cannot be empty")

	// Billing errors
	ErrBillingFailed            = errors.New("billing operation failed")
	ErrBillingPreDeductFailed   = errors.New("billing pre-deduct failed")
	ErrBillingSettleFailed      = errors.New("billing settle failed")
	ErrBillingLaneMismatch      = errors.New("billing lane mismatch")
	ErrPricingCalculationFailed = errors.New("pricing calculation failed")
)

type reportedProviderFailureError struct {
	cause error
}

func (e *reportedProviderFailureError) Error() string {
	return "all providers failed: " + e.cause.Error()
}

func (e *reportedProviderFailureError) Unwrap() error {
	return e.cause
}

// NewReportedProviderFailureError marks an error whose channel-aware provider
// event has already been emitted before the HTTP layer sees it.
func NewReportedProviderFailureError(err error) error {
	if err == nil {
		return nil
	}
	return &reportedProviderFailureError{cause: err}
}

// IsProviderFailureReported prevents a second owner-ambiguous HTTP event.
func IsProviderFailureReported(err error) bool {
	var reportedErr *reportedProviderFailureError
	return errors.As(err, &reportedErr)
}

// ProviderSelectionConversionError preserves the concrete failures produced
// while turning routed channels into provider selections. Callers need the
// original causes to distinguish a database outage from a bad route without
// parsing an aggregated error string.
type ProviderSelectionConversionError struct {
	causes []error
}

func (*ProviderSelectionConversionError) providerSelectionPreparationError() {}

// NewProviderSelectionConversionError joins conversion causes without losing
// their concrete types.
func NewProviderSelectionConversionError(causes ...error) error {
	filtered := make([]error, 0, len(causes))
	for _, cause := range causes {
		if cause != nil {
			filtered = append(filtered, cause)
		}
	}
	if len(filtered) == 0 {
		return nil
	}
	return &ProviderSelectionConversionError{causes: filtered}
}

func (e *ProviderSelectionConversionError) Error() string {
	return fmt.Sprintf("failed to convert %d channel selection(s)", len(e.causes))
}

func (e *ProviderSelectionConversionError) Unwrap() error {
	return errors.Join(e.causes...)
}

// IsProviderSelectionConversionError reports whether an error originated while
// converting a routed channel into a provider selection.
func IsProviderSelectionConversionError(err error) bool {
	var conversionErr *ProviderSelectionConversionError
	return errors.As(err, &conversionErr)
}

type shadowContextError struct {
	cause error
}

func (e *shadowContextError) Error() string {
	return fmt.Sprintf("failed to resolve provider selection context: %v", e.cause)
}

func (e *shadowContextError) Unwrap() error {
	return e.cause
}

func (*shadowContextError) providerSelectionPreparationError() {}

type providerSelectionPreparationFailure interface {
	providerSelectionPreparationError()
}

// IsProviderSelectionPreparationError reports errors that occur at a known
// routing/cache persistence boundary before any provider request is made.
func IsProviderSelectionPreparationError(err error) bool {
	var preparationErr providerSelectionPreparationFailure
	return errors.As(err, &preparationErr)
}

func wrapPricingCalculationError(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("failed to calculate credits: %w: %w", ErrPricingCalculationFailed, err)
}

// NewNoProviderAvailableError creates a detailed error message for no provider available scenarios
func NewNoProviderAvailableError(modelName, organizationID string) error {
	return fmt.Errorf("%w: model '%s' (tenant: %s). Please check: 1) Model is enabled in your workspace, 2) Provider credentials are configured and active, 3) System channels exist and are not deleted", ErrNoProviderAvailable, modelName, organizationID)
}

// ErrorCode represents HTTP error codes for LLM gateway
type ErrorCode struct {
	Code    int
	Message string
}

// Common error codes
var (
	ErrCodeInvalidAPIKey       = ErrorCode{Code: 40101, Message: "Invalid API key"}
	ErrCodeAPIKeyExpired       = ErrorCode{Code: 40102, Message: "API key expired"}
	ErrCodeAPIKeyInactive      = ErrorCode{Code: 40103, Message: "API key is inactive"}
	ErrCodeInsufficientQuota   = ErrorCode{Code: 114009, Message: "Insufficient API key quota"}
	ErrCodeInsufficientBalance = ErrorCode{Code: 114009, Message: "Insufficient account balance"}
	ErrCodeModelNotFound       = ErrorCode{Code: 40401, Message: "Model not found"}
	ErrCodeModelNotAuthorized  = ErrorCode{Code: 40303, Message: "Model not authorized"}
	ErrCodeProviderUnavailable = ErrorCode{Code: 50301, Message: "Provider unavailable"}
	ErrCodeInvalidRequest      = ErrorCode{Code: 40001, Message: "Invalid request"}
)
