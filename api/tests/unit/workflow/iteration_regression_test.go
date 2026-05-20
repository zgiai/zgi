package workflow_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zgiai/ginext/internal/modules/app/workflow/graph_engine"
	"github.com/zgiai/ginext/internal/modules/app/workflow/graph_engine/entities"
	"github.com/zgiai/ginext/internal/modules/app/workflow/graph_engine/subgraph"
	"github.com/zgiai/ginext/internal/modules/app/workflow/nodes/iter"
)

func TestIterationOutputUnchanged(t *testing.T) {
	iterNodeID := "iter_1"
	startNodeID := "iter_start"
	answerNodeID := "iter_answer"

	nodes := []map[string]any{
		{
			"id": iterNodeID,
			"data": map[string]any{
				"type":              "iteration",
				"start_node_id":     startNodeID,
				"iterator_selector": []string{"start", "items"},
				"output_selector":   []string{answerNodeID, "answer"},
				"is_parallel":       false,
				"parallel_nums":     1,
				"error_handle_mode": "terminated",
			},
		},
		{
			"id":       startNodeID,
			"parentId": iterNodeID,
			"data": map[string]any{
				"type": "iteration-start",
			},
		},
		{
			"id":       answerNodeID,
			"parentId": iterNodeID,
			"data": map[string]any{
				"type":   "answer",
				"answer": "{{#iter_1.item#}}",
			},
		},
	}

	edges := []map[string]any{
		{"source": startNodeID, "target": answerNodeID},
	}

	graphConfig := newGraphConfig(nodes, edges)
	initParams := newInitParams(graphConfig)
	variablePool := entities.NewVariablePool()
	variablePool.Add([]string{"start", "items"}, []any{"a", "b"})
	runtimeState := entities.NewGraphRuntimeState(variablePool)
	graph := &entities.Graph{Config: graphConfig}

	iterConfig := map[string]any{
		"id": iterNodeID,
		"data": map[string]any{
			"type":              "iteration",
			"start_node_id":     startNodeID,
			"iterator_selector": []string{"start", "items"},
			"output_selector":   []string{answerNodeID, "answer"},
			"is_parallel":       false,
			"parallel_nums":     1,
			"error_handle_mode": "terminated",
		},
	}

	node, err := iter.New(iterNodeID, iterConfig, initParams, graph, runtimeState, nil, testSubgraphEngineFactory())
	require.NoError(t, err)

	events, runErr := runNode(t, node)
	require.NoError(t, runErr)

	result := findRunCompletedResult(t, events)
	require.NotNil(t, result)
	output, ok := result.Outputs["output"].([]any)
	require.True(t, ok)
	assert.Equal(t, []any{"a", "b"}, output)
}

func TestIterationNodeCapsParallelNums(t *testing.T) {
	iterNodeID := "iter-1"
	startNodeID := "iter-start"
	answerNodeID := "iter-answer"

	graphConfig := newGraphConfig(
		[]map[string]any{
			{
				"id": iterNodeID,
				"data": map[string]any{
					"type":              "iteration",
					"start_node_id":     startNodeID,
					"iterator_selector": []string{"start", "items"},
					"is_parallel":       false,
					"parallel_nums":     99,
				},
			},
			{
				"id":       startNodeID,
				"parentId": iterNodeID,
				"data": map[string]any{
					"type": "iteration-start",
				},
			},
			{
				"id":       answerNodeID,
				"parentId": iterNodeID,
				"data": map[string]any{
					"type": "answer",
				},
			},
		},
		[]map[string]any{
			{"source": startNodeID, "target": answerNodeID},
		},
	)

	variablePool := entities.NewVariablePool()
	variablePool.Add([]string{"start", "items"}, []any{"a"})
	runtimeState := entities.NewGraphRuntimeState(variablePool)
	capturedParallelism := -1

	node, err := iter.New(iterNodeID, graphConfig["nodes"].([]interface{})[0].(map[string]interface{}), newInitParams(graphConfig), &entities.Graph{Config: graphConfig}, runtimeState, nil, subgraph.EngineFactory(func(parallelism int) subgraph.Engine {
		capturedParallelism = parallelism
		return &recordingEngine{}
	}))
	require.NoError(t, err)

	_, runErr := runNode(t, node)
	require.NoError(t, runErr)
	require.Equal(t, graph_engine.MaxConcurrencyLimit, capturedParallelism)
}
