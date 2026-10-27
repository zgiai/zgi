package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	shared_dto "github.com/zgiai/zgi/api/internal/dto"
	"github.com/zgiai/zgi/api/internal/modules/orgaudit"
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
