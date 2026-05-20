package util

import (
	"context"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/zgiai/ginext/pkg/jwt"
	"github.com/zgiai/ginext/pkg/logger"
)

// GetTenantID is a legacy compatibility helper.
// New permission code should prefer canonical organization_id/workspace_id helpers.
func GetTenantID(c *gin.Context) string {
	if tenantID, exists := c.Get("tenant_id"); exists {
		if tenantIDStr, ok := tenantID.(string); ok {
			logger.Debug("util.GetTenantID: found in context: %s", tenantIDStr)
			return tenantIDStr
		}
		logger.Warn("util.GetTenantID: tenant_id in context is not a string")
	}

	// Try to read tenant_id from JWT claims if provided
	authHeader := c.GetHeader("Authorization")
	if authHeader != "" {
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) == 2 && parts[0] == "Bearer" {
			if claims, err := jwt.ParseTokenFixed(parts[1]); err == nil {
				if tid, ok := claims["tenant_id"].(string); ok && tid != "" {
					logger.Debug("util.GetTenantID: found in JWT claims: %s", tid)
					return tid
				}
				logger.Debug("util.GetTenantID: JWT has no tenant_id claim")
			}
		}
	}

	if isCurrentOrDefaultWorkspace(c.Request.URL.Path) {
		logger.Debug("util.GetTenantID: using account current tenant for special path")
		return getCurrentUserTenantIDInternal(c)
	}

	logger.Debug("util.GetTenantID: fallback to account current tenant")
	return getCurrentUserTenantIDInternal(c)
}

func GetAccountID(c *gin.Context) string {
	if accountID, exists := c.Get("account_id"); exists {
		if id, ok := accountID.(string); ok {
			return id
		}
	}
	return ""
}

// isCurrentOrDefaultWorkspace checks if the path is for current or default workspace
func isCurrentOrDefaultWorkspace(path string) bool {
	return strings.Contains(path, "/workspaces/current/") ||
		strings.Contains(path, "/workspaces/default/")
}

// getCurrentUserTenantIDInternal gets current user's tenant ID from account service
func getCurrentUserTenantIDInternal(c *gin.Context) string {
	accountID, exists := c.Get("account_id")
	if !exists {
		logger.Error("account_id not found in context", nil)
		return ""
	}

	accountService, exists := c.Get("account_service")
	if !exists {
		logger.Info("AccountService not found in context, skipping tenant resolution", nil)
		return ""
	}

	// Match AccountServiceAdapter signature
	service, ok := accountService.(interface {
		EnsureCurrentOrganizationID(ctx context.Context, accountID string) (string, error)
	})
	if !ok {
		logger.Error("AccountService is not the correct type", nil)
		return ""
	}

	tenantID, err := service.EnsureCurrentOrganizationID(c.Request.Context(), accountID.(string))
	if err != nil {
		logger.Error("Failed to get current user tenant ID: %v", err)
		return ""
	}

	return tenantID
}
