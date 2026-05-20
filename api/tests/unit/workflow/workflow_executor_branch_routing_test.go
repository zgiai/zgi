package workflow_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	workflowpkg "github.com/zgiai/ginext/internal/modules/app/workflow"
	"github.com/zgiai/ginext/internal/modules/app/workflow/graph_engine"
	"github.com/zgiai/ginext/internal/modules/app/workflow/shared"
)

func TestWorkflowExecutorPreservesBranchSourceHandlesAtMerge(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		wantRun      string
		wantSkipped  string
		wantIfHandle string
	}{
		{
			name:         "true branch selected",
			input:        "A",
			wantRun:      "true-answer",
			wantSkipped:  "false-answer",
			wantIfHandle: "true",
		},
		{
			name:         "false branch selected",
			input:        "B",
			wantRun:      "false-answer",
			wantSkipped:  "true-answer",
			wantIfHandle: "false",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			executor := workflowpkg.NewWorkflowExecutor()
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			result, err := executor.ExecuteSimpleWorkflowWithRunID(ctx, "", branchMergeWorkflowData(), map[string]any{
				"x": tt.input,
			})
			require.NoError(t, err)
			require.NotNil(t, result)

			snapshots := snapshotsByNodeID(result.NodeExecutions)
			require.Equal(t, shared.SUCCEEDED, snapshots["if"].Status)
			require.Equal(t, tt.wantIfHandle, snapshots["if"].EdgeSourceHandle)
			require.Equal(t, shared.SUCCEEDED, snapshots[tt.wantRun].Status)
			require.Equal(t, shared.SKIPPED, snapshots[tt.wantSkipped].Status)
			require.Equal(t, shared.SUCCEEDED, snapshots["merge"].Status)
		})
	}
}

func TestWorkflowExecutorAllowsMultipleBranchHandlesToSameTarget(t *testing.T) {
	executor := workflowpkg.NewWorkflowExecutor()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := executor.ExecuteSimpleWorkflowWithRunID(ctx, "", multiHandleSameTargetWorkflowData(), map[string]any{
		"x": "A",
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	snapshots := snapshotsByNodeID(result.NodeExecutions)
	require.Equal(t, shared.SUCCEEDED, snapshots["if"].Status)
	require.Equal(t, "true", snapshots["if"].EdgeSourceHandle)
	require.Equal(t, shared.SUCCEEDED, snapshots["merge"].Status)
}

func snapshotsByNodeID(snapshots []graph_engine.NodeExecutionSnapshot) map[string]graph_engine.NodeExecutionSnapshot {
	result := make(map[string]graph_engine.NodeExecutionSnapshot, len(snapshots))
	for _, snapshot := range snapshots {
		result[snapshot.NodeID] = snapshot
	}
	return result
}

func branchMergeWorkflowData() map[string]any {
	return newGraphConfig(
		[]map[string]any{
			startNode("start"),
			ifNode("if"),
			answerNode("true-answer", "true path"),
			answerNode("false-answer", "false path"),
			answerNode("merge", "merged"),
		},
		[]map[string]any{
			edge("start", "source", "if"),
			edge("if", "true", "true-answer"),
			edge("if", "false", "false-answer"),
			edge("true-answer", "source", "merge"),
			edge("false-answer", "source", "merge"),
		},
	)
}

func multiHandleSameTargetWorkflowData() map[string]any {
	return newGraphConfig(
		[]map[string]any{
			startNode("start"),
			ifNode("if"),
			answerNode("merge", "merged"),
		},
		[]map[string]any{
			edge("start", "source", "if"),
			edge("if", "true", "merge"),
			edge("if", "false", "merge"),
		},
	)
}

func startNode(id string) map[string]any {
	return map[string]any{
		"id": id,
		"data": map[string]any{
			"type":      "start",
			"variables": []any{},
		},
	}
}

func ifNode(id string) map[string]any {
	return map[string]any{
		"id": id,
		"data": map[string]any{
			"type": "if-else",
			"cases": []any{
				map[string]any{
					"case_id":          "true",
					"logical_operator": "and",
					"conditions": []any{
						map[string]any{
							"variable_selector":   []any{"start", "x"},
							"comparison_operator": "is",
							"value":               "A",
						},
					},
				},
			},
		},
	}
}

func answerNode(id, answer string) map[string]any {
	return map[string]any{
		"id": id,
		"data": map[string]any{
			"type":   "answer",
			"answer": answer,
		},
	}
}

func edge(source, sourceHandle, target string) map[string]any {
	return map[string]any{
		"source":       source,
		"sourceHandle": sourceHandle,
		"target":       target,
	}
}
