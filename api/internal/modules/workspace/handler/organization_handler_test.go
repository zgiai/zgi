package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	shared_dto "github.com/zgiai/ginext/internal/dto"
	interfaces "github.com/zgiai/ginext/internal/modules/shared/interface"
	"github.com/zgiai/ginext/pkg/response"
)

type fakeOrganizationService struct {
	interfaces.OrganizationService
	getWorkspaceMemberPermissionsFn func(ctx context.Context, organizationID, workspaceID, accountID, targetAccountID string) (*shared_dto.WorkspaceMemberPermissionsResponse, error)
}

func (f fakeOrganizationService) GetWorkspaceMemberPermissions(ctx context.Context, organizationID, workspaceID, accountID, targetAccountID string) (*shared_dto.WorkspaceMemberPermissionsResponse, error) {
	if f.getWorkspaceMemberPermissionsFn != nil {
		return f.getWorkspaceMemberPermissionsFn(ctx, organizationID, workspaceID, accountID, targetAccountID)
	}
	return nil, nil
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
