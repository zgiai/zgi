package workflow

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	workflow_interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	workspace_model "github.com/zgiai/zgi/api/internal/modules/workspace/model"
	"github.com/zgiai/zgi/api/internal/util"
	"github.com/zgiai/zgi/api/pkg/response"
)

func TestExportWorkflowRequiresAgentViewPermission(t *testing.T) {
	service := &workflowImportExportService{workspaceID: "agent-workspace"}
	permissionChecker := &workflowImportExportPermissionChecker{allowed: false}
	handler := &WorkflowHandler{
		workflowService:            service,
		agentWorkspaceResolver:     service,
		workspacePermissionChecker: permissionChecker,
	}
	ctx, recorder := newWorkflowExportContext("agent-1", "account-1", "org-1")

	handler.ExportWorkflow(ctx)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d, body=%s", recorder.Code, http.StatusForbidden, recorder.Body.String())
	}
	requireWorkflowRunAccessCode(t, recorder, response.ErrPermissionDenied)
	if !permissionChecker.checked {
		t.Fatalf("expected agent.view permission check")
	}
	if permissionChecker.lastWorkspaceID != "agent-workspace" {
		t.Fatalf("workspace checked = %q, want agent-workspace", permissionChecker.lastWorkspaceID)
	}
	if permissionChecker.lastPermission != workspace_model.WorkspacePermissionAgentView {
		t.Fatalf("permission = %q, want %q", permissionChecker.lastPermission, workspace_model.WorkspacePermissionAgentView)
	}
}

func TestImportWorkflowUsesFormWorkspaceForAgentManagePermission(t *testing.T) {
	permissionChecker := &workflowImportExportPermissionChecker{allowed: false}
	handler := &WorkflowHandler{
		workspacePermissionChecker: permissionChecker,
	}
	ctx, recorder := newWorkflowImportContext("account-1", "org-1", "current-workspace", "target-workspace")

	handler.ImportWorkflow(ctx)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d, body=%s", recorder.Code, http.StatusForbidden, recorder.Body.String())
	}
	requireWorkflowRunAccessCode(t, recorder, response.ErrPermissionDenied)
	if !permissionChecker.checked {
		t.Fatalf("expected agent.manage permission check")
	}
	if permissionChecker.lastOrganizationID != "org-1" || permissionChecker.lastWorkspaceID != "target-workspace" || permissionChecker.lastAccountID != "account-1" {
		t.Fatalf("permission scope = org:%q workspace:%q account:%q, want org-1/target-workspace/account-1",
			permissionChecker.lastOrganizationID, permissionChecker.lastWorkspaceID, permissionChecker.lastAccountID)
	}
	if permissionChecker.lastPermission != workspace_model.WorkspacePermissionAgentManage {
		t.Fatalf("permission = %q, want %q", permissionChecker.lastPermission, workspace_model.WorkspacePermissionAgentManage)
	}
}

func TestImportWorkflowRejectsUnauthorizedBeforeReadingForm(t *testing.T) {
	handler := &WorkflowHandler{}
	body := &trackingReadCloser{}
	ctx, recorder := newWorkflowImportUnreadBodyContext(body, "", "org-1", "current-workspace")

	handler.ImportWorkflow(ctx)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d, body=%s", recorder.Code, http.StatusUnauthorized, recorder.Body.String())
	}
	requireWorkflowRunAccessCode(t, recorder, response.ErrUnauthorized)
	if body.readCalled {
		t.Fatalf("request body was read before unauthorized import was rejected")
	}
}

func TestImportWorkflowRejectsMissingOrganizationBeforeReadingForm(t *testing.T) {
	handler := &WorkflowHandler{}
	body := &trackingReadCloser{}
	ctx, recorder := newWorkflowImportUnreadBodyContext(body, "account-1", "", "current-workspace")

	handler.ImportWorkflow(ctx)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d, body=%s", recorder.Code, http.StatusNotFound, recorder.Body.String())
	}
	requireWorkflowRunAccessCode(t, recorder, response.ErrOrganizationNotFound)
	if body.readCalled {
		t.Fatalf("request body was read before missing-organization import was rejected")
	}
}

func newWorkflowExportContext(agentID, accountID, organizationID string) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/agents/"+agentID+"/workflows/export", nil)
	ctx.Params = gin.Params{{Key: "agent_id", Value: agentID}}
	ctx.Set("account_id", accountID)
	util.SetOrganizationID(ctx, organizationID)
	return ctx, recorder
}

func newWorkflowImportContext(accountID, organizationID, currentWorkspaceID, formWorkspaceID string) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	form := url.Values{}
	form.Set("workspace_id", formWorkspaceID)
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/workflows/import", strings.NewReader(form.Encode()))
	ctx.Request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	ctx.Set("account_id", accountID)
	util.SetOrganizationID(ctx, organizationID)
	util.SetWorkspaceID(ctx, currentWorkspaceID)
	return ctx, recorder
}

func newWorkflowImportUnreadBodyContext(body *trackingReadCloser, accountID, organizationID, currentWorkspaceID string) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/workflows/import", nil)
	ctx.Request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	ctx.Request.Body = body
	ctx.Request.ContentLength = 16
	if accountID != "" {
		ctx.Set("account_id", accountID)
	}
	if organizationID != "" {
		util.SetOrganizationID(ctx, organizationID)
	}
	util.SetWorkspaceID(ctx, currentWorkspaceID)
	return ctx, recorder
}

type workflowImportExportService struct {
	workflow_interfaces.WorkflowService

	workspaceID string
}

func (s *workflowImportExportService) GetAgentWorkspaceID(_ context.Context, _ string) (string, error) {
	return s.workspaceID, nil
}

type workflowImportExportPermissionChecker struct {
	allowed            bool
	checked            bool
	lastOrganizationID string
	lastWorkspaceID    string
	lastAccountID      string
	lastPermission     workspace_model.WorkspacePermissionCode
}

func (c *workflowImportExportPermissionChecker) CheckWorkspacePermission(_ context.Context, organizationID, workspaceID, accountID string, permissionCode workspace_model.WorkspacePermissionCode) (bool, error) {
	c.checked = true
	c.lastOrganizationID = organizationID
	c.lastWorkspaceID = workspaceID
	c.lastAccountID = accountID
	c.lastPermission = permissionCode
	return c.allowed, nil
}

type trackingReadCloser struct {
	readCalled bool
}

func (b *trackingReadCloser) Read(_ []byte) (int, error) {
	b.readCalled = true
	return 0, io.ErrUnexpectedEOF
}

func (b *trackingReadCloser) Close() error {
	return nil
}
