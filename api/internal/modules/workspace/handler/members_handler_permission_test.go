package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	auth_model "github.com/zgiai/zgi/api/internal/modules/user/auth/model"
	"github.com/zgiai/zgi/api/internal/modules/workspace/model"
	"github.com/zgiai/zgi/api/pkg/response"
)

func TestMembersHandlerCurrentMembersUsesCurrentWorkspaceOrganizationForPermission(t *testing.T) {
	gin.SetMode(gin.TestMode)

	workspaceID := "workspace-current"
	organizationID := "org-from-workspace"
	accountID := "account-1"
	workspaceSvc := &membersHandlerWorkspaceManagementService{
		currentWorkspace: &model.WorkspaceMember{WorkspaceID: workspaceID},
	}
	organizationSvc := &membersHandlerOrganizationService{
		workspaceOrganizationID:         organizationID,
		checkWorkspacePermissionAllowed: false,
	}
	handler := NewMembersHandler(workspaceSvc, &membersHandlerAccountService{}, organizationSvc, "")

	c, recorder := newMembersHandlerContext(http.MethodGet, "/workspaces/current/members", accountID)

	handler.GetCurrentOrganizationMembers(c)

	require.Equal(t, http.StatusForbidden, recorder.Code, recorder.Body.String())
	requireWorkspaceHandlerResponseCode(t, recorder, response.ErrPermissionDenied)
	require.True(t, workspaceSvc.currentWorkspaceCalled)
	require.False(t, workspaceSvc.membersPaginatedCalled)
	require.Equal(t, workspaceID, organizationSvc.lastWorkspaceIDForOrganization)
	require.Equal(t, organizationID, organizationSvc.lastPermissionOrganizationID)
	require.Equal(t, workspaceID, organizationSvc.lastPermissionWorkspaceID)
	require.Equal(t, accountID, organizationSvc.lastPermissionAccountID)
	require.Equal(t, model.WorkspacePermissionWorkspaceMemberView, organizationSvc.lastPermissionCode)
}

func TestMembersHandlerCurrentMemberDetailRequiresMemberViewBeforeLookup(t *testing.T) {
	gin.SetMode(gin.TestMode)

	workspaceID := "workspace-current"
	organizationID := "org-from-workspace"
	accountID := "account-1"
	memberID := "member-1"
	workspaceSvc := &membersHandlerWorkspaceManagementService{
		currentWorkspace: &model.WorkspaceMember{WorkspaceID: workspaceID},
	}
	organizationSvc := &membersHandlerOrganizationService{
		workspaceOrganizationID:         organizationID,
		checkWorkspacePermissionAllowed: false,
	}
	handler := NewMembersHandler(workspaceSvc, &membersHandlerAccountService{}, organizationSvc, "")

	c, recorder := newMembersHandlerContext(http.MethodGet, "/workspaces/current/members/"+memberID, accountID)
	c.Params = gin.Params{{Key: "member_id", Value: memberID}}

	handler.GetCurrentOrganizationMemberDetail(c)

	require.Equal(t, http.StatusForbidden, recorder.Code, recorder.Body.String())
	requireWorkspaceHandlerResponseCode(t, recorder, response.ErrPermissionDenied)
	require.True(t, workspaceSvc.currentWorkspaceCalled)
	require.False(t, workspaceSvc.memberWithExtensionsByIDCalled)
	require.Equal(t, workspaceID, organizationSvc.lastWorkspaceIDForOrganization)
	require.Equal(t, organizationID, organizationSvc.lastPermissionOrganizationID)
	require.Equal(t, workspaceID, organizationSvc.lastPermissionWorkspaceID)
	require.Equal(t, accountID, organizationSvc.lastPermissionAccountID)
	require.Equal(t, model.WorkspacePermissionWorkspaceMemberView, organizationSvc.lastPermissionCode)
}

