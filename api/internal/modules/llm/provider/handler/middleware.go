package handler

import (
	"github.com/gin-gonic/gin"
	"github.com/zgiai/zgi/api/pkg/response"
	"gorm.io/gorm"
)

// ExtractOrganizationID extracts organization_id from workspace and sets it as tenant_id
// Note: In our system, tenant_id = organization_id (1:1 relationship)
func ExtractOrganizationID(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 1. Try to get organization_id directly from context (if already set by JWT)
		orgID := c.GetString("organization_id")
		if orgID == "" {
			// JWTWithOrganizationAndService sets tenant_id (legacy key) for console requests.
			// Reuse it as organization_id to keep handlers consistent.
			orgID = c.GetString("tenant_id")
			if orgID != "" {
				c.Set("organization_id", orgID)
				c.Next()
				return
			}
		}
		if orgID != "" {
			c.Next()
			return
		}

		// 2. Direct organization header support (bypass workspace lookup)
		headerOrgID := c.GetHeader("X-Organization-ID")
		if headerOrgID == "" {
			headerOrgID = c.GetHeader("X-Tenant-ID")
		}
		if headerOrgID != "" {
			c.Set("organization_id", headerOrgID)
			c.Set("tenant_id", headerOrgID)
			c.Next()
			return
		}

		// 3. Get workspace_id from context or header
		workspaceID := c.GetString("workspace_id")
		if workspaceID == "" {
			workspaceID = c.GetHeader("X-Workspace-ID")
		}

		if workspaceID == "" {
			response.FailWithMessage(c, response.ErrInvalidParam, "workspace_id or organization_id is required")
			c.Abort()
			return
		}

		// 4. Query workspace to get organization_id
		var workspace struct {
			OrganizationID *string `gorm:"column:organization_id"`
		}

		err := db.Table("workspaces").
			Select("organization_id").
			Where("id = ?", workspaceID).
			First(&workspace).Error

		if err != nil {
			response.FailWithMessage(c, response.ErrInvalidParam, "workspace not found")
			c.Abort()
			return
		}

		if workspace.OrganizationID == nil {
			response.FailWithMessage(c, response.ErrInvalidParam, "workspace does not belong to any organization")
			c.Abort()
			return
		}

		// 5. Set organization_id and workspace_id
		c.Set("organization_id", *workspace.OrganizationID)
		c.Set("tenant_id", *workspace.OrganizationID)
		c.Set("workspace_id", workspaceID)

		c.Next()
	}
}
