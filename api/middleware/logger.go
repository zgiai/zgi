package middleware

import (
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/zgiai/zgi/api/internal/util"
	"github.com/zgiai/zgi/api/pkg/logger"
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
			"path", requestLogPath(c),
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

// requestLogPath avoids persisting path parameters such as public invitation
// tokens. Matched requests use Gin's route template; unmatched requests retain
// their concrete path so 404 and fallback diagnostics remain useful.
func requestLogPath(c *gin.Context) string {
	if c == nil {
		return ""
	}
	if route := strings.TrimSpace(c.FullPath()); route != "" {
		return route
	}
	if c.Request == nil || c.Request.URL == nil {
		return ""
	}
	return redactSensitiveRequestPath(c.Request.URL.Path)
}

func redactSensitiveRequestPath(path string) string {
	segments := strings.Split(path, "/")
	for i := 0; i+2 < len(segments); i++ {
		if strings.EqualFold(segments[i], "public") &&
			strings.EqualFold(segments[i+1], "invites") &&
			strings.TrimSpace(segments[i+2]) != "" {
			segments[i+2] = "[REDACTED]"
		}
	}
	return strings.Join(segments, "/")
}
