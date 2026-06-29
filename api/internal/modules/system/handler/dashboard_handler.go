package handler

import (
	"context"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/zgiai/zgi/api/internal/dto"
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
	ListWorkspaceIDsByPermission(ctx context.Context, organizationID, accountID string, permissionCode workspacemodel.WorkspacePermissionCode) ([]string, error)
	GetUserWorkspacesInOrganization(ctx context.Context, organizationID, accountID string, page, limit int) (*dto.WorkspacePaginationResponse, error)
}

const dashboardWorkspaceScopePageLimit = 200

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
		WorkflowWorkspaceIDs:   scopes.WorkflowWorkspaceIDs,
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
		WorkflowWorkspaceIDs:   scopes.WorkflowWorkspaceIDs,
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

	workspaceIDs, err := h.listDashboardWorkspaceIDs(ctx, organizationID, accountID)
	if err != nil {
		return systemmodel.DashboardWorkspaceScopes{}, err
	}
	agentWorkspaceIDs, err := h.listWorkspaceIDsByAnyPermission(ctx, organizationID, accountID, dashboardAgentVisiblePermissionCodes()...)
	if err != nil {
		return systemmodel.DashboardWorkspaceScopes{}, err
	}
	workflowWorkspaceIDs, err := h.listWorkspaceIDsByAnyPermission(ctx, organizationID, accountID, dashboardWorkflowVisiblePermissionCodes()...)
	if err != nil {
		return systemmodel.DashboardWorkspaceScopes{}, err
	}
	datasetWorkspaceIDs, err := h.listWorkspaceIDsByAnyPermission(
		ctx,
		organizationID,
		accountID,
		dashboardKnowledgeBaseVisiblePermissionCodes()...,
	)
	if err != nil {
		return systemmodel.DashboardWorkspaceScopes{}, err
	}
	dataSourceWorkspaceIDs, err := h.listWorkspaceIDsByAnyPermission(ctx, organizationID, accountID, dashboardDatabaseVisiblePermissionCodes()...)
	if err != nil {
		return systemmodel.DashboardWorkspaceScopes{}, err
	}
	fileWorkspaceIDs, err := h.listWorkspaceIDsByAnyPermission(ctx, organizationID, accountID, dashboardFileVisiblePermissionCodes()...)
	if err != nil {
		return systemmodel.DashboardWorkspaceScopes{}, err
	}

	return systemmodel.DashboardWorkspaceScopes{
		WorkspaceIDs:           workspaceIDs,
		AgentWorkspaceIDs:      agentWorkspaceIDs,
		WorkflowWorkspaceIDs:   workflowWorkspaceIDs,
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

	workspaceIDs, err := h.listDashboardWorkspaceIDs(ctx, organizationID, accountID)
	if err != nil {
		return scopes, err
	}
	if containsWorkspaceID(workspaceIDs, workspaceID) {
		scopes.WorkspaceIDs = []string{workspaceID}
	}
	if ok, err := h.hasAnyWorkspacePermission(ctx, organizationID, workspaceID, accountID, dashboardAgentVisiblePermissionCodes()...); err != nil {
		return scopes, err
	} else if ok {
		scopes.AgentWorkspaceIDs = []string{workspaceID}
	}
	if ok, err := h.hasAnyWorkspacePermission(ctx, organizationID, workspaceID, accountID, dashboardWorkflowVisiblePermissionCodes()...); err != nil {
		return scopes, err
	} else if ok {
		scopes.WorkflowWorkspaceIDs = []string{workspaceID}
	}
	if ok, err := h.hasAnyWorkspacePermission(ctx, organizationID, workspaceID, accountID,
		dashboardKnowledgeBaseVisiblePermissionCodes()...,
	); err != nil {
		return scopes, err
	} else if ok {
		scopes.DatasetWorkspaceIDs = []string{workspaceID}
	}
	if ok, err := h.hasAnyWorkspacePermission(ctx, organizationID, workspaceID, accountID, dashboardDatabaseVisiblePermissionCodes()...); err != nil {
		return scopes, err
	} else if ok {
		scopes.DataSourceWorkspaceIDs = []string{workspaceID}
	}
	if ok, err := h.hasAnyWorkspacePermission(ctx, organizationID, workspaceID, accountID, dashboardFileVisiblePermissionCodes()...); err != nil {
		return scopes, err
	} else if ok {
		scopes.FileWorkspaceIDs = []string{workspaceID}
	}

	return scopes, nil
}

