package handler

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	llmerrors "github.com/zgiai/zgi/api/internal/modules/llm/errors"
	"github.com/zgiai/zgi/api/pkg/apperror"
	apptransport "github.com/zgiai/zgi/api/pkg/apperror/transport"
)

const defaultGatewayRequestBodyLimit int64 = 10 << 20

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
