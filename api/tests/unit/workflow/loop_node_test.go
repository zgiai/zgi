package workflow_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zgiai/ginext/internal/modules/app/workflow/graph_engine"
	"github.com/zgiai/ginext/internal/modules/app/workflow/graph_engine/entities"
	"github.com/zgiai/ginext/internal/modules/app/workflow/graph_engine/subgraph"
	"github.com/zgiai/ginext/internal/modules/app/workflow/nodes/loop"
	"github.com/zgiai/ginext/internal/modules/app/workflow/shared"
)

func TestLoopNodeParseConfig(t *testing.T) {
	graphConfig := newGraphConfig(nil, nil)
	initParams := newInitParams(graphConfig)
	runtimeState := entities.NewGraphRuntimeState(entities.NewVariablePool())
	graph := &entities.Graph{Config: graphConfig}

	_, err := loop.New("loop-1", map[string]any{
		"data": map[string]any{"type": "loop"},
	}, initParams, graph, runtimeState, nil)
	require.Error(t, err)
}

func TestLoopNodeStrictValidation(t *testing.T) {
	graphConfig := newGraphConfig(nil, nil)
	initParams := newInitParams(graphConfig)
	runtimeState := entities.NewGraphRuntimeState(entities.NewVariablePool())
	graph := &entities.Graph{Config: graphConfig}

	loopConfig := map[string]any{
		"id": "loop-1",
		"data": map[string]any{
			"type":          "loop",
			"start_node_id": "loop-start",
			"loop_count":    1,
			"loop_variables": []map[string]any{
				{
					"label":      "flag",
					"var_type":   "boolean",
					"value_type": "constant",
					"value":      "not-bool",
				},
			},
		},
	}

	node, err := loop.New("loop-1", loopConfig, initParams, graph, runtimeState, nil, testSubgraphEngineFactory())
	require.NoError(t, err)

	_, runErr := runNode(t, node)
	require.Error(t, runErr)
}

func TestLoopNodeCapsParallelNums(t *testing.T) {
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
	initParams := newInitParams(graphConfig)
	runtimeState := entities.NewGraphRuntimeState(entities.NewVariablePool())
	graph := &entities.Graph{Config: graphConfig}
	capturedParallelism := -1

	loopConfig := map[string]any{
		"id": "loop-1",
		"data": map[string]any{
			"type":          "loop",
			"start_node_id": startNodeID,
			"loop_count":    1,
			"parallel_nums": 99,
		},
	}

	node, err := loop.New("loop-1", loopConfig, initParams, graph, runtimeState, nil, subgraph.EngineFactory(func(parallelism int) subgraph.Engine {
		capturedParallelism = parallelism
		return &recordingEngine{}
	}))
	require.NoError(t, err)

	_, runErr := runNode(t, node)
	require.NoError(t, runErr)
	require.Equal(t, graph_engine.MaxConcurrencyLimit, capturedParallelism)
}

func TestLoopNodeBreakConditionStopsEarly(t *testing.T) {
	graphConfig := newGraphConfig(nil, nil)
	initParams := newInitParams(graphConfig)
	runtimeState := entities.NewGraphRuntimeState(entities.NewVariablePool())
	graph := &entities.Graph{Config: graphConfig}

	loopConfig := map[string]any{
		"id": "loop-1",
		"data": map[string]any{
			"type":             "loop",
			"start_node_id":    "loop-start",
			"loop_count":       2,
			"logical_operator": "and",
			"break_conditions": []map[string]any{
				{
					"variable_selector":   []string{"missing", "value"},
					"comparison_operator": "not exists",
				},
			},
		},
	}

	node, err := loop.New("loop-1", loopConfig, initParams, graph, runtimeState, nil, testSubgraphEngineFactory())
	require.NoError(t, err)

	events, runErr := runNode(t, node)
	require.NoError(t, runErr)

	assert.Equal(t, 0, countEventType(events, shared.EventTypeLoopNext))

	result := findRunCompletedResult(t, events)
	require.NotNil(t, result)
	_, hasLoopRound := result.Outputs["loop_round"]
	assert.False(t, hasLoopRound)
}

