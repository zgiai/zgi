package workflow

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
	workflow_interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	workspace_model "github.com/zgiai/zgi/api/internal/modules/workspace/model"
	"github.com/zgiai/zgi/api/internal/util"
	"github.com/zgiai/zgi/api/pkg/response"
)

func TestDraftWorkflowManagementHandlersRequireWorkspacePermissionBeforeWork(t *testing.T) {
	tests := []struct {
		name               string
		method             string
		target             string
		body               string
		wantPermission     workspace_model.WorkspacePermissionCode
		call               func(*WorkflowHandler, *gin.Context)
		serviceWasCalled   func(*workflowDraftPermissionService) bool
		serviceCallMessage string
	}{
		{
			name:               "get draft workflow",
			method:             http.MethodGet,
			target:             "/agents/agent-1/workflows/draft",
			wantPermission:     workspace_model.WorkspacePermissionWorkflowView,
			call:               (*WorkflowHandler).GetDraftWorkflow,
			serviceWasCalled:   func(s *workflowDraftPermissionService) bool { return s.getDraftCalled },
			serviceCallMessage: "GetDraftWorkflow should not be called when permission is denied",
		},
		{
			name:               "sync draft workflow",
			method:             http.MethodPost,
			target:             "/agents/agent-1/workflows/draft",
			body:               "{",
			wantPermission:     workspace_model.WorkspacePermissionWorkflowUpdate,
			call:               (*WorkflowHandler).SyncDraftWorkflow,
			serviceWasCalled:   func(s *workflowDraftPermissionService) bool { return s.syncDraftCalled },
			serviceCallMessage: "SyncDraftWorkflow should not be called when permission is denied",
		},
		{
			name:               "run draft workflow",
			method:             http.MethodPost,
			target:             "/agents/agent-1/workflows/draft/run",
			body:               "{",
			wantPermission:     workspace_model.WorkspacePermissionWorkflowRunDraft,
			call:               (*WorkflowHandler).RunDraftWorkflow,
			serviceWasCalled:   func(s *workflowDraftPermissionService) bool { return s.runDraftCalled || s.getDraftCalled },
			serviceCallMessage: "RunDraftWorkflow/GetDraftWorkflow should not be called when permission is denied",
		},
		{
			name:               "precheck draft workflow",
			method:             http.MethodPost,
			target:             "/agents/agent-1/workflows/draft/precheck",
			wantPermission:     workspace_model.WorkspacePermissionWorkflowRunDraft,
			call:               (*WorkflowHandler).PrecheckDraftWorkflow,
			serviceWasCalled:   func(s *workflowDraftPermissionService) bool { return s.getDraftCalled },
			serviceCallMessage: "GetDraftWorkflow should not be called when permission is denied",
		},
		{
			name:               "publish workflow",
			method:             http.MethodPost,
			target:             "/agents/agent-1/workflows/publish",
			body:               "{",
			wantPermission:     workspace_model.WorkspacePermissionWorkflowPublish,
			call:               (*WorkflowHandler).PublishWorkflow,
			serviceWasCalled:   func(s *workflowDraftPermissionService) bool { return s.publishCalled },
			serviceCallMessage: "PublishWorkflow should not be called when permission is denied",
		},
		{
			name:               "precheck published workflow",
			method:             http.MethodPost,
			target:             "/agents/agent-1/workflows/precheck",
			wantPermission:     workspace_model.WorkspacePermissionWorkflowView,
			call:               (*WorkflowHandler).PrecheckPublishedWorkflow,
			serviceWasCalled:   func(s *workflowDraftPermissionService) bool { return s.getLatestPublishedCalled },
			serviceCallMessage: "GetLatestPublishedWorkflow should not be called when permission is denied",
		},
		{
			name:           "run published workflow",
			method:         http.MethodPost,
			target:         "/agents/agent-1/workflows/run",
			body:           "{",
			wantPermission: workspace_model.WorkspacePermissionWorkflowView,
			call:           (*WorkflowHandler).RunPublishedWorkflow,
			serviceWasCalled: func(s *workflowDraftPermissionService) bool {
				return s.getLatestPublishedCalled || s.runPublishedCalled
			},
			serviceCallMessage: "RunPublishedWorkflow/GetLatestPublishedWorkflow should not be called when permission is denied",
		},
		{
			name:               "precheck advanced chat published workflow",
			method:             http.MethodPost,
			target:             "/agents/agent-1/advanced-chat/workflows/precheck",
			wantPermission:     workspace_model.WorkspacePermissionWorkflowView,
			call:               (*WorkflowHandler).PrecheckAdvancedChatWorkflow,
			serviceWasCalled:   func(s *workflowDraftPermissionService) bool { return s.getLatestPublishedCalled },
			serviceCallMessage: "GetLatestPublishedWorkflow should not be called when permission is denied",
		},
		{
			name:           "run advanced chat published workflow",
			method:         http.MethodPost,
			target:         "/agents/agent-1/advanced-chat/workflows/run",
			body:           "{",
			wantPermission: workspace_model.WorkspacePermissionWorkflowView,
			call:           (*WorkflowHandler).RunAdvancedChatWorkflow,
			serviceWasCalled: func(s *workflowDraftPermissionService) bool {
				return s.getLatestPublishedCalled || s.runAdvancedChatPublishedCalled
			},
			serviceCallMessage: "RunAdvancedChatWorkflow/GetLatestPublishedWorkflow should not be called when permission is denied",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := &workflowDraftPermissionService{workspaceID: "agent-workspace"}
			permissionChecker := &workflowTaskStopPermissionChecker{allowed: false}
			handler := &WorkflowHandler{
				workflowService:            service,
				agentWorkspaceResolver:     service,
				workspacePermissionChecker: permissionChecker,
				validator:                  validator.New(),
			}
			ctx, recorder := newDraftWorkflowManagementContext(tt.method, tt.target, "agent-1", "account-1", "org-1", tt.body)

			tt.call(handler, ctx)

			if recorder.Code != http.StatusForbidden {
				t.Fatalf("status = %d, want %d, body=%s", recorder.Code, http.StatusForbidden, recorder.Body.String())
			}
			requireWorkflowRunAccessCode(t, recorder, response.ErrPermissionDenied)
			if !permissionChecker.checked {
				t.Fatalf("expected workspace permission check")
			}
			if permissionChecker.lastWorkspaceID != "agent-workspace" {
				t.Fatalf("workspace checked = %q, want agent-workspace", permissionChecker.lastWorkspaceID)
			}
			if permissionChecker.lastPermission != tt.wantPermission {
				t.Fatalf("permission = %q, want %q", permissionChecker.lastPermission, tt.wantPermission)
			}
			if tt.serviceWasCalled(service) {
				t.Fatal(tt.serviceCallMessage)
			}
		})
	}
}

