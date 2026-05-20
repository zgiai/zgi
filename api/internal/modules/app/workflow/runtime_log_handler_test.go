package workflow

import (
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
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
