package workflow

import "fmt"

const workflowGraphStartNodeType = "start"

type workflowGraphReachabilityViews struct {
	ExecutionGraphData map[string]any
	RuntimeGraphData   map[string]any
	StartNodeID        string
}

func filterWorkflowGraphReachableFromStart(graphData map[string]any) (map[string]any, string, error) {
	views, err := buildWorkflowGraphReachabilityViews(graphData)
	if err != nil {
		return nil, "", err
	}
	return views.RuntimeGraphData, views.StartNodeID, nil
}

func buildWorkflowGraphReachabilityViews(graphData map[string]any) (workflowGraphReachabilityViews, error) {
	if graphData == nil {
		return workflowGraphReachabilityViews{}, fmt.Errorf("workflow graph is nil")
	}

	nodesData, err := workflowGraphNodes(graphData)
	if err != nil {
		return workflowGraphReachabilityViews{}, err
	}

	edgesData, err := workflowGraphEdges(graphData)
	if err != nil {
		return workflowGraphReachabilityViews{}, err
	}

	nodeMap, startNodeID, err := workflowGraphNodeMapAndStart(nodesData)
	if err != nil {
		return workflowGraphReachabilityViews{}, err
	}

	executionReachable := workflowReachableNodeIDs(startNodeID, edgesData, nodeMap, workflowTopLevelNodeIDs(nodeMap))
	runtimeReachable := copyWorkflowBoolMap(executionReachable)
	expandWorkflowContainerReachability(runtimeReachable, edgesData, nodeMap)

	executionGraphData := copyWorkflowAnyMap(graphData)
	executionGraphData["nodes"] = filterWorkflowReachableNodes(nodesData, executionReachable)
	executionGraphData["edges"] = filterWorkflowReachableEdges(edgesData, executionReachable, nodeMap)

	runtimeGraphData := copyWorkflowAnyMap(graphData)
	runtimeGraphData["nodes"] = filterWorkflowReachableNodes(nodesData, runtimeReachable)
	runtimeGraphData["edges"] = filterWorkflowReachableEdges(edgesData, runtimeReachable, nodeMap)

	return workflowGraphReachabilityViews{
		ExecutionGraphData: executionGraphData,
		RuntimeGraphData:   runtimeGraphData,
		StartNodeID:        startNodeID,
	}, nil
}

func workflowGraphNodes(graphData map[string]any) ([]any, error) {
	nodesData, ok := graphData["nodes"].([]any)
	if !ok {
		return nil, fmt.Errorf("invalid nodes data format")
	}
	return nodesData, nil
}

func workflowGraphEdges(graphData map[string]any) ([]any, error) {
	edgesInterface, exists := graphData["edges"]
	if !exists || edgesInterface == nil {
		return nil, nil
	}

	edgesData, ok := edgesInterface.([]any)
	if !ok {
		return nil, fmt.Errorf("invalid edges data format")
	}
	return edgesData, nil
}

func workflowGraphNodeMapAndStart(nodesData []any) (map[string]map[string]any, string, error) {
	nodeMap := make(map[string]map[string]any, len(nodesData))
	startNodeID := ""
	for _, nodeInterface := range nodesData {
		node, ok := nodeInterface.(map[string]any)
		if !ok {
			continue
		}
		nodeID, _ := node["id"].(string)
		if nodeID == "" {
			continue
		}

		nodeMap[nodeID] = node
		if workflowGraphNodeType(node) == workflowGraphStartNodeType && workflowGraphNodeContainerID(node) == "" {
			if startNodeID != "" && startNodeID != nodeID {
				return nil, "", fmt.Errorf("multiple start nodes found in workflow")
			}
			startNodeID = nodeID
		}
	}

	if startNodeID == "" {
		return nil, "", fmt.Errorf("no start node found in workflow")
	}
	return nodeMap, startNodeID, nil
}

func filterWorkflowReachableNodes(nodesData []any, reachable map[string]bool) []any {
	filteredNodes := make([]any, 0, len(nodesData))
	for _, nodeInterface := range nodesData {
		node, ok := nodeInterface.(map[string]any)
		if !ok {
			continue
		}
		nodeID, _ := node["id"].(string)
		if reachable[nodeID] {
			filteredNodes = append(filteredNodes, node)
		}
	}
	return filteredNodes
}

