package workflow

import (
	"testing"

	graphentities "github.com/zgiai/zgi/api/internal/modules/app/workflow/graph_engine/entities"
)

func TestRestoreWorkflowResolvedPausedNodesSkipsConnectorAndEnqueuesDownstream(t *testing.T) {
	pool := graphentities.NewVariablePool()
	pool.Add([]string{"connector-node", "secret"}, "must-be-removed")
	queue := []string{"connector-node", "already-queued"}
	completed := map[string]bool{"connector-node": true}
	outputs := map[string]map[string]interface{}{
		"connector-node": {"state": "completed", "decision_id": "decision-1"},
	}
	predecessors := map[string]*string{}
	edges := map[string]map[string][]string{
		"connector-node": {"source": {"next-node"}},
	}

	if err := restoreWorkflowResolvedPausedNodes(
		[]string{"connector-node"}, completed, outputs, pool, &queue, edges, predecessors,
	); err != nil {
		t.Fatalf("restoreWorkflowResolvedPausedNodes() error = %v", err)
	}
	if len(queue) != 3 || queue[0] != "connector-node" || queue[1] != "already-queued" || queue[2] != "next-node" {
		t.Fatalf("resume queue = %#v", queue)
	}
	if predecessor := predecessors["next-node"]; predecessor == nil || *predecessor != "connector-node" {
		t.Fatalf("next-node predecessor = %v", predecessor)
	}
	if pool.Get([]string{"connector-node", "secret"}) != nil {
		t.Fatal("stale Connector variable survived safe terminal restoration")
	}
	if state := pool.Get([]string{"connector-node", "state"}); state == nil || state.ToObject() != "completed" {
		t.Fatalf("restored state = %#v", state)
	}
	if decision := pool.Get([]string{"connector-node", "decision_id"}); decision == nil || decision.ToObject() != "decision-1" {
		t.Fatalf("restored decision = %#v", decision)
	}
}

func TestRestoreWorkflowResolvedPausedNodesFailsClosedWithoutSafeOutputs(t *testing.T) {
	queue := []string{"connector-node"}
	err := restoreWorkflowResolvedPausedNodes(
		[]string{"connector-node"}, map[string]bool{"connector-node": true},
		map[string]map[string]interface{}{}, graphentities.NewVariablePool(), &queue,
		map[string]map[string][]string{"connector-node": {"source": {"next-node"}}},
		map[string]*string{},
	)
	if err == nil {
		t.Fatal("missing resolved outputs unexpectedly resumed the Workflow")
	}
	if len(queue) != 1 || queue[0] != "connector-node" {
		t.Fatalf("failed restore mutated queue = %#v", queue)
	}
}
