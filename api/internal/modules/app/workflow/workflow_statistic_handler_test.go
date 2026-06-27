package workflow

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	workspace_model "github.com/zgiai/zgi/api/internal/modules/workspace/model"
	"github.com/zgiai/zgi/api/internal/util"
)

func TestWorkflowStatisticHandlersRequireWorkflowStatsViewBeforeQueryBinding(t *testing.T) {
	gin.SetMode(gin.TestMode)

	endpoints := []struct {
		name string
		call func(*WorkflowStatisticHandler, *gin.Context)
	}{
		{name: "daily runs", call: (*WorkflowStatisticHandler).GetWorkflowDailyRuns},
		{name: "daily terminals", call: (*WorkflowStatisticHandler).GetWorkflowDailyTerminals},
		{name: "daily token cost", call: (*WorkflowStatisticHandler).GetWorkflowDailyTokenCost},
		{name: "average app interaction", call: (*WorkflowStatisticHandler).GetWorkflowAverageAppInteraction},
	}

	for _, endpoint := range endpoints {
		t.Run(endpoint.name, func(t *testing.T) {
			service := &workflowStatisticAccessService{workspaceID: "workspace-1"}
			permissionChecker := &workflowStatisticPermissionChecker{allowed: false}
			handler := NewWorkflowStatisticHandler(service, WithWorkflowStatisticAuthorization(permissionChecker))
			ctx, recorder := newWorkflowStatisticContext("/agents/agent-1/statistics?start=not-a-date")

			endpoint.call(handler, ctx)

			if recorder.Code != http.StatusForbidden {
				t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusForbidden, recorder.Body.String())
			}
			if !permissionChecker.checked {
				t.Fatalf("expected workflow.stats.view permission check before query binding")
			}
			if permissionChecker.lastPermission != workspace_model.WorkspacePermissionWorkflowStatsView {
				t.Fatalf("permission = %q, want %q", permissionChecker.lastPermission, workspace_model.WorkspacePermissionWorkflowStatsView)
			}
			if service.statisticCalls != 0 {
				t.Fatalf("statistic calls = %d, want 0 before permission passes", service.statisticCalls)
			}
		})
	}
}

func TestWorkflowStatisticUsesRouteAgentWorkspace(t *testing.T) {
	gin.SetMode(gin.TestMode)

	service := &workflowStatisticAccessService{workspaceID: "workspace-1"}
	permissionChecker := &workflowStatisticPermissionChecker{allowed: true}
	handler := NewWorkflowStatisticHandler(service, WithWorkflowStatisticAuthorization(permissionChecker))
	ctx, recorder := newWorkflowStatisticContext("/agents/agent-1/statistics")
	ctx.Set("tenant_id", "ambient-workspace")

	handler.GetWorkflowDailyRuns(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	if service.lastStatisticWorkspaceID != "workspace-1" {
		t.Fatalf("statistic workspace = %q, want route agent workspace", service.lastStatisticWorkspaceID)
	}
	if service.lastStatisticAgentID != "agent-1" {
		t.Fatalf("statistic agent = %q, want agent-1", service.lastStatisticAgentID)
	}
	if permissionChecker.lastWorkspaceID != "workspace-1" {
		t.Fatalf("permission workspace = %q, want workspace-1", permissionChecker.lastWorkspaceID)
	}
	if service.statisticCalls != 1 {
		t.Fatalf("statistic calls = %d, want 1", service.statisticCalls)
	}
}

func newWorkflowStatisticContext(target string) (*gin.Context, *httptest.ResponseRecorder) {
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, target, nil)
	ctx.Params = gin.Params{{Key: "agent_id", Value: "agent-1"}}
	ctx.Set("account_id", "account-1")
	util.SetOrganizationID(ctx, "org-1")
	return ctx, recorder
}

type workflowStatisticAccessService struct {
	workspaceID              string
	statisticCalls           int
	lastStatisticWorkspaceID string
	lastStatisticAgentID     string
}

func (s *workflowStatisticAccessService) GetAgentWorkspaceID(_ context.Context, _ string) (string, error) {
	return s.workspaceID, nil
}

func (s *workflowStatisticAccessService) GetWorkflowDailyRuns(_ context.Context, workspaceID, agentID string) (interface{}, error) {
	s.recordStatisticCall(workspaceID, agentID)
	return map[string]interface{}{"daily_runs": []interface{}{}}, nil
}

func (s *workflowStatisticAccessService) GetWorkflowDailyTerminals(_ context.Context, workspaceID, agentID string) (interface{}, error) {
	s.recordStatisticCall(workspaceID, agentID)
	return map[string]interface{}{"daily_terminals": []interface{}{}}, nil
}

func (s *workflowStatisticAccessService) GetWorkflowDailyTokenCost(_ context.Context, workspaceID, agentID string) (interface{}, error) {
	s.recordStatisticCall(workspaceID, agentID)
	return map[string]interface{}{"daily_token_costs": []interface{}{}}, nil
}

func (s *workflowStatisticAccessService) GetWorkflowAverageAppInteraction(_ context.Context, workspaceID, agentID string) (interface{}, error) {
	s.recordStatisticCall(workspaceID, agentID)
	return map[string]interface{}{"average_interactions": 0}, nil
}

func (s *workflowStatisticAccessService) recordStatisticCall(workspaceID, agentID string) {
	s.statisticCalls++
	s.lastStatisticWorkspaceID = workspaceID
	s.lastStatisticAgentID = agentID
}

type workflowStatisticPermissionChecker struct {
	allowed         bool
	checked         bool
	lastWorkspaceID string
	lastPermission  workspace_model.WorkspacePermissionCode
}

func (c *workflowStatisticPermissionChecker) CheckWorkspacePermission(_ context.Context, _ string, workspaceID, _ string, permissionCode workspace_model.WorkspacePermissionCode) (bool, error) {
	c.checked = true
	c.lastWorkspaceID = workspaceID
	c.lastPermission = permissionCode
	return c.allowed, nil
}
