package workflow_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zgiai/zgi/api/internal/modules/app/workflow/graph_engine"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/graph_engine/entities"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/graph_engine/loop_subgraph"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/graph_engine/subgraph"
)

func TestLoopSubgraphDefaultsParallelism(t *testing.T) {
	startNodeID := "loop-start"
	graphConfig := newGraphConfig(
		[]map[string]any{
			{
				"id": startNodeID,
				"data": map[string]any{
					"type":    "loop-start",
					"loop_id": "loop-1",
				},
			},
		},
		nil,
	)

	capturedParallelism := -1
	executor := loop_subgraph.New(loop_subgraph.Config{
		NodeID:       "loop-1",
		StartNodeID:  &startNodeID,
		GraphConfig:  graphConfig,
		RuntimeState: entities.NewGraphRuntimeState(entities.NewVariablePool()),
		EngineFactory: func(parallelism int) subgraph.Engine {
			capturedParallelism = parallelism
			return &recordingEngine{}
		},
	})

	_, err := executor.Run(t.Context(), 0)
	require.NoError(t, err)
	require.Equal(t, graph_engine.DefaultMaxConcurrency, capturedParallelism)
}

func TestLoopSubgraphVariableCleanup(t *testing.T) {
	startNodeID := "loop-start"
	answerNodeID := "answer1"
	endNodeID := "loop-end"

	nodes := []map[string]any{
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
				"answer":  "{{#answer1.answer#}}x",
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
	}

	edges := []map[string]any{
		{"source": startNodeID, "target": answerNodeID},
		{"source": answerNodeID, "target": endNodeID},
	}

	graphConfig := newGraphConfig(nodes, edges)
	runtimeState := entities.NewGraphRuntimeState(entities.NewVariablePool())

	executor := loop_subgraph.New(loop_subgraph.Config{
		NodeID:        "loop-1",
		StartNodeID:   &startNodeID,
		GraphConfig:   graphConfig,
		RuntimeState:  runtimeState,
		EngineFactory: testSubgraphEngineFactory(),
	})

	result1, err := executor.Run(t.Context(), 0)
	require.NoError(t, err)
	assert.Equal(t, "x", result1.Outputs["answer"])

	result2, err := executor.Run(t.Context(), 1)
	require.NoError(t, err)
	assert.Equal(t, "x", result2.Outputs["answer"])
}
