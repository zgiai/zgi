package handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	llmerrors "github.com/zgiai/zgi/api/internal/modules/llm/errors"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/pkg/apperror"
	appcatalog "github.com/zgiai/zgi/api/pkg/apperror/catalog"
	apptransport "github.com/zgiai/zgi/api/pkg/apperror/transport"
)

var (
	legacyOpenAIUpstreamTimeout = appcatalog.MustLegacyKey("llm.openai:upstream_timeout")
	legacyAnthropicTimeoutError = appcatalog.MustLegacyKey("llm.anthropic:timeout_error")
)

// localizedProtocolError is an error-only compatibility adapter. The protocol
// classifier continues to own status, type, code, and wire shape; the
// application catalog may replace only the safe public message.
func (h *LLMHandler) localizedProtocolError(c *gin.Context, err error) protocolError {
	legacy := classifyProtocolError(err)
	if h == nil || h.errorProjector == nil {
		return legacy
	}
	appErr, supported := gatewayApplicationError(err)
	if !supported {
		return legacy
	}

	legacyKey := legacyOpenAIUpstreamTimeout
	if isAnthropicRequest(c) {
		legacyKey = legacyAnthropicTimeoutError
	}
	locale := apptransport.LocaleFromAcceptLanguage(c.GetHeader("Accept-Language"))
	message := h.errorProjector.ProjectLegacyMessage(appErr, locale, legacyKey)
	if message.Resolution == apptransport.ResolutionMatched {
		legacy.message = message.Message
	}
	return legacy
}

func (h *LLMHandler) localizedChatStreamError(c *gin.Context, err error) string {
	if _, supported := gatewayApplicationError(err); !supported {
		return err.Error()
	}
	return h.localizedProtocolError(c, err).message
}

func gatewayApplicationError(err error) (error, bool) {
	if apperror.IsCode(err, llmerrors.AppCodeProviderTimeout) {
		return err, true
	}
	if !isGatewayProviderTimeout(err) {
		return err, false
	}
	return apperror.Wrap(
		err,
		llmerrors.AppCodeProviderTimeout,
		apperror.WithOperation("gateway.protocol_error"),
	), true
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
	appErr, _ := gatewayApplicationError(err)
	return appErr
}
