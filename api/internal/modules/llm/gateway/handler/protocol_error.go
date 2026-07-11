package handler

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	llmerrors "github.com/zgiai/zgi/api/internal/modules/llm/errors"
	"github.com/zgiai/zgi/api/internal/modules/llm/gateway"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

type protocolError struct {
	openAIStatus    int
	anthropicStatus int
	openAIType      string
	openAICode      string
	anthropicType   string
	message         string
}

func classifyProtocolError(err error) protocolError {
	if err == nil {
		return internalProtocolError()
	}

	switch {
	case errors.Is(err, gateway.ErrModelNotAuthorized), errors.Is(err, llmerrors.DomainErrModelNotAuthorized):
		return newProtocolError(http.StatusForbidden, "permission_error", "model_not_authorized", "permission_error", "Model not authorized for this API key")
	case errors.Is(err, llmerrors.DomainErrModelNotFound), errors.Is(err, llmerrors.DomainErrRouteNotFound), errors.Is(err, gateway.ErrModelNotFound), errors.Is(err, gateway.ErrModelNotActive), errors.Is(err, adapter.ErrModelNotFound):
		return newProtocolError(http.StatusNotFound, "invalid_request_error", "model_not_found", "not_found_error", "Model not found")
	case adapter.IsCapabilityUnsupported(err):
		return newProtocolError(http.StatusBadRequest, "invalid_request_error", "unsupported_protocol", "invalid_request_error", "Requested model does not support this API protocol")
	case errors.Is(err, adapter.ErrContentPolicyViolation):
		return newProtocolError(http.StatusBadRequest, "invalid_request_error", "content_policy_violation", "invalid_request_error", "Request was rejected by the content policy")
	case errors.Is(err, gateway.ErrMissingModel), errors.Is(err, gateway.ErrMissingMessages), errors.Is(err, gateway.ErrEmptyMessages), errors.Is(err, gateway.ErrInvalidRequest), errors.Is(err, adapter.ErrInvalidRequest), errors.Is(err, llmerrors.DomainErrMissingModel), errors.Is(err, llmerrors.DomainErrMissingMessages), errors.Is(err, llmerrors.DomainErrInvalidMessages), errors.Is(err, llmerrors.DomainErrInvalidTemperature), errors.Is(err, llmerrors.DomainErrInvalidMaxTokens):
		return invalidRequestProtocolError("Invalid request")
	case errors.Is(err, gateway.ErrInvalidAPIKey), errors.Is(err, gateway.ErrAPIKeyExpired), errors.Is(err, gateway.ErrAPIKeyInactive), errors.Is(err, gateway.ErrAPIKeyNotFound), errors.Is(err, llmerrors.DomainErrInvalidAPIKey), errors.Is(err, llmerrors.DomainErrAPIKeyDisabled):
		return invalidAPIKeyProtocolError("Invalid API key")
	case errors.Is(err, gateway.ErrInsufficientQuota), errors.Is(err, gateway.ErrInsufficientBalance), errors.Is(err, llmerrors.DomainErrInsufficientBalance), errors.Is(err, adapter.ErrInsufficientBalance):
		return quotaProtocolError()
	case errors.Is(err, llmerrors.DomainErrRateLimitExceeded), errors.Is(err, adapter.ErrRateLimited):
		return newProtocolError(http.StatusTooManyRequests, "rate_limit_error", "rate_limit_exceeded", "rate_limit_error", "Rate limit exceeded")
	case errors.Is(err, gateway.ErrNoProviderAvailable), errors.Is(err, gateway.ErrProviderNotFound), errors.Is(err, gateway.ErrProviderUnavailable), errors.Is(err, llmerrors.DomainErrNoProviderAvailable), errors.Is(err, llmerrors.DomainErrProviderNotFound), errors.Is(err, llmerrors.DomainErrChannelNotFound), errors.Is(err, llmerrors.DomainErrUpstreamUnavailable), strings.Contains(err.Error(), "no provider available"):
		return newProtocolError(http.StatusServiceUnavailable, "server_error", "provider_unavailable", "api_error", "No provider is currently available for this model")
	case errors.Is(err, adapter.ErrTimeout), errors.Is(err, llmerrors.DomainErrUpstreamTimeout):
		return newProtocolError(http.StatusGatewayTimeout, "server_error", "upstream_timeout", "timeout_error", "Upstream provider timed out")
	case errors.Is(err, gateway.ErrBalanceNotFound), errors.Is(err, gateway.ErrBillingFailed), errors.Is(err, gateway.ErrBillingPreDeductFailed), errors.Is(err, gateway.ErrBillingSettleFailed), errors.Is(err, gateway.ErrBillingLaneMismatch), errors.Is(err, llmerrors.DomainErrBillingFailed), errors.Is(err, llmerrors.DomainErrDatabaseError), errors.Is(err, llmerrors.DomainErrInternalError):
		return newProtocolError(http.StatusInternalServerError, "server_error", "internal_error", "api_error", "Internal server error")
	default:
		return newProtocolError(http.StatusBadGateway, "server_error", "upstream_error", "api_error", "Upstream provider request failed")
	}
}

