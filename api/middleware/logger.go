package middleware

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/zgiai/ginext/internal/util"
	"github.com/zgiai/ginext/pkg/logger"
)

// Logger records a structured access log for each HTTP request.
func Logger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()

		status := c.Writer.Status()
		latency := time.Since(start)
		fields := []interface{}{
			"log_type", "access",
			"request_id", c.GetString(requestIDContextKey),
			"method", c.Request.Method,
			"path", c.Request.URL.Path,
			"status", status,
			"latency_ms", latency.Milliseconds(),
			"client_ip", c.ClientIP(),
			"user_agent", c.Request.UserAgent(),
		}

		if route := c.FullPath(); route != "" {
			fields = append(fields, "route", route)
		}
		if accountID := c.GetString("account_id"); accountID != "" {
			fields = append(fields, "account_id", accountID)
		}
		if tenantID := util.GetOrganizationIDCompat(c); tenantID != "" {
			fields = append(fields, "tenant_id", tenantID)
		}

		switch {
		case status >= 500:
			logger.Error("http request", fields...)
		case status >= 400:
			logger.Warn("http request", fields...)
		default:
			logger.Info("http request", fields...)
		}
	}
}
