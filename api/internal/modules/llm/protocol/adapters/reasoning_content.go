package adapter

import (
	"os"
	"strings"
)

// ReasoningHistoryKept reports whether the transcript keeps the reasoning of
// earlier turns, so a later turn replays the requests the provider cached
// (ADR 0045). It is on unless ZGI_REASONING_HISTORY=turn keeps reasoning
// within its turn only (ADR 0033).
func ReasoningHistoryKept() bool {
	return !strings.EqualFold(strings.TrimSpace(os.Getenv("ZGI_REASONING_HISTORY")), "turn")
}

// KeepsReasoningHistory reports whether the transcript of a turn answered by
// model keeps its reasoning. Only models that replay reasoning store it; for
// any other model it would be data no later request uses.
func KeepsReasoningHistory(model string) bool {
	return ReasoningHistoryKept() && ModelReplaysReasoningContent(model)
}

// ModelReplaysReasoningContent reports whether a chat model expects the
// reasoning_content of its own tool-call turns sent back. DeepSeek thinking
// models require it for tool calls within one turn, and Kimi thinking models
// use it the same way. Other OpenAI-compatible APIs may reject the unknown
// message field, so it is left out for them.
func ModelReplaysReasoningContent(model string) bool {
	model = strings.ToLower(strings.TrimSpace(model))
	return strings.Contains(model, "deepseek") || strings.Contains(model, "kimi")
}

// MessagesForModel returns messages without reasoning_content unless the model
// replays it. The input is not modified.
func MessagesForModel(model string, messages []Message) []Message {
	if ModelReplaysReasoningContent(model) {
		return messages
	}
	return WithoutReasoningContent(messages)
}

// WithoutReasoningContent returns messages with reasoning_content cleared. It
// returns the input slice when no message carries reasoning.
func WithoutReasoningContent(messages []Message) []Message {
	carries := false
	for _, message := range messages {
		if message.ReasoningContent != "" {
			carries = true
			break
		}
	}
	if !carries {
		return messages
	}
	cleared := make([]Message, len(messages))
	copy(cleared, messages)
	for index := range cleared {
		cleared[index].ReasoningContent = ""
	}
	return cleared
}
