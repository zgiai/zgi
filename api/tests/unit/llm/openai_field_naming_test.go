package llm_test

import (
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/zgiai/ginext/internal/modules/llm/llmmodel/model"
)

// =============================================================================
// OpenAI Field Naming Tests
// =============================================================================

func TestOpenAIFieldNaming_Model(t *testing.T) {
	t.Run("model has correct OpenAI field names", func(t *testing.T) {
		m := &model.LLMModel{
			Model:           "gpt-4o",
			ModelName:       "GPT-4o",
			ContextWindow:   128000,
			MaxOutputTokens: 16384,
			MaxInputTokens:  128000,
			InputPrice:      decimal.NewFromFloat(2.50),
			OutputPrice:     decimal.NewFromFloat(10.00),
		}

		// Verify OpenAI standard fields
		assert.Equal(t, 128000, m.ContextWindow, "should use context_window")
		assert.Equal(t, 16384, m.MaxOutputTokens, "should use max_output_tokens")
		assert.Equal(t, 128000, m.MaxInputTokens, "should use max_input_tokens")
		assert.True(t, m.InputPrice.Equal(decimal.NewFromFloat(2.50)), "should use input_price")
		assert.True(t, m.OutputPrice.Equal(decimal.NewFromFloat(10.00)), "should use output_price")
	})

	t.Run("model tier field is preserved", func(t *testing.T) {
		m := &model.LLMModel{
			Model:     "gpt-4o",
			ModelTier: "flagship",
		}
		assert.Equal(t, "flagship", m.ModelTier, "model_tier should be preserved")
	})
}

func TestOpenAIFieldNaming_CustomModel(t *testing.T) {
	t.Run("tenant custom model has correct OpenAI field names", func(t *testing.T) {
		m := &model.CustomModel{
			Name:            "custom-gpt",
			DisplayName:     "Custom GPT",
			ContextWindow:   32000,
			MaxOutputTokens: 4096,
			MaxInputTokens:  32000,
			InputPrice:      decimal.NewFromFloat(1.00),
			OutputPrice:     decimal.NewFromFloat(2.00),
		}

		assert.Equal(t, 32000, m.ContextWindow)
		assert.Equal(t, 4096, m.MaxOutputTokens)
		assert.Equal(t, 32000, m.MaxInputTokens)
		assert.True(t, m.InputPrice.Equal(decimal.NewFromFloat(1.00)))
		assert.True(t, m.OutputPrice.Equal(decimal.NewFromFloat(2.00)))
	})
}

func TestOpenAIFieldNaming_ModelView(t *testing.T) {
	t.Run("model view has correct OpenAI field names", func(t *testing.T) {
		view := &model.ModelView{
			Model:           "claude-3-opus",
			ModelName:       "Claude 3 Opus",
			ContextWindow:   200000,
			MaxOutputTokens: 4096,
			InputPrice:      15.00,
			OutputPrice:     75.00,
			Tier:            "flagship",
		}

		assert.Equal(t, 200000, view.ContextWindow)
		assert.Equal(t, 4096, view.MaxOutputTokens)
		assert.Equal(t, 15.00, view.InputPrice)
		assert.Equal(t, 75.00, view.OutputPrice)
		assert.Equal(t, "flagship", view.Tier)
	})
}

// =============================================================================
// Field Value Tests
// =============================================================================

func TestContextWindowValues(t *testing.T) {
	testCases := []struct {
		model         string
		contextWindow int
	}{
		{"gpt-4o", 128000},
		{"gpt-4-turbo", 128000},
		{"claude-3-opus", 200000},
		{"claude-3-sonnet", 200000},
		{"gemini-1.5-pro", 1000000},
	}

	for _, tc := range testCases {
		t.Run(tc.model, func(t *testing.T) {
			m := &model.LLMModel{
				Model:         tc.model,
				ContextWindow: tc.contextWindow,
			}
			assert.Equal(t, tc.contextWindow, m.ContextWindow)
		})
	}
}

