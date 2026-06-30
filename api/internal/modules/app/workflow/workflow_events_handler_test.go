package workflow

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/zgiai/zgi/api/internal/dto"
	workspace_model "github.com/zgiai/zgi/api/internal/modules/workspace/model"
	"github.com/zgiai/zgi/api/internal/util"
)

func TestGetWorkflowRunEventsRequiresWorkflowRunDraftPermission(t *testing.T) {
	runID := "run-1"
	permissionChecker := &workflowRunEventPermissionChecker{allowed: false}
	handler := &WorkflowHandler{
		workflowService: &WorkflowService{
			workflowRunLogRepo: &mockWorkflowRunLogRepo{
				runsByID: map[string]*WorkflowRunLog{
					runID: {
						ID:            runID,
						TenantID:      "workspace-1",
						AgentID:       "agent-1",
						Status:        dto.WorkflowRunStatusRunning,
						CreatedByRole: CreatedByRoleAccount,
						CreatedBy:     "other-account",
					},
				},
			},
		},
		workspacePermissionChecker: permissionChecker,
	}
	ctx, recorder := newWorkflowRunEventsContext(http.MethodGet, "/workflow-runs/"+runID+"/events", runID, "account-1", "org-1")

	handler.GetWorkflowRunEvents(ctx)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d, body=%s", recorder.Code, http.StatusForbidden, recorder.Body.String())
	}
	if !permissionChecker.checked {
		t.Fatalf("expected workflow.run.draft permission check")
	}
	if permissionChecker.lastOrganizationID != "org-1" || permissionChecker.lastWorkspaceID != "workspace-1" || permissionChecker.lastAccountID != "account-1" {
		t.Fatalf("permission scope = org:%q workspace:%q account:%q, want org-1/workspace-1/account-1",
			permissionChecker.lastOrganizationID, permissionChecker.lastWorkspaceID, permissionChecker.lastAccountID)
	}
	if permissionChecker.lastPermission != workspace_model.WorkspacePermissionWorkflowRunDraft {
		t.Fatalf("permission = %q, want %q", permissionChecker.lastPermission, workspace_model.WorkspacePermissionWorkflowRunDraft)
	}
}

func TestGetWorkflowRunEventsRequiresPermissionBeforeQueryValidation(t *testing.T) {
	runID := "run-1"
	permissionChecker := &workflowRunEventPermissionChecker{allowed: false}
	handler := &WorkflowHandler{
		workflowService: &WorkflowService{
			workflowRunLogRepo: &mockWorkflowRunLogRepo{
				runsByID: map[string]*WorkflowRunLog{
					runID: {
						ID:            runID,
						TenantID:      "workspace-1",
						AgentID:       "agent-1",
						Status:        dto.WorkflowRunStatusRunning,
						CreatedByRole: CreatedByRoleAccount,
						CreatedBy:     "other-account",
					},
				},
			},
		},
		workspacePermissionChecker: permissionChecker,
	}
	ctx, recorder := newWorkflowRunEventsContext(http.MethodGet, "/workflow-runs/"+runID+"/events?after=bad", runID, "account-1", "org-1")

	handler.GetWorkflowRunEvents(ctx)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d, body=%s", recorder.Code, http.StatusForbidden, recorder.Body.String())
	}
	if !permissionChecker.checked {
		t.Fatalf("expected workflow.run.draft permission check before query validation")
	}
	if contentType := recorder.Header().Get("Content-Type"); contentType == "text/event-stream" {
		t.Fatalf("SSE should not be opened before permission passes")
	}
}

func TestGetWorkflowRunEventsRejectsSystemRunFromAnotherAccount(t *testing.T) {
	runID := "run-1"
	permissionChecker := &workflowRunEventPermissionChecker{allowed: true}
	handler := &WorkflowHandler{
		workflowService: &WorkflowService{
			workflowRunLogRepo: &mockWorkflowRunLogRepo{
				runsByID: map[string]*WorkflowRunLog{
					runID: {
						ID:            runID,
						TenantID:      builtInWorkflowTenantID,
						AgentID:       "agent-1",
						Status:        dto.WorkflowRunStatusRunning,
						CreatedByRole: CreatedByRoleAccount,
						CreatedBy:     "other-account",
					},
				},
			},
		},
		workspacePermissionChecker: permissionChecker,
	}
	ctx, recorder := newWorkflowRunEventsContext(http.MethodGet, "/workflow-runs/"+runID+"/events", runID, "account-1", "org-1")

	handler.GetWorkflowRunEvents(ctx)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d, body=%s", recorder.Code, http.StatusForbidden, recorder.Body.String())
	}
	if permissionChecker.checked {
		t.Fatalf("system workflow run should not require workspace permission")
	}
}

func newWorkflowRunEventsContext(method, target, runID, accountID, organizationID string) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(method, target, nil)
	ctx.Params = gin.Params{{Key: "workflow_run_id", Value: runID}}
	ctx.Set("account_id", accountID)
	util.SetOrganizationID(ctx, organizationID)
	return ctx, recorder
}

type workflowRunEventPermissionChecker struct {
	allowed            bool
	checked            bool
	lastOrganizationID string
	lastWorkspaceID    string
	lastAccountID      string
	lastPermission     workspace_model.WorkspacePermissionCode
}

func (c *workflowRunEventPermissionChecker) CheckWorkspacePermission(_ context.Context, organizationID, workspaceID, accountID string, permissionCode workspace_model.WorkspacePermissionCode) (bool, error) {
	c.checked = true
	c.lastOrganizationID = organizationID
	c.lastWorkspaceID = workspaceID
	c.lastAccountID = accountID
	c.lastPermission = permissionCode
	return c.allowed, nil
}
