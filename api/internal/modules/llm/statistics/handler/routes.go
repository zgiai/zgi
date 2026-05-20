package handler

import (
	"github.com/gin-gonic/gin"
)

// RegisterStatisticsRoutes registers statistics routes for tenant users
func RegisterStatisticsRoutes(rg *gin.RouterGroup, h *StatisticsHandler) {
	statistics := rg.Group("/statistics")
	{
		statistics.GET("/model-usage", h.GetModelUsage)
		statistics.GET("/workspace-quota", h.GetWorkspaceQuota)
	}
}
