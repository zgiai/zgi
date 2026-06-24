package handler

import (
	"context"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	systemmodel "github.com/zgiai/zgi/api/internal/modules/system/model"
	"github.com/zgiai/zgi/api/internal/modules/system/service"
	authmodel "github.com/zgiai/zgi/api/internal/modules/user/auth/model"
	workspacemodel "github.com/zgiai/zgi/api/internal/modules/workspace/model"
	"github.com/zgiai/zgi/api/internal/util"
	"github.com/zgiai/zgi/api/pkg/response"
)

type accountContextReader interface {
	GetAccountContext(ctx context.Context, accountID string) (*authmodel.AccountContext, error)
}

type organizationAccessChecker interface {
	CheckWorkspacePermission(ctx context.Context, organizationID, workspaceID, accountID string, permissionCode workspacemodel.WorkspacePermissionCode) (bool, error)
	IsOrganizationAdminOrOwner(ctx context.Context, organizationID, accountID string) (bool, error)
	ListWorkspaceIDsByPermission(ctx context.Context, organizationID, accountID string, permissionCode workspacemodel.WorkspacePermissionCode) ([]string, error)
}

// DashboardHandler handles dashboard related requests
type DashboardHandler struct {
	dashboardService  service.DashboardService
	enterpriseService organizationAccessChecker
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

	accountID := util.GetAccountID(c)
	scopes, err := h.buildDashboardWorkspaceScopes(ctx, organizationID, accountID)
	if err != nil {
		response.Fail(c, response.ErrSystemError)
		return
	}

	stats, err := h.dashboardService.GetDashboardStats(ctx, organizationID, accountID, scopes)
	if err != nil {
		response.Fail(c, response.ErrSystemError)
		return
	}

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
	limit := dashboardQueryLimit(c)
	scope := strings.TrimSpace(c.DefaultQuery("scope", "overview"))
	if scope == "workspace" {
		recentWork, ok := h.getWorkspaceRecentWork(c, organizationID, accountID, limit)
		if !ok {
			return
		}
		response.Success(c, recentWork)
		return
	}

	scopes, err := h.buildDashboardWorkspaceScopes(ctx, organizationID, accountID)
	if err != nil {
		response.Fail(c, response.ErrSystemError)
		return
	}
	recentWork, err := h.dashboardService.GetRecentWork(ctx, systemmodel.RecentWorkRequest{
		OrganizationID:         organizationID,
		AccountID:              accountID,
		Limit:                  limit,
		WorkspaceIDs:           scopes.WorkspaceIDs,
		AgentWorkspaceIDs:      scopes.AgentWorkspaceIDs,
		DatasetWorkspaceIDs:    scopes.DatasetWorkspaceIDs,
		DataSourceWorkspaceIDs: scopes.DataSourceWorkspaceIDs,
		FileWorkspaceIDs:       scopes.FileWorkspaceIDs,
	})
	if err != nil {
		response.Fail(c, response.ErrSystemError)
		return
	}

	response.Success(c, recentWork)
}

func (h *DashboardHandler) getWorkspaceRecentWork(c *gin.Context, organizationID string, accountID string, limit int) (*systemmodel.RecentWorkResponse, bool) {
	ctx := c.Request.Context()
	workspaceID := strings.TrimSpace(c.Query("workspace_id"))
	if workspaceID == "" {
		if h.accountService == nil {
			response.Fail(c, response.ErrWorkspaceJoinedNotFound)
			return nil, false
		}
		accountContext, err := h.accountService.GetAccountContext(ctx, accountID)
		if err != nil || accountContext == nil || accountContext.CurrentWorkspaceID == nil {
			response.Fail(c, response.ErrWorkspaceJoinedNotFound)
			return nil, false
		}
		workspaceID = strings.TrimSpace(*accountContext.CurrentWorkspaceID)
	}
	if workspaceID == "" {
		response.Fail(c, response.ErrWorkspaceJoinedNotFound)
		return nil, false
	}

	scopes, err := h.buildSingleWorkspaceRecentWorkScopes(ctx, organizationID, workspaceID, accountID)
	if err != nil {
		response.Fail(c, response.ErrSystemError)
		return nil, false
	}

	recentWork, err := h.dashboardService.GetRecentWork(ctx, systemmodel.RecentWorkRequest{
		OrganizationID:         organizationID,
		AccountID:              accountID,
		Limit:                  limit,
		WorkspaceIDs:           scopes.WorkspaceIDs,
		AgentWorkspaceIDs:      scopes.AgentWorkspaceIDs,
		DatasetWorkspaceIDs:    scopes.DatasetWorkspaceIDs,
		DataSourceWorkspaceIDs: scopes.DataSourceWorkspaceIDs,
		FileWorkspaceIDs:       scopes.FileWorkspaceIDs,
	})
	if err != nil {
		response.Fail(c, response.ErrSystemError)
		return nil, false
	}
	return recentWork, true
}

