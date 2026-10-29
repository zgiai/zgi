package adapter

import "strings"

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
