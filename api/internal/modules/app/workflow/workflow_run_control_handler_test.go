package workflow

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
	"github.com/zgiai/zgi/api/internal/dto"
	workflow_interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	workspace_model "github.com/zgiai/zgi/api/internal/modules/workspace/model"
	"github.com/zgiai/zgi/api/internal/util"
	"github.com/zgiai/zgi/api/pkg/response"
)

func TestStopWorkflowTaskRequiresWorkflowRunStopPermission(t *testing.T) {
	service := &workflowTaskStopService{workspaceID: "agent-workspace"}
	permissionChecker := &workflowTaskStopPermissionChecker{allowed: false}
	handler := &WorkflowHandler{
		workflowService:            service,
		agentWorkspaceResolver:     service,
		workspacePermissionChecker: permissionChecker,
	}
	ctx, recorder := newWorkflowTaskStopContext("agent-1", "run-1", "account-1", "org-1")
	util.SetWorkspaceID(ctx, "current-workspace")

	handler.StopWorkflowTask(ctx)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d, body=%s", recorder.Code, http.StatusForbidden, recorder.Body.String())
	}
	requireWorkflowRunAccessCode(t, recorder, response.ErrPermissionDenied)
	if !permissionChecker.checked {
		t.Fatalf("expected workflow.run.stop permission check")
	}
	if permissionChecker.lastWorkspaceID != "agent-workspace" {
		t.Fatalf("workspace checked = %q, want agent-workspace", permissionChecker.lastWorkspaceID)
	}
	if permissionChecker.lastPermission != workspace_model.WorkspacePermissionWorkflowRunStop {
		t.Fatalf("permission = %q, want %q", permissionChecker.lastPermission, workspace_model.WorkspacePermissionWorkflowRunStop)
	}
	if service.stopCalled {
		t.Fatalf("StopWorkflowTask should not be called when permission is denied")
	}
}

func TestStopWorkflowTaskUsesResolvedAgentWorkspace(t *testing.T) {
	service := &workflowTaskStopService{workspaceID: "agent-workspace"}
	permissionChecker := &workflowTaskStopPermissionChecker{allowed: true}
	handler := &WorkflowHandler{
		workflowService:            service,
		agentWorkspaceResolver:     service,
		workspacePermissionChecker: permissionChecker,
	}
	ctx, recorder := newWorkflowTaskStopContext("agent-1", "run-1", "account-1", "org-1")
	util.SetWorkspaceID(ctx, "current-workspace")

	handler.StopWorkflowTask(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	if !service.stopCalled {
		t.Fatalf("expected StopWorkflowTask to be called")
	}
	if service.stopWorkspaceID != "agent-workspace" || service.stopAgentID != "agent-1" || service.stopTaskID != "run-1" || service.stopAccountID != "account-1" {
		t.Fatalf("stop args = workspace:%q agent:%q task:%q account:%q, want agent-workspace/agent-1/run-1/account-1",
			service.stopWorkspaceID, service.stopAgentID, service.stopTaskID, service.stopAccountID)
	}
}

func TestRunDraftWorkflowNodeRequiresWorkflowDebugBeforeBindingRequest(t *testing.T) {
	service := &workflowTaskStopService{workspaceID: "agent-workspace"}
	permissionChecker := &workflowTaskStopPermissionChecker{allowed: false}
	handler := &WorkflowHandler{
		workflowService:            service,
		agentWorkspaceResolver:     service,
		workspacePermissionChecker: permissionChecker,
		validator:                  validator.New(),
	}
	ctx, recorder := newDraftWorkflowNodeRunContext("agent-1", "node-1", "account-1", "org-1", "{")

	handler.RunDraftWorkflowNode(ctx)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d, body=%s", recorder.Code, http.StatusForbidden, recorder.Body.String())
	}
	requireWorkflowRunAccessCode(t, recorder, response.ErrPermissionDenied)
	if !permissionChecker.checked {
		t.Fatalf("expected workflow.debug permission check")
	}
	if permissionChecker.lastWorkspaceID != "agent-workspace" {
		t.Fatalf("workspace checked = %q, want agent-workspace", permissionChecker.lastWorkspaceID)
	}
	if permissionChecker.lastPermission != workspace_model.WorkspacePermissionWorkflowDebug {
		t.Fatalf("permission = %q, want %q", permissionChecker.lastPermission, workspace_model.WorkspacePermissionWorkflowDebug)
	}
	if service.runNodeCalled {
		t.Fatalf("RunDraftWorkflowNode should not be called when permission is denied")
	}
}