func (h *DashboardHandler) listDashboardWorkspaceIDs(ctx context.Context, organizationID, accountID string) ([]string, error) {
	seen := make(map[string]struct{})
	workspaceIDs := make([]string, 0)
	addWorkspaceID := func(workspaceID string) {
		workspaceID = strings.TrimSpace(workspaceID)
		if workspaceID == "" {
			return
		}
		if _, ok := seen[workspaceID]; ok {
			return
		}
		seen[workspaceID] = struct{}{}
		workspaceIDs = append(workspaceIDs, workspaceID)
	}

	for page := 1; ; page++ {
		resp, err := h.enterpriseService.GetUserWorkspacesInOrganization(ctx, organizationID, accountID, page, dashboardWorkspaceScopePageLimit)
		if err != nil {
			return nil, err
		}
		if resp == nil {
			break
		}
		for _, workspace := range resp.Data {
			if workspace != nil {
				addWorkspaceID(workspace.ID)
			}
		}
		if !resp.HasMore || len(resp.Data) == 0 {
			break
		}
	}

	legacyWorkspaceIDs, err := h.enterpriseService.ListWorkspaceIDsByPermission(ctx, organizationID, accountID, workspacemodel.WorkspacePermissionWorkspaceView)
	if err != nil {
		return nil, err
	}
	for _, workspaceID := range legacyWorkspaceIDs {
		addWorkspaceID(workspaceID)
	}

	return workspaceIDs, nil
}

