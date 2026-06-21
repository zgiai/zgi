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

func TestDashboardRecentWorkRequiresCurrentWorkspaceBeforeServiceCall(t *testing.T) {
	recorder, c := newDashboardRecentWorkContext("org-1", "acc-1")
	dashboardSvc := &dashboardHandlerService{}
	permissionSvc := &dashboardHandlerWorkspacePermissionService{allowed: true}
	accountSvc := &dashboardHandlerAccountContextService{
		accountContext: &authmodel.AccountContext{AccountID: "acc-1"},
	}
	h := &DashboardHandler{
		dashboardService:  dashboardSvc,
		enterpriseService: permissionSvc,
		accountService:    accountSvc,
	}

	h.GetRecentWork(c)

	require.Equal(t, http.StatusNotFound, recorder.Code)
	require.Equal(t, "205016", decodeDashboardHandlerResponseCode(t, recorder))
	require.False(t, permissionSvc.called, "workspace permission check should not run without a current workspace")
	require.False(t, dashboardSvc.recentWorkCalled, "recent-work service should not run without a current workspace")
}

func TestDashboardRecentWorkRequiresWorkspaceViewBeforeServiceCall(t *testing.T) {
	recorder, c := newDashboardRecentWorkContext("org-1", "acc-1")
	workspaceID := "ws-1"
	dashboardSvc := &dashboardHandlerService{}
	permissionSvc := &dashboardHandlerWorkspacePermissionService{allowed: false}
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

	require.Equal(t, http.StatusForbidden, recorder.Code)
	require.Equal(t, "403001", decodeDashboardHandlerResponseCode(t, recorder))
	require.True(t, permissionSvc.called)
	require.Equal(t, "org-1", permissionSvc.organizationID)
	require.Equal(t, workspaceID, permissionSvc.workspaceID)
	require.Equal(t, "acc-1", permissionSvc.accountID)
	require.Equal(t, workspacemodel.WorkspacePermissionWorkspaceView, permissionSvc.permissionCode)
	require.False(t, dashboardSvc.recentWorkCalled, "recent-work service should not run before workspace.view passes")
}

func TestDashboardRecentWorkUsesCurrentWorkspaceScope(t *testing.T) {
	recorder, c := newDashboardRecentWorkContext("org-1", "acc-1")
	workspaceID := "ws-1"
	dashboardSvc := &dashboardHandlerService{
		recentWork: &systemmodel.RecentWorkResponse{
			Items: []systemmodel.RecentWorkItem{
				{ID: "agent:agent-1", Type: "agent", ResourceID: "agent-1", Title: "Agent One", UpdatedAt: 1710000000},
			},
		},
	}
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
	require.Equal(t, "org-1", dashboardSvc.organizationID)
	require.Equal(t, workspaceID, dashboardSvc.workspaceID)
	require.Equal(t, "acc-1", dashboardSvc.accountID)
	require.Equal(t, 10, dashboardSvc.limit)
}

func newDashboardRecentWorkContext(organizationID string, accountID string) (*httptest.ResponseRecorder, *gin.Context) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/console/api/dashboard/recent-work", nil)
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
	recentWork       *systemmodel.RecentWorkResponse
	recentWorkCalled bool
	organizationID   string
	workspaceID      string
	accountID        string
	limit            int
}

func (s *dashboardHandlerService) GetDashboardStats(context.Context, string) (*systemmodel.DashboardStatsResponse, error) {
	return &systemmodel.DashboardStatsResponse{}, nil
}

func (s *dashboardHandlerService) GetRecentWork(_ context.Context, organizationID string, workspaceID string, accountID string, limit int) (*systemmodel.RecentWorkResponse, error) {
	s.recentWorkCalled = true
	s.organizationID = organizationID
	s.workspaceID = workspaceID
	s.accountID = accountID
	s.limit = limit
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
	allowed        bool
	called         bool
	organizationID string
	workspaceID    string
	accountID      string
	permissionCode workspacemodel.WorkspacePermissionCode
}

func (s *dashboardHandlerWorkspacePermissionService) CheckWorkspacePermission(_ context.Context, organizationID string, workspaceID string, accountID string, permissionCode workspacemodel.WorkspacePermissionCode) (bool, error) {
	s.called = true
	s.organizationID = organizationID
	s.workspaceID = workspaceID
	s.accountID = accountID
	s.permissionCode = permissionCode
	return s.allowed, nil
}
