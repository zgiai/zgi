package workflow

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/zgiai/zgi/api/internal/dto"
	workflow_interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	workspace_model "github.com/zgiai/zgi/api/internal/modules/workspace/model"
	"github.com/zgiai/zgi/api/internal/util"
	"github.com/zgiai/zgi/api/pkg/response"
)

func TestGetWorkflowRunsRequiresWorkflowLogsViewBeforeBindingQuery(t *testing.T) {
	service := &workflowRunAccessService{workspaceID: "workspace-1"}
	permissionChecker := &workflowRunAccessPermissionChecker{allowed: false}
	handler := &WorkflowHandler{
		workflowService:            service,
		agentWorkspaceResolver:     service,
		workspacePermissionChecker: permissionChecker,
	}
	ctx, recorder := newWorkflowRunAccessContext(
		http.MethodGet,
		"/agents/agent-1/workflow-runs?page=0&status=not-a-status",
		"agent-1",
		"",
		"account-1",
		"org-1",
	)

	handler.GetWorkflowRuns(ctx)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusForbidden, recorder.Body.String())
	}
	requireWorkflowRunAccessCode(t, recorder, response.ErrPermissionDenied)
	if !permissionChecker.checked {
		t.Fatalf("expected workflow.logs.view permission check")
	}
	if permissionChecker.lastPermission != workspace_model.WorkspacePermissionWorkflowLogsView {
		t.Fatalf("permission = %q, want %q", permissionChecker.lastPermission, workspace_model.WorkspacePermissionWorkflowLogsView)
	}
	if service.runsCalled {
		t.Fatalf("GetWorkflowRuns should not be called before permission passed")
	}
}

func TestGetWorkflowRunDetailRequiresWorkflowLogsViewPermission(t *testing.T) {
	service := &workflowRunAccessService{workspaceID: "workspace-1"}
	permissionChecker := &workflowRunAccessPermissionChecker{allowed: false}
	handler := &WorkflowHandler{
		workflowService:            service,
		agentWorkspaceResolver:     service,
		workspacePermissionChecker: permissionChecker,
	}
	ctx, recorder := newWorkflowRunAccessContext(http.MethodGet, "/agents/agent-1/workflow-runs/run-1", "agent-1", "run-1", "account-1", "org-1")

	handler.GetWorkflowRunDetail(ctx)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusForbidden)
	}
	requireWorkflowRunAccessCode(t, recorder, response.ErrPermissionDenied)
	if !permissionChecker.checked {
		t.Fatalf("expected workflow.logs.view permission check")
	}
	if service.validateCalled {
		t.Fatalf("ValidateWorkflowRunAccess called before permission passed")
	}
	if service.detailCalled {
		t.Fatalf("GetWorkflowRunDetail should not be called")
	}
}

func TestGetWorkflowRunDetailRejectsSystemRunFromAnotherAccount(t *testing.T) {
	service := &workflowRunAccessService{
		workspaceID:  builtInWorkflowTenantID,
		validateErr:  errWorkflowRunAccessDenied,
		detailResult: &dto.WorkflowRunDetailResponse{},
	}
	permissionChecker := &workflowRunAccessPermissionChecker{allowed: true}
	handler := &WorkflowHandler{
		workflowService:            service,
		agentWorkspaceResolver:     service,
		workspacePermissionChecker: permissionChecker,
	}
	ctx, recorder := newWorkflowRunAccessContext(http.MethodGet, "/agents/agent-1/workflow-runs/run-1", "agent-1", "run-1", "account-1", "org-1")

	handler.GetWorkflowRunDetail(ctx)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusForbidden)
	}
	requireWorkflowRunAccessCode(t, recorder, response.ErrPermissionDenied)
	if permissionChecker.checked {
		t.Fatalf("system workflow run should not require workspace permission")
	}
	if !service.validateCalled {
		t.Fatalf("expected ValidateWorkflowRunAccess for system workflow run")
	}
	if service.detailCalled {
		t.Fatalf("GetWorkflowRunDetail should not be called")
	}
}

