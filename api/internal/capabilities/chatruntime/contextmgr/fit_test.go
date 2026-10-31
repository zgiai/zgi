package contextmgr

import (
	"context"
	"strings"
	"testing"

	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

func TestPromptBudgetLeavesRoomForTheAnswer(t *testing.T) {
	budget, err := budgetForRequest(Config{ModelContextWindow: 128_000, MaxOutputTokens: 64_000}, 64_000)
	if err != nil {
		t.Fatal(err)
	}
	if budget.PromptBudget != 64_000 {
		t.Fatalf("prompt budget = %d, want the window minus the 64K answer", budget.PromptBudget)
	}
}

// The provider's usage calibrates the conservative estimate: a model that
// counts fewer tokens than estimated gets a larger budget in estimated tokens.
func TestUsageCalibratesTheBudget(t *testing.T) {
	ctx := context.Background()
	manager, err := New(Config{AgentRunID: "run-scale", ModelContextWindow: 100_000, MaxOutputTokens: 4_000}, nil, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	request := &adapter.ChatRequest{Model: "deepseek-v4-flash", MaxTokens: intPointerForTest(4_000), Messages: []adapter.Message{{Role: "user", Content: "hi"}}}
	_, before, err := manager.PrepareBeforeModelCall(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.ObserveModelResponse(ctx, adapter.Message{Role: "assistant", Content: "hello"}, &adapter.Usage{PromptTokens: max(1, before.FinalPromptTokens/2)}); err != nil {
		t.Fatal(err)
	}
	_, after, err := manager.PrepareBeforeModelCall(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	if after.EstimateScale >= 1 || after.Budget.PromptBudget <= before.Budget.PromptBudget {
		t.Fatalf("scale %.2f budget %d -> %d, want a smaller scale and a larger budget", after.EstimateScale, before.Budget.PromptBudget, after.Budget.PromptBudget)
	}
	if got := clampEstimateScale(0.01); got != minEstimateScale {
		t.Fatalf("scale clamp = %v", got)
	}
}

// Without a compactor, history that does not fit is left out whole turns at a
// time, oldest first, so every tool call keeps its result.
func TestHistoryThatDoesNotFitIsLeftOutByWholeTurns(t *testing.T) {
	manager, err := New(Config{AgentRunID: "run-drop", ModelContextWindow: 40_000, MaxOutputTokens: 4_000}, nil, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	old := strings.Repeat("old evidence ", 5_000)
	prepared, decision, err := manager.PrepareBeforeModelCall(context.Background(), &adapter.ChatRequest{
		Model: "test-model", MaxTokens: intPointerForTest(4_000),
		Messages: []adapter.Message{
			{Role: "system", Content: "persona"},
			{Role: "user", Content: "first question"},
			{Role: "assistant", ToolCalls: []adapter.ToolCall{{ID: "a", Type: "function", Function: adapter.FunctionCall{Name: "read", Arguments: "{}"}}}},
			{Role: "tool", ToolCallID: "a", Content: "rows"},
			{Role: "assistant", Content: "first answer " + old},
			{Role: "user", Content: "second question"},
			{Role: "assistant", ToolCalls: []adapter.ToolCall{{ID: "b", Type: "function", Function: adapter.FunctionCall{Name: "read", Arguments: "{}"}}}},
			{Role: "tool", ToolCallID: "b", Content: "rows"},
			{Role: "assistant", Content: "second answer " + old},
			{Role: "user", Content: "current question"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if decision.Action != DecisionDropHistory {
		t.Fatalf("action = %s", decision.Action)
	}
	if err := validateToolPairing(prepared.Messages); err != nil {
		t.Fatalf("tool pairing broken: %v", err)
	}
	text := ""
	for _, message := range prepared.Messages {
		text += contentString(message.Content) + "\n"
	}
	if strings.Contains(text, "first question") || !strings.Contains(text, "current question") || !strings.Contains(text, "persona") {
		t.Fatalf("want the oldest turn left out and the persona and current question kept: %.300s", text)
	}
}

// When a request does not fit, the reasoning of earlier turns goes first. The
// current turn keeps its reasoning, and no history is left out when that is
// enough (ADR 0045).
func TestEarlierTurnReasoningIsLeftOutFirst(t *testing.T) {
	manager, err := New(Config{AgentRunID: "run-reasoning-fit", ModelContextWindow: 40_000, MaxOutputTokens: 4_000}, nil, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	long := strings.Repeat("earlier thinking ", 6_000)
	messages := []adapter.Message{
		{Role: "system", Content: "persona"},
		{Role: "user", Content: "first question"},
		{Role: "assistant", Content: "first answer", ReasoningContent: long},
		{Role: "user", Content: "current question"},
		{Role: "assistant", ReasoningContent: "current thinking", ToolCalls: []adapter.ToolCall{{ID: "a", Type: "function", Function: adapter.FunctionCall{Name: "read", Arguments: "{}"}}}},
		{Role: "tool", ToolCallID: "a", Content: "rows"},
	}
	prepared, decision, err := manager.PrepareBeforeModelCall(context.Background(), &adapter.ChatRequest{
		Model: "deepseek-v4-flash", MaxTokens: intPointerForTest(4_000), Messages: messages,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !decision.ReasoningDropped || decision.Action == DecisionDropHistory {
		t.Fatalf("decision = %+v, want only the earlier reasoning left out", decision)
	}
	text := ""
	for _, message := range prepared.Messages {
		text += contentString(message.Content) + "|" + message.ReasoningContent + "\n"
	}
	if strings.Contains(text, "earlier thinking") || !strings.Contains(text, "first answer") || !strings.Contains(text, "current thinking") {
		t.Fatalf("want earlier reasoning dropped and the answers and current reasoning kept: %.300s", text)
	}
	if messages[2].ReasoningContent != long {
		t.Fatal("the caller's messages were modified")
	}
}
