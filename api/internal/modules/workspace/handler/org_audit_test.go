package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	shared_dto "github.com/zgiai/zgi/api/internal/dto"
	usererrors "github.com/zgiai/zgi/api/internal/errors"
	"github.com/zgiai/zgi/api/internal/modules/orgaudit"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	auth_model "github.com/zgiai/zgi/api/internal/modules/user/auth/model"
	"github.com/zgiai/zgi/api/internal/modules/workspace/model"
)

const (
	auditOrgID       = "11111111-1111-1111-1111-111111111111"
	auditOperatorID  = "22222222-2222-2222-2222-222222222222"
	auditMemberID    = "33333333-3333-3333-3333-333333333333"
	auditWorkspaceID = "44444444-4444-4444-4444-444444444444"
	auditOtherID     = "55555555-5555-5555-5555-555555555555"
)

type auditCapture struct{ logs []orgaudit.Log }

func (w *auditCapture) Create(_ context.Context, logs []orgaudit.Log) error {
	w.logs = append(w.logs, logs...)
	return nil
}

// statusOrganizationService adds the status update the shared fake lacks.
type statusOrganizationService struct {
	fakeOrganizationService
	updateStatus func(req *shared_dto.UpdateOrganizationMemberStatusRequest) error
}

func (s statusOrganizationService) UpdateMemberStatus(_ context.Context, req *shared_dto.UpdateOrganizationMemberStatusRequest) error {
	return s.updateStatus(req)
}

func serveAudited(t *testing.T, method, route, target, body string, handler gin.HandlerFunc, action orgaudit.Action) (*auditCapture, *httptest.ResponseRecorder) {
	t.Helper()
	capture := &auditCapture{}
	recorder := orgaudit.NewRecorder(capture)
	engine := gin.New()
	engine.Handle(method, route, func(c *gin.Context) {
		c.Set("account_id", auditOperatorID)
		c.Set("request_id", "req-audit")
	}, recorder.Wrap(action, handler))
	request := httptest.NewRequest(method, target, bytes.NewBufferString(body))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	engine.ServeHTTP(response, request)
	return capture, response
}

func auditFields(t *testing.T, raw []byte) map[string]any {
	t.Helper()
	fields := map[string]any{}
	require.NoError(t, json.Unmarshal(raw, &fields))
	return fields
}

func TestUpdateMemberStatusWritesOneSuccessRecord(t *testing.T) {
	updated := 0
	handler := &OrganizationHandler{organizationService: statusOrganizationService{
		fakeOrganizationService: fakeOrganizationService{
			getMemberByAccountIDFn: func(context.Context, string, string) (*shared_dto.OrganizationMemberWithExtensionResponse, error) {
				return &shared_dto.OrganizationMemberWithExtensionResponse{Status: "active"}, nil
			},
		},
		updateStatus: func(req *shared_dto.UpdateOrganizationMemberStatusRequest) error {
			updated++
			return nil
		},
	}}
	handler.audit = orgaudit.NewRecorder(&auditCapture{}) // enables the before lookup
	capture, response := serveAudited(t, http.MethodPut, "/organizations/:organization_id/members/:member_id/status",
		"/organizations/"+auditOrgID+"/members/"+auditMemberID+"/status", `{"status":"inactive"}`,
		handler.UpdateMemberStatus, orgaudit.ActionMemberStatusUpdate)

	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	require.Equal(t, 1, updated)
	require.Len(t, capture.logs, 1)
	log := capture.logs[0]
	require.Equal(t, orgaudit.ResultSuccess, log.Result)
	require.Equal(t, orgaudit.ActionMemberStatusUpdate, log.Action)
	require.Equal(t, auditMemberID, log.TargetAccountID.String())
	require.Equal(t, auditOperatorID, log.ActorAccountID.String())
	require.Equal(t, "req-audit", log.RequestID)
	require.Equal(t, "inactive", auditFields(t, log.After)["status"])
	require.Equal(t, "active", auditFields(t, log.Before)["status"])
}

