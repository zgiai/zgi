package workflow

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	agentspkg "github.com/zgiai/zgi/api/internal/modules/app/agents"
	workspace_model "github.com/zgiai/zgi/api/internal/modules/workspace/model"
)

func TestGetWorkflowRunNodeLogs_ReturnsInputsOutputsAndProcessData(t *testing.T) {
	gin.SetMode(gin.TestMode)

	inputsJSON := `{"sys.query":"hello"}`
	outputsJSON := `{"text":"world"}`
	processDataJSON := `{"llm_gateway_request":{"model":"deepseek-v3","messages":[{"role":"user","content":"hello"}],"params":{"temperature":0.3}}}`
	metadataJSON := `{"total_tokens":42}`
	finishedAt := time.Unix(1700000001, 0)
	createdAt := time.Unix(1700000000, 0)

	repo := &mockWorkflowNodeRuntimeLogRepo{
		logsByWorkflowRunID: []WorkflowNodeRuntimeLog{
			{
				ID:                "node-log-1",
				NodeID:            "llm-node",
				NodeType:          "llm",
				Title:             "LLM",
				Index:             1,
				Status:            "succeeded",
				ElapsedTime:       1.25,
				Inputs:            &inputsJSON,
				Outputs:           &outputsJSON,
				ProcessData:       &processDataJSON,
				ExecutionMetadata: &metadataJSON,
				CreatedAt:         createdAt,
				FinishedAt:        &finishedAt,
			},
		},
	}

	handler := NewRuntimeLogHandler(nil, repo)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Params = gin.Params{
		{Key: "agent_id", Value: "agent-1"},
		{Key: "run_id", Value: "run-1"},
	}
	ctx.Set("account_id", "account-1")
	ctx.Request = httptest.NewRequest(http.MethodPost, "/agents/agent-1/workflow-runs/run-1/nodes", nil)

	handler.GetWorkflowRunNodeLogs(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", recorder.Code)
	}

	var resp map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	payload, ok := resp["data"].(map[string]any)
	if !ok {
		t.Fatalf("expected response data object, got %T", resp["data"])
	}
	items, ok := payload["data"].([]any)
	if !ok || len(items) != 1 {
		t.Fatalf("expected one node log item, got %#v", payload["data"])
	}

	item, ok := items[0].(map[string]any)
	if !ok {
		t.Fatalf("expected node log item object, got %T", items[0])
	}
	if _, ok := item["inputs"].(map[string]any); !ok {
		t.Fatalf("expected inputs in response, got %T", item["inputs"])
	}
	if _, ok := item["outputs"].(map[string]any); !ok {
		t.Fatalf("expected outputs in response, got %T", item["outputs"])
	}

	processData, ok := item["process_data"].(map[string]any)
	if !ok {
		t.Fatalf("expected process_data in response, got %T", item["process_data"])
	}
	gatewayRequest, ok := processData["llm_gateway_request"].(map[string]any)
	if !ok {
		t.Fatalf("expected llm_gateway_request in response, got %T", processData["llm_gateway_request"])
	}
	if got := gatewayRequest["model"]; got != "deepseek-v3" {
		t.Fatalf("expected llm_gateway_request.model=deepseek-v3, got %v", got)
	}
	params, ok := gatewayRequest["params"].(map[string]any)
	if !ok {
		t.Fatalf("expected llm_gateway_request.params in response, got %T", gatewayRequest["params"])
	}
	if got := params["temperature"]; got != 0.3 {
		t.Fatalf("expected llm_gateway_request.params.temperature=0.3, got %v", got)
	}
	if got, ok := item["elapsed_time"].(float64); !ok || math.Abs(got-1250) > 0.000001 {
		t.Fatalf("expected elapsed_time to be normalized to 1250 ms, got %#v", item["elapsed_time"])
	}
}

func TestGetWorkflowRunNodeLogs_FiltersFrontendInputs(t *testing.T) {
	gin.SetMode(gin.TestMode)

	inputsJSON := `{"url":"http://baidu.com","method":"GET","header":{},"param":{},"body":null,"auth":null,"timeout":{"read":60},"sys.user_id":"u1"}`
	outputsJSON := `{"status_code":200}`
	createdAt := time.Unix(1700000000, 0)

	repo := &mockWorkflowNodeRuntimeLogRepo{
		logsByWorkflowRunID: []WorkflowNodeRuntimeLog{
			{
				ID:          "node-log-1",
				NodeID:      "http-node",
				NodeType:    "http-request",
				Title:       "HTTP Request",
				Index:       1,
				Status:      "succeeded",
				ElapsedTime: 1,
				Inputs:      &inputsJSON,
				Outputs:     &outputsJSON,
				CreatedAt:   createdAt,
			},
		},
	}

	handler := NewRuntimeLogHandler(nil, repo)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Params = gin.Params{
		{Key: "agent_id", Value: "agent-1"},
		{Key: "run_id", Value: "run-1"},
	}
	ctx.Set("account_id", "account-1")
	ctx.Request = httptest.NewRequest(http.MethodPost, "/agents/agent-1/workflow-runs/run-1/nodes", nil)

	handler.GetWorkflowRunNodeLogs(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", recorder.Code)
	}

	var resp map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	payload := resp["data"].(map[string]any)
	items := payload["data"].([]any)
	item := items[0].(map[string]any)
	inputs := item["inputs"].(map[string]any)

	for _, key := range []string{"url", "method", "header", "param", "auth"} {
		if _, exists := inputs[key]; !exists {
			t.Fatalf("expected key %s in runtime log inputs: %#v", key, inputs)
		}
	}
	for _, key := range []string{"body", "timeout", "sys.user_id"} {
		if _, exists := inputs[key]; exists {
			t.Fatalf("key %s should be removed from runtime log inputs: %#v", key, inputs)
		}
	}
}

