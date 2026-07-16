package skillloop

import (
	"context"
	"errors"
	"strings"
	"testing"

	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/modules/skills"
)

func TestRunnerRetriesLengthTruncatedPlanningOnce(t *testing.T) {
	index := 0
	initialMaxTokens := 2048
	client := &runnerTestLLMClient{appChatStreams: [][]adapter.StreamResponse{
		{{Choices: []adapter.StreamChoice{{FinishReason: "length"}}, Done: true}},
		{
			{Choices: []adapter.StreamChoice{{Delta: adapter.Message{ToolCalls: []adapter.ToolCall{{
				Index: &index,
				ID:    "final-after-retry",
				Type:  "function",
				Function: adapter.FunctionCall{
					Name:      skills.MetaToolFinalAnswer,
					Arguments: `{"answer":"done"}`,
				},
			}}}}}},
			{Choices: []adapter.StreamChoice{{FinishReason: "tool_calls"}}, Done: true},
		},
	}}
	runner := &Runner{LLMClient: client, SkillRuntime: skills.NewRuntime(nil, nil)}
	prepared := NewPreparedChat("conv-length-retry", "msg-length-retry", "qwen", "auto", &adapter.ChatRequest{
		Model:     "qwen-plus",
		MaxTokens: &initialMaxTokens,
	})

	answer, _, err := runner.Run(context.Background(), RunRequest{
		Prepared:                  prepared,
		Resolved:                  runnerTestResolvedSkills(),
		PreferExplicitFinalAnswer: true,
		PlanningOutputTokenLimit:  12000,
		RuntimeStateSnapshot:      func() map[string]interface{} { return map[string]interface{}{} },
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if answer != "done" {
		t.Fatalf("answer = %q, want done", answer)
	}
	if client.appChatStreamCalls != 2 {
		t.Fatalf("AppChatStream calls = %d, want 2", client.appChatStreamCalls)
	}
	retryRequest := client.appChatStreamRequests[1]
	if retryRequest.MaxTokens == nil || *retryRequest.MaxTokens != 8192 {
		t.Fatalf("retry max tokens = %#v, want 8192", retryRequest.MaxTokens)
	}
	if len(retryRequest.Messages) == 0 || retryRequest.Messages[0].Role != "system" {
		t.Fatalf("retry messages = %#v, want a leading system message", retryRequest.Messages)
	}
	if !strings.Contains(messageContent(retryRequest.Messages[0].Content), "truncated") {
		t.Fatalf("retry system message = %#v, want truncation guidance", retryRequest.Messages[0])
	}
	for index, message := range retryRequest.Messages[1:] {
		if strings.EqualFold(strings.TrimSpace(message.Role), "system") {
			t.Fatalf("retry message %d = %#v, want all system guidance merged at the beginning", index+1, message)
		}
	}
}

func TestRunnerStopsAfterSecondLengthTruncation(t *testing.T) {
	client := &runnerTestLLMClient{appChatStreams: [][]adapter.StreamResponse{
		{{Choices: []adapter.StreamChoice{{FinishReason: "length"}}, Done: true}},
		{{Choices: []adapter.StreamChoice{{FinishReason: "max_tokens"}}, Done: true}},
	}}
	runner := &Runner{LLMClient: client, SkillRuntime: skills.NewRuntime(nil, nil)}
	prepared := NewPreparedChat("conv-length-failed", "msg-length-failed", "qwen", "auto", &adapter.ChatRequest{Model: "qwen-plus"})

	_, _, err := runner.Run(context.Background(), RunRequest{
		Prepared:                  prepared,
		Resolved:                  runnerTestResolvedSkills(),
		PreferExplicitFinalAnswer: true,
		RuntimeStateSnapshot:      func() map[string]interface{} { return map[string]interface{}{} },
	})
	if err == nil || !strings.Contains(err.Error(), "planning_output_truncated") {
		t.Fatalf("Run() error = %v, want planning_output_truncated", err)
	}
	if client.appChatStreamCalls != 2 {
		t.Fatalf("AppChatStream calls = %d, want 2", client.appChatStreamCalls)
	}
}

func TestRunnerDoesNotRetryContentFilterTermination(t *testing.T) {
	client := &runnerTestLLMClient{appChatStreams: [][]adapter.StreamResponse{
		{{Choices: []adapter.StreamChoice{{FinishReason: "content_filter"}}, Done: true}},
	}}
	runner := &Runner{LLMClient: client, SkillRuntime: skills.NewRuntime(nil, nil)}
	prepared := NewPreparedChat("conv-filtered", "msg-filtered", "qwen", "auto", &adapter.ChatRequest{Model: "qwen-plus"})

	_, _, err := runner.Run(context.Background(), RunRequest{
		Prepared:                  prepared,
		Resolved:                  runnerTestResolvedSkills(),
		PreferExplicitFinalAnswer: true,
		RuntimeStateSnapshot:      func() map[string]interface{} { return map[string]interface{}{} },
	})
	var terminationErr *PlanningTerminationError
	if !errors.As(err, &terminationErr) || terminationErr.Recoverable {
		t.Fatalf("Run() error = %#v, want non-recoverable content filter termination", err)
	}
	if client.appChatStreamCalls != 1 {
		t.Fatalf("AppChatStream calls = %d, want 1", client.appChatStreamCalls)
	}
}
