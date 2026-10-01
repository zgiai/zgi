package handler

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	llmerrors "github.com/zgiai/zgi/api/internal/modules/llm/errors"
	"github.com/zgiai/zgi/api/internal/modules/llm/gateway"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/pkg/apperror"
	apptransport "github.com/zgiai/zgi/api/pkg/apperror/transport"
)

const defaultRetryAfterSeconds = "60"

type protocolError struct {
	openAIStatus    int
	anthropicStatus int
	openAIType      string
	openAICode      string
	anthropicType   string
	message         string
	// appCode is the stable cataloged identity (domain.subject.reason). It may
	// differ from openAICode, which stays on the legacy protocol wire value.
	appCode string
	// retryable and errorID are populated at the projection boundary.
	retryable bool
	errorID   string
	requestID string
}

func classifyProtocolError(err error) protocolError {
	if err == nil {
		return internalProtocolError()
	}

	// The four billing causes must be resolved before the generic billing
	// failure: BillingUserError wraps DomainErrBillingFailed, so a later branch
	// would collapse organization/private-channel/workspace/model causes into a
	// system error or a generic insufficient_quota outcome.
	if protocolErr, ok := billingUserProtocolError(err); ok {
		return protocolErr
	}

	switch {
	case errors.Is(err, gateway.ErrModelNotAuthorized), errors.Is(err, llmerrors.DomainErrModelNotAuthorized):
		return newProtocolError(http.StatusForbidden, "permission_error", "model_not_authorized", "permission_error", "Model not authorized for this API key")
	case errors.Is(err, llmerrors.DomainErrModelNotFound), errors.Is(err, gateway.ErrModelNotFound), errors.Is(err, gateway.ErrModelNotActive), errors.Is(err, adapter.ErrModelNotFound):
		return newProtocolError(http.StatusNotFound, "invalid_request_error", "model_not_found", "not_found_error", "Model not found")
	case adapter.IsCapabilityUnsupported(err):
		return newAppProtocolError(llmerrors.AppCodeRequestInvalid, http.StatusBadRequest, "invalid_request_error", "unsupported_protocol", "invalid_request_error", "Requested model does not support this API protocol", false)
	case errors.Is(err, adapter.ErrContentPolicyViolation):
		return newAppProtocolError(llmerrors.AppCodeRequestInvalid, http.StatusBadRequest, "invalid_request_error", "content_policy_violation", "invalid_request_error", "Request was rejected by the content policy", false)
	case errors.Is(err, gateway.ErrMissingModel), errors.Is(err, gateway.ErrMissingMessages), errors.Is(err, gateway.ErrEmptyMessages), errors.Is(err, gateway.ErrInvalidRequest), errors.Is(err, adapter.ErrInvalidRequest), errors.Is(err, llmerrors.DomainErrMissingModel), errors.Is(err, llmerrors.DomainErrMissingMessages), errors.Is(err, llmerrors.DomainErrInvalidMessages), errors.Is(err, llmerrors.DomainErrInvalidTemperature), errors.Is(err, llmerrors.DomainErrInvalidMaxTokens):
		return invalidRequestProtocolError("Invalid request")
	case errors.Is(err, gateway.ErrInvalidAPIKey), errors.Is(err, gateway.ErrAPIKeyExpired), errors.Is(err, gateway.ErrAPIKeyInactive), errors.Is(err, gateway.ErrAPIKeyNotFound), errors.Is(err, llmerrors.DomainErrInvalidAPIKey), errors.Is(err, llmerrors.DomainErrAPIKeyDisabled):
		return invalidAPIKeyProtocolError("Invalid API key")
	case errors.Is(err, gateway.ErrInsufficientQuota), errors.Is(err, gateway.ErrInsufficientBalance), errors.Is(err, llmerrors.DomainErrInsufficientBalance), errors.Is(err, adapter.ErrInsufficientBalance), errors.Is(err, adapter.ErrQuotaExhausted):
		return quotaProtocolError()
	case errors.Is(err, llmerrors.DomainErrRateLimitExceeded):
		// The legacy OpenAI wire code stays rate_limit_exceeded; the stable
		// catalog identity is the same llm.rate_limit.exceeded code used by
		// persisted usage events.
		return newAppProtocolError(llmerrors.AppCodeRateLimitExceeded, http.StatusTooManyRequests, "rate_limit_error", "rate_limit_exceeded", "rate_limit_error", "The model service is receiving too many requests right now. Please try again shortly.", true)
	case errors.Is(err, adapter.ErrRateLimited):
		// An upstream 429 keeps the same legacy wire code but carries the
		// provider rate-limit identity, distinct from this gateway's own limit.
		return newAppProtocolError(llmerrors.AppCodeProviderRateLimited, http.StatusTooManyRequests, "rate_limit_error", "rate_limit_exceeded", "rate_limit_error", "The model provider is rate limiting requests. Please try again shortly.", true)
	case errors.Is(err, gateway.ErrPricingNotConfigured):
		return pricingNotConfiguredProtocolError()
	case errors.Is(err, llmerrors.DomainErrPrivateChannelUpstreamUnavailable):
		return newProtocolError(http.StatusServiceUnavailable, "server_error", "private_channel_upstream_unavailable", "api_error", "Private channel upstream credential is unavailable")
	case errors.Is(err, adapter.ErrPlatformChannelUnavailable):
		return newProtocolError(http.StatusBadGateway, "server_error", adapter.ErrorCodePlatformChannelUnavailable, "api_error", "Platform model service is temporarily unavailable")
	// The four selection failures below each keep a distinct branch and stable
	// code. They must never collapse into a single provider_unavailable outcome.
	case errors.Is(err, gateway.ErrNoProviderAvailable), errors.Is(err, llmerrors.DomainErrNoProviderAvailable):
		return newAppProtocolError(llmerrors.AppCodeNoProviderAvailable, http.StatusServiceUnavailable, "server_error", llmerrors.AppCodeNoProviderAvailable.String(), "api_error", "No model provider is currently available. Try another model or contact an administrator.", true)
	case errors.Is(err, llmerrors.DomainErrRouteNotFound):
		return newAppProtocolError(llmerrors.AppCodeNoProviderAvailable, http.StatusServiceUnavailable, "server_error", llmerrors.AppCodeNoProviderAvailable.String(), "api_error", "No model provider is currently available. Try another model or contact an administrator.", true)
	case errors.Is(err, gateway.ErrProviderNotFound), errors.Is(err, llmerrors.DomainErrProviderNotFound):
		return newAppProtocolError(llmerrors.AppCodeProviderNotFound, http.StatusNotFound, "invalid_request_error", llmerrors.AppCodeProviderNotFound.String(), "not_found_error", "The model provider was not found. Check the model configuration.", false)
	case errors.Is(err, llmerrors.DomainErrChannelNotFound):
		return newAppProtocolError(llmerrors.AppCodeChannelNotFound, http.StatusNotFound, "invalid_request_error", llmerrors.AppCodeChannelNotFound.String(), "not_found_error", "No configured model channel was found. Contact an administrator.", false)
	case errors.Is(err, gateway.ErrProviderUnavailable), errors.Is(err, llmerrors.DomainErrUpstreamUnavailable):
		return newAppProtocolError(llmerrors.AppCodeProviderUnavailable, http.StatusServiceUnavailable, "server_error", llmerrors.AppCodeProviderUnavailable.String(), "api_error", "The model service is temporarily unavailable. Try again later or choose another model.", true)
	// Temporary compatibility branch for legacy aggregated errors that carry no
	// stable code. T21 removes it once every producer emits an error_code or
	// ErrorInfo.Reason.
	case strings.Contains(err.Error(), "no provider available"):
		return newAppProtocolError(llmerrors.AppCodeNoProviderAvailable, http.StatusServiceUnavailable, "server_error", llmerrors.AppCodeNoProviderAvailable.String(), "api_error", "No model provider is currently available. Try another model or contact an administrator.", true)
	case isGatewayProviderTimeout(err):
		// Keep the legacy upstream_timeout wire code, add the catalog identity
		// so the error catalog code and localized message reach the client.
		return newAppProtocolError(llmerrors.AppCodeProviderTimeout, http.StatusGatewayTimeout, "server_error", "upstream_timeout", "timeout_error", "The model service took too long to respond. Please try again or choose another model.", true)
	case errors.Is(err, gateway.ErrBillingPreDeductFailed):
		return billingFailureProtocolError(llmerrors.AppCodeBillingPredeductFailed)
	case errors.Is(err, gateway.ErrBillingSettleFailed):
		return billingFailureProtocolError(llmerrors.AppCodeBillingSettleFailed)
	case errors.Is(err, gateway.ErrBalanceNotFound), errors.Is(err, gateway.ErrBillingFailed), errors.Is(err, gateway.ErrBillingLaneMismatch), errors.Is(err, gateway.ErrPricingCalculationFailed), errors.Is(err, llmerrors.DomainErrBillingFailed), errors.Is(err, llmerrors.DomainErrDatabaseError), errors.Is(err, llmerrors.DomainErrInternalError):
		return newAppProtocolError(llmerrors.AppCodeInvocationFailed, http.StatusBadGateway, "server_error", "internal_error", "api_error", "Internal server error", true)
	default:
		// Every unclassified upstream failure must still carry a catalog
		// identity, otherwise the client sees a bare upstream_error with no
		// code to render and no retry guidance.
		return newAppProtocolError(llmerrors.AppCodeInvocationFailed, http.StatusBadGateway, "server_error", "upstream_error", "api_error", "The model service could not complete the request. Please try again or choose another model.", true)
	}
}