func TestGetRuntimeLogsRequiresWorkflowLogsViewBeforeLogQuery(t *testing.T) {
	gin.SetMode(gin.TestMode)

	workspaceID := uuid.New()
	agentID := uuid.New()
	runRepo := &mockWorkflowRunLogRepo{
		createdLogs: []WorkflowRunLog{{ID: "run-1", AgentID: agentID.String()}},
	}
	permissionChecker := &fakeRuntimeLogWorkspacePermissionChecker{allowed: false}
	handler := NewRuntimeLogHandler(runRepo, nil, WithRuntimeLogAuthorization(
		&fakeRuntimeLogAgentResolver{agent: &agentspkg.Agent{ID: agentID, TenantID: workspaceID}},
		permissionChecker,
	))

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Params = gin.Params{{Key: "agent_id", Value: agentID.String()}}
	ctx.Set("account_id", "account-1")
	ctx.Set("organization_id", "org-1")
	ctx.Request = httptest.NewRequest(http.MethodPost, "/agents/"+agentID.String()+"/runtime-logs", strings.NewReader(`{"page":1}`))
	ctx.Request.Header.Set("Content-Type", "application/json")

	handler.GetRuntimeLogs(ctx)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusForbidden, recorder.Body.String())
	}
	if !permissionChecker.checked {
		t.Fatalf("expected workflow.logs.view permission check")
	}
	if runRepo.runtimeLogsCalled != 0 {
		t.Fatalf("runtime log queries = %d, want 0 before permission passes", runRepo.runtimeLogsCalled)
	}
}

func TestGetRuntimeLogsFiltersSystemRunsByCaller(t *testing.T) {
	gin.SetMode(gin.TestMode)

	agentID := uuid.New()
	accountID := uuid.NewString()
	runRepo := &mockWorkflowRunLogRepo{
		createdLogs: []WorkflowRunLog{{ID: "run-1", AgentID: agentID.String(), CreatedBy: accountID}},
	}
	permissionChecker := &fakeRuntimeLogWorkspacePermissionChecker{allowed: false}
	handler := NewRuntimeLogHandler(runRepo, nil, WithRuntimeLogAuthorization(
		&fakeRuntimeLogAgentResolver{agent: &agentspkg.Agent{ID: agentID, TenantID: uuid.Nil}},
		permissionChecker,
	))

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Params = gin.Params{{Key: "agent_id", Value: agentID.String()}}
	ctx.Set("account_id", accountID)
	ctx.Set("organization_id", "org-1")
	ctx.Request = httptest.NewRequest(http.MethodPost, "/agents/"+agentID.String()+"/runtime-logs", strings.NewReader(`{"page":2,"limit":5,"triggered_from":"web-app"}`))
	ctx.Request.Header.Set("Content-Type", "application/json")

	handler.GetRuntimeLogs(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	if permissionChecker.checked {
		t.Fatalf("system workflow runtime logs should not require workspace permission")
	}
	if runRepo.runtimeLogsCalled != 1 {
		t.Fatalf("runtime log queries = %d, want 1", runRepo.runtimeLogsCalled)
	}
	filter := runRepo.lastRuntimeFilter
	if filter.AgentID != agentID.String() {
		t.Fatalf("filter.AgentID = %q, want %q", filter.AgentID, agentID.String())
	}
	if filter.CreatedBy != accountID {
		t.Fatalf("filter.CreatedBy = %q, want %q", filter.CreatedBy, accountID)
	}
	if filter.TriggeredFrom != "web-app" {
		t.Fatalf("filter.TriggeredFrom = %q, want web-app", filter.TriggeredFrom)
	}
	if !filter.ExcludeDebug {
		t.Fatalf("filter.ExcludeDebug = false, want true")
	}
	if runRepo.lastRuntimePage != 2 || runRepo.lastRuntimeLimit != 5 {
		t.Fatalf("page/limit = %d/%d, want 2/5", runRepo.lastRuntimePage, runRepo.lastRuntimeLimit)
	}
}

func TestGetWorkflowRunNodeLogsRejectsRunFromAnotherAgent(t *testing.T) {
	gin.SetMode(gin.TestMode)

	workspaceID := uuid.New()
	agentID := uuid.New()
	runRepo := &mockWorkflowRunLogRepo{
		runsByID: map[string]*WorkflowRunLog{
			"run-1": {ID: "run-1", AgentID: uuid.NewString()},
		},
	}
	nodeRepo := &mockWorkflowNodeRuntimeLogRepo{
		logsByWorkflowRunID: []WorkflowNodeRuntimeLog{
			{ID: "node-log-1", AgentID: "other-agent"},
		},
	}
	handler := NewRuntimeLogHandler(runRepo, nodeRepo, WithRuntimeLogAuthorization(
		&fakeRuntimeLogAgentResolver{agent: &agentspkg.Agent{ID: agentID, TenantID: workspaceID}},
		&fakeRuntimeLogWorkspacePermissionChecker{allowed: true},
	))

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Params = gin.Params{
		{Key: "agent_id", Value: agentID.String()},
		{Key: "run_id", Value: "run-1"},
	}
	ctx.Set("account_id", "account-1")
	ctx.Request = httptest.NewRequest(http.MethodPost, "/agents/agent-1/workflow-runs/run-1/nodes", nil)

	handler.GetWorkflowRunNodeLogs(ctx)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNotFound)
	}
	if runRepo.getByIDCalls == 0 {
		t.Fatalf("expected run lookup after permission passed")
	}
}