func (h *DashboardHandler) buildDashboardWorkspaceScopes(ctx context.Context, organizationID string, accountID string) (systemmodel.DashboardWorkspaceScopes, error) {
	if h.enterpriseService == nil || organizationID == "" || accountID == "" {
		return systemmodel.DashboardWorkspaceScopes{}, nil
	}

	workspaceIDs, err := h.enterpriseService.ListWorkspaceIDsByPermission(ctx, organizationID, accountID, workspacemodel.WorkspacePermissionWorkspaceView)
	if err != nil {
		return systemmodel.DashboardWorkspaceScopes{}, err
	}
	agentWorkspaceIDs, err := h.enterpriseService.ListWorkspaceIDsByPermission(ctx, organizationID, accountID, workspacemodel.WorkspacePermissionAgentView)
	if err != nil {
		return systemmodel.DashboardWorkspaceScopes{}, err
	}
	datasetWorkspaceIDs, err := h.listWorkspaceIDsByAnyPermission(
		ctx,
		organizationID,
		accountID,
		workspacemodel.WorkspacePermissionKnowledgeBaseView,
		workspacemodel.WorkspacePermissionKnowledgeBaseManage,
		workspacemodel.WorkspacePermissionKnowledgeBaseFolderManage,
	)
	if err != nil {
		return systemmodel.DashboardWorkspaceScopes{}, err
	}
	dataSourceWorkspaceIDs, err := h.enterpriseService.ListWorkspaceIDsByPermission(ctx, organizationID, accountID, workspacemodel.WorkspacePermissionDatabaseView)
	if err != nil {
		return systemmodel.DashboardWorkspaceScopes{}, err
	}
	fileWorkspaceIDs, err := h.enterpriseService.ListWorkspaceIDsByPermission(ctx, organizationID, accountID, workspacemodel.WorkspacePermissionFileView)
	if err != nil {
		return systemmodel.DashboardWorkspaceScopes{}, err
	}

	return systemmodel.DashboardWorkspaceScopes{
		WorkspaceIDs:           workspaceIDs,
		AgentWorkspaceIDs:      agentWorkspaceIDs,
		DatasetWorkspaceIDs:    datasetWorkspaceIDs,
		DataSourceWorkspaceIDs: dataSourceWorkspaceIDs,
		FileWorkspaceIDs:       fileWorkspaceIDs,
	}, nil
}

func (h *DashboardHandler) buildSingleWorkspaceRecentWorkScopes(ctx context.Context, organizationID, workspaceID, accountID string) (systemmodel.DashboardWorkspaceScopes, error) {
	var scopes systemmodel.DashboardWorkspaceScopes
	if h.enterpriseService == nil || organizationID == "" || workspaceID == "" || accountID == "" {
		return scopes, nil
	}

	if ok, err := h.enterpriseService.CheckWorkspacePermission(ctx, organizationID, workspaceID, accountID, workspacemodel.WorkspacePermissionWorkspaceView); err != nil {
		return scopes, err
	} else if ok {
		scopes.WorkspaceIDs = []string{workspaceID}
	}
	if ok, err := h.enterpriseService.CheckWorkspacePermission(ctx, organizationID, workspaceID, accountID, workspacemodel.WorkspacePermissionAgentView); err != nil {
		return scopes, err
	} else if ok {
		scopes.AgentWorkspaceIDs = []string{workspaceID}
	}
	if ok, err := h.hasAnyWorkspacePermission(ctx, organizationID, workspaceID, accountID,
		workspacemodel.WorkspacePermissionKnowledgeBaseView,
		workspacemodel.WorkspacePermissionKnowledgeBaseManage,
		workspacemodel.WorkspacePermissionKnowledgeBaseFolderManage,
	); err != nil {
		return scopes, err
	} else if ok {
		scopes.DatasetWorkspaceIDs = []string{workspaceID}
	}
	if ok, err := h.enterpriseService.CheckWorkspacePermission(ctx, organizationID, workspaceID, accountID, workspacemodel.WorkspacePermissionDatabaseView); err != nil {
		return scopes, err
	} else if ok {
		scopes.DataSourceWorkspaceIDs = []string{workspaceID}
	}
	if ok, err := h.enterpriseService.CheckWorkspacePermission(ctx, organizationID, workspaceID, accountID, workspacemodel.WorkspacePermissionFileView); err != nil {
		return scopes, err
	} else if ok {
		scopes.FileWorkspaceIDs = []string{workspaceID}
	}

	return scopes, nil
}

func (h *DashboardHandler) hasAnyWorkspacePermission(ctx context.Context, organizationID, workspaceID, accountID string, permissionCodes ...workspacemodel.WorkspacePermissionCode) (bool, error) {
	for _, permissionCode := range permissionCodes {
		ok, err := h.enterpriseService.CheckWorkspacePermission(ctx, organizationID, workspaceID, accountID, permissionCode)
		if err != nil {
			return false, err
		}
		if ok {
			return true, nil
		}
	}
	return false, nil
}

func (h *DashboardHandler) listWorkspaceIDsByAnyPermission(ctx context.Context, organizationID, accountID string, permissionCodes ...workspacemodel.WorkspacePermissionCode) ([]string, error) {
	var out []string
	seen := make(map[string]struct{})
	for _, permissionCode := range permissionCodes {
		workspaceIDs, err := h.enterpriseService.ListWorkspaceIDsByPermission(ctx, organizationID, accountID, permissionCode)
		if err != nil {
			return nil, err
		}
		for _, workspaceID := range workspaceIDs {
			workspaceID = strings.TrimSpace(workspaceID)
			if workspaceID == "" {
				continue
			}
			if _, ok := seen[workspaceID]; ok {
				continue
			}
			seen[workspaceID] = struct{}{}
			out = append(out, workspaceID)
		}
	}
	return out, nil
}

func dashboardQueryLimit(c *gin.Context) int {
	limit, err := strconv.Atoi(strings.TrimSpace(c.DefaultQuery("limit", "10")))
	if err != nil || limit <= 0 || limit > 20 {
		return 10
	}
	return limit
}
