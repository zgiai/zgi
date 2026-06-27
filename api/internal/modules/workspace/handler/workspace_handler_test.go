package handler

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	"github.com/zgiai/zgi/api/internal/modules/workspace/model"
	workspace_service "github.com/zgiai/zgi/api/internal/modules/workspace/service"
	"github.com/zgiai/zgi/api/pkg/response"
)

func TestWorkspaceStatisticsUsesRouteWorkspaceOrganizationForPermission(t *testing.T) {
	gin.SetMode(gin.TestMode)

	workspaceID := "workspace-route"
	routeOrganizationID := "org-from-workspace"
	accountID := "account-1"
	organizationSvc := &workspaceHandlerOrganizationService{
		workspaceOrganizationID:         routeOrganizationID,
		checkWorkspacePermissionAllowed: false,
	}
	workspaceSvc := &workspaceHandlerWorkspaceService{}
	handler := NewWorkspaceHandler(workspaceSvc, &workspaceHandlerAccountService{}, organizationSvc)

	c, recorder := newWorkspaceHandlerContext(http.MethodGet, "/workspaces/"+workspaceID+"/statistics", accountID)
	c.Params = gin.Params{{Key: "workspace_id", Value: workspaceID}}

	handler.GetWorkspaceStatistics(c)

	require.Equal(t, http.StatusForbidden, recorder.Code, recorder.Body.String())
	requireWorkspaceHandlerResponseCode(t, recorder, response.ErrPermissionDenied)
	require.True(t, organizationSvc.getOrganizationByWorkspaceIDCalled)
	require.Equal(t, workspaceID, organizationSvc.lastWorkspaceIDForOrganization)
	require.True(t, organizationSvc.checkWorkspacePermissionCalled)
	require.Equal(t, routeOrganizationID, organizationSvc.lastPermissionOrganizationID)
	require.Equal(t, workspaceID, organizationSvc.lastPermissionWorkspaceID)
	require.Equal(t, accountID, organizationSvc.lastPermissionAccountID)
	require.Equal(t, model.WorkspacePermissionWorkspaceView, organizationSvc.lastPermissionCode)
	require.False(t, workspaceSvc.statisticsCalled)
}

func TestUpdateWorkspaceRequiresWorkspaceManagePermission(t *testing.T) {
	gin.SetMode(gin.TestMode)

	workspaceID := "workspace-route"
	routeOrganizationID := "org-from-workspace"
	accountID := "account-1"
	organizationSvc := &workspaceHandlerOrganizationService{
		workspaceOrganizationID:         routeOrganizationID,
		checkWorkspacePermissionAllowed: false,
	}
	accountSvc := &workspaceHandlerAccountService{organizationAdmin: true}
	workspaceSvc := &workspaceHandlerWorkspaceService{}
	handler := NewWorkspaceHandler(workspaceSvc, accountSvc, organizationSvc)

	c, recorder := newWorkspaceHandlerContext(http.MethodPut, "/workspaces/"+workspaceID+"/update", accountID)
	c.Params = gin.Params{{Key: "workspace_id", Value: workspaceID}}
	c.Request.Body = requestBody(`{"name":"Renamed workspace"}`)
	c.Request.Header.Set("Content-Type", "application/json")

	handler.UpdateWorkspace(c)

	require.Equal(t, http.StatusForbidden, recorder.Code, recorder.Body.String())
	requireWorkspaceHandlerResponseCode(t, recorder, response.ErrPermissionDenied)
	require.True(t, organizationSvc.getOrganizationByWorkspaceIDCalled)
	require.Equal(t, workspaceID, organizationSvc.lastWorkspaceIDForOrganization)
	require.True(t, organizationSvc.checkWorkspacePermissionCalled)
	require.Equal(t, routeOrganizationID, organizationSvc.lastPermissionOrganizationID)
	require.Equal(t, workspaceID, organizationSvc.lastPermissionWorkspaceID)
	require.Equal(t, accountID, organizationSvc.lastPermissionAccountID)
	require.Equal(t, model.WorkspacePermissionWorkspaceManage, organizationSvc.lastPermissionCode)
	require.False(t, accountSvc.isOrganizationAdminOrOwnerCalled)
	require.False(t, workspaceSvc.updateCalled)
}