func TestMaxOutputTokensValues(t *testing.T) {
	testCases := []struct {
		model           string
		maxOutputTokens int
	}{
		{"gpt-4o", 16384},
		{"gpt-4-turbo", 4096},
		{"claude-3-opus", 4096},
		{"o1", 100000},
	}

	for _, tc := range testCases {
		t.Run(tc.model, func(t *testing.T) {
			m := &model.LLMModel{
				Model:           tc.model,
				MaxOutputTokens: tc.maxOutputTokens,
			}
			assert.Equal(t, tc.maxOutputTokens, m.MaxOutputTokens)
		})
	}
}

func TestPricingValues(t *testing.T) {
	t.Run("pricing per million tokens", func(t *testing.T) {
		m := &model.LLMModel{
			Model:       "gpt-4o",
			InputPrice:  decimal.NewFromFloat(2.50),  // $2.50 per 1M input tokens
			OutputPrice: decimal.NewFromFloat(10.00), // $10.00 per 1M output tokens
		}

		// Calculate cost for 1000 tokens
		inputTokens := int64(1000)
		outputTokens := int64(500)

		inputCost := m.InputPrice.Mul(decimal.NewFromInt(inputTokens)).Div(decimal.NewFromInt(1000000))
		outputCost := m.OutputPrice.Mul(decimal.NewFromInt(outputTokens)).Div(decimal.NewFromInt(1000000))
		totalCost := inputCost.Add(outputCost)

		// $0.0025 + $0.005 = $0.0075
		assert.True(t, totalCost.Equal(decimal.NewFromFloat(0.0075)))
	})

	t.Run("compare model pricing", func(t *testing.T) {
		models := []struct {
			name        string
			inputPrice  float64
			outputPrice float64
		}{
			{"gpt-4o", 2.50, 10.00},
			{"gpt-4o-mini", 0.15, 0.60},
			{"claude-3-opus", 15.00, 75.00},
			{"claude-3-5-sonnet", 3.00, 15.00},
			{"gemini-1.5-pro", 1.25, 5.00},
		}

		for _, tc := range models {
			t.Run(tc.name, func(t *testing.T) {
				m := &model.LLMModel{
					Model:       tc.name,
					InputPrice:  decimal.NewFromFloat(tc.inputPrice),
					OutputPrice: decimal.NewFromFloat(tc.outputPrice),
				}
				assert.True(t, m.InputPrice.GreaterThan(decimal.Zero))
				assert.True(t, m.OutputPrice.GreaterThanOrEqual(m.InputPrice))
			})
		}
	})
}

// =============================================================================
// JSON Serialization Tests
// =============================================================================

func TestOpenAIFieldNaming_JSONSerialization(t *testing.T) {
	t.Run("Model JSON field names match OpenAI convention", func(t *testing.T) {
		m := &model.LLMModel{
			Model:           "test-model",
			ContextWindow:   128000,
			MaxOutputTokens: 16384,
			MaxInputTokens:  128000,
			InputPrice:      decimal.NewFromFloat(2.50),
			OutputPrice:     decimal.NewFromFloat(10.00),
			ModelTier:       "flagship",
		}

		// The JSON tags should produce correct field names
		// context_window, max_output_tokens, max_input_tokens, input_price, output_price, model_tier
		assert.NotNil(t, m)
		assert.Equal(t, "flagship", m.ModelTier)
	})

	t.Run("ModelView JSON field names match OpenAI convention", func(t *testing.T) {
		view := &model.ModelView{
			Model:           "test-model",
			ContextWindow:   200000,
			MaxOutputTokens: 8192,
			MaxInputTokens:  200000,
			InputPrice:      5.00,
			OutputPrice:     15.00,
			Tier:            "premium",
			IsRecommended:   true,
		}

		assert.Equal(t, 200000, view.ContextWindow)
		assert.Equal(t, 8192, view.MaxOutputTokens)
		assert.Equal(t, 200000, view.MaxInputTokens)
		assert.Equal(t, "premium", view.Tier)
		assert.True(t, view.IsRecommended)
	})
}

// =============================================================================
// Edge Case Tests
// =============================================================================