func TestGetDraftWorkflowAllowsEditorReachabilityPermissions(t *testing.T) {
	service := &workflowDraftPermissionService{workspaceID: "agent-workspace"}
	permissionChecker := &workflowTaskStopPermissionChecker{
		allowedPermissions: map[workspace_model.WorkspacePermissionCode]bool{
			workspace_model.WorkspacePermissionWorkflowRunDraft: true,
		},
	}
	handler := &WorkflowHandler{
		workflowService:            service,
		agentWorkspaceResolver:     service,
		workspacePermissionChecker: permissionChecker,
		validator:                  validator.New(),
	}
	ctx, recorder := newDraftWorkflowManagementContext(http.MethodGet, "/agents/agent-1/workflows/draft", "agent-1", "account-1", "org-1", "")

	handler.GetDraftWorkflow(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	if !permissionChecker.checked {
		t.Fatalf("expected workspace permission check")
	}
	if permissionChecker.lastWorkspaceID != "agent-workspace" {
		t.Fatalf("workspace checked = %q, want agent-workspace", permissionChecker.lastWorkspaceID)
	}
	if got, want := permissionChecker.lastPermissions, workflowDraftReadPermissionCodes(); !sameWorkspacePermissions(got, want) {
		t.Fatalf("permissions = %v, want %v", got, want)
	}
	if !service.getDraftCalled {
		t.Fatalf("GetDraftWorkflow should be called when one editor reachability permission is allowed")
	}
}

func TestPublishedRuntimeHandlersUseAPIKeyScopeInsteadOfWorkspaceMembership(t *testing.T) {
	tests := []struct {
		name string
		path string
		call func(*WorkflowHandler, *gin.Context)
	}{
		{
			name: "published workflow",
			path: "/agents/agent-1/workflows/run",
			call: (*WorkflowHandler).RunPublishedWorkflow,
		},
		{
			name: "advanced chat published workflow",
			path: "/agents/agent-1/advanced-chat/workflows/run",
			call: (*WorkflowHandler).RunAdvancedChatWorkflow,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := &workflowDraftPermissionService{workspaceID: "agent-workspace"}
			permissionChecker := &workflowTaskStopPermissionChecker{allowed: false}
			handler := &WorkflowHandler{
				workflowService:            service,
				agentWorkspaceResolver:     service,
				workspacePermissionChecker: permissionChecker,
				validator:                  validator.New(),
			}
			ctx, recorder := newDraftWorkflowManagementContext(http.MethodPost, tt.path, "agent-1", "api-key-1", "", "{")
			util.SetWorkspaceScopeCompat(ctx, "agent-workspace")
			ctx.Set("invoke_from", string(InvokeFromExternalAPI))
			ctx.Set("api_key_info", struct{}{})
			ctx.Set("agent_id", "agent-1")

			tt.call(handler, ctx)

			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d, body=%s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
			}
			requireWorkflowRunAccessCode(t, recorder, response.ErrInvalidParam)
			if permissionChecker.checked {
				t.Fatalf("workspace permission checker should not run for API key runtime calls")
			}
			if service.getLatestPublishedCalled || service.runPublishedCalled || service.runAdvancedChatPublishedCalled {
				t.Fatalf("service should not be called after malformed API key runtime body")
			}
		})
	}
}