// billingUserProtocolError projects one of the four explicit billing causes.
// Each cause keeps its own cataloged identity and never collapses into a
// generic insufficient_quota or internal_error response.
func billingUserProtocolError(err error) (protocolError, bool) {
	var userErr *gateway.BillingUserError
	if !errors.As(err, &userErr) || userErr == nil {
		return protocolError{}, false
	}
	switch userErr.Kind {
	case gateway.BillingUserErrorKindOrganizationBalanceInsufficient:
		return newAppProtocolError(
			llmerrors.AppCodeBillingOrganizationBalanceInsufficient,
			http.StatusPaymentRequired, "billing_error",
			llmerrors.AppCodeBillingOrganizationBalanceInsufficient.String(), "billing_error",
			"Your organization balance is insufficient. Add funds or contact an administrator.", false), true
	case gateway.BillingUserErrorKindPrivateChannelBalanceInsufficient:
		return newAppProtocolError(
			llmerrors.AppCodeBillingPrivateChannelBalanceInsufficient,
			http.StatusPaymentRequired, "billing_error",
			llmerrors.AppCodeBillingPrivateChannelBalanceInsufficient.String(), "billing_error",
			"The private channel balance is insufficient. Recharge the channel or choose another model.", false), true
	case gateway.BillingUserErrorKindWorkspaceQuotaInsufficient:
		return newAppProtocolError(
			llmerrors.AppCodeBillingWorkspaceQuotaInsufficient,
			http.StatusPaymentRequired, "billing_error",
			llmerrors.AppCodeBillingWorkspaceQuotaInsufficient.String(), "billing_error",
			"This workspace has reached its quota. Review the workspace quota or contact an administrator.", false), true
	case gateway.BillingUserErrorKindAPIKeyQuotaInsufficient:
		// The legacy OpenAI wire value stays insufficient_quota; the stable
		// identity moves to llm.quota.exceeded.
		err := newAppProtocolError(
			llmerrors.AppCodeQuotaExceeded,
			http.StatusTooManyRequests, "insufficient_quota", "insufficient_quota", "billing_error",
			"API key has insufficient quota", false)
		return err, true
	case gateway.BillingUserErrorKindModelPricingNotConfigured:
		return pricingNotConfiguredProtocolError(), true
	default:
		return protocolError{}, false
	}
}