func TestRunAdvancedChatDraftWorkflowRequiresWorkflowRunDraftBeforeBindingRequest(t *testing.T) {
	service := &workflowTaskStopService{workspaceID: "agent-workspace"}
	permissionChecker := &workflowTaskStopPermissionChecker{allowed: false}
	handler := &WorkflowHandler{
		workflowService:            service,
		agentWorkspaceResolver:     service,
		workspacePermissionChecker: permissionChecker,
		validator:                  validator.New(),
	}
	ctx, recorder := newAdvancedChatDraftRunContext("agent-1", "account-1", "org-1", "{")

	handler.RunAdvancedChatDraftWorkflow(ctx)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d, body=%s", recorder.Code, http.StatusForbidden, recorder.Body.String())
	}
	requireWorkflowRunAccessCode(t, recorder, response.ErrPermissionDenied)
	if !permissionChecker.checked {
		t.Fatalf("expected workflow.run.draft permission check")
	}
	if permissionChecker.lastWorkspaceID != "agent-workspace" {
		t.Fatalf("workspace checked = %q, want agent-workspace", permissionChecker.lastWorkspaceID)
	}
	if permissionChecker.lastPermission != workspace_model.WorkspacePermissionWorkflowRunDraft {
		t.Fatalf("permission = %q, want %q", permissionChecker.lastPermission, workspace_model.WorkspacePermissionWorkflowRunDraft)
	}
	if service.getDraftWorkflowCalled {
		t.Fatalf("GetDraftWorkflow should not be called when permission is denied")
	}
}

func TestGenerateDraftWorkflowSuggestedQuestionsRequiresWorkflowDebugBeforeBindingRequest(t *testing.T) {
	service := &workflowTaskStopService{workspaceID: "agent-workspace"}
	permissionChecker := &workflowTaskStopPermissionChecker{allowed: false}
	handler := &WorkflowHandler{
		workflowService:            service,
		agentWorkspaceResolver:     service,
		workspacePermissionChecker: permissionChecker,
		validator:                  validator.New(),
	}
	ctx, recorder := newSuggestedQuestionsContext("agent-1", "account-1", "org-1", "{")

	handler.GenerateDraftWorkflowSuggestedQuestions(ctx)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d, body=%s", recorder.Code, http.StatusForbidden, recorder.Body.String())
	}
	requireWorkflowRunAccessCode(t, recorder, response.ErrPermissionDenied)
	if !permissionChecker.checked {
		t.Fatalf("expected workflow.debug permission check")
	}
	if permissionChecker.lastWorkspaceID != "agent-workspace" {
		t.Fatalf("workspace checked = %q, want agent-workspace", permissionChecker.lastWorkspaceID)
	}
	if permissionChecker.lastPermission != workspace_model.WorkspacePermissionWorkflowDebug {
		t.Fatalf("permission = %q, want %q", permissionChecker.lastPermission, workspace_model.WorkspacePermissionWorkflowDebug)
	}
	if service.suggestedQuestionsCalled {
		t.Fatalf("GenerateDraftWorkflowSuggestedQuestions should not be called when permission is denied")
	}
}

func TestManualDiagnoseNodeRequiresWorkflowDebugBeforeBindingRequest(t *testing.T) {
	service := &workflowTaskStopService{workspaceID: "agent-workspace"}
	permissionChecker := &workflowTaskStopPermissionChecker{allowed: false}
	handler := &WorkflowHandler{
		workflowService:            service,
		agentWorkspaceResolver:     service,
		workspacePermissionChecker: permissionChecker,
	}
	ctx, recorder := newManualDiagnoseNodeContext("agent-1", "run-1", "node-log-1", "account-1", "org-1", "{")

	handler.ManualDiagnoseNode(ctx)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d, body=%s", recorder.Code, http.StatusForbidden, recorder.Body.String())
	}
	requireWorkflowRunAccessCode(t, recorder, response.ErrPermissionDenied)
	if !permissionChecker.checked {
		t.Fatalf("expected workflow.debug permission check")
	}
	if permissionChecker.lastWorkspaceID != "agent-workspace" {
		t.Fatalf("workspace checked = %q, want agent-workspace", permissionChecker.lastWorkspaceID)
	}
	if permissionChecker.lastPermission != workspace_model.WorkspacePermissionWorkflowDebug {
		t.Fatalf("permission = %q, want %q", permissionChecker.lastPermission, workspace_model.WorkspacePermissionWorkflowDebug)
	}
	if service.validateNodeScopeCalled {
		t.Fatalf("ValidateWorkflowRunNodeScope should not be called before permission passes")
	}
	if service.manualDiagnoseCalled {
		t.Fatalf("ManualDiagnoseNode should not be called when permission is denied")
	}
}

