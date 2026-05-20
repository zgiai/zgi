package middleware

import (
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/pkg/logger"
	"go.uber.org/zap"
)

const (
	requestIDContextKey = "request_id"
	requestIDHeader     = "X-Request-ID"
)

// RequestID ensures every request has a stable request ID.
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		requestID := strings.TrimSpace(c.GetHeader(requestIDHeader))
		if requestID == "" {
			requestID = uuid.NewString()
		}

		c.Set(requestIDContextKey, requestID)
		c.Request = c.Request.WithContext(
			logger.WithFields(c.Request.Context(), zap.String(requestIDContextKey, requestID)),
		)
		c.Header(requestIDHeader, requestID)
		c.Next()
	}
}