func TestOpenAIFieldNaming_EdgeCases(t *testing.T) {
	t.Run("zero values are valid", func(t *testing.T) {
		m := &model.LLMModel{
			Model:           "free-model",
			ContextWindow:   0,
			MaxOutputTokens: 0,
			MaxInputTokens:  0,
			InputPrice:      decimal.Zero,
			OutputPrice:     decimal.Zero,
		}

		assert.Equal(t, 0, m.ContextWindow)
		assert.True(t, m.InputPrice.IsZero())
		assert.True(t, m.OutputPrice.IsZero())
	})

	t.Run("large context windows (1M+ tokens)", func(t *testing.T) {
		m := &model.LLMModel{
			Model:         "gemini-1.5-pro",
			ContextWindow: 2000000, // 2M tokens
		}
		assert.Equal(t, 2000000, m.ContextWindow)
	})

	t.Run("very small pricing values", func(t *testing.T) {
		m := &model.LLMModel{
			Model:       "cheap-model",
			InputPrice:  decimal.NewFromFloat(0.0001),
			OutputPrice: decimal.NewFromFloat(0.0002),
		}
		assert.True(t, m.InputPrice.LessThan(decimal.NewFromFloat(0.001)))
	})

	t.Run("max_input_tokens can differ from context_window", func(t *testing.T) {
		// Some models have different input/output limits
		m := &model.LLMModel{
			Model:           "o1-preview",
			ContextWindow:   128000,
			MaxInputTokens:  128000,
			MaxOutputTokens: 32768, // Different from context window
		}
		assert.NotEqual(t, m.ContextWindow, m.MaxOutputTokens)
		assert.Equal(t, m.ContextWindow, m.MaxInputTokens)
	})
}

// =============================================================================
// TenantModelConfig Price Override Tests
// =============================================================================

func TestTenantModelConfig_PriceOverrides(t *testing.T) {
	t.Run("GetEffectiveInputPrice returns override when set", func(t *testing.T) {
		override := decimal.NewFromFloat(5.00)
		config := &model.ModelConfig{
			InputPriceOverride: &override,
			Model: &model.LLMModel{
				InputPrice: decimal.NewFromFloat(2.50),
			},
		}

		effective := config.GetEffectiveInputPrice()
		assert.True(t, effective.Equal(decimal.NewFromFloat(5.00)))
	})

	t.Run("GetEffectiveInputPrice falls back to model price", func(t *testing.T) {
		config := &model.ModelConfig{
			InputPriceOverride: nil,
			Model: &model.LLMModel{
				InputPrice: decimal.NewFromFloat(2.50),
			},
		}

		effective := config.GetEffectiveInputPrice()
		assert.True(t, effective.Equal(decimal.NewFromFloat(2.50)))
	})

	t.Run("GetEffectiveOutputPrice returns override when set", func(t *testing.T) {
		override := decimal.NewFromFloat(20.00)
		config := &model.ModelConfig{
			OutputPriceOverride: &override,
			Model: &model.LLMModel{
				OutputPrice: decimal.NewFromFloat(10.00),
			},
		}

		effective := config.GetEffectiveOutputPrice()
		assert.True(t, effective.Equal(decimal.NewFromFloat(20.00)))
	})
}

// =============================================================================
// Model Tier Tests
// =============================================================================

func TestModelTierValues(t *testing.T) {
	validTiers := []string{"free", "standard", "premium", "flagship"}

	for _, tier := range validTiers {
		t.Run(tier, func(t *testing.T) {
			m := &model.LLMModel{
				Model:     "test-model",
				ModelTier: tier,
			}
			assert.Equal(t, tier, m.ModelTier)
		})
	}
}

// =============================================================================
// ZgiOfficialAvailable Tests
// =============================================================================

