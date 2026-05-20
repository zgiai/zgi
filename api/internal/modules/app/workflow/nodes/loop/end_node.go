package loop

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/zgiai/ginext/internal/modules/app/workflow/graph_engine/entities"
	"github.com/zgiai/ginext/internal/modules/app/workflow/nodes/base"
	"github.com/zgiai/ginext/internal/modules/app/workflow/shared"
)

// EndNode represents the loop-end node that marks the end of a loop subgraph.
type EndNode struct {
	base.NodeStruct
	nodeData EndNodeData
}

// NewEndNode creates a loop-end node instance from workflow config.
func NewEndNode(
	id string,
	config map[string]any,
	graphInitParams entities.GraphInitParams,
	graph *entities.Graph,
	graphRuntimeState *entities.GraphRuntimeState,
	previousNodeID *string,
	optionalDeps ...interface{},
) (shared.NodeInterface, error) {
	nodeData, nodeID, err := parseEndNodeDataFromConfig(config)
	if err != nil {
		return nil, err
	}

	return &EndNode{
		NodeStruct: base.NodeStruct{
			InstanceID: id,
			NodeID:     nodeID,
			NodeType:   shared.LoopEnd,

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

func parseEndNodeDataFromConfig(config map[string]any) (EndNodeData, string, error) {
	rawNodeID, ok := config["id"]
	if !ok {
		return EndNodeData{}, "", fmt.Errorf("node ID is required")
	}

	nodeID, ok := rawNodeID.(string)
	if !ok {
		return EndNodeData{}, "", fmt.Errorf("node ID must be string")
	}

	rawData, ok := config["data"]
	if !ok {
		return EndNodeData{}, "", fmt.Errorf("node data is required")
	}

	payload, err := json.Marshal(rawData)
	if err != nil {
		return EndNodeData{}, "", fmt.Errorf("marshal node data: %w", err)
	}

	var nodeData EndNodeData
	if err := json.Unmarshal(payload, &nodeData); err != nil {
		return EndNodeData{}, "", fmt.Errorf("unmarshal node data: %w", err)
	}

	return nodeData, nodeID, nil
}

// Run executes the loop-end node.
func (n *EndNode) Run(ctx context.Context, eventChan chan *shared.NodeEventCh) error {
	select {
	case eventChan <- &shared.NodeEventCh{
		Type:      shared.EventTypeRunStarted,
		NodeID:    n.NodeID,
		Timestamp: time.Now(),
	}:
	case <-ctx.Done():
		return ctx.Err()
	}

	result := &shared.NodeRunResult{
		Status:  shared.SUCCEEDED,
		Inputs:  map[string]any{},
		Outputs: map[string]any{},
	}

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
