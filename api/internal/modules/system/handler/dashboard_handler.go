package handler

import (
	"context"
	"strings"

	"github.com/gin-gonic/gin"

	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	"github.com/zgiai/zgi/api/internal/modules/system/service"
	authmodel "github.com/zgiai/zgi/api/internal/modules/user/auth/model"
	workspacemodel "github.com/zgiai/zgi/api/internal/modules/workspace/model"
	"github.com/zgiai/zgi/api/internal/util"
	"github.com/zgiai/zgi/api/pkg/response"
)

type accountContextReader interface {
	GetAccountContext(ctx context.Context, accountID string) (*authmodel.AccountContext, error)
}

type workspacePermissionChecker interface {
	CheckWorkspacePermission(ctx context.Context, organizationID, workspaceID, accountID string, permissionCode workspacemodel.WorkspacePermissionCode) (bool, error)
}

// DashboardHandler handles dashboard related requests
type DashboardHandler struct {
	dashboardService  service.DashboardService
	enterpriseService workspacePermissionChecker
	accountService    accountContextReader
}

// NewDashboardHandler creates a new DashboardHandler instance
func NewDashboardHandler(dashboardService service.DashboardService, enterpriseService interfaces.OrganizationService, accountService interfaces.AccountService) *DashboardHandler {
	return &DashboardHandler{
		dashboardService:  dashboardService,
		enterpriseService: enterpriseService,
		accountService:    accountService,
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
	accountContext, err := h.accountService.GetAccountContext(ctx, accountID)
	if err != nil || accountContext == nil || accountContext.CurrentWorkspaceID == nil {
		response.Fail(c, response.ErrWorkspaceJoinedNotFound)
		return
	}

	workspaceID := strings.TrimSpace(*accountContext.CurrentWorkspaceID)
	if workspaceID == "" {
		response.Fail(c, response.ErrWorkspaceJoinedNotFound)
		return
	}

	hasPermission, err := h.enterpriseService.CheckWorkspacePermission(
		ctx,
		organizationID,
		workspaceID,
		accountID,
		workspacemodel.WorkspacePermissionWorkspaceView,
	)
	if err != nil || !hasPermission {
		response.Fail(c, response.ErrPermissionDenied)
		return
	}

	recentWork, _ := h.dashboardService.GetRecentWork(ctx, organizationID, workspaceID, accountID, 10)

	response.Success(c, recentWork)
}
