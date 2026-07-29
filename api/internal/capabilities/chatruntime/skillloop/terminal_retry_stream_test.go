package skillloop

import (
	"strings"
	"testing"

	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/modules/skills"
)

func TestRunnerTerminalOnlyRetryDoesNotLeakRejectedStreamedAnswer(t *testing.T) {
	index := 0
	client := &runnerTestLLMClient{appChatStreams: [][]adapter.StreamResponse{
		{
			{Choices: []adapter.StreamChoice{{Delta: adapter.Message{ToolCalls: []adapter.ToolCall{{
				Index: &index,
				ID:    "stale-final-answer",
				Type:  "function",
				Function: adapter.FunctionCall{
					Name:      skills.MetaToolFinalAnswer,
					Arguments: `{"answer":"stale answer"}`,
				},
			}}}}}},
			{Choices: []adapter.StreamChoice{{FinishReason: "tool_calls"}}, Done: true},
		},
		{
			{Choices: []adapter.StreamChoice{{Delta: adapter.Message{Content: "correct answer"}}}},
			{Choices: []adapter.StreamChoice{{FinishReason: "stop"}}, Done: true},
		},
	}}
	var events []Event
	runner := &Runner{
		LLMClient: client,
		OnEvent: func(event Event) error {
			events = append(events, event)
			return nil
		},
	}
	prepared := NewPreparedChat(
		"conv-terminal-stream-retry",
		"msg-terminal-stream-retry",
		"qwen",
		"auto",
		&adapter.ChatRequest{Model: "qwen-plus"},
	)

	answer, _, err := runner.Run(t.Context(), RunRequest{
		Prepared:     prepared,
		Resolved:     &skills.ResolvedSkills{},
		TerminalOnly: true,
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if answer != "correct answer" {
		t.Fatalf("answer = %q, want correct answer", answer)
	}
	if got := terminalRetryStreamedAnswer(events); got != "correct answer" {
		t.Fatalf("streamed answer = %q, want only the accepted retry answer", got)
	}
	if count := terminalRetryEventCount(events, EventMessage); count != 1 {
		t.Fatalf("message events = %d, want the accepted answer emitted once", count)
	}
	if terminalRetryEventCount(events, EventMessageRetract) != 0 {
		t.Fatalf("events = %#v, want no retraction after a buffered terminal retry", events)
	}
	if client.appChatStreamCalls != 2 {
		t.Fatalf("AppChatStream calls = %d, want initial attempt plus one retry", client.appChatStreamCalls)
	}
}

func TestRunnerTerminalOnlyAcceptsStreamedNaturalAnswerAtOutputLimit(t *testing.T) {
	client := &runnerTestLLMClient{appChatStreams: [][]adapter.StreamResponse{{
		{Choices: []adapter.StreamChoice{{Delta: adapter.Message{Content: "partial answer"}}}},
		{Choices: []adapter.StreamChoice{{FinishReason: "length"}}, Done: true},
	}}}
	var events []Event
	runner := &Runner{
		LLMClient: client,
		OnEvent: func(event Event) error {
			events = append(events, event)
			return nil
		},
	}
	prepared := NewPreparedChat(
		"conv-terminal-stream-length",
		"msg-terminal-stream-length",
		"qwen",
		"auto",
		&adapter.ChatRequest{Model: "qwen-plus"},
	)

	answer, _, err := runner.Run(t.Context(), RunRequest{
		Prepared:     prepared,
		Resolved:     &skills.ResolvedSkills{},
		TerminalOnly: true,
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if answer != "partial answer" {
		t.Fatalf("answer = %q, want usable truncated answer", answer)
	}
	if got := terminalRetryStreamedAnswer(events); got != "partial answer" {
		t.Fatalf("streamed answer = %q, want one accepted truncated answer", got)
	}
	if count := terminalRetryEventCount(events, EventMessage); count != 1 {
		t.Fatalf("message events = %d, want the truncated answer emitted once", count)
	}
	if client.appChatStreamCalls != 1 {
		t.Fatalf("AppChatStream calls = %d, want no retry for usable truncated content", client.appChatStreamCalls)
	}
}

func TestRunnerTerminalOnlyAcceptsTruncatedFinalAnswerArgumentsAtOutputLimit(t *testing.T) {
	index := 0
	client := &runnerTestLLMClient{appChatStreams: [][]adapter.StreamResponse{{
		{Choices: []adapter.StreamChoice{{Delta: adapter.Message{ToolCalls: []adapter.ToolCall{{
			Index: &index,
			ID:    "truncated-final-answer",
			Type:  "function",
			Function: adapter.FunctionCall{
				Name:      skills.MetaToolFinalAnswer,
				Arguments: `{"answer":"partial final answer`,
			},
		}}}}}},
		{Choices: []adapter.StreamChoice{{FinishReason: "max_tokens"}}, Done: true},
	}}}
	var events []Event
	runner := &Runner{
		LLMClient: client,
		OnEvent: func(event Event) error {
			events = append(events, event)
			return nil
		},
	}
	prepared := NewPreparedChat(
		"conv-terminal-stream-json-length",
		"msg-terminal-stream-json-length",
		"qwen",
		"auto",
		&adapter.ChatRequest{Model: "qwen-plus"},
	)

	answer, _, err := runner.Run(t.Context(), RunRequest{
		Prepared:     prepared,
		Resolved:     &skills.ResolvedSkills{},
		TerminalOnly: true,
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if answer != "partial final answer" {
		t.Fatalf("answer = %q, want recovered truncated final answer", answer)
	}
	if got := terminalRetryStreamedAnswer(events); got != "partial final answer" {
		t.Fatalf("streamed answer = %q, want recovered truncated final answer", got)
	}
	if count := terminalRetryEventCount(events, EventMessage); count != 1 {
		t.Fatalf("message events = %d, want the recovered answer emitted once", count)
	}
	if client.appChatStreamCalls != 1 {
		t.Fatalf("AppChatStream calls = %d, want no retry for a recoverable final answer", client.appChatStreamCalls)
	}
}

func TestRunnerTerminalOnlyRetriesOutputLimitWithoutVisibleAnswer(t *testing.T) {
	client := &runnerTestLLMClient{appChatStreams: [][]adapter.StreamResponse{
		{{Choices: []adapter.StreamChoice{{FinishReason: "length"}}, Done: true}},
		{
			{Choices: []adapter.StreamChoice{{Delta: adapter.Message{Content: "answer after retry"}}}},
			{Choices: []adapter.StreamChoice{{FinishReason: "stop"}}, Done: true},
		},
	}}
	var events []Event
	runner := &Runner{
		LLMClient: client,
		OnEvent: func(event Event) error {
			events = append(events, event)
			return nil
		},
	}
	prepared := NewPreparedChat(
		"conv-terminal-stream-empty-length",
		"msg-terminal-stream-empty-length",
		"qwen",
		"auto",
		&adapter.ChatRequest{Model: "qwen-plus"},
	)

	answer, _, err := runner.Run(t.Context(), RunRequest{
		Prepared:     prepared,
		Resolved:     &skills.ResolvedSkills{},
		TerminalOnly: true,
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if answer != "answer after retry" {
		t.Fatalf("answer = %q, want answer after retry", answer)
	}
	if got := terminalRetryStreamedAnswer(events); got != "answer after retry" {
		t.Fatalf("streamed answer = %q, want only the retry answer", got)
	}
	if client.appChatStreamCalls != 2 {
		t.Fatalf("AppChatStream calls = %d, want one retry for empty truncated content", client.appChatStreamCalls)
	}
}

func terminalRetryStreamedAnswer(events []Event) string {
	var answer strings.Builder
	for _, event := range events {
		if event.Type != EventMessage {
			continue
		}
		if chunk, ok := event.Payload["answer"].(string); ok {
			answer.WriteString(chunk)
		}
	}
	return answer.String()
}

func terminalRetryEventCount(events []Event, eventType string) int {
	count := 0
	for _, event := range events {
		if event.Type == eventType {
			count++
		}
	}
	return count
}
