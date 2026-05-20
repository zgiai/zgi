package llm_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/model"
)

// =============================================================================
// Use Cases Type Tests
// =============================================================================

func TestUseCaseEnum(t *testing.T) {
	t.Run("use case enum values are correct", func(t *testing.T) {
		assert.Equal(t, model.UseCase("text-chat"), model.UseCaseTextChat)
		assert.Equal(t, model.UseCase("vision"), model.UseCaseVision)
		assert.Equal(t, model.UseCase("image-gen"), model.UseCaseImageGen)
		assert.Equal(t, model.UseCase("embedding"), model.UseCaseEmbedding)
		assert.Equal(t, model.UseCase("rerank"), model.UseCaseRerank)
		assert.Equal(t, model.UseCase("speech-to-text"), model.UseCaseSpeechToText)
		assert.Equal(t, model.UseCase("text-to-speech"), model.UseCaseTextToSpeech)
		assert.Equal(t, model.UseCase("realtime-audio"), model.UseCaseRealtimeAudio)
		assert.Equal(t, model.UseCase("video-gen"), model.UseCaseVideoGen)
		assert.Equal(t, model.UseCase("moderation"), model.UseCaseModeration)
		assert.Equal(t, model.UseCase("reasoning"), model.UseCaseReasoning)
		assert.Equal(t, model.UseCase("function-calling"), model.UseCaseFuncCalling)
	})

	t.Run("ValidUseCases returns all 12 values", func(t *testing.T) {
		useCases := model.ValidUseCases()
		assert.Len(t, useCases, 12)

		// Verify all expected values are present
		expected := []model.UseCase{
			model.UseCaseTextChat, model.UseCaseVision, model.UseCaseImageGen,
			model.UseCaseEmbedding, model.UseCaseRerank, model.UseCaseSpeechToText,
			model.UseCaseTextToSpeech, model.UseCaseRealtimeAudio, model.UseCaseVideoGen,
			model.UseCaseModeration, model.UseCaseReasoning, model.UseCaseFuncCalling,
		}
		assert.Equal(t, expected, useCases)
	})
}

// =============================================================================
// Use Cases Validation Tests
// =============================================================================

func TestUseCaseValidation(t *testing.T) {
	t.Run("valid use cases pass validation", func(t *testing.T) {
		validUseCases := []string{
			"text-chat", "vision", "image-gen", "embedding", "rerank",
			"speech-to-text", "text-to-speech", "realtime-audio",
			"video-gen", "moderation", "reasoning", "function-calling",
		}

		for _, uc := range validUseCases {
			isValid := isValidUseCase(uc)
			assert.True(t, isValid, "use case should be valid: %s", uc)
		}
	})

	t.Run("invalid use cases fail validation", func(t *testing.T) {
		invalidUseCases := []string{
			"", "unknown", "chat", "invalid-case", "TEXT-CHAT", // case-sensitive
		}

		for _, uc := range invalidUseCases {
			isValid := isValidUseCase(uc)
			assert.False(t, isValid, "use case should be invalid: %s", uc)
		}
	})

	t.Run("validate use cases array", func(t *testing.T) {
		// Valid array
		valid := []string{"text-chat", "vision", "function-calling"}
		assert.True(t, validateUseCasesArray(valid))

		// Empty array is valid (no use cases specified)
		empty := []string{}
		assert.True(t, validateUseCasesArray(empty))

		// Array with invalid value
		invalid := []string{"text-chat", "invalid"}
		assert.False(t, validateUseCasesArray(invalid))
	})
}

// isValidUseCase checks if a string is a valid use case
func isValidUseCase(uc string) bool {
	validUseCases := model.ValidUseCases()
	for _, valid := range validUseCases {
		if string(valid) == uc {
			return true
		}
	}
	return false
}

// validateUseCasesArray checks if all values in the array are valid
func validateUseCasesArray(useCases []string) bool {
	for _, uc := range useCases {
		if !isValidUseCase(uc) {
			return false
		}
	}
	return true
}

// =============================================================================
// Model Use Cases Assignment Tests
// =============================================================================