func TestPublishedRuntimeHandlersRejectAPIKeyAgentMismatchBeforeWorkspacePermission(t *testing.T) {
	service := &workflowDraftPermissionService{workspaceID: "agent-workspace"}
	permissionChecker := &workflowTaskStopPermissionChecker{allowed: false}
	handler := &WorkflowHandler{
		workflowService:            service,
		agentWorkspaceResolver:     service,
		workspacePermissionChecker: permissionChecker,
		validator:                  validator.New(),
	}
	ctx, recorder := newDraftWorkflowManagementContext(http.MethodPost, "/agents/agent-1/workflows/run", "agent-1", "api-key-1", "", "{")
	util.SetWorkspaceScopeCompat(ctx, "agent-workspace")
	ctx.Set("invoke_from", string(InvokeFromExternalAPI))
	ctx.Set("api_key_info", struct{}{})
	ctx.Set("agent_id", "other-agent")

	handler.RunPublishedWorkflow(ctx)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d, body=%s", recorder.Code, http.StatusForbidden, recorder.Body.String())
	}
	requireWorkflowRunAccessCode(t, recorder, response.ErrPermissionDenied)
	if permissionChecker.checked {
		t.Fatalf("workspace permission checker should not run for mismatched API key runtime calls")
	}
}

func newDraftWorkflowManagementContext(method, target, agentID, accountID, organizationID, body string) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(method, target, strings.NewReader(body))
	if body != "" {
		ctx.Request.Header.Set("Content-Type", "application/json")
	}
	ctx.Params = gin.Params{{Key: "agent_id", Value: agentID}}
	ctx.Set("account_id", accountID)
	util.SetOrganizationID(ctx, organizationID)
	return ctx, recorder
}

type workflowDraftPermissionService struct {
	workflow_interfaces.WorkflowService

	workspaceID     string
	getDraftCalled  bool
	syncDraftCalled bool
	runDraftCalled  bool
	publishCalled   bool

	getLatestPublishedCalled       bool
	runPublishedCalled             bool
	runAdvancedChatPublishedCalled bool
}

func (s *workflowDraftPermissionService) GetAgentWorkspaceID(_ context.Context, _ string) (string, error) {
	return s.workspaceID, nil
}

func (s *workflowDraftPermissionService) GetDraftWorkflow(context.Context, string, ...bool) (interface{}, error) {
	s.getDraftCalled = true
	return map[string]interface{}{}, nil
}

func (s *workflowDraftPermissionService) SyncDraftWorkflow(context.Context, string, string, interface{}, string) (interface{}, error) {
	s.syncDraftCalled = true
	return map[string]interface{}{}, nil
}

func (s *workflowDraftPermissionService) RunDraftWorkflow(context.Context, string, string, interface{}, string) (interface{}, error) {
	s.runDraftCalled = true
	return map[string]interface{}{}, nil
}

func (s *workflowDraftPermissionService) GetLatestPublishedWorkflow(context.Context, string, string, ...bool) (interface{}, error) {
	s.getLatestPublishedCalled = true
	return map[string]interface{}{}, nil
}

func (s *workflowDraftPermissionService) RunPublishedWorkflow(context.Context, string, string, interface{}, string) (interface{}, error) {
	s.runPublishedCalled = true
	return map[string]interface{}{}, nil
}

func (s *workflowDraftPermissionService) RunAdvancedChatWorkflow(context.Context, string, string, interface{}, string) (interface{}, error) {
	s.runAdvancedChatPublishedCalled = true
	return map[string]interface{}{}, nil
}

func (s *workflowDraftPermissionService) PublishWorkflow(context.Context, string, string, interface{}, string) (interface{}, error) {
	s.publishCalled = true
	return map[string]interface{}{}, nil
}

func sameWorkspacePermissions(left, right []workspace_model.WorkspacePermissionCode) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
