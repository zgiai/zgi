package variableaggregator

import (
	"encoding/json"
	"fmt"

	"github.com/zgiai/ginext/internal/modules/app/workflow/nodes/base"
	"github.com/zgiai/ginext/internal/modules/app/workflow/shared"
)

// NodeData represents the configuration for variable aggregator node
type NodeData struct {
	base.NodeData
	OutputType       shared.SegmentType `json:"output_type"`       // Output type filter
	Variables        [][]string         `json:"variables"`         // Variable selectors (priority order)
	AdvancedSettings *AdvancedSettings  `json:"advanced_settings"` // Optional multi-group configuration
}

// AdvancedSettings enables multi-group mode
type AdvancedSettings struct {
	GroupEnabled bool    `json:"group_enabled"` // Enable multi-group mode
	Groups       []Group `json:"groups"`        // Group configurations
}

// Group represents one aggregation group in multi-group mode
type Group struct {
	GroupName  string             `json:"group_name"`  // Group identifier
	OutputType shared.SegmentType `json:"output_type"` // Output type for this group
	Variables  [][]string         `json:"variables"`   // Variable selectors for this group
}

// parseVariableAggregatorNodeDataFromConfig parses configuration into NodeData
func parseVariableAggregatorNodeDataFromConfig(config map[string]any) (NodeData, string, error) {
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

	// Validate configuration
	if err := validateNodeData(&nodeData); err != nil {
		return NodeData{}, "", fmt.Errorf("invalid node configuration: %w", err)
	}

	return nodeData, nodeIDStr, nil
}

// validateNodeData validates the node configuration
func validateNodeData(nd *NodeData) error {
	// Check if advanced settings is enabled
	if nd.AdvancedSettings != nil && nd.AdvancedSettings.GroupEnabled {
		// Multi-group mode validation
		if len(nd.AdvancedSettings.Groups) == 0 {
			return fmt.Errorf("at least one group is required in multi-group mode")
		}

		// Validate each group
		for i, group := range nd.AdvancedSettings.Groups {
			if group.GroupName == "" {
				return fmt.Errorf("group %d: group_name is required", i)
			}
			if len(group.Variables) == 0 {
				return fmt.Errorf("group %s: at least one variable is required", group.GroupName)
			}
			// Validate variable selectors
			for j, selector := range group.Variables {
				if len(selector) < 2 {
					return fmt.Errorf("group %s, variable %d: selector must have at least 2 elements", group.GroupName, j)
				}
			}
		}
	} else {
		// Single-group mode validation
		if len(nd.Variables) == 0 {
			return fmt.Errorf("at least one variable is required")
		}
		// Validate variable selectors
		for i, selector := range nd.Variables {
			if len(selector) < 2 {
				return fmt.Errorf("variable %d: selector must have at least 2 elements", i)
			}
		}
	}

	return nil
}
