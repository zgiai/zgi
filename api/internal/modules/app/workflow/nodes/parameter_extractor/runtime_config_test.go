package parameterextractor

import (
	"testing"

	"github.com/zgiai/zgi/api/internal/modules/app/workflow/graph_engine/entities"
)

// TestGetRuntimeModelConfig tests the getRuntimeModelConfig method
func TestGetRuntimeModelConfig(t *testing.T) {
	tests := []struct {
		name           string
		userInputs     map[string]interface{}
		expectedConfig *ModelConfig
		expectNil      bool
	}{
		{
			name: "Valid runtime config with all fields",
			userInputs: map[string]interface{}{
				"model_config": map[string]interface{}{
					"provider": "openai",
					"model":    "gpt-4",
					"mode":     "chat",
					"completion_params": map[string]interface{}{
						"temperature": 0.7,
						"max_tokens":  1000,
					},
				},
			},
			expectedConfig: &ModelConfig{
				Provider: "openai",
				Name:     "gpt-4",
				Mode:     "chat",
				CompletionParams: map[string]any{
					"temperature": 0.7,
					"max_tokens":  1000,
				},
			},
			expectNil: false,
		},
		{
			name: "Valid runtime config without completion_params",
			userInputs: map[string]interface{}{
				"model_config": map[string]interface{}{
					"provider": "deepseek",
					"model":    "deepseek-chat",
					"mode":     "chat",
				},
			},
			expectedConfig: &ModelConfig{
				Provider:         "deepseek",
				Name:             "deepseek-chat",
				Mode:             "chat",
				CompletionParams: map[string]any{},
			},
			expectNil: false,
		},
		{
			name: "Missing provider - should return nil",
			userInputs: map[string]interface{}{
				"model_config": map[string]interface{}{
					"model": "gpt-4",
					"mode":  "chat",
				},
			},
			expectNil: true,
		},
		{
			name: "Missing model name - should return nil",
			userInputs: map[string]interface{}{
				"model_config": map[string]interface{}{
					"provider": "openai",
					"mode":     "chat",
				},
			},
			expectNil: true,
		},
		{
			name:       "No model_config in userInputs - should return nil",
			userInputs: map[string]interface{}{},
			expectNil:  true,
		},
		{
			name:       "Nil userInputs - should return nil",
			userInputs: nil,
			expectNil:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create node with GraphRuntimeState
			node := &Node{}
			node.GraphRuntimeState = &entities.GraphRuntimeState{
				VariablePool: &entities.VariablePool{
					UserInputs: tt.userInputs,
				},
			}

			// Call getRuntimeModelConfig
			result := node.getRuntimeModelConfig()

			// Check if result matches expectation
			if tt.expectNil {
				if result != nil {
					t.Errorf("Expected nil result, got: %+v", result)
				}
				return
			}

			if result == nil {
				t.Errorf("Expected non-nil result, got nil")
				return
			}

			// Verify fields
			if result.Provider != tt.expectedConfig.Provider {
				t.Errorf("Provider mismatch: expected %s, got %s", tt.expectedConfig.Provider, result.Provider)
			}
			if result.Name != tt.expectedConfig.Name {
				t.Errorf("Name mismatch: expected %s, got %s", tt.expectedConfig.Name, result.Name)
			}
			if result.Mode != tt.expectedConfig.Mode {
				t.Errorf("Mode mismatch: expected %s, got %s", tt.expectedConfig.Mode, result.Mode)
			}

			// Verify completion params
			if len(result.CompletionParams) != len(tt.expectedConfig.CompletionParams) {
				t.Errorf("CompletionParams length mismatch: expected %d, got %d",
					len(tt.expectedConfig.CompletionParams), len(result.CompletionParams))
			}

			for key, expectedValue := range tt.expectedConfig.CompletionParams {
				actualValue, exists := result.CompletionParams[key]
				if !exists {
					t.Errorf("CompletionParams missing key: %s", key)
					continue
				}
				if actualValue != expectedValue {
					t.Errorf("CompletionParams[%s] mismatch: expected %v, got %v", key, expectedValue, actualValue)
				}
			}
		})
	}
}

// TestGetRuntimeModelConfig_NilGraphRuntimeState tests behavior with nil GraphRuntimeState
func TestGetRuntimeModelConfig_NilGraphRuntimeState(t *testing.T) {
	node := &Node{}
	node.GraphRuntimeState = nil

	result := node.getRuntimeModelConfig()
	if result != nil {
		t.Errorf("Expected nil result when GraphRuntimeState is nil, got: %+v", result)
	}
}

// TestGetRuntimeModelConfig_NilVariablePool tests behavior with nil VariablePool
func TestGetRuntimeModelConfig_NilVariablePool(t *testing.T) {
	node := &Node{}
	node.GraphRuntimeState = &entities.GraphRuntimeState{
		VariablePool: nil,
	}

	result := node.getRuntimeModelConfig()
	if result != nil {
		t.Errorf("Expected nil result when VariablePool is nil, got: %+v", result)
	}
}