func TestMembersHandlerCurrentDatasetOperatorsRequiresWorkspaceViewBeforeLookup(t *testing.T) {
	gin.SetMode(gin.TestMode)

	workspaceID := "workspace-current"
	organizationID := "org-from-workspace"
	accountID := "account-1"
	workspaceSvc := &membersHandlerWorkspaceManagementService{
		currentWorkspace: &model.WorkspaceMember{WorkspaceID: workspaceID},
	}
	organizationSvc := &membersHandlerOrganizationService{
		workspaceOrganizationID:         organizationID,
		checkWorkspacePermissionAllowed: false,
	}
	handler := NewMembersHandler(workspaceSvc, &membersHandlerAccountService{}, organizationSvc, "")

	c, recorder := newMembersHandlerContext(http.MethodGet, "/workspaces/current/dataset-operators", accountID)

	handler.GetDatasetOperatorMembers(c)

	require.Equal(t, http.StatusForbidden, recorder.Code, recorder.Body.String())
	requireWorkspaceHandlerResponseCode(t, recorder, response.ErrPermissionDenied)
	require.True(t, workspaceSvc.currentWorkspaceCalled)
	require.False(t, workspaceSvc.datasetOperatorMembersCalled)
	require.Equal(t, workspaceID, organizationSvc.lastWorkspaceIDForOrganization)
	require.Equal(t, organizationID, organizationSvc.lastPermissionOrganizationID)
	require.Equal(t, workspaceID, organizationSvc.lastPermissionWorkspaceID)
	require.Equal(t, accountID, organizationSvc.lastPermissionAccountID)
	require.Equal(t, model.WorkspacePermissionWorkspaceView, organizationSvc.lastPermissionCode)
}

func TestMembersHandlerWorkspaceMembersExtensionUsesRouteWorkspaceOrganizationForPermission(t *testing.T) {
	gin.SetMode(gin.TestMode)

	workspaceID := "workspace-route"
	organizationID := "org-from-workspace"
	accountID := "account-1"
	workspaceSvc := &membersHandlerWorkspaceManagementService{}
	organizationSvc := &membersHandlerOrganizationService{
		workspaceOrganizationID:         organizationID,
		checkWorkspacePermissionAllowed: false,
	}
	handler := NewMembersHandler(workspaceSvc, &membersHandlerAccountService{}, organizationSvc, "")

	c, recorder := newMembersHandlerContext(http.MethodGet, "/workspaces/"+workspaceID+"/members", accountID)
	c.Params = gin.Params{{Key: "workspace_id", Value: workspaceID}}

	handler.GetWorkspaceMembersExtension(c)

	require.Equal(t, http.StatusForbidden, recorder.Code, recorder.Body.String())
	requireWorkspaceHandlerResponseCode(t, recorder, response.ErrPermissionDenied)
	require.False(t, workspaceSvc.membersWithExtensionsCalled)
	require.Equal(t, workspaceID, organizationSvc.lastWorkspaceIDForOrganization)
	require.Equal(t, organizationID, organizationSvc.lastPermissionOrganizationID)
	require.Equal(t, workspaceID, organizationSvc.lastPermissionWorkspaceID)
	require.Equal(t, accountID, organizationSvc.lastPermissionAccountID)
	require.Equal(t, model.WorkspacePermissionWorkspaceMemberView, organizationSvc.lastPermissionCode)
}

func TestMembersHandlerCancelWorkspaceInviteRequiresManageBeforeMemberLookup(t *testing.T) {
	gin.SetMode(gin.TestMode)

	workspaceID := "workspace-route"
	organizationID := "org-from-workspace"
	accountID := "account-1"
	memberID := "member-1"
	workspaceSvc := &membersHandlerWorkspaceManagementService{}
	accountSvc := &membersHandlerAccountService{}
	organizationSvc := &membersHandlerOrganizationService{
		workspaceOrganizationID:         organizationID,
		checkWorkspacePermissionAllowed: false,
	}
	handler := NewMembersHandler(workspaceSvc, accountSvc, organizationSvc, "")

	c, recorder := newMembersHandlerContext(http.MethodDelete, "/workspaces/"+workspaceID+"/members/"+memberID, accountID)
	c.Params = gin.Params{
		{Key: "workspace_id", Value: workspaceID},
		{Key: "member_id", Value: memberID},
	}

	handler.CancelWorkspaceMemberInvite(c)

	require.Equal(t, http.StatusForbidden, recorder.Code, recorder.Body.String())
	requireWorkspaceHandlerResponseCode(t, recorder, response.ErrPermissionDenied)
	require.False(t, accountSvc.getAccountByIDCalled)
	require.False(t, workspaceSvc.removeMemberFromWorkspaceCalled)
	require.Equal(t, workspaceID, organizationSvc.lastWorkspaceIDForOrganization)
	require.Equal(t, organizationID, organizationSvc.lastPermissionOrganizationID)
	require.Equal(t, workspaceID, organizationSvc.lastPermissionWorkspaceID)
	require.Equal(t, accountID, organizationSvc.lastPermissionAccountID)
	require.Equal(t, model.WorkspacePermissionWorkspaceMemberManage, organizationSvc.lastPermissionCode)
}

