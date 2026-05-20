package base

import (
	"context"
	"time"

	"github.com/zgiai/ginext/internal/modules/app/workflow/graph_engine/entities"
	"github.com/zgiai/ginext/internal/modules/app/workflow/shared"
)

// NodeStruct base node structure - designed according to start, end, http node calling patterns
type NodeStruct struct {
	InstanceID string // Node runtime instance ID (different for each run)
	NodeID     string // Node ID (node ID in workflow flowchart)

	//NodeData
	NodeType shared.NodeType

	TenantID          string         // Workspace ID
	WorkspaceID       string         // Canonical workspace ID
	OrganizationID    string         // Canonical organization ID
	APPID             string         // Application ID
	WorkflowType      string         // Workflow type: chat or completion
	WorkflowID        string         // Workflow ID
	GraphConfig       map[string]any // Graph configuration
	UserID            string         // User ID
	UserFrom          string         // User source
	InvokeFrom        string         // Invoke source
	WorkflowCallDepth int            // Workflow call depth

	Graph             *entities.Graph             // Graph parameters
	GraphRuntimeState *entities.GraphRuntimeState // Graph runtime state

	PreviousNodeID *string // Previous node ID
}

// NodeImpl base node implementation - for other nodes
type NodeImpl struct {
	*NodeStruct
}

// NodeInterface node interface - for other nodes
type NodeInterface interface {
	GetID() string
	GetNodeID() string
	GetType() string
	GetNodeData() NodeData
	Run(ctx context.Context) (<-chan *NodeEvent, error)
	ExtractVariableSelectorToVariableMapping(graphConfig map[string]interface{}, config map[string]interface{}) (map[string][]string, error)
	ShouldContinueOnError() bool
	ShouldRetry() bool
	GetDefaultConfig(filters map[string]interface{}) map[string]interface{}
}

// Add interface alias for backward compatibility
type Node = NodeInterface

func (n *NodeStruct) GetID() string {
	return n.InstanceID
}

func (n *NodeStruct) GetNodeID() string {
	return n.NodeID
}

func (n *NodeStruct) GetType() string {
	return string(n.NodeType)
}

func (n *NodeStruct) GetNodeData() NodeData {
	return NodeData{}
}

func (n *NodeStruct) Run(ctx context.Context) (<-chan *NodeEvent, error) {
	eventChan := make(chan *NodeEvent, 1)
	event := NewNodeEvent("executed", map[string]interface{}{"status": "completed"})
	eventChan <- event
	close(eventChan)
	return eventChan, nil
}

func (n *NodeStruct) ExtractVariableSelectorToVariableMapping(graphConfig map[string]interface{}, config map[string]interface{}) (map[string][]string, error) {
	return make(map[string][]string), nil
}

func (n *NodeStruct) ShouldContinueOnError() bool {
	return false
}

func (n *NodeStruct) ShouldRetry() bool {
	return false
}

func (n *NodeStruct) GetDefaultConfig(filters map[string]interface{}) map[string]interface{} {
	return make(map[string]interface{})
}

// NewBaseNodeImpl creates base node implementation - backward compatible version
func NewBaseNodeImpl(
	id string,
	config map[string]any,
	graphInitParams entities.GraphInitParams,
	graph *entities.Graph,
	graphRuntimeState *entities.GraphRuntimeState,
) *NodeImpl {

	_, nodeID, err := getData(config)
	if err != nil {
		// If error occurs, return a default NodeImpl
		return &NodeImpl{
			NodeStruct: &NodeStruct{
				InstanceID: id,
				NodeID:     id,
				NodeType:   shared.Code,
			},
		}
	}

	return &NodeImpl{
		NodeStruct: &NodeStruct{
			InstanceID: id,
			NodeID:     nodeID,
			NodeType:   shared.Code, // Default type

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

			PreviousNodeID: nil,
		},
	}
}

// Keep original New function for backward compatibility
func New(
	id string,
	config map[string]any,
	graphInitParams entities.GraphInitParams,
	graph *entities.Graph,
	graphRuntimeState *entities.GraphRuntimeState,
	previousNodeID *string,
	optionalDeps ...interface{},
) (shared.NodeInterface, error) {

	_, nodeID, err := getData(config)
	if err != nil {
		return nil, err
	}

	nodeImpl := &NodeImpl{
		NodeStruct: &NodeStruct{
			InstanceID: id,
			NodeID:     nodeID,
			//NodeData:   nd,

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

			PreviousNodeID: previousNodeID,
		},
	}

	// Return a wrapper that implements shared.NodeInterface
	return &nodeWrapper{nodeImpl}, nil
}

// nodeWrapper wraps NodeImpl to implement shared.NodeInterface
type nodeWrapper struct {
	*NodeImpl
}

func (n *nodeWrapper) Run(ctx context.Context, eventChan chan *shared.NodeEventCh) error {
	// Event type conversion can be done here
	eventChan <- &shared.NodeEventCh{
		Type:      shared.EventTypeRunCompleted,
		NodeID:    n.GetNodeID(),
		Data:      map[string]interface{}{"status": "completed"},
		Timestamp: time.Now(),
	}
	return nil
}

/*
node_config = {
    "id": "node_id",
    "data": {
        "type": "code",
        "title": "test",
        "variables": [...],
        "code": "...",
    }
}
*/

const (
	NodeID = "id"
)

func getData(config map[string]any) (NodeData, string, error) {
	nodeID, ok := config[NodeID]
	if !ok {
		return NodeData{}, "", NewBaseNodeError("Node ID is required")
	}
	nodeIDStr, ok := nodeID.(string)
	if !ok {
		return NodeData{}, "", NewBaseNodeError("Node ID Type is unsupported")
	}

	data, ok := config["data"]
	if !ok {
		return NodeData{}, "", NewBaseNodeError("Node data is required")
	}

	// Current data format is uncertain, will be improved when format is determined
	nd := data.(NodeData)

	return nd, nodeIDStr, nil
}
