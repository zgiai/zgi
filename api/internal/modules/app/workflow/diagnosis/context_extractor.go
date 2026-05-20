package diagnosis

import (
	"fmt"

	"github.com/zgiai/ginext/internal/modules/app/workflow/graph_engine"
	"github.com/zgiai/ginext/pkg/logger"
	"gopkg.in/yaml.v3"
)

// Extractor handles retrieving Node YAMLs and Upstream states from the Workflow Engine
type Extractor struct{}

// NewExtractor creates a new Extractor
func NewExtractor() *Extractor {
	return &Extractor{}
}

// MapToYAML converts a map to YAML string for prompt readability
func MapToYAML(m map[string]interface{}) string {
	if m == nil || len(m) == 0 {
		return ""
	}
	b, err := yaml.Marshal(m)
	if err != nil {
		return ""
	}
	return string(b)
}

// findDependencies recursively searches the nodeData for any string that matches a known node ID
func (e *Extractor) findDependencies(nodeData any, knownNodeIDs map[string]bool, deps map[string]bool) {
	switch v := nodeData.(type) {
	case string:
		if knownNodeIDs[v] {
			deps[v] = true
		}
	case []any:
		for _, item := range v {
			e.findDependencies(item, knownNodeIDs, deps)
		}
	case map[string]any:
		for _, item := range v {
			e.findDependencies(item, knownNodeIDs, deps)
		}
	}
}

// ExtractFromEngine pulls state from the engine
func (e *Extractor) ExtractFromEngine(engine *graph_engine.WorkflowEngine, nodeID string, predecessorNodeID string) (nodeConfig map[string]any, upstreamContexts map[string]UpstreamNodeContext) {
	nodeConfig = nil
	upstreamContexts = make(map[string]UpstreamNodeContext)

	if engine == nil {
		return
	}

	graph := engine.GetGraph()
	if graph == nil {
		logger.Error("ExtractFromEngine: graph is nil")
		return
	}
	if graph.Nodes == nil {
		logger.Error("ExtractFromEngine: graph.Nodes is nil")
		return
	}

	knownNodeIDs := make(map[string]bool)
	for id := range graph.Nodes {
		knownNodeIDs[id] = true
	}

	deps := make(map[string]bool)
	if predecessorNodeID != "" {
		deps[predecessorNodeID] = true
	}

	if nodeData, ok := graph.Nodes[nodeID]; ok {
		logger.Info("ExtractFromEngine: found nodeData for nodeID", "nodeID", nodeID)
		if m, ok := nodeData.(map[string]any); ok {
			nodeConfig = m
			logger.Info("ExtractFromEngine: successfully asserted nodeData to map[string]any", "nodeID", nodeID)
		} else {
			logger.Error("ExtractFromEngine: failed to assert nodeData to map[string]any", "nodeID", nodeID, "nodeDataType", fmt.Sprintf("%T", nodeData))
		}
		// Find dependencies
		e.findDependencies(nodeData, knownNodeIDs, deps)
	} else {
		logger.Error("ExtractFromEngine: nodeID not found in graph.Nodes", "nodeID", nodeID, "graphNodesCount", len(graph.Nodes))
	}

	results := engine.GetNodeResults()

	for depID := range deps {
		if depID == nodeID {
			continue // skip self
		}
		ctx := UpstreamNodeContext{}
		if depData, ok := graph.Nodes[depID]; ok {
			if m, ok := depData.(map[string]any); ok {
				ctx.Config = m
			}
		}
		if state, ok := results[depID]; ok && state != nil && state.Outputs != nil {
			ctx.Output = state.Outputs
		}
		upstreamContexts[depID] = ctx
	}

	return nodeConfig, upstreamContexts
}

// ExtractFromMaps pulls state from handler-level maps
func (e *Extractor) ExtractFromMaps(nodeID string, predecessorNodeID string, nodeMap map[string]map[string]interface{}, executionOutputs map[string]map[string]interface{}) (nodeConfig map[string]any, upstreamContexts map[string]UpstreamNodeContext) {
	nodeConfig = nil
	upstreamContexts = make(map[string]UpstreamNodeContext)

	if nodeMap == nil {
		return
	}

	knownNodeIDs := make(map[string]bool)
	for id := range nodeMap {
		knownNodeIDs[id] = true
	}

	deps := make(map[string]bool)
	if predecessorNodeID != "" {
		deps[predecessorNodeID] = true
	}

	if nodeData, ok := nodeMap[nodeID]; ok {
		nodeConfig = nodeData
		// Find dependencies
		e.findDependencies(nodeData, knownNodeIDs, deps)
	}

	for depID := range deps {
		if depID == nodeID {
			continue // skip self
		}
		ctx := UpstreamNodeContext{}
		if depData, ok := nodeMap[depID]; ok {
			ctx.Config = depData
		}
		if predOutputs, ok := executionOutputs[depID]; ok {
			ctx.Output = predOutputs
		}
		upstreamContexts[depID] = ctx
	}

	return nodeConfig, upstreamContexts
}
