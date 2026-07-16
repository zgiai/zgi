package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

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

func TestRuntimeCollectStreamAnswerTimesOutWhenModelIsIdle(t *testing.T) {
	svc := &service{streams: newStreamRegistry(), modelIdleTimeout: 20 * time.Millisecond}
	prepared := runtimeStreamTestPreparedChat()
	stream := make(chan adapter.StreamResponse)

	startedAt := time.Now()
	_, _, err := svc.collectStreamAnswerWithEvents(context.Background(), prepared, stream, nil, nil)
	if !errors.Is(err, ErrModelIdleTimeout) {
		t.Fatalf("collectStreamAnswerWithEvents() error = %v, want model idle timeout", err)
	}
	if elapsed := time.Since(startedAt); elapsed > 500*time.Millisecond {
		t.Fatalf("model idle timeout took %s, want prompt cancellation", elapsed)
	}
}

func TestRuntimeCollectStreamAnswerReasoningResetsIdleTimer(t *testing.T) {
	svc := &service{streams: newStreamRegistry(), modelIdleTimeout: 50 * time.Millisecond}
	prepared := runtimeStreamTestPreparedChat()
	stream := make(chan adapter.StreamResponse)
	go func() {
		defer close(stream)
		for index := 0; index < 4; index++ {
			time.Sleep(20 * time.Millisecond)
			stream <- adapter.StreamResponse{Choices: []adapter.StreamChoice{{Delta: adapter.Message{
				Role:             "assistant",
				ReasoningContent: "thinking",
			}}}}
		}
		stream <- adapter.StreamResponse{Done: true}
	}()

	if _, _, err := svc.collectStreamAnswerWithEvents(context.Background(), prepared, stream, nil, nil); err != nil {
		t.Fatalf("collectStreamAnswerWithEvents() error = %v, want reasoning activity to keep stream alive", err)
	}
}

func TestAppendDirectAnswerGroundingMessageOnlyForDirectAnswers(t *testing.T) {
	direct := &PreparedChat{
		LLMRequest: &adapter.ChatRequest{Messages: []adapter.Message{{Role: "user", Content: "delete the file"}}},
		parts:      &chatRequestParts{},
	}
	appendDirectAnswerGroundingMessage(direct)
	appendDirectAnswerGroundingMessage(direct)

	if len(direct.LLMRequest.Messages) != 2 {
		t.Fatalf("direct messages = %#v, want one grounding message appended exactly once", direct.LLMRequest.Messages)
	}
	grounding := direct.LLMRequest.Messages[len(direct.LLMRequest.Messages)-1]
	if grounding.Role != "system" || grounding.Content != directAnswerGroundingMessage {
		t.Fatalf("last direct message = %#v, want direct-answer grounding", grounding)
	}
	for _, required := range []string{
		"user request, intent, authorization",
		"Do not invent tools",
		"action was not performed or cannot be verified",
	} {
		if !strings.Contains(stringFromAny(grounding.Content), required) {
			t.Fatalf("grounding message = %q, want %q", grounding.Content, required)
		}
	}

	toolLoop := &PreparedChat{
		LLMRequest: &adapter.ChatRequest{Messages: []adapter.Message{{Role: "user", Content: "calculate 1+1"}}},
		parts: &chatRequestParts{
			SkillMode: skillModeAuto,
			SkillIDs:  []string{"calculator"},
		},
	}
	appendDirectAnswerGroundingMessage(toolLoop)
	if len(toolLoop.LLMRequest.Messages) != 1 {
		t.Fatalf("tool-loop messages = %#v, direct-answer grounding must not be appended", toolLoop.LLMRequest.Messages)
	}
}

func TestOpenChatStreamMovesAndMergesSystemMessagesBeforeModelCall(t *testing.T) {
	llm := &toolGovernanceStreamLLM{}
	svc := &service{
		llmClient:        llm,
		modelIdleTimeout: time.Second,
	}
	prepared := &PreparedChat{
		Conversation: &runtimemodel.Conversation{ID: uuid.New()},
		Message: &runtimemodel.Message{
			ID:        uuid.New(),
			ModelName: "test-model",
			Metadata:  map[string]interface{}{},
		},
		LLMRequest: &adapter.ChatRequest{
			Model: "test-model",
			Messages: []adapter.Message{
				{Role: "system", Content: "base instructions"},
				{Role: "user", Content: "question"},
				{Role: "assistant", Content: "working"},
				{Role: "system", Content: "continuation guidance"},
			},
		},
		parts: &chatRequestParts{Provider: "test-provider"},
	}

	stream, err := svc.openChatStream(context.Background(), prepared)
	if err != nil {
		t.Fatalf("openChatStream() error = %v", err)
	}
	for range stream {
	}
	if len(llm.streamRequests) != 1 {
		t.Fatalf("stream requests = %d, want 1", len(llm.streamRequests))
	}

	messages := llm.streamRequests[0].Messages
	if len(messages) != 3 {
		t.Fatalf("messages = %#v, want one system plus two conversation messages", messages)
	}
	if messages[0].Role != "system" || messages[0].Content != "base instructions\n\ncontinuation guidance" {
		t.Fatalf("messages[0] = %#v, want merged leading system message", messages[0])
	}
	if messages[1].Role != "user" || messages[2].Role != "assistant" {
		t.Fatalf("conversation order = %#v, want user then assistant", messages[1:])
	}
}

func runtimeStreamTestPreparedChat() *PreparedChat {
	return &PreparedChat{
		Conversation: &runtimemodel.Conversation{ID: uuid.New()},
		Message: &runtimemodel.Message{
			ID:        uuid.New(),
			ModelName: "test-model",
			Metadata:  map[string]interface{}{},
		},
		parts: &chatRequestParts{Provider: "test-provider"},
	}
}
