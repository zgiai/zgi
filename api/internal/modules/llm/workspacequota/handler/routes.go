package handler

import "github.com/gin-gonic/gin"

// RegisterWorkspaceQuotaRoutes registers workspace quota management routes.
func RegisterWorkspaceQuotaRoutes(rg *gin.RouterGroup, h *WorkspaceQuotaHandler) {
	workspaceQuotas := rg.Group("/workspace-quotas")
	{
		workspaceQuotas.GET("", h.ListWorkspaceQuotas)
		workspaceQuotas.GET("/:workspace_id", h.GetWorkspaceQuota)
		workspaceQuotas.PUT("/:workspace_id", h.UpdateWorkspaceQuota)
	}
}
