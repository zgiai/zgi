package model

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWorkflowUsesTextChatWithoutSeparateUseCase(t *testing.T) {
	useCases := EnsureUseCases([]string{"chat"}, nil)

	require.Equal(t, []string{"text-chat"}, useCases)
	assert.NotContains(t, ValidUseCases(), UseCase("workflow"))
}

// TestParameterDefinition tests the ParameterDefinition struct
func TestParameterDefinition(t *testing.T) {
	t.Run("slider parameter", func(t *testing.T) {
		param := ParameterDefinition{
			Name:        "temperature",
			Type:        "slider",
			Label:       "Temperature",
			Default:     0.7,
			Min:         floatPtr(0),
			Max:         floatPtr(2),
			Step:        floatPtr(0.1),
			Description: "Controls randomness",
		}

		assert.Equal(t, "temperature", param.Name)
		assert.Equal(t, "slider", param.Type)
		assert.Equal(t, 0.7, param.Default)
		assert.Equal(t, float64(0), *param.Min)
		assert.Equal(t, float64(2), *param.Max)
	})

	t.Run("select parameter", func(t *testing.T) {
		param := ParameterDefinition{
			Name:    "reasoning_effort",
			Type:    "select",
			Label:   "Reasoning Effort",
			Default: "medium",
			Options: []string{"low", "medium", "high"},
		}

		assert.Equal(t, "reasoning_effort", param.Name)
		assert.Equal(t, "select", param.Type)
		assert.Equal(t, "medium", param.Default)
		assert.Len(t, param.Options, 3)
		assert.Contains(t, param.Options, "medium")
	})

	t.Run("switch parameter", func(t *testing.T) {
		param := ParameterDefinition{
			Name:    "stream",
			Type:    "switch",
			Label:   "Stream Output",
			Default: false,
		}

		assert.Equal(t, "switch", param.Type)
		assert.Equal(t, false, param.Default)
	})
}

// TestParameterDefinitions tests the ParameterDefinitions slice type
func TestParameterDefinitions(t *testing.T) {
	t.Run("JSON marshal/unmarshal", func(t *testing.T) {
		params := ParameterDefinitions{
			{
				Name:    "temperature",
				Type:    "slider",
				Label:   "Temperature",
				Default: 0.7,
				Min:     floatPtr(0),
				Max:     floatPtr(2),
			},
			{
				Name:    "reasoning_effort",
				Type:    "select",
				Label:   "Reasoning Effort",
				Default: "medium",
				Options: []string{"low", "medium", "high"},
			},
		}

		// Marshal to JSON
		data, err := json.Marshal(params)
		require.NoError(t, err)
		assert.Contains(t, string(data), "temperature")
		assert.Contains(t, string(data), "reasoning_effort")

		// Unmarshal back
		var decoded ParameterDefinitions
		err = json.Unmarshal(data, &decoded)
		require.NoError(t, err)
		assert.Len(t, decoded, 2)
		assert.Equal(t, "temperature", decoded[0].Name)
		assert.Equal(t, "reasoning_effort", decoded[1].Name)
	})

	t.Run("GORM Value/Scan", func(t *testing.T) {
		params := ParameterDefinitions{
			{Name: "temperature", Type: "slider", Default: 0.7},
		}

		// Test Value()
		val, err := params.Value()
		require.NoError(t, err)
		assert.NotNil(t, val)

		// Test Scan()
		var scanned ParameterDefinitions
		err = scanned.Scan(val)
		require.NoError(t, err)
		assert.Len(t, scanned, 1)
		assert.Equal(t, "temperature", scanned[0].Name)
	})

	t.Run("Scan nil value", func(t *testing.T) {
		var params ParameterDefinitions
		err := params.Scan(nil)
		require.NoError(t, err)
		assert.Nil(t, params)
	})

	t.Run("Scan from []byte", func(t *testing.T) {
		jsonData := `[{"name":"top_p","type":"slider","label":"Top P","default":1}]`

		var params ParameterDefinitions
		err := params.Scan([]byte(jsonData))
		require.NoError(t, err)
		assert.Len(t, params, 1)
		assert.Equal(t, "top_p", params[0].Name)
	})

	t.Run("Scan from object map", func(t *testing.T) {
		jsonData := `{
			"temperature": {
				"supported": true,
				"default": 0.7,
				"min": 0,
				"max": 2,
				"description": "Controls randomness"
			},
			"stream": {
				"supported": true,
				"default": false
			},
			"reasoning_effort": {
				"supported": true,
				"default": "medium",
				"options": ["low", "medium", "high"]
			},
			"seed": {
				"supported": false,
				"default": 1
			},
			"parallel_tool_calls": true
		}`

		var params ParameterDefinitions
		err := params.Scan([]byte(jsonData))
		require.NoError(t, err)
		assert.Len(t, params, 3)

		byName := make(map[string]ParameterDefinition, len(params))
		for _, param := range params {
			byName[param.Name] = param
		}

		temperature, ok := byName["temperature"]
		require.True(t, ok)
		assert.Equal(t, "number", temperature.Type)
		assert.Equal(t, 0.7, temperature.Default)
		require.NotNil(t, temperature.Min)
		require.NotNil(t, temperature.Max)
		assert.Equal(t, float64(0), *temperature.Min)
		assert.Equal(t, float64(2), *temperature.Max)

		stream, ok := byName["stream"]
		require.True(t, ok)
		assert.Equal(t, "switch", stream.Type)
		assert.Equal(t, false, stream.Default)

		reasoning, ok := byName["reasoning_effort"]
		require.True(t, ok)
		assert.Equal(t, "select", reasoning.Type)
		assert.Equal(t, "medium", reasoning.Default)
		assert.Equal(t, []string{"low", "medium", "high"}, reasoning.Options)

		_, hasSeed := byName["seed"]
		assert.False(t, hasSeed)
		_, hasParallelToolCalls := byName["parallel_tool_calls"]
		assert.False(t, hasParallelToolCalls)
	})

	t.Run("Scan invalid type", func(t *testing.T) {
		var params ParameterDefinitions
		err := params.Scan(123) // Invalid type
		assert.Error(t, err)
	})
}

