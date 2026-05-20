package workflow_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	workflowpkg "github.com/zgiai/zgi/api/internal/modules/app/workflow"
)

func TestEnvironmentNamespaceEnvironmentOnly(t *testing.T) {
	executor := workflowpkg.NewWorkflowExecutor()

	t.Run("environment_prefix_succeeds", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		workflowData := buildAssignerWorkflowGraph([]map[string]any{
			{
				"name":       "threshold",
				"value_type": "number",
				"value":      1,
			},
		}, nil, []map[string]any{
			{
				"variable_selector": []any{"environment", "threshold"},
				"input_type":        "constant",
				"operation":         "over-write",
				"value":             2,
			},
		})

		result, err := executor.ExecuteSimpleWorkflowWithRunID(ctx, "", workflowData, map[string]any{})
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, "succeeded", result.Status)
	})

	t.Run("env_prefix_is_not_compatible", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		workflowData := buildAssignerWorkflowGraphNoTerminal([]map[string]any{
			{
				"name":       "threshold",
				"value_type": "number",
				"value":      1,
			},
		}, nil, []map[string]any{
			{
				"variable_selector": []any{"env", "threshold"},
				"input_type":        "constant",
				"operation":         "over-write",
				"value":             2,
			},
		})

		result, err := executor.ExecuteSimpleWorkflowWithRunID(ctx, "", workflowData, map[string]any{})
		if err != nil {
			assert.True(t, strings.Contains(err.Error(), "variable [env threshold] not found"))
			return
		}
		require.NotNil(t, result)
		assert.Equal(t, "failed", result.Status)
	})
}

func TestVariableConfigListSupportsAnyAndMapSlices(t *testing.T) {
	executor := workflowpkg.NewWorkflowExecutor()

	t.Run("slice_any_shape", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		workflowData := buildAssignerWorkflowGraph(
			[]map[string]any{
				{
					"name":       "threshold",
					"value_type": "number",
					"value":      1,
				},
			},
			nil,
			[]map[string]any{
				{
					"variable_selector": []any{"environment", "threshold"},
					"input_type":        "constant",
					"operation":         "over-write",
					"value":             2,
				},
			},
		)
		workflowData["environment_variables"] = []any{
			map[string]any{
				"name":       "threshold",
				"value_type": "number",
				"value":      1,
			},
		}
		workflowData["conversation_variables"] = []any{}

		result, err := executor.ExecuteSimpleWorkflowWithRunID(ctx, "", workflowData, map[string]any{})
		require.NoError(t, err)
		assert.Equal(t, "succeeded", result.Status)
	})

	t.Run("slice_map_shape", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		workflowData := buildAssignerWorkflowGraph(
			[]map[string]any{
				{
					"name":       "threshold",
					"value_type": "number",
					"value":      1,
				},
			},
			nil,
			[]map[string]any{
				{
					"variable_selector": []any{"environment", "threshold"},
					"input_type":        "constant",
					"operation":         "over-write",
					"value":             2,
				},
			},
		)

		result, err := executor.ExecuteSimpleWorkflowWithRunID(ctx, "", workflowData, map[string]any{})
		require.NoError(t, err)
		assert.Equal(t, "succeeded", result.Status)
	})
}

func buildAssignerWorkflowGraph(
	environmentVariables []map[string]any,
	conversationVariables []map[string]any,
	assignItems []map[string]any,
) map[string]any {
	nodes := []any{
		map[string]any{
			"id": "start",
			"data": map[string]any{
				"type":      "start",
				"variables": []any{},
			},
		},
		map[string]any{
			"id": "assign",
			"data": map[string]any{
				"type":  "assigner",
				"items": assignItems,
			},
		},
		map[string]any{
			"id": "end",
			"data": map[string]any{
				"type": "end",
				"outputs": []any{
					map[string]any{
						"variable":       "threshold",
						"value_selector": []any{"environment", "threshold"},
					},
				},
			},
		},
	}

	edges := []any{
		map[string]any{"source": "start", "target": "assign"},
		map[string]any{"source": "assign", "target": "end"},
	}

	return map[string]any{
		"nodes":                  nodes,
		"edges":                  edges,
		"environment_variables":  environmentVariables,
		"conversation_variables": conversationVariables,
	}
}

func buildAssignerWorkflowGraphNoTerminal(
	environmentVariables []map[string]any,
	conversationVariables []map[string]any,
	assignItems []map[string]any,
) map[string]any {
	nodes := []any{
		map[string]any{
			"id": "start",
			"data": map[string]any{
				"type":      "start",
				"variables": []any{},
			},
		},
		map[string]any{
			"id": "assign",
			"data": map[string]any{
				"type":  "assigner",
				"items": assignItems,
			},
		},
	}

	edges := []any{
		map[string]any{"source": "start", "target": "assign"},
	}

	return map[string]any{
		"nodes":                  nodes,
		"edges":                  edges,
		"environment_variables":  environmentVariables,
		"conversation_variables": conversationVariables,
	}
}
