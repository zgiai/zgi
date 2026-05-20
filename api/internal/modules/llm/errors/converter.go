package llmerrors

import (
	"errors"
	"fmt"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/zgiai/ginext/pkg/response"
)

// Common domain errors that can be returned from service layer
var (
	// Domain errors for model operations
	DomainErrModelNotFound      = errors.New("model not found")
	DomainErrProviderNotFound   = errors.New("provider not found")
	DomainErrChannelNotFound    = errors.New("channel not found")
	DomainErrRouteNotFound      = errors.New("route not found")
	DomainErrCredentialNotFound = errors.New("credential not found")

	// Domain errors for provider operations
	DomainErrNoProviderAvailable = errors.New("no provider available for model")
	DomainErrUpstreamFailed      = errors.New("upstream provider failed")
	DomainErrUpstreamTimeout     = errors.New("upstream provider timeout")
	DomainErrUpstreamUnavailable = errors.New("upstream provider unavailable")

	// Domain errors for authentication/authorization
	DomainErrInvalidAPIKey       = errors.New("invalid API key")
	DomainErrAPIKeyDisabled      = errors.New("API key disabled")
	DomainErrInsufficientBalance = errors.New("insufficient balance")
	DomainErrModelNotAuthorized  = errors.New("model not authorized")
	DomainErrRateLimitExceeded   = errors.New("rate limit exceeded")

	// Domain errors for request validation
	DomainErrMissingModel       = errors.New("missing model parameter")
	DomainErrMissingMessages    = errors.New("missing messages parameter")
	DomainErrInvalidMessages    = errors.New("invalid messages format")
	DomainErrInvalidTemperature = errors.New("invalid temperature value")
	DomainErrInvalidMaxTokens   = errors.New("invalid max tokens value")

	// Domain errors for internal operations
	DomainErrInternalError = errors.New("internal server error")
	DomainErrDatabaseError = errors.New("database operation failed")
	DomainErrBillingFailed = errors.New("billing operation failed")
)

// ConvertError converts domain errors to response.ErrorCode
// This is the central place for error code mapping
func ConvertError(err error) response.ErrorCode {
	if err == nil {
		return response.ErrorCode{Code: 0, Message: "success", UserVisible: true}
	}

	// Try to match domain errors
	switch {
	// Model/Resource errors -> 404
	case errors.Is(err, DomainErrModelNotFound):
		errCode := ErrModelNotFound
		if detail := detailedDomainErrorMessage(err, DomainErrModelNotFound); detail != "" {
			errCode.Message = detail
		}
		return errCode
	case errors.Is(err, DomainErrProviderNotFound):
		return ErrProviderNotFound
	case errors.Is(err, DomainErrChannelNotFound):
		return ErrChannelNotFound
	case errors.Is(err, DomainErrRouteNotFound):
		return ErrRouteNotFound

	// Provider errors -> 503/502/504
	case errors.Is(err, DomainErrNoProviderAvailable):
		return ErrNoProviderAvailable
	case errors.Is(err, DomainErrUpstreamFailed):
		return ErrUpstreamError
	case errors.Is(err, DomainErrUpstreamTimeout):
		return ErrUpstreamTimeout
	case errors.Is(err, DomainErrUpstreamUnavailable):
		return ErrUpstreamUnavailable

	// Authentication errors -> 401
	case errors.Is(err, DomainErrInvalidAPIKey):
		return ErrInvalidAPIKey
	case errors.Is(err, DomainErrAPIKeyDisabled):
		return ErrAPIKeyDisabled

	// Authorization errors -> 403
	case errors.Is(err, DomainErrInsufficientBalance):
		return ErrInsufficientBalance
	case errors.Is(err, DomainErrModelNotAuthorized):
		return ErrModelNotAuthorized
	case errors.Is(err, DomainErrRateLimitExceeded):
		return ErrRateLimitExceeded

	// Request validation errors -> 400
	case errors.Is(err, DomainErrMissingModel):
		return ErrMissingModel
	case errors.Is(err, DomainErrMissingMessages):
		return ErrMissingMessages
	case errors.Is(err, DomainErrInvalidMessages):
		return ErrInvalidMessages
	case errors.Is(err, DomainErrInvalidTemperature):
		return ErrInvalidTemperature
	case errors.Is(err, DomainErrInvalidMaxTokens):
		return ErrInvalidMaxTokens

	// Internal errors -> 500
	case errors.Is(err, DomainErrDatabaseError):
		return ErrDatabaseError
	case errors.Is(err, DomainErrBillingFailed):
		return ErrBillingFailed

	// Default: return internal error with original message
	default:
		return response.ErrorCode{
			Code:        ErrCodeInternalError,
			Message:     err.Error(),
			UserVisible: false,
		}
	}
}

// HandleServiceError is a helper to handle service layer errors in handlers
// Usage: llmerrors.HandleServiceError(c, err)
func HandleServiceError(c *gin.Context, err error) {
	if err == nil {
		return
	}
	response.Fail(c, ConvertError(err))
}

// HandleServiceErrorWithContext provides error with additional context
// Usage: llmerrors.HandleServiceErrorWithContext(c, err, "failed to create model")
func HandleServiceErrorWithContext(c *gin.Context, err error, context string) {
	if err == nil {
		return
	}

	errCode := ConvertError(err)
	// Add context to error message
	errCode.Message = fmt.Sprintf("%s: %s", context, errCode.Message)
	response.Fail(c, errCode)
}

// WrapError wraps an error with domain error for proper conversion
// Usage: return llmerrors.WrapError(err, llmerrors.DomainErrModelNotFound)
func WrapError(err error, domainErr error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%w: %v", domainErr, err)
}

// NewModelNotFoundErrorWithName creates a model not found error with model name
func NewModelNotFoundErrorWithName(modelName string) error {
	normalizedModelName := strings.TrimSpace(modelName)
	return fmt.Errorf(
		"%w: current workspace has no enabled route for model %q; if you configured route models manually, use the full model name exactly as shown in the model list",
		DomainErrModelNotFound,
		normalizedModelName,
	)
}

func detailedDomainErrorMessage(err error, domainErr error) string {
	if err == nil || domainErr == nil {
		return ""
	}

	prefix := domainErr.Error() + ": "
	message := err.Error()
	if !strings.HasPrefix(message, prefix) {
		return ""
	}

	return strings.TrimSpace(strings.TrimPrefix(message, prefix))
}

// NewRouteNotFoundErrorWithDetails creates a route not found error with details
func NewRouteNotFoundErrorWithDetails(modelName, organizationID string, totalRoutes int) error {
	return fmt.Errorf("%w: model '%s' for tenant %s (checked %d routes)",
		DomainErrRouteNotFound, modelName, organizationID, totalRoutes)
}

// NewUpstreamErrorWithProvider creates an upstream error with provider name
func NewUpstreamErrorWithProvider(providerName string, originalErr error) error {
	return fmt.Errorf("%w from %s: %v", DomainErrUpstreamFailed, providerName, originalErr)
}