func pricingNotConfiguredProtocolError() protocolError {
	return newAppProtocolError(
		llmerrors.AppCodeBillingModelPricingNotConfigured,
		http.StatusPaymentRequired, "invalid_request_error",
		llmerrors.AppCodeBillingModelPricingNotConfigured.String(), "invalid_request_error",
		"Pricing is not configured for this model. An administrator must configure it in model management or billing policy.", false)
}

func billingFailureProtocolError(code apperror.Code) protocolError {
	message := "The request could not be reserved for billing. Try again; if it continues, contact an administrator."
	if code == llmerrors.AppCodeBillingSettleFailed {
		message = "The request completed but billing settlement failed. It will be reconciled automatically; contact an administrator if it persists."
	}
	return newAppProtocolError(code, http.StatusInternalServerError, "server_error", code.String(), "api_error", message, true)
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

// newAppProtocolError builds a protocol error that carries a stable cataloged
// identity in addition to its legacy wire shape.
func newAppProtocolError(code apperror.Code, status int, openAIType, openAICode, anthropicType, message string, retryable bool) protocolError {
	protocolErr := newProtocolError(status, openAIType, openAICode, anthropicType, message)
	protocolErr.appCode = code.String()
	protocolErr.retryable = retryable
	return protocolErr
}

// invalidAPIKeyProtocolError keeps the legacy invalid_api_key wire shape and
// adds the catalog identity so shell-layer 401s carry the same top-level code
// as business errors.
func invalidAPIKeyProtocolError(message string) protocolError {
	return newAppProtocolError(llmerrors.AppCodeAPIKeyInvalid, http.StatusUnauthorized, "invalid_request_error", "invalid_api_key", "authentication_error", message, false)
}

// invalidRequestProtocolError keeps the legacy invalid_request wire shape and
// adds the catalog identity for shell-layer 400s.
func invalidRequestProtocolError(message string) protocolError {
	return newAppProtocolError(llmerrors.AppCodeRequestInvalid, http.StatusBadRequest, "invalid_request_error", "invalid_request", "invalid_request_error", message, false)
}

func quotaProtocolError() protocolError {
	// The legacy OpenAI wire shape stays insufficient_quota; the catalog
	// identity is llm.quota.exceeded.
	err := newAppProtocolError(llmerrors.AppCodeQuotaExceeded, http.StatusTooManyRequests, "insufficient_quota", "insufficient_quota", "billing_error", "This API key has no remaining quota. Check your usage or contact an administrator.", false)
	err.anthropicStatus = http.StatusPaymentRequired
	return err
}

func internalProtocolError() protocolError {
	return newAppProtocolError(llmerrors.AppCodeInvocationFailed, http.StatusBadGateway, "server_error", "internal_error", "api_error", "Internal server error", true)
}

func writeProtocolError(c *gin.Context, protocolErr protocolError) {
	if isGeminiRequest(c) {
		writeGeminiProtocolError(c, protocolErr)
		return
	}
	if isAnthropicRequest(c) {
		writeAnthropicProtocolError(c, protocolErr)
		return
	}
	writeOpenAIProtocolError(c, protocolErr)
}

// writeProtocolErrorHeaders applies the headers that belong to every protocol
// envelope. Retry-After accompanies any 429 regardless of protocol.
func writeProtocolErrorHeaders(c *gin.Context, protocolErr protocolError) {
	if protocolErr.openAIStatus == http.StatusTooManyRequests || protocolErr.anthropicStatus == http.StatusTooManyRequests {
		c.Header("Retry-After", defaultRetryAfterSeconds)
	}
	if strings.TrimSpace(protocolErr.appCode) != "" {
		c.Header(apptransport.HeaderApplicationErrorCode, protocolErr.appCode)
	}
	requestID := protocolErr.requestID
	if requestID == "" {
		requestID = requestIDFromContext(c)
	}
	if requestID != "" && strings.TrimSpace(c.Writer.Header().Get("X-Request-ID")) == "" {
		c.Header("X-Request-ID", requestID)
	}
}

// applyProtocolErrorEnvelope adds the cataloged public fields. It is additive
// and only runs when the error came through the projection boundary, so
// unprojected legacy responses keep their original exact shape.
func applyProtocolErrorEnvelope(c *gin.Context, body gin.H, protocolErr protocolError) gin.H {
	code := strings.TrimSpace(protocolErr.appCode)
	if code == "" {
		code = strings.TrimSpace(protocolErr.openAICode)
	}
	if code == "" {
		code = "internal_error"
	}
	body["code"] = code
	body["error_code"] = code
	body["params"] = map[string]string{}
	body["retryable"] = protocolErr.retryable
	if strings.TrimSpace(protocolErr.errorID) == "" {
		protocolErr.errorID = uuid.NewString()
	}
	body["error_id"] = protocolErr.errorID
	requestID := protocolErr.requestID
	if requestID == "" {
		requestID = requestIDFromContext(c)
	}
	if requestID != "" {
		body["request_id"] = requestID
	}
	body["message"] = protocolErr.message
	return body
}

func writeGeminiProtocolError(c *gin.Context, protocolErr protocolError) {
	writeProtocolErrorHeaders(c, protocolErr)
	body := applyProtocolErrorEnvelope(c, gin.H{
		"error": gin.H{
			"code": protocolErr.openAIStatus, "message": protocolErr.message,
			"status": geminiErrorStatus(protocolErr.openAIStatus),
		},
	}, protocolErr)
	c.JSON(protocolErr.openAIStatus, body)
}

func geminiErrorStatus(status int) string {
	switch status {
	case http.StatusBadRequest:
		return "INVALID_ARGUMENT"
	case http.StatusUnauthorized:
		return "UNAUTHENTICATED"
	case http.StatusForbidden:
		return "PERMISSION_DENIED"
	case http.StatusNotFound:
		return "NOT_FOUND"
	case http.StatusTooManyRequests, http.StatusRequestEntityTooLarge:
		return "RESOURCE_EXHAUSTED"
	case http.StatusPaymentRequired:
		return "FAILED_PRECONDITION"
	case http.StatusServiceUnavailable:
		return "UNAVAILABLE"
	default:
		return "INTERNAL"
	}
}

func isGeminiRequest(c *gin.Context) bool {
	return strings.HasPrefix(c.Request.URL.Path, "/v1beta/")
}

func writeOpenAIProtocolError(c *gin.Context, protocolErr protocolError) {
	writeProtocolErrorHeaders(c, protocolErr)
	body := applyProtocolErrorEnvelope(c, gin.H{
		"error": gin.H{
			"message": protocolErr.message,
			"type":    protocolErr.openAIType,
			"param":   nil,
			"code":    protocolErr.openAICode,
		},
	}, protocolErr)
	c.JSON(protocolErr.openAIStatus, body)
}

func writeAnthropicProtocolError(c *gin.Context, protocolErr protocolError) {
	writeProtocolErrorHeaders(c, protocolErr)
	body := applyProtocolErrorEnvelope(c, gin.H{
		"type": "error",
		"error": gin.H{
			"type":    protocolErr.anthropicType,
			"message": protocolErr.message,
		},
		"request_id": requestIDFromContext(c),
	}, protocolErr)
	c.JSON(protocolErr.anthropicStatus, body)
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
