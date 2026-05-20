package workflow_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/zgiai/zgi/api/internal/modules/app/workflow/graph_engine/entities"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/shared"
)

// This test reproduces a branch-routing bug:
// When a node is skipped due to conditional branch mismatch, its downstream nodes connected
// via default "source" edges must also be skipped (otherwise the engine can "leak" execution
// into an inactive branch).
func TestBranchSkipPropagatesAcrossUnconditionalEdges(t *testing.T) {
	// Graph shape:
	//   if1(case2) -> d (selected)
	//   if1(true)  -> b (skipped) -> c (must be skipped)

	nodes := []map[string]any{
		{
			"id": "if1",
			"data": map[string]any{
				"type": "if-else",
				"cases": []any{
					map[string]any{
						"case_id":          "true",
						"logical_operator": "and",
						"conditions": []any{
							map[string]any{
								"variable_selector":      []any{"input", "x"},
								"comparison_operator":    "is",
								"value":                  "A",
								"sub_variable_condition": nil,
							},
						},
					},
					map[string]any{
						"case_id":          "case2",
						"logical_operator": "and",
						"conditions": []any{
							map[string]any{
								"variable_selector":   []any{"input", "x"},
								"comparison_operator": "is",
								"value":               "B",
							},
						},
					},
				},
			},
		},
		{
			"id": "b",
			"data": map[string]any{
				"type":   "answer",
				"answer": "B",
			},
		},
		{
			"id": "c",
			"data": map[string]any{
				"type":   "answer",
				"answer": "C",
			},
		},
		{
			"id": "d",
			"data": map[string]any{
				"type":   "answer",
				"answer": "D",
			},
		},
	}

	edges := []map[string]any{
		{"source": "if1", "sourceHandle": "true", "target": "b"},
		{"source": "b", "target": "c"},
		{"source": "if1", "sourceHandle": "case2", "target": "d"},
	}

	graphConfig := newGraphConfig(nodes, edges)

	variablePool := entities.NewVariablePool()
	variablePool.Add([]string{"input", "x"}, "B") // selects case2

	runtimeState := entities.NewGraphRuntimeState(variablePool)
	graph := &entities.Graph{Config: graphConfig}

	engine := newTestWorkflowEngine(4)
	engine.SetRuntimeState(runtimeState, graph)

	// Add nodes
	engine.AddNode("if1", shared.IfElse, map[string]any{"id": "if1", "data": nodes[0]["data"]})
	engine.AddNode("b", shared.Answer, map[string]any{"id": "b", "data": nodes[1]["data"]})
	engine.AddNode("c", shared.Answer, map[string]any{"id": "c", "data": nodes[2]["data"]})
	engine.AddNode("d", shared.Answer, map[string]any{"id": "d", "data": nodes[3]["data"]})

	// Add dependencies with correct sourceHandle routing.
	require.NoError(t, engine.AddDependencyWithHandle("if1", "b", "true"))
	require.NoError(t, engine.AddDependencyWithHandle("b", "c", "source"))
	require.NoError(t, engine.AddDependencyWithHandle("if1", "d", "case2"))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	require.NoError(t, engine.Execute(ctx))

	// Selected branch runs.
	dVar := variablePool.Get([]string{"d", "answer"})
	require.NotNil(t, dVar)
	require.Equal(t, "D", dVar.ToObject())

	// Inactive branch must not "leak" to downstream nodes.
	// Node b is behind a mismatched conditional edge and is skipped.
	// Node c is downstream of b via an unconditional edge, and must also be skipped.
	cVar := variablePool.Get([]string{"c", "answer"})
	require.Nil(t, cVar)
}

func TestMergeRunsWhenAnyConditionalUpstreamBranchIsActive(t *testing.T) {
	// Graph shape mirrors a parallel conditional fan-in:
	//   start -> if1(false) -> end
	//   start -> if2(true)  -> code -> end
	//
	// The code node has two conditional upstreams. It must run because if2 activates it,
	// even though if1's true handle is inactive.
	nodes := []map[string]any{
		{
			"id": "start",
			"data": map[string]any{
				"type":      "start",
				"variables": []any{},
			},
		},
		{
			"id": "if1",
			"data": map[string]any{
				"type": "if-else",
				"cases": []any{
					map[string]any{
						"case_id":          "true",
						"logical_operator": "and",
						"conditions": []any{
							map[string]any{
								"variable_selector":   []any{"start", "input"},
								"comparison_operator": "is",
								"value":               "123",
							},
						},
					},
				},
			},
		},
		{
			"id": "if2",
			"data": map[string]any{
				"type": "if-else",
				"cases": []any{
					map[string]any{
						"case_id":          "true",
						"logical_operator": "and",
						"conditions": []any{
							map[string]any{
								"variable_selector":   []any{"start", "input"},
								"comparison_operator": "contains",
								"value":               "1",
							},
						},
					},
				},
			},
		},
		{
			"id": "code",
			"data": map[string]any{
				"type":   "answer",
				"answer": "code ran",
			},
		},
		{
			"id": "end",
			"data": map[string]any{
				"type":   "answer",
				"answer": "end ran",
			},
		},
	}

	edges := []map[string]any{
		{"source": "start", "sourceHandle": "source", "target": "if1"},
		{"source": "start", "sourceHandle": "source", "target": "if2"},
		{"source": "if1", "sourceHandle": "true", "target": "code"},
		{"source": "if1", "sourceHandle": "false", "target": "end"},
		{"source": "if2", "sourceHandle": "true", "target": "code"},
		{"source": "if2", "sourceHandle": "false", "target": "end"},
		{"source": "code", "sourceHandle": "source", "target": "end"},
	}

	graphConfig := newGraphConfig(nodes, edges)

	variablePool := entities.NewVariablePool()
	variablePool.Add([]string{"start", "input"}, "1")

	runtimeState := entities.NewGraphRuntimeState(variablePool)
	graph := &entities.Graph{Config: graphConfig}

	engine := newTestWorkflowEngine(4)
	engine.SetRuntimeState(runtimeState, graph)

	engine.AddNode("start", shared.Start, map[string]any{"id": "start", "data": nodes[0]["data"]})
	engine.AddNode("if1", shared.IfElse, map[string]any{"id": "if1", "data": nodes[1]["data"]})
	engine.AddNode("if2", shared.IfElse, map[string]any{"id": "if2", "data": nodes[2]["data"]})
	engine.AddNode("code", shared.Answer, map[string]any{"id": "code", "data": nodes[3]["data"]})
	engine.AddNode("end", shared.Answer, map[string]any{"id": "end", "data": nodes[4]["data"]})

	require.NoError(t, engine.AddDependencyWithHandle("start", "if1", "source"))
	require.NoError(t, engine.AddDependencyWithHandle("start", "if2", "source"))
	require.NoError(t, engine.AddDependencyWithHandle("if1", "code", "true"))
	require.NoError(t, engine.AddDependencyWithHandle("if1", "end", "false"))
	require.NoError(t, engine.AddDependencyWithHandle("if2", "code", "true"))
	require.NoError(t, engine.AddDependencyWithHandle("if2", "end", "false"))
	require.NoError(t, engine.AddDependencyWithHandle("code", "end", "source"))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	require.NoError(t, engine.Execute(ctx))

	codeVar := variablePool.Get([]string{"code", "answer"})
	require.NotNil(t, codeVar)
	require.Equal(t, "code ran", codeVar.ToObject())

	endVar := variablePool.Get([]string{"end", "answer"})
	require.NotNil(t, endVar)
	require.Equal(t, "end ran", endVar.ToObject())
}