func TestMembersHandlerCurrentCancelRequiresManageBeforeAccountLookup(t *testing.T) {
	gin.SetMode(gin.TestMode)

	workspaceID := "workspace-current"
	organizationID := "org-from-workspace"
	accountID := "account-1"
	memberID := "member-1"
	workspaceSvc := &membersHandlerWorkspaceManagementService{
		currentWorkspace: &model.WorkspaceMember{WorkspaceID: workspaceID},
	}
	accountSvc := &membersHandlerAccountService{}
	organizationSvc := &membersHandlerOrganizationService{
		workspaceOrganizationID:         organizationID,
		checkWorkspacePermissionAllowed: false,
	}
	handler := NewMembersHandler(workspaceSvc, accountSvc, organizationSvc, "")

	c, recorder := newMembersHandlerContext(http.MethodDelete, "/workspaces/current/members/"+memberID, accountID)
	c.Params = gin.Params{{Key: "member_id", Value: memberID}}

	handler.CancelMemberInvite(c)

	require.Equal(t, http.StatusForbidden, recorder.Code, recorder.Body.String())
	requireWorkspaceHandlerResponseCode(t, recorder, response.ErrPermissionDenied)
	require.True(t, workspaceSvc.currentWorkspaceCalled)
	require.False(t, accountSvc.getAccountByIDCalled)
	require.False(t, workspaceSvc.removeMemberFromWorkspaceCalled)
	require.Equal(t, workspaceID, organizationSvc.lastWorkspaceIDForOrganization)
	require.Equal(t, organizationID, organizationSvc.lastPermissionOrganizationID)
	require.Equal(t, workspaceID, organizationSvc.lastPermissionWorkspaceID)
	require.Equal(t, accountID, organizationSvc.lastPermissionAccountID)
	require.Equal(t, model.WorkspacePermissionWorkspaceMemberManage, organizationSvc.lastPermissionCode)
}

func TestMembersHandlerCurrentInviteRequiresManageBeforeInvite(t *testing.T) {
	gin.SetMode(gin.TestMode)

	workspaceID := "workspace-current"
	organizationID := "org-from-workspace"
	accountID := "account-1"
	workspaceSvc := &membersHandlerWorkspaceManagementService{
		currentWorkspace: &model.WorkspaceMember{WorkspaceID: workspaceID},
	}
	accountSvc := &membersHandlerAccountService{}
	organizationSvc := &membersHandlerOrganizationService{
		workspaceOrganizationID:         organizationID,
		checkWorkspacePermissionAllowed: false,
	}
	handler := NewMembersHandler(workspaceSvc, accountSvc, organizationSvc, "")

	c, recorder := newMembersHandlerContext(http.MethodPost, "/workspaces/current/members/invite-email", accountID)
	c.Request.Body = http.NoBody
	c.Request = httptest.NewRequest(
		http.MethodPost,
		"/workspaces/current/members/invite-email",
		strings.NewReader(`{"emails":["alice@example.com"],"role":"normal"}`),
	)
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("account_id", accountID)

	handler.InviteMemberByEmail(c)

	require.Equal(t, http.StatusForbidden, recorder.Code, recorder.Body.String())
	requireWorkspaceHandlerResponseCode(t, recorder, response.ErrPermissionDenied)
	require.True(t, workspaceSvc.currentWorkspaceCalled)
	require.False(t, accountSvc.inviteMemberCalled)
	require.Equal(t, workspaceID, organizationSvc.lastWorkspaceIDForOrganization)
	require.Equal(t, organizationID, organizationSvc.lastPermissionOrganizationID)
	require.Equal(t, workspaceID, organizationSvc.lastPermissionWorkspaceID)
	require.Equal(t, accountID, organizationSvc.lastPermissionAccountID)
	require.Equal(t, model.WorkspacePermissionWorkspaceMemberManage, organizationSvc.lastPermissionCode)
}

