package streamscheduler

// DownstreamNodeIDs returns all downstream node IDs of currentNodeID across all source handles.
// Order is not guaranteed and duplicates are preserved (to match the underlying edge config).
func DownstreamNodeIDs(edgeMap map[string]map[string][]string, currentNodeID string) []string {
	handleMap, exists := edgeMap[currentNodeID]
	if !exists {
		return nil
	}

	var nextNodeIDs []string
	for _, targets := range handleMap {
		nextNodeIDs = append(nextNodeIDs, targets...)
	}
	return nextNodeIDs
}

// EnqueueDownstreams appends all downstream nodes of currentNodeID (across all source handles)
// into nodeQueue. This is intentionally handle-agnostic: downstream nodes are later evaluated
// for activation/skipping based on upstream branch selection.
//
// This function is used by streaming workflow execution to ensure "skipped" nodes still
// propagate to their downstream nodes; otherwise, deeper nodes in an inactive branch are never
// visited and can deadlock merge nodes that require all upstreams to be completed.
func EnqueueDownstreams(
	nodeQueue *[]string,
	edgeMap map[string]map[string][]string,
	currentNodeID string,
	completedNodes map[string]bool,
	nodePredecessors map[string]*string,
) []string {
	if nodeQueue == nil {
		return nil
	}

	nextNodeIDs := DownstreamNodeIDs(edgeMap, currentNodeID)

	for _, nextNodeID := range nextNodeIDs {
		if nextNodeID == "" {
			continue
		}
		if completedNodes[nextNodeID] {
			continue
		}

		*nodeQueue = append(*nodeQueue, nextNodeID)

		// Track predecessor (use the first predecessor if multiple exist).
		if nodePredecessors != nil && nodePredecessors[nextNodeID] == nil {
			pred := currentNodeID
			nodePredecessors[nextNodeID] = &pred
		}
	}

	return nextNodeIDs
}
