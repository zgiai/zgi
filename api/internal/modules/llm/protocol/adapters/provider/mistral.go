package provider

import (
	"context"
	"crypto/rand"
	"regexp"

	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

const defaultMistralBaseURL = "https://api.mistral.ai/v1"

var mistralToolCallIDPattern = regexp.MustCompile("^[a-zA-Z0-9]{9}$")

// MistralAdapter wraps Mistral's OpenAI-compatible API surface.
type MistralAdapter struct {
	*openAIAnthropicCompatAdapter
}

func NewMistralAdapter(config *adapter.AdapterConfig) (*MistralAdapter, error) {
	compat, err := newOpenAIAnthropicCompatAdapter(config, "mistral", defaultMistralBaseURL)
	if err != nil {
		return nil, err
	}
	return &MistralAdapter{openAIAnthropicCompatAdapter: compat}, nil
}

func (a *MistralAdapter) ChatCompletion(ctx context.Context, request *adapter.ChatRequest) (*adapter.ChatResponse, error) {
	return a.openAI.ChatCompletion(ctx, normalizeMistralChatRequest(request))
}

func (a *MistralAdapter) ChatCompletionStream(ctx context.Context, request *adapter.ChatRequest) (<-chan adapter.StreamResponse, error) {
	return a.openAI.ChatCompletionStream(ctx, normalizeMistralChatRequest(request))
}

func normalizeMistralChatRequest(request *adapter.ChatRequest) *adapter.ChatRequest {
	if request == nil {
		return nil
	}
	normalized := *request
	normalized.Messages = make([]adapter.Message, 0, len(request.Messages))
	toolCallIDs := map[string]string{}
	for _, message := range request.Messages {
		next := message
		if len(message.ToolCalls) > 0 {
			next.ToolCalls = make([]adapter.ToolCall, len(message.ToolCalls))
			copy(next.ToolCalls, message.ToolCalls)
			for i := range next.ToolCalls {
				next.ToolCalls[i].ID = mistralToolCallID(next.ToolCalls[i].ID, toolCallIDs)
			}
		}
		if next.ToolCallID != "" {
			next.ToolCallID = mistralToolCallID(next.ToolCallID, toolCallIDs)
		}
		if next.Role == "assistant" && len(next.ToolCalls) > 0 && isEmptyMessageContent(next.Content) {
			next.Content = nil
		}
		normalized.Messages = append(normalized.Messages, next)
	}
	return &normalized
}

func isEmptyMessageContent(content interface{}) bool {
	value, ok := content.(string)
	return ok && value == ""
}

func mistralToolCallID(raw string, mapping map[string]string) string {
	if mistralToolCallIDPattern.MatchString(raw) {
		return raw
	}
	if raw == "" {
		return randomMistralToolCallID()
	}
	if mapped, ok := mapping[raw]; ok {
		return mapped
	}
	next := randomMistralToolCallID()
	mapping[raw] = next
	return next
}

func randomMistralToolCallID() string {
	const alphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	var bytes [9]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return "abcdefghi"
	}
	for i := range bytes {
		bytes[i] = alphabet[int(bytes[i])%len(alphabet)]
	}
	return string(bytes[:])
}
