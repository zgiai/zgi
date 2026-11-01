package service

import "time"

// ZGI-13: the model's reasoning is shown to the user as a collapsible
// "thinking" block. Its text and how long the model spent thinking are kept in
// the assistant message metadata so history renders the same block. The
// reasoning is display-only and never replayed into the model context.
const (
	reasoningContentMetadataKey  = "reasoning_content"
	reasoningDurationMetadataKey = "reasoning_duration_ms"
)

// reasoningClock sums the spans in which the model was streaming reasoning.
// A tool loop has one span per model round; a span closes as soon as anything
// other than reasoning (answer text, a tool event) is produced.
type reasoningClock struct {
	spanStart time.Time
	spanLast  time.Time
	closed    time.Duration
}

func (c *reasoningClock) observe(now time.Time) time.Duration {
	if c.spanStart.IsZero() {
		c.spanStart = now
	}
	if now.After(c.spanLast) {
		c.spanLast = now
	}
	return c.total()
}

func (c *reasoningClock) closeSpan() {
	if c.spanStart.IsZero() {
		return
	}
	c.closed = c.total()
	c.spanStart = time.Time{}
	c.spanLast = time.Time{}
}

func (c *reasoningClock) total() time.Duration {
	if c.spanStart.IsZero() {
		return c.closed
	}
	return c.closed + c.spanLast.Sub(c.spanStart)
}

// appendPreparedReasoningContent appends a reasoning delta to the message
// metadata and refreshes the thinking duration.
func appendPreparedReasoningContent(prepared *PreparedChat, reasoning string) {
	appendPreparedReasoningContentAt(prepared, reasoning, time.Now())
}

func appendPreparedReasoningContentAt(prepared *PreparedChat, reasoning string, now time.Time) {
	if prepared == nil || prepared.Message == nil || reasoning == "" {
		return
	}
	metadata := copyStringAnyMap(prepared.Message.Metadata)
	if metadata == nil {
		metadata = map[string]interface{}{}
	}
	metadata[reasoningContentMetadataKey] = stringFromAny(metadata[reasoningContentMetadataKey]) + reasoning
	metadata[reasoningDurationMetadataKey] = prepared.reasoning.observe(now).Milliseconds()
	prepared.Message.Metadata = metadata
}

// closePreparedReasoningSpan ends the current thinking span; the next
// reasoning delta starts a new one.
func closePreparedReasoningSpan(prepared *PreparedChat) {
	if prepared == nil {
		return
	}
	prepared.reasoning.closeSpan()
}

// isReasoningOnlyMessageEvent reports a message event that carries reasoning
// and no answer text.
func isReasoningOnlyMessageEvent(eventType string, payload map[string]interface{}) bool {
	if eventType != streamEventMessage || payload == nil {
		return false
	}
	if stringFromAny(payload[reasoningContentMetadataKey]) == "" {
		return false
	}
	return stringFromAny(payload["answer"]) == ""
}
