package handler

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	llmerrors "github.com/zgiai/zgi/api/internal/modules/llm/errors"
)

const defaultGatewayRequestBodyLimit int64 = 10 << 20

// GatewayBodyLimit rejects oversized public LLM protocol requests before API
// key lookup, JSON parsing, model discovery, routing, or billing.
func GatewayBodyLimit(maxBytes int64) gin.HandlerFunc {
	if maxBytes <= 0 {
		maxBytes = defaultGatewayRequestBodyLimit
	}
	return func(c *gin.Context) {
		if !isBodyLimitedGatewayPath(c.Request.URL.Path) {
			c.Next()
			return
		}
		if c.Request.ContentLength > maxBytes {
			writeGatewayBodyTooLarge(c)
			return
		}
		limited := http.MaxBytesReader(c.Writer, c.Request.Body, maxBytes)
		body, err := io.ReadAll(limited)
		if err != nil {
			var tooLarge *http.MaxBytesError
			if errors.As(err, &tooLarge) {
				writeGatewayBodyTooLarge(c)
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

func writeGatewayBodyTooLarge(c *gin.Context) {
	writeGatewayRequestError(c, http.StatusRequestEntityTooLarge, llmerrors.AppCodeRequestTooLarge.String(), "Request body too large")
}

func writeGatewayMalformedBody(c *gin.Context) {
	writeGatewayRequestError(c, http.StatusBadRequest, "invalid_request", "Invalid request body")
}

func writeGatewayRequestError(c *gin.Context, status int, code, message string) {
	path := strings.ToLower(c.Request.URL.Path)
	switch {
	case strings.Contains(path, "/messages"):
		c.AbortWithStatusJSON(status, gin.H{"type": "error", "error": gin.H{"type": "invalid_request_error", "message": message}})
	case strings.Contains(path, "/v1beta/models/"):
		c.AbortWithStatusJSON(status, gin.H{"error": gin.H{"code": status, "message": message, "status": geminiErrorStatus(status)}})
	default:
		c.AbortWithStatusJSON(status, gin.H{"error": gin.H{"message": message, "type": "invalid_request_error", "code": code, "param": nil}})
	}
}
