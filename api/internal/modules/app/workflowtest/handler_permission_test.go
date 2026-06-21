package workflowtest

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	workspace_model "github.com/zgiai/zgi/api/internal/modules/workspace/model"
	"github.com/zgiai/zgi/api/internal/util"
	"github.com/zgiai/zgi/api/pkg/response"
)

func TestHandlerListBatchItemsRequiresAgentViewPermission(t *testing.T) {
	agentID := "11111111-1111-1111-1111-111111111111"
	batchID := "22222222-2222-2222-2222-222222222222"
	workflowService := &workflowTestPermissionWorkspaceResolver{workspaceID: "workspace-1"}
	permissionChecker := &workflowTestPermissionChecker{allowed: false}
	handler := &Handler{
		agentWorkspaceResolver: workflowService,
		organizationService:    permissionChecker,
	}
	ctx, recorder := newWorkflowTestPermissionContext(http.MethodGet, "/agents/"+agentID+"/workflow-tests/batches/"+batchID+"/items", "account-1", "org-1", gin.Params{
		{Key: "agent_id", Value: agentID},
		{Key: "batch_id", Value: batchID},
	})

	handler.ListBatchItems(ctx)

	require.Equal(t, http.StatusForbidden, recorder.Code)
	requireResponseCode(t, recorder, response.ErrPermissionDenied)
	require.Equal(t, agentID, workflowService.lastAgentID)
	require.Equal(t, "org-1", permissionChecker.lastOrganizationID)
	require.Equal(t, "workspace-1", permissionChecker.lastWorkspaceID)
	require.Equal(t, "account-1", permissionChecker.lastAccountID)
	require.Equal(t, workspace_model.WorkspacePermissionAgentView, permissionChecker.lastPermission)
}

func TestHandlerTaskReadRoutesRequireAgentViewBeforeTaskLookup(t *testing.T) {
	agentID := "11111111-1111-1111-1111-111111111111"
	taskID := "44444444-4444-4444-4444-444444444444"
	tests := []struct {
		name   string
		method string
		target string
		params gin.Params
		call   func(*Handler, *gin.Context)
	}{
		{
			name:   "active scenario recognition task",
			method: http.MethodGet,
			target: "/agents/" + agentID + "/workflow-tests/scenarios/recognition-tasks/active",
			params: gin.Params{{Key: "agent_id", Value: agentID}},
			call:   (*Handler).GetActiveScenarioRecognitionTask,
		},
		{
			name:   "latest scenario recognition task",
			method: http.MethodGet,
			target: "/agents/" + agentID + "/workflow-tests/scenarios/recognition-tasks",
			params: gin.Params{{Key: "agent_id", Value: agentID}},
			call:   (*Handler).GetLatestScenarioRecognitionTask,
		},
		{
			name:   "specific scenario recognition task",
			method: http.MethodGet,
			target: "/agents/" + agentID + "/workflow-tests/scenarios/recognition-tasks/" + taskID,
			params: gin.Params{
				{Key: "agent_id", Value: agentID},
				{Key: "task_id", Value: taskID},
			},
			call: (*Handler).GetScenarioRecognitionTask,
		},
		{
			name:   "active generation task",
			method: http.MethodGet,
			target: "/agents/" + agentID + "/workflow-tests/cases/generation-tasks/active",
			params: gin.Params{{Key: "agent_id", Value: agentID}},
			call:   (*Handler).GetActiveGenerationTask,
		},
		{
			name:   "latest generation task",
			method: http.MethodGet,
			target: "/agents/" + agentID + "/workflow-tests/cases/generation-tasks",
			params: gin.Params{{Key: "agent_id", Value: agentID}},
			call:   (*Handler).GetLatestGenerationTask,
		},
		{
			name:   "specific generation task",
			method: http.MethodGet,
			target: "/agents/" + agentID + "/workflow-tests/cases/generation-tasks/" + taskID,
			params: gin.Params{
				{Key: "agent_id", Value: agentID},
				{Key: "task_id", Value: taskID},
			},
			call: (*Handler).GetGenerationTask,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			workflowService := &workflowTestPermissionWorkspaceResolver{workspaceID: "workspace-1"}
			permissionChecker := &workflowTestPermissionChecker{allowed: false}
			handler := &Handler{
				agentWorkspaceResolver: workflowService,
				organizationService:    permissionChecker,
			}
			ctx, recorder := newWorkflowTestPermissionContext(tt.method, tt.target, "account-1", "org-1", tt.params)

			tt.call(handler, ctx)

			require.Equal(t, http.StatusForbidden, recorder.Code)
			requireResponseCode(t, recorder, response.ErrPermissionDenied)
			require.Equal(t, agentID, workflowService.lastAgentID)
			require.Equal(t, "org-1", permissionChecker.lastOrganizationID)
			require.Equal(t, "workspace-1", permissionChecker.lastWorkspaceID)
			require.Equal(t, "account-1", permissionChecker.lastAccountID)
			require.Equal(t, workspace_model.WorkspacePermissionAgentView, permissionChecker.lastPermission)
		})
	}
}

