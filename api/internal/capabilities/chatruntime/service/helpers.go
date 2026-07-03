package service

import (
	"errors"
	"strings"
	"unicode/utf8"

	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"gorm.io/gorm"
)

func usageMetadata(usage *adapter.Usage) map[string]interface{} {
	if usage == nil {
		return map[string]interface{}{}
	}
	return map[string]interface{}{
		"prompt_tokens":     usage.PromptTokens,
		"completion_tokens": usage.CompletionTokens,
		"total_tokens":      usage.TotalTokens,
	}
}

func mapRepoError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrNotFound
	}
	return err
}

func clampLimit(value, defaultValue, maxValue int) int {
	if value <= 0 {
		return defaultValue
	}
	if value > maxValue {
		return maxValue
	}
	return value
}

func pageOffset(page, limit int) int {
	if page < 1 {
		page = 1
	}
	return (page - 1) * limit
}

func normalizeTitle(title, fallback string) string {
	title = strings.TrimSpace(title)
	if title == "" {
		title = fallback
	}
	if utf8.RuneCountInString(title) <= maxConversationTitleLen {
		return title
	}
	runes := []rune(title)
	return string(runes[:maxConversationTitleLen])
}

func initialConversationTitle() string {
	return defaultConversationTitle
}

func isActiveMessageStatus(status string) bool {
	return status == runtimemodel.MessageStatusPending || status == runtimemodel.MessageStatusStreaming
}

func isStoppableMessageStatus(status string) bool {
	return isActiveMessageStatus(status) ||
		status == runtimemodel.MessageStatusWaitingApproval ||
		status == runtimemodel.MessageStatusWaitingQuestion
}

func floatValue(value interface{}) (float64, bool) {
	switch typed := value.(type) {
	case float64:
		return typed, true
	case float32:
		return float64(typed), true
	case int:
		return float64(typed), true
	case int64:
		return float64(typed), true
	case int32:
		return float64(typed), true
	default:
		return 0, false
	}
}

func intValue(value interface{}) (int, bool) {
	switch typed := value.(type) {
	case int:
		return typed, true
	case int64:
		return int(typed), true
	case int32:
		return int(typed), true
	case float64:
		if typed == float64(int(typed)) {
			return int(typed), true
		}
	case float32:
		if typed == float32(int(typed)) {
			return int(typed), true
		}
	}
	return 0, false
}

func stringSliceValue(value interface{}) ([]string, bool) {
	switch typed := value.(type) {
	case []string:
		return typed, true
	case []interface{}:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			text, ok := item.(string)
			if !ok {
				return nil, false
			}
			out = append(out, text)
		}
		return out, true
	default:
		return nil, false
	}
}

func copyStringAnyMap(input map[string]interface{}) map[string]interface{} {
	if input == nil {
		return nil
	}
	output := make(map[string]interface{}, len(input))
	for key, value := range input {
		output[key] = value
	}
	return output
}
