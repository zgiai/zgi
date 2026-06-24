package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	systemmodel "github.com/zgiai/zgi/api/internal/modules/system/model"
	authmodel "github.com/zgiai/zgi/api/internal/modules/user/auth/model"
	workspacemodel "github.com/zgiai/zgi/api/internal/modules/workspace/model"
	"github.com/zgiai/zgi/api/internal/util"
	"github.com/zgiai/zgi/api/pkg/response"
)

func TestDashboardRecentWorkOverviewDoesNotRequireCurrentWorkspace(t *testing.T) {
	recorder, c := newDashboardRecentWorkContext("org-1", "acc-1", "")
	dashboardSvc := &dashboardHandlerService{
		recentWork: &systemmodel.RecentWorkResponse{
			Items: []systemmodel.RecentWorkItem{{
				ID:            "agent:agent-1",
				Type:          "agent",
				ResourceID:    "agent-1",
				Title:         "Agent One",
				WorkspaceID:   "ws-agent",
				WorkspaceName: "Agent Space",
				UpdatedAt:     1710000000,
			}},
		},
	}
	permissionSvc := &dashboardHandlerWorkspacePermissionService{
		workspaceIDsByPermission: map[workspacemodel.WorkspacePermissionCode][]string{
			workspacemodel.WorkspacePermissionWorkspaceView:             {"ws-1", "ws-2"},
			workspacemodel.WorkspacePermissionAgentView:                 {"ws-agent"},
			workspacemodel.WorkspacePermissionKnowledgeBaseView:         {"ws-knowledge-view", "ws-knowledge-shared"},
			workspacemodel.WorkspacePermissionKnowledgeBaseManage:       {"ws-knowledge-manage"},
			workspacemodel.WorkspacePermissionKnowledgeBaseFolderManage: {"ws-knowledge-shared", "ws-knowledge-folder"},
			workspacemodel.WorkspacePermissionDatabaseView:              {"ws-db"},
			workspacemodel.WorkspacePermissionFileView:                  {"ws-file"},
		},
	}
	accountSvc := &dashboardHandlerAccountContextService{
		accountContext: &authmodel.AccountContext{AccountID: "acc-1"},
	}
	h := &DashboardHandler{
		dashboardService:  dashboardSvc,
		enterpriseService: permissionSvc,
		accountService:    accountSvc,
	}

	h.GetRecentWork(c)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.False(t, permissionSvc.called, "overview recent-work should not require current workspace")
	require.True(t, dashboardSvc.recentWorkCalled)
	require.Equal(t, "org-1", dashboardSvc.recentWorkReq.OrganizationID)
	require.Equal(t, "acc-1", dashboardSvc.recentWorkReq.AccountID)
	require.Equal(t, []string{"ws-1", "ws-2"}, dashboardSvc.recentWorkReq.WorkspaceIDs)
	require.Equal(t, []string{"ws-agent"}, dashboardSvc.recentWorkReq.AgentWorkspaceIDs)
	require.Equal(t, []string{"ws-knowledge-view", "ws-knowledge-shared", "ws-knowledge-manage", "ws-knowledge-folder"}, dashboardSvc.recentWorkReq.DatasetWorkspaceIDs)
	require.Equal(t, []string{"ws-db"}, dashboardSvc.recentWorkReq.DataSourceWorkspaceIDs)
	require.Equal(t, []string{"ws-file"}, dashboardSvc.recentWorkReq.FileWorkspaceIDs)
}

func TestDashboardRecentWorkWorkspaceScopeUsesResourcePermissions(t *testing.T) {
	recorder, c := newDashboardRecentWorkContext("org-1", "acc-1", "?scope=workspace&workspace_id=ws-1")
	dashboardSvc := &dashboardHandlerService{}
	permissionSvc := &dashboardHandlerWorkspacePermissionService{
		allowedByPermission: map[workspacemodel.WorkspacePermissionCode]bool{
			workspacemodel.WorkspacePermissionWorkspaceView:             false,
			workspacemodel.WorkspacePermissionAgentView:                 true,
			workspacemodel.WorkspacePermissionKnowledgeBaseView:         false,
			workspacemodel.WorkspacePermissionKnowledgeBaseManage:       true,
			workspacemodel.WorkspacePermissionKnowledgeBaseFolderManage: false,
			workspacemodel.WorkspacePermissionDatabaseView:              false,
			workspacemodel.WorkspacePermissionFileView:                  true,
		},
	}
	h := &DashboardHandler{
		dashboardService:  dashboardSvc,
		enterpriseService: permissionSvc,
		accountService:    &dashboardHandlerAccountContextService{},
	}

	h.GetRecentWork(c)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.True(t, permissionSvc.called)
	require.Equal(t, "org-1", permissionSvc.organizationID)
	require.Equal(t, "ws-1", permissionSvc.workspaceID)
	require.Equal(t, "acc-1", permissionSvc.accountID)
	require.True(t, dashboardSvc.recentWorkCalled)
	require.Equal(t, []string(nil), dashboardSvc.recentWorkReq.WorkspaceIDs)
	require.Equal(t, []string{"ws-1"}, dashboardSvc.recentWorkReq.AgentWorkspaceIDs)
	require.Equal(t, []string{"ws-1"}, dashboardSvc.recentWorkReq.DatasetWorkspaceIDs)
	require.Equal(t, []string(nil), dashboardSvc.recentWorkReq.DataSourceWorkspaceIDs)
	require.Equal(t, []string{"ws-1"}, dashboardSvc.recentWorkReq.FileWorkspaceIDs)
}

