package llmerrors

import "github.com/zgiai/zgi/api/pkg/response"

// LLM Module Error Code Range: 40000-40999
// Module Code: 40 (LLM Gateway)

const (
	// ============================================
	// Authentication & Authorization (401xx)
	// ============================================

	// ErrCodeInvalidAPIKey Invalid or expired API key
	// HTTP 401
	ErrCodeInvalidAPIKey = 40101

	// ErrCodeAPIKeyDisabled API key has been disabled
	// HTTP 401
	ErrCodeAPIKeyDisabled = 40102

	// ErrCodeAPIKeyExpired API key has expired
	// HTTP 401
	ErrCodeAPIKeyExpired = 40103

	// ============================================
	// Authorization & Permissions (403xx)
	// ============================================

	// ErrCodeInsufficientBalance Insufficient balance to complete request
	// HTTP 403
	ErrCodeInsufficientBalance = 40301

	// ErrCodeModelNotAuthorized Model not authorized for this API key
	// HTTP 403
	ErrCodeModelNotAuthorized = 40302

	// ErrCodeRateLimitExceeded Rate limit exceeded
	// HTTP 429
	ErrCodeRateLimitExceeded = 40901

	// ============================================
	// Resource Not Found (404xx)
	// ============================================

	// ErrCodeModelNotFound Model not found in system
	// HTTP 404
	ErrCodeModelNotFound = 40401

	// ErrCodeProviderNotFound Provider not found
	// HTTP 404
	ErrCodeProviderNotFound = 40402

	// ErrCodeChannelNotFound Channel not found
	// HTTP 404
	ErrCodeChannelNotFound = 40403

	// ErrCodeRouteNotFound No route available for requested model
	// HTTP 404
	ErrCodeRouteNotFound = 40404

	// ============================================
	// Request Validation (400xx)
	// ============================================

	// ErrCodeMissingModel Model parameter is required
	// HTTP 400
	ErrCodeMissingModel = 40001

	// ErrCodeMissingMessages Messages parameter is required
	// HTTP 400
	ErrCodeMissingMessages = 40002

	// ErrCodeInvalidMessages Messages format is invalid
	// HTTP 400
	ErrCodeInvalidMessages = 40003

	// ErrCodeInvalidTemperature Temperature must be between 0 and 2
	// HTTP 400
	ErrCodeInvalidTemperature = 40004

	// ErrCodeInvalidMaxTokens MaxTokens must be positive
	// HTTP 400
	ErrCodeInvalidMaxTokens = 40005

	// ErrCodeInvalidStreamOptions Stream options format is invalid
	// HTTP 400
	ErrCodeInvalidStreamOptions = 40006

	// ============================================
	// Upstream Provider Errors (405xx)
	// ============================================

	// ErrCodeUpstreamAuthFailed Upstream provider authentication failed
	// HTTP 502
	ErrCodeUpstreamAuthFailed = 40501

	// ErrCodeUpstreamRateLimit Upstream provider rate limit
	// HTTP 429
	ErrCodeUpstreamRateLimit = 40502

	// ErrCodeUpstreamTimeout Upstream provider timeout
	// HTTP 504
	ErrCodeUpstreamTimeout = 40503

	// ErrCodeUpstreamUnavailable Upstream provider unavailable
	// HTTP 503
	ErrCodeUpstreamUnavailable = 40504

	// ErrCodeUpstreamError Generic upstream provider error
	// HTTP 502
	ErrCodeUpstreamError = 40505

	// ErrCodeNoProviderAvailable No provider available for this model
	// HTTP 503
	ErrCodeNoProviderAvailable               = 40506
	ErrCodePrivateChannelUpstreamUnavailable = 40507

	// ============================================
	// System Errors (406xx)
	// ============================================

	// ErrCodeInternalError Internal server error
	// HTTP 500
	ErrCodeInternalError = 40601

	// ErrCodeDatabaseError Database operation failed
	// HTTP 500
	ErrCodeDatabaseError = 40602

	// ErrCodeBillingFailed Billing operation failed
	// HTTP 500
	ErrCodeBillingFailed = 40603
)

// ============================================
// Error Definitions
// ============================================

