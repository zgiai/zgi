package iter

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/zgiai/zgi/api/internal/modules/app/workflow/graph_engine/entities"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/nodes/base"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/shared"
)

// StartNode represents the iteration-start node that marks the beginning of an iteration subgraph.
type StartNode struct {
	base.NodeStruct
	nodeData StartNodeData
}

// NewStartNode creates an iteration-start node instance from workflow config.
func NewStartNode(
	id string,
	config map[string]any,
	graphInitParams entities.GraphInitParams,
	graph *entities.Graph,
	graphRuntimeState *entities.GraphRuntimeState,
	previousNodeID *string,
	optionalDeps ...interface{},
) (shared.NodeInterface, error) {
	nodeData, nodeID, err := parseStartNodeDataFromConfig(config)
	if err != nil {
		return nil, err
	}

	return &StartNode{
		NodeStruct: base.NodeStruct{
			InstanceID: id,
			NodeID:     nodeID,
			NodeType:   shared.IterationStart,

			TenantID:          graphInitParams.TenantID,
			APPID:             graphInitParams.AppID,
			WorkflowType:      string(graphInitParams.WorkflowType),
			WorkflowID:        graphInitParams.WorkflowID,
			UserFrom:          string(graphInitParams.UserFrom),
			UserID:            graphInitParams.UserID,
			GraphConfig:       graphInitParams.GraphConfig,
			InvokeFrom:        string(graphInitParams.InvokeFrom),
			WorkflowCallDepth: graphInitParams.CallDepth,

			Graph:             graph,
			GraphRuntimeState: graphRuntimeState,
			PreviousNodeID:    previousNodeID,
		},
		nodeData: nodeData,
	}, nil
}

func parseStartNodeDataFromConfig(config map[string]any) (StartNodeData, string, error) {
	rawNodeID, ok := config["id"]
	if !ok {
		return StartNodeData{}, "", fmt.Errorf("node ID is required")
	}

	nodeID, ok := rawNodeID.(string)
	if !ok {
		return StartNodeData{}, "", fmt.Errorf("node ID must be string")
	}

	rawData, ok := config["data"]
	if !ok {
		return StartNodeData{}, "", fmt.Errorf("node data is required")
	}

	payload, err := json.Marshal(rawData)
	if err != nil {
		return StartNodeData{}, "", fmt.Errorf("failed to marshal node data: %w", err)
	}

	var nodeData StartNodeData
	if err := json.Unmarshal(payload, &nodeData); err != nil {
		return StartNodeData{}, "", fmt.Errorf("failed to unmarshal node data: %w", err)
	}

	return nodeData, nodeID, nil
}

// Run executes the iteration-start node.
// This node simply passes through the iteration context (index, item) that was
// already set by the parent iteration node in the variable pool.
func (n *StartNode) Run(ctx context.Context, eventChan chan *shared.NodeEventCh) error {
	// Send start event
	select {
	case eventChan <- &shared.NodeEventCh{
		Type:      shared.EventTypeRunStarted,
		NodeID:    n.NodeID,
		Timestamp: time.Now(),
	}:
	case <-ctx.Done():
		return ctx.Err()
	}

	// The iteration-start node is a pass-through node.
	// The parent iteration node has already set the index and item in the variable pool.
	// We just need to signal completion.

	result := &shared.NodeRunResult{
		Status:  shared.SUCCEEDED,
		Inputs:  map[string]any{},
		Outputs: map[string]any{},
	}

	// Send completion event
	select {
	case eventChan <- &shared.NodeEventCh{
		Type:      shared.EventTypeRunCompleted,
		NodeID:    n.NodeID,
		Data:      &shared.RunCompletedEvent{RunResult: result},
		Timestamp: time.Now(),
	}:
	case <-ctx.Done():
		return ctx.Err()
	}

	return nil
}
