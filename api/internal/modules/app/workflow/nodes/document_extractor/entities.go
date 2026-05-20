package documentextractor

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/zgiai/zgi/api/internal/modules/app/workflow/nodes/base"
)

const (
	// RetryInterval is the fixed interval between retry attempts
	RetryInterval = 2 * time.Second
	// RetryTimeout is the maximum total time to retry before giving up
	RetryTimeout = 60 * time.Second
)

// NodeData represents the data structure for document extractor nodes
type NodeData struct {
	base.NodeData
	VariableSelector []string `json:"variable_selector"` // Variable selector pointing to file or file array
}

// parseDocumentExtractorNodeDataFromConfig parses node data and id from config
func parseDocumentExtractorNodeDataFromConfig(config map[string]any) (NodeData, string, error) {
	// 1. Get node ID
	nodeID, ok := config["id"]
	if !ok {
		return NodeData{}, "", fmt.Errorf("node ID is required")
	}
	nodeIDStr, ok := nodeID.(string)
	if !ok {
		return NodeData{}, "", fmt.Errorf("node ID must be string")
	}

	// 2. Get node data
	data, ok := config["data"]
	if !ok {
		return NodeData{}, "", fmt.Errorf("node data is required")
	}

	// 3. Convert to JSON and back to parse structure into NodeData
	jsonBytes, err := json.Marshal(data)
	if err != nil {
		return NodeData{}, "", fmt.Errorf("failed to marshal node data: %w", err)
	}

	var nodeData NodeData
	if err := json.Unmarshal(jsonBytes, &nodeData); err != nil {
		return NodeData{}, "", fmt.Errorf("failed to unmarshal node data: %w", err)
	}

	// 4. Validate required fields
	if len(nodeData.VariableSelector) == 0 {
		return NodeData{}, "", fmt.Errorf("variable_selector is required")
	}

	return nodeData, nodeIDStr, nil
}
