package middleware

import (
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/zgiai/zgi/api/internal/util"
	"github.com/zgiai/zgi/api/pkg/logger"
)

// AuditLogger records console write operations in the shared app log.
func AuditLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()

		if !shouldAudit(c) {
			return
		}

		fields := []interface{}{
			"log_type", "audit",
			"request_id", c.GetString(requestIDContextKey),
			"method", c.Request.Method,
			"path", c.Request.URL.Path,
			"status", c.Writer.Status(),
			"client_ip", c.ClientIP(),
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

		logger.Info("console audit", fields...)
	}
}

func shouldAudit(c *gin.Context) bool {
	if !strings.HasPrefix(c.Request.URL.Path, "/console/api") {
		return false
	}

	switch c.Request.Method {
	case "POST", "PUT", "PATCH", "DELETE":
		return true
	default:
		return false
	}
}