func TestUpdateMemberStatusByNonAdminWritesOneDeniedRecord(t *testing.T) {
	handler := &OrganizationHandler{organizationService: statusOrganizationService{
		fakeOrganizationService: fakeOrganizationService{
			getUserOrganizationRoleFn: func(context.Context, string, string) (model.OrganizationRole, error) {
				return model.OrganizationRoleNormal, nil
			},
		},
		updateStatus: func(*shared_dto.UpdateOrganizationMemberStatusRequest) error {
			t.Fatal("non-admin reached the status update")
			return nil
		},
	}}
	capture, response := serveAudited(t, http.MethodPut, "/organizations/:organization_id/members/:member_id/status",
		"/organizations/"+auditOrgID+"/members/"+auditMemberID+"/status", `{"status":"inactive"}`,
		handler.UpdateMemberStatus, orgaudit.ActionMemberStatusUpdate)

	require.Equal(t, http.StatusForbidden, response.Code)
	require.Len(t, capture.logs, 1)
	require.Equal(t, orgaudit.ResultDenied, capture.logs[0].Result)
	require.Equal(t, auditMemberID, capture.logs[0].TargetAccountID.String())
}

func TestUpdateWorkspaceMemberPermissionsRecordsBeforeAndAfter(t *testing.T) {
	roleID := "role-1"
	handler := &OrganizationHandler{
		organizationService: fakeOrganizationService{
			checkWorkspacePermissionFn: func(context.Context, string, string, string, model.WorkspacePermissionCode) (bool, error) {
				return true, nil
			},
			getOrganizationByWorkspaceIDFn: func(context.Context, string) (*model.Organization, error) {
				return &model.Organization{ID: auditOrgID}, nil
			},
			getWorkspaceMemberPermissionsFn: func(_ context.Context, _, _, _, target string) (*shared_dto.WorkspaceMemberPermissionsResponse, error) {
				require.Equal(t, auditMemberID, target)
				return &shared_dto.WorkspaceMemberPermissionsResponse{
					WorkspaceRole: "normal", WorkspaceRoleID: &roleID, Permissions: []string{"agent.use"},
					PermissionSource: model.WorkspaceMemberPermissionSource("role_template"),
				}, nil
			},
		},
		workspaceManagementService: fakeWorkspaceManagementService{},
		accountService:             fakeAccountService{},
	}
	handler.audit = orgaudit.NewRecorder(&auditCapture{}) // enables the before lookup
	capture, response := serveAudited(t, http.MethodPut, "/organizations/:organization_id/workspaces/:workspace_id/members/:member_id/permissions",
		"/organizations/"+auditOrgID+"/workspaces/"+auditWorkspaceID+"/members/"+auditMemberID+"/permissions",
		`{"permissions":["agent.manage"]}`, handler.UpdateOrganizationWorkspaceMemberPermissions, orgaudit.ActionWorkspaceMemberPerms)

	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	require.Len(t, capture.logs, 1)
	log := capture.logs[0]
	require.Equal(t, auditWorkspaceID, log.WorkspaceID.String())
	require.Equal(t, []any{"agent.use"}, auditFields(t, log.Before)["permissions"])
	require.Equal(t, "role-1", auditFields(t, log.Before)["workspace_role_id"])
	require.Equal(t, []any{"agent.manage"}, auditFields(t, log.After)["permissions"])
}

func TestApplyRoleTemplateWritesOneRecordPerMember(t *testing.T) {
	handler := &OrganizationHandler{organizationService: fakeOrganizationService{
		applyWorkspaceRoleTemplateFn: func(_ context.Context, req *shared_dto.ApplyWorkspaceRoleTemplateRequest) (*shared_dto.ApplyWorkspaceRoleTemplateResponse, error) {
			return &shared_dto.ApplyWorkspaceRoleTemplateResponse{AppliedCount: 1, FailedCount: 1, Results: []shared_dto.ApplyWorkspaceRoleTemplateResult{
				{WorkspaceID: auditWorkspaceID, AccountID: auditMemberID, Status: "applied"},
				{WorkspaceID: auditWorkspaceID, AccountID: auditOtherID, Status: "failed", Message: "member account not found"},
			}}, nil
		},
	}}
	body := `{"members":[{"workspace_id":"` + auditWorkspaceID + `","account_id":"` + auditMemberID + `"},{"workspace_id":"` + auditWorkspaceID + `","account_id":"` + auditOtherID + `"}]}`
	capture, response := serveAudited(t, http.MethodPost, "/organizations/:organization_id/roles/:role_id/apply-template",
		"/organizations/"+auditOrgID+"/roles/role-1/apply-template", body, handler.ApplyWorkspaceRoleTemplate, orgaudit.ActionRoleTemplateApply)

	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	require.Len(t, capture.logs, 2)
	require.Equal(t, auditMemberID, capture.logs[0].TargetAccountID.String())
	require.Equal(t, orgaudit.ResultSuccess, capture.logs[0].Result)
	require.Equal(t, auditOtherID, capture.logs[1].TargetAccountID.String())
	require.Equal(t, orgaudit.ResultFailed, capture.logs[1].Result)
	require.Equal(t, auditWorkspaceID, capture.logs[1].WorkspaceID.String())
	require.Equal(t, "role-1", auditFields(t, capture.logs[0].After)["role_id"])
}

