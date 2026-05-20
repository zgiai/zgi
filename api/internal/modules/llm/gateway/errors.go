package gateway

import (
	"errors"
	"fmt"
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
	ErrModelNotAuthorized = errors.New("model not authorized for this account")
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
	ErrBillingFailed          = errors.New("billing operation failed")
	ErrBillingPreDeductFailed = errors.New("billing pre-deduct failed")
	ErrBillingSettleFailed    = errors.New("billing settle failed")
	ErrBillingLaneMismatch    = errors.New("billing lane mismatch")
)

// NewNoProviderAvailableError creates a detailed error message for no provider available scenarios
func NewNoProviderAvailableError(modelName, organizationID string) error {
	return fmt.Errorf("no provider available for model '%s' (tenant: %s). Please check: 1) Model is enabled in your workspace, 2) Provider credentials are configured and active, 3) System channels exist and are not deleted", modelName, organizationID)
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