// TestModelParameterSupport tests the ModelParameters struct
func TestModelParameterSupport(t *testing.T) {
	t.Run("GPT-4o parameters", func(t *testing.T) {
		params := ModelParameters{
			SupportsTemperature:      true,
			SupportsTopP:             true,
			SupportsPresencePenalty:  true,
			SupportsFrequencyPenalty: true,
			SupportsSeed:             true,
			SupportsStop:             true,
			SupportsLogitBias:        true,
			MaxStopSequences:         4,
		}

		assert.True(t, params.SupportsTemperature)
		assert.True(t, params.SupportsTopP)
		assert.True(t, params.SupportsPresencePenalty)
		assert.True(t, params.SupportsFrequencyPenalty)
		assert.Equal(t, 4, params.MaxStopSequences)
	})

	t.Run("O1 parameters (no temperature)", func(t *testing.T) {
		params := ModelParameters{
			SupportsTemperature:      false, // O1 doesn't support temperature
			SupportsTopP:             false, // O1 doesn't support top_p
			SupportsPresencePenalty:  false,
			SupportsFrequencyPenalty: false,
			SupportsSeed:             false,
			SupportsStop:             false,
			SupportsLogitBias:        false,
			MaxStopSequences:         0,
		}

		assert.False(t, params.SupportsTemperature)
		assert.False(t, params.SupportsTopP)
	})
}

// TestModelFeatures tests the ModelFeatures struct
func TestModelFeatures(t *testing.T) {
	t.Run("Chat model features", func(t *testing.T) {
		features := ModelFeatures{
			Streaming:       true,
			FunctionCalling: true,
			JsonMode:        true,
			Attachment:      true,
			Reasoning:       false,
			SystemPrompt:    true,
			ReasoningEffort: false,
		}

		assert.True(t, features.Streaming)
		assert.True(t, features.FunctionCalling)
		assert.True(t, features.JsonMode)
		assert.False(t, features.Reasoning)
		assert.False(t, features.ReasoningEffort)
	})

	t.Run("O1 model features", func(t *testing.T) {
		features := ModelFeatures{
			Reasoning:       true,
			ReasoningEffort: true,
			SystemPrompt:    false, // O1 doesn't support system prompt
			Streaming:       false, // Limited streaming for O1
		}

		assert.True(t, features.Reasoning)
		assert.True(t, features.ReasoningEffort)
		assert.False(t, features.SystemPrompt)
	})
}

// TestModelTools tests the ModelTools struct
func TestModelTools(t *testing.T) {
	t.Run("GPT-4o tools", func(t *testing.T) {
		tools := ModelTools{
			ParallelToolCalls: true,
			CodeInterpreter:   true,
			FileSearch:        true,
			ComputerUse:       false,
			Mcp:               false,
		}

		assert.True(t, tools.ParallelToolCalls)
		assert.True(t, tools.CodeInterpreter)
		assert.False(t, tools.ComputerUse)
	})

	t.Run("Claude computer use", func(t *testing.T) {
		tools := ModelTools{
			ParallelToolCalls: false,
			CodeInterpreter:   false,
			FileSearch:        false,
			ComputerUse:       true,
			Mcp:               true,
		}

		assert.True(t, tools.ComputerUse)
		assert.True(t, tools.Mcp)
	})
}

// TestModelEndpoints tests the ModelEndpoints struct
func TestModelEndpoints(t *testing.T) {
	t.Run("Chat model endpoints", func(t *testing.T) {
		endpoints := ModelEndpoints{
			ChatCompletions:  true,
			Embeddings:       false,
			ImageGeneration:  false,
			SpeechGeneration: false,
			Transcription:    false,
			Realtime:         false,
			Batch:            true,
			Responses:        true,
		}

		assert.True(t, endpoints.ChatCompletions)
		assert.False(t, endpoints.Embeddings)
		assert.True(t, endpoints.Batch)
	})

	t.Run("Embedding model endpoints", func(t *testing.T) {
		endpoints := ModelEndpoints{
			ChatCompletions: false,
			Embeddings:      true,
		}

		assert.False(t, endpoints.ChatCompletions)
		assert.True(t, endpoints.Embeddings)
	})
}

// Helper function for tests
func floatPtr(f float64) *float64 {
	return &f
}
