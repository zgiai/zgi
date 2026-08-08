package handler

import (
	"github.com/gin-gonic/gin"
	"github.com/zgiai/zgi/api/middleware"
)

// RegisterStatisticsRoutes registers statistics routes for tenant users
func RegisterStatisticsRoutes(rg *gin.RouterGroup, h *StatisticsHandler) {
	statistics := rg.Group("/statistics")
	{
		statistics.GET("/model-usage", h.GetModelUsage)
		statistics.GET("/invocations", h.GetInvocationLog)
		statistics.GET("/workspace-quota", h.GetWorkspaceQuota)
	}

	sensitive := statistics.Group("")
	sensitive.Use(middleware.EnterpriseAdminOrOwnerRequired())
	{
		sensitive.GET("/invocation-content/settings", h.GetInvocationContentSettings)
		sensitive.PUT("/invocation-content/settings", h.UpdateInvocationContentSettings)
		sensitive.GET("/invocations/:invocation_id/content", h.GetInvocationContent)
	}
}
