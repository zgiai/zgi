package service

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

func TestRuntimeCollectStreamAnswerPreservesReasoningContent(t *testing.T) {
	svc := &service{streams: newStreamRegistry()}
	prepared := &PreparedChat{
		Conversation: &runtimemodel.Conversation{ID: uuid.New()},
		Message:      &runtimemodel.Message{ID: uuid.New(), Metadata: map[string]interface{}{"existing": "keep"}},
	}
	stream := make(chan adapter.StreamResponse, 3)
	stream <- adapter.StreamResponse{Choices: []adapter.StreamChoice{{Delta: adapter.Message{Role: "assistant", ReasoningContent: "think"}}}}
	stream <- adapter.StreamResponse{Choices: []adapter.StreamChoice{{Delta: adapter.Message{Role: "assistant", Content: "answer"}}}}
	stream <- adapter.StreamResponse{Done: true}
	close(stream)

	var events []StreamEvent
	var chunks []string
	answer, _, err := svc.collectStreamAnswerWithEvents(context.Background(), prepared, stream, func(event StreamEvent) error {
		events = append(events, event)
		return nil
	}, func(text string) error {
		chunks = append(chunks, text)
		return nil
	})
	if err != nil {
		t.Fatalf("collectStreamAnswerWithEvents() error = %v", err)
	}
	if answer != "answer" {
		t.Fatalf("answer = %q, want answer", answer)
	}
	if got := prepared.Message.Metadata["reasoning_content"]; got != "think" {
		t.Fatalf("reasoning_content metadata = %#v, want think", got)
	}
	if got := strings.Join(chunks, ""); got != "answer" {
		t.Fatalf("streamed chunks = %q, want answer", got)
	}
	var reasoningEventSeen bool
	for _, event := range events {
		if event.EventType == streamEventMessage && event.Payload["reasoning_content"] == "think" {
			reasoningEventSeen = true
			break
		}
	}
	if !reasoningEventSeen {
		t.Fatalf("events = %#v, want reasoning_content message event", events)
	}
}