func TestLoopNodeLoopEndStopsIteration(t *testing.T) {
	startNodeID := "loop-start"
	answerNodeID := "loop-answer"
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
				"answer":  "ok",
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
	initParams := newInitParams(graphConfig)
	runtimeState := entities.NewGraphRuntimeState(entities.NewVariablePool())
	graph := &entities.Graph{Config: graphConfig}

	loopConfig := map[string]any{
		"id": "loop-1",
		"data": map[string]any{
			"type":          "loop",
			"start_node_id": startNodeID,
			"loop_count":    3,
		},
	}

	node, err := loop.New("loop-1", loopConfig, initParams, graph, runtimeState, nil, testSubgraphEngineFactory())
	require.NoError(t, err)

	events, runErr := runNode(t, node)
	require.NoError(t, runErr)

	assert.Equal(t, 0, countEventType(events, shared.EventTypeLoopNext))

	result := findRunCompletedResult(t, events)
	require.NotNil(t, result)
	assert.Equal(t, 1, result.Outputs["loop_round"])

	var succeeded *loop.LoopSucceededEvent
	for _, event := range events {
		if event.Type != shared.EventTypeLoopSucceeded {
			continue
		}
		payload, ok := event.Data.(*loop.LoopSucceededEvent)
		require.True(t, ok)
		succeeded = payload
		break
	}
	require.NotNil(t, succeeded)

	loopVarsRaw, ok := succeeded.Metadata["loop_variable_map"]
	require.True(t, ok)
	switch v := loopVarsRaw.(type) {
	case map[string]any:
		assert.Len(t, v, 1)
	case map[string]map[string]any:
		assert.Len(t, v, 1)
	default:
		require.Fail(t, "unexpected loop_variable_map type")
	}
}

func TestLoopNextEmittedAfterIterationWithPreviousOutput(t *testing.T) {
	startNodeID := "loop-start"
	answerNodeID := "loop-answer"

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
				"answer":  "ok",
				"loop_id": "loop-1",
			},
		},
	}

	edges := []map[string]any{
		{"source": startNodeID, "target": answerNodeID},
	}

	graphConfig := newGraphConfig(nodes, edges)
	initParams := newInitParams(graphConfig)
	runtimeState := entities.NewGraphRuntimeState(entities.NewVariablePool())
	graph := &entities.Graph{Config: graphConfig}

	loopConfig := map[string]any{
		"id": "loop-1",
		"data": map[string]any{
			"type":          "loop",
			"start_node_id": startNodeID,
			"loop_count":    2,
		},
	}

	node, err := loop.New("loop-1", loopConfig, initParams, graph, runtimeState, nil, testSubgraphEngineFactory())
	require.NoError(t, err)

	events, runErr := runNode(t, node)
	require.NoError(t, runErr)

	var firstInternalFinished int = -1
	var loopNextIndex int = -1
	var loopNextPayload *loop.LoopNextEvent
	for idx, event := range events {
		if event.Type == shared.EventTypeInternalNodeFinished && event.NodeID == answerNodeID && firstInternalFinished == -1 {
			firstInternalFinished = idx
		}
		if event.Type == shared.EventTypeLoopNext {
			loopNextIndex = idx
			payload, ok := event.Data.(*loop.LoopNextEvent)
			require.True(t, ok)
			loopNextPayload = payload
		}
	}

	require.NotEqual(t, -1, firstInternalFinished)
	require.NotEqual(t, -1, loopNextIndex)
	require.NotNil(t, loopNextPayload)
	assert.Greater(t, loopNextIndex, firstInternalFinished)
	assert.Equal(t, 1, loopNextPayload.Index)
	assert.Equal(t, "ok", loopNextPayload.PreLoopOutput["answer"])
}

func TestLoopEventStructure(t *testing.T) {
	startNodeID := "loop-start"
	answerNodeID := "loop-answer"

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
				"answer":  "done",
				"loop_id": "loop-1",
			},
		},
	}

	edges := []map[string]any{
		{"source": startNodeID, "target": answerNodeID},
	}

	graphConfig := newGraphConfig(nodes, edges)
	initParams := newInitParams(graphConfig)
	runtimeState := entities.NewGraphRuntimeState(entities.NewVariablePool())
	graph := &entities.Graph{Config: graphConfig}

	loopConfig := map[string]any{
		"id": "loop-1",
		"data": map[string]any{
			"type":          "loop",
			"start_node_id": startNodeID,
			"loop_count":    1,
		},
	}

	node, err := loop.New("loop-1", loopConfig, initParams, graph, runtimeState, nil, testSubgraphEngineFactory())
	require.NoError(t, err)

	events, runErr := runNode(t, node)
	require.NoError(t, runErr)

	var started *loop.LoopStartedEvent
	var succeeded *loop.LoopSucceededEvent
	for _, event := range events {
		switch event.Type {
		case shared.EventTypeLoopStarted:
			payload, ok := event.Data.(*loop.LoopStartedEvent)
			require.True(t, ok)
			started = payload
		case shared.EventTypeLoopSucceeded:
			payload, ok := event.Data.(*loop.LoopSucceededEvent)
			require.True(t, ok)
			succeeded = payload
		}
	}

	require.NotNil(t, started)
	assert.Equal(t, 1, started.Metadata["loop_length"])

	require.NotNil(t, succeeded)
	_, ok := succeeded.Metadata["loop_duration_map"]
	assert.True(t, ok)
	_, ok = succeeded.Metadata["loop_variable_map"]
	assert.True(t, ok)
}
