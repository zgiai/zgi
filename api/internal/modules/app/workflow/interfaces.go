package workflow

import (
	"context"

	"github.com/zgiai/zgi/api/internal/modules/app/workflow/nodes/base"
)

type WorkflowNode interface {
	GetID() string
	GetNodeID() string
	GetType() string
	GetNodeData() base.NodeData

	Run(ctx context.Context) (<-chan base.NodeEvent, error)

	ExtractVariableSelectorToVariableMapping(graphConfig map[string]interface{}, config map[string]interface{}) (map[string][]string, error)

	ShouldContinueOnError() bool
	ShouldRetry() bool

	GetDefaultConfig(filters map[string]interface{}) map[string]interface{}
}

type WorkflowEngine interface {
	Run(ctx context.Context, inputs map[string]interface{}) error
	Stop() error
	GetStatus() string
}

type VariablePool interface {
	Add(selector []string, value interface{}) error
	Get(selector []string) interface{}
	SetVariable(name string, value interface{})
	GetVariable(name string) interface{}
}

type Graph interface {
	GetRootNodeID() string
	GetNodeIDs() []string
	GetNodeConfig(nodeID string) map[string]interface{}
	GetEdges() map[string][]interface{}
}