func TestMembersHandlerLegacyInviteRoutesRejectDirectPermissions(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name   string
		method string
		target string
		body   string
		params gin.Params
		call   func(*MembersHandler, *gin.Context)
	}{
		{
			name:   "current bulk email invite",
			method: http.MethodPost,
			target: "/workspaces/current/members/invite-email",
			body:   `{"emails":["alice@example.com"],"role":"normal","permissions":["agent.view"]}`,
			call:   (*MembersHandler).InviteMemberByEmail,
		},
		{
			name:   "route bulk email invite",
			method: http.MethodPost,
			target: "/workspaces/workspace-route/members/invite-email",
			body:   `{"emails":["alice@example.com"],"role":"normal","permissions":["agent.view"]}`,
			params: gin.Params{{Key: "workspace_id", Value: "workspace-route"}},
			call:   (*MembersHandler).InviteWorkspaceMemberByEmail,
		},
		{
			name:   "route email invite ex",
			method: http.MethodPost,
			target: "/workspaces/workspace-route/members/invite-email-ex",
			body:   `{"email":"alice@example.com","role":"normal","permissions":["agent.view"]}`,
			params: gin.Params{{Key: "workspace_id", Value: "workspace-route"}},
			call:   (*MembersHandler).InviteWorkspaceMemberByEmailEx,
		},
		{
			name:   "route account invite",
			method: http.MethodPost,
			target: "/workspaces/workspace-route/members/invite-by-id",
			body:   `{"account_id":"member-1","role":"normal","permissions":["agent.view"]}`,
			params: gin.Params{{Key: "workspace_id", Value: "workspace-route"}},
			call:   (*MembersHandler).InviteWorkspaceMemberByAccountId,
		},
		{
			name:   "route batch invite",
			method: http.MethodPost,
			target: "/workspaces/workspace-route/members/batch-invite",
			body:   `{"account_ids":["member-1"],"role":"normal","permissions":["agent.view"]}`,
			params: gin.Params{{Key: "workspace_id", Value: "workspace-route"}},
			call:   (*MembersHandler).BatchInviteWorkspaceMembers,
		},
		{
			name:   "current default invite",
			method: http.MethodPost,
			target: "/workspaces/current/members/invite-email-ex",
			body:   `{"email":"alice@example.com","role":"normal","permissions":["agent.view"]}`,
			call:   (*MembersHandler).InviteDefaultMemberByEmailEx,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			workspaceSvc := &membersHandlerWorkspaceManagementService{
				currentWorkspace: &model.WorkspaceMember{WorkspaceID: "workspace-current"},
			}
			accountSvc := &membersHandlerAccountService{}
			organizationSvc := &membersHandlerOrganizationService{
				workspaceOrganizationID:         "org-from-workspace",
				checkWorkspacePermissionAllowed: true,
			}
			handler := NewMembersHandler(workspaceSvc, accountSvc, organizationSvc, "")

			c, recorder := newMembersHandlerContext(tt.method, tt.target, "account-1")
			c.Params = tt.params
			c.Request = httptest.NewRequest(tt.method, tt.target, strings.NewReader(tt.body))
			c.Request.Header.Set("Content-Type", "application/json")
			c.Set("account_id", "account-1")

			tt.call(handler, c)

			require.Equal(t, http.StatusBadRequest, recorder.Code, recorder.Body.String())
			requireWorkspaceHandlerResponseCode(t, recorder, response.ErrInvalidParam)
			require.False(t, workspaceSvc.currentWorkspaceCalled)
			require.False(t, workspaceSvc.getWorkspaceByIDCalled)
			require.False(t, accountSvc.getAccountByIDCalled)
			require.False(t, accountSvc.inviteMemberCalled)
			require.False(t, accountSvc.inviteMemberExCalled)
		})
	}
}