func TestDashboardRecentWorkWorkspaceScopeFallsBackToCurrentWorkspace(t *testing.T) {
	recorder, c := newDashboardRecentWorkContext("org-1", "acc-1", "?scope=workspace")
	workspaceID := "ws-1"
	dashboardSvc := &dashboardHandlerService{}
	permissionSvc := &dashboardHandlerWorkspacePermissionService{allowed: true}
	accountSvc := &dashboardHandlerAccountContextService{
		accountContext: &authmodel.AccountContext{
			AccountID:          "acc-1",
			CurrentWorkspaceID: &workspaceID,
		},
	}
	h := &DashboardHandler{
		dashboardService:  dashboardSvc,
		enterpriseService: permissionSvc,
		accountService:    accountSvc,
	}

	h.GetRecentWork(c)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.True(t, dashboardSvc.recentWorkCalled)
	require.Equal(t, "org-1", dashboardSvc.recentWorkReq.OrganizationID)
	require.Equal(t, "acc-1", dashboardSvc.recentWorkReq.AccountID)
	require.Equal(t, []string{"ws-1"}, dashboardSvc.recentWorkReq.WorkspaceIDs)
	require.Equal(t, []string{"ws-1"}, dashboardSvc.recentWorkReq.AgentWorkspaceIDs)
	require.Equal(t, []string{"ws-1"}, dashboardSvc.recentWorkReq.DatasetWorkspaceIDs)
	require.Equal(t, []string{"ws-1"}, dashboardSvc.recentWorkReq.DataSourceWorkspaceIDs)
	require.Equal(t, []string{"ws-1"}, dashboardSvc.recentWorkReq.FileWorkspaceIDs)
	require.Equal(t, 10, dashboardSvc.recentWorkReq.Limit)
}