func filterWorkflowReachableEdges(edgesData []any, reachable map[string]bool, nodeMap map[string]map[string]any) []any {
	filteredEdges := make([]any, 0, len(edgesData))
	for _, edgeInterface := range edgesData {
		edge, ok := edgeInterface.(map[string]any)
		if !ok {
			continue
		}

		source, _ := edge["source"].(string)
		target, _ := edge["target"].(string)
		if !reachable[source] || !reachable[target] {
			continue
		}
		if _, exists := nodeMap[source]; !exists {
			continue
		}
		if _, exists := nodeMap[target]; !exists {
			continue
		}
		filteredEdges = append(filteredEdges, edge)
	}
	return filteredEdges
}

func workflowReachableNodeIDs(startNodeID string, edgesData []any, nodeMap map[string]map[string]any, candidates map[string]bool) map[string]bool {
	adjacency := make(map[string][]string)
	for _, edgeInterface := range edgesData {
		edge, ok := edgeInterface.(map[string]any)
		if !ok {
			continue
		}
		source, _ := edge["source"].(string)
		target, _ := edge["target"].(string)
		if source == "" || target == "" {
			continue
		}
		if _, exists := nodeMap[source]; !exists {
			continue
		}
		if _, exists := nodeMap[target]; !exists {
			continue
		}
		if len(candidates) > 0 && (!candidates[source] || !candidates[target]) {
			continue
		}
		adjacency[source] = append(adjacency[source], target)
	}

	reachable := map[string]bool{startNodeID: true}
	queue := []string{startNodeID}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		for _, next := range adjacency[current] {
			if reachable[next] {
				continue
			}
			reachable[next] = true
			queue = append(queue, next)
		}
	}

	return reachable
}

func workflowTopLevelNodeIDs(nodeMap map[string]map[string]any) map[string]bool {
	topLevel := make(map[string]bool, len(nodeMap))
	for nodeID, node := range nodeMap {
		if workflowGraphParentID(node) == "" && workflowGraphLegacyContainerID(node) == "" {
			topLevel[nodeID] = true
		}
	}
	return topLevel
}

func expandWorkflowContainerReachability(reachable map[string]bool, edgesData []any, nodeMap map[string]map[string]any) {
	processed := make(map[string]bool)
	for {
		changed := false
		for nodeID, isReachable := range reachable {
			if !isReachable || processed[nodeID] {
				continue
			}
			node := nodeMap[nodeID]
			if !workflowGraphNodeIsContainer(node) {
				processed[nodeID] = true
				continue
			}
			processed[nodeID] = true
			childReachable := workflowReachableContainerNodeIDs(nodeID, node, edgesData, nodeMap)
			for childID := range childReachable {
				if !reachable[childID] {
					reachable[childID] = true
					changed = true
				}
			}
		}
		if !changed {
			return
		}
	}
}

func workflowReachableContainerNodeIDs(containerID string, containerNode map[string]any, edgesData []any, nodeMap map[string]map[string]any) map[string]bool {
	startNodeID := workflowGraphContainerStartNodeID(containerNode)
	if startNodeID == "" {
		return nil
	}

	candidates := make(map[string]bool)
	for nodeID, node := range nodeMap {
		if nodeID == startNodeID || workflowGraphNodeContainerID(node) == containerID {
			candidates[nodeID] = true
		}
	}
	if !candidates[startNodeID] {
		return nil
	}

	return workflowReachableNodeIDs(startNodeID, edgesData, nodeMap, candidates)
}

func workflowGraphNodeIsContainer(node map[string]any) bool {
	switch workflowGraphNodeType(node) {
	case "iteration", "loop":
		return true
	default:
		return false
	}
}

func workflowGraphContainerStartNodeID(node map[string]any) string {
	data, _ := node["data"].(map[string]any)
	if data == nil {
		return ""
	}
	startNodeID, _ := data["start_node_id"].(string)
	return startNodeID
}

func workflowGraphNodeContainerID(node map[string]any) string {
	if parentID := workflowGraphParentID(node); parentID != "" {
		return parentID
	}
	return workflowGraphLegacyContainerID(node)
}

func workflowGraphParentID(node map[string]any) string {
	parentID, _ := node["parentId"].(string)
	return parentID
}

func workflowGraphLegacyContainerID(node map[string]any) string {
	data, _ := node["data"].(map[string]any)
	if data == nil {
		return ""
	}
	if iterationID, _ := data["iteration_id"].(string); iterationID != "" {
		return iterationID
	}
	loopID, _ := data["loop_id"].(string)
	return loopID
}

func workflowGraphNodeType(node map[string]any) string {
	if data, ok := node["data"].(map[string]any); ok {
		if nodeType, ok := data["type"].(string); ok {
			return nodeType
		}
	}
	nodeType, _ := node["type"].(string)
	return nodeType
}
