package parameterextractor

import (
	"encoding/json"
	"fmt"

	"github.com/zgiai/zgi/api/internal/modules/app/workflow/nodes/base"
)

// ParameterType represents the type of a parameter
type ParameterType string

const (
	ParameterTypeString      ParameterType = "string"
	ParameterTypeNumber      ParameterType = "number"
	ParameterTypeBool        ParameterType = "bool"
	ParameterTypeBoolean     ParameterType = "boolean"
	ParameterTypeSelect      ParameterType = "select"
	ParameterTypeArrayString ParameterType = "array[string]"
	ParameterTypeArrayNumber ParameterType = "array[number]"
	ParameterTypeArrayBool   ParameterType = "array[boolean]"
	ParameterTypeArrayObject ParameterType = "array[object]"
)

// ReasoningMode represents the reasoning mode for parameter extraction
// Only "prompt" mode is supported after Gateway migration
type ReasoningMode string

const (
	ReasoningModePrompt ReasoningMode = "prompt"
)

const AppType = "agent"

// PromptMessage represents a message in the conversation for Gateway
type PromptMessage struct {
	Role    string `json:"role"`
	Content any    `json:"content"`
}

// ParameterConfig represents the configuration for a single parameter
type ParameterConfig struct {
	Name        string        `json:"name"`        // Parameter name
	Type        ParameterType `json:"type"`        // Parameter type
	Description string        `json:"description"` // Parameter description for LLM
	Required    bool          `json:"required"`    // Whether parameter is required
	Options     []string      `json:"options"`     // Valid options for select type
}

// ModelConfig represents the LLM model configuration
type ModelConfig struct {
	Provider         string         `json:"provider"`          // Model provider (openai, deepseek, etc.)
	Name             string         `json:"name"`              // Model name (gpt-4, etc.)
	Mode             string         `json:"mode"`              // "chat" or "completion"
	CompletionParams map[string]any `json:"completion_params"` // temperature, max_tokens, etc.
}

// VisionConfigOptions represents vision configuration options
type VisionConfigOptions struct {
	VariableSelector []string `json:"variable_selector"`
	Detail           string   `json:"detail"` // "high" or "low"
}

// VisionConfig represents vision configuration
type VisionConfig struct {
	Enabled bool                `json:"enabled"`
	Configs VisionConfigOptions `json:"configs"`
}

// NodeData represents the data structure for parameter extractor nodes
type NodeData struct {
	base.NodeData
	Model         ModelConfig       `json:"model"`                 // LLM model configuration
	Query         []string          `json:"query"`                 // Variable selector for input text
	Parameters    []ParameterConfig `json:"parameters"`            // Parameters to extract
	Instruction   *string           `json:"instruction,omitempty"` // Custom extraction instructions
	ReasoningMode ReasoningMode     `json:"reasoning_mode"`        // Only "prompt" mode supported
	Vision        VisionConfig      `json:"vision"`                // Image processing config

	// IsStaticConfig marks whether this config came from graph (true) or runtime (false)
	// This field is not serialized to JSON
	IsStaticConfig bool `json:"-"`
}

// parseParameterExtractorNodeDataFromConfig parses configuration into NodeData
func parseParameterExtractorNodeDataFromConfig(config map[string]any) (NodeData, string, error) {
	// Extract node ID
	nodeID, ok := config["id"]
	if !ok {
		return NodeData{}, "", fmt.Errorf("node ID is required")
	}
	nodeIDStr, ok := nodeID.(string)
	if !ok {
		return NodeData{}, "", fmt.Errorf("node ID must be string")
	}

	// Extract node data
	data, ok := config["data"]
	if !ok {
		return NodeData{}, "", fmt.Errorf("node data is required")
	}

	// Marshal and unmarshal to parse into NodeData
	jsonBytes, err := json.Marshal(data)
	if err != nil {
		return NodeData{}, "", fmt.Errorf("failed to marshal node data: %w", err)
	}

	var nodeData NodeData
	if err := json.Unmarshal(jsonBytes, &nodeData); err != nil {
		return NodeData{}, "", fmt.Errorf("failed to unmarshal node data: %w", err)
	}
	normalizeNodeDataParameterTypes(&nodeData)

	// Validate configuration
	if err := validateNodeData(&nodeData); err != nil {
		return NodeData{}, "", fmt.Errorf("invalid node configuration: %w", err)
	}

	return nodeData, nodeIDStr, nil
}

func normalizeNodeDataParameterTypes(nd *NodeData) {
	if nd == nil {
		return
	}
	for i := range nd.Parameters {
		nd.Parameters[i].Type = normalizeParameterType(nd.Parameters[i].Type)
	}
}

func normalizeParameterType(paramType ParameterType) ParameterType {
	if paramType == ParameterTypeBoolean {
		return ParameterTypeBool
	}
	return paramType
}

// validateNodeData validates the node configuration
func validateNodeData(nd *NodeData) error {
	// Validate at least one parameter is defined
	if len(nd.Parameters) == 0 {
		return fmt.Errorf("at least one parameter is required")
	}

	// Validate parameter names are unique
	paramNames := make(map[string]bool)
	for i := range nd.Parameters {
		param := &nd.Parameters[i]
		param.Type = normalizeParameterType(param.Type)

		if param.Name == "" {
			return fmt.Errorf("parameter %d: name is required", i)
		}
		if paramNames[param.Name] {
			return fmt.Errorf("parameter name '%s' is duplicated", param.Name)
		}
		paramNames[param.Name] = true

		// Validate select type has options
		if param.Type == ParameterTypeSelect {
			if len(param.Options) == 0 {
				return fmt.Errorf("parameter '%s': select type must have at least one option", param.Name)
			}
		}

		// Validate array types have valid element types
		if param.Type == ParameterTypeArrayString ||
			param.Type == ParameterTypeArrayNumber ||
			param.Type == ParameterTypeArrayBool ||
			param.Type == ParameterTypeArrayObject {
			// Array types are valid, no additional validation needed
		} else if param.Type != ParameterTypeString &&
			param.Type != ParameterTypeNumber &&
			param.Type != ParameterTypeBool &&
			param.Type != ParameterTypeSelect {
			return fmt.Errorf("parameter '%s': invalid type '%s'", param.Name, param.Type)
		}
	}

	// Validate model configuration is complete
	if nd.Model.Provider == "" {
		return fmt.Errorf("model provider is required")
	}
	if nd.Model.Name == "" {
		return fmt.Errorf("model name is required")
	}
	if nd.Model.Mode == "" {
		return fmt.Errorf("model mode is required")
	}
	if nd.Model.Mode != "chat" && nd.Model.Mode != "completion" {
		return fmt.Errorf("model mode must be 'chat' or 'completion'")
	}

	// Validate query selector
	if len(nd.Query) == 0 {
		return fmt.Errorf("query variable selector is required")
	}

	return nil
}
