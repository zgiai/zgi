package llm

import (
	"fmt"
	"strings"

	"github.com/zgiai/zgi/api/pkg/logger"
	"go.uber.org/zap"
)

// ModelConfigWithCredentialsEntity represents model configuration with credentials
type ModelConfigWithCredentialsEntity struct {
	Provider            string               `json:"provider"`
	Model               string               `json:"model"`
	ModelSchema         ModelSchema          `json:"model_schema"`
	Mode                Mode                 `json:"mode"`
	ProviderModelBundle *ProviderModelBundle `json:"provider_model_bundle"`
	Credentials         map[string]any       `json:"credentials"`
	Parameters          map[string]any       `json:"parameters"`
	Stop                []string             `json:"stop"`
}

// TokenBufferMemory represents memory for conversation history with enhanced functionality
type TokenBufferMemory struct {
	Conversation              any
	ModelInstance             *ModelInstance
	MaxTokens                 int
	Messages                  []PromptMessage
	AppID                     string
	UserID                    string
	HistoryExplicitlyProvided bool // When true, skip loading from database even if Messages is empty
}

// NewTokenBufferMemory creates a new TokenBufferMemory instance
func NewTokenBufferMemory(
	conversation any,
	modelInstance *ModelInstance,
	maxTokens int,
	appID string,
	userID string,
) *TokenBufferMemory {
	return &TokenBufferMemory{
		Conversation:  conversation,
		ModelInstance: modelInstance,
		MaxTokens:     maxTokens,
		Messages:      make([]PromptMessage, 0),
		AppID:         appID,
		UserID:        userID,
	}
}

// GetHistoryPromptMessages returns history messages for chat mode
func (m *TokenBufferMemory) GetHistoryPromptMessages(
	maxTokenLimit int,
	messageLimit int,
) []PromptMessage {
	// In real implementation, this would:
	// 1. Load conversation history from database
	// 2. Calculate token count for each message
	// 3. Filter messages based on token and message limits
	// 4. Return formatted prompt messages

	logger.Debug("LLM token buffer memory requested",
		zap.Int("max_token_limit", maxTokenLimit),
		zap.Int("message_limit", messageLimit),
		zap.Int("messages_count", len(m.Messages)),
	)

	// History must be provided by the LLM node execution policy. Do not load
	// from the database here; missing history means the prompt carries none.
	if len(m.Messages) == 0 && !m.HistoryExplicitlyProvided {
		logger.Debug("LLM token buffer memory has no explicit history; using empty history")
	} else if len(m.Messages) == 0 && m.HistoryExplicitlyProvided {
		logger.Debug("LLM token buffer memory skipping database load for explicit empty history")
	}

	// Apply message limit
	if messageLimit > 0 && len(m.Messages) > messageLimit {
		logger.Debug("LLM token buffer memory applying message limit",
			zap.Int("messages_count", len(m.Messages)),
			zap.Int("message_limit", messageLimit),
		)
		// Take the most recent messages
		startIndex := len(m.Messages) - messageLimit
		return m.Messages[startIndex:]
	}

	// Apply token limit using more accurate token estimation
	if maxTokenLimit > 0 {
		logger.Debug("LLM token buffer memory applying token limit",
			zap.Int("max_token_limit", maxTokenLimit),
		)
		totalTokens := 0
		filteredMessages := make([]PromptMessage, 0)

		// Start from the most recent messages and work backwards
		for i := len(m.Messages) - 1; i >= 0; i-- {
			message := m.Messages[i]
			messageTokens := m.estimateMessageTokens(message)

			if totalTokens+messageTokens <= maxTokenLimit {
				filteredMessages = append([]PromptMessage{message}, filteredMessages...)
				totalTokens += messageTokens
			} else {
				logger.Debug("LLM token buffer memory token limit reached",
					zap.Int("current_tokens", totalTokens),
					zap.Int("message_tokens", messageTokens),
					zap.Int("max_token_limit", maxTokenLimit),
				)
				break
			}
		}

		logger.Debug("LLM token buffer memory token filtering completed",
			zap.Int("messages_count", len(filteredMessages)),
			zap.Int("total_tokens", totalTokens),
		)
		return filteredMessages
	}

	logger.Debug("LLM token buffer memory returning all messages",
		zap.Int("messages_count", len(m.Messages)),
	)
	return m.Messages
}