func TestMembersHandlerDefaultInviteRequiresManageBeforeWorkspaceLookup(t *testing.T) {
	gin.SetMode(gin.TestMode)

	workspaceID := "workspace-current"
	organizationID := "org-from-workspace"
	accountID := "account-1"
	workspaceSvc := &membersHandlerWorkspaceManagementService{
		currentWorkspace: &model.WorkspaceMember{WorkspaceID: workspaceID},
	}
	accountSvc := &membersHandlerAccountService{}
	organizationSvc := &membersHandlerOrganizationService{
		workspaceOrganizationID:         organizationID,
		checkWorkspacePermissionAllowed: false,
	}
	handler := NewMembersHandler(workspaceSvc, accountSvc, organizationSvc, "")

	c, recorder := newMembersHandlerContext(http.MethodPost, "/workspaces/current/members/invite-email-ex", accountID)
	c.Request = httptest.NewRequest(
		http.MethodPost,
		"/workspaces/current/members/invite-email-ex",
		strings.NewReader(`{"email":"alice@example.com","role":"normal"}`),
	)
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("account_id", accountID)

	handler.InviteDefaultMemberByEmailEx(c)

	require.Equal(t, http.StatusForbidden, recorder.Code, recorder.Body.String())
	requireWorkspaceHandlerResponseCode(t, recorder, response.ErrPermissionDenied)
	require.True(t, workspaceSvc.currentWorkspaceCalled)
	require.False(t, workspaceSvc.getWorkspaceByIDCalled)
	require.False(t, accountSvc.inviteMemberExCalled)
	require.Equal(t, workspaceID, organizationSvc.lastWorkspaceIDForOrganization)
	require.Equal(t, organizationID, organizationSvc.lastPermissionOrganizationID)
	require.Equal(t, workspaceID, organizationSvc.lastPermissionWorkspaceID)
	require.Equal(t, accountID, organizationSvc.lastPermissionAccountID)
	require.Equal(t, model.WorkspacePermissionWorkspaceMemberManage, organizationSvc.lastPermissionCode)
}

func TestMembersHandlerCurrentUpdateRoleUsesCurrentWorkspaceForPermissionAndMutation(t *testing.T) {
	gin.SetMode(gin.TestMode)

	workspaceID := "workspace-current"
	organizationID := "org-from-workspace"
	accountID := "account-1"
	memberID := "member-1"
	workspaceSvc := &membersHandlerWorkspaceManagementService{
		currentWorkspace: &model.WorkspaceMember{WorkspaceID: workspaceID},
	}
	organizationSvc := &membersHandlerOrganizationService{
		workspaceOrganizationID:         organizationID,
		checkWorkspacePermissionAllowed: true,
	}
	handler := NewMembersHandler(workspaceSvc, &membersHandlerAccountService{}, organizationSvc, "")

	c, recorder := newMembersHandlerContext(http.MethodPut, "/workspaces/current/members/"+memberID+"/update-role", accountID)
	c.Params = gin.Params{{Key: "member_id", Value: memberID}}
	c.Request = httptest.NewRequest(
		http.MethodPut,
		"/workspaces/current/members/"+memberID+"/update-role",
		strings.NewReader(`{"role":"normal"}`),
	)
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("account_id", accountID)

	handler.UpdateMemberRole(c)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	require.True(t, workspaceSvc.currentWorkspaceCalled)
	require.True(t, workspaceSvc.updateMemberRoleCalled)
	require.Equal(t, workspaceID, organizationSvc.lastWorkspaceIDForOrganization)
	require.Equal(t, organizationID, organizationSvc.lastPermissionOrganizationID)
	require.Equal(t, workspaceID, organizationSvc.lastPermissionWorkspaceID)
	require.Equal(t, accountID, organizationSvc.lastPermissionAccountID)
	require.Equal(t, model.WorkspacePermissionWorkspacePermissionManage, organizationSvc.lastPermissionCode)
	require.NotNil(t, workspaceSvc.lastUpdateMemberRoleRequest)
	require.Equal(t, workspaceID, workspaceSvc.lastUpdateMemberRoleRequest.WorkspaceID)
	require.Equal(t, memberID, workspaceSvc.lastUpdateMemberRoleRequest.AccountID)
	require.Equal(t, model.WorkspaceRoleNormal, workspaceSvc.lastUpdateMemberRoleRequest.Role)
}

func newMembersHandlerContext(method, target, accountID string) (*gin.Context, *httptest.ResponseRecorder) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(method, target, nil)
	c.Set("account_id", accountID)
	return c, recorder
}

type membersHandlerWorkspaceManagementService struct {
	interfaces.WorkspaceManagementService

	currentWorkspace                *model.WorkspaceMember
	currentWorkspaceCalled          bool
	membersPaginatedCalled          bool
	membersWithExtensionsCalled     bool
	memberWithExtensionsByIDCalled  bool
	datasetOperatorMembersCalled    bool
	getWorkspaceByIDCalled          bool
	removeMemberFromWorkspaceCalled bool
	updateMemberRoleCalled          bool
	lastUpdateMemberRoleRequest     *interfaces.UpdateMemberRoleRequest
}

