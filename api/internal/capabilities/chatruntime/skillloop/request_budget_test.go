package skillloop

import (
	"strings"
	"testing"

	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

func TestFinalPlanningRequestBudgetDoesNotRewriteTranscript(t *testing.T) {
	request := &adapter.ChatRequest{Model: "deepseek-chat", Messages: []adapter.Message{
		{Role: "assistant", ToolCalls: []adapter.ToolCall{{ID: "old", Function: adapter.FunctionCall{Name: "read_file", Arguments: `{"path":"a"}`}}}},
		{Role: "tool", ToolCallID: "old", Content: "OLD_TOOL_RESULT_MUST_REMAIN"},
	}}
	runner := &Runner{requestBudget: planningRequestBudget{safeContextLimit: 10000, promptBudget: 9000}}
	if err := runner.applyFinalPlanningRequestBudget(request, request.Messages); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(messageContent(request.Messages[1].Content), "OLD_TOOL_RESULT_MUST_REMAIN") {
		t.Fatalf("legacy budget path rewrote transcript: %#v", request.Messages)
	}
}

func TestFinalPlanningRequestBudgetRejectsOverLimitRequest(t *testing.T) {
	request := &adapter.ChatRequest{Model: "deepseek-chat", Messages: []adapter.Message{{Role: "user", Content: strings.Repeat("schema", 1000)}}}
	runner := &Runner{requestBudget: planningRequestBudget{safeContextLimit: 100, promptBudget: 50}}
	err := runner.applyFinalPlanningRequestBudget(request, request.Messages)
	if err == nil || !strings.Contains(err.Error(), "exceeds safe context limit") {
		t.Fatalf("error = %v", err)
	}
}

func TestFinalPlanningRequestBudgetUsesInitialReserveInsteadOfRetryCeiling(t *testing.T) {
	request := &adapter.ChatRequest{Model: "qwen3.7-plus", Messages: []adapter.Message{{Role: "user", Content: "current goal"}}}
	runner := &Runner{requestBudget: planningRequestBudget{safeContextLimit: 900000, promptBudget: 883616, initialOutputTokens: 16384, outputTokenLimit: 64000}}
	if err := runner.applyFinalPlanningRequestBudget(request, request.Messages); err != nil {
		t.Fatal(err)
	}
	if request.MaxTokens == nil || *request.MaxTokens != 16384 {
		t.Fatalf("MaxTokens = %#v", request.MaxTokens)
	}
}

func TestFinalPlanningRequestBudgetLeavesNativeOutputLimitToProvider(t *testing.T) {
	metadata := map[string]interface{}{"context_control": map[string]interface{}{"agent_context_window": 900000, "prompt_budget": 883616, "reserved_output_tokens": 16384}}
	request := &adapter.ChatRequest{Model: "deepseek-chat", Messages: []adapter.Message{{Role: "user", Content: "current goal"}}}
	runRequest := RunRequest{Prepared: NewPreparedChat("conversation", "message", "openai", "auto", request), NativeAgentLoop: true, CurrentMetadata: func() map[string]interface{} { return metadata }}
	runner := &Runner{requestBudget: planningRequestBudgetForRun(runRequest)}
	if err := runner.applyFinalPlanningRequestBudget(request, request.Messages); err != nil {
		t.Fatal(err)
	}
	if request.MaxTokens != nil {
		t.Fatalf("MaxTokens = %#v, want nil", request.MaxTokens)
	}
}

func TestPlanningRequestBudgetUsesOnlyMatchingVersionedCalibration(t *testing.T) {
	metadata := map[string]interface{}{
		"context_control": map[string]interface{}{"agent_context_window": 10000, "prompt_budget": 8000},
		"prompt_usage_calibration": map[string]interface{}{
			"provider-a/model-a": map[string]interface{}{"estimate_version": "chat_request.v1", "prompt_estimate_scale": 2.5},
			"provider-a/legacy":  map[string]interface{}{"prompt_estimate_scale": 9.0},
		},
	}
	requestFor := func(model string) RunRequest {
		return RunRequest{Prepared: NewPreparedChat("conversation", "message", "provider-a", "auto", &adapter.ChatRequest{Provider: "provider-a", Model: model}), CurrentMetadata: func() map[string]interface{} { return metadata }}
	}
	if got := planningRequestBudgetForRun(requestFor("model-a")).estimateScale; got != 2.5 {
		t.Fatalf("scale = %v", got)
	}
	if got := planningRequestBudgetForRun(requestFor("legacy")).estimateScale; got != 1 {
		t.Fatalf("legacy scale = %v", got)
	}
}