func TestDashboardStatsUsesVisibleWorkspaceScopes(t *testing.T) {
	recorder, c := newDashboardStatsContext("org-1", "acc-1")
	dashboardSvc := &dashboardHandlerService{
		stats: &systemmodel.DashboardStatsResponse{
			Models: systemmodel.ModelsStats{
				Total:     1,
				ByUseCase: map[string]int64{"text-chat": 1},
			},
			Resources: systemmodel.ResourceStats{
				Workspaces:  2,
				Agents:      9,
				Datasets:    8,
				DataSources: 7,
			},
		},
	}
	permissionSvc := &dashboardHandlerWorkspacePermissionService{
		workspaceIDsByPermission: map[workspacemodel.WorkspacePermissionCode][]string{
			workspacemodel.WorkspacePermissionWorkspaceView:             {"ws-1", "ws-2"},
			workspacemodel.WorkspacePermissionAgentView:                 {"ws-agent"},
			workspacemodel.WorkspacePermissionKnowledgeBaseView:         {"ws-knowledge"},
			workspacemodel.WorkspacePermissionKnowledgeBaseManage:       {"ws-knowledge-manage"},
			workspacemodel.WorkspacePermissionKnowledgeBaseFolderManage: {"ws-knowledge-folder"},
			workspacemodel.WorkspacePermissionDatabaseView:              {"ws-db"},
			workspacemodel.WorkspacePermissionFileView:                  {"ws-file"},
		},
	}
	h := &DashboardHandler{
		dashboardService:  dashboardSvc,
		enterpriseService: permissionSvc,
		accountService:    &dashboardHandlerAccountContextService{},
	}

	h.GetDashboardStats(c)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.True(t, dashboardSvc.statsCalled)
	require.Equal(t, "org-1", dashboardSvc.statsOrganizationID)
	require.Equal(t, "acc-1", dashboardSvc.statsAccountID)
	require.Equal(t, []string{"ws-1", "ws-2"}, dashboardSvc.statsScopes.WorkspaceIDs)
	require.Equal(t, []string{"ws-agent"}, dashboardSvc.statsScopes.AgentWorkspaceIDs)
	require.Equal(t, []string{"ws-knowledge", "ws-knowledge-manage", "ws-knowledge-folder"}, dashboardSvc.statsScopes.DatasetWorkspaceIDs)
	require.Equal(t, []string{"ws-db"}, dashboardSvc.statsScopes.DataSourceWorkspaceIDs)
	require.Equal(t, []string{"ws-file"}, dashboardSvc.statsScopes.FileWorkspaceIDs)

	var payload struct {
		Data systemmodel.DashboardStatsResponse `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &payload))
	require.Equal(t, int64(1), payload.Data.Models.ByUseCase["text-chat"])
	require.Equal(t, int64(2), payload.Data.Resources.Workspaces)
	require.Equal(t, int64(9), payload.Data.Resources.Agents)
	require.Equal(t, int64(8), payload.Data.Resources.Datasets)
	require.Equal(t, int64(7), payload.Data.Resources.DataSources)
}

func newDashboardStatsContext(organizationID string, accountID string) (*httptest.ResponseRecorder, *gin.Context) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/console/api/dashboard/stats", nil)
	util.SetOrganizationID(c, organizationID)
	c.Set("account_id", accountID)
	return recorder, c
}

func newDashboardRecentWorkContext(organizationID string, accountID string, query string) (*httptest.ResponseRecorder, *gin.Context) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/console/api/dashboard/recent-work"+query, nil)
	util.SetOrganizationID(c, organizationID)
	c.Set("account_id", accountID)
	return recorder, c
}

func decodeDashboardHandlerResponseCode(t *testing.T, recorder *httptest.ResponseRecorder) string {
	t.Helper()

	var body response.Response
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body))
	return body.Code
}

type dashboardHandlerService struct {
	stats      *systemmodel.DashboardStatsResponse
	recentWork *systemmodel.RecentWorkResponse

	statsCalled         bool
	statsOrganizationID string
	statsAccountID      string
	statsScopes         systemmodel.DashboardWorkspaceScopes

	recentWorkCalled bool
	recentWorkReq    systemmodel.RecentWorkRequest
}

func (s *dashboardHandlerService) GetDashboardStats(_ context.Context, organizationID string, accountID string, scopes systemmodel.DashboardWorkspaceScopes) (*systemmodel.DashboardStatsResponse, error) {
	s.statsCalled = true
	s.statsOrganizationID = organizationID
	s.statsAccountID = accountID
	s.statsScopes = scopes
	if s.stats != nil {
		return s.stats, nil
	}
	return &systemmodel.DashboardStatsResponse{}, nil
}

func (s *dashboardHandlerService) GetRecentWork(_ context.Context, req systemmodel.RecentWorkRequest) (*systemmodel.RecentWorkResponse, error) {
	s.recentWorkCalled = true
	s.recentWorkReq = req
	if s.recentWork != nil {
		return s.recentWork, nil
	}
	return &systemmodel.RecentWorkResponse{}, nil
}

type dashboardHandlerAccountContextService struct {
	accountContext *authmodel.AccountContext
}

func (s *dashboardHandlerAccountContextService) GetAccountContext(context.Context, string) (*authmodel.AccountContext, error) {
	return s.accountContext, nil
}

type dashboardHandlerWorkspacePermissionService struct {
	allowed                  bool
	allowedByPermission      map[workspacemodel.WorkspacePermissionCode]bool
	called                   bool
	organizationAdmin        bool
	adminCheckCalled         bool
	workspaceIDsByPermission map[workspacemodel.WorkspacePermissionCode][]string
	organizationID           string
	workspaceID              string
	accountID                string
	permissionCode           workspacemodel.WorkspacePermissionCode
}

func (s *dashboardHandlerWorkspacePermissionService) CheckWorkspacePermission(_ context.Context, organizationID string, workspaceID string, accountID string, permissionCode workspacemodel.WorkspacePermissionCode) (bool, error) {
	s.called = true
	s.organizationID = organizationID
	s.workspaceID = workspaceID
	s.accountID = accountID
	s.permissionCode = permissionCode
	if s.allowedByPermission != nil {
		return s.allowedByPermission[permissionCode], nil
	}
	return s.allowed, nil
}

func (s *dashboardHandlerWorkspacePermissionService) IsOrganizationAdminOrOwner(_ context.Context, organizationID, accountID string) (bool, error) {
	s.adminCheckCalled = true
	s.organizationID = organizationID
	s.accountID = accountID
	return s.organizationAdmin, nil
}

func (s *dashboardHandlerWorkspacePermissionService) ListWorkspaceIDsByPermission(_ context.Context, organizationID, accountID string, permissionCode workspacemodel.WorkspacePermissionCode) ([]string, error) {
	s.organizationID = organizationID
	s.accountID = accountID
	if s.workspaceIDsByPermission == nil {
		return []string{}, nil
	}
	return s.workspaceIDsByPermission[permissionCode], nil
}