func TestModelUseCasesAssignment(t *testing.T) {
	t.Run("model can have multiple use cases", func(t *testing.T) {
		m := &model.LLMModel{
			Model:     "gpt-4o",
			ModelName: "GPT-4o",
			UseCases:  model.StringArray{"text-chat", "vision", "function-calling"},
		}

		assert.Len(t, m.UseCases, 3)
		assert.Contains(t, []string(m.UseCases), "text-chat")
		assert.Contains(t, []string(m.UseCases), "vision")
		assert.Contains(t, []string(m.UseCases), "function-calling")
	})

	t.Run("model can have empty use cases", func(t *testing.T) {
		m := &model.LLMModel{
			Model:     "legacy-model",
			ModelName: "Legacy Model",
			UseCases:  model.StringArray{},
		}

		assert.Len(t, m.UseCases, 0)
	})

	t.Run("custom model can have use cases", func(t *testing.T) {
		cm := &model.CustomModel{
			Name:        "custom-gpt",
			DisplayName: "Custom GPT",
			UseCases:    []string{"text-chat", "reasoning"},
		}

		assert.Len(t, cm.UseCases, 2)
		assert.Contains(t, cm.UseCases, "text-chat")
		assert.Contains(t, cm.UseCases, "reasoning")
	})
}

// =============================================================================
// Model View Use Cases Tests
// =============================================================================

func TestModelViewUseCases(t *testing.T) {
	t.Run("model view includes use cases", func(t *testing.T) {
		view := &model.ModelView{
			Model:     "claude-3-opus",
			ModelName: "Claude 3 Opus",
			UseCases:  []string{"text-chat", "vision", "function-calling", "reasoning"},
		}

		assert.Len(t, view.UseCases, 4)
		assert.Contains(t, view.UseCases, "reasoning")
	})

	t.Run("model view use cases defaults to empty", func(t *testing.T) {
		view := &model.ModelView{
			Model:     "test-model",
			ModelName: "Test Model",
		}

		// UseCases should be nil (not initialized)
		assert.Nil(t, view.UseCases)
	})
}

// =============================================================================
// Use Cases Filtering Tests
// =============================================================================

func TestUseCasesFiltering(t *testing.T) {
	t.Run("filter models by use case", func(t *testing.T) {
		models := []*model.ModelView{
			{Model: "gpt-4o", UseCases: []string{"text-chat", "vision"}},
			{Model: "claude-3", UseCases: []string{"text-chat", "function-calling"}},
			{Model: "dall-e-3", UseCases: []string{"image-gen"}},
			{Model: "text-embedding-3", UseCases: []string{"embedding"}},
		}

		// Filter for models with "vision" use case
		filtered := filterByUseCase(models, "vision")
		assert.Len(t, filtered, 1)
		assert.Equal(t, "gpt-4o", filtered[0].Model)

		// Filter for models with "text-chat" use case
		filtered = filterByUseCase(models, "text-chat")
		assert.Len(t, filtered, 2)
	})

	t.Run("filter by multiple use cases", func(t *testing.T) {
		models := []*model.ModelView{
			{Model: "gpt-4o", UseCases: []string{"text-chat", "vision", "function-calling"}},
			{Model: "claude-3", UseCases: []string{"text-chat", "function-calling"}},
			{Model: "gpt-3.5", UseCases: []string{"text-chat"}},
		}

		// Filter for models with both "vision" AND "function-calling"
		filtered := filterByMultipleUseCases(models, []string{"vision", "function-calling"})
		assert.Len(t, filtered, 1)
		assert.Equal(t, "gpt-4o", filtered[0].Model)
	})
}

// filterByUseCase filters models that have a specific use case
func filterByUseCase(models []*model.ModelView, useCase string) []*model.ModelView {
	var result []*model.ModelView
	for _, m := range models {
		for _, uc := range m.UseCases {
			if uc == useCase {
				result = append(result, m)
				break
			}
		}
	}
	return result
}

// filterByMultipleUseCases filters models that have ALL specified use cases
func filterByMultipleUseCases(models []*model.ModelView, useCases []string) []*model.ModelView {
	var result []*model.ModelView
	for _, m := range models {
		if hasAllUseCases(m.UseCases, useCases) {
			result = append(result, m)
		}
	}
	return result
}

func hasAllUseCases(modelUseCases []string, required []string) bool {
	useCaseSet := make(map[string]bool)
	for _, uc := range modelUseCases {
		useCaseSet[uc] = true
	}
	for _, req := range required {
		if !useCaseSet[req] {
			return false
		}
	}
	return true
}
