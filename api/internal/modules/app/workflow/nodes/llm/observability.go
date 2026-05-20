package llm

func promptMessagesTextLength(messages []PromptMessage) int {
	total := 0
	for _, message := range messages {
		total += promptMessageContentTextLength(message.Content)
	}
	return total
}

func promptMessageContentTextLength(content any) int {
	switch value := content.(type) {
	case string:
		return len(value)
	case []PromptMessageContent:
		total := 0
		for _, item := range value {
			if item.Type == PromptMessageContentTypeText {
				total += len(item.Data)
			}
		}
		return total
	default:
		return 0
	}
}
