package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

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
	updateOrganizationFn             func(ctx context.Context, organizationID, accountID string, req *shared_dto.UpdateOrganizationRequest) (*model.Organization, error)
	updateCurrentMemberRoleFn        func(ctx context.Context, operatorID, memberID string, role model.OrganizationRole) error
	updateMemberInfoFn               func(ctx context.Context, req *shared_dto.UpdateOrganizationMemberRequest) error
	getByIDFn                        func(ctx context.Context, organizationID string) (*model.Organization, error)
	checkAnyManagedWorkspaceFn       func(ctx context.Context, organizationID, accountID string) (bool, error)
	getMembersPaginatedFn            func(ctx context.Context, organizationID string, page, limit int, keyword string) (*shared_dto.OrganizationMemberPaginationResponse, error)
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

func (f fakeOrganizationService) UpdateOrganization(ctx context.Context, organizationID, accountID string, req *shared_dto.UpdateOrganizationRequest) (*model.Organization, error) {
	if f.updateOrganizationFn != nil {
		return f.updateOrganizationFn(ctx, organizationID, accountID, req)
	}
	return nil, nil
}

func (f fakeOrganizationService) UpdateCurrentOrganizationMemberRole(ctx context.Context, operatorID, memberID string, role model.OrganizationRole) error {
	if f.updateCurrentMemberRoleFn != nil {
		return f.updateCurrentMemberRoleFn(ctx, operatorID, memberID, role)
	}
	return nil
}

func (f fakeOrganizationService) UpdateMemberInfo(ctx context.Context, req *shared_dto.UpdateOrganizationMemberRequest) error {
	if f.updateMemberInfoFn != nil {
		return f.updateMemberInfoFn(ctx, req)
	}
	return nil
}

func (f fakeOrganizationService) GetByID(ctx context.Context, organizationID string) (*model.Organization, error) {
	if f.getByIDFn != nil {
		return f.getByIDFn(ctx, organizationID)
	}
	return &model.Organization{ID: organizationID, Status: model.OrganizationStatusActive}, nil
}

func (f fakeOrganizationService) CheckAnyManagedWorkspacePermission(ctx context.Context, organizationID, accountID string) (bool, error) {
	if f.checkAnyManagedWorkspaceFn != nil {
		return f.checkAnyManagedWorkspaceFn(ctx, organizationID, accountID)
	}
	return true, nil
}

