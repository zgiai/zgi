package workflow_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	workflowpkg "github.com/zgiai/zgi/api/internal/modules/app/workflow"
)

func TestMissingValueStillInitializesVariable(t *testing.T) {
	executor := workflowpkg.NewWorkflowExecutor()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	workflowData := buildAssignerWorkflowGraph(
		[]map[string]any{
			{
				"name":       "threshold",
				"value_type": "number",
				// Intentionally no "value" field: should still initialize by lenient default.
			},
		},
		nil,
		[]map[string]any{
			{
				"variable_selector": []any{"environment", "threshold"},
				"input_type":        "constant",
				"operation":         "+=",
				"value":             2,
			},
		},
	)

	result, err := executor.ExecuteSimpleWorkflowWithRunID(ctx, "", workflowData, map[string]any{})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "succeeded", result.Status)
}

func TestTypeFieldHasPriorityOverValueType(t *testing.T) {
	executor := workflowpkg.NewWorkflowExecutor()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	workflowData := buildAssignerWorkflowGraphNoTerminal(
		[]map[string]any{
			{
				"name":       "threshold",
				"type":       "string",
				"value_type": "number",
				"value":      "abc",
			},
		},
		nil,
		[]map[string]any{
			{
				"variable_selector": []any{"environment", "threshold"},
				"input_type":        "constant",
				"operation":         "+=",
				"value":             1,
			},
		},
	)

	result, err := executor.ExecuteSimpleWorkflowWithRunID(ctx, "", workflowData, map[string]any{})
	if err != nil {
		assert.Contains(t, err.Error(), `operation "+=" is not supported for type string`)
		return
	}
	require.NotNil(t, result)
	assert.Equal(t, "failed", result.Status)
}
