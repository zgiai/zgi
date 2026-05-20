package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// validateWorkflowInputs validates workflow inputs including model_config variables
func (h *WorkflowHandler) validateWorkflowInputs(
	ctx context.Context,
	workflow interface{},
	inputs map[string]interface{},
) error {
	// Parse workflow data
	workflowData, ok := workflow.(map[string]interface{})
	if !ok {
		return nil // Skip validation if workflow data format is unexpected
	}

	// Get graph configuration
	graphConfig, ok := workflowData["graph"].(map[string]interface{})
	if !ok {
		// Try parsing from string
		if graphStr, ok := workflowData["graph"].(string); ok {
			var graphMap map[string]interface{}
			if err := json.Unmarshal([]byte(graphStr), &graphMap); err != nil {
				return nil // Skip validation if graph cannot be parsed
			}
			graphConfig = graphMap
		} else {
			return nil // Skip validation if no graph config
		}
	}

	// Find start node configuration
	startNode := h.findStartNode(graphConfig)
	if startNode == nil {
		return nil // No validation needed if no start node
	}

	// Parse start node variables
	variables := h.parseStartNodeVariables(startNode)

	// Validate each input
	for key, value := range inputs {
		// Skip system variables
		if strings.HasPrefix(key, "sys.") {
			continue
		}

		variable := h.findVariable(variables, key)
		if variable == nil {
			continue // Skip validation for undefined variables
		}

		// Validate model_config type variables
		varType, ok := (*variable)["type"].(string)
		if ok && varType == "model_config" {
			if err := h.validateModelConfigInput(key, value); err != nil {
				return fmt.Errorf("invalid model_config input '%s': %w", key, err)
			}
		}
	}

	return nil
}

// findStartNode finds the start node in the workflow graph
func (h *WorkflowHandler) findStartNode(graphConfig map[string]interface{}) map[string]interface{} {
	nodes, ok := graphConfig["nodes"].([]interface{})
	if !ok {
		return nil
	}

	for _, nodeInterface := range nodes {
		node, ok := nodeInterface.(map[string]interface{})
		if !ok {
			continue
		}

		nodeType, ok := node["type"].(string)
		if !ok {
			continue
		}

		if nodeType == "start" {
			return node
		}
	}

	return nil
}

// parseStartNodeVariables parses variables from start node configuration
func (h *WorkflowHandler) parseStartNodeVariables(startNode map[string]interface{}) []map[string]interface{} {
	data, ok := startNode["data"].(map[string]interface{})
	if !ok {
		return nil
	}

	variablesInterface, ok := data["variables"].([]interface{})
	if !ok {
		return nil
	}

	variables := make([]map[string]interface{}, 0, len(variablesInterface))
	for _, varInterface := range variablesInterface {
		if varMap, ok := varInterface.(map[string]interface{}); ok {
			variables = append(variables, varMap)
		}
	}

	return variables
}

// findVariable finds a variable definition by name
func (h *WorkflowHandler) findVariable(variables []map[string]interface{}, name string) *map[string]interface{} {
	for i := range variables {
		variable := &variables[i]
		if varName, ok := (*variable)["variable"].(string); ok && varName == name {
			return variable
		}
	}
	return nil
}

// validateModelConfigInput validates a model_config input value
func (h *WorkflowHandler) validateModelConfigInput(key string, value interface{}) error {
	configMap, ok := value.(map[string]interface{})
	if !ok {
		return fmt.Errorf("must be an object")
	}

	// Validate required fields
	provider, hasProvider := configMap["provider"]
	if !hasProvider {
		return fmt.Errorf("missing required field 'provider'")
	}
	if providerStr, ok := provider.(string); !ok || providerStr == "" {
		return fmt.Errorf("field 'provider' must be a non-empty string")
	}

	model, hasModel := configMap["model"]
	if !hasModel {
		return fmt.Errorf("missing required field 'model'")
	}
	if modelStr, ok := model.(string); !ok || modelStr == "" {
		return fmt.Errorf("field 'model' must be a non-empty string")
	}

	// Validate optional mode field
	if mode, hasMode := configMap["mode"]; hasMode {
		if modeStr, ok := mode.(string); ok {
			if modeStr != "chat" && modeStr != "completion" {
				return fmt.Errorf("field 'mode' must be 'chat' or 'completion'")
			}
		} else {
			return fmt.Errorf("field 'mode' must be a string")
		}
	}

	// Validate optional completion_params field
	if params, hasParams := configMap["completion_params"]; hasParams {
		if _, ok := params.(map[string]interface{}); !ok {
			return fmt.Errorf("field 'completion_params' must be an object")
		}
	}

	return nil
}
