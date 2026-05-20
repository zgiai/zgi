package workflow_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/zgiai/ginext/internal/modules/app/workflow/graph_engine/entities"
	"github.com/zgiai/ginext/internal/modules/app/workflow/graph_engine/loop_subgraph"
)

func TestLoopSubgraphReturnsInnerNodeErrorWithoutHanging(t *testing.T) {
	startNodeID := "loop-start"
	llmNodeID := "loop-llm"
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
			"id": llmNodeID,
			"data": map[string]any{
				"type":  "llm",
				"title": "LLM",
				"context": map[string]any{
					"enabled":           false,
					"variable_selector": []any{},
				},
				"memory": map[string]any{
					"role_prefix": map[string]any{
						"assistant": "",
						"user":      "",
					},
					"window": map[string]any{
						"enabled": false,
						"size":    50,
					},
				},
				"model": map[string]any{
					"provider": "openai",
					"name":     "gpt-4o-mini",
					"mode":     "chat",
					"completion_params": map[string]any{
						"frequency_penalty": 0,
						"max_tokens":        16,
						"presence_penalty":  0,
						"temperature":       0,
						"top_p":             1,
					},
				},
				"prompt_template": []map[string]any{
					{
						"role": "system",
						"text": "hello",
					},
				},
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
		{"source": startNodeID, "target": llmNodeID},
		{"source": llmNodeID, "target": endNodeID},
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

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	_, err := executor.Run(ctx, 0)
	require.Error(t, err)
	require.ErrorContains(t, err, "llm invoker is not initialized")
}
