package handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	llmerrors "github.com/zgiai/zgi/api/internal/modules/llm/errors"
	"github.com/zgiai/zgi/api/internal/modules/llm/gateway"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/pkg/apperror"
	appcatalog "github.com/zgiai/zgi/api/pkg/apperror/catalog"
	apptransport "github.com/zgiai/zgi/api/pkg/apperror/transport"
)

var (
	legacyOpenAIUpstreamTimeout = appcatalog.MustLegacyKey("llm.openai:upstream_timeout")
	legacyAnthropicTimeoutError = appcatalog.MustLegacyKey("llm.anthropic:timeout_error")
	legacyPersonalKeyQuota      = appcatalog.MustLegacyKey("llm.personal_key.quota:invalid_api_key")
	legacyPersonalKeyExpired    = appcatalog.MustLegacyKey("llm.personal_key.expired:invalid_api_key")
	legacyPersonalKeyInactive   = appcatalog.MustLegacyKey("llm.personal_key.inactive:invalid_api_key")
)

// principalAccessProtocolError changes only the public message. Keep HTTP 401,
// invalid_api_key/authentication_error and the existing JSON/header contract
// until a separately reviewed protocol migration changes these wire values.
func principalAccessProtocolError(c *gin.Context, err error, projector *apptransport.Projector) protocolError {
	legacy := invalidAPIKeyProtocolError("API key access has been revoked")
	var legacyKey appcatalog.LegacyKey
	switch {
	case apperror.IsCode(err, llmerrors.AppCodeDeveloperQuotaExhausted):
		legacyKey = legacyPersonalKeyQuota
		legacy.message = "Developer API access has no available quota. Check usage or contact your workspace administrator."
	case apperror.IsCode(err, llmerrors.AppCodeAPIKeyExpired):
		legacyKey = legacyPersonalKeyExpired
		legacy.message = "The API key has expired. Create or select an active key."
	case apperror.IsCode(err, llmerrors.AppCodeAPIKeyInactive):
		legacyKey = legacyPersonalKeyInactive
		legacy.message = "The API key is disabled. Enable it or use another key."
	default:
		return legacy
	}
	if projector != nil {
		message := projector.ProjectLegacyMessage(err,
			apptransport.LocaleFromAcceptLanguage(c.GetHeader("Accept-Language")), legacyKey)
		if message.Resolution == apptransport.ResolutionMatched {
			legacy.message = message.Message
		}
	}
	return legacy
}

// localizedProtocolError projects the application error at the protocol
// boundary. The classifier keeps ownership of status, type, and legacy wire
// code; the catalog supplies the localized message, stable code, and retryable
// flag, and the projector mints the error_id.
func (h *LLMHandler) localizedProtocolError(c *gin.Context, err error) protocolError {
	legacy := classifyProtocolError(err)
	if h == nil || h.errorProjector == nil || err == nil {
		return legacy
	}
	projectionTarget, supported := applicationErrorFor(err)
	if !supported {
		projectionTarget = err
	}

	locale := apptransport.LocaleFromAcceptLanguage(c.GetHeader("Accept-Language"))
	result := h.errorProjector.Project(projectionTarget, locale)
	legacy.errorID = result.ErrorID
	if result.Resolution == apptransport.ResolutionMatched {
		legacy.message = result.Presentation.Message
		legacy.appCode = result.Presentation.Code.String()
		legacy.retryable = result.Presentation.Retryable
	}
	return legacy
}

// localizedChatStreamError returns a safe projected message for every stream
// failure. Diagnostic causes never reach the client, whether or not the error
// has a registered catalog code.
func (h *LLMHandler) localizedChatStreamError(c *gin.Context, err error) string {
	return h.localizedProtocolError(c, err).message
}

// localizedInvalidRequest overlays the catalog's locale-specific request
// message while preserving each protocol's legacy status/type/code shape.
// Validation errors are created at the protocol boundary, so they otherwise
// bypass the application-error projector and leak the English detail string.
func (h *LLMHandler) localizedInvalidRequest(c *gin.Context, protocolErr protocolError) protocolError {
	projected := h.localizedProtocolError(c, gateway.ErrInvalidRequest)
	protocolErr.message = projected.message
	protocolErr.appCode = projected.appCode
	protocolErr.retryable = projected.retryable
	protocolErr.errorID = projected.errorID
	return protocolErr
}

func isGatewayProviderTimeout(err error) bool {
	if errors.Is(err, adapter.ErrTimeout) ||
		errors.Is(err, llmerrors.DomainErrUpstreamTimeout) {
		return true
	}

	var adapterErr *adapter.AdapterError
	if errors.As(err, &adapterErr) && adapterErr.StatusCode == http.StatusGatewayTimeout {
		return true
	}
	var statusErr *adapter.HTTPStatusError
	return errors.As(err, &statusErr) && statusErr.StatusCode == http.StatusGatewayTimeout
}

func normalizeGatewayApplicationError(err error) error {
	appErr, _ := applicationErrorFor(err)
	return appErr
}
