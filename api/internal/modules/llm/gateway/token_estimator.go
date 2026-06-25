package gateway

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

// TokenEstimator estimates token usage for requests
type TokenEstimator struct {
	cache sync.Map // Cache for tokenizers
}

// NewTokenEstimator creates a new token estimator
func NewTokenEstimator() *TokenEstimator {
	return &TokenEstimator{}
}

// EstimatePromptTokens estimates the number of tokens in the prompt
func (te *TokenEstimator) EstimatePromptTokens(messages []adapter.Message, model string) int {
	totalTokens := 0

	// Add tokens for each message
	for _, msg := range messages {
		// Message format overhead (based on OpenAI documentation)
		// <im_start>{role}\n{content}<im_end>\n
		totalTokens += 4

		// Content tokens - simple estimation: 1 token ≈ 4 characters
		// This is a fallback estimation. For production, use tiktoken library
		// Handle both string and multimodal content types
		switch content := msg.Content.(type) {
		case string:
			totalTokens += te.estimateTextTokens(content)
		case []interface{}:
			// For multimodal content, estimate tokens from text parts
			for _, part := range content {
				if partMap, ok := part.(map[string]interface{}); ok {
					if text, ok := partMap["text"].(string); ok {
						totalTokens += te.estimateTextTokens(text)
					}
				}
			}
		default:
			// Fallback: try to convert to string
			if content != nil {
				totalTokens += te.estimateTextTokens(fmt.Sprintf("%v", content))
			}
		}

		// Add tokens for role
		totalTokens += 1
	}

	// Add tokens for conversation end marker
	totalTokens += 2

	return totalTokens
}

// EstimateCompletionTokens estimates completion tokens based on max_tokens or default
func (te *TokenEstimator) EstimateCompletionTokens(maxTokens *int, model string) int {
	if maxTokens != nil && *maxTokens > 0 {
		return *maxTokens
	}

	// Default completion tokens based on model
	if strings.Contains(model, "gpt-4") {
		return 1000 // Default for GPT-4
	} else if strings.Contains(model, "gpt-3.5") {
		return 500 // Default for GPT-3.5
	}

	// Generic default
	return 500
}

// estimateTextTokens provides a simple token estimation
// For production use, integrate tiktoken library
func (te *TokenEstimator) estimateTextTokens(text string) int {
	// Simple estimation: 1 token ≈ 4 characters
	// This is a rough approximation
	charCount := len(text)
	tokenCount := charCount / 4

	// Ensure at least 1 token for non-empty text
	if charCount > 0 && tokenCount == 0 {
		tokenCount = 1
	}

	return tokenCount
}

// EstimateTotalTokens estimates total tokens for a request
func (te *TokenEstimator) EstimateTotalTokens(
	messages []adapter.Message,
	maxTokens *int,
	model string,
) (promptTokens, completionTokens, totalTokens int) {

	// TODO: The token calculation method should be based on tiktoken standard
	promptTokens = te.EstimatePromptTokens(messages, model)
	completionTokens = te.EstimateCompletionTokens(maxTokens, model)
	totalTokens = promptTokens + completionTokens

	return
}

// EstimateEmbeddingTokens estimates tokens for embedding request
func (te *TokenEstimator) EstimateEmbeddingTokens(input interface{}, model string) int {
	switch v := input.(type) {
	case string:
		return te.estimateTextTokens(v)
	case []string:
		totalTokens := 0
		for _, s := range v {
			totalTokens += te.estimateTextTokens(s)
		}
		return totalTokens
	case []int:
		return len(v)
	case [][]int:
		totalTokens := 0
		for _, tokens := range v {
			totalTokens += len(tokens)
		}
		return totalTokens
	case []interface{}:
		totalTokens := 0
		for _, item := range v {
			totalTokens += te.estimateEmbeddingItemTokens(item)
		}
		return totalTokens
	default:
		return 0
	}
}

func (te *TokenEstimator) estimateEmbeddingItemTokens(input interface{}) int {
	switch v := input.(type) {
	case string:
		return te.estimateTextTokens(v)
	case []int:
		return len(v)
	case []interface{}:
		totalTokens := 0
		for _, item := range v {
			totalTokens += te.estimateEmbeddingItemTokens(item)
		}
		return totalTokens
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64, json.Number:
		return 1
	default:
		return 0
	}
}

// EstimateRerankTokens estimates tokens for rerank request
func (te *TokenEstimator) EstimateRerankTokens(query string, documents interface{}, model string) int {
	// Start with query tokens
	totalTokens := te.estimateTextTokens(query)

	// Add document tokens
	switch v := documents.(type) {
	case []string:
		for _, doc := range v {
			totalTokens += te.estimateTextTokens(doc)
		}
	case []interface{}:
		for _, item := range v {
			if s, ok := item.(string); ok {
				totalTokens += te.estimateTextTokens(s)
			} else if m, ok := item.(map[string]interface{}); ok {
				// If it's a structured document, extract text field
				if text, ok := m["text"].(string); ok {
					totalTokens += te.estimateTextTokens(text)
				}
			}
		}
	}

	return totalTokens
}
