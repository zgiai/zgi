package skillloop

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/zgiai/zgi/api/internal/capabilities/chatruntime/contextmgr"
	llmclient "github.com/zgiai/zgi/api/internal/modules/llm/client"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

func TestRunModelToolRoundReactivelyCompactsAndRetriesPromptTooLongOnce(t *testing.T) {
	compactor := &reactiveTestCompactor{}
	manager, err := contextmgr.New(contextmgr.Config{
		AgentRunID:             "reactive-run",
		ConfiguredAgentWindowK: 64,
		ModelContextWindow:     128_000,
		MaxInputTokens:         128_000,
		MaxOutputTokens:        8_000,
		TailMinTextRounds:      1,
	}, compactor, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	client := &runnerTestLLMClient{
		appChatErrors: []error{errors.New("context_length_exceeded"), nil},
		appChatResponses: []*adapter.ChatResponse{
			nil,
			{
				Choices: []adapter.Choice{{Message: adapter.Message{Role: "assistant", Content: "recovered answer"}, FinishReason: "stop"}},
				Usage:   &adapter.Usage{PromptTokens: 300, CompletionTokens: 50, TotalTokens: 350},
			},
		},
	}
	request := &adapter.ChatRequest{Model: "gpt-5", Messages: []adapter.Message{
		{Role: "user", Content: strings.Repeat("old task context ", 1000)},
		{Role: "assistant", ToolCalls: []adapter.ToolCall{{ID: "call-1", Function: adapter.FunctionCall{Name: "search"}}}},
		{Role: "tool", ToolCallID: "call-1", Content: "latest search result"},
	}}
	prepared := NewPreparedChat("conversation", "message", "", "auto", request)
	runner := &Runner{LLMClient: client, AppContext: &llmclient.AppContext{}, ContextManager: manager}

	result, err := runner.runModelToolRound(context.Background(), prepared, request, 0, nil, false, false, false, "skill_planning")
	if err != nil {
		t.Fatal(err)
	}
	if got := assistantMessageText(result.message); got != "recovered answer" {
		t.Fatalf("answer = %q", got)
	}
	if client.appChatCalls != 2 || compactor.calls != 1 {
		t.Fatalf("main calls=%d compact calls=%d, want 2 and 1", client.appChatCalls, compactor.calls)
	}
	if state := manager.State(); state.Summary == nil || !strings.Contains(state.Summary.Content, "earlier task state") {
		t.Fatalf("reactive compact summary was not committed: %#v", state.Summary)
	}
	if state := manager.State(); state.NextRound != 2 {
		t.Fatalf("prompt-too-long retry advanced API rounds more than once: next_round=%d", state.NextRound)
	}
	if state := manager.State(); state.LastUsage == nil || state.LastUsage.PromptTokens != 300 {
		t.Fatalf("context state usage must contain only main-model usage: %#v", state.LastUsage)
	}
	if result.usage == nil || result.usage.PromptTokens != 400 || result.usage.TotalTokens != 470 {
		t.Fatalf("returned usage must include compaction billing: %#v", result.usage)
	}
}

func TestTerminalToolBatchCompletesEveryToolCall(t *testing.T) {
	manager, err := contextmgr.New(contextmgr.Config{
		AgentRunID:             "terminal-tool-batch",
		ConfiguredAgentWindowK: 64,
		ModelContextWindow:     128_000,
		MaxOutputTokens:        8_000,
	}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	messages := []adapter.Message{{Role: "user", Content: "finish the task"}}
	request := &adapter.ChatRequest{Model: "gpt-5", Messages: messages}
	if _, _, err := manager.PrepareBeforeModelCall(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	calls := []adapter.ToolCall{
		{ID: "final", Function: adapter.FunctionCall{Name: "submit_final_answer"}},
		{ID: "sibling", Function: adapter.FunctionCall{Name: "search"}},
	}
	assistant := adapter.Message{Role: "assistant", Content: "done", ToolCalls: calls, ReasoningContent: "private"}
	if err := manager.ObserveModelResponse(context.Background(), assistant, nil); err != nil {
		t.Fatal(err)
	}
	runner := &Runner{ContextManager: manager}
	if err := runner.checkpointTerminalToolBatch(context.Background(), messages, assistant, calls, "final", adapter.Message{Role: "tool", ToolCallID: "final", Content: `{"status":"accepted"}`}, "terminal answer accepted"); err != nil {
		t.Fatal(err)
	}
	state := manager.State()
	if len(state.Messages) != 4 || state.Messages[2].ToolCallID != "final" || state.Messages[3].ToolCallID != "sibling" {
		t.Fatalf("terminal batch state = %#v", state.Messages)
	}
	if state.Messages[1].ReasoningContent != "" {
		t.Fatalf("terminal assistant reasoning was retained: %q", state.Messages[1].ReasoningContent)
	}
	if _, _, err := manager.PrepareBeforeModelCall(context.Background(), &adapter.ChatRequest{Model: "gpt-5", Messages: state.Messages}); err != nil {
		t.Fatalf("terminal batch left invalid tool pairing: %v", err)
	}
}

func TestRunModelToolRoundReturnsContextExhaustedAfterReactiveRetryFails(t *testing.T) {
	manager, err := contextmgr.New(contextmgr.Config{
		AgentRunID:             "reactive-failure-run",
		ConfiguredAgentWindowK: 64,
		ModelContextWindow:     128_000,
		MaxInputTokens:         128_000,
		MaxOutputTokens:        8_000,
		TailMinTextRounds:      1,
	}, &reactiveTestCompactor{}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	client := &runnerTestLLMClient{appChatErrors: []error{
		errors.New("context_length_exceeded"),
		errors.New("context_length_exceeded"),
	}}
	request := &adapter.ChatRequest{Model: "gpt-5", Messages: []adapter.Message{
		{Role: "user", Content: strings.Repeat("old task context ", 1000)},
		{Role: "assistant", ToolCalls: []adapter.ToolCall{{ID: "call-1", Function: adapter.FunctionCall{Name: "search"}}}},
		{Role: "tool", ToolCallID: "call-1", Content: "latest search result"},
	}}
	prepared := NewPreparedChat("conversation", "message", "", "auto", request)
	runner := &Runner{LLMClient: client, AppContext: &llmclient.AppContext{}, ContextManager: manager}

	_, err = runner.runModelToolRound(context.Background(), prepared, request, 0, nil, false, false, false, "skill_planning")
	if !errors.Is(err, contextmgr.ErrContextExhausted) {
		t.Fatalf("error = %v, want context exhausted", err)
	}
	if client.appChatCalls != 2 || manager.State().NextRound != 1 {
		t.Fatalf("main calls=%d next_round=%d", client.appChatCalls, manager.State().NextRound)
	}
}

type reactiveTestCompactor struct {
	calls int
}

func (c *reactiveTestCompactor) Compact(_ context.Context, _ *adapter.ChatRequest, _ contextmgr.CompactCall) (string, *adapter.Usage, error) {
	c.calls++
	return "<summary>earlier task state</summary>", &adapter.Usage{PromptTokens: 100, CompletionTokens: 20, TotalTokens: 120}, nil
}
