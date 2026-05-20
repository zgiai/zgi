package workflow_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/zgiai/ginext/internal/modules/app/workflow/graph_engine"
	"github.com/zgiai/ginext/internal/modules/app/workflow/graph_engine/entities"
	"github.com/zgiai/ginext/internal/modules/app/workflow/graph_engine/loop_subgraph"
	"github.com/zgiai/ginext/internal/modules/app/workflow/graph_engine/subgraph"
	"github.com/zgiai/ginext/internal/modules/app/workflow/shared"
)

type recordingEngine struct {
	addedNodes []string
}

func (e *recordingEngine) AddNode(nodeID string, nodeType shared.NodeType, config map[string]any) {
	e.addedNodes = append(e.addedNodes, nodeID)
}

func (e *recordingEngine) AddDependency(from, to string) error { return nil }

func (e *recordingEngine) AddDependencyWithHandle(from, to, sourceHandle string) error { return nil }

func (e *recordingEngine) SetRuntimeState(state *entities.GraphRuntimeState, graph *entities.Graph) {}

func (e *recordingEngine) SetStreamEventCallback(func(nodeID string, event *shared.RunStreamChunkEvent)) {
}

func (e *recordingEngine) SetLLMClient(llmClient interface{}) {}

func (e *recordingEngine) SetContentExtractor(contentExtractor interface{}) {}

func (e *recordingEngine) SetToolEngine(toolEngine interface{}) {}

func (e *recordingEngine) Execute(ctx context.Context) error { return nil }

func (e *recordingEngine) SetNodeEventCallbacks(
	onStarted func(nodeID string, nodeType string, inputs map[string]any),
	onFinished func(nodeID string, nodeType string, status string, outputs map[string]any, edgeSourceHandle string, err error),
) {
}

func (e *recordingEngine) SetReadyBatchCallback(scope graph_engine.ReadyBatchScope, onReadyBatch func(scope graph_engine.ReadyBatchScope, nodeIDs []string)) {
}

func TestSubgraphRunSkipsUnreachableNodes(t *testing.T) {
	startNodeID := "iter-start"
	answerNodeID := "answer"
	orphanNodeID := "orphan"

	graphConfig := newGraphConfig(
		[]map[string]any{
			{
				"id": startNodeID,
				"data": map[string]any{
					"type": "start",
				},
			},
			{
				"id":       answerNodeID,
				"parentId": "iter-1",
				"data": map[string]any{
					"type": "answer",
				},
			},
			{
				"id":       orphanNodeID,
				"parentId": "iter-1",
				"data": map[string]any{
					"type": "answer",
				},
			},
		},
		[]map[string]any{
			{"source": startNodeID, "target": answerNodeID},
		},
	)

	engine := &recordingEngine{}
	executor := subgraph.New(subgraph.Config{
		NodeID:       "iter-1",
		StartNodeID:  &startNodeID,
		GraphConfig:  graphConfig,
		RuntimeState: entities.NewGraphRuntimeState(entities.NewVariablePool()),
		EngineFactory: func(parallelism int) subgraph.Engine {
			return engine
		},
	})

	_, err := executor.Run(context.Background(), 0, "item")
	require.NoError(t, err)
	require.ElementsMatch(t, []string{startNodeID, answerNodeID}, engine.addedNodes)
}

func TestSubgraphRunDefaultsParallelism(t *testing.T) {
	startNodeID := "iter-start"
	answerNodeID := "answer"

	graphConfig := newGraphConfig(
		[]map[string]any{
			{
				"id": startNodeID,
				"data": map[string]any{
					"type": "start",
				},
			},
			{
				"id":       answerNodeID,
				"parentId": "iter-1",
				"data": map[string]any{
					"type": "answer",
				},
			},
		},
		[]map[string]any{
			{"source": startNodeID, "target": answerNodeID},
		},
	)

	capturedParallelism := -1
	executor := subgraph.New(subgraph.Config{
		NodeID:       "iter-1",
		StartNodeID:  &startNodeID,
		GraphConfig:  graphConfig,
		RuntimeState: entities.NewGraphRuntimeState(entities.NewVariablePool()),
		EngineFactory: func(parallelism int) subgraph.Engine {
			capturedParallelism = parallelism
			return &recordingEngine{}
		},
	})

	_, err := executor.Run(context.Background(), 0, "item")
	require.NoError(t, err)
	require.Equal(t, graph_engine.DefaultMaxConcurrency, capturedParallelism)
}

func TestLoopSubgraphRunDoesNotMutateOriginalGraphConfig(t *testing.T) {
	startNodeID := "loop-start"
	answerNodeID := "answer"
	endNodeID := "loop-end"

	graphConfig := newGraphConfig(
		[]map[string]any{
			{
				"id": startNodeID,
				"data": map[string]any{
					"type":    "loop-start",
					"loop_id": "loop-1",
				},
			},
			{
				"id": answerNodeID,
				"data": map[string]any{
					"type":    "answer",
					"loop_id": "loop-1",
				},
			},
			{
				"id": endNodeID,
				"data": map[string]any{
					"type":    "loop-end",
					"loop_id": "loop-1",
				},
			},
		},
		[]map[string]any{
			{"source": startNodeID, "target": answerNodeID},
			{"source": answerNodeID, "target": endNodeID},
		},
	)

	original := cloneGraphConfigForTest(t, graphConfig)

	executor := loop_subgraph.New(loop_subgraph.Config{
		NodeID:       "loop-1",
		StartNodeID:  &startNodeID,
		GraphConfig:  graphConfig,
		RuntimeState: entities.NewGraphRuntimeState(entities.NewVariablePool()),
		EngineFactory: func(parallelism int) subgraph.Engine {
			return &recordingEngine{}
		},
	})

	_, err := executor.Run(context.Background(), 0)
	require.NoError(t, err)
	require.Equal(t, original, graphConfig)
}

func cloneGraphConfigForTest(t *testing.T, graphConfig map[string]any) map[string]any {
	t.Helper()

	data, err := json.Marshal(graphConfig)
	require.NoError(t, err)

	var cloned map[string]any
	require.NoError(t, json.Unmarshal(data, &cloned))
	return cloned
}
