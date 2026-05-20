package workflow

import (
	"testing"
	"time"

	"github.com/zgiai/ginext/internal/modules/app/workflow/graph_engine"
	workflow_shared "github.com/zgiai/ginext/internal/modules/app/workflow/shared"
)

func TestWorkflowGraphPauseExecutorStateKeepsPendingNodesForResume(t *testing.T) {
	snapshots := []graph_engine.NodeExecutionSnapshot{
		{
			NodeID:  "start",
			Status:  workflow_shared.SUCCEEDED,
			Outputs: map[string]interface{}{"ok": true},
		},
		{
			NodeID:  "approval",
			Status:  workflow_shared.PAUSED,
			Outputs: map[string]interface{}{"__approval_form_id": "form-1"},
		},
		{
			NodeID: "side",
			Status: workflow_shared.PENDING,
		},
		{
			NodeID: "after",
			Status: workflow_shared.PENDING,
		},
	}

	nodeQueue, completedNodes, failedNodes, executionOutputs := workflowGraphPauseExecutorState(snapshots, []string{"approval"})

	if len(nodeQueue) != 2 || nodeQueue[0] != "after" || nodeQueue[1] != "side" {
		t.Fatalf("nodeQueue = %#v, want [after side]", nodeQueue)
	}
	if !completedNodes["start"] {
		t.Fatalf("completedNodes[start] = false, want true")
	}
	if completedNodes["approval"] {
		t.Fatalf("completedNodes[approval] = true, want false")
	}
	if len(failedNodes) != 0 {
		t.Fatalf("failedNodes = %#v, want empty", failedNodes)
	}
	if got := executionOutputs["approval"]["__approval_form_id"]; got != "form-1" {
		t.Fatalf("executionOutputs[approval][__approval_form_id] = %#v, want form-1", got)
	}
}

func TestWorkflowGraphPauseExecutorStateKeepsMultiplePausedNodes(t *testing.T) {
	baseTime := time.Unix(100, 0)
	snapshots := []graph_engine.NodeExecutionSnapshot{
		{
			NodeID:    "approval-b",
			Status:    workflow_shared.PAUSED,
			StartTime: baseTime.Add(time.Second),
			Outputs:   map[string]interface{}{"__approval_form_id": "form-b"},
		},
		{
			NodeID:    "approval-a",
			Status:    workflow_shared.PAUSED,
			StartTime: baseTime,
			Outputs:   map[string]interface{}{"__approval_form_id": "form-a"},
		},
		{
			NodeID:  "side",
			Status:  workflow_shared.SUCCEEDED,
			Outputs: map[string]interface{}{"ok": true},
		},
		{
			NodeID: "after-side",
			Status: workflow_shared.PENDING,
		},
	}

	pausedSnapshots := workflowGraphPausedSnapshots(snapshots)
	if len(pausedSnapshots) != 2 || pausedSnapshots[0].NodeID != "approval-a" || pausedSnapshots[1].NodeID != "approval-b" {
		t.Fatalf("workflowGraphPausedSnapshots() = %#v, want approval-a then approval-b", pausedSnapshots)
	}

	nodeQueue, completedNodes, failedNodes, executionOutputs := workflowGraphPauseExecutorState(snapshots, []string{"approval-a", "approval-b"})
	if len(nodeQueue) != 1 || nodeQueue[0] != "after-side" {
		t.Fatalf("nodeQueue = %#v, want [after-side]", nodeQueue)
	}
	if !completedNodes["side"] {
		t.Fatalf("completedNodes[side] = false, want true")
	}
	if completedNodes["approval-a"] || completedNodes["approval-b"] {
		t.Fatalf("paused approvals must not be completed: %#v", completedNodes)
	}
	if len(failedNodes) != 0 {
		t.Fatalf("failedNodes = %#v, want empty", failedNodes)
	}
	if got := executionOutputs["approval-a"]["__approval_form_id"]; got != "form-a" {
		t.Fatalf("executionOutputs[approval-a][__approval_form_id] = %#v, want form-a", got)
	}
	if got := executionOutputs["approval-b"]["__approval_form_id"]; got != "form-b" {
		t.Fatalf("executionOutputs[approval-b][__approval_form_id] = %#v, want form-b", got)
	}
}