func TestGetWorkflowRunNodeLogsRequiresWorkflowLogsViewPermission(t *testing.T) {
	gin.SetMode(gin.TestMode)

	workspaceID := uuid.New()
	agentID := uuid.New()
	runRepo := &mockWorkflowRunLogRepo{
		runsByID: map[string]*WorkflowRunLog{
			"run-1": {ID: "run-1", AgentID: agentID.String()},
		},
	}
	nodeRepo := &mockWorkflowNodeRuntimeLogRepo{}
	permissionChecker := &fakeRuntimeLogWorkspacePermissionChecker{allowed: false}
	handler := NewRuntimeLogHandler(runRepo, nodeRepo, WithRuntimeLogAuthorization(
		&fakeRuntimeLogAgentResolver{agent: &agentspkg.Agent{ID: agentID, TenantID: workspaceID}},
		permissionChecker,
	))

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Params = gin.Params{
		{Key: "agent_id", Value: agentID.String()},
		{Key: "run_id", Value: "run-1"},
	}
	ctx.Set("account_id", "account-1")
	ctx.Set("organization_id", "org-1")
	ctx.Request = httptest.NewRequest(http.MethodPost, "/agents/"+agentID.String()+"/workflow-runs/run-1/nodes", nil)

	handler.GetWorkflowRunNodeLogs(ctx)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusForbidden)
	}
	if !permissionChecker.checked {
		t.Fatalf("expected workflow.logs.view permission check")
	}
}

func TestGetWorkflowRunNodeLogsRequiresWorkflowLogsViewBeforeRunLookup(t *testing.T) {
	gin.SetMode(gin.TestMode)

	workspaceID := uuid.New()
	agentID := uuid.New()
	runRepo := &mockWorkflowRunLogRepo{
		runsByID: map[string]*WorkflowRunLog{
			"run-1": {ID: "run-1", AgentID: agentID.String()},
		},
	}
	nodeRepo := &mockWorkflowNodeRuntimeLogRepo{}
	permissionChecker := &fakeRuntimeLogWorkspacePermissionChecker{allowed: false}
	handler := NewRuntimeLogHandler(runRepo, nodeRepo, WithRuntimeLogAuthorization(
		&fakeRuntimeLogAgentResolver{agent: &agentspkg.Agent{ID: agentID, TenantID: workspaceID}},
		permissionChecker,
	))

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Params = gin.Params{
		{Key: "agent_id", Value: agentID.String()},
		{Key: "run_id", Value: "run-1"},
	}
	ctx.Set("account_id", "account-1")
	ctx.Set("organization_id", "org-1")
	ctx.Request = httptest.NewRequest(http.MethodPost, "/agents/"+agentID.String()+"/workflow-runs/run-1/nodes", nil)

	handler.GetWorkflowRunNodeLogs(ctx)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusForbidden)
	}
	if !permissionChecker.checked {
		t.Fatalf("expected workflow.logs.view permission check")
	}
	if runRepo.getByIDCalls != 0 {
		t.Fatalf("run lookup calls = %d, want 0 before permission passes", runRepo.getByIDCalls)
	}
}

type fakeRuntimeLogAgentResolver struct {
	agent *agentspkg.Agent
}

func (r *fakeRuntimeLogAgentResolver) GetByID(context.Context, string) (*agentspkg.Agent, error) {
	if r.agent == nil {
		return nil, errRuntimeLogAgentNotFound{}
	}
	return r.agent, nil
}

type errRuntimeLogAgentNotFound struct{}

func (errRuntimeLogAgentNotFound) Error() string {
	return "agent not found"
}

type fakeRuntimeLogWorkspacePermissionChecker struct {
	allowed bool
	checked bool
}

func (c *fakeRuntimeLogWorkspacePermissionChecker) CheckWorkspacePermission(_ context.Context, _ string, _ string, _ string, _ workspace_model.WorkspacePermissionCode) (bool, error) {
	c.checked = true
	return c.allowed, nil
}
