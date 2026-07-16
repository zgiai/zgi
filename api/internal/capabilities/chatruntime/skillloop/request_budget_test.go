package skillloop

import (
	"encoding/json"
	"strings"
	"testing"

	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/modules/skills"
)

func TestFinalPlanningRequestBudgetCompactsHistoryAndPreservesRequiredEvidence(t *testing.T) {
	long := func(marker string, count int) string {
		return marker + strings.Repeat("x", count)
	}
	toolCall := func(id string, name string, arguments string) adapter.Message {
		return adapter.Message{
			Role: "assistant",
			ToolCalls: []adapter.ToolCall{{
				ID: id,
				Function: adapter.FunctionCall{
					Name:      name,
					Arguments: mustBudgetJSON(t, map[string]interface{}{"content": arguments}),
				},
			}},
		}
	}
	stateMessage := "Current turn structured state: turn_state=" + long("DUPLICATE_STATE_", 900)
	source := []adapter.Message{
		{Role: "system", Content: "base system instructions"},
		{Role: "system", Content: stateMessage},
		{Role: "system", Content: stateMessage},
		{Role: "system", Content: strings.Join([]string{
			"The following skill instructions were loaded earlier in this same user turn and remain active.",
			"Restored skill: target-skill\nDescription: target\nInstructions:\n" + long("TARGET_INSTRUCTIONS_", 500),
			"Restored skill: other-skill\nDescription: other\nInstructions:\n" + long("OTHER_INSTRUCTIONS_", 1600),
		}, "\n\n")},
		{Role: "user", Content: "CURRENT_USER_GOAL must remain"},
		toolCall("state-1", skills.MetaToolTurnState, long("OLD_TURN_STATE_ARGUMENTS_", 900)),
		{Role: "tool", ToolCallID: "state-1", Content: long("OLD_TURN_STATE_RESULT_", 900)},
		toolCall("intermediate-1", skills.MetaToolIntermediateAnswer, long("OLD_INTERMEDIATE_ANSWER_", 1000)),
		{Role: "tool", ToolCallID: "intermediate-1", Content: long("OLD_INTERMEDIATE_RESULT_", 900)},
		toolCall("old-tool", "call_skill_tool", long("OLD_TOOL_ARGUMENTS_", 1000)),
		{Role: "tool", ToolCallID: "old-tool", Content: long("OLD_TOOL_RESULT_", 1000)},
		toolCall("latest-tool", "call_skill_tool", "LATEST_ARGUMENT must remain"),
		{Role: "tool", ToolCallID: "latest-tool", Content: "LATEST_EVIDENCE must remain"},
	}
	maxTokens := 5000
	request := &adapter.ChatRequest{
		Model:     "deepseek-chat",
		Messages:  adapter.NormalizeSystemMessages(source),
		MaxTokens: &maxTokens,
	}
	runner := &Runner{requestBudget: planningRequestBudget{
		safeContextLimit:       1800,
		promptBudget:           1,
		preferredRestoredSkill: "target-skill",
	}}

	if err := runner.applyFinalPlanningRequestBudget(request, source); err != nil {
		t.Fatalf("applyFinalPlanningRequestBudget() error = %v", err)
	}

	encoded, err := json.Marshal(request.Messages)
	if err != nil {
		t.Fatalf("marshal compacted messages: %v", err)
	}
	payload := string(encoded)
	for _, required := range []string{"CURRENT_USER_GOAL", "TARGET_INSTRUCTIONS_", "LATEST_ARGUMENT", "LATEST_EVIDENCE"} {
		if !strings.Contains(payload, required) {
			t.Fatalf("compacted request lost %q: %s", required, payload)
		}
	}
	for _, removed := range []string{"OTHER_INSTRUCTIONS_", "OLD_TURN_STATE_ARGUMENTS_", "OLD_TURN_STATE_RESULT_", "OLD_INTERMEDIATE_ANSWER_", "OLD_INTERMEDIATE_RESULT_", "OLD_TOOL_ARGUMENTS_", "OLD_TOOL_RESULT_"} {
		if strings.Contains(payload, removed) {
			t.Fatalf("compacted request retained %q", removed)
		}
	}
	diagnostics := runner.diagnostics.requestBudget
	for _, component := range []string{
		budgetComponentTurnEvidence,
		budgetComponentRestoredSkills,
		budgetComponentHistoricalTools,
		budgetComponentIntermediateAnswer,
	} {
		if diagnostics.compressionChars[component] <= 0 {
			t.Fatalf("compression chars[%q] = %d, all=%#v", component, diagnostics.compressionChars[component], diagnostics.compressionChars)
		}
	}
	if diagnostics.finalPromptTokens >= diagnostics.originalPromptTokens {
		t.Fatalf("prompt tokens before=%d after=%d, want reduction", diagnostics.originalPromptTokens, diagnostics.finalPromptTokens)
	}
	if diagnostics.finalPromptTokens >= runner.requestBudget.safeContextLimit {
		t.Fatalf("final prompt tokens = %d, safe limit = %d", diagnostics.finalPromptTokens, runner.requestBudget.safeContextLimit)
	}
	if !diagnostics.maxTokensClamped || request.MaxTokens == nil {
		t.Fatalf("max token diagnostics = %#v request=%#v", diagnostics, request.MaxTokens)
	}
	if got, want := *request.MaxTokens, runner.requestBudget.safeContextLimit-diagnostics.finalPromptTokens; got != want {
		t.Fatalf("MaxTokens = %d, want remaining budget %d", got, want)
	}
}