func (s *membersHandlerWorkspaceManagementService) GetCurrentWorkspace(context.Context, string) (*model.WorkspaceMember, error) {
	s.currentWorkspaceCalled = true
	return s.currentWorkspace, nil
}

func (s *membersHandlerWorkspaceManagementService) GetWorkspaceMembersPaginated(context.Context, string, int, int, string, string) ([]*interfaces.AccountWithRole, int64, error) {
	s.membersPaginatedCalled = true
	return nil, 0, nil
}

func (s *membersHandlerWorkspaceManagementService) GetWorkspaceMembersWithExtensions(context.Context, string) ([]*interfaces.WorkspaceMemberWithExtensionResponse, error) {
	s.membersWithExtensionsCalled = true
	return nil, nil
}

func (s *membersHandlerWorkspaceManagementService) GetWorkspaceMemberWithExtensionsById(context.Context, string, string) (*interfaces.WorkspaceMemberWithExtensionResponse, error) {
	s.memberWithExtensionsByIDCalled = true
	return nil, nil
}

func (s *membersHandlerWorkspaceManagementService) GetDatasetOperatorMembers(context.Context, string) ([]*interfaces.AccountWithRole, error) {
	s.datasetOperatorMembersCalled = true
	return nil, nil
}

func (s *membersHandlerWorkspaceManagementService) GetWorkspaceByID(_ context.Context, workspaceID string) (*model.Workspace, error) {
	s.getWorkspaceByIDCalled = true
	return &model.Workspace{ID: workspaceID}, nil
}

func (s *membersHandlerWorkspaceManagementService) RemoveMemberFromWorkspace(context.Context, *model.Workspace, *auth_model.Account, *auth_model.Account) error {
	s.removeMemberFromWorkspaceCalled = true
	return nil
}

func (s *membersHandlerWorkspaceManagementService) UpdateMemberRole(_ context.Context, req *interfaces.UpdateMemberRoleRequest) error {
	s.updateMemberRoleCalled = true
	s.lastUpdateMemberRoleRequest = req
	return nil
}

type membersHandlerOrganizationService struct {
	interfaces.OrganizationService

	workspaceOrganizationID         string
	checkWorkspacePermissionAllowed bool
	lastWorkspaceIDForOrganization  string
	lastPermissionOrganizationID    string
	lastPermissionWorkspaceID       string
	lastPermissionAccountID         string
	lastPermissionCode              model.WorkspacePermissionCode
}

func (s *membersHandlerOrganizationService) GetOrganizationByWorkspaceID(_ context.Context, workspaceID string) (*model.Organization, error) {
	s.lastWorkspaceIDForOrganization = workspaceID
	if s.workspaceOrganizationID == "" {
		return nil, nil
	}
	return &model.Organization{ID: s.workspaceOrganizationID, Status: model.OrganizationStatusActive}, nil
}

func (s *membersHandlerOrganizationService) CheckWorkspacePermission(_ context.Context, organizationID, workspaceID, accountID string, permissionCode model.WorkspacePermissionCode) (bool, error) {
	s.lastPermissionOrganizationID = organizationID
	s.lastPermissionWorkspaceID = workspaceID
	s.lastPermissionAccountID = accountID
	s.lastPermissionCode = permissionCode
	return s.checkWorkspacePermissionAllowed, nil
}

type membersHandlerAccountService struct {
	interfaces.AccountService

	getAccountByIDCalled bool
	inviteMemberCalled   bool
	inviteMemberExCalled bool
}

func (s *membersHandlerAccountService) GetAccountByID(context.Context, string) (*auth_model.Account, error) {
	s.getAccountByIDCalled = true
	return nil, nil
}

func (s *membersHandlerAccountService) InviteMember(context.Context, string, string, string, model.WorkspaceMemberRole, string) (string, error) {
	s.inviteMemberCalled = true
	return "", nil
}

func (s *membersHandlerAccountService) InviteMemberEx(context.Context, string, string, string, model.WorkspaceMemberRole, string, string, string, string, string, bool) (string, error) {
	s.inviteMemberExCalled = true
	return "", nil
}

var _ interfaces.WorkspaceManagementService = (*membersHandlerWorkspaceManagementService)(nil)
var _ interfaces.OrganizationService = (*membersHandlerOrganizationService)(nil)
var _ interfaces.AccountService = (*membersHandlerAccountService)(nil)
