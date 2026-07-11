package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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
	auth_model "github.com/zgiai/zgi/api/internal/modules/user/auth/model"
	"github.com/zgiai/zgi/api/internal/modules/workspace/model"
	workspace_service "github.com/zgiai/zgi/api/internal/modules/workspace/service"
	"github.com/zgiai/zgi/api/pkg/response"
)

type fakeOrganizationService struct {
	interfaces.OrganizationService
	getWorkspaceMemberPermissionsFn  func(ctx context.Context, organizationID, workspaceID, accountID, targetAccountID string) (*shared_dto.WorkspaceMemberPermissionsResponse, error)
	getOrganizationWorkspaceDetailFn func(ctx context.Context, organizationID, workspaceID, accountID string) (*shared_dto.OrganizationWorkspaceResponse, error)
	getOrganizationByWorkspaceIDFn   func(ctx context.Context, workspaceID string) (*model.Organization, error)
	checkWorkspacePermissionFn       func(ctx context.Context, organizationID, workspaceID, accountID string, permissionCode model.WorkspacePermissionCode) (bool, error)
	checkWorkspaceAssetsFn           func(ctx context.Context, workspaceID string) (bool, map[string]int64, error)
	updateOrganizationFn             func(ctx context.Context, organizationID, accountID string, req *shared_dto.UpdateOrganizationRequest) (*model.Organization, error)
	updateCurrentMemberRoleFn        func(ctx context.Context, operatorID, memberID string, role model.OrganizationRole) error
	updateMemberInfoFn               func(ctx context.Context, req *shared_dto.UpdateOrganizationMemberRequest) error
	getByIDFn                        func(ctx context.Context, organizationID string) (*model.Organization, error)
	checkAnyManagedWorkspaceFn       func(ctx context.Context, organizationID, accountID string) (bool, error)
	isOrganizationAdminOrOwnerFn     func(ctx context.Context, organizationID, accountID string) (bool, error)
	getMembersPaginatedFn            func(ctx context.Context, organizationID string, page, limit int, keyword string) (*shared_dto.OrganizationMemberPaginationResponse, error)
	getVisibleMembersPaginatedFn     func(ctx context.Context, organizationID, accountID string, page, limit int, keyword string) (*shared_dto.OrganizationMemberPaginationResponse, error)
	getMemberByAccountIDFn           func(ctx context.Context, organizationID, accountID string) (*shared_dto.OrganizationMemberWithExtensionResponse, error)
	existsMemberByNameFn             func(ctx context.Context, organizationID string, name string, excludeAccountID string) (bool, error)
	isOrganizationMemberFn           func(ctx context.Context, organizationID, accountID string) (bool, error)
	addMemberFn                      func(ctx context.Context, req *shared_dto.AddOrganizationMemberRequest) error
	directAddMemberFn                func(ctx context.Context, req *shared_dto.DirectAddOrganizationMemberRequest) (*shared_dto.DirectAddOrganizationMemberResponse, error)
	listWorkspacePermissionDefsFn    func(ctx context.Context, organizationID, accountID string) ([]string, error)
	listWorkspaceRolesFn             func(ctx context.Context, organizationID, accountID string, includeOwner bool) (*shared_dto.WorkspaceRoleListResponse, error)
	getWorkspaceRoleDetailFn         func(ctx context.Context, organizationID, roleID, accountID string) (*shared_dto.OrganizationRoleDetailResponse, error)
	applyWorkspaceRoleTemplateFn     func(ctx context.Context, req *shared_dto.ApplyWorkspaceRoleTemplateRequest) (*shared_dto.ApplyWorkspaceRoleTemplateResponse, error)
	replaceAndDeleteRoleFn           func(ctx context.Context, req *shared_dto.ReplaceWorkspaceRoleTemplateRequest) (*shared_dto.ReplaceWorkspaceRoleTemplateResponse, error)
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

func (f fakeOrganizationService) GetOrganizationByWorkspaceID(ctx context.Context, workspaceID string) (*model.Organization, error) {
	if f.getOrganizationByWorkspaceIDFn != nil {
		return f.getOrganizationByWorkspaceIDFn(ctx, workspaceID)
	}
	return &model.Organization{ID: "org-1", Status: model.OrganizationStatusActive}, nil
}

func (f fakeOrganizationService) CheckWorkspacePermission(ctx context.Context, organizationID, workspaceID, accountID string, permissionCode model.WorkspacePermissionCode) (bool, error) {
	if f.checkWorkspacePermissionFn != nil {
		return f.checkWorkspacePermissionFn(ctx, organizationID, workspaceID, accountID, permissionCode)
	}
	return true, nil
}

func (f fakeOrganizationService) CheckWorkspaceAssets(ctx context.Context, workspaceID string) (bool, map[string]int64, error) {
	if f.checkWorkspaceAssetsFn != nil {
		return f.checkWorkspaceAssetsFn(ctx, workspaceID)
	}
	return false, map[string]int64{}, nil
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

func (f fakeOrganizationService) IsOrganizationAdminOrOwner(ctx context.Context, organizationID, accountID string) (bool, error) {
	if f.isOrganizationAdminOrOwnerFn != nil {
		return f.isOrganizationAdminOrOwnerFn(ctx, organizationID, accountID)
	}
	return false, nil
}

func (f fakeOrganizationService) GetOrganizationMembersPaginated(ctx context.Context, organizationID string, page, limit int, keyword string) (*shared_dto.OrganizationMemberPaginationResponse, error) {
	if f.getMembersPaginatedFn != nil {
		return f.getMembersPaginatedFn(ctx, organizationID, page, limit, keyword)
	}
	return &shared_dto.OrganizationMemberPaginationResponse{}, nil
}

func (f fakeOrganizationService) GetVisibleOrganizationMembersPaginated(ctx context.Context, organizationID, accountID string, page, limit int, keyword string) (*shared_dto.OrganizationMemberPaginationResponse, error) {
	if f.getVisibleMembersPaginatedFn != nil {
		return f.getVisibleMembersPaginatedFn(ctx, organizationID, accountID, page, limit, keyword)
	}
	return &shared_dto.OrganizationMemberPaginationResponse{}, nil
}

func (f fakeOrganizationService) GetOrganizationMemberByAccountID(ctx context.Context, organizationID, accountID string) (*shared_dto.OrganizationMemberWithExtensionResponse, error) {
	if f.getMemberByAccountIDFn != nil {
		return f.getMemberByAccountIDFn(ctx, organizationID, accountID)
	}
	return nil, nil
}

func (f fakeOrganizationService) ExistsMemberByName(ctx context.Context, organizationID string, name string, excludeAccountID string) (bool, error) {
	if f.existsMemberByNameFn != nil {
		return f.existsMemberByNameFn(ctx, organizationID, name, excludeAccountID)
	}
	return false, nil
}

func (f fakeOrganizationService) IsOrganizationMember(ctx context.Context, organizationID, accountID string) (bool, error) {
	if f.isOrganizationMemberFn != nil {
		return f.isOrganizationMemberFn(ctx, organizationID, accountID)
	}
	return true, nil
}

func (f fakeOrganizationService) AddMember(ctx context.Context, req *shared_dto.AddOrganizationMemberRequest) error {
	if f.addMemberFn != nil {
		return f.addMemberFn(ctx, req)
	}
	return nil
}

func (f fakeOrganizationService) DirectAddOrganizationMember(ctx context.Context, req *shared_dto.DirectAddOrganizationMemberRequest) (*shared_dto.DirectAddOrganizationMemberResponse, error) {
	if f.directAddMemberFn != nil {
		return f.directAddMemberFn(ctx, req)
	}
	return &shared_dto.DirectAddOrganizationMemberResponse{
		AccountID:      "member-1",
		Email:          "alice@example.com",
		Name:           "Alice",
		OrganizationID: req.OrganizationID,
		Workspace: &shared_dto.MemberWorkspaceInfo{
			ID:   req.WorkspaceID,
			Name: "Workspace",
		},
	}, nil
}

func (f fakeOrganizationService) ListWorkspacePermissionDefinitions(ctx context.Context, organizationID, accountID string) ([]string, error) {
	if f.listWorkspacePermissionDefsFn != nil {
		return f.listWorkspacePermissionDefsFn(ctx, organizationID, accountID)
	}
	return nil, nil
}

func (f fakeOrganizationService) ListWorkspaceRoles(ctx context.Context, organizationID, accountID string, includeOwner bool) (*shared_dto.WorkspaceRoleListResponse, error) {
	if f.listWorkspaceRolesFn != nil {
		return f.listWorkspaceRolesFn(ctx, organizationID, accountID, includeOwner)
	}
	return &shared_dto.WorkspaceRoleListResponse{}, nil
}

func (f fakeOrganizationService) GetWorkspaceRoleDetail(ctx context.Context, organizationID, roleID, accountID string) (*shared_dto.OrganizationRoleDetailResponse, error) {
	if f.getWorkspaceRoleDetailFn != nil {
		return f.getWorkspaceRoleDetailFn(ctx, organizationID, roleID, accountID)
	}
	return &shared_dto.OrganizationRoleDetailResponse{}, nil
}

func (f fakeOrganizationService) ApplyWorkspaceRoleTemplate(ctx context.Context, req *shared_dto.ApplyWorkspaceRoleTemplateRequest) (*shared_dto.ApplyWorkspaceRoleTemplateResponse, error) {
	if f.applyWorkspaceRoleTemplateFn != nil {
		return f.applyWorkspaceRoleTemplateFn(ctx, req)
	}
	return &shared_dto.ApplyWorkspaceRoleTemplateResponse{}, nil
}

func (f fakeOrganizationService) ReplaceAndDeleteCustomWorkspaceRole(ctx context.Context, req *shared_dto.ReplaceWorkspaceRoleTemplateRequest) (*shared_dto.ReplaceWorkspaceRoleTemplateResponse, error) {
	if f.replaceAndDeleteRoleFn != nil {
		return f.replaceAndDeleteRoleFn(ctx, req)
	}
	return &shared_dto.ReplaceWorkspaceRoleTemplateResponse{}, nil
}

type fakeWorkspaceManagementService struct {
	interfaces.WorkspaceManagementService
	getWorkspaceMembersFn           func(ctx context.Context, workspaceID string) ([]*interfaces.AccountWithRole, error)
	getWorkspaceMembersPaginatedFn  func(ctx context.Context, workspaceID string, page, limit int, keyword, roleFilter string) ([]*interfaces.AccountWithRole, int64, error)
	getWorkspaceByIDFn              func(ctx context.Context, id string) (*model.Workspace, error)
	getCurrentWorkspaceFn           func(ctx context.Context, accountID string) (*model.WorkspaceMember, error)
	getUserRoleFn                   func(ctx context.Context, accountID, workspaceID string) (*model.WorkspaceMemberRole, error)
	addMemberFn                     func(ctx context.Context, req *interfaces.AddMemberRequest) error
	updateMemberDirectPermissionsFn func(ctx context.Context, workspaceID, accountID string, permissions []string) error
}

func (f fakeWorkspaceManagementService) GetWorkspaceMembers(ctx context.Context, workspaceID string) ([]*interfaces.AccountWithRole, error) {
	if f.getWorkspaceMembersFn != nil {
		return f.getWorkspaceMembersFn(ctx, workspaceID)
	}
	return nil, nil
}

func (f fakeWorkspaceManagementService) GetWorkspaceMembersPaginated(ctx context.Context, workspaceID string, page, limit int, keyword, roleFilter string) ([]*interfaces.AccountWithRole, int64, error) {
	if f.getWorkspaceMembersPaginatedFn != nil {
		return f.getWorkspaceMembersPaginatedFn(ctx, workspaceID, page, limit, keyword, roleFilter)
	}
	return nil, 0, nil
}

func (f fakeWorkspaceManagementService) GetWorkspaceByID(ctx context.Context, id string) (*model.Workspace, error) {
	if f.getWorkspaceByIDFn != nil {
		return f.getWorkspaceByIDFn(ctx, id)
	}
	return nil, nil
}

func (f fakeWorkspaceManagementService) GetCurrentWorkspace(ctx context.Context, accountID string) (*model.WorkspaceMember, error) {
	if f.getCurrentWorkspaceFn != nil {
		return f.getCurrentWorkspaceFn(ctx, accountID)
	}
	return nil, nil
}

func (f fakeWorkspaceManagementService) GetUserRole(ctx context.Context, accountID, workspaceID string) (*model.WorkspaceMemberRole, error) {
	if f.getUserRoleFn != nil {
		return f.getUserRoleFn(ctx, accountID, workspaceID)
	}
	return nil, nil
}

func (f fakeWorkspaceManagementService) AddMember(ctx context.Context, req *interfaces.AddMemberRequest) error {
	if f.addMemberFn != nil {
		return f.addMemberFn(ctx, req)
	}
	return nil
}

func (f fakeWorkspaceManagementService) UpdateMemberDirectPermissions(ctx context.Context, workspaceID, accountID string, permissions []string) error {
	if f.updateMemberDirectPermissionsFn != nil {
		return f.updateMemberDirectPermissionsFn(ctx, workspaceID, accountID, permissions)
	}
	return nil
}

type fakeAccountService struct {
	interfaces.AccountService
	getUserThroughEmailFn              func(ctx context.Context, email string) (*auth_model.Account, error)
	getAccountByIDFn                   func(ctx context.Context, id string) (*auth_model.Account, error)
	getAccountContextFn                func(ctx context.Context, accountID string) (*auth_model.AccountContext, error)
	createAccountFn                    func(ctx context.Context, req *shared_dto.CreateAccountRequest) (*auth_model.Account, error)
	ensureAccountContextForWorkspaceFn func(ctx context.Context, accountID, organizationID, workspaceID string) (*auth_model.AccountContext, bool, error)
	isOrganizationAdminOrOwnerFn       func(ctx context.Context, organizationID, accountID string) (bool, error)
	isEmailSendIPLimitFn               func(ctx context.Context, ipAddress string) (bool, error)
	sendDirectAddMemberEmailFn         func(ctx context.Context, account *auth_model.Account, groupID, groupName, departmentName, language string) error
}

func (f fakeAccountService) GetUserThroughEmail(ctx context.Context, email string) (*auth_model.Account, error) {
	if f.getUserThroughEmailFn != nil {
		return f.getUserThroughEmailFn(ctx, email)
	}
	return nil, nil
}

func (f fakeAccountService) GetAccountByID(ctx context.Context, id string) (*auth_model.Account, error) {
	if f.getAccountByIDFn != nil {
		return f.getAccountByIDFn(ctx, id)
	}
	return &auth_model.Account{ID: id}, nil
}

func (f fakeAccountService) GetAccountContext(ctx context.Context, accountID string) (*auth_model.AccountContext, error) {
	if f.getAccountContextFn != nil {
		return f.getAccountContextFn(ctx, accountID)
	}
	return nil, nil
}

func (f fakeAccountService) CreateAccount(ctx context.Context, req *shared_dto.CreateAccountRequest) (*auth_model.Account, error) {
	if f.createAccountFn != nil {
		return f.createAccountFn(ctx, req)
	}
	return nil, nil
}

func (f fakeAccountService) EnsureAccountContextForWorkspace(ctx context.Context, accountID, organizationID, workspaceID string) (*auth_model.AccountContext, bool, error) {
	if f.ensureAccountContextForWorkspaceFn != nil {
		return f.ensureAccountContextForWorkspaceFn(ctx, accountID, organizationID, workspaceID)
	}
	return &auth_model.AccountContext{AccountID: accountID}, false, nil
}

func (f fakeAccountService) IsOrganizationAdminOrOwner(ctx context.Context, organizationID, accountID string) (bool, error) {
	if f.isOrganizationAdminOrOwnerFn != nil {
		return f.isOrganizationAdminOrOwnerFn(ctx, organizationID, accountID)
	}
	return true, nil
}

func (f fakeAccountService) IsEmailSendIPLimit(ctx context.Context, ipAddress string) (bool, error) {
	if f.isEmailSendIPLimitFn != nil {
		return f.isEmailSendIPLimitFn(ctx, ipAddress)
	}
	return false, nil
}

func (f fakeAccountService) SendDirectAddMemberEmail(ctx context.Context, account *auth_model.Account, groupID, groupName, departmentName, language string) error {
	if f.sendDirectAddMemberEmailFn != nil {
		return f.sendDirectAddMemberEmailFn(ctx, account, groupID, groupName, departmentName, language)
	}
	return nil
}

type fakeDepartmentService struct {
	workspace_service.DepartmentService
	getMemberDepartmentFn func(ctx context.Context, organizationID, accountID string) (*model.Department, error)
	getDepartmentFn       func(ctx context.Context, id string) (*model.Department, error)
	getDepartmentTreeFn   func(ctx context.Context, organizationID string) ([]*workspace_service.DepartmentTreeNode, error)
	addMemberFn           func(ctx context.Context, organizationID, departmentID, accountID string) (*model.DepartmentMember, error)
}

func (f fakeDepartmentService) GetDepartment(ctx context.Context, id string) (*model.Department, error) {
	if f.getDepartmentFn != nil {
		return f.getDepartmentFn(ctx, id)
	}
	return nil, workspace_service.ErrDepartmentNotFound
}

func (f fakeDepartmentService) AddMemberToDepartment(ctx context.Context, organizationID, departmentID, accountID string) (*model.DepartmentMember, error) {
	if f.addMemberFn != nil {
		return f.addMemberFn(ctx, organizationID, departmentID, accountID)
	}
	return &model.DepartmentMember{DepartmentID: departmentID, AccountID: accountID}, nil
}

func (f fakeDepartmentService) GetMemberDepartment(ctx context.Context, organizationID, accountID string) (*model.Department, error) {
	if f.getMemberDepartmentFn != nil {
		return f.getMemberDepartmentFn(ctx, organizationID, accountID)
	}
	return nil, workspace_service.ErrMemberNotInDept
}

func (f fakeDepartmentService) GetDepartmentTree(ctx context.Context, organizationID string) ([]*workspace_service.DepartmentTreeNode, error) {
	if f.getDepartmentTreeFn != nil {
		return f.getDepartmentTreeFn(ctx, organizationID)
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

func TestGetWorkspaceMemberPermissionsMapsMissingWorkspaceMember(t *testing.T) {
	t.Parallel()

	handler := &OrganizationHandler{
		organizationService: fakeOrganizationService{
			getWorkspaceMemberPermissionsFn: func(ctx context.Context, organizationID, workspaceID, accountID, targetAccountID string) (*shared_dto.WorkspaceMemberPermissionsResponse, error) {
				require.Equal(t, "org-1", organizationID)
				require.Equal(t, "ws-1", workspaceID)
				require.Equal(t, "acc-1", accountID)
				require.Equal(t, "acc-1", targetAccountID)
				return nil, errors.New("workspace member not found")
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

	var resp response.Response
	err := json.Unmarshal(recorder.Body.Bytes(), &resp)
	require.NoError(t, err)
	require.Equal(t, "205007", resp.Code)
	require.Equal(t, response.ErrMemberNotInWorkspace.Message, resp.Message)
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

func TestGetWorkspaceMemberPermissionsResolvesCurrentWorkspace(t *testing.T) {
	t.Parallel()

	handler := &OrganizationHandler{
		organizationService: fakeOrganizationService{
			getOrganizationByWorkspaceIDFn: func(ctx context.Context, workspaceID string) (*model.Organization, error) {
				require.Equal(t, "ws-current", workspaceID)
				return &model.Organization{ID: "org-1", Status: model.OrganizationStatusActive}, nil
			},
			getWorkspaceMemberPermissionsFn: func(ctx context.Context, organizationID, workspaceID, accountID, targetAccountID string) (*shared_dto.WorkspaceMemberPermissionsResponse, error) {
				require.Equal(t, "org-1", organizationID)
				require.Equal(t, "ws-current", workspaceID)
				require.Equal(t, "acc-1", accountID)
				require.Equal(t, "acc-1", targetAccountID)
				return &shared_dto.WorkspaceMemberPermissionsResponse{
					OrganizationID: organizationID,
					WorkspaceID:    workspaceID,
					AccountID:      targetAccountID,
					Permissions:    []string{"agent.view"},
				}, nil
			},
		},
		accountService: fakeAccountService{
			getAccountContextFn: func(ctx context.Context, accountID string) (*auth_model.AccountContext, error) {
				require.Equal(t, "acc-1", accountID)
				currentWorkspaceID := "ws-current"
				return &auth_model.AccountContext{AccountID: accountID, CurrentWorkspaceID: &currentWorkspaceID}, nil
			},
		},
		workspaceManagementService: fakeWorkspaceManagementService{
			getCurrentWorkspaceFn: func(ctx context.Context, accountID string) (*model.WorkspaceMember, error) {
				t.Fatalf("workspace member current fallback should not be called when account context has current workspace")
				return nil, nil
			},
		},
	}

	c, recorder := newOrganizationHandlerTestContext(http.MethodGet, "/organizations/current/workspaces/current/accounts/current/permissions")
	c.Set("account_id", "acc-1")
	c.Set("organization_id", "org-1")
	c.Set("tenant_id", "org-1")
	c.Params = gin.Params{
		{Key: "organization_id", Value: "current"},
		{Key: "workspace_id", Value: "current"},
		{Key: "account_id", Value: "current"},
	}

	handler.GetWorkspaceMemberPermissions(c)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Contains(t, recorder.Body.String(), `"workspace_id":"ws-current"`)
	require.Contains(t, recorder.Body.String(), `"agent.view"`)
}

func TestUpdateOrganizationWorkspaceMemberPermissionsUsesPermissionManage(t *testing.T) {
	t.Parallel()

	updateCalled := false
	handler := &OrganizationHandler{
		organizationService: fakeOrganizationService{
			checkWorkspacePermissionFn: func(ctx context.Context, organizationID, workspaceID, accountID string, permissionCode model.WorkspacePermissionCode) (bool, error) {
				require.Equal(t, "org-1", organizationID)
				require.Equal(t, "ws-1", workspaceID)
				require.Equal(t, "operator-1", accountID)
				require.Equal(t, model.WorkspacePermissionWorkspacePermissionManage, permissionCode)
				return true, nil
			},
		},
		workspaceManagementService: fakeWorkspaceManagementService{
			updateMemberDirectPermissionsFn: func(ctx context.Context, workspaceID, accountID string, permissions []string) error {
				updateCalled = true
				require.Equal(t, "ws-1", workspaceID)
				require.Equal(t, "member-1", accountID)
				require.ElementsMatch(t, []string{"agent.manage"}, permissions)
				return nil
			},
		},
		accountService: fakeAccountService{},
	}

	c, recorder := newOrganizationHandlerTestContext(http.MethodPut, "/organizations/org-1/workspaces/ws-1/members/member-1/permissions")
	c.Set("account_id", "operator-1")
	c.Params = gin.Params{
		{Key: "organization_id", Value: "org-1"},
		{Key: "workspace_id", Value: "ws-1"},
		{Key: "member_id", Value: "member-1"},
	}
	c.Request.Body = io.NopCloser(bytes.NewBufferString(`{"permissions":["agent.manage"]}`))
	c.Request.Header.Set("Content-Type", "application/json")

	handler.UpdateOrganizationWorkspaceMemberPermissions(c)

	require.True(t, updateCalled)
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Contains(t, recorder.Body.String(), `"result":"success"`)
}

func TestUpdateOrganizationWorkspaceMemberPermissionsRejectsMissingPermission(t *testing.T) {
	t.Parallel()

	updateCalled := false
	handler := &OrganizationHandler{
		organizationService: fakeOrganizationService{
			checkWorkspacePermissionFn: func(ctx context.Context, organizationID, workspaceID, accountID string, permissionCode model.WorkspacePermissionCode) (bool, error) {
				require.Equal(t, model.WorkspacePermissionWorkspacePermissionManage, permissionCode)
				return false, nil
			},
		},
		workspaceManagementService: fakeWorkspaceManagementService{
			updateMemberDirectPermissionsFn: func(ctx context.Context, workspaceID, accountID string, permissions []string) error {
				updateCalled = true
				return nil
			},
		},
		accountService: fakeAccountService{},
	}

	c, recorder := newOrganizationHandlerTestContext(http.MethodPut, "/organizations/org-1/workspaces/ws-1/members/member-1/permissions")
	c.Set("account_id", "operator-1")
	c.Params = gin.Params{
		{Key: "organization_id", Value: "org-1"},
		{Key: "workspace_id", Value: "ws-1"},
		{Key: "member_id", Value: "member-1"},
	}
	c.Request.Body = io.NopCloser(bytes.NewBufferString(`{"permissions":["agent.manage"]}`))
	c.Request.Header.Set("Content-Type", "application/json")

	handler.UpdateOrganizationWorkspaceMemberPermissions(c)

	require.False(t, updateCalled)
	require.Equal(t, http.StatusForbidden, recorder.Code)
}

func TestBatchAddOrganizationMembersToWorkspaceDefaultsToNormalRole(t *testing.T) {
	t.Parallel()

	addCalled := false
	var addedRole model.WorkspaceMemberRole
	handler := &OrganizationHandler{
		organizationService: fakeOrganizationService{
			getOrganizationByWorkspaceIDFn: func(ctx context.Context, workspaceID string) (*model.Organization, error) {
				require.Equal(t, "ws-1", workspaceID)
				return &model.Organization{ID: "org-1", Status: model.OrganizationStatusActive}, nil
			},
			checkWorkspacePermissionFn: func(ctx context.Context, organizationID, workspaceID, accountID string, permissionCode model.WorkspacePermissionCode) (bool, error) {
				require.Equal(t, "org-1", organizationID)
				require.Equal(t, "ws-1", workspaceID)
				require.Equal(t, "operator-1", accountID)
				require.Equal(t, model.WorkspacePermissionWorkspaceMemberManage, permissionCode)
				return true, nil
			},
		},
		workspaceManagementService: fakeWorkspaceManagementService{
			getWorkspaceByIDFn: func(ctx context.Context, id string) (*model.Workspace, error) {
				require.Equal(t, "ws-1", id)
				return &model.Workspace{ID: id}, nil
			},
			getUserRoleFn: func(ctx context.Context, accountID, workspaceID string) (*model.WorkspaceMemberRole, error) {
				return nil, nil
			},
			addMemberFn: func(ctx context.Context, req *interfaces.AddMemberRequest) error {
				addCalled = true
				require.Equal(t, "ws-1", req.WorkspaceID)
				require.Equal(t, "member-1", req.AccountID)
				addedRole = req.Role
				require.Nil(t, req.Permissions)
				require.Nil(t, req.RoleID)
				return nil
			},
		},
		accountService: fakeAccountService{},
	}

	c, recorder := newOrganizationHandlerTestContext(http.MethodPost, "/organizations/org-1/workspaces/ws-1/members/batch-add")
	c.Set("account_id", "operator-1")
	c.Set("organization_id", "org-1")
	c.Set("tenant_id", "org-1")
	c.Params = gin.Params{
		{Key: "organization_id", Value: "org-1"},
		{Key: "workspace_id", Value: "ws-1"},
	}
	c.Request.Body = io.NopCloser(bytes.NewBufferString(`{"account_ids":["member-1"]}`))
	c.Request.Header.Set("Content-Type", "application/json")

	handler.BatchAddOrganizationMembersToWorkspace(c)

	require.True(t, addCalled)
	require.Equal(t, model.WorkspaceRoleNormal, addedRole)
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
}

func TestBatchAddOrganizationMembersToWorkspaceRejectsDirectPermissions(t *testing.T) {
	t.Parallel()

	addCalled := false
	handler := &OrganizationHandler{
		workspaceManagementService: fakeWorkspaceManagementService{
			addMemberFn: func(ctx context.Context, req *interfaces.AddMemberRequest) error {
				addCalled = true
				return nil
			},
		},
		accountService: fakeAccountService{},
	}

	c, recorder := newOrganizationHandlerTestContext(http.MethodPost, "/organizations/org-1/workspaces/ws-1/members/batch-add")
	c.Set("account_id", "operator-1")
	c.Params = gin.Params{
		{Key: "organization_id", Value: "org-1"},
		{Key: "workspace_id", Value: "ws-1"},
	}
	c.Request.Body = io.NopCloser(bytes.NewBufferString(`{"account_ids":["member-1"],"permissions":["agent.view"]}`))
	c.Request.Header.Set("Content-Type", "application/json")

	handler.BatchAddOrganizationMembersToWorkspace(c)

	require.False(t, addCalled)
	require.Equal(t, http.StatusBadRequest, recorder.Code)
}

func TestOrganizationWorkspaceRoutesRejectCrossOrganizationWorkspaceBeforePermission(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		method string
		target string
		params gin.Params
		body   string
		call   func(*OrganizationHandler, *gin.Context)
	}{
		{
			name:   "workspace assets",
			method: http.MethodGet,
			target: "/organizations/org-1/workspaces/ws-other/assets",
			params: gin.Params{
				{Key: "organization_id", Value: "org-1"},
				{Key: "workspace_id", Value: "ws-other"},
			},
			call: (*OrganizationHandler).GetOrganizationWorkspaceAssets,
		},
		{
			name:   "remove workspace member",
			method: http.MethodDelete,
			target: "/organizations/org-1/workspaces/ws-other/members/member-1",
			params: gin.Params{
				{Key: "organization_id", Value: "org-1"},
				{Key: "workspace_id", Value: "ws-other"},
				{Key: "member_id", Value: "member-1"},
			},
			call: (*OrganizationHandler).RemoveOrganizationWorkspaceMember,
		},
		{
			name:   "update workspace member role",
			method: http.MethodPut,
			target: "/organizations/org-1/workspaces/ws-other/members/member-1/update-role",
			params: gin.Params{
				{Key: "organization_id", Value: "org-1"},
				{Key: "workspace_id", Value: "ws-other"},
				{Key: "member_id", Value: "member-1"},
			},
			body: `{"role":"normal"}`,
			call: (*OrganizationHandler).UpdateOrganizationWorkspaceMemberRole,
		},
		{
			name:   "batch add workspace members",
			method: http.MethodPost,
			target: "/organizations/org-1/workspaces/ws-other/members/batch-add",
			params: gin.Params{
				{Key: "organization_id", Value: "org-1"},
				{Key: "workspace_id", Value: "ws-other"},
			},
			body: `{"account_ids":["member-1"],"role":"normal"}`,
			call: (*OrganizationHandler).BatchAddOrganizationMembersToWorkspace,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			permissionChecked := false
			workspaceAssetsChecked := false
			workspaceDetailLoaded := false
			accountLoaded := false
			workspaceLoaded := false
			handler := &OrganizationHandler{
				organizationService: fakeOrganizationService{
					getOrganizationByWorkspaceIDFn: func(ctx context.Context, workspaceID string) (*model.Organization, error) {
						require.Equal(t, "ws-other", workspaceID)
						return &model.Organization{ID: "org-other", Status: model.OrganizationStatusActive}, nil
					},
					checkWorkspacePermissionFn: func(ctx context.Context, organizationID, workspaceID, accountID string, permissionCode model.WorkspacePermissionCode) (bool, error) {
						permissionChecked = true
						return true, nil
					},
					checkWorkspaceAssetsFn: func(ctx context.Context, workspaceID string) (bool, map[string]int64, error) {
						workspaceAssetsChecked = true
						return false, nil, nil
					},
					getOrganizationWorkspaceDetailFn: func(ctx context.Context, organizationID, workspaceID, accountID string) (*shared_dto.OrganizationWorkspaceResponse, error) {
						workspaceDetailLoaded = true
						return nil, nil
					},
				},
				workspaceManagementService: fakeWorkspaceManagementService{
					getWorkspaceByIDFn: func(ctx context.Context, id string) (*model.Workspace, error) {
						workspaceLoaded = true
						return &model.Workspace{ID: id}, nil
					},
				},
				accountService: fakeAccountService{
					getAccountByIDFn: func(ctx context.Context, id string) (*auth_model.Account, error) {
						accountLoaded = true
						return &auth_model.Account{ID: id}, nil
					},
				},
			}

			c, recorder := newOrganizationHandlerTestContext(tt.method, tt.target)
			c.Set("account_id", "operator-1")
			c.Set("organization_id", "org-1")
			c.Set("tenant_id", "org-1")
			c.Params = tt.params
			if tt.body != "" {
				c.Request.Body = io.NopCloser(bytes.NewBufferString(tt.body))
				c.Request.Header.Set("Content-Type", "application/json")
			}

			tt.call(handler, c)

			require.Equal(t, http.StatusBadRequest, recorder.Code, recorder.Body.String())
			var resp response.Response
			require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &resp), recorder.Body.String())
			require.Equal(t, strconv.Itoa(response.ErrWorkspaceNotInOrganization.Code), resp.Code, recorder.Body.String())
			require.False(t, permissionChecked)
			require.False(t, workspaceAssetsChecked)
			require.False(t, workspaceDetailLoaded)
			require.False(t, accountLoaded)
			require.False(t, workspaceLoaded)
		})
	}
}

func TestGetOrganizationWorkspaceMemberDetailByIDReturnsHasMobile(t *testing.T) {
	t.Parallel()

	var requestedPermission model.WorkspacePermissionCode
	handler := &OrganizationHandler{
		organizationService: fakeOrganizationService{
			checkWorkspacePermissionFn: func(ctx context.Context, organizationID, workspaceID, accountID string, permissionCode model.WorkspacePermissionCode) (bool, error) {
				require.Equal(t, "org-1", organizationID)
				require.Equal(t, "ws-1", workspaceID)
				require.Equal(t, "acc-1", accountID)
				requestedPermission = permissionCode
				return true, nil
			},
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
	require.Equal(t, model.WorkspacePermissionWorkspaceMemberView, requestedPermission)
	require.Contains(t, recorder.Body.String(), `"has_mobile":true`)
}

func TestGetOrganizationWorkspaceMembersRequiresMemberViewPermission(t *testing.T) {
	t.Parallel()

	var requestedPermission model.WorkspacePermissionCode
	membersLoaded := false
	handler := &OrganizationHandler{
		organizationService: fakeOrganizationService{
			checkWorkspacePermissionFn: func(ctx context.Context, organizationID, workspaceID, accountID string, permissionCode model.WorkspacePermissionCode) (bool, error) {
				require.Equal(t, "org-1", organizationID)
				require.Equal(t, "ws-1", workspaceID)
				require.Equal(t, "acc-1", accountID)
				requestedPermission = permissionCode
				return false, nil
			},
		},
		workspaceManagementService: fakeWorkspaceManagementService{
			getWorkspaceMembersPaginatedFn: func(ctx context.Context, workspaceID string, page, limit int, keyword, roleFilter string) ([]*interfaces.AccountWithRole, int64, error) {
				membersLoaded = true
				return nil, 0, nil
			},
		},
		departmentService: fakeDepartmentService{},
	}

	c, recorder := newOrganizationHandlerTestContext(http.MethodGet, "/organizations/org-1/workspaces/ws-1/members")
	c.Set("account_id", "acc-1")
	c.Set("organization_id", "org-1")
	c.Set("tenant_id", "org-1")
	c.Params = gin.Params{
		{Key: "organization_id", Value: "org-1"},
		{Key: "workspace_id", Value: "ws-1"},
	}

	handler.GetOrganizationWorkspaceMembers(c)

	var resp response.Response
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &resp), recorder.Body.String())
	require.Equal(t, strconv.Itoa(response.ErrPermissionDenied.Code), resp.Code, recorder.Body.String())
	require.Equal(t, model.WorkspacePermissionWorkspaceMemberView, requestedPermission)
	require.False(t, membersLoaded)
}

func TestGetOrganizationWorkspaceMemberOptionsUsesWorkspaceViewAndHidesManagementFields(t *testing.T) {
	t.Parallel()

	var requestedPermission model.WorkspacePermissionCode
	handler := &OrganizationHandler{
		organizationService: fakeOrganizationService{
			checkWorkspacePermissionFn: func(ctx context.Context, organizationID, workspaceID, accountID string, permissionCode model.WorkspacePermissionCode) (bool, error) {
				require.Equal(t, "org-1", organizationID)
				require.Equal(t, "ws-1", workspaceID)
				require.Equal(t, "acc-1", accountID)
				requestedPermission = permissionCode
				return true, nil
			},
		},
		workspaceManagementService: fakeWorkspaceManagementService{
			getWorkspaceMembersPaginatedFn: func(ctx context.Context, workspaceID string, page, limit int, keyword, roleFilter string) ([]*interfaces.AccountWithRole, int64, error) {
				require.Equal(t, "ws-1", workspaceID)
				require.Equal(t, 2, page)
				require.Equal(t, 5, limit)
				require.Equal(t, "ali", keyword)
				require.Empty(t, roleFilter)
				memberName := "Alice Member"
				return []*interfaces.AccountWithRole{
					{
						ID:          "member-1",
						Name:        "Alice",
						AccountName: "alice",
						MemberName:  &memberName,
						Email:       "alice@example.com",
						Role:        "admin",
						Permissions: []string{"workspace.permission.manage"},
						Status:      "active",
						HasMobile:   true,
					},
				}, 6, nil
			},
		},
		departmentService: fakeDepartmentService{},
	}

	c, recorder := newOrganizationHandlerTestContext(http.MethodGet, "/organizations/org-1/workspaces/ws-1/member-options?page=2&limit=5&keyword=ali")
	c.Set("account_id", "acc-1")
	c.Set("organization_id", "org-1")
	c.Set("tenant_id", "org-1")
	c.Params = gin.Params{
		{Key: "organization_id", Value: "org-1"},
		{Key: "workspace_id", Value: "ws-1"},
	}

	handler.GetOrganizationWorkspaceMemberOptions(c)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	require.Equal(t, model.WorkspacePermissionWorkspaceView, requestedPermission)
	require.Contains(t, recorder.Body.String(), `"email":"alice@example.com"`)
	require.Contains(t, recorder.Body.String(), `"has_mobile":true`)
	require.NotContains(t, recorder.Body.String(), `"permissions"`)
	require.NotContains(t, recorder.Body.String(), `"role"`)
}

func TestGetOrganizationWorkspaceMemberOptionDetailUsesWorkspaceViewAndHidesManagementFields(t *testing.T) {
	t.Parallel()

	var requestedPermission model.WorkspacePermissionCode
	handler := &OrganizationHandler{
		organizationService: fakeOrganizationService{
			checkWorkspacePermissionFn: func(ctx context.Context, organizationID, workspaceID, accountID string, permissionCode model.WorkspacePermissionCode) (bool, error) {
				require.Equal(t, "org-1", organizationID)
				require.Equal(t, "ws-1", workspaceID)
				require.Equal(t, "acc-1", accountID)
				requestedPermission = permissionCode
				return true, nil
			},
		},
		workspaceManagementService: fakeWorkspaceManagementService{
			getWorkspaceMembersFn: func(ctx context.Context, workspaceID string) ([]*interfaces.AccountWithRole, error) {
				require.Equal(t, "ws-1", workspaceID)
				memberName := "Alice Member"
				return []*interfaces.AccountWithRole{
					{
						ID:          "member-1",
						Name:        "Alice",
						AccountName: "alice",
						MemberName:  &memberName,
						Email:       "alice@example.com",
						Role:        "admin",
						Permissions: []string{"workspace.permission.manage"},
						Status:      "active",
						HasMobile:   true,
					},
				}, nil
			},
		},
		departmentService: fakeDepartmentService{},
	}

	c, recorder := newOrganizationHandlerTestContext(http.MethodGet, "/organizations/org-1/workspaces/ws-1/member-options/member-1")
	c.Set("account_id", "acc-1")
	c.Set("organization_id", "org-1")
	c.Set("tenant_id", "org-1")
	c.Params = gin.Params{
		{Key: "organization_id", Value: "org-1"},
		{Key: "workspace_id", Value: "ws-1"},
		{Key: "member_id", Value: "member-1"},
	}

	handler.GetOrganizationWorkspaceMemberOptionDetailByID(c)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	require.Equal(t, model.WorkspacePermissionWorkspaceView, requestedPermission)
	require.Contains(t, recorder.Body.String(), `"email":"alice@example.com"`)
	require.Contains(t, recorder.Body.String(), `"has_mobile":true`)
	require.NotContains(t, recorder.Body.String(), `"permissions"`)
	require.NotContains(t, recorder.Body.String(), `"role"`)
}

func TestGetOrganizationWorkspaceAssetsRequiresMemberManagePermission(t *testing.T) {
	t.Parallel()

	var requestedPermission model.WorkspacePermissionCode
	assetsChecked := false
	handler := &OrganizationHandler{
		organizationService: fakeOrganizationService{
			checkWorkspacePermissionFn: func(ctx context.Context, organizationID, workspaceID, accountID string, permissionCode model.WorkspacePermissionCode) (bool, error) {
				require.Equal(t, "org-1", organizationID)
				require.Equal(t, "ws-1", workspaceID)
				require.Equal(t, "acc-1", accountID)
				requestedPermission = permissionCode
				return false, nil
			},
			checkWorkspaceAssetsFn: func(ctx context.Context, workspaceID string) (bool, map[string]int64, error) {
				assetsChecked = true
				return true, map[string]int64{"agents": 1}, nil
			},
		},
	}

	c, recorder := newOrganizationHandlerTestContext(http.MethodGet, "/organizations/org-1/workspaces/ws-1/assets")
	c.Set("account_id", "acc-1")
	c.Set("organization_id", "org-1")
	c.Set("tenant_id", "org-1")
	c.Params = gin.Params{
		{Key: "organization_id", Value: "org-1"},
		{Key: "workspace_id", Value: "ws-1"},
	}

	handler.GetOrganizationWorkspaceAssets(c)

	var resp response.Response
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &resp), recorder.Body.String())
	require.Equal(t, strconv.Itoa(response.ErrPermissionDenied.Code), resp.Code, recorder.Body.String())
	require.Equal(t, model.WorkspacePermissionWorkspaceMemberManage, requestedPermission)
	require.False(t, assetsChecked)
}

func TestPatchOrganizationReturnsUpdatedOrganization(t *testing.T) {
	t.Parallel()

	shortName := "Acme"
	handler := &OrganizationHandler{
		organizationService: fakeOrganizationService{
			isOrganizationAdminOrOwnerFn: func(ctx context.Context, organizationID, accountID string) (bool, error) {
				require.Equal(t, "org-1", organizationID)
				require.Equal(t, "acc-1", accountID)
				return true, nil
			},
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
					isOrganizationAdminOrOwnerFn: func(ctx context.Context, organizationID, accountID string) (bool, error) {
						require.Equal(t, "org-1", organizationID)
						require.Equal(t, "acc-1", accountID)
						return true, nil
					},
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
				t.Fatalf("current organization members should use visible member pagination")
				return nil, nil
			},
			getVisibleMembersPaginatedFn: func(ctx context.Context, organizationID, accountID string, page, limit int, keyword string) (*shared_dto.OrganizationMemberPaginationResponse, error) {
				require.Equal(t, "org-1", organizationID)
				require.Equal(t, "owner-1", accountID)
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

func TestGetCurrentOrganizationMembersAllowsOrganizationAdminWithoutManagedWorkspace(t *testing.T) {
	t.Parallel()

	handler := &OrganizationHandler{
		organizationService: fakeOrganizationService{
			isOrganizationAdminOrOwnerFn: func(ctx context.Context, organizationID, accountID string) (bool, error) {
				require.Equal(t, "org-1", organizationID)
				require.Equal(t, "admin-1", accountID)
				return true, nil
			},
			checkAnyManagedWorkspaceFn: func(ctx context.Context, organizationID, accountID string) (bool, error) {
				t.Fatalf("organization admin should not need managed workspace permission")
				return false, nil
			},
			getMembersPaginatedFn: func(ctx context.Context, organizationID string, page, limit int, keyword string) (*shared_dto.OrganizationMemberPaginationResponse, error) {
				t.Fatalf("handler should delegate current member visibility to the visible member pagination service")
				return nil, nil
			},
			getVisibleMembersPaginatedFn: func(ctx context.Context, organizationID, accountID string, page, limit int, keyword string) (*shared_dto.OrganizationMemberPaginationResponse, error) {
				require.Equal(t, "org-1", organizationID)
				require.Equal(t, "admin-1", accountID)
				require.Equal(t, 1, page)
				require.Equal(t, 20, limit)
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
					Page:  1,
					Limit: 20,
					Total: 1,
				}, nil
			},
		},
	}

	c, recorder := newOrganizationHandlerTestContext(http.MethodGet, "/organizations/current/members?keyword=alice")
	c.Set("account_id", "admin-1")
	c.Set("organization_id", "org-1")
	c.Set("tenant_id", "org-1")

	handler.GetCurrentOrganizationMembers(c)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Contains(t, recorder.Body.String(), `"email":"alice@example.com"`)
}

func TestGetCurrentOrganizationMembersAllowsPlainOrganizationMemberWithVisibleScope(t *testing.T) {
	t.Parallel()

	handler := &OrganizationHandler{
		organizationService: fakeOrganizationService{
			isOrganizationAdminOrOwnerFn: func(ctx context.Context, organizationID, accountID string) (bool, error) {
				require.Equal(t, "org-1", organizationID)
				require.Equal(t, "member-1", accountID)
				return false, nil
			},
			isOrganizationMemberFn: func(ctx context.Context, organizationID, accountID string) (bool, error) {
				require.Equal(t, "org-1", organizationID)
				require.Equal(t, "member-1", accountID)
				return true, nil
			},
			checkAnyManagedWorkspaceFn: func(ctx context.Context, organizationID, accountID string) (bool, error) {
				t.Fatalf("plain organization members should not need workspace management permission to load their visible member list")
				return false, nil
			},
			getMembersPaginatedFn: func(ctx context.Context, organizationID string, page, limit int, keyword string) (*shared_dto.OrganizationMemberPaginationResponse, error) {
				t.Fatalf("plain organization members must not receive the all-organization member list")
				return nil, nil
			},
			getVisibleMembersPaginatedFn: func(ctx context.Context, organizationID, accountID string, page, limit int, keyword string) (*shared_dto.OrganizationMemberPaginationResponse, error) {
				require.Equal(t, "org-1", organizationID)
				require.Equal(t, "member-1", accountID)
				return &shared_dto.OrganizationMemberPaginationResponse{
					Data: []*shared_dto.OrganizationMemberWithExtensionResponse{
						{
							ID:               "member-1",
							Name:             "Member",
							Email:            "member@example.com",
							Status:           "active",
							OrganizationRole: model.OrganizationRoleNormal,
						},
					},
					Page:  1,
					Limit: 20,
					Total: 1,
				}, nil
			},
		},
	}

	c, recorder := newOrganizationHandlerTestContext(http.MethodGet, "/organizations/current/members")
	c.Set("account_id", "member-1")
	c.Set("organization_id", "org-1")
	c.Set("tenant_id", "org-1")

	handler.GetCurrentOrganizationMembers(c)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Contains(t, recorder.Body.String(), `"email":"member@example.com"`)
}

func TestGetCurrentOrganizationMemberDetailUsesOrganizationScope(t *testing.T) {
	t.Parallel()

	handler := &OrganizationHandler{
		organizationService: fakeOrganizationService{
			isOrganizationMemberFn: func(ctx context.Context, organizationID, accountID string) (bool, error) {
				require.Equal(t, "org-1", organizationID)
				require.Equal(t, "admin-1", accountID)
				return true, nil
			},
			isOrganizationAdminOrOwnerFn: func(ctx context.Context, organizationID, accountID string) (bool, error) {
				require.Equal(t, "org-1", organizationID)
				require.Equal(t, "admin-1", accountID)
				return true, nil
			},
			getMemberByAccountIDFn: func(ctx context.Context, organizationID, accountID string) (*shared_dto.OrganizationMemberWithExtensionResponse, error) {
				require.Equal(t, "org-1", organizationID)
				require.Equal(t, "member-1", accountID)
				memberName := "Team Alice"
				return &shared_dto.OrganizationMemberWithExtensionResponse{
					ID:               "member-1",
					Name:             "Team Alice",
					MemberName:       &memberName,
					Email:            "alice@example.com",
					Status:           "active",
					OrganizationRole: model.OrganizationRoleNormal,
				}, nil
			},
		},
	}

	c, recorder := newOrganizationHandlerTestContext(http.MethodGet, "/organizations/current/members/member-1")
	c.Set("account_id", "admin-1")
	c.Set("organization_id", "org-1")
	c.Set("tenant_id", "org-1")
	c.Params = gin.Params{{Key: "member_id", Value: "member-1"}}

	handler.GetOrganizationMemberDetailByID(c)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Contains(t, recorder.Body.String(), `"email":"alice@example.com"`)
	require.Contains(t, recorder.Body.String(), `"member_name":"Team Alice"`)
}

func TestGetCurrentOrganizationMemberDetailRejectsNormalMemberReadingOtherMember(t *testing.T) {
	t.Parallel()

	handler := &OrganizationHandler{
		organizationService: fakeOrganizationService{
			isOrganizationMemberFn: func(ctx context.Context, organizationID, accountID string) (bool, error) {
				require.Equal(t, "org-1", organizationID)
				require.Equal(t, "viewer-1", accountID)
				return true, nil
			},
			isOrganizationAdminOrOwnerFn: func(ctx context.Context, organizationID, accountID string) (bool, error) {
				require.Equal(t, "org-1", organizationID)
				require.Equal(t, "viewer-1", accountID)
				return false, nil
			},
		},
	}

	c, recorder := newOrganizationHandlerTestContext(http.MethodGet, "/organizations/current/members/member-1")
	c.Set("account_id", "viewer-1")
	c.Set("organization_id", "org-1")
	c.Set("tenant_id", "org-1")
	c.Params = gin.Params{{Key: "member_id", Value: "member-1"}}

	handler.GetOrganizationMemberDetailByID(c)

	require.Equal(t, http.StatusForbidden, recorder.Code)
}

func TestGetJoinedWorkspacesRejectsNormalMemberReadingOtherAccount(t *testing.T) {
	t.Parallel()

	handler := &OrganizationHandler{
		organizationService: fakeOrganizationService{
			isOrganizationAdminOrOwnerFn: func(ctx context.Context, organizationID, accountID string) (bool, error) {
				require.Equal(t, "org-1", organizationID)
				require.Equal(t, "viewer-1", accountID)
				return false, nil
			},
		},
	}

	c, recorder := newOrganizationHandlerTestContext(http.MethodGet, "/organizations/org-1/joined-workspaces/member-1")
	c.Set("account_id", "viewer-1")
	c.Params = gin.Params{
		{Key: "organization_id", Value: "org-1"},
		{Key: "account_id", Value: "member-1"},
	}

	handler.GetJoinedWorkspaces(c)

	require.Equal(t, http.StatusForbidden, recorder.Code)
}

func TestCheckManagePermissionRejectsAccountWithoutOrganizationAccess(t *testing.T) {
	t.Parallel()

	handler := &OrganizationHandler{
		organizationService: fakeOrganizationService{
			isOrganizationMemberFn: func(ctx context.Context, organizationID, accountID string) (bool, error) {
				require.Equal(t, "org-1", organizationID)
				require.Equal(t, "outsider-1", accountID)
				return false, nil
			},
			checkAnyManagedWorkspaceFn: func(ctx context.Context, organizationID, accountID string) (bool, error) {
				require.Equal(t, "org-1", organizationID)
				require.Equal(t, "outsider-1", accountID)
				return false, nil
			},
		},
	}

	c, recorder := newOrganizationHandlerTestContext(http.MethodGet, "/organizations/org-1/check-manage-permission")
	c.Set("account_id", "outsider-1")
	c.Params = gin.Params{{Key: "organization_id", Value: "org-1"}}

	handler.CheckManagePermission(c)

	require.Equal(t, http.StatusForbidden, recorder.Code)
}

func TestDirectAddMemberMapsDepartmentConflict(t *testing.T) {
	t.Parallel()

	handler := &OrganizationHandler{
		organizationService: fakeOrganizationService{
			isOrganizationAdminOrOwnerFn: func(ctx context.Context, organizationID, accountID string) (bool, error) {
				require.Equal(t, "org-1", organizationID)
				require.Equal(t, "owner-1", accountID)
				return true, nil
			},
			directAddMemberFn: func(ctx context.Context, req *shared_dto.DirectAddOrganizationMemberRequest) (*shared_dto.DirectAddOrganizationMemberResponse, error) {
				require.Equal(t, "org-1", req.OrganizationID)
				require.Equal(t, "owner-1", req.OperatorAccountID)
				require.Equal(t, "ws-1", req.WorkspaceID)
				require.Equal(t, "alice@example.com", req.Email)
				require.Equal(t, "Alice", req.Name)
				require.NotNil(t, req.DepartmentID)
				require.Equal(t, "dept-1", *req.DepartmentID)
				return nil, &workspace_service.MemberAlreadyInDepartmentError{
					CurrentDepartment: &model.Department{ID: "dept-existing", Name: "Current Department"},
				}
			},
		},
	}

	c, recorder := newOrganizationHandlerTestContext(http.MethodPost, "/organizations/org-1/members/direct-add")
	c.Set("account_id", "owner-1")
	c.Set("organization_id", "org-1")
	c.Params = gin.Params{{Key: "organization_id", Value: "org-1"}}
	c.Request.Body = io.NopCloser(bytes.NewBufferString(`{"name":"Alice","email":"alice@example.com","workspace_id":"ws-1","department_id":"dept-1"}`))
	c.Request.Header.Set("Content-Type", "application/json")

	handler.DirectAddMember(c)

	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.Contains(t, recorder.Body.String(), `"code":"MemberAlreadyInDepartment"`)
	require.Contains(t, recorder.Body.String(), `"id":"dept-existing"`)
}

func TestDirectAddMemberAllowsMissingWorkspace(t *testing.T) {
	t.Parallel()

	serviceCalled := false
	handler := &OrganizationHandler{
		organizationService: fakeOrganizationService{
			isOrganizationAdminOrOwnerFn: func(ctx context.Context, organizationID, accountID string) (bool, error) {
				require.Equal(t, "org-1", organizationID)
				require.Equal(t, "owner-1", accountID)
				return true, nil
			},
			directAddMemberFn: func(ctx context.Context, req *shared_dto.DirectAddOrganizationMemberRequest) (*shared_dto.DirectAddOrganizationMemberResponse, error) {
				serviceCalled = true
				require.Equal(t, "org-1", req.OrganizationID)
				require.Equal(t, "owner-1", req.OperatorAccountID)
				require.Empty(t, req.WorkspaceID)
				require.Equal(t, "alice@example.com", req.Email)
				require.Equal(t, "Alice", req.Name)
				return &shared_dto.DirectAddOrganizationMemberResponse{
					AccountID:      "member-1",
					Email:          "alice@example.com",
					Name:           "Alice",
					OrganizationID: "org-1",
				}, nil
			},
		},
	}

	c, recorder := newOrganizationHandlerTestContext(http.MethodPost, "/organizations/org-1/members/direct-add")
	c.Set("account_id", "owner-1")
	c.Set("organization_id", "org-1")
	c.Params = gin.Params{{Key: "organization_id", Value: "org-1"}}
	c.Request.Body = io.NopCloser(bytes.NewBufferString(`{"name":"Alice","email":"alice@example.com"}`))
	c.Request.Header.Set("Content-Type", "application/json")

	handler.DirectAddMember(c)

	require.True(t, serviceCalled)
	require.Equal(t, http.StatusOK, recorder.Code)
	require.NotContains(t, recorder.Body.String(), `"workspace"`)
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

func TestListWorkspacePermissionsRejectsAccountWithoutManagedWorkspaceAccess(t *testing.T) {
	t.Parallel()

	serviceCalled := false
	handler := &OrganizationHandler{
		organizationService: fakeOrganizationService{
			isOrganizationMemberFn: func(ctx context.Context, organizationID, accountID string) (bool, error) {
				t.Fatalf("permission definition list should use manager access instead of plain organization membership")
				return true, nil
			},
			checkAnyManagedWorkspaceFn: func(ctx context.Context, organizationID, accountID string) (bool, error) {
				require.Equal(t, "org-1", organizationID)
				require.Equal(t, "ordinary-member", accountID)
				return false, nil
			},
			listWorkspacePermissionDefsFn: func(ctx context.Context, organizationID, accountID string) ([]string, error) {
				serviceCalled = true
				return []string{string(model.WorkspacePermissionAgentView)}, nil
			},
		},
	}

	c, recorder := newOrganizationHandlerTestContext(http.MethodGet, "/organizations/org-1/permissions")
	c.Set("account_id", "ordinary-member")
	c.Params = gin.Params{{Key: "organization_id", Value: "org-1"}}

	handler.ListWorkspacePermissions(c)

	require.False(t, serviceCalled)
	require.Equal(t, http.StatusForbidden, recorder.Code)
}

func TestListWorkspacePermissionsAllowsWorkspaceManagerAccess(t *testing.T) {
	t.Parallel()

	serviceCalled := false
	handler := &OrganizationHandler{
		organizationService: fakeOrganizationService{
			isOrganizationMemberFn: func(ctx context.Context, organizationID, accountID string) (bool, error) {
				t.Fatalf("permission definition list should use manager access instead of plain organization membership")
				return false, nil
			},
			checkAnyManagedWorkspaceFn: func(ctx context.Context, organizationID, accountID string) (bool, error) {
				require.Equal(t, "org-1", organizationID)
				require.Equal(t, "workspace-manager", accountID)
				return true, nil
			},
			listWorkspacePermissionDefsFn: func(ctx context.Context, organizationID, accountID string) ([]string, error) {
				serviceCalled = true
				require.Equal(t, "org-1", organizationID)
				require.Equal(t, "workspace-manager", accountID)
				return []string{string(model.WorkspacePermissionAgentView)}, nil
			},
		},
	}

	c, recorder := newOrganizationHandlerTestContext(http.MethodGet, "/organizations/org-1/permissions")
	c.Set("account_id", "workspace-manager")
	c.Params = gin.Params{{Key: "organization_id", Value: "org-1"}}

	handler.ListWorkspacePermissions(c)

	require.True(t, serviceCalled)
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Contains(t, recorder.Body.String(), string(model.WorkspacePermissionAgentView))
}

func TestListWorkspaceRolesRejectsAccountWithoutManagedWorkspaceAccess(t *testing.T) {
	t.Parallel()

	serviceCalled := false
	handler := &OrganizationHandler{
		organizationService: fakeOrganizationService{
			isOrganizationMemberFn: func(ctx context.Context, organizationID, accountID string) (bool, error) {
				t.Fatalf("role template list should use manager access instead of plain organization membership")
				return false, nil
			},
			checkAnyManagedWorkspaceFn: func(ctx context.Context, organizationID, accountID string) (bool, error) {
				require.Equal(t, "org-2", organizationID)
				require.Equal(t, "account-1", accountID)
				return false, nil
			},
			listWorkspaceRolesFn: func(ctx context.Context, organizationID, accountID string, includeOwner bool) (*shared_dto.WorkspaceRoleListResponse, error) {
				serviceCalled = true
				return &shared_dto.WorkspaceRoleListResponse{}, nil
			},
		},
	}

	c, recorder := newOrganizationHandlerTestContext(http.MethodGet, "/organizations/org-2/roles")
	c.Set("account_id", "account-1")
	c.Params = gin.Params{{Key: "organization_id", Value: "org-2"}}

	handler.ListWorkspaceRoles(c)

	require.False(t, serviceCalled)
	require.Equal(t, http.StatusForbidden, recorder.Code)
	var resp response.Response
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &resp))
	require.Equal(t, strconv.Itoa(response.ErrPermissionDenied.Code), resp.Code)
}

func TestListWorkspaceRolesAllowsWorkspaceManagerAccess(t *testing.T) {
	t.Parallel()

	serviceCalled := false
	handler := &OrganizationHandler{
		organizationService: fakeOrganizationService{
			isOrganizationMemberFn: func(ctx context.Context, organizationID, accountID string) (bool, error) {
				t.Fatalf("role template list should use manager access instead of plain organization membership")
				return false, nil
			},
			checkAnyManagedWorkspaceFn: func(ctx context.Context, organizationID, accountID string) (bool, error) {
				require.Equal(t, "org-1", organizationID)
				require.Equal(t, "workspace-manager", accountID)
				return true, nil
			},
			listWorkspaceRolesFn: func(ctx context.Context, organizationID, accountID string, includeOwner bool) (*shared_dto.WorkspaceRoleListResponse, error) {
				serviceCalled = true
				require.Equal(t, "org-1", organizationID)
				require.Equal(t, "workspace-manager", accountID)
				require.True(t, includeOwner)
				return &shared_dto.WorkspaceRoleListResponse{Roles: []shared_dto.WorkspaceRoleSummary{{ID: model.WorkspaceBuiltinRoleAdminID}}}, nil
			},
		},
	}

	c, recorder := newOrganizationHandlerTestContext(http.MethodGet, "/organizations/org-1/roles?include_owner=true")
	c.Set("account_id", "workspace-manager")
	c.Params = gin.Params{{Key: "organization_id", Value: "org-1"}}
	c.Request.URL.RawQuery = "include_owner=true"

	handler.ListWorkspaceRoles(c)

	require.True(t, serviceCalled)
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Contains(t, recorder.Body.String(), model.WorkspaceBuiltinRoleAdminID)
}

func TestGetWorkspaceRoleRejectsAccountWithoutManagedWorkspaceAccess(t *testing.T) {
	t.Parallel()

	serviceCalled := false
	handler := &OrganizationHandler{
		organizationService: fakeOrganizationService{
			isOrganizationMemberFn: func(ctx context.Context, organizationID, accountID string) (bool, error) {
				t.Fatalf("role detail should use manager access instead of plain organization membership")
				return true, nil
			},
			checkAnyManagedWorkspaceFn: func(ctx context.Context, organizationID, accountID string) (bool, error) {
				require.Equal(t, "org-1", organizationID)
				require.Equal(t, "ordinary-member", accountID)
				return false, nil
			},
			getWorkspaceRoleDetailFn: func(ctx context.Context, organizationID, roleID, accountID string) (*shared_dto.OrganizationRoleDetailResponse, error) {
				serviceCalled = true
				return &shared_dto.OrganizationRoleDetailResponse{}, nil
			},
		},
	}

	c, recorder := newOrganizationHandlerTestContext(http.MethodGet, "/organizations/org-1/roles/role-1")
	c.Set("account_id", "ordinary-member")
	c.Params = gin.Params{
		{Key: "organization_id", Value: "org-1"},
		{Key: "role_id", Value: "role-1"},
	}

	handler.GetWorkspaceRole(c)

	require.False(t, serviceCalled)
	require.Equal(t, http.StatusForbidden, recorder.Code)
}

func TestGetWorkspaceRoleAllowsWorkspaceManagerAccess(t *testing.T) {
	t.Parallel()

	serviceCalled := false
	handler := &OrganizationHandler{
		organizationService: fakeOrganizationService{
			isOrganizationMemberFn: func(ctx context.Context, organizationID, accountID string) (bool, error) {
				t.Fatalf("role detail should use manager access instead of plain organization membership")
				return false, nil
			},
			checkAnyManagedWorkspaceFn: func(ctx context.Context, organizationID, accountID string) (bool, error) {
				require.Equal(t, "org-1", organizationID)
				require.Equal(t, "workspace-manager", accountID)
				return true, nil
			},
			getWorkspaceRoleDetailFn: func(ctx context.Context, organizationID, roleID, accountID string) (*shared_dto.OrganizationRoleDetailResponse, error) {
				serviceCalled = true
				require.Equal(t, "org-1", organizationID)
				require.Equal(t, "role-1", roleID)
				require.Equal(t, "workspace-manager", accountID)
				return &shared_dto.OrganizationRoleDetailResponse{ID: roleID, OrganizationID: organizationID}, nil
			},
		},
	}

	c, recorder := newOrganizationHandlerTestContext(http.MethodGet, "/organizations/org-1/roles/role-1")
	c.Set("account_id", "workspace-manager")
	c.Params = gin.Params{
		{Key: "organization_id", Value: "org-1"},
		{Key: "role_id", Value: "role-1"},
	}

	handler.GetWorkspaceRole(c)

	require.True(t, serviceCalled)
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Contains(t, recorder.Body.String(), `"id":"role-1"`)
}

func TestApplyWorkspaceRoleTemplateBindsTargetsAndOperator(t *testing.T) {
	t.Parallel()

	serviceCalled := false
	handler := &OrganizationHandler{
		organizationService: fakeOrganizationService{
			isOrganizationAdminOrOwnerFn: func(ctx context.Context, organizationID, accountID string) (bool, error) {
				t.Fatalf("ApplyWorkspaceRoleTemplate should delegate workspace-manager checks to the service layer")
				return false, nil
			},
			applyWorkspaceRoleTemplateFn: func(ctx context.Context, req *shared_dto.ApplyWorkspaceRoleTemplateRequest) (*shared_dto.ApplyWorkspaceRoleTemplateResponse, error) {
				serviceCalled = true
				require.Equal(t, "org-1", req.OrganizationID)
				require.Equal(t, model.WorkspaceBuiltinRoleMemberID, req.RoleID)
				require.Equal(t, "workspace-manager", req.OperatorID)
				require.Len(t, req.Members, 2)
				require.Equal(t, "ws-1", req.Members[0].WorkspaceID)
				require.Equal(t, "member-1", req.Members[0].AccountID)
				require.Equal(t, "ws-2", req.Members[1].WorkspaceID)
				require.Equal(t, "member-1", req.Members[1].AccountID)
				return &shared_dto.ApplyWorkspaceRoleTemplateResponse{AppliedCount: 2}, nil
			},
		},
	}

	c, recorder := newOrganizationHandlerTestContext(http.MethodPost, "/organizations/org-1/roles/00000000-0000-0000-0000-000000000003/apply-template")
	c.Set("account_id", "workspace-manager")
	c.Params = gin.Params{
		{Key: "organization_id", Value: "org-1"},
		{Key: "role_id", Value: model.WorkspaceBuiltinRoleMemberID},
	}
	c.Request.Body = io.NopCloser(bytes.NewBufferString(`{"members":[{"workspace_id":"ws-1","account_id":"member-1"},{"workspace_id":"ws-2","account_id":"member-1"}]}`))
	c.Request.Header.Set("Content-Type", "application/json")

	handler.ApplyWorkspaceRoleTemplate(c)

	require.True(t, serviceCalled)
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Contains(t, recorder.Body.String(), `"applied_count":2`)
}

func TestReplaceAndDeleteWorkspaceRoleRequiresOrganizationAdminAndBindsReplacement(t *testing.T) {
	t.Parallel()

	serviceCalled := false
	handler := &OrganizationHandler{
		organizationService: fakeOrganizationService{
			isOrganizationAdminOrOwnerFn: func(ctx context.Context, organizationID, accountID string) (bool, error) {
				require.Equal(t, "org-1", organizationID)
				require.Equal(t, "org-admin", accountID)
				return true, nil
			},
			replaceAndDeleteRoleFn: func(ctx context.Context, req *shared_dto.ReplaceWorkspaceRoleTemplateRequest) (*shared_dto.ReplaceWorkspaceRoleTemplateResponse, error) {
				serviceCalled = true
				require.Equal(t, "org-1", req.OrganizationID)
				require.Equal(t, "old-role", req.RoleID)
				require.Equal(t, "new-role", req.ReplacementRoleID)
				require.Equal(t, "org-admin", req.OperatorID)
				return &shared_dto.ReplaceWorkspaceRoleTemplateResponse{ReplacedCount: 3, Deleted: true}, nil
			},
		},
	}

	c, recorder := newOrganizationHandlerTestContext(http.MethodPost, "/organizations/org-1/roles/old-role/replace-and-delete")
	c.Set("account_id", "org-admin")
	c.Params = gin.Params{
		{Key: "organization_id", Value: "org-1"},
		{Key: "role_id", Value: "old-role"},
	}
	c.Request.Body = io.NopCloser(bytes.NewBufferString(`{"replacement_role_id":"new-role"}`))
	c.Request.Header.Set("Content-Type", "application/json")

	handler.ReplaceAndDeleteWorkspaceRole(c)

	require.True(t, serviceCalled)
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Contains(t, recorder.Body.String(), `"replaced_count":3`)
	require.Contains(t, recorder.Body.String(), `"deleted":true`)
}

func TestGetOrganizationMembersRejectsNonAdminWorkspaceManager(t *testing.T) {
	t.Parallel()

	listCalled := false
	handler := &OrganizationHandler{
		organizationService: fakeOrganizationService{
			isOrganizationAdminOrOwnerFn: func(ctx context.Context, organizationID, accountID string) (bool, error) {
				require.Equal(t, "org-1", organizationID)
				require.Equal(t, "workspace-manager", accountID)
				return false, nil
			},
			checkAnyManagedWorkspaceFn: func(ctx context.Context, organizationID, accountID string) (bool, error) {
				return true, nil
			},
			getMembersPaginatedFn: func(ctx context.Context, organizationID string, page, limit int, keyword string) (*shared_dto.OrganizationMemberPaginationResponse, error) {
				listCalled = true
				return &shared_dto.OrganizationMemberPaginationResponse{}, nil
			},
		},
		departmentService: fakeDepartmentService{},
	}

	c, recorder := newOrganizationHandlerTestContext(http.MethodGet, "/organizations/org-1/members")
	c.Set("account_id", "workspace-manager")
	c.Params = gin.Params{{Key: "organization_id", Value: "org-1"}}

	handler.GetOrganizationMembers(c)

	require.False(t, listCalled)
	require.Equal(t, http.StatusForbidden, recorder.Code)
}

func TestOrganizationRoutesRegisterCurrentMemberDetail(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	router := gin.New()
	handler := &OrganizationHandler{organizationService: fakeOrganizationService{}}

	handler.RegisterRoutes(router.Group(""))

	for _, route := range router.Routes() {
		if route.Method == http.MethodGet && route.Path == "/organizations/current/members/:member_id" {
			return
		}
	}

	t.Fatalf("GET /organizations/current/members/:member_id route was not registered")
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
