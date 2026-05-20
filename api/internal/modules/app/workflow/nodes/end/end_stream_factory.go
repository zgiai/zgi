package end

import (
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/graph_engine/entities"
)

// EndStreamFactory is the factory for end stream
type EndStreamFactory struct{}

// CreateEndStreamGeneratorRouter creates an end stream generator router
func (f *EndStreamFactory) CreateEndStreamGeneratorRouter() *EndStreamGeneratorRouter {
	return &EndStreamGeneratorRouter{}
}

// CreateEndStreamProcessor creates an end stream processor
func (f *EndStreamFactory) CreateEndStreamProcessor(
	graph *entities.Graph,
	variablePool *entities.VariablePool,
	endStreamParam *EndStreamParam,
) *EndStreamProcessor {
	return NewEndStreamProcessor(graph, variablePool, endStreamParam)
}

// InitEndStreamParam initializes end stream parameters
func (f *EndStreamFactory) InitEndStreamParam(
	nodeIDConfigMapping map[string]map[string]interface{},
	reverseEdgeMapping map[string][]*GraphEdge,
	nodeParallelMapping map[string]string,
) (*EndStreamParam, error) {
	router := f.CreateEndStreamGeneratorRouter()
	return router.Init(nodeIDConfigMapping, reverseEdgeMapping, nodeParallelMapping)
}

// CreateCompleteEndStreamSystem creates a complete end stream system
func (f *EndStreamFactory) CreateCompleteEndStreamSystem(
	graph *entities.Graph,
	variablePool *entities.VariablePool,
	nodeIDConfigMapping map[string]map[string]interface{},
	reverseEdgeMapping map[string][]*GraphEdge,
	nodeParallelMapping map[string]string,
) (*EndStreamProcessor, error) {
	// 1. Initialize stream parameters
	endStreamParam, err := f.InitEndStreamParam(nodeIDConfigMapping, reverseEdgeMapping, nodeParallelMapping)
	if err != nil {
		return nil, err
	}

	// 2. Create stream processor
	processor := f.CreateEndStreamProcessor(graph, variablePool, endStreamParam)

	return processor, nil
}

// NewEndStreamFactory creates a new end stream factory
func NewEndStreamFactory() *EndStreamFactory {
	return &EndStreamFactory{}
}