func TestHandlerTaskCancelRoutesRequireAgentManageBeforeTaskLookup(t *testing.T) {
	agentID := "11111111-1111-1111-1111-111111111111"
	taskID := "44444444-4444-4444-4444-444444444444"
	tests := []struct {
		name   string
		method string
		target string
		params gin.Params
		call   func(*Handler, *gin.Context)
	}{
		{
			name:   "scenario recognition task cancel",
			method: http.MethodPost,
			target: "/agents/" + agentID + "/workflow-tests/scenarios/recognition-tasks/" + taskID + "/cancel",
			params: gin.Params{
				{Key: "agent_id", Value: agentID},
				{Key: "task_id", Value: taskID},
			},
			call: (*Handler).CancelScenarioRecognitionTask,
		},
		{
			name:   "generation task cancel",
			method: http.MethodPost,
			target: "/agents/" + agentID + "/workflow-tests/cases/generation-tasks/" + taskID + "/cancel",
			params: gin.Params{
				{Key: "agent_id", Value: agentID},
				{Key: "task_id", Value: taskID},
			},
			call: (*Handler).CancelGenerationTask,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			workflowService := &workflowTestPermissionWorkspaceResolver{workspaceID: "workspace-1"}
			permissionChecker := &workflowTestPermissionChecker{allowed: false}
			handler := &Handler{
				agentWorkspaceResolver: workflowService,
				organizationService:    permissionChecker,
			}
			ctx, recorder := newWorkflowTestPermissionContext(tt.method, tt.target, "account-1", "org-1", tt.params)

			tt.call(handler, ctx)

			require.Equal(t, http.StatusForbidden, recorder.Code)
			requireResponseCode(t, recorder, response.ErrPermissionDenied)
			require.Equal(t, agentID, workflowService.lastAgentID)
			require.Equal(t, "org-1", permissionChecker.lastOrganizationID)
			require.Equal(t, "workspace-1", permissionChecker.lastWorkspaceID)
			require.Equal(t, "account-1", permissionChecker.lastAccountID)
			require.Equal(t, workspace_model.WorkspacePermissionAgentManage, permissionChecker.lastPermission)
		})
	}
}

func TestHandlerUpdateCaseRequiresAgentManagePermission(t *testing.T) {
	agentID := "11111111-1111-1111-1111-111111111111"
	caseID := "33333333-3333-3333-3333-333333333333"
	permissionChecker := &workflowTestPermissionChecker{allowed: false}
	handler := &Handler{
		agentWorkspaceResolver: &workflowTestPermissionWorkspaceResolver{workspaceID: "workspace-1"},
		organizationService:    permissionChecker,
	}
	ctx, recorder := newWorkflowTestPermissionContext(http.MethodPut, "/agents/"+agentID+"/workflow-tests/cases/"+caseID, "account-1", "org-1", gin.Params{
		{Key: "agent_id", Value: agentID},
		{Key: "case_id", Value: caseID},
	})

	handler.UpdateCase(ctx)

	require.Equal(t, http.StatusForbidden, recorder.Code)
	requireResponseCode(t, recorder, response.ErrPermissionDenied)
	require.Equal(t, workspace_model.WorkspacePermissionAgentManage, permissionChecker.lastPermission)
}

