package graph_engine

import (
	"context"

	"github.com/zgiai/zgi/api/internal/modules/app/workflow/graph_engine/entities"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/shared"
)

// NodeRunRequest is the graph engine's node execution contract.
type NodeRunRequest struct {
	NodeID          string
	NodeType        shared.NodeType
	Config          map[string]any
	GraphInitParams entities.GraphInitParams
	Graph           *entities.Graph
	RuntimeState    *entities.GraphRuntimeState
}

// NodeRunner executes one workflow node for the graph engine.
type NodeRunner interface {
	RunNode(ctx context.Context, req NodeRunRequest, eventChan chan<- *shared.NodeEventCh) (*shared.NodeRunResult, error)
}
