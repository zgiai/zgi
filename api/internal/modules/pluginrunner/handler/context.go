package handler

import (
	"errors"

	"github.com/gin-gonic/gin"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
)

var errOrganizationNotFound = errors.New("organization_id not found in context")

func getAccountIDFromContext(c *gin.Context) string {
	if accountID, exists := c.Get("account_id"); exists {
		if id, ok := accountID.(string); ok {
			return id
		}
	}
	return ""
}

func getOrganizationIDFromContext(c *gin.Context, accountService interfaces.AccountService) (string, error) {
	if orgID, exists := c.Get("organization_id"); exists {
		if id, ok := orgID.(string); ok && id != "" {
			return id, nil
		}
	}

	if groupID, exists := c.Get("group_id"); exists {
		if id, ok := groupID.(string); ok && id != "" {
			c.Set("organization_id", id)
			return id, nil
		}
	}

	if tenantID, exists := c.Get("tenant_id"); exists {
		if id, ok := tenantID.(string); ok && id != "" {
			c.Set("organization_id", id)
			return id, nil
		}
	}

	if accountService == nil {
		return "", errOrganizationNotFound
	}

	accountID := getAccountIDFromContext(c)
	if accountID == "" {
		return "", errOrganizationNotFound
	}

	orgID, err := accountService.EnsureCurrentOrganizationID(c.Request.Context(), accountID)
	if err != nil || orgID == "" {
		return "", errOrganizationNotFound
	}

	c.Set("organization_id", orgID)
	c.Set("tenant_id", orgID)
	c.Set("group_id", orgID)
	return orgID, nil
}

func ensureOrganizationMember(c *gin.Context, accountService interfaces.AccountService, organizationID, accountID string) (bool, error) {
	if accountService == nil {
		return false, errors.New("account service is not initialized")
	}
	return accountService.IsOrganizationMember(c.Request.Context(), organizationID, accountID)
}
