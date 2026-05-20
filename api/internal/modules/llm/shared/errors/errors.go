package errors

import (
	"errors"
	"fmt"
)

// Common LLM module errors
var (
	// Provider errors
	ErrProviderNotFound      = errors.New("provider not found")
	ErrProviderAlreadyExists = errors.New("provider already exists")
	ErrProviderNotActive     = errors.New("provider is not active")
	ErrProviderNotEditable   = errors.New("system provider cannot be edited")

	// Model errors
	ErrModelNotFound      = errors.New("model not found")
	ErrModelAlreadyExists = errors.New("model already exists")
	ErrModelNotActive     = errors.New("model is not active")
	ErrModelNotEditable   = errors.New("system model cannot be edited")

	// Credential errors
	ErrCredentialNotFound = errors.New("credential not found")
	ErrCredentialInvalid  = errors.New("credential is invalid")
	ErrCredentialExpired  = errors.New("credential has expired")

	// Route errors
	ErrRouteNotFound    = errors.New("route not found")
	ErrNoAvailableRoute = errors.New("no available route for model")

	// Channel errors
	ErrChannelNotFound = errors.New("channel not found")
	ErrChannelDisabled = errors.New("channel is disabled")

	// API Key errors
	ErrAPIKeyNotFound      = errors.New("API key not found")
	ErrAPIKeyInvalid       = errors.New("API key is invalid")
	ErrAPIKeyExpired       = errors.New("API key has expired")
	ErrAPIKeyDisabled      = errors.New("API key is disabled")
	ErrAPIKeyQuotaExceeded = errors.New("API key quota exceeded")

	// Wallet errors
	ErrInsufficientBalance = errors.New("insufficient balance")
	ErrTransactionFailed   = errors.New("transaction failed")

	// Permission errors
	ErrPermissionDenied = errors.New("permission denied")
	ErrTenantMismatch   = errors.New("tenant mismatch")

	// Validation errors
	ErrInvalidRequest = errors.New("invalid request")
	ErrMissingField   = errors.New("missing required field")
)

// WrapError wraps an error with additional context
func WrapError(err error, message string) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", message, err)
}

// IsNotFound checks if the error is a "not found" type error
func IsNotFound(err error) bool {
	return errors.Is(err, ErrProviderNotFound) ||
		errors.Is(err, ErrModelNotFound) ||
		errors.Is(err, ErrCredentialNotFound) ||
		errors.Is(err, ErrRouteNotFound) ||
		errors.Is(err, ErrChannelNotFound) ||
		errors.Is(err, ErrAPIKeyNotFound)
}

// IsPermissionError checks if the error is a permission-related error
func IsPermissionError(err error) bool {
	return errors.Is(err, ErrPermissionDenied) ||
		errors.Is(err, ErrTenantMismatch) ||
		errors.Is(err, ErrProviderNotEditable) ||
		errors.Is(err, ErrModelNotEditable)
}

// IsQuotaError checks if the error is a quota-related error
func IsQuotaError(err error) bool {
	return errors.Is(err, ErrAPIKeyQuotaExceeded) ||
		errors.Is(err, ErrInsufficientBalance)
}