func TestFinalPlanningRequestBudgetRejectsUncompressibleRequest(t *testing.T) {
	maxTokens := 1000
	request := &adapter.ChatRequest{
		Model:     "deepseek-chat",
		Messages:  []adapter.Message{{Role: "user", Content: "current goal"}},
		MaxTokens: &maxTokens,
		Tools: []adapter.Tool{{
			Type: "function",
			Function: adapter.Function{
				Name:        "oversized_tool",
				Description: strings.Repeat("schema", 1000),
			},
		}},
	}
	runner := &Runner{requestBudget: planningRequestBudget{safeContextLimit: 100, promptBudget: 50}}

	err := runner.applyFinalPlanningRequestBudget(request, request.Messages)
	if err == nil || !strings.Contains(err.Error(), "exceeds safe context limit") {
		t.Fatalf("error = %v, want safe context limit rejection", err)
	}
	if runner.diagnostics.requestBudget.finalPromptTokens < runner.requestBudget.safeContextLimit {
		t.Fatalf("final prompt tokens = %d, want over safe limit", runner.diagnostics.requestBudget.finalPromptTokens)
	}
}

func TestPlanningRequestBudgetUsesOnlyMatchingVersionedCalibration(t *testing.T) {
	metadata := map[string]interface{}{
		"context_control": map[string]interface{}{
			"safe_context_limit": 10000,
			"prompt_budget":      8000,
		},
		"prompt_usage_calibration": map[string]interface{}{
			"provider-a/model-a": map[string]interface{}{
				"estimate_version":      "chat_request.v1",
				"prompt_estimate_scale": 2.5,
			},
			"provider-b/model-a": map[string]interface{}{
				"estimate_version":      "chat_request.v1",
				"prompt_estimate_scale": 0.5,
			},
			"provider-a/legacy": map[string]interface{}{
				"prompt_estimate_scale": 9.0,
			},
		},
	}
	requestFor := func(provider string, model string) RunRequest {
		return RunRequest{
			Prepared: NewPreparedChat("conversation", "message", provider, "auto", &adapter.ChatRequest{
				Provider: provider,
				Model:    model,
			}),
			CurrentMetadata: func() map[string]interface{} { return metadata },
		}
	}

	if got := planningRequestBudgetForRun(requestFor("provider-a", "model-a")).estimateScale; got != 2.5 {
		t.Fatalf("provider A scale = %v, want 2.5", got)
	}
	if got := planningRequestBudgetForRun(requestFor("provider-b", "model-a")).estimateScale; got != 0.5 {
		t.Fatalf("provider B scale = %v, want 0.5", got)
	}
	if got := planningRequestBudgetForRun(requestFor("provider-a", "legacy")).estimateScale; got != 1 {
		t.Fatalf("legacy unversioned scale = %v, want 1", got)
	}
}

func mustBudgetJSON(t *testing.T, value interface{}) string {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal value: %v", err)
	}
	return string(encoded)
}
