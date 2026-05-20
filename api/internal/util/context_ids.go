package util

import "github.com/gin-gonic/gin"

const (
	contextKeyOrganizationID = "organization_id"
	contextKeyWorkspaceID    = "workspace_id"
)

// GetOrganizationID returns the canonical organization scope from context.
func GetOrganizationID(c *gin.Context) string {
	return c.GetString(contextKeyOrganizationID)
}

// GetOrganizationIDCompat returns the canonical organization scope and falls back
// to legacy tenant_id when older handlers have not been migrated yet.
func GetOrganizationIDCompat(c *gin.Context) string {
	if organizationID := GetOrganizationID(c); organizationID != "" {
		return organizationID
	}

	return c.GetString("tenant_id")
}

// GetWorkspaceID returns the canonical workspace scope from context.
func GetWorkspaceID(c *gin.Context) string {
	return c.GetString(contextKeyWorkspaceID)
}

// SetOrganizationID stores the canonical organization scope in context.
func SetOrganizationID(c *gin.Context, organizationID string) {
	c.Set(contextKeyOrganizationID, organizationID)
}

// SetWorkspaceID stores the canonical workspace scope in context.
func SetWorkspaceID(c *gin.Context, workspaceID string) {
	c.Set(contextKeyWorkspaceID, workspaceID)
}

// SetOrganizationScopeCompat stores canonical organization scope and legacy tenant compatibility.
func SetOrganizationScopeCompat(c *gin.Context, organizationID string) {
	SetOrganizationID(c, organizationID)
	c.Set("tenant_id", organizationID)
}

// SetWorkspaceScopeCompat stores canonical workspace scope and legacy tenant compatibility.
func SetWorkspaceScopeCompat(c *gin.Context, workspaceID string) {
	SetWorkspaceID(c, workspaceID)
	c.Set("tenant_id", workspaceID)
}
