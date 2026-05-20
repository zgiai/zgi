package middleware

import (
	"net/http"

	"github.com/getsentry/sentry-go"
	sentrygin "github.com/getsentry/sentry-go/gin"
	"github.com/gin-gonic/gin"
)

// SentryErrorReporter captures HTTP errors (4xx, 5xx) to Sentry
func SentryErrorReporter() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()

		// Only capture errors for status codes >= 400
		statusCode := c.Writer.Status()
		if statusCode < 400 {
			return
		}

		// Get Sentry hub from context
		hub := sentrygin.GetHubFromContext(c)
		if hub == nil {
			return
		}

		// Determine severity level
		var level sentry.Level
		if statusCode >= 500 {
			level = sentry.LevelError
		} else if statusCode >= 400 {
			level = sentry.LevelWarning
		}

		// Skip common client errors that are not bugs
		skipErrors := map[int]bool{
			400: true, // Bad Request - usually client error
			401: true, // Unauthorized - expected behavior
			403: true, // Forbidden - expected behavior
			404: true, // Not Found - expected behavior
			409: true, // Conflict - expected business logic
		}

		// Only report 4xx errors that are likely bugs
		if statusCode >= 400 && statusCode < 500 && skipErrors[statusCode] {
			return
		}

		// Get error from context if available
		var errorMsg string
		if len(c.Errors) > 0 {
			errorMsg = c.Errors.String()
		} else {
			errorMsg = http.StatusText(statusCode)
		}

		hub.WithScope(func(scope *sentry.Scope) {
			scope.SetLevel(level)
			scope.SetTag("http_status", http.StatusText(statusCode))
			scope.SetTag("http_method", c.Request.Method)
			scope.SetTag("http_path", c.Request.URL.Path)
			scope.SetExtra("status_code", statusCode)
			scope.SetExtra("request_id", c.GetString("request_id"))
			scope.SetExtra("user_id", c.GetString("user_id"))
			scope.SetExtra("tenant_id", c.GetString("tenant_id"))

			// Capture the error
			if len(c.Errors) > 0 {
				// If there are errors in context, capture the last one
				hub.CaptureException(c.Errors.Last().Err)
			} else {
				// Otherwise, capture a message
				hub.CaptureMessage(errorMsg)
			}
		})
	}
}