func TestGetWorkflowRunNodeExecutionsRejectsRunFromAnotherAgent(t *testing.T) {
	service := &workflowRunAccessService{
		workspaceID: "workspace-1",
		validateErr: errWorkflowRunNotFoundOrDenied,
	}
	handler := &WorkflowHandler{
		workflowService:            service,
		agentWorkspaceResolver:     service,
		workspacePermissionChecker: &workflowRunAccessPermissionChecker{allowed: true},
	}
	ctx, recorder := newWorkflowRunAccessContext(http.MethodGet, "/agents/agent-1/workflow-runs/run-1/node-executions", "agent-1", "run-1", "account-1", "org-1")

	handler.GetWorkflowRunNodeExecutions(ctx)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNotFound)
	}
	requireWorkflowRunAccessCode(t, recorder, response.ErrNotFound)
	if !service.validateCalled {
		t.Fatalf("expected ValidateWorkflowRunAccess before node execution lookup")
	}
	if service.nodeExecutionsCalled {
		t.Fatalf("GetWorkflowRunNodeExecutions should not be called")
	}
}

func newWorkflowRunAccessContext(method, target, agentID, runID, accountID, organizationID string) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(method, target, nil)
	ctx.Params = gin.Params{
		{Key: "agent_id", Value: agentID},
		{Key: "run_id", Value: runID},
	}
	ctx.Set("account_id", accountID)
	util.SetOrganizationID(ctx, organizationID)
	return ctx, recorder
}

func requireWorkflowRunAccessCode(t *testing.T, recorder *httptest.ResponseRecorder, expected response.ErrorCode) {
	t.Helper()
	var body response.Response
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("response body is not valid JSON: %v", err)
	}
	if body.Code != strconv.Itoa(expected.Code) {
		t.Fatalf("code = %q, want %q", body.Code, strconv.Itoa(expected.Code))
	}
}

type workflowRunAccessService struct {
	workflow_interfaces.WorkflowService

	workspaceID          string
	validateErr          error
	validateCalled       bool
	validateWorkspaceID  string
	validateAgentID      string
	validateRunID        string
	validateAccountID    string
	detailCalled         bool
	detailResult         *dto.WorkflowRunDetailResponse
	nodeExecutionsCalled bool
	nodeExecutionsResult interface{}
	runsCalled           bool
	runsResult           *dto.WorkflowRunsResponse
}

func (s *workflowRunAccessService) GetAgentWorkspaceID(_ context.Context, agentID string) (string, error) {
	s.validateAgentID = agentID
	return s.workspaceID, nil
}

func (s *workflowRunAccessService) ValidateWorkflowRunAccess(_ context.Context, appWorkspaceID, agentID, runID, accountID string) error {
	s.validateCalled = true
	s.validateWorkspaceID = appWorkspaceID
	s.validateAgentID = agentID
	s.validateRunID = runID
	s.validateAccountID = accountID
	return s.validateErr
}

func (s *workflowRunAccessService) GetWorkflowRunDetail(_ context.Context, workspaceID, agentID, runID string) (*dto.WorkflowRunDetailResponse, error) {
	s.detailCalled = true
	return s.detailResult, nil
}

func (s *workflowRunAccessService) GetWorkflowRuns(_ context.Context, _ string, _ *dto.WorkflowRunsRequest, _ string, _ string) (*dto.WorkflowRunsResponse, error) {
	s.runsCalled = true
	return s.runsResult, nil
}

func (s *workflowRunAccessService) GetWorkflowRunNodeExecutions(_ context.Context, workspaceID, agentID, runID string) (interface{}, error) {
	s.nodeExecutionsCalled = true
	return s.nodeExecutionsResult, nil
}

type workflowRunAccessPermissionChecker struct {
	allowed        bool
	checked        bool
	lastPermission workspace_model.WorkspacePermissionCode
}

func (c *workflowRunAccessPermissionChecker) CheckWorkspacePermission(_ context.Context, organizationID, workspaceID, accountID string, permissionCode workspace_model.WorkspacePermissionCode) (bool, error) {
	c.checked = true
	c.lastPermission = permissionCode
	return c.allowed, nil
}