func TestHandlerMutationRoutesRequireAgentManageBeforeSubresourceIDValidation(t *testing.T) {
	agentID := "11111111-1111-1111-1111-111111111111"
	tests := []struct {
		name   string
		method string
		target string
		params gin.Params
		call   func(*Handler, *gin.Context)
	}{
		{
			name:   "update case",
			method: http.MethodPut,
			target: "/agents/" + agentID + "/workflow-tests/cases/not-a-uuid",
			params: gin.Params{
				{Key: "agent_id", Value: agentID},
				{Key: "case_id", Value: "not-a-uuid"},
			},
			call: (*Handler).UpdateCase,
		},
		{
			name:   "delete case",
			method: http.MethodDelete,
			target: "/agents/" + agentID + "/workflow-tests/cases/not-a-uuid",
			params: gin.Params{
				{Key: "agent_id", Value: agentID},
				{Key: "case_id", Value: "not-a-uuid"},
			},
			call: (*Handler).DeleteCase,
		},
		{
			name:   "retest batch",
			method: http.MethodPost,
			target: "/agents/" + agentID + "/workflow-tests/batches/not-a-uuid/retest",
			params: gin.Params{
				{Key: "agent_id", Value: agentID},
				{Key: "batch_id", Value: "not-a-uuid"},
			},
			call: (*Handler).RetestBatch,
		},
		{
			name:   "start batch",
			method: http.MethodPost,
			target: "/agents/" + agentID + "/workflow-tests/batches/not-a-uuid/start",
			params: gin.Params{
				{Key: "agent_id", Value: agentID},
				{Key: "batch_id", Value: "not-a-uuid"},
			},
			call: (*Handler).StartBatch,
		},
		{
			name:   "execute batch",
			method: http.MethodPost,
			target: "/agents/" + agentID + "/workflow-tests/batches/not-a-uuid/execute",
			params: gin.Params{
				{Key: "agent_id", Value: agentID},
				{Key: "batch_id", Value: "not-a-uuid"},
			},
			call: (*Handler).ExecuteBatch,
		},
		{
			name:   "cancel batch",
			method: http.MethodPost,
			target: "/agents/" + agentID + "/workflow-tests/batches/not-a-uuid/cancel",
			params: gin.Params{
				{Key: "agent_id", Value: agentID},
				{Key: "batch_id", Value: "not-a-uuid"},
			},
			call: (*Handler).CancelBatch,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			permissionChecker := &workflowTestPermissionChecker{allowed: false}
			handler := &Handler{
				agentWorkspaceResolver: &workflowTestPermissionWorkspaceResolver{workspaceID: "workspace-1"},
				organizationService:    permissionChecker,
			}
			ctx, recorder := newWorkflowTestPermissionContext(tt.method, tt.target, "account-1", "org-1", tt.params)

			tt.call(handler, ctx)

			require.Equal(t, http.StatusForbidden, recorder.Code)
			requireResponseCode(t, recorder, response.ErrPermissionDenied)
			require.Equal(t, workspace_model.WorkspacePermissionAgentManage, permissionChecker.lastPermission)
		})
	}
}

func TestHandlerMutationRoutesRequireAgentManageBeforeBindingRequest(t *testing.T) {
	agentID := "11111111-1111-1111-1111-111111111111"
	batchID := "22222222-2222-2222-2222-222222222222"
	tests := []struct {
		name   string
		method string
		target string
		params gin.Params
		call   func(*Handler, *gin.Context)
	}{
		{
			name:   "update settings",
			method: http.MethodPut,
			target: "/agents/" + agentID + "/workflow-tests/settings",
			params: gin.Params{{Key: "agent_id", Value: agentID}},
			call:   (*Handler).UpdateSettings,
		},
		{
			name:   "create scenario",
			method: http.MethodPost,
			target: "/agents/" + agentID + "/workflow-tests/scenarios",
			params: gin.Params{{Key: "agent_id", Value: agentID}},
			call:   (*Handler).CreateScenario,
		},
		{
			name:   "save scenarios",
			method: http.MethodPut,
			target: "/agents/" + agentID + "/workflow-tests/scenarios",
			params: gin.Params{{Key: "agent_id", Value: agentID}},
			call:   (*Handler).SaveScenarios,
		},
		{
			name:   "recognize scenarios",
			method: http.MethodPost,
			target: "/agents/" + agentID + "/workflow-tests/scenarios/recognize",
			params: gin.Params{{Key: "agent_id", Value: agentID}},
			call:   (*Handler).RecognizeScenarios,
		},
		{
			name:   "create scenario recognition task",
			method: http.MethodPost,
			target: "/agents/" + agentID + "/workflow-tests/scenarios/recognition-tasks",
			params: gin.Params{{Key: "agent_id", Value: agentID}},
			call:   (*Handler).CreateScenarioRecognitionTask,
		},
		{
			name:   "create case",
			method: http.MethodPost,
			target: "/agents/" + agentID + "/workflow-tests/cases",
			params: gin.Params{{Key: "agent_id", Value: agentID}},
			call:   (*Handler).CreateCase,
		},
		{
			name:   "delete cases",
			method: http.MethodDelete,
			target: "/agents/" + agentID + "/workflow-tests/cases",
			params: gin.Params{{Key: "agent_id", Value: agentID}},
			call:   (*Handler).DeleteCases,
		},
		{
			name:   "generate cases",
			method: http.MethodPost,
			target: "/agents/" + agentID + "/workflow-tests/cases/generate",
			params: gin.Params{{Key: "agent_id", Value: agentID}},
			call:   (*Handler).GenerateCases,
		},
		{
			name:   "create generation task",
			method: http.MethodPost,
			target: "/agents/" + agentID + "/workflow-tests/cases/generation-tasks",
			params: gin.Params{{Key: "agent_id", Value: agentID}},
			call:   (*Handler).CreateGenerationTask,
		},
		{
			name:   "create batch",
			method: http.MethodPost,
			target: "/agents/" + agentID + "/workflow-tests/batches",
			params: gin.Params{{Key: "agent_id", Value: agentID}},
			call:   (*Handler).CreateBatch,
		},
		{
			name:   "retest batch",
			method: http.MethodPost,
			target: "/agents/" + agentID + "/workflow-tests/batches/" + batchID + "/retest",
			params: gin.Params{
				{Key: "agent_id", Value: agentID},
				{Key: "batch_id", Value: batchID},
			},
			call: (*Handler).RetestBatch,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			permissionChecker := &workflowTestPermissionChecker{allowed: false}
			handler := &Handler{
				agentWorkspaceResolver: &workflowTestPermissionWorkspaceResolver{workspaceID: "workspace-1"},
				organizationService:    permissionChecker,
			}
			ctx, recorder := newWorkflowTestPermissionBodyContext(tt.method, tt.target, "account-1", "org-1", tt.params, `{"broken":`)

			tt.call(handler, ctx)

			require.Equal(t, http.StatusForbidden, recorder.Code)
			requireResponseCode(t, recorder, response.ErrPermissionDenied)
			require.Equal(t, workspace_model.WorkspacePermissionAgentManage, permissionChecker.lastPermission)
		})
	}
}

