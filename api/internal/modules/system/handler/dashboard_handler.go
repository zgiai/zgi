package handler

import (
	"github.com/gin-gonic/gin"

	interfaces "github.com/zgiai/ginext/internal/modules/shared/interface"
	"github.com/zgiai/ginext/internal/modules/system/service"
	"github.com/zgiai/ginext/internal/util"
	"github.com/zgiai/ginext/pkg/response"
)

// DashboardHandler handles dashboard related requests
type DashboardHandler struct {
	dashboardService  service.DashboardService
	enterpriseService interfaces.OrganizationService
}

// NewDashboardHandler creates a new DashboardHandler instance
func NewDashboardHandler(dashboardService service.DashboardService, enterpriseService interfaces.OrganizationService) *DashboardHandler {
	return &DashboardHandler{
		dashboardService:  dashboardService,
		enterpriseService: enterpriseService,
	}
}

// GetDashboardStats returns dashboard statistics for the current enterprise group
// @Summary Get dashboard statistics
// @Description Get system statistics including model counts, app count, dataset count, and datasource count
// @Tags Dashboard
// @Accept json
// @Produce json
// @Success 200 {object} response.Response{data=model.DashboardStatsResponse}
// @Failure 401 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /console/api/dashboard/stats [get]
func (h *DashboardHandler) GetDashboardStats(c *gin.Context) {
	ctx := c.Request.Context()

	organizationID := util.GetOrganizationID(c)
	if organizationID == "" {
		response.Fail(c, response.ErrWorkspaceNotFound)
		return
	}

	stats, _ := h.dashboardService.GetDashboardStats(ctx, organizationID)

	response.Success(c, stats)
}

// GetRecentWork returns recently updated console work items.
// @Summary Get recent console work
// @Description Get recently updated conversations, agents, datasets, and data sources
// @Tags Dashboard
// @Accept json
// @Produce json
// @Success 200 {object} response.Response{data=model.RecentWorkResponse}
// @Failure 401 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /console/api/dashboard/recent-work [get]
func (h *DashboardHandler) GetRecentWork(c *gin.Context) {
	ctx := c.Request.Context()

	organizationID := util.GetOrganizationID(c)
	if organizationID == "" {
		response.Fail(c, response.ErrWorkspaceNotFound)
		return
	}

	accountID := util.GetAccountID(c)
	recentWork, _ := h.dashboardService.GetRecentWork(ctx, organizationID, accountID, 10)

	response.Success(c, recentWork)
}