// TestWriteRoutesAreAudited keeps every member, role and permission write
// route behind the recorder, so a new route cannot silently skip the log.
func TestWriteRoutesAreAudited(t *testing.T) {
	recorder := orgaudit.NewRecorder(&auditCapture{})
	organizationHandler := &OrganizationHandler{}
	organizationHandler.SetAuditRecorder(recorder)
	membersHandler := &MembersHandler{}
	membersHandler.SetAuditRecorder(recorder)
	departmentHandler := &DepartmentHandler{}
	departmentHandler.SetAuditRecorder(recorder)

	engine := gin.New()
	organizationHandler.RegisterRoutes(engine.Group(""))
	membersHandler.RegisterRoutes(engine.Group(""))
	departmentHandler.RegisterRoutes(engine.Group(""))

	// Writes that change no member, role or permission.
	unaudited := map[string]bool{
		"POST /organizations/":                                                true,
		"PUT /organizations/":                                                 true,
		"DELETE /organizations/":                                              true,
		"PATCH /organizations/info/:organization_id":                          true,
		"POST /organizations/:organization_id/workspaces":                     true,
		"PUT /organizations/:organization_id/workspaces/:workspace_id":        true,
		"DELETE /organizations/:organization_id/workspaces/:workspace_id":     true,
		"PUT /organizations/:organization_id/members/:member_id":              true,
		"PUT /workspaces/:workspace_id/members/:member_id/update-ex":          false,
		"PUT /workspaces/:workspace_id/members/:member_id":                    false,
		"POST /organizations/:organization_id/members/direct-add":             false,
		"POST /organizations/:organization_id/workspaces/:workspace_id/leave": false,
		"POST /organizations/:organization_id/departments":                    false,
		"PUT /organizations/:organization_id/departments/member/:account_id":  false,
	}
	checked := 0
	for _, route := range engine.Routes() {
		if route.Method == http.MethodGet {
			continue
		}
		key := route.Method + " " + route.Path
		// Inlining can rename the closure, so match the Wrap closure suffix.
		wrapped := strings.Contains(route.Handler, "Wrap.func")
		if unaudited[key] {
			require.False(t, wrapped, "%s is audited but listed as unaudited", key)
			continue
		}
		require.True(t, wrapped, "%s writes without an audit record", key)
		checked++
	}
	require.Greater(t, checked, 35)
}

// inviteOrganizationService answers organization membership per account.
type inviteOrganizationService struct {
	membersHandlerOrganizationService
	members map[string]bool
}

func (s *inviteOrganizationService) IsOrganizationMember(_ context.Context, _, accountID string) (bool, error) {
	return s.members[accountID], nil
}

// inviteWorkspaceService records the accounts it adds to the workspace.
type inviteWorkspaceService struct {
	membersHandlerWorkspaceManagementService
	added []string
}

func (s *inviteWorkspaceService) GetUserRole(context.Context, string, string) (*model.WorkspaceMemberRole, error) {
	return nil, nil
}

func (s *inviteWorkspaceService) AddMember(_ context.Context, req *interfaces.AddMemberRequest) error {
	s.added = append(s.added, req.AccountID)
	return nil
}

// inviteAccountService resolves invited emails to accounts.
type inviteAccountService struct {
	membersHandlerAccountService
	byEmail map[string]string
}

