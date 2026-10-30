package skillloop

import (
	"strings"
	"testing"

	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/modules/skills"
)

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