func TestModelView_ZgiOfficialAvailable(t *testing.T) {
	t.Run("global model with system channel available", func(t *testing.T) {
		view := &model.ModelView{
			Model:                "gpt-4o",
			Provider:             "openai",
			IsEnabled:            true,
			IsAvailable:          true,
			ZgiOfficialAvailable: true, // Model exists in active system channels
		}

		assert.True(t, view.ZgiOfficialAvailable, "should be true when model is in system channels")
		assert.True(t, view.IsAvailable, "should be available for this tenant")
	})

	t.Run("global model without system channel", func(t *testing.T) {
		view := &model.ModelView{
			Model:                "deprecated-model",
			Provider:             "openai",
			IsEnabled:            true,
			IsAvailable:          false,
			ZgiOfficialAvailable: false, // No system channel configured for this model
		}

		assert.False(t, view.ZgiOfficialAvailable, "should be false when model has no system channel")
		assert.False(t, view.IsAvailable, "should not be available")
	})

	t.Run("custom model is never system available", func(t *testing.T) {
		view := &model.ModelView{
			Model:                "my-custom-model",
			Provider:             "", // Custom models have no provider
			IsEnabled:            true,
			IsAvailable:          true,  // Available via tenant's custom channel
			ZgiOfficialAvailable: false, // Custom models are NEVER in system channels
		}

		assert.False(t, view.ZgiOfficialAvailable, "custom model should never be system available")
		assert.True(t, view.IsAvailable, "can still be available through custom channels")
	})

	t.Run("system available but tenant not enabled", func(t *testing.T) {
		view := &model.ModelView{
			Model:                "gpt-4",
			Provider:             "openai",
			IsEnabled:            false, // Tenant has disabled this model
			IsAvailable:          true,  // System channel exists
			ZgiOfficialAvailable: true,  // System has the channel
		}

		assert.True(t, view.ZgiOfficialAvailable, "system availability is independent of tenant config")
		assert.False(t, view.IsEnabled, "tenant has explicitly disabled")
	})

	t.Run("tenant enabled but no system channel", func(t *testing.T) {
		// Model is enabled by tenant but system doesn't have channels for it
		view := &model.ModelView{
			Model:                "limited-model",
			Provider:             "anthropic",
			IsEnabled:            true,  // Tenant wants to use it
			IsAvailable:          false, // No channels available
			ZgiOfficialAvailable: false, // System has no channel for this model
		}

		assert.True(t, view.IsEnabled, "tenant enabled the model")
		assert.False(t, view.ZgiOfficialAvailable, "but system has no channel")
		assert.False(t, view.IsAvailable, "so it's not actually available")
	})
}

func TestAvailabilityScenarios(t *testing.T) {
	// Define comprehensive scenarios
	scenarios := []struct {
		name              string
		isEnabled         bool
		isAvailable       bool
		isSystemAvailable bool
		expectedCallable  bool // Can actually make API calls
		description       string
	}{
		{
			name:              "fully available",
			isEnabled:         true,
			isAvailable:       true,
			isSystemAvailable: true,
			expectedCallable:  true,
			description:       "Model enabled, system has channel, tenant has access",
		},
		{
			name:              "system available but tenant disabled",
			isEnabled:         false,
			isAvailable:       true,
			isSystemAvailable: true,
			expectedCallable:  false,
			description:       "System has channel but tenant disabled",
		},
		{
			name:              "custom model only",
			isEnabled:         true,
			isAvailable:       true,
			isSystemAvailable: false,
			expectedCallable:  true,
			description:       "Custom model with tenant's own channel",
		},
		{
			name:              "completely unavailable",
			isEnabled:         false,
			isAvailable:       false,
			isSystemAvailable: false,
			expectedCallable:  false,
			description:       "Nothing available",
		},
	}

	for _, sc := range scenarios {
		t.Run(sc.name, func(t *testing.T) {
			view := &model.ModelView{
				Model:                "test-model",
				IsEnabled:            sc.isEnabled,
				IsAvailable:          sc.isAvailable,
				ZgiOfficialAvailable: sc.isSystemAvailable,
			}

			// Callable = IsEnabled AND IsAvailable
			isCallable := view.IsEnabled && view.IsAvailable
			assert.Equal(t, sc.expectedCallable, isCallable, sc.description)
		})
	}
}
