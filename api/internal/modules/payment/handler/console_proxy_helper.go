package handler

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	platformconsole "github.com/zgiai/ginext/internal/infra/platform/console"
)

func isConsolePaymentProxyEnabled(cp platformconsole.ConsoleProvider) bool {
	// Payment domain is console-only. As long as provider is injected, handlers
	// should route to console and surface provider unavailability as an error
	// instead of falling back to legacy local payment logic.
	return cp != nil && cp.IsAvailable()
}

func requireConsolePaymentProxy(c *gin.Context, cp platformconsole.ConsoleProvider) bool {
	if isConsolePaymentProxyEnabled(cp) {
		return true
	}
	errorResponse(c, http.StatusServiceUnavailable, "Console payment service is unavailable")
	return false
}

func writeConsoleProxyError(c *gin.Context, err error, fallbackStatus int, fallbackMessage string) {
	var apiErr *platformconsole.ConsoleAPIError
	if errors.As(err, &apiErr) {
		status := apiErr.StatusCode
		if status < 400 || status >= 600 {
			status = fallbackStatus
		}
		msg := strings.TrimSpace(apiErr.Message)
		if msg == "" {
			msg = fallbackMessage
		}
		errorResponse(c, status, msg)
		return
	}

	msg := fallbackMessage
	if strings.TrimSpace(err.Error()) != "" {
		msg = err.Error()
	}
	errorResponse(c, fallbackStatus, msg)
}