func newWorkspaceHandlerContext(method, target, accountID string) (*gin.Context, *httptest.ResponseRecorder) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(method, target, nil)
	c.Set("account_id", accountID)
	return c, recorder
}

func requestBody(body string) io.ReadCloser {
	return io.NopCloser(strings.NewReader(body))
}

type workspaceHandlerWorkspaceService struct {
	workspace_service.WorkspaceService

	updateCalled                     bool
	statisticsCalled                 bool
	lastUpdateWorkspaceID            string
	lastUpdateName                   string
	lastUpdateHasWorkspacePermission bool
}

func (s *workspaceHandlerWorkspaceService) CreateWorkspace(context.Context, string, string) error {
	return nil
}

func (s *workspaceHandlerWorkspaceService) UpdateWorkspace(_ context.Context, workspaceID, name string, status *model.WorkspaceStatus, accountID string, hasWorkspacePermission bool) (*model.WorkspaceUpdateResponse, error) {
	s.updateCalled = true
	s.lastUpdateWorkspaceID = workspaceID
	s.lastUpdateName = name
	s.lastUpdateHasWorkspacePermission = hasWorkspacePermission
	return &model.WorkspaceUpdateResponse{
		Result: "success",
		Tenant: struct {
			ID     string `json:"id"`
			Name   string `json:"name"`
			Status string `json:"status"`
		}{
			ID:     workspaceID,
			Name:   name,
			Status: string(model.WorkspaceStatusNormal),
		},
	}, nil
}

func (s *workspaceHandlerWorkspaceService) GetWorkspaceStatistics(context.Context, string) (*model.WorkspaceInfo, error) {
	s.statisticsCalled = true
	return &model.WorkspaceInfo{}, nil
}

type workspaceHandlerOrganizationService struct {
	interfaces.OrganizationService

	workspaceOrganizationID            string
	checkWorkspacePermissionAllowed    bool
	getOrganizationByWorkspaceIDCalled bool
	checkWorkspacePermissionCalled     bool
	lastWorkspaceIDForOrganization     string
	lastPermissionOrganizationID       string
	lastPermissionWorkspaceID          string
	lastPermissionAccountID            string
	lastPermissionCode                 model.WorkspacePermissionCode
}

func (s *workspaceHandlerOrganizationService) GetOrganizationByWorkspaceID(_ context.Context, workspaceID string) (*model.Organization, error) {
	s.getOrganizationByWorkspaceIDCalled = true
	s.lastWorkspaceIDForOrganization = workspaceID
	if s.workspaceOrganizationID == "" {
		return nil, nil
	}
	return &model.Organization{ID: s.workspaceOrganizationID, Status: model.OrganizationStatusActive}, nil
}

func (s *workspaceHandlerOrganizationService) CheckWorkspacePermission(_ context.Context, organizationID, workspaceID, accountID string, permissionCode model.WorkspacePermissionCode) (bool, error) {
	s.checkWorkspacePermissionCalled = true
	s.lastPermissionOrganizationID = organizationID
	s.lastPermissionWorkspaceID = workspaceID
	s.lastPermissionAccountID = accountID
	s.lastPermissionCode = permissionCode
	return s.checkWorkspacePermissionAllowed, nil
}

type workspaceHandlerAccountService struct {
	interfaces.AccountService

	organizationAdmin                bool
	isOrganizationAdminOrOwnerCalled bool
	lastOrganizationID               string
	lastAccountID                    string
}

func (s *workspaceHandlerAccountService) IsOrganizationAdminOrOwner(_ context.Context, organizationID, accountID string) (bool, error) {
	s.isOrganizationAdminOrOwnerCalled = true
	s.lastOrganizationID = organizationID
	s.lastAccountID = accountID
	return s.organizationAdmin, nil
}

func requireWorkspaceHandlerResponseCode(t *testing.T, recorder *httptest.ResponseRecorder, err response.ErrorCode) {
	t.Helper()

	var body struct {
		Code string `json:"code"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body), recorder.Body.String())
	require.Equal(t, strconv.Itoa(err.Code), body.Code, recorder.Body.String())
}

var _ workspace_service.WorkspaceService = (*workspaceHandlerWorkspaceService)(nil)
var _ interfaces.OrganizationService = (*workspaceHandlerOrganizationService)(nil)
var _ interfaces.AccountService = (*workspaceHandlerAccountService)(nil)
