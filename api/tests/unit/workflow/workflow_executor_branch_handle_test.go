package workflow_test

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/zgiai/zgi/api/internal/modules/app/workflow"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/graph_engine"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/shared"
)

type branchHandleNodeRunner struct {
	mu      sync.Mutex
	started map[string]bool
}

func newBranchHandleNodeRunner() *branchHandleNodeRunner {
	return &branchHandleNodeRunner{started: make(map[string]bool)}
}

func (r *branchHandleNodeRunner) RunNode(ctx context.Context, req graph_engine.NodeRunRequest, eventChan chan<- *shared.NodeEventCh) (*shared.NodeRunResult, error) {
	_ = ctx
	_ = eventChan

	r.mu.Lock()
	r.started[req.NodeID] = true
	r.mu.Unlock()

	result := &shared.NodeRunResult{
		Status:  shared.SUCCEEDED,
		Inputs:  map[string]any{},
		Outputs: map[string]any{"node_id": req.NodeID},
	}
	if req.NodeID == "if1" {
		result.EdgeSourceHandle = "case2"
	}
	return result, nil
}

func (r *branchHandleNodeRunner) wasStarted(nodeID string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.started[nodeID]
}

func TestWorkflowExecutorPreservesTopLevelSourceHandleRouting(t *testing.T) {
	runner := newBranchHandleNodeRunner()
	executor := workflow.NewWorkflowExecutorWithRuntimeDeps(workflow.WorkflowExecutorDeps{
		EngineFactory: graph_engine.NewEngineFactory(10, runner),
	})

	graphData := map[string]interface{}{
		"nodes": []interface{}{
			workflowTestNode("start", "start"),
			workflowTestNode("if1", "if-else"),
			workflowTestNode("answer-case1", "answer"),
			workflowTestNode("answer-case2", "answer"),
			workflowTestNode("leaked-downstream", "answer"),
		},
		"edges": []interface{}{
			workflowTestEdge("start", "if1"),
			workflowTestEdgeWithHandle("if1", "true", "answer-case1"),
			workflowTestEdgeWithHandle("if1", "case2", "answer-case2"),
			workflowTestEdge("answer-case1", "leaked-downstream"),
		},
	}

	result, err := executor.ExecuteSimpleWorkflow(context.Background(), graphData, map[string]interface{}{})

	require.NoError(t, err)
	require.Equal(t, "succeeded", result.Status)
	require.True(t, runner.wasStarted("start"))
	require.True(t, runner.wasStarted("if1"))
	require.True(t, runner.wasStarted("answer-case2"))
	require.False(t, runner.wasStarted("answer-case1"))
	require.False(t, runner.wasStarted("leaked-downstream"))
}

func workflowTestEdgeWithHandle(source string, sourceHandle string, target string) map[string]interface{} {
	return map[string]interface{}{
		"source":       source,
		"sourceHandle": sourceHandle,
		"target":       target,
	}
}