func (f fakeOrganizationService) GetOrganizationMembersPaginated(ctx context.Context, organizationID string, page, limit int, keyword string) (*shared_dto.OrganizationMemberPaginationResponse, error) {
	if f.getMembersPaginatedFn != nil {
		return f.getMembersPaginatedFn(ctx, organizationID, page, limit, keyword)
	}
	return &shared_dto.OrganizationMemberPaginationResponse{}, nil
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

func TestPatchOrganizationReturnsUpdatedOrganization(t *testing.T) {
	t.Parallel()

	shortName := "Acme"
	handler := &OrganizationHandler{
		organizationService: fakeOrganizationService{
			updateOrganizationFn: func(ctx context.Context, organizationID, accountID string, req *shared_dto.UpdateOrganizationRequest) (*model.Organization, error) {
				require.Equal(t, "org-1", organizationID)
				require.Equal(t, "acc-1", accountID)
				require.Equal(t, "Acme Corporation", req.Name)
				require.NotNil(t, req.ShortName)
				require.Equal(t, shortName, *req.ShortName)
				return &model.Organization{
					ID:        organizationID,
					Name:      req.Name,
					ShortName: req.ShortName,
					Status:    model.OrganizationStatusActive,
					CreatedAt: time.Now(),
					UpdatedAt: time.Now(),
				}, nil
			},
		},
	}

	c, recorder := newOrganizationHandlerTestContext(http.MethodPatch, "/organizations/info/org-1")
	c.Set("account_id", "acc-1")
	c.Params = gin.Params{{Key: "organization_id", Value: "org-1"}}
	c.Request.Body = io.NopCloser(bytes.NewBufferString(`{"name":"Acme Corporation","short_name":"Acme"}`))
	c.Request.Header.Set("Content-Type", "application/json")

	handler.PatchOrganization(c)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Contains(t, recorder.Body.String(), `"name":"Acme Corporation"`)
	require.Contains(t, recorder.Body.String(), `"short_name":"Acme"`)
}

func TestPatchOrganizationMapsExpectedErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		err      error
		wantCode int
	}{
		{
			name:     "invalid name",
			err:      workspace_service.ErrInvalidOrganizationName,
			wantCode: response.ErrInvalidParam.Code,
		},
		{
			name:     "not found",
			err:      workspace_service.ErrOrganizationNotFound,
			wantCode: response.ErrOrganizationNotFound.Code,
		},
		{
			name:     "duplicate name",
			err:      workspace_service.ErrOrganizationNameExists,
			wantCode: response.ErrOrganizationExists.Code,
		},
		{
			name:     "permission denied",
			err:      workspace_service.ErrOrganizationPermissionDenied,
			wantCode: response.ErrPermissionDenied.Code,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			handler := &OrganizationHandler{
				organizationService: fakeOrganizationService{
					updateOrganizationFn: func(ctx context.Context, organizationID, accountID string, req *shared_dto.UpdateOrganizationRequest) (*model.Organization, error) {
						return nil, tt.err
					},
				},
			}

			c, recorder := newOrganizationHandlerTestContext(http.MethodPatch, "/organizations/info/org-1")
			c.Set("account_id", "acc-1")
			c.Params = gin.Params{{Key: "organization_id", Value: "org-1"}}
			c.Request.Body = io.NopCloser(bytes.NewBufferString(`{"name":"Acme"}`))
			c.Request.Header.Set("Content-Type", "application/json")

			handler.PatchOrganization(c)

			var resp response.Response
			require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &resp))
			require.Equal(t, strconv.Itoa(tt.wantCode), resp.Code)
		})
	}
}

func TestUpdateCurrentOrganizationMemberRoleReturnsSuccess(t *testing.T) {
	t.Parallel()

	handler := &OrganizationHandler{
		organizationService: fakeOrganizationService{
			updateCurrentMemberRoleFn: func(ctx context.Context, operatorID, memberID string, role model.OrganizationRole) error {
				require.Equal(t, "owner-1", operatorID)
				require.Equal(t, "member-1", memberID)
				require.Equal(t, model.OrganizationRoleAdmin, role)
				return nil
			},
		},
	}

	c, recorder := newOrganizationHandlerTestContext(http.MethodPatch, "/organizations/current/members/member-1/organization-role")
	c.Set("account_id", "owner-1")
	c.Params = gin.Params{{Key: "member_id", Value: "member-1"}}
	c.Request.Body = io.NopCloser(bytes.NewBufferString(`{"role":"admin"}`))
	c.Request.Header.Set("Content-Type", "application/json")

	handler.UpdateCurrentOrganizationMemberRole(c)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Contains(t, recorder.Body.String(), `"result":"success"`)
}

