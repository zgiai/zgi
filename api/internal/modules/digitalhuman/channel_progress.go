package digitalhuman

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/channelruntime"
	chatruntime "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/service"
	"github.com/zgiai/zgi/api/pkg/logger"
)

const (
	channelEventAgentProgress         = "agent_progress"
	channelEventIntermediateAnswer    = "agent_intermediate_answer"
	channelEventMessageRetract        = "message_retract"
	channelProgressDispositionDiscard = "discard"
	maxChannelProgressRunes           = 4_000
)

type channelProgressCollector struct {
	dispatcher ChannelTransientDispatcher
	runID      uuid.UUID
	buffers    map[string]*strings.Builder
	published  map[string]struct{}
	count      int
}

func newChannelProgressCollector(dispatcher ChannelTransientDispatcher, runID uuid.UUID) *channelProgressCollector {
	if dispatcher == nil || runID == uuid.Nil {
		return nil
	}
	return &channelProgressCollector{
		dispatcher: dispatcher, runID: runID,
		buffers: make(map[string]*strings.Builder), published: make(map[string]struct{}),
	}
}

func (collector *channelProgressCollector) Handle(ctx context.Context, event chatruntime.StreamEvent) error {
	if collector == nil {
		return nil
	}
	switch event.EventType {
	case channelEventMessageRetract:
		if strings.EqualFold(channelProgressString(event.Payload, "presentation_disposition"), channelProgressDispositionDiscard) {
			return nil
		}
		return collector.publish(ctx, channelProgressKey(event, "presentation"), channelProgressSequence(event), channelProgressString(event.Payload, "content"))
	case channelEventIntermediateAnswer:
		return collector.handleIntermediateAnswer(ctx, event)
	case channelEventAgentProgress:
		return collector.publish(ctx, channelProgressKey(event, "progress"), channelProgressSequence(event), channelProgressString(event.Payload, "content"))
	default:
		return nil
	}
}

func (collector *channelProgressCollector) handleIntermediateAnswer(ctx context.Context, event chatruntime.StreamEvent) error {
	answerID := channelProgressString(event.Payload, "answer_id")
	key := channelProgressKey(event, "intermediate")
	buffer := collector.buffers[key]
	if buffer == nil {
		buffer = &strings.Builder{}
		collector.buffers[key] = buffer
	}
	content := channelProgressRawString(event.Payload, "content")
	if channelProgressBool(event.Payload, "delta") {
		buffer.WriteString(content)
	} else if content != "" {
		buffer.Reset()
		buffer.WriteString(content)
	}
	if !channelProgressBool(event.Payload, "done") {
		return nil
	}
	delete(collector.buffers, key)
	text := strings.TrimSpace(buffer.String())
	if title := channelProgressString(event.Payload, "title"); title != "" && text != "" {
		text = title + "\n" + text
	}
	if answerID == "" && text == "" {
		return nil
	}
	return collector.publish(ctx, key, channelProgressSequence(event), text)
}

func (collector *channelProgressCollector) publish(ctx context.Context, key string, sequence int64, text string) error {
	text = boundedRunes(strings.TrimSpace(text), maxChannelProgressRunes)
	if text == "" || !utf8.ValidString(text) {
		return nil
	}
	if _, exists := collector.published[key]; exists {
		return nil
	}
	if sequence <= 0 {
		sequence = int64(collector.count + 1)
	}
	if err := collector.dispatcher.EnqueueTransient(ctx, ChannelTransientDeliveryRequest{
		RunID: collector.runID, Kind: channelruntime.DeliveryKindProgress,
		DeliveryKey: key, Sequence: sequence, Text: text,
	}); err != nil {
		logger.WarnContext(ctx, "DigitalHuman channel progress was dropped", "run_id", collector.runID.String(), err)
		return nil
	}
	collector.published[key] = struct{}{}
	collector.count++
	return nil
}

func channelProgressKey(event chatruntime.StreamEvent, prefix string) string {
	for _, field := range []string{"answer_id", "segment_id", "runtime_id", "event_id"} {
		if value := channelProgressString(event.Payload, field); value != "" {
			return boundedRunes(prefix+":"+value, 128)
		}
	}
	sum := sha256.Sum256([]byte(event.EventType + "\x00" + channelProgressString(event.Payload, "content")))
	return prefix + ":sha256:" + hex.EncodeToString(sum[:12])
}

func channelProgressSequence(event chatruntime.StreamEvent) int64 {
	if event.CreatedAtMS <= 0 {
		return event.Sequence
	}
	return event.CreatedAtMS*1_000 + max(event.Sequence, 0)
}

func channelProgressString(payload map[string]any, key string) string {
	return strings.TrimSpace(channelProgressRawString(payload, key))
}

func channelProgressRawString(payload map[string]any, key string) string {
	value, _ := payload[key].(string)
	return value
}

func channelProgressBool(payload map[string]any, key string) bool {
	value, _ := payload[key].(bool)
	return value
}
