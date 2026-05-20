package middleware

import (
	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// OpenTelemetryRequestAttributes adds project-specific request metadata to the active HTTP span.
func OpenTelemetryRequestAttributes() gin.HandlerFunc {
	return func(c *gin.Context) {
		span := trace.SpanFromContext(c.Request.Context())
		if span.IsRecording() {
			if requestID := c.GetString(requestIDContextKey); requestID != "" {
				span.SetAttributes(attribute.String("zgi.http_request_id", requestID))
			}
		}

		c.Next()

		span = trace.SpanFromContext(c.Request.Context())
		if !span.IsRecording() {
			return
		}
		if route := c.FullPath(); route != "" {
			span.SetAttributes(attribute.String("zgi.http.route", route))
		}
	}
}
