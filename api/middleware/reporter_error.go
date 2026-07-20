package middleware

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/zgiai/zgi/api/internal/observability"
)

// ZGIErrorReporter reports unexpected HTTP failures through the provider-neutral
// ZGI Reporter facade. Common client and business errors remain excluded.
func ZGIErrorReporter(reporter *observability.ZGIReporter) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()

		statusCode := c.Writer.Status()
		if reporter == nil || !reporter.Enabled() || statusCode < http.StatusBadRequest {
			return
		}

		switch statusCode {
		case http.StatusBadRequest,
			http.StatusUnauthorized,
			http.StatusForbidden,
			http.StatusNotFound,
			http.StatusConflict:
			return
		}

		level := observability.LevelWarning
		if statusCode >= http.StatusInternalServerError {
			level = observability.LevelError
		}

		reportErr := errors.New(http.StatusText(statusCode))
		if len(c.Errors) > 0 && c.Errors.Last().Err != nil {
			reportErr = c.Errors.Last().Err
		}
		_ = reporter.Report(c.Request.Context(), observability.Event{
			Name:  "http.request.failed",
			Kind:  observability.EventKindError,
			Level: level,
			Err:   reportErr,
			Tags: map[string]string{
				"http.method":      c.Request.Method,
				"http.route":       c.FullPath(),
				"http.status_code": strconv.Itoa(statusCode),
			},
			Attributes: map[string]any{
				"request_id": c.GetString("request_id"),
				"user_id":    c.GetString("user_id"),
				"tenant_id":  c.GetString("tenant_id"),
			},
		})
	}
}