func TestHandlerWorkflowTestAccessRejectsAgentWithoutWorkspace(t *testing.T) {
	agentID := "11111111-1111-1111-1111-111111111111"
	batchID := "22222222-2222-2222-2222-222222222222"
	permissionChecker := &workflowTestPermissionChecker{allowed: true}
	handler := &Handler{
		agentWorkspaceResolver: &workflowTestPermissionWorkspaceResolver{workspaceID: ""},
		organizationService:    permissionChecker,
	}
	ctx, recorder := newWorkflowTestPermissionContext(http.MethodGet, "/agents/"+agentID+"/workflow-tests/batches/"+batchID+"/items", "account-1", "org-1", gin.Params{
		{Key: "agent_id", Value: agentID},
		{Key: "batch_id", Value: batchID},
	})

	handler.ListBatchItems(ctx)

	require.Equal(t, http.StatusNotFound, recorder.Code)
	requireResponseCode(t, recorder, response.ErrWorkspaceNotFound)
	require.Zero(t, permissionChecker.calls)
}

func newWorkflowTestPermissionContext(method, target, accountID, organizationID string, params gin.Params) (*gin.Context, *httptest.ResponseRecorder) {
	return newWorkflowTestPermissionBodyContext(method, target, accountID, organizationID, params, "")
}

func newWorkflowTestPermissionBodyContext(method, target, accountID, organizationID string, params gin.Params, body string) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(method, target, strings.NewReader(body))
	if body != "" {
		ctx.Request.Header.Set("Content-Type", "application/json")
	}
	ctx.Params = params
	ctx.Set("account_id", accountID)
	util.SetOrganizationID(ctx, organizationID)
	return ctx, recorder
}

func requireResponseCode(t *testing.T, recorder *httptest.ResponseRecorder, expected response.ErrorCode) {
	t.Helper()
	var body response.Response
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body))
	require.Equal(t, strconv.Itoa(expected.Code), body.Code)
}

type workflowTestPermissionWorkspaceResolver struct {
	workspaceID string
	err         error
	lastAgentID string
}

func (r *workflowTestPermissionWorkspaceResolver) GetAgentWorkspaceID(_ context.Context, agentID string) (string, error) {
	r.lastAgentID = agentID
	return r.workspaceID, r.err
}

type workflowTestPermissionChecker struct {
	allowed            bool
	err                error
	calls              int
	lastOrganizationID string
	lastWorkspaceID    string
	lastAccountID      string
	lastPermission     workspace_model.WorkspacePermissionCode
}

func (c *workflowTestPermissionChecker) CheckWorkspacePermission(_ context.Context, organizationID, workspaceID, accountID string, permissionCode workspace_model.WorkspacePermissionCode) (bool, error) {
	c.calls++
	c.lastOrganizationID = organizationID
	c.lastWorkspaceID = workspaceID
	c.lastAccountID = accountID
	c.lastPermission = permissionCode
	return c.allowed, c.err
}
