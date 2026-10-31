package adapter

import "strings"

// NormalizeSystemMessages returns a copy with every system message before all
// conversational messages. Text-only system messages are merged in their
// original order so providers that accept only one leading system message can
// consume the request. Structured system content is kept losslessly as
// separate leading messages. The context manager uses it to separate fixed
// instructions from the compactable dialogue; a request sent to a provider
// goes through PlaceSystemMessages instead, which keeps later guidance in
// place (ADR 0032).
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

// PlaceSystemMessages returns the copy of a final request sent to a provider
// (ADR 0032). Some providers accept system content only at the start, so:
//
//   - the first message, when it is a system message, is the system prompt;
//   - every later text system message, such as turn context, a turn boundary
//     or skill instructions, stays where it is as a user turn wrapped in
//     <system-reminder>, merged with the user message next to it.
//
// Moving later guidance to the front instead would change the system prompt
// on every turn and invalidate every provider's prompt cache from the first
// token. Structured system content that is not text cannot become a reminder
// and is kept as a leading system message. Two original user messages are
// never merged, and a merged message keeps the original's EntryID.
func PlaceSystemMessages(messages []Message) []Message {
	if len(messages) == 0 {
		return nil
	}

	leading := make([]Message, 0, 1)
	conversation := make([]placedMessage, 0, len(messages))

	for index, message := range messages {
		if !isSystemMessage(message) {
			conversation = appendPlacedMessage(conversation, placedMessage{Message: message})
			continue
		}
		message.Role = "system"
		text, isText := systemMessageText(message.Content)
		if index == 0 || !isText {
			leading = append(leading, message)
			continue
		}
		if text = strings.TrimSpace(text); text == "" {
			continue
		}
		conversation = appendPlacedMessage(conversation, placedMessage{
			Message: Message{Role: "user", Content: SystemReminder(text)}, reminder: true,
		})
	}

	placed := make([]Message, 0, len(leading)+len(conversation))
	placed = append(placed, leading...)
	for _, message := range conversation {
		placed = append(placed, message.Message)
	}
	return placed
}

// placedMessage remembers whether a message holds only reminders, which may
// absorb or be absorbed by a neighbouring user message.
type placedMessage struct {
	Message
	reminder bool
}

func appendPlacedMessage(messages []placedMessage, message placedMessage) []placedMessage {
	if len(messages) == 0 {
		return append(messages, message)
	}
	previous := &messages[len(messages)-1]
	if (!message.reminder && !previous.reminder) || !plainUserMessage(message.Message) || !plainUserMessage(previous.Message) {
		return append(messages, message)
	}
	merged, ok := mergeUserContent(previous.Content, message.Content)
	if !ok {
		return append(messages, message)
	}
	previous.Content = merged
	if previous.reminder && !message.reminder {
		// The merged message is the original's, with its transcript identity.
		previous.EntryID = message.EntryID
		previous.reminder = false
	}
	return messages
}

// PrependSystemReminders puts reminders before a user message's content, in
// the form PlaceSystemMessages gives system messages placed before it. A turn
// that sends its context this way stores it with the user message, so the next
// turn replays the request it sent and extends its cached prefix (ADR 0043).
// Content that is neither text nor text and image parts is returned unchanged.
func PrependSystemReminders(reminders []string, content interface{}) interface{} {
	var merged interface{} = ""
	for _, reminder := range reminders {
		if reminder = strings.TrimSpace(reminder); reminder == "" {
			continue
		}
		next, ok := mergeUserContent(merged, SystemReminder(reminder))
		if !ok {
			return content
		}
		merged = next
	}
	if text, _ := merged.(string); text == "" {
		return content
	}
	result, ok := mergeUserContent(merged, content)
	if !ok {
		return content
	}
	return result
}

// SystemReminder wraps platform guidance that travels in a user turn, so the
// model can tell it from what the person wrote.
func SystemReminder(text string) string {
	return "<system-reminder>\n" + strings.TrimSpace(text) + "\n</system-reminder>"
}

func isSystemMessage(message Message) bool {
	return strings.EqualFold(strings.TrimSpace(message.Role), "system")
}

func plainUserMessage(message Message) bool {
	return strings.EqualFold(strings.TrimSpace(message.Role), "user") && message.ToolCallID == "" &&
		len(message.ToolCalls) == 0 && message.FunctionCall == nil && strings.TrimSpace(message.Name) == ""
}

func mergeUserContent(first, second interface{}) (interface{}, bool) {
	firstText, firstIsText := first.(string)
	secondText, secondIsText := second.(string)
	if firstIsText && secondIsText {
		switch {
		case strings.TrimSpace(firstText) == "":
			return secondText, true
		case strings.TrimSpace(secondText) == "":
			return firstText, true
		}
		return firstText + "\n\n" + secondText, true
	}
	firstParts, ok := userContentParts(first)
	if !ok {
		return nil, false
	}
	secondParts, ok := userContentParts(second)
	if !ok {
		return nil, false
	}
	return append(append([]MessageContentPart{}, firstParts...), secondParts...), true
}

func userContentParts(content interface{}) ([]MessageContentPart, bool) {
	switch typed := content.(type) {
	case nil:
		return nil, true
	case string:
		if strings.TrimSpace(typed) == "" {
			return nil, true
		}
		return []MessageContentPart{{Type: "text", Text: typed}}, true
	case []MessageContentPart:
		return typed, true
	default:
		return nil, false
	}
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
