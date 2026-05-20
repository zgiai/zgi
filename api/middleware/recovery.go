package middleware

import (
	"fmt"

	"github.com/gin-gonic/gin"
	"github.com/zgiai/zgi/api/internal/util"
	"github.com/zgiai/zgi/api/pkg/logger"
	"github.com/zgiai/zgi/api/pkg/response"
)

// Recovery handles panic recovery and writes a structured error log.
func Recovery() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if err := recover(); err != nil {
				fields := []interface{}{
					"method", c.Request.Method,
					"path", c.Request.URL.Path,
					"client_ip", c.ClientIP(),
					"error", fmt.Sprintf("%v", err),
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

				logger.CriticalContext(c.Request.Context(), "panic recovered", fields...)

				response.Fail(c, response.ErrSystemError)
				c.Abort()
			}
		}()
		c.Next()
	}
}
