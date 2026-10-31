package handler

import (
	"bytes"
	"errors"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	llmerrors "github.com/zgiai/zgi/api/internal/modules/llm/errors"
	"github.com/zgiai/zgi/api/pkg/apperror"
	apptransport "github.com/zgiai/zgi/api/pkg/apperror/transport"
	"github.com/zgiai/zgi/api/pkg/logger"
)

const (
	// GatewayRequestBodyLimitEnv names the setting that caps public gateway
	// request bodies, in MiB.
	GatewayRequestBodyLimitEnv = "LLM_GATEWAY_REQUEST_BODY_LIMIT_MB"
	// DefaultGatewayRequestBodyLimitMB is used when the setting is unset or invalid.
	DefaultGatewayRequestBodyLimitMB int64 = 100

	defaultGatewayRequestBodyLimit int64 = DefaultGatewayRequestBodyLimitMB << 20
)

// GatewayRequestBodyLimitBytes converts the raw LLM_GATEWAY_REQUEST_BODY_LIMIT_MB
// value to bytes. An empty value uses the default; a value that is not a
// positive whole number of MiB (or would overflow) falls back to the default
// and logs a warning, so a typo never takes the gateway down.
func GatewayRequestBodyLimitBytes(raw string) int64 {
	return resolveGatewayRequestBodyLimit(raw, logger.Warn)
}

func resolveGatewayRequestBodyLimit(raw string, warn func(msg string, args ...interface{})) int64 {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return defaultGatewayRequestBodyLimit
	}
	mb, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || mb <= 0 || mb > math.MaxInt64>>20 {
		if warn != nil {
			warn("invalid gateway request body limit, using default",
				"env", GatewayRequestBodyLimitEnv,
				"value", raw,
				"default_mb", DefaultGatewayRequestBodyLimitMB)
		}
		return defaultGatewayRequestBodyLimit
	}
	return mb << 20
}

// GatewayBodyLimit rejects oversized public LLM protocol requests before API
// key lookup, JSON parsing, model discovery, routing, or billing.
func GatewayBodyLimit(maxBytes int64, projectors ...*apptransport.Projector) gin.HandlerFunc {
	if maxBytes <= 0 {
		maxBytes = defaultGatewayRequestBodyLimit
	}
	var projector *apptransport.Projector
	if len(projectors) > 0 {
		projector = projectors[0]
	}
	return func(c *gin.Context) {
		if !isBodyLimitedGatewayPath(c.Request.URL.Path) {
			c.Next()
			return
		}
		if c.Request.ContentLength > maxBytes {
			writeGatewayBodyTooLarge(c, projector)
			return
		}
		limited := http.MaxBytesReader(c.Writer, c.Request.Body, maxBytes)
		body, err := io.ReadAll(limited)
		if err != nil {
			var tooLarge *http.MaxBytesError
			if errors.As(err, &tooLarge) {
				writeGatewayBodyTooLarge(c, projector)
				return
			}
			writeGatewayMalformedBody(c)
			return
		}
		c.Request.Body = io.NopCloser(bytes.NewReader(body))
		c.Request.ContentLength = int64(len(body))
		c.Next()
	}
}

func isBodyLimitedGatewayPath(path string) bool {
	path = strings.TrimSuffix(path, "/")
	return strings.HasPrefix(path, "/v1/") ||
		strings.HasPrefix(path, "/anthropic/") ||
		strings.HasPrefix(path, "/v1beta/")
}

// writeGatewayBodyTooLarge keeps the 413 wire shape and takes the message and
// error_id from the catalog's llm.request.too_large entry, so the rejection is
// answered in the caller's language.
func writeGatewayBodyTooLarge(c *gin.Context, projector *apptransport.Projector) {
	protocolErr := newAppProtocolError(llmerrors.AppCodeRequestTooLarge, http.StatusRequestEntityTooLarge, "invalid_request_error", llmerrors.AppCodeRequestTooLarge.String(), "invalid_request_error", "Request body too large", false)
	if projector != nil {
		locale := apptransport.LocaleFromAcceptLanguage(c.GetHeader("Accept-Language"))
		result := projector.ProjectWithRequestID(apperror.New(llmerrors.AppCodeRequestTooLarge), locale, c.Writer.Header().Get("X-Request-ID"))
		protocolErr.errorID = result.ErrorID
		if result.Resolution == apptransport.ResolutionMatched {
			protocolErr.message = result.Presentation.Message
		}
	}
	abortWithProtocolError(c, protocolErr)
}

func writeGatewayMalformedBody(c *gin.Context) {
	abortWithProtocolError(c, invalidRequestProtocolError("Invalid request body"))
}
