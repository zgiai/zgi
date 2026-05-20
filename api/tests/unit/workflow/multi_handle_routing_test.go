package workflow_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/zgiai/ginext/internal/modules/app/workflow/graph_engine/entities"
	"github.com/zgiai/ginext/internal/modules/app/workflow/shared"
)

func TestIfElseSupportsMultipleHandlesToSameTarget(t *testing.T) {
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
								"variable_selector":   []any{"input", "x"},
								"comparison_operator": "is",
								"value":               "A",
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
					map[string]any{
						"case_id":          "case3",
						"logical_operator": "and",
						"conditions": []any{
							map[string]any{
								"variable_selector":   []any{"input", "x"},
								"comparison_operator": "is",
								"value":               "C",
							},
						},
					},
				},
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
		{"source": "if1", "sourceHandle": "case2", "target": "d"},
		{"source": "if1", "sourceHandle": "case3", "target": "d"},
	}

	graphConfig := newGraphConfig(nodes, edges)

	variablePool := entities.NewVariablePool()
	variablePool.Add([]string{"input", "x"}, "B")

	runtimeState := entities.NewGraphRuntimeState(variablePool)
	graph := &entities.Graph{Config: graphConfig}

	engine := newTestWorkflowEngine(4)
	engine.SetRuntimeState(runtimeState, graph)

	engine.AddNode("if1", shared.IfElse, map[string]any{"id": "if1", "data": nodes[0]["data"]})
	engine.AddNode("d", shared.Answer, map[string]any{"id": "d", "data": nodes[1]["data"]})

	require.NoError(t, engine.AddDependencyWithHandle("if1", "d", "case2"))
	require.NoError(t, engine.AddDependencyWithHandle("if1", "d", "case3"))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	require.NoError(t, engine.Execute(ctx))

	dVar := variablePool.Get([]string{"d", "answer"})
	require.NotNil(t, dVar)
	require.Equal(t, "D", dVar.ToObject())
}
