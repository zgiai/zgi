package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	shared_dto "github.com/zgiai/zgi/api/internal/dto"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	"github.com/zgiai/zgi/api/internal/modules/workspace/model"
	workspace_service "github.com/zgiai/zgi/api/internal/modules/workspace/service"
	"github.com/zgiai/zgi/api/pkg/response"
)

type fakeOrganizationService struct {
	interfaces.OrganizationService
	getWorkspaceMemberPermissionsFn  func(ctx context.Context, organizationID, workspaceID, accountID, targetAccountID string) (*shared_dto.WorkspaceMemberPermissionsResponse, error)
	getOrganizationWorkspaceDetailFn func(ctx context.Context, organizationID, workspaceID, accountID string) (*shared_dto.OrganizationWorkspaceResponse, error)
}

func (f fakeOrganizationService) GetWorkspaceMemberPermissions(ctx context.Context, organizationID, workspaceID, accountID, targetAccountID string) (*shared_dto.WorkspaceMemberPermissionsResponse, error) {
	if f.getWorkspaceMemberPermissionsFn != nil {
		return f.getWorkspaceMemberPermissionsFn(ctx, organizationID, workspaceID, accountID, targetAccountID)
	}
	return nil, nil
}

func (f fakeOrganizationService) GetOrganizationWorkspaceDetail(ctx context.Context, organizationID, workspaceID, accountID string) (*shared_dto.OrganizationWorkspaceResponse, error) {
	if f.getOrganizationWorkspaceDetailFn != nil {
		return f.getOrganizationWorkspaceDetailFn(ctx, organizationID, workspaceID, accountID)
	}
	return nil, nil
}

type fakeWorkspaceManagementService struct {
	interfaces.WorkspaceManagementService
	getWorkspaceMembersFn func(ctx context.Context, workspaceID string) ([]*interfaces.AccountWithRole, error)
}

func (f fakeWorkspaceManagementService) GetWorkspaceMembers(ctx context.Context, workspaceID string) ([]*interfaces.AccountWithRole, error) {
	if f.getWorkspaceMembersFn != nil {
		return f.getWorkspaceMembersFn(ctx, workspaceID)
	}
	return nil, nil
}

type fakeDepartmentService struct {
	workspace_service.DepartmentService
	getMemberDepartmentFn func(ctx context.Context, organizationID, accountID string) (*model.Department, error)
}

func (f fakeDepartmentService) GetMemberDepartment(ctx context.Context, organizationID, accountID string) (*model.Department, error) {
	if f.getMemberDepartmentFn != nil {
		return f.getMemberDepartmentFn(ctx, organizationID, accountID)
	}
	return nil, workspace_service.ErrMemberNotInDept
}

func TestGetWorkspaceMemberPermissionsReturnsSingleErrorResponse(t *testing.T) {
	t.Parallel()

	handler := &OrganizationHandler{
		organizationService: fakeOrganizationService{
			getWorkspaceMemberPermissionsFn: func(ctx context.Context, organizationID, workspaceID, accountID, targetAccountID string) (*shared_dto.WorkspaceMemberPermissionsResponse, error) {
				require.Equal(t, "org-1", organizationID)
				require.Equal(t, "ws-1", workspaceID)
				require.Equal(t, "acc-1", accountID)
				require.Equal(t, "acc-1", targetAccountID)
				return nil, errWorkspaceNotInOrganization
			},
		},
	}

	c, recorder := newOrganizationHandlerTestContext(http.MethodGet, "/organizations/current/workspaces/ws-1/accounts/current/permissions")
	c.Set("account_id", "acc-1")
	c.Set("organization_id", "org-1")
	c.Set("tenant_id", "org-1")
	c.Params = gin.Params{
		{Key: "organization_id", Value: "current"},
		{Key: "workspace_id", Value: "ws-1"},
		{Key: "account_id", Value: "current"},
	}

	handler.GetWorkspaceMemberPermissions(c)

	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.NotContains(t, recorder.Body.String(), `"code":"0"`)

	var resp response.Response
	err := json.Unmarshal(recorder.Body.Bytes(), &resp)
	require.NoError(t, err)
	require.Equal(t, "205017", resp.Code)
	require.Equal(t, response.ErrWorkspaceNotInOrganization.Message, resp.Message)
}

