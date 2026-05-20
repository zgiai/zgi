package workflow_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/zgiai/zgi/api/internal/modules/app/workflow/streamscheduler"
)

// This test reproduces a streaming-scheduler deadlock class:
// When a conditional branch node is skipped, its downstream nodes must still be enqueued
// so they can also be marked skipped; otherwise, merge nodes can wait forever on "incomplete"
// upstreams that are never visited.
func TestStreamScheduler_SkipBranchStillEnqueuesDownstream(t *testing.T) {
	// Graph shape:
	//   if(case2) -> c1 -> merge
	//   if(true)  -> t1 (skipped) -> t2 (must be visited+skipped) -> merge
	edgeMap := map[string]map[string][]string{
		"if": {
			"true":  {"t1"},
			"case2": {"c1"},
		},
		"t1": {"source": {"t2"}},
		"t2": {"source": {"merge"}},
		"c1": {"source": {"merge"}},
	}

	// reverseEdgeMap[target] = []upstreams
	reverseEdgeMap := map[string][]string{
		"t1":    {"if"},
		"c1":    {"if"},
		"t2":    {"t1"},
		"merge": {"t2", "c1"},
	}

	nodeQueue := []string{"if"}
	completed := map[string]bool{}
	skipped := map[string]bool{}
	executionOutputs := map[string]map[string]any{
		"if": {"selected_case_id": "case2"},
	}

	connectingHandle := func(upstreamID, targetID string) string {
		for handle, targets := range edgeMap[upstreamID] {
			for _, tID := range targets {
				if tID == targetID {
					return handle
				}
			}
		}
		return ""
	}

	const maxIters = 50
	for i := 0; i < maxIters && len(nodeQueue) > 0; i++ {
		current := nodeQueue[0]
		nodeQueue = nodeQueue[1:]

		if completed[current] {
			continue
		}

		// Must wait until all upstreams are completed.
		allUpstreamComplete := true
		for _, up := range reverseEdgeMap[current] {
			if !completed[up] {
				allUpstreamComplete = false
				break
			}
		}
		if !allUpstreamComplete {
			nodeQueue = append(nodeQueue, current)
			continue
		}

		// Determine whether this node should run or be skipped.
		shouldRun := false
		upstreams := reverseEdgeMap[current]
		if len(upstreams) == 0 {
			shouldRun = true
		} else {
			for _, up := range upstreams {
				if skipped[up] {
					continue
				}

				handle := connectingHandle(up, current)
				selected := "source"
				if out := executionOutputs[up]; out != nil {
					if caseID, ok := out["selected_case_id"].(string); ok && caseID != "" {
						selected = caseID
					}
				}

				if handle == "" || handle == selected {
					shouldRun = true
					break
				}
			}
		}

		completed[current] = true
		if !shouldRun {
			skipped[current] = true
		}

		// Key invariant: always enqueue downstreams for both executed and skipped nodes.
		streamscheduler.EnqueueDownstreams(&nodeQueue, edgeMap, current, completed, nil)
	}

	require.True(t, completed["merge"], "merge should eventually be completed")
	require.False(t, skipped["merge"], "merge should not be skipped when case2 branch is active")
}
