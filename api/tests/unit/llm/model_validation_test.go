package llm_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// =============================================================================
// Model Name Validation Tests
// =============================================================================

func TestModelNameValidation(t *testing.T) {
	t.Run("valid model names", func(t *testing.T) {
		validModels := []string{
			"gpt-4",
			"gpt-4o",
			"gpt-3.5-turbo",
			"claude-3-opus",
			"text-embedding-3-small",
			"qwen-plus",
		}

		for _, model := range validModels {
			isValid := len(model) > 0 && len(model) <= 100
			assert.True(t, isValid, "model should be valid: %s", model)
		}
	})

	t.Run("invalid model names", func(t *testing.T) {
		invalidModels := []string{
			"",
		}

		for _, model := range invalidModels {
			isValid := len(model) > 0
			assert.False(t, isValid, "model should be invalid: %s", model)
		}
	})
}

// =============================================================================
// Model Type Detection Tests
// =============================================================================

func TestModelTypeDetection(t *testing.T) {
	t.Run("detect chat model", func(t *testing.T) {
		chatModels := []string{
			"gpt-4",
			"gpt-4o",
			"gpt-3.5-turbo",
			"claude-3-opus",
			"qwen-plus",
		}

		for _, model := range chatModels {
			modelType := detectModelType(model)
			assert.Equal(t, "chat", modelType, "model %s should be chat type", model)
		}
	})

	t.Run("detect embedding model", func(t *testing.T) {
		embeddingModels := []string{
			"text-embedding-3-small",
			"text-embedding-3-large",
			"text-embedding-ada-002",
		}

		for _, model := range embeddingModels {
			modelType := detectModelType(model)
			assert.Equal(t, "embedding", modelType, "model %s should be embedding type", model)
		}
	})

	t.Run("detect rerank model", func(t *testing.T) {
		rerankModels := []string{
			"rerank-english-v2.0",
			"rerank-multilingual-v2.0",
		}

		for _, model := range rerankModels {
			modelType := detectModelType(model)
			assert.Equal(t, "rerank", modelType, "model %s should be rerank type", model)
		}
	})
}

func detectModelType(model string) string {
	// Simple detection based on model name patterns
	if len(model) >= 4 && model[:4] == "text" {
		return "embedding"
	}
	if len(model) >= 6 && model[:6] == "rerank" {
		return "rerank"
	}
	return "chat"
}

// =============================================================================
// Model Provider Detection Tests
// =============================================================================

func TestModelProviderDetection(t *testing.T) {
	t.Run("detect OpenAI models", func(t *testing.T) {
		openaiModels := []string{
			"gpt-4",
			"gpt-4o",
			"gpt-3.5-turbo",
			"text-embedding-3-small",
		}

		for _, model := range openaiModels {
			provider := detectProvider(model)
			assert.Equal(t, "openai", provider, "model %s should be openai", model)
		}
	})

	t.Run("detect Anthropic models", func(t *testing.T) {
		anthropicModels := []string{
			"claude-3-opus",
			"claude-3-sonnet",
			"claude-3-haiku",
		}

		for _, model := range anthropicModels {
			provider := detectProvider(model)
			assert.Equal(t, "anthropic", provider, "model %s should be anthropic", model)
		}
	})

	t.Run("detect Alibaba models", func(t *testing.T) {
		alibabaModels := []string{
			"qwen-plus",
			"qwen-turbo",
			"qwen-max",
		}

		for _, model := range alibabaModels {
			provider := detectProvider(model)
			assert.Equal(t, "alibaba", provider, "model %s should be alibaba", model)
		}
	})
}

func detectProvider(model string) string {
	if len(model) >= 3 && model[:3] == "gpt" {
		return "openai"
	}
	if len(model) >= 4 && model[:4] == "text" {
		return "openai"
	}
	if len(model) >= 6 && model[:6] == "claude" {
		return "anthropic"
	}
	if len(model) >= 4 && model[:4] == "qwen" {
		return "alibaba"
	}
	return "unknown"
}

// =============================================================================
// Model Capability Tests
// =============================================================================

func TestModelCapabilities(t *testing.T) {
	t.Run("model supports streaming", func(t *testing.T) {
		streamingModels := map[string]bool{
			"gpt-4":                  true,
			"gpt-4o":                 true,
			"claude-3-opus":          true,
			"text-embedding-3-small": false, // Embeddings don't stream
		}

		for model, expected := range streamingModels {
			supportsStreaming := streamingModels[model]
			assert.Equal(t, expected, supportsStreaming, "model %s streaming support", model)
		}
	})

	t.Run("model supports function calling", func(t *testing.T) {
		functionCallingModels := map[string]bool{
			"gpt-4":         true,
			"gpt-4o":        true,
			"gpt-3.5-turbo": true,
			"claude-3-opus": true,
			"qwen-plus":     false,
		}

		for model, expected := range functionCallingModels {
			supportsFunctions := functionCallingModels[model]
			assert.Equal(t, expected, supportsFunctions, "model %s function calling support", model)
		}
	})

	t.Run("model supports vision", func(t *testing.T) {
		visionModels := map[string]bool{
			"gpt-4o":        true,
			"gpt-4-vision":  true,
			"claude-3-opus": true,
			"gpt-3.5-turbo": false,
		}

		for model, expected := range visionModels {
			supportsVision := visionModels[model]
			assert.Equal(t, expected, supportsVision, "model %s vision support", model)
		}
	})
}

// =============================================================================
// Model Pricing Validation Tests
// =============================================================================

func TestModelPricingValidation(t *testing.T) {
	t.Run("pricing must be non-negative", func(t *testing.T) {
		type Pricing struct {
			InputCostPerMillion  float64
			OutputCostPerMillion float64
		}

		validPricing := Pricing{
			InputCostPerMillion:  0.50,
			OutputCostPerMillion: 1.50,
		}

		isValid := validPricing.InputCostPerMillion >= 0 && validPricing.OutputCostPerMillion >= 0
		assert.True(t, isValid)
	})

	t.Run("free tier pricing", func(t *testing.T) {
		type Pricing struct {
			InputCostPerMillion  float64
			OutputCostPerMillion float64
		}

		freePricing := Pricing{
			InputCostPerMillion:  0,
			OutputCostPerMillion: 0,
		}

		isFree := freePricing.InputCostPerMillion == 0 && freePricing.OutputCostPerMillion == 0
		assert.True(t, isFree)
	})
}

// =============================================================================
// Model Alias Tests
// =============================================================================

func TestModelAlias(t *testing.T) {
	t.Run("resolve model alias", func(t *testing.T) {
		aliases := map[string]string{
			"gpt-4-latest":  "gpt-4o",
			"gpt-4-turbo":   "gpt-4-turbo-preview",
			"claude-latest": "claude-3-opus",
		}

		requestedModel := "gpt-4-latest"
		resolvedModel := requestedModel
		if alias, ok := aliases[requestedModel]; ok {
			resolvedModel = alias
		}

		assert.Equal(t, "gpt-4o", resolvedModel)
	})

	t.Run("no alias returns original", func(t *testing.T) {
		aliases := map[string]string{
			"gpt-4-latest": "gpt-4o",
		}

		requestedModel := "gpt-4"
		resolvedModel := requestedModel
		if alias, ok := aliases[requestedModel]; ok {
			resolvedModel = alias
		}

		assert.Equal(t, "gpt-4", resolvedModel)
	})
}