func newProtocolError(status int, openAIType, openAICode, anthropicType, message string) protocolError {
	return protocolError{
		openAIStatus:    status,
		anthropicStatus: status,
		openAIType:      openAIType,
		openAICode:      openAICode,
		anthropicType:   anthropicType,
		message:         message,
	}
}

func invalidAPIKeyProtocolError(message string) protocolError {
	return newProtocolError(http.StatusUnauthorized, "invalid_request_error", "invalid_api_key", "authentication_error", message)
}

func invalidRequestProtocolError(message string) protocolError {
	return newProtocolError(http.StatusBadRequest, "invalid_request_error", "invalid_request", "invalid_request_error", message)
}

func quotaProtocolError() protocolError {
	err := newProtocolError(http.StatusTooManyRequests, "insufficient_quota", "insufficient_quota", "billing_error", "API key has insufficient quota")
	err.anthropicStatus = http.StatusPaymentRequired
	return err
}

func internalProtocolError() protocolError {
	return newProtocolError(http.StatusInternalServerError, "server_error", "internal_error", "api_error", "Internal server error")
}

func writeProtocolError(c *gin.Context, protocolErr protocolError) {
	if isAnthropicRequest(c) {
		writeAnthropicProtocolError(c, protocolErr)
		return
	}
	writeOpenAIProtocolError(c, protocolErr)
}

func writeOpenAIProtocolError(c *gin.Context, protocolErr protocolError) {
	c.JSON(protocolErr.openAIStatus, gin.H{
		"error": gin.H{
			"message": protocolErr.message,
			"type":    protocolErr.openAIType,
			"param":   nil,
			"code":    protocolErr.openAICode,
		},
	})
}

func writeAnthropicProtocolError(c *gin.Context, protocolErr protocolError) {
	c.JSON(protocolErr.anthropicStatus, gin.H{
		"type": "error",
		"error": gin.H{
			"type":    protocolErr.anthropicType,
			"message": protocolErr.message,
		},
		"request_id": requestIDFromContext(c),
	})
}

func abortWithProtocolError(c *gin.Context, protocolErr protocolError) {
	writeProtocolError(c, protocolErr)
	c.Abort()
}

func isAnthropicRequest(c *gin.Context) bool {
	path := c.Request.URL.Path
	return path == "/v1/messages" || strings.HasPrefix(path, "/anthropic/")
}

func requestIDFromContext(c *gin.Context) string {
	if requestID := strings.TrimSpace(c.GetString("request_id")); requestID != "" {
		return requestID
	}
	return strings.TrimSpace(c.Writer.Header().Get("X-Request-ID"))
}
