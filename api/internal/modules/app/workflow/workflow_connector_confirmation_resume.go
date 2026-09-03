package workflow

import (
	"fmt"

	graphentities "github.com/zgiai/zgi/api/internal/modules/app/workflow/graph_engine/entities"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/streamscheduler"
)

// restoreWorkflowResolvedPausedNodes restores only safe, recorder-projected
// outputs for paused nodes that have already reached a terminal success. It
// expands their downstream edges before the main loop skips the completed
// nodes, so Connector confirmation never invokes the Connector a second time.
func restoreWorkflowResolvedPausedNodes(
	pausedNodeIDs []string,
	completedNodes map[string]bool,
	executionOutputs map[string]map[string]interface{},
	variablePool *graphentities.VariablePool,
	nodeQueue *[]string,
	edgeMap map[string]map[string][]string,
	nodePredecessors map[string]*string,
) error {
	for _, nodeID := range pausedNodeIDs {
		if !completedNodes[nodeID] {
			continue
		}
		outputs, exists := executionOutputs[nodeID]
		if !exists || outputs == nil {
			return fmt.Errorf("workflow resolved pause node %s has no execution outputs", nodeID)
		}
		clearResumedNodeVariables(variablePool, nodeID)
		addWorkflowStreamOutputsToVariablePool(variablePool, nodeID, outputs)
		streamscheduler.EnqueueDownstreams(nodeQueue, edgeMap, nodeID, completedNodes, nodePredecessors)
	}
	return nil
}
