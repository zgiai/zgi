package llm_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// =============================================================================
// Token Estimation Tests
// =============================================================================

func TestTokenEstimationFromText(t *testing.T) {
	t.Run("estimate tokens from text length", func(t *testing.T) {
		// Approximate: 1 token ≈ 4 characters for English
		text := "Hello, how are you today?"
		charsPerToken := 4

		estimatedTokens := (len(text) + charsPerToken - 1) / charsPerToken

		assert.Greater(t, estimatedTokens, 0)
		assert.LessOrEqual(t, estimatedTokens, 10)
	})

	t.Run("estimate tokens for empty text", func(t *testing.T) {
		text := ""
		charsPerToken := 4

		estimatedTokens := len(text) / charsPerToken

		assert.Equal(t, 0, estimatedTokens)
	})

	t.Run("estimate tokens for long text", func(t *testing.T) {
		// 1000 characters ≈ 250 tokens
		text := make([]byte, 1000)
		for i := range text {
			text[i] = 'a'
		}
		charsPerToken := 4

		estimatedTokens := len(text) / charsPerToken

		assert.Equal(t, 250, estimatedTokens)
	})
}

// =============================================================================
// Chat Message Token Estimation Tests
// =============================================================================

func TestChatMessageTokenEstimation(t *testing.T) {
	t.Run("estimate tokens for single message", func(t *testing.T) {
		type Message struct {
			Role    string
			Content string
		}

		msg := Message{
			Role:    "user",
			Content: "What is the capital of France?",
		}

		// Role overhead + content
		roleOverhead := 4 // Approximate overhead for role
		contentTokens := (len(msg.Content) + 3) / 4

		totalTokens := roleOverhead + contentTokens

		assert.Greater(t, totalTokens, 0)
	})

	t.Run("estimate tokens for conversation", func(t *testing.T) {
		type Message struct {
			Role    string
			Content string
		}

		messages := []Message{
			{Role: "system", Content: "You are a helpful assistant."},
			{Role: "user", Content: "Hello!"},
			{Role: "assistant", Content: "Hi there! How can I help you?"},
			{Role: "user", Content: "What is 2+2?"},
		}

		totalTokens := 0
		for _, msg := range messages {
			roleOverhead := 4
			contentTokens := (len(msg.Content) + 3) / 4
			totalTokens += roleOverhead + contentTokens
		}

		// Conversation overhead
		conversationOverhead := 3
		totalTokens += conversationOverhead

		assert.Greater(t, totalTokens, 20)
	})
}

// =============================================================================
// Completion Token Estimation Tests
// =============================================================================

func TestCompletionTokenEstimation(t *testing.T) {
	t.Run("estimate from max_tokens parameter", func(t *testing.T) {
		maxTokens := 1000

		// Use max_tokens as upper bound for completion
		estimatedCompletion := maxTokens

		assert.Equal(t, 1000, estimatedCompletion)
	})

	t.Run("default max_tokens when not specified", func(t *testing.T) {
		var maxTokens *int = nil
		defaultMaxTokens := 4096

		var estimatedCompletion int
		if maxTokens == nil {
			estimatedCompletion = defaultMaxTokens
		} else {
			estimatedCompletion = *maxTokens
		}

		assert.Equal(t, 4096, estimatedCompletion)
	})

	t.Run("cap max_tokens to model limit", func(t *testing.T) {
		maxTokens := 100000
		modelMaxTokens := 8192

		estimatedCompletion := maxTokens
		if estimatedCompletion > modelMaxTokens {
			estimatedCompletion = modelMaxTokens
		}

		assert.Equal(t, 8192, estimatedCompletion)
	})
}

// =============================================================================
// Embedding Token Estimation Tests
// =============================================================================

func TestEmbeddingTokenEstimation(t *testing.T) {
	t.Run("estimate tokens for single input", func(t *testing.T) {
		input := "This is a test document for embedding."
		charsPerToken := 4

		estimatedTokens := (len(input) + charsPerToken - 1) / charsPerToken

		assert.Greater(t, estimatedTokens, 0)
	})

	t.Run("estimate tokens for batch inputs", func(t *testing.T) {
		inputs := []string{
			"First document",
			"Second document with more content",
			"Third document",
		}

		totalTokens := 0
		charsPerToken := 4
		for _, input := range inputs {
			totalTokens += (len(input) + charsPerToken - 1) / charsPerToken
		}

		assert.Greater(t, totalTokens, 10)
	})

	t.Run("embedding model token limit", func(t *testing.T) {
		input := make([]byte, 50000) // Very long input
		for i := range input {
			input[i] = 'a'
		}

		modelMaxTokens := 8191 // text-embedding-3-small limit
		estimatedTokens := len(input) / 4

		exceedsLimit := estimatedTokens > modelMaxTokens

		assert.True(t, exceedsLimit)
	})
}

// =============================================================================
// Model-Specific Token Estimation Tests
// =============================================================================

func TestModelSpecificTokenEstimation(t *testing.T) {
	t.Run("GPT-4 context window", func(t *testing.T) {
		modelContextWindow := map[string]int{
			"gpt-4":         8192,
			"gpt-4-32k":     32768,
			"gpt-4o":        128000,
			"gpt-3.5-turbo": 16385,
		}

		assert.Equal(t, 8192, modelContextWindow["gpt-4"])
		assert.Equal(t, 128000, modelContextWindow["gpt-4o"])
	})

	t.Run("check if request fits context window", func(t *testing.T) {
		promptTokens := 5000
		maxCompletionTokens := 2000
		modelContextWindow := 8192

		totalRequired := promptTokens + maxCompletionTokens
		fitsInContext := totalRequired <= modelContextWindow

		assert.True(t, fitsInContext)
	})

	t.Run("request exceeds context window", func(t *testing.T) {
		promptTokens := 7000
		maxCompletionTokens := 2000
		modelContextWindow := 8192

		totalRequired := promptTokens + maxCompletionTokens
		fitsInContext := totalRequired <= modelContextWindow

		assert.False(t, fitsInContext)
	})
}

// =============================================================================
// Token Count Accuracy Tests
// =============================================================================

func TestTokenCountAccuracy(t *testing.T) {
	t.Run("estimation vs actual comparison", func(t *testing.T) {
		// In real scenarios, we compare estimated vs actual from API response
		estimatedPrompt := 100
		estimatedCompletion := 50
		actualPrompt := 95
		actualCompletion := 48

		promptDiff := abs(estimatedPrompt - actualPrompt)
		completionDiff := abs(estimatedCompletion - actualCompletion)

		// Allow 10% variance
		promptVariance := float64(promptDiff) / float64(actualPrompt)
		completionVariance := float64(completionDiff) / float64(actualCompletion)

		assert.Less(t, promptVariance, 0.1)
		assert.Less(t, completionVariance, 0.1)
	})
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
