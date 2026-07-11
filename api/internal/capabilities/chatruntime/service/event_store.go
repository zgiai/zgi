package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

const (
	streamEventTTL          = 24 * time.Hour
	streamEventReadCount    = 100
	streamEventReadBlock    = 15 * time.Second
	streamMessageFlushAfter = 300 * time.Millisecond
	streamMessageFlushSize  = 512
)

type StreamEvent struct {
	ID        string
	EventType string
	Payload   map[string]interface{}
	CreatedAt int64
}

type streamEventStore struct {
	client *redis.Client
}

func newStreamEventStore(client *redis.Client) *streamEventStore {
	return &streamEventStore{client: client}
}

func (s *streamEventStore) available() bool {
	return s != nil && s.client != nil
}

func (s *streamEventStore) append(ctx context.Context, messageID uuid.UUID, conversationID uuid.UUID, eventType string, payload map[string]interface{}) (*StreamEvent, error) {
	if !s.available() {
		return nil, ErrStreamEventsUnavailable
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal aichat stream event payload: %w", err)
	}
	createdAt := time.Now().Unix()
	key := streamEventsKey(messageID)
	id, err := s.client.XAdd(ctx, &redis.XAddArgs{
		Stream: key,
		Values: map[string]interface{}{
			"conversation_id": conversationID.String(),
			"message_id":      messageID.String(),
			"event_type":      eventType,
			"payload":         string(payloadBytes),
			"created_at":      strconv.FormatInt(createdAt, 10),
		},
	}).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to append aichat stream event: %w", err)
	}
	event := &StreamEvent{ID: id, EventType: eventType, Payload: payload, CreatedAt: createdAt}
	if err := s.client.Expire(ctx, key, streamEventTTL).Err(); err != nil {
		return event, fmt.Errorf("failed to refresh aichat stream event ttl: %w", err)
	}
	return event, nil
}

func (s *streamEventStore) exists(ctx context.Context, messageID uuid.UUID) (bool, error) {
	if !s.available() {
		return false, ErrStreamEventsUnavailable
	}
	count, err := s.client.Exists(ctx, streamEventsKey(messageID)).Result()
	if err != nil {
		return false, fmt.Errorf("failed to check aichat stream events: %w", err)
	}
	return count > 0, nil
}

func (s *streamEventStore) reset(ctx context.Context, messageID uuid.UUID) error {
	if !s.available() {
		return ErrStreamEventsUnavailable
	}
	if err := s.client.Del(ctx, streamEventsKey(messageID)).Err(); err != nil {
		return fmt.Errorf("failed to reset aichat stream events: %w", err)
	}
	return nil
}

func (s *streamEventStore) read(ctx context.Context, messageID uuid.UUID, afterID string, block time.Duration) ([]StreamEvent, error) {
	if !s.available() {
		return nil, ErrStreamEventsUnavailable
	}
	afterID = normalizeStreamAfterID(afterID)
	result, err := s.client.XRead(ctx, &redis.XReadArgs{
		Streams: []string{streamEventsKey(messageID), afterID},
		Count:   streamEventReadCount,
		Block:   block,
	}).Result()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to read aichat stream events: %w", err)
	}
	var events []StreamEvent
	for _, stream := range result {
		for _, message := range stream.Messages {
			event, err := decodeStreamEvent(message)
			if err != nil {
				return nil, err
			}
			events = append(events, event)
		}
	}
	return events, nil
}

func decodeStreamEvent(message redis.XMessage) (StreamEvent, error) {
	eventType := stringField(message.Values, "event_type")
	payloadRaw := stringField(message.Values, "payload")
	var payload map[string]interface{}
	if strings.TrimSpace(payloadRaw) != "" {
		if err := json.Unmarshal([]byte(payloadRaw), &payload); err != nil {
			return StreamEvent{}, fmt.Errorf("failed to decode aichat stream event payload: %w", err)
		}
	}
	if payload == nil {
		payload = map[string]interface{}{}
	}
	createdAt, _ := strconv.ParseInt(stringField(message.Values, "created_at"), 10, 64)
	return StreamEvent{
		ID:        message.ID,
		EventType: eventType,
		Payload:   payload,
		CreatedAt: createdAt,
	}, nil
}

func streamEventsKey(messageID uuid.UUID) string {
	return "aichat:message:" + messageID.String() + ":events"
}

func normalizeStreamAfterID(afterID string) string {
	afterID = strings.TrimSpace(afterID)
	if afterID == "" {
		return "0"
	}
	return afterID
}

func stringField(values map[string]interface{}, key string) string {
	value, ok := values[key]
	if !ok || value == nil {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return typed
	case []byte:
		return string(typed)
	default:
		return fmt.Sprint(typed)
	}
}

type streamMessageEventBuffer struct {
	store          *streamEventStore
	conversationID uuid.UUID
	messageID      uuid.UUID
	builder        strings.Builder
	lastFlush      time.Time
}

func newStreamMessageEventBuffer(store *streamEventStore, conversationID, messageID uuid.UUID) *streamMessageEventBuffer {
	return &streamMessageEventBuffer{
		store:          store,
		conversationID: conversationID,
		messageID:      messageID,
		lastFlush:      time.Now(),
	}
}

func (b *streamMessageEventBuffer) add(ctx context.Context, chunk string) (*StreamEvent, error) {
	if chunk == "" {
		return nil, nil
	}
	b.builder.WriteString(chunk)
	if b.builder.Len() < streamMessageFlushSize && time.Since(b.lastFlush) < streamMessageFlushAfter {
		return nil, nil
	}
	return b.flush(ctx)
}

func fallbackStreamMessageEvent(conversationID, messageID uuid.UUID, chunk string) *StreamEvent {
	return &StreamEvent{
		EventType: streamEventMessage,
		Payload: map[string]interface{}{
			"conversation_id": conversationID.String(),
			"message_id":      messageID.String(),
			"answer":          chunk,
		},
		CreatedAt: time.Now().Unix(),
	}
}

func (b *streamMessageEventBuffer) flush(ctx context.Context) (*StreamEvent, error) {
	if b == nil || b.builder.Len() == 0 {
		return nil, nil
	}
	chunk := b.builder.String()
	b.builder.Reset()
	b.lastFlush = time.Now()
	if !b.store.available() {
		return fallbackStreamMessageEvent(b.conversationID, b.messageID, chunk), nil
	}
	payload := map[string]interface{}{
		"conversation_id": b.conversationID.String(),
		"message_id":      b.messageID.String(),
		"answer":          chunk,
	}
	event, err := b.store.append(ctx, b.messageID, b.conversationID, streamEventMessage, payload)
	if err != nil && event == nil {
		return fallbackStreamMessageEvent(b.conversationID, b.messageID, chunk), err
	}
	return event, err
}