var (
	// Authentication Errors
	ErrInvalidAPIKey = response.ErrorCode{
		Code:        ErrCodeInvalidAPIKey,
		Message:     "Invalid API key",
		UserVisible: true,
	}

	ErrAPIKeyDisabled = response.ErrorCode{
		Code:        ErrCodeAPIKeyDisabled,
		Message:     "API key has been disabled",
		UserVisible: true,
	}

	ErrAPIKeyExpired = response.ErrorCode{
		Code:        ErrCodeAPIKeyExpired,
		Message:     "API key has expired",
		UserVisible: true,
	}

	// Authorization Errors
	ErrInsufficientBalance = response.ErrorCode{
		Code:        ErrCodeInsufficientBalance,
		Message:     "Insufficient balance to complete request",
		UserVisible: true,
	}

	ErrModelNotAuthorized = response.ErrorCode{
		Code:        ErrCodeModelNotAuthorized,
		Message:     "Model not authorized for this API key",
		UserVisible: true,
	}

	ErrRateLimitExceeded = response.ErrorCode{
		Code:        ErrCodeRateLimitExceeded,
		Message:     "Rate limit exceeded. Please try again later",
		UserVisible: true,
	}

	// Resource Not Found
	ErrModelNotFound = response.ErrorCode{
		Code:        ErrCodeModelNotFound,
		Message:     "Model not found",
		UserVisible: true,
	}

	ErrProviderNotFound = response.ErrorCode{
		Code:        ErrCodeProviderNotFound,
		Message:     "Provider not found",
		UserVisible: true,
	}

	ErrChannelNotFound = response.ErrorCode{
		Code:        ErrCodeChannelNotFound,
		Message:     "Channel not found",
		UserVisible: true,
	}

	ErrRouteNotFound = response.ErrorCode{
		Code:        ErrCodeRouteNotFound,
		Message:     "No route available for requested model",
		UserVisible: true,
	}

	// Request Validation
	ErrMissingModel = response.ErrorCode{
		Code:        ErrCodeMissingModel,
		Message:     "Model parameter is required",
		UserVisible: true,
	}

	ErrMissingMessages = response.ErrorCode{
		Code:        ErrCodeMissingMessages,
		Message:     "Messages parameter is required",
		UserVisible: true,
	}

	ErrInvalidMessages = response.ErrorCode{
		Code:        ErrCodeInvalidMessages,
		Message:     "Messages format is invalid",
		UserVisible: true,
	}

	ErrInvalidTemperature = response.ErrorCode{
		Code:        ErrCodeInvalidTemperature,
		Message:     "Temperature must be between 0 and 2",
		UserVisible: true,
	}

	ErrInvalidMaxTokens = response.ErrorCode{
		Code:        ErrCodeInvalidMaxTokens,
		Message:     "MaxTokens must be positive",
		UserVisible: true,
	}

	ErrInvalidStreamOptions = response.ErrorCode{
		Code:        ErrCodeInvalidStreamOptions,
		Message:     "Stream options format is invalid",
		UserVisible: true,
	}

	// Upstream Errors
	ErrUpstreamAuthFailed = response.ErrorCode{
		Code:        ErrCodeUpstreamAuthFailed,
		Message:     "Upstream provider authentication failed",
		UserVisible: true,
	}

	ErrUpstreamRateLimit = response.ErrorCode{
		Code:        ErrCodeUpstreamRateLimit,
		Message:     "Upstream provider rate limit exceeded",
		UserVisible: true,
	}

	ErrUpstreamTimeout = response.ErrorCode{
		Code:        ErrCodeUpstreamTimeout,
		Message:     "Upstream provider request timeout",
		UserVisible: true,
	}

	ErrUpstreamUnavailable = response.ErrorCode{
		Code:        ErrCodeUpstreamUnavailable,
		Message:     "Upstream provider temporarily unavailable",
		UserVisible: true,
	}

	ErrUpstreamError = response.ErrorCode{
		Code:        ErrCodeUpstreamError,
		Message:     "Upstream provider error",
		UserVisible: false,
	}

	ErrNoProviderAvailable = response.ErrorCode{
		Code:        ErrCodeNoProviderAvailable,
		Message:     "No provider available for this model",
		UserVisible: true,
	}

	ErrPrivateChannelUpstreamUnavailable = response.ErrorCode{
		Code:        ErrCodePrivateChannelUpstreamUnavailable,
		Message:     "private_channel_upstream_unavailable",
		UserVisible: true,
	}

	// System Errors
	ErrInternalError = response.ErrorCode{
		Code:        ErrCodeInternalError,
		Message:     "Internal server error",
		UserVisible: false,
	}

	ErrDatabaseError = response.ErrorCode{
		Code:        ErrCodeDatabaseError,
		Message:     "Database operation failed",
		UserVisible: false,
	}

	ErrBillingFailed = response.ErrorCode{
		Code:        ErrCodeBillingFailed,
		Message:     "Billing operation failed",
		UserVisible: false,
	}
)

// ============================================
// Helper Functions
// ============================================

// NewModelNotFoundError creates a model not found error with model name
func NewModelNotFoundError(modelName string) response.ErrorCode {
	return response.ErrorCode{
		Code:        ErrCodeModelNotFound,
		Message:     "Model '" + modelName + "' not found",
		UserVisible: true,
	}
}

// NewRouteNotFoundError creates a route not found error with details
func NewRouteNotFoundError(modelName, organizationID string, totalRoutes int) response.ErrorCode {
	return response.ErrorCode{
		Code: ErrCodeRouteNotFound,
		Message: "No routes support model '" + modelName + "' for organization " + organizationID +
			". Found " + string(rune(totalRoutes)) + " total routes but none have this model in their models list",
		UserVisible: true,
	}
}

// NewUpstreamError creates an upstream error with details
func NewUpstreamError(providerName string, details string) response.ErrorCode {
	return response.ErrorCode{
		Code:        ErrCodeUpstreamError,
		Message:     "Upstream provider " + providerName + " error: " + details,
		UserVisible: true,
	}
}
