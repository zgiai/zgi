package end

import (
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/shared"
)

// EndStreamGeneratorRouter is the end stream generator router
type EndStreamGeneratorRouter struct{}

// Init initializes stream generation routing
func (r *EndStreamGeneratorRouter) Init(
	nodeIDConfigMapping map[string]map[string]interface{},
	reverseEdgeMapping map[string][]*GraphEdge,
	nodeParallelMapping map[string]string,
) (*EndStreamParam, error) {
	// Parse end nodes for stream output node value selectors
	endStreamVariableSelectorsMapping := make(map[string][][]string)

	for endNodeID, nodeConfig := range nodeIDConfigMapping {
		data, ok := nodeConfig["data"].(map[string]interface{})
		if !ok {
			continue
		}

		nodeType, ok := data["type"].(string)
		if !ok || nodeType != string(shared.End) {
			continue
		}

		// Skip end nodes in parallel
		if _, exists := nodeParallelMapping[endNodeID]; exists {
			continue
		}

		// Get generation route for stream output
		streamVariableSelectors := r.extractStreamVariableSelector(nodeIDConfigMapping, nodeConfig)
		endStreamVariableSelectorsMapping[endNodeID] = streamVariableSelectors
	}

	// Get end dependencies
	endNodeIDs := make([]string, 0, len(endStreamVariableSelectorsMapping))
	for endNodeID := range endStreamVariableSelectorsMapping {
		endNodeIDs = append(endNodeIDs, endNodeID)
	}

	endDependencies, err := r.fetchEndsDependencies(
		endNodeIDs,
		reverseEdgeMapping,
		nodeIDConfigMapping,
	)
	if err != nil {
		return nil, err
	}

	return &EndStreamParam{
		EndStreamVariableSelectorMapping: endStreamVariableSelectorsMapping,
		EndDependencies:                  endDependencies,
	}, nil
}

// ExtractStreamVariableSelectorFromNodeData extracts stream variable selectors from node data
func (r *EndStreamGeneratorRouter) ExtractStreamVariableSelectorFromNodeData(
	nodeIDConfigMapping map[string]map[string]interface{},
	nodeData *NodeData,
) [][]string {
	variableSelectors := nodeData.Outputs
	valueSelectors := make([][]string, 0)

	for _, variableSelector := range variableSelectors {
		if len(variableSelector.ValueSelector) == 0 {
			continue
		}

		nodeID := variableSelector.ValueSelector[0]
		if nodeID != "sys" {
			if nodeConfig, exists := nodeIDConfigMapping[nodeID]; exists {
				data, ok := nodeConfig["data"].(map[string]interface{})
				if !ok {
					continue
				}

				nodeType, ok := data["type"].(string)
				if !ok {
					continue
				}

				// Check if it's an LLM node and variable selector is text
				if nodeType == string(shared.LLM) && len(variableSelector.ValueSelector) > 1 && variableSelector.ValueSelector[1] == "text" {
					// Avoid duplicate additions
					exists := false
					for _, existing := range valueSelectors {
						if len(existing) == len(variableSelector.ValueSelector) {
							match := true
							for i, v := range existing {
								if v != variableSelector.ValueSelector[i] {
									match = false
									break
								}
							}
							if match {
								exists = true
								break
							}
						}
					}
					if !exists {
						valueSelectors = append(valueSelectors, variableSelector.ValueSelector)
					}
				}
			}
		}
	}

	return valueSelectors
}

// extractStreamVariableSelector extracts stream variable selectors from node configuration
func (r *EndStreamGeneratorRouter) extractStreamVariableSelector(
	nodeIDConfigMapping map[string]map[string]interface{},
	config map[string]interface{},
) [][]string {
	data, ok := config["data"].(map[string]interface{})
	if !ok {
		return [][]string{}
	}

	// Parse NodeData
	nodeData := &NodeData{}
	if outputs, ok := data["outputs"].([]interface{}); ok {
		for _, output := range outputs {
			if outputMap, ok := output.(map[string]interface{}); ok {
				variableSelector := VariableSelector{}
				if variable, ok := outputMap["variable"].(string); ok {
					variableSelector.Variable = variable
				}
				if selector, ok := outputMap["selector"].([]interface{}); ok {
					valueSelector := make([]string, len(selector))
					for i, v := range selector {
						if s, ok := v.(string); ok {
							valueSelector[i] = s
						}
					}
					variableSelector.ValueSelector = valueSelector
				}
				nodeData.Outputs = append(nodeData.Outputs, variableSelector)
			}
		}
	}

	return r.ExtractStreamVariableSelectorFromNodeData(nodeIDConfigMapping, nodeData)
}

// fetchEndsDependencies gets end dependencies
func (r *EndStreamGeneratorRouter) fetchEndsDependencies(
	endNodeIDs []string,
	reverseEdgeMapping map[string][]*GraphEdge,
	nodeIDConfigMapping map[string]map[string]interface{},
) (map[string][]string, error) {
	endDependencies := make(map[string][]string)

	for _, endNodeID := range endNodeIDs {
		if endDependencies[endNodeID] == nil {
			endDependencies[endNodeID] = make([]string, 0)
		}

		err := r.recursiveFetchEndDependencies(
			endNodeID,
			endNodeID,
			nodeIDConfigMapping,
			reverseEdgeMapping,
			endDependencies,
		)
		if err != nil {
			return nil, err
		}
	}

	return endDependencies, nil
}

// recursiveFetchEndDependencies recursively gets end dependencies
func (r *EndStreamGeneratorRouter) recursiveFetchEndDependencies(
	currentNodeID string,
	endNodeID string,
	nodeIDConfigMapping map[string]map[string]interface{},
	reverseEdgeMapping map[string][]*GraphEdge,
	endDependencies map[string][]string,
) error {
	reverseEdges, exists := reverseEdgeMapping[currentNodeID]
	if !exists {
		return nil
	}

	for _, edge := range reverseEdges {
		sourceNodeID := edge.SourceNodeID
		if _, exists := nodeIDConfigMapping[sourceNodeID]; !exists {
			continue
		}

		nodeConfig := nodeIDConfigMapping[sourceNodeID]
		data, ok := nodeConfig["data"].(map[string]interface{})
		if !ok {
			continue
		}

		sourceNodeType, ok := data["type"].(string)
		if !ok {
			continue
		}

		// Check if it's a specific node type
		if sourceNodeType == string(shared.IfElse) || sourceNodeType == string(shared.QuestionClassifier) {
			endDependencies[endNodeID] = append(endDependencies[endNodeID], sourceNodeID)
		} else {
			err := r.recursiveFetchEndDependencies(
				sourceNodeID,
				endNodeID,
				nodeIDConfigMapping,
				reverseEdgeMapping,
				endDependencies,
			)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

// GraphEdge defines a graph edge
type GraphEdge struct {
	SourceNodeID string                 `json:"source_node_id"`
	TargetNodeID string                 `json:"target_node_id"`
	RunCondition map[string]interface{} `json:"run_condition,omitempty"`
}
