package service

import (
	"context"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

func TestCollectStreamAnswerWithEventsEmitsRecoverableMessageID(t *testing.T) {
	redisServer := miniredis.RunT(t)
	redisClient := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	defer redisClient.Close()

	conversationID := uuid.New()
	messageID := uuid.New()
	svc := &service{
		events:  newStreamEventStore(redisClient),
		streams: newStreamRegistry(),
	}
	prepared := &PreparedChat{
		Conversation: &runtimemodel.Conversation{ID: conversationID},
		Message:      &runtimemodel.Message{ID: messageID},
	}
	stream := make(chan adapter.StreamResponse, 2)
	stream <- adapter.StreamResponse{Choices: []adapter.StreamChoice{{Delta: adapter.Message{Content: "hello"}}}}
	stream <- adapter.StreamResponse{Done: true}
	close(stream)

	var events []StreamEvent
	chunks := 0
	answer, _, err := svc.collectStreamAnswerWithEvents(
		context.Background(),
		prepared,
		stream,
		func(event StreamEvent) error {
			events = append(events, event)
			return nil
		},
		func(string) error {
			chunks++
			return nil
		},
	)
	if err != nil {
		t.Fatalf("collectStreamAnswerWithEvents() error = %v", err)
	}
	if answer != "hello" {
		t.Fatalf("answer = %q, want hello", answer)
	}
	if chunks != 1 {
		t.Fatalf("chunk callback calls = %d, want 1 for direct collect helper", chunks)
	}
	if len(events) != 1 {
		t.Fatalf("events = %#v, want one message event", events)
	}
	if events[0].ID == "" {
		t.Fatalf("event id is empty, want recoverable stream id")
	}
	if events[0].EventType != streamEventMessage {
		t.Fatalf("event type = %q, want %q", events[0].EventType, streamEventMessage)
	}
	if events[0].Payload["answer"] != "hello" {
		t.Fatalf("event payload = %#v, want answer hello", events[0].Payload)
	}
}

func TestRunPreparedStreamModelCallbackPrefersRecoverableEvents(t *testing.T) {
	if got := modelStreamChunkCallback(func(StreamEvent) error { return nil }, func(string) error { return nil }); got != nil {
		t.Fatal("modelStreamChunkCallback() returned legacy chunk callback when event callback is available")
	}
	chunkCallback := func(string) error { return nil }
	if got := modelStreamChunkCallback(nil, chunkCallback); got == nil {
		t.Fatal("modelStreamChunkCallback() = nil without event callback, want legacy chunk callback")
	}
}