func TestManualDiagnoseNodeRejectsNodeScopeBeforeBindingRequest(t *testing.T) {
	service := &workflowTaskStopService{
		workspaceID:           "agent-workspace",
		validateNodeScopeErr:  errors.New("node log outside run"),
		manualDiagnoseResult:  map[string]interface{}{},
		manualDiagnoseNodeLog: "node-log-1",
	}
	permissionChecker := &workflowTaskStopPermissionChecker{allowed: true}
	handler := &WorkflowHandler{
		workflowService:            service,
		agentWorkspaceResolver:     service,
		workspacePermissionChecker: permissionChecker,
	}
	ctx, recorder := newManualDiagnoseNodeContext("agent-1", "run-1", "node-log-1", "account-1", "org-1", "{")

	handler.ManualDiagnoseNode(ctx)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d, body=%s", recorder.Code, http.StatusNotFound, recorder.Body.String())
	}
	requireWorkflowRunAccessCode(t, recorder, response.ErrNotFound)
	if !service.validateNodeScopeCalled {
		t.Fatalf("expected ValidateWorkflowRunNodeScope before request binding")
	}
	if service.validateNodeScopeAgentID != "agent-1" || service.validateNodeScopeRunID != "run-1" || service.validateNodeScopeNodeLogID != "node-log-1" {
		t.Fatalf("validate node scope args = agent:%q run:%q node:%q, want agent-1/run-1/node-log-1",
			service.validateNodeScopeAgentID, service.validateNodeScopeRunID, service.validateNodeScopeNodeLogID)
	}
	if service.manualDiagnoseCalled {
		t.Fatalf("ManualDiagnoseNode should not be called when node scope is rejected")
	}
}

func newWorkflowTaskStopContext(agentID, taskID, accountID, organizationID string) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/agents/"+agentID+"/workflow-runs/tasks/"+taskID+"/stop", nil)
	ctx.Params = gin.Params{
		{Key: "agent_id", Value: agentID},
		{Key: "task_id", Value: taskID},
	}
	ctx.Set("account_id", accountID)
	util.SetOrganizationID(ctx, organizationID)
	return ctx, recorder
}

func newDraftWorkflowNodeRunContext(agentID, nodeID, accountID, organizationID, body string) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/agents/"+agentID+"/workflows/draft/nodes/"+nodeID+"/run", strings.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Params = gin.Params{
		{Key: "agent_id", Value: agentID},
		{Key: "node_id", Value: nodeID},
	}
	ctx.Set("account_id", accountID)
	util.SetOrganizationID(ctx, organizationID)
	return ctx, recorder
}

func newAdvancedChatDraftRunContext(agentID, accountID, organizationID, body string) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/agents/"+agentID+"/advanced-chat/workflows/draft/run", strings.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Params = gin.Params{{Key: "agent_id", Value: agentID}}
	ctx.Set("account_id", accountID)
	util.SetOrganizationID(ctx, organizationID)
	return ctx, recorder
}

func newSuggestedQuestionsContext(agentID, accountID, organizationID, body string) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/agents/"+agentID+"/workflows/draft/suggested-questions/generate", strings.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Params = gin.Params{{Key: "agent_id", Value: agentID}}
	ctx.Set("account_id", accountID)
	util.SetOrganizationID(ctx, organizationID)
	return ctx, recorder
}

func newManualDiagnoseNodeContext(agentID, runID, nodeLogID, accountID, organizationID, body string) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/agents/"+agentID+"/workflow-runs/"+runID+"/nodes/"+nodeLogID+"/diagnose", strings.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Params = gin.Params{
		{Key: "agent_id", Value: agentID},
		{Key: "run_id", Value: runID},
		{Key: "node_log_id", Value: nodeLogID},
	}
	ctx.Set("account_id", accountID)
	util.SetOrganizationID(ctx, organizationID)
	return ctx, recorder
}