func TestUpdateCurrentOrganizationMemberRoleMapsExpectedErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		err      error
		wantCode int
	}{
		{
			name:     "invalid role",
			err:      workspace_service.ErrInvalidOrganizationMemberRole,
			wantCode: response.ErrInvalidParam.Code,
		},
		{
			name:     "owner immutable",
			err:      workspace_service.ErrOrganizationOwnerRoleImmutable,
			wantCode: response.ErrInvalidParam.Code,
		},
		{
			name:     "inactive member",
			err:      workspace_service.ErrOrganizationMemberNotActive,
			wantCode: response.ErrInvalidParam.Code,
		},
		{
			name:     "member not found",
			err:      workspace_service.ErrOrganizationMemberNotFound,
			wantCode: response.ErrMemberNotFound.Code,
		},
		{
			name:     "permission denied",
			err:      workspace_service.ErrOrganizationPermissionDenied,
			wantCode: response.ErrPermissionDenied.Code,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			handler := &OrganizationHandler{
				organizationService: fakeOrganizationService{
					updateCurrentMemberRoleFn: func(ctx context.Context, operatorID, memberID string, role model.OrganizationRole) error {
						return tt.err
					},
				},
			}

			c, recorder := newOrganizationHandlerTestContext(http.MethodPatch, "/organizations/current/members/member-1/organization-role")
			c.Set("account_id", "owner-1")
			c.Params = gin.Params{{Key: "member_id", Value: "member-1"}}
			c.Request.Body = io.NopCloser(bytes.NewBufferString(`{"role":"admin"}`))
			c.Request.Header.Set("Content-Type", "application/json")

			handler.UpdateCurrentOrganizationMemberRole(c)

			var resp response.Response
			require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &resp))
			require.Equal(t, strconv.Itoa(tt.wantCode), resp.Code)
		})
	}
}

func TestUpdateOrganizationMemberRejectsRoleUpdates(t *testing.T) {
	t.Parallel()

	called := false
	handler := &OrganizationHandler{
		organizationService: fakeOrganizationService{
			updateMemberInfoFn: func(ctx context.Context, req *shared_dto.UpdateOrganizationMemberRequest) error {
				called = true
				return nil
			},
		},
	}

	c, recorder := newOrganizationHandlerTestContext(http.MethodPut, "/organizations/org-1/members/member-1")
	c.Set("account_id", "owner-1")
	c.Params = gin.Params{
		{Key: "organization_id", Value: "org-1"},
		{Key: "member_id", Value: "member-1"},
	}
	c.Request.Body = io.NopCloser(bytes.NewBufferString(`{"role":"admin"}`))
	c.Request.Header.Set("Content-Type", "application/json")

	handler.UpdateOrganizationMember(c)

	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.False(t, called)
}

func TestGetCurrentOrganizationMembersUsesKeyword(t *testing.T) {
	t.Parallel()

	handler := &OrganizationHandler{
		organizationService: fakeOrganizationService{
			getMembersPaginatedFn: func(ctx context.Context, organizationID string, page, limit int, keyword string) (*shared_dto.OrganizationMemberPaginationResponse, error) {
				require.Equal(t, "org-1", organizationID)
				require.Equal(t, 2, page)
				require.Equal(t, 5, limit)
				require.Equal(t, "alice", keyword)
				return &shared_dto.OrganizationMemberPaginationResponse{
					Data: []*shared_dto.OrganizationMemberWithExtensionResponse{
						{
							ID:               "member-1",
							Name:             "Alice",
							Email:            "alice@example.com",
							Status:           "active",
							OrganizationRole: model.OrganizationRoleNormal,
						},
					},
					Page:  2,
					Limit: 5,
					Total: 1,
				}, nil
			},
		},
	}

	c, recorder := newOrganizationHandlerTestContext(http.MethodGet, "/organizations/current/members?page=2&limit=5&keyword=alice")
	c.Set("account_id", "owner-1")
	c.Set("organization_id", "org-1")
	c.Set("tenant_id", "org-1")

	handler.GetCurrentOrganizationMembers(c)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Contains(t, recorder.Body.String(), `"email":"alice@example.com"`)
}

func TestOrganizationRoutesRegisterCurrentMembersList(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	router := gin.New()
	handler := &OrganizationHandler{organizationService: fakeOrganizationService{}}

	handler.RegisterRoutes(router.Group(""))

	for _, route := range router.Routes() {
		if route.Method == http.MethodGet && route.Path == "/organizations/current/members" {
			return
		}
	}

	t.Fatalf("GET /organizations/current/members route was not registered")
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
