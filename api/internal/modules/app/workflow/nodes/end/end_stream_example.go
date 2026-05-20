package end

import (
	"fmt"

	"github.com/zgiai/ginext/internal/modules/app/workflow/graph_engine/entities"
)

// EndStreamExample demonstrates the usage of end stream
type EndStreamExample struct{}

// ExampleUsage demonstrates how to use the end stream system
func (e *EndStreamExample) ExampleUsage() {
	// 1. Prepare test data
	nodeIDConfigMapping := map[string]map[string]interface{}{
		"end_node_1": {
			"data": map[string]interface{}{
				"type": "end",
				"outputs": []interface{}{
					map[string]interface{}{
						"variable": "result",
						"selector": []interface{}{"llm_node_1", "text"},
					},
				},
			},
		},
	}

	reverseEdgeMapping := map[string][]*GraphEdge{
		"end_node_1": {
			{
				SourceNodeID: "llm_node_1",
				TargetNodeID: "end_node_1",
			},
		},
	}

	nodeParallelMapping := map[string]string{}

	// 2. Create factory
	factory := NewEndStreamFactory()

	endStreamParam, err := factory.InitEndStreamParam(
		nodeIDConfigMapping,
		reverseEdgeMapping,
		nodeParallelMapping,
	)
	if err != nil {
		fmt.Printf("Failed to initialize stream parameters: %v\n", err)
		return
	}

	fmt.Printf("Stream parameters initialized successfully: %+v\n", endStreamParam)

	// 4. Create graph and variable pool (using mock data here)
	graph := &entities.Graph{} // Need actual graph instance
	variablePool := entities.NewVariablePool()

	processor, err := factory.CreateCompleteEndStreamSystem(
		graph,
		variablePool,
		nodeIDConfigMapping,
		reverseEdgeMapping,
		nodeParallelMapping,
	)
	if err != nil {
		fmt.Printf("Failed to create stream system: %v\n", err)
		return
	}

	fmt.Printf("Stream processor created successfully: %+v\n", processor)

	// 6. Use stream processor
	inputChan := make(chan GraphEngineEvent, 10)
	outputChan := processor.Process(inputChan)

	// Simulate sending events
	go func() {
		defer close(inputChan)

		// Send a stream chunk event
		streamEvent := &NodeRunStreamChunkEvent{
			BaseNodeEvent: BaseNodeEvent{
				ID:             "test_id",
				NodeID:         "llm_node_1",
				NodeType:       "llm",
				RouteNodeState: &entities.RouteNodeState{NodeID: "llm_node_1"},
			},
			ChunkContent:         "Hello, World!",
			FromVariableSelector: []string{"llm_node_1", "text"},
		}
		inputChan <- streamEvent
	}()

	for event := range outputChan {
		fmt.Printf("Received event: %s\n", event.GetEventType())
	}
}

// ExampleRouterOnly demonstrates router usage only
func (e *EndStreamExample) ExampleRouterOnly() {
	// Create router
	router := &EndStreamGeneratorRouter{}

	// Prepare node configuration
	nodeIDConfigMapping := map[string]map[string]interface{}{
		"end_node_1": {
			"data": map[string]interface{}{
				"type": "end",
				"outputs": []interface{}{
					map[string]interface{}{
						"variable": "final_result",
						"selector": []interface{}{"llm_node_1", "text"},
					},
				},
			},
		},
	}

	reverseEdgeMapping := map[string][]*GraphEdge{}
	nodeParallelMapping := map[string]string{}

	param, err := router.Init(nodeIDConfigMapping, reverseEdgeMapping, nodeParallelMapping)
	if err != nil {
		fmt.Printf("Failed to initialize router: %v\n", err)
		return
	}

	fmt.Printf("Router initialized successfully: %+v\n", param)
}
