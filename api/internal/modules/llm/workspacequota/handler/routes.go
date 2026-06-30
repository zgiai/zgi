package handler

import "github.com/gin-gonic/gin"

// RegisterWorkspaceQuotaReadRoutes registers workspace-scoped quota read routes.
func RegisterWorkspaceQuotaReadRoutes(rg *gin.RouterGroup, h *WorkspaceQuotaHandler) {
	workspaceQuotas := rg.Group("/workspace-quotas")
	{
		workspaceQuotas.GET("/:workspace_id", h.GetWorkspaceQuota)
	}
}

// RegisterWorkspaceQuotaAdminRoutes registers organization-admin quota management routes.
func RegisterWorkspaceQuotaAdminRoutes(rg *gin.RouterGroup, h *WorkspaceQuotaHandler) {
	workspaceQuotas := rg.Group("/workspace-quotas")
	{
		workspaceQuotas.GET("", h.ListWorkspaceQuotas)
		workspaceQuotas.PUT("/:workspace_id", h.UpdateWorkspaceQuota)
	}
}

// RegisterWorkspaceQuotaRoutes registers all workspace quota routes on the same group.
func RegisterWorkspaceQuotaRoutes(rg *gin.RouterGroup, h *WorkspaceQuotaHandler) {
	RegisterWorkspaceQuotaReadRoutes(rg, h)
	RegisterWorkspaceQuotaAdminRoutes(rg, h)
}