// estimateMessageTokens estimates the number of tokens in a message using a more sophisticated approach
func (m *TokenBufferMemory) estimateMessageTokens(message PromptMessage) int {
	var content string

	// Extract text content from message
	switch c := message.Content.(type) {
	case string:
		content = c
	case []PromptMessageContent:
		for _, item := range c {
			if item.Type == PromptMessageContentTypeText {
				content += item.Data + " "
			}
		}
	default:
		content = fmt.Sprintf("%v", c)
	}

	// More accurate token estimation based on content characteristics:
	// - English text: ~1 token per 3-4 characters
	// - Code/numbers: ~1 token per 2-3 characters
	// - Asian text (Chinese/Japanese/Korean): ~1 token per 1-2 characters
	// - Add base tokens for message structure (role, formatting)

	baseTokens := 4 // Tokens for message structure (role, punctuation, etc.)

	// Detect content type for better estimation
	contentTokens := 0
	if content != "" {
		// Check for Asian characters (more tokens per character)
		asianCharCount := 0
		for _, r := range content {
			if (r >= 0x4E00 && r <= 0x9FFF) || // CJK Unified Ideographs
				(r >= 0x3400 && r <= 0x4DBF) || // CJK Extension A
				(r >= 0x20000 && r <= 0x2A6DF) || // CJK Extension B
				(r >= 0x3040 && r <= 0x309F) || // Hiragana
				(r >= 0x30A0 && r <= 0x30FF) { // Katakana
				asianCharCount++
			}
		}

		// Calculate tokens based on content characteristics
		if asianCharCount > 0 {
			// Asian text: ~1 token per 1-2 characters
			asianRatio := float64(asianCharCount) / float64(len(content))
			asianTokens := int(float64(asianCharCount) * (0.7 + asianRatio*0.3))
			nonAsianTokens := (len(content) - asianCharCount) / 4
			contentTokens = asianTokens + nonAsianTokens
		} else if strings.Contains(content, "{") && strings.Contains(content, "}") {
			// Likely code/JSON: ~1 token per 2-3 characters
			contentTokens = len(content) / 3
		} else {
			// Regular English text: ~1 token per 3-4 characters
			contentTokens = len(content) / 4
		}

		// Ensure minimum 1 token for any non-empty content
		if contentTokens == 0 {
			contentTokens = 1
		}
	}

	return baseTokens + contentTokens
}

// GetHistoryPromptText returns history text for completion mode
func (m *TokenBufferMemory) GetHistoryPromptText(
	maxTokenLimit int,
	messageLimit int,
	humanPrefix string,
	aiPrefix string,
) string {
	// Get history messages
	historyMessages := m.GetHistoryPromptMessages(maxTokenLimit, messageLimit)

	// Convert messages to text format with role prefixes
	var textBuilder strings.Builder
	for _, message := range historyMessages {
		var prefix string
		switch message.Role {
		case PromptMessageRoleUser:
			prefix = humanPrefix
		case PromptMessageRoleAssistant:
			prefix = aiPrefix
		default:
			continue // Skip system messages in completion mode
		}

		// Convert content to string
		var content string
		switch c := message.Content.(type) {
		case string:
			content = c
		case []PromptMessageContent:
			// Extract text content only
			for _, item := range c {
				if item.Type == PromptMessageContentTypeText {
					content += item.Data
				}
			}
		default:
			content = fmt.Sprintf("%v", c)
		}

		if content != "" {
			textBuilder.WriteString(fmt.Sprintf("%s: %s\n", prefix, content))
		}
	}

	return textBuilder.String()
}

// AddMessage adds a new message to memory
func (m *TokenBufferMemory) AddMessage(message PromptMessage) {
	m.Messages = append(m.Messages, message)

	// Trim messages if exceeding max tokens using accurate token calculation
	if m.MaxTokens > 0 {
		totalTokens := 0
		for _, msg := range m.Messages {
			totalTokens += m.estimateMessageTokens(msg)
		}

		// Remove oldest messages if we exceed the token limit
		for totalTokens > m.MaxTokens && len(m.Messages) > 1 {
			// Remove the oldest message
			removedMessage := m.Messages[0]
			m.Messages = m.Messages[1:]
			totalTokens -= m.estimateMessageTokens(removedMessage)
		}
	}
}

func parseAndRemoveStopParameter(completionParams map[string]any) ([]string, error) {
	if completionParams == nil {
		return []string{}, nil
	}

	stopValue, exists := completionParams["stop"]
	if !exists {
		return []string{}, nil
	}

	delete(completionParams, "stop")

	switch v := stopValue.(type) {
	case []string:
		return v, nil
	case []interface{}:
		stop := make([]string, 0, len(v))
		for _, item := range v {
			if str, ok := item.(string); ok {
				stop = append(stop, str)
			}
		}
		return stop, nil
	case string:
		return []string{v}, nil
	default:
		return []string{}, nil
	}
}
