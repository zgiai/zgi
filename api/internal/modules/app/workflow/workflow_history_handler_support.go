package workflow

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/ginext/internal/modules/app/conversation"
)

type conversationHistoryAccessService interface {
	GetConversationByIDAndAgent(ctx context.Context, id, agentID uuid.UUID) (*conversation.AgentConversation, error)
	GetConversationHistoryByAgent(ctx context.Context, filter conversation.AgentConversationHistoryFilter) ([]*conversation.AgentConversation, int64, error)
}

type conversationMessageQueryService interface {
	GetMessagesByConversation(ctx context.Context, conversationID uuid.UUID, limit, offset int) ([]*conversation.AgentMessage, int64, error)
}

func parsePositiveInt(raw string, defaultValue int) int {
	if raw == "" {
		return defaultValue
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return defaultValue
	}
	return value
}

func parseOptionalTime(raw string) (*time.Time, error) {
	if raw == "" {
		return nil, nil
	}

	formats := []string{
		time.RFC3339,
		"2006-01-02 15:04:05",
		"2006-01-02",
	}

	for _, format := range formats {
		parsed, err := time.Parse(format, raw)
		if err == nil {
			return &parsed, nil
		}
	}

	return nil, fmt.Errorf("invalid time format: %s", raw)
}

func parseMultiValueQuery(values []string, defaultValues ...string) []string {
	normalized := make([]string, 0, len(values))
	seen := make(map[string]struct{})

	appendValue := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		if _, exists := seen[value]; exists {
			return
		}
		seen[value] = struct{}{}
		normalized = append(normalized, value)
	}

	for _, value := range values {
		parts := strings.Split(value, ",")
		for _, part := range parts {
			appendValue(part)
		}
	}

	if len(normalized) == 0 {
		for _, value := range defaultValues {
			appendValue(value)
		}
	}

	return normalized
}