func TestGetWorkspaceMemberPermissionsReturnsSuccessResponse(t *testing.T) {
	t.Parallel()

	handler := &OrganizationHandler{
		organizationService: fakeOrganizationService{
			getWorkspaceMemberPermissionsFn: func(ctx context.Context, organizationID, workspaceID, accountID, targetAccountID string) (*shared_dto.WorkspaceMemberPermissionsResponse, error) {
				return &shared_dto.WorkspaceMemberPermissionsResponse{
					OrganizationID:    organizationID,
					WorkspaceID:       workspaceID,
					AccountID:         targetAccountID,
					OrganizationRole:  "owner",
					WorkspaceRole:     "admin",
					WorkspaceRoleName: "Admin",
					Permissions:       []string{"apps:create"},
				}, nil
			},
		},
	}

	c, recorder := newOrganizationHandlerTestContext(http.MethodGet, "/organizations/current/workspaces/ws-1/accounts/current/permissions")
	c.Set("account_id", "acc-1")
	c.Set("organization_id", "org-1")
	c.Set("tenant_id", "org-1")
	c.Params = gin.Params{
		{Key: "organization_id", Value: "current"},
		{Key: "workspace_id", Value: "ws-1"},
		{Key: "account_id", Value: "current"},
	}

	handler.GetWorkspaceMemberPermissions(c)

	require.Equal(t, http.StatusOK, recorder.Code)

	var resp response.Response
	err := json.Unmarshal(recorder.Body.Bytes(), &resp)
	require.NoError(t, err)
	require.Equal(t, "0", resp.Code)
	require.Equal(t, "success", resp.Message)
	require.Contains(t, recorder.Body.String(), `"workspace_id":"ws-1"`)
}

func TestGetOrganizationWorkspaceMemberDetailByIDReturnsHasMobile(t *testing.T) {
	t.Parallel()

	handler := &OrganizationHandler{
		organizationService: fakeOrganizationService{
			getOrganizationWorkspaceDetailFn: func(ctx context.Context, organizationID, workspaceID, accountID string) (*shared_dto.OrganizationWorkspaceResponse, error) {
				require.Equal(t, "org-1", organizationID)
				require.Equal(t, "ws-1", workspaceID)
				require.Equal(t, "acc-1", accountID)
				return &shared_dto.OrganizationWorkspaceResponse{}, nil
			},
		},
		workspaceManagementService: fakeWorkspaceManagementService{
			getWorkspaceMembersFn: func(ctx context.Context, workspaceID string) ([]*interfaces.AccountWithRole, error) {
				require.Equal(t, "ws-1", workspaceID)
				return []*interfaces.AccountWithRole{
					{
						ID:        "member-with-mobile",
						Name:      "Mobile User",
						Email:     "mobile@example.com",
						Role:      "member",
						Status:    "active",
						HasMobile: true,
					},
					{
						ID:        "member-without-mobile",
						Name:      "No Mobile User",
						Email:     "nomobile@example.com",
						Role:      "member",
						Status:    "active",
						HasMobile: false,
					},
				}, nil
			},
		},
		departmentService: fakeDepartmentService{},
	}

	c, recorder := newOrganizationHandlerTestContext(http.MethodGet, "/organizations/org-1/workspaces/ws-1/members/member-with-mobile")
	c.Set("account_id", "acc-1")
	c.Set("organization_id", "org-1")
	c.Set("tenant_id", "org-1")
	c.Params = gin.Params{
		{Key: "organization_id", Value: "org-1"},
		{Key: "workspace_id", Value: "ws-1"},
		{Key: "member_id", Value: "member-with-mobile"},
	}

	handler.GetOrganizationWorkspaceMemberDetailByID(c)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Contains(t, recorder.Body.String(), `"has_mobile":true`)
}

func newOrganizationHandlerTestContext(method, target string) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(method, target, nil)

	c, _ := gin.CreateTestContext(recorder)
	c.Request = request
	c.Request.RemoteAddr = "127.0.0.1:12345"

	return c, recorder
}

var errWorkspaceNotInOrganization = &workspaceNotInOrganizationError{}

type workspaceNotInOrganizationError struct{}

func (e *workspaceNotInOrganizationError) Error() string {
	return "workspace not in organization"
}
