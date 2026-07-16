package adapter

import "strings"

// NormalizeSystemMessages returns a copy with every system message before all
// conversational messages. Text-only system messages are merged in their
// original order so providers that accept only one leading system message can
// consume the request. Structured system content is kept losslessly as
// separate leading messages.
func NormalizeSystemMessages(messages []Message) []Message {
	if len(messages) == 0 {
		return nil
	}

	systemMessages := make([]Message, 0, len(messages))
	conversationMessages := make([]Message, 0, len(messages))
	textParts := make([]string, 0, len(messages))
	allSystemMessagesAreText := true

	for _, message := range messages {
		if !strings.EqualFold(strings.TrimSpace(message.Role), "system") {
			conversationMessages = append(conversationMessages, message)
			continue
		}

		message.Role = "system"
		systemMessages = append(systemMessages, message)
		text, ok := systemMessageText(message.Content)
		if !ok {
			allSystemMessagesAreText = false
			continue
		}
		if text = strings.TrimSpace(text); text != "" {
			textParts = append(textParts, text)
		}
	}

	normalized := make([]Message, 0, len(systemMessages)+len(conversationMessages))
	if allSystemMessagesAreText {
		if len(textParts) > 0 {
			normalized = append(normalized, Message{
				Role:    "system",
				Content: strings.Join(textParts, "\n\n"),
			})
		}
	} else {
		normalized = append(normalized, systemMessages...)
	}
	normalized = append(normalized, conversationMessages...)
	return normalized
}

func systemMessageText(content interface{}) (string, bool) {
	switch typed := content.(type) {
	case nil:
		return "", true
	case string:
		return typed, true
	case []MessageContentPart:
		parts := make([]string, 0, len(typed))
		for _, part := range typed {
			partType := strings.ToLower(strings.TrimSpace(part.Type))
			if part.ImageURL != nil || (partType != "" && partType != "text") {
				return "", false
			}
			if text := strings.TrimSpace(part.Text); text != "" {
				parts = append(parts, text)
			}
		}
		return strings.Join(parts, "\n"), true
	default:
		return "", false
	}
}
