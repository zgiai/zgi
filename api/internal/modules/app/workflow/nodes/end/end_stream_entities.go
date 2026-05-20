package end

import (
	"github.com/zgiai/ginext/internal/modules/app/workflow/graph_engine/entities"
	"github.com/zgiai/ginext/internal/modules/app/workflow/shared"
)

// EndStreamParam represents end stream parameters
type EndStreamParam struct {
	// End stream variable selector mapping: end_node_id -> [][]string
	EndStreamVariableSelectorMapping map[string][][]string `json:"end_stream_variable_selector_mapping"`
	// End dependencies: end_node_id -> []string
	EndDependencies map[string][]string `json:"end_dependencies"`
}

// GraphEngineEvent is the interface for graph engine events
type GraphEngineEvent interface {
	GetEventType() string
}

// BaseNodeEvent represents a base node event
type BaseNodeEvent struct {
	ID                        string                   `json:"id"`
	NodeID                    string                   `json:"node_id"`
	NodeType                  shared.NodeType          `json:"node_type"`
	NodeData                  interface{}              `json:"node_data"`
	RouteNodeState            *entities.RouteNodeState `json:"route_node_state"`
	ParallelID                *string                  `json:"parallel_id,omitempty"`
	ParallelStartNodeID       *string                  `json:"parallel_start_node_id,omitempty"`
	ParentParallelID          *string                  `json:"parent_parallel_id,omitempty"`
	ParentParallelStartNodeID *string                  `json:"parent_parallel_start_node_id,omitempty"`
	InIterationID             *string                  `json:"in_iteration_id,omitempty"`
	InLoopID                  *string                  `json:"in_loop_id,omitempty"`
	NodeVersion               string                   `json:"node_version"`
}

func (e *BaseNodeEvent) GetEventType() string {
	return "base_node_event"
}

// NodeRunStartedEvent represents a node run started event
type NodeRunStartedEvent struct {
	BaseNodeEvent
	PredecessorNodeID *string     `json:"predecessor_node_id,omitempty"`
	ParallelModeRunID *string     `json:"parallel_mode_run_id,omitempty"`
	AgentStrategy     interface{} `json:"agent_strategy,omitempty"`
}

func (e *NodeRunStartedEvent) GetEventType() string {
	return "node_run_started"
}

// NodeRunStreamChunkEvent represents a node run stream chunk event
type NodeRunStreamChunkEvent struct {
	BaseNodeEvent
	ChunkContent         string   `json:"chunk_content"`
	FromVariableSelector []string `json:"from_variable_selector,omitempty"`
}

func (e *NodeRunStreamChunkEvent) GetEventType() string {
	return "node_run_stream_chunk"
}

// NodeRunSucceededEvent represents a node run succeeded event
type NodeRunSucceededEvent struct {
	BaseNodeEvent
}

func (e *NodeRunSucceededEvent) GetEventType() string {
	return "node_run_succeeded"
}

// VariableMarkdown is the interface for variable markdown methods
type VariableMarkdown interface {
	GetMarkdown() string
}
