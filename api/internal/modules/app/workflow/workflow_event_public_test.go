package workflow

import (
	"context"
	"testing"

	workflowpause "github.com/zgiai/zgi/api/internal/modules/app/workflow/pause"
)

func TestWorkflowRunEventDispatcherNormalizesLoopRoundIndex(t *testing.T) {
	ctx := context.Background()
	events := make([]string, 0, 4)
	dispatcher := &workflowRunEventDispatcher{
		onEvent: func(eventType string, data map[string]interface{}, _ *workflowpause.RunEventPayload) error {
			events = append(events, eventType)
			return nil
		},
		containers: map[string]workflowRunContainerState{},
	}

	dispatcher.Dispatch(ctx, "loop_started", map[string]interface{}{
		"node_id":   "loop-1",
		"node_type": "loop",
		"title":     "循环",
	})
	dispatcher.Dispatch(ctx, "node_started", map[string]interface{}{
		"node_id":    "llm-1",
		"node_type":  "llm",
		"title":      "LLM",
		"loop_id":    "loop-1",
		"loop_index": 0,
	})
	if len(events) != 1 || events[0] != "loop_started" {
		t.Fatalf("events before loop_next = %#v, want only loop_started", events)
	}

	dispatcher.Dispatch(ctx, "loop_next", map[string]interface{}{
		"node_id":   "loop-1",
		"node_type": "loop",
		"title":     "循环",
		"index":     1,
	})
	if len(events) != 3 || events[1] != "loop_next" || events[2] != "node_started" {
		t.Fatalf("events after loop_next = %#v, want child flushed after normalized round", events)
	}
}

func TestWorkflowRunEventDispatcherDropsUnmatchedPendingAfterTerminal(t *testing.T) {
	ctx := context.Background()
	events := make([]string, 0, 4)
	dispatcher := &workflowRunEventDispatcher{
		onEvent: func(eventType string, data map[string]interface{}, _ *workflowpause.RunEventPayload) error {
			events = append(events, eventType)
			return nil
		},
		containers: map[string]workflowRunContainerState{},
	}

	dispatcher.Dispatch(ctx, "loop_started", map[string]interface{}{
		"node_id":   "loop-1",
		"node_type": "loop",
	})
	dispatcher.Dispatch(ctx, "node_started", map[string]interface{}{
		"node_id":    "late-child",
		"node_type":  "llm",
		"loop_id":    "loop-1",
		"loop_index": 9,
	})
	dispatcher.Dispatch(ctx, "workflow_finished", map[string]interface{}{
		"workflow_run_id": "run-1",
		"status":          "succeeded",
	})
	dispatcher.Close(ctx)

	if len(events) != 2 || events[0] != "loop_started" || events[1] != "workflow_finished" {
		t.Fatalf("events = %#v, want unmatched child dropped instead of flushed after terminal", events)
	}
}
