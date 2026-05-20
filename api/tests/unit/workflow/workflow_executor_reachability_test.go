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

type recordingNodeRunner struct {
	mu      sync.Mutex
	started map[string]bool
}

func newRecordingNodeRunner() *recordingNodeRunner {
	return &recordingNodeRunner{
		started: make(map[string]bool),
	}
}

func (r *recordingNodeRunner) RunNode(ctx context.Context, req graph_engine.NodeRunRequest, eventChan chan<- *shared.NodeEventCh) (*shared.NodeRunResult, error) {
	r.mu.Lock()
	r.started[req.NodeID] = true
	r.mu.Unlock()

	return &shared.NodeRunResult{
		Status:  shared.SUCCEEDED,
		Inputs:  map[string]any{},
		Outputs: map[string]any{"node_id": req.NodeID},
	}, nil
}

func (r *recordingNodeRunner) wasStarted(nodeID string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.started[nodeID]
}

func TestWorkflowExecutorIgnoresNodesUnreachableFromStart(t *testing.T) {
	runner := newRecordingNodeRunner()
	executor := workflow.NewWorkflowExecutorWithRuntimeDeps(workflow.WorkflowExecutorDeps{
		EngineFactory: graph_engine.NewEngineFactory(10, runner),
	})

	graphData := map[string]interface{}{
		"nodes": []interface{}{
			workflowTestNode("start", "start"),
			workflowTestNode("reachable-answer", "answer"),
			workflowTestNode("orphan-llm", "llm"),
			workflowTestNode("orphan-answer", "answer"),
		},
		"edges": []interface{}{
			workflowTestEdge("start", "reachable-answer"),
			workflowTestEdge("start", "missing-node"),
			workflowTestEdge("missing-node", "orphan-llm"),
			workflowTestEdge("orphan-llm", "orphan-answer"),
		},
	}

	result, err := executor.ExecuteSimpleWorkflow(context.Background(), graphData, map[string]interface{}{})

	require.NoError(t, err)
	require.Equal(t, "succeeded", result.Status)
	require.True(t, runner.wasStarted("start"))
	require.True(t, runner.wasStarted("reachable-answer"))
	require.False(t, runner.wasStarted("orphan-llm"))
	require.False(t, runner.wasStarted("orphan-answer"))
	require.Contains(t, result.NodeResults, "start")
	require.Contains(t, result.NodeResults, "reachable-answer")
	require.NotContains(t, result.NodeResults, "orphan-llm")
	require.NotContains(t, result.NodeResults, "orphan-answer")
}

func workflowTestNode(id string, nodeType string) map[string]interface{} {
	return map[string]interface{}{
		"id": id,
		"data": map[string]interface{}{
			"type":  nodeType,
			"title": id,
		},
	}
}

func workflowTestEdge(source string, target string) map[string]interface{} {
	return map[string]interface{}{
		"source": source,
		"target": target,
	}
}