type workflowTaskStopService struct {
	workflow_interfaces.WorkflowService

	workspaceID                string
	stopCalled                 bool
	stopWorkspaceID            string
	stopAgentID                string
	stopTaskID                 string
	stopAccountID              string
	runNodeCalled              bool
	getDraftWorkflowCalled     bool
	suggestedQuestionsCalled   bool
	validateNodeScopeErr       error
	validateNodeScopeCalled    bool
	validateNodeScopeAgentID   string
	validateNodeScopeRunID     string
	validateNodeScopeNodeLogID string
	manualDiagnoseCalled       bool
	manualDiagnoseNodeLog      string
	manualDiagnoseModel        string
	manualDiagnoseLang         string
	manualDiagnoseResult       interface{}
}

func (s *workflowTaskStopService) GetAgentWorkspaceID(_ context.Context, _ string) (string, error) {
	return s.workspaceID, nil
}

func (s *workflowTaskStopService) StopWorkflowTask(_ context.Context, workspaceID, agentID, taskID, accountID string) error {
	s.stopCalled = true
	s.stopWorkspaceID = workspaceID
	s.stopAgentID = agentID
	s.stopTaskID = taskID
	s.stopAccountID = accountID
	return nil
}

func (s *workflowTaskStopService) RunDraftWorkflowNode(_ context.Context, workspaceID, agentID, nodeID string, req interface{}, accountID string) (interface{}, error) {
	s.runNodeCalled = true
	return map[string]interface{}{}, nil
}

func (s *workflowTaskStopService) GetDraftWorkflow(context.Context, string, ...bool) (interface{}, error) {
	s.getDraftWorkflowCalled = true
	return nil, nil
}

func (s *workflowTaskStopService) GenerateDraftWorkflowSuggestedQuestions(_ context.Context, _ string, _ string, _ *dto.GenerateSuggestedQuestionsRequest, _ string) (*dto.GenerateSuggestedQuestionsResponse, error) {
	s.suggestedQuestionsCalled = true
	return &dto.GenerateSuggestedQuestionsResponse{}, nil
}

func (s *workflowTaskStopService) ValidateWorkflowRunNodeScope(_ context.Context, agentID, runID, nodeLogID string) error {
	s.validateNodeScopeCalled = true
	s.validateNodeScopeAgentID = agentID
	s.validateNodeScopeRunID = runID
	s.validateNodeScopeNodeLogID = nodeLogID
	return s.validateNodeScopeErr
}

func (s *workflowTaskStopService) ManualDiagnoseNode(_ context.Context, nodeLogID string, model string, lang string) (interface{}, error) {
	s.manualDiagnoseCalled = true
	s.manualDiagnoseNodeLog = nodeLogID
	s.manualDiagnoseModel = model
	s.manualDiagnoseLang = lang
	if s.manualDiagnoseResult != nil {
		return s.manualDiagnoseResult, nil
	}
	return map[string]interface{}{}, nil
}

type workflowTaskStopPermissionChecker struct {
	allowed            bool
	allowedPermissions map[workspace_model.WorkspacePermissionCode]bool
	checked            bool
	lastWorkspaceID    string
	lastPermission     workspace_model.WorkspacePermissionCode
	lastPermissions    []workspace_model.WorkspacePermissionCode
}

func (c *workflowTaskStopPermissionChecker) CheckWorkspacePermission(_ context.Context, _ string, workspaceID string, _ string, permissionCode workspace_model.WorkspacePermissionCode) (bool, error) {
	c.checked = true
	c.lastWorkspaceID = workspaceID
	c.lastPermission = permissionCode
	c.lastPermissions = []workspace_model.WorkspacePermissionCode{permissionCode}
	return c.allows(permissionCode), nil
}

func (c *workflowTaskStopPermissionChecker) CheckWorkspaceOrganizationAnyPermission(_ context.Context, _ string, workspaceID string, _ string, permissions ...workspace_model.WorkspacePermissionCode) (bool, error) {
	c.checked = true
	c.lastWorkspaceID = workspaceID
	c.lastPermissions = append([]workspace_model.WorkspacePermissionCode(nil), permissions...)
	if len(permissions) > 0 {
		c.lastPermission = permissions[0]
	}
	return c.allows(permissions...), nil
}

func (c *workflowTaskStopPermissionChecker) allows(permissions ...workspace_model.WorkspacePermissionCode) bool {
	if len(c.allowedPermissions) == 0 {
		return c.allowed
	}
	for _, permission := range permissions {
		if c.allowedPermissions[permission] {
			return true
		}
	}
	return false
}