func containsWorkspaceID(workspaceIDs []string, targetWorkspaceID string) bool {
	targetWorkspaceID = strings.TrimSpace(targetWorkspaceID)
	for _, workspaceID := range workspaceIDs {
		if strings.TrimSpace(workspaceID) == targetWorkspaceID {
			return true
		}
	}
	return false
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

func dashboardAgentVisiblePermissionCodes() []workspacemodel.WorkspacePermissionCode {
	return []workspacemodel.WorkspacePermissionCode{
		workspacemodel.WorkspacePermissionAgentCreate,
		workspacemodel.WorkspacePermissionAgentImport,
		workspacemodel.WorkspacePermissionAgentLogsView,
		workspacemodel.WorkspacePermissionAgentStatsView,
		workspacemodel.WorkspacePermissionAgentConversationView,
		workspacemodel.WorkspacePermissionAgentUpdate,
		workspacemodel.WorkspacePermissionAgentDelete,
		workspacemodel.WorkspacePermissionAgentMove,
		workspacemodel.WorkspacePermissionAgentCopy,
		workspacemodel.WorkspacePermissionAgentExport,
		workspacemodel.WorkspacePermissionAgentPublish,
		workspacemodel.WorkspacePermissionAgentRuntimeConfigManage,
		workspacemodel.WorkspacePermissionAgentRuntimeAccessManage,
		workspacemodel.WorkspacePermissionAgentConversationManage,
	}
}

func dashboardWorkflowVisiblePermissionCodes() []workspacemodel.WorkspacePermissionCode {
	return []workspacemodel.WorkspacePermissionCode{
		workspacemodel.WorkspacePermissionWorkflowView,
		workspacemodel.WorkspacePermissionWorkflowCreate,
		workspacemodel.WorkspacePermissionWorkflowImport,
		workspacemodel.WorkspacePermissionWorkflowLogsView,
		workspacemodel.WorkspacePermissionWorkflowStatsView,
		workspacemodel.WorkspacePermissionWorkflowEventsView,
		workspacemodel.WorkspacePermissionWorkflowUpdate,
		workspacemodel.WorkspacePermissionWorkflowDelete,
		workspacemodel.WorkspacePermissionWorkflowMove,
		workspacemodel.WorkspacePermissionWorkflowCopy,
		workspacemodel.WorkspacePermissionWorkflowExport,
		workspacemodel.WorkspacePermissionWorkflowRunDraft,
		workspacemodel.WorkspacePermissionWorkflowRunStop,
		workspacemodel.WorkspacePermissionWorkflowDebug,
		workspacemodel.WorkspacePermissionWorkflowPublish,
		workspacemodel.WorkspacePermissionWorkflowRuntimeConfigManage,
		workspacemodel.WorkspacePermissionWorkflowRuntimeAccessManage,
	}
}

func dashboardKnowledgeBaseVisiblePermissionCodes() []workspacemodel.WorkspacePermissionCode {
	return []workspacemodel.WorkspacePermissionCode{
		workspacemodel.WorkspacePermissionKnowledgeBaseCreate,
		workspacemodel.WorkspacePermissionKnowledgeBaseFolderView,
		workspacemodel.WorkspacePermissionKnowledgeBaseDocumentView,
		workspacemodel.WorkspacePermissionKnowledgeBaseSegmentView,
		workspacemodel.WorkspacePermissionKnowledgeBaseGraphView,
		workspacemodel.WorkspacePermissionKnowledgeBaseRetrievalTest,
		workspacemodel.WorkspacePermissionKnowledgeBaseUpdate,
		workspacemodel.WorkspacePermissionKnowledgeBaseDelete,
		workspacemodel.WorkspacePermissionKnowledgeBaseMove,
		workspacemodel.WorkspacePermissionKnowledgeBaseDocumentCreate,
		workspacemodel.WorkspacePermissionKnowledgeBaseDocumentUpdate,
		workspacemodel.WorkspacePermissionKnowledgeBaseDocumentDelete,
		workspacemodel.WorkspacePermissionKnowledgeBaseSegmentUpdate,
		workspacemodel.WorkspacePermissionKnowledgeBaseSegmentDelete,
		workspacemodel.WorkspacePermissionKnowledgeBaseIndexManage,
		workspacemodel.WorkspacePermissionKnowledgeBaseGraphManage,
		workspacemodel.WorkspacePermissionKnowledgeBaseFolderManage,
	}
}

func dashboardDatabaseVisiblePermissionCodes() []workspacemodel.WorkspacePermissionCode {
	return []workspacemodel.WorkspacePermissionCode{
		workspacemodel.WorkspacePermissionDatabaseCreate,
		workspacemodel.WorkspacePermissionDatabaseUpdate,
		workspacemodel.WorkspacePermissionDatabaseDelete,
		workspacemodel.WorkspacePermissionDatabaseMove,
		workspacemodel.WorkspacePermissionDatabaseSchemaView,
		workspacemodel.WorkspacePermissionDatabaseSchemaManage,
		workspacemodel.WorkspacePermissionDatabaseRecordView,
		workspacemodel.WorkspacePermissionDatabaseRecordCreate,
		workspacemodel.WorkspacePermissionDatabaseRecordUpdate,
		workspacemodel.WorkspacePermissionDatabaseRecordDelete,
		workspacemodel.WorkspacePermissionDatabaseImportAnalyze,
		workspacemodel.WorkspacePermissionDatabaseImportExecute,
		workspacemodel.WorkspacePermissionDatabaseImportErrorsView,
		workspacemodel.WorkspacePermissionDatabaseGuardPolicyManage,
		workspacemodel.WorkspacePermissionDatabaseTablePromptView,
		workspacemodel.WorkspacePermissionDatabaseTablePromptManage,
		workspacemodel.WorkspacePermissionDatabaseOperationLogsView,
		workspacemodel.WorkspacePermissionDatabaseSQLAuditView,
		workspacemodel.WorkspacePermissionDatabaseAIQueryRead,
		workspacemodel.WorkspacePermissionDatabaseAIQueryWrite,
	}
}

func dashboardFileVisiblePermissionCodes() []workspacemodel.WorkspacePermissionCode {
	return []workspacemodel.WorkspacePermissionCode{
		workspacemodel.WorkspacePermissionFileMetadataView,
		workspacemodel.WorkspacePermissionFilePreview,
		workspacemodel.WorkspacePermissionFileFolderView,
		workspacemodel.WorkspacePermissionFileRelatedView,
		workspacemodel.WorkspacePermissionFileUpload,
		workspacemodel.WorkspacePermissionFileTextCreate,
		workspacemodel.WorkspacePermissionFileUpdate,
		workspacemodel.WorkspacePermissionFileDelete,
		workspacemodel.WorkspacePermissionFileMove,
		workspacemodel.WorkspacePermissionFileArchive,
		workspacemodel.WorkspacePermissionFileFolderManage,
		workspacemodel.WorkspacePermissionFileShareManage,
		workspacemodel.WorkspacePermissionFileFavoriteManage,
		workspacemodel.WorkspacePermissionFileDownload,
	}
}

func dashboardQueryLimit(c *gin.Context) int {
	limit, err := strconv.Atoi(strings.TrimSpace(c.DefaultQuery("limit", "10")))
	if err != nil || limit <= 0 || limit > 20 {
		return 10
	}
	return limit
}