func (s *inviteAccountService) GetUserThroughEmail(_ context.Context, email string) (*auth_model.Account, error) {
	if id, ok := s.byEmail[email]; ok {
		return &auth_model.Account{ID: id, Email: email}, nil
	}
	return nil, nil
}

func newInviteHandler(organizations *inviteOrganizationService, workspaces *inviteWorkspaceService, accounts *inviteAccountService) *MembersHandler {
	organizations.workspaceOrganizationID = auditOrgID
	organizations.checkWorkspacePermissionAllowed = true
	orgID := auditOrgID
	workspaces.workspaceByID = &model.Workspace{ID: auditWorkspaceID, OrganizationID: &orgID}
	accounts.account = &auth_model.Account{ID: auditOperatorID}
	handler := NewMembersHandler(workspaces, accounts, organizations, "")
	handler.SetAuditRecorder(orgaudit.NewRecorder(&auditCapture{})) // enables the email lookup
	return handler
}

func TestBatchInviteRecordsEachMemberWithItsOwnResult(t *testing.T) {
	workspaces := &inviteWorkspaceService{}
	handler := newInviteHandler(&inviteOrganizationService{members: map[string]bool{auditMemberID: true}}, workspaces, &inviteAccountService{})
	body := `{"account_ids":["` + auditMemberID + `","` + auditOtherID + `"],"role":"normal"}`
	capture, response := serveAudited(t, http.MethodPost, "/workspaces/:workspace_id/members/batch-invite-by-ids",
		"/workspaces/"+auditWorkspaceID+"/members/batch-invite-by-ids", body, handler.BatchInviteWorkspaceMembers, orgaudit.ActionWorkspaceMemberAdd)

	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	require.Contains(t, response.Body.String(), `"status":"failed"`)
	require.Equal(t, []string{auditMemberID}, workspaces.added)
	require.Len(t, capture.logs, 2)
	byTarget := map[string]orgaudit.Log{}
	for _, log := range capture.logs {
		require.NotNil(t, log.TargetAccountID)
		require.Equal(t, auditOrgID, log.OrganizationID.String())
		require.Equal(t, auditWorkspaceID, log.WorkspaceID.String())
		require.Equal(t, "normal", auditFields(t, log.After)["workspace_role"])
		byTarget[log.TargetAccountID.String()] = log
	}
	require.Equal(t, orgaudit.ResultSuccess, byTarget[auditMemberID].Result)
	require.Equal(t, orgaudit.ResultFailed, byTarget[auditOtherID].Result)
}

func TestEmailInviteRecordsTheResolvedMemberAndItsResult(t *testing.T) {
	invite := func(inviteErr error) []orgaudit.Log {
		accounts := &inviteAccountService{byEmail: map[string]string{"new@example.com": auditMemberID}}
		accounts.inviteMemberExErr = inviteErr
		handler := newInviteHandler(&inviteOrganizationService{}, &inviteWorkspaceService{}, accounts)
		capture, response := serveAudited(t, http.MethodPost, "/workspaces/:workspace_id/members/invite-email-ex",
			"/workspaces/"+auditWorkspaceID+"/members/invite-email-ex", `{"email":"new@example.com","role":"normal"}`,
			handler.InviteWorkspaceMemberByEmailEx, orgaudit.ActionWorkspaceMemberAdd)
		require.Equal(t, http.StatusOK, response.Code, response.Body.String())
		return capture.logs
	}

	logs := invite(nil)
	require.Len(t, logs, 1)
	require.Equal(t, orgaudit.ResultSuccess, logs[0].Result)
	require.Equal(t, auditMemberID, logs[0].TargetAccountID.String())

	// The response is 200 either way; the record carries the item's failure.
	logs = invite(errors.New("smtp refused"))
	require.Len(t, logs, 1)
	require.Equal(t, orgaudit.ResultFailed, logs[0].Result)

	// Created but the email bounced: the member still exists.
	logs = invite(usererrors.ErrInviteEmailDeliveryFailed)
	require.Len(t, logs, 1)
	require.Equal(t, orgaudit.ResultSuccess, logs[0].Result)

	// Already in the workspace: nothing changed, nothing recorded.
	require.Empty(t, invite(usererrors.ErrAccountAlreadyInWorkspace))
}
