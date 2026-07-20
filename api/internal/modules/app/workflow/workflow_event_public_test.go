package workflow

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/zgiai/zgi/api/internal/dto"
	workflowpause "github.com/zgiai/zgi/api/internal/modules/app/workflow/pause"
	"github.com/zgiai/zgi/api/pkg/database"
)

func TestAppendWorkflowRunEventKeepsContainerRoundNodeExecutionsDistinct(t *testing.T) {
	db := openWorkflowStopV2TestDB(t)
	if err := db.AutoMigrate(&WorkflowRunLog{}, &WorkflowNodeRuntimeLog{}, &workflowpause.RunEvent{}); err != nil {
		t.Fatalf("migrate runtime event tables: %v", err)
	}
	oldDB := database.GetDB()
	database.SetDB(db)
	t.Cleanup(func() { database.SetDB(oldDB) })

	executionID := "00000000-0000-0000-0000-000000000701"
	run := WorkflowRunLog{
		ID: "00000000-0000-0000-0000-000000000711", TenantID: "00000000-0000-0000-0000-000000000721",
		AgentID: "00000000-0000-0000-0000-000000000731", WorkflowID: "00000000-0000-0000-0000-000000000741",
		Type: dto.WorkflowTypeChat, TriggeredFrom: "debugging", Version: "draft", Status: dto.WorkflowRunStatusRunning,
		CreatedByRole: CreatedByRoleAccount, CreatedBy: "00000000-0000-0000-0000-000000000751",
		RuntimeProtocolVersion: 2, ExecutionGeneration: 1, ActiveExecutionID: &executionID,
	}
	if err := db.Create(&run).Error; err != nil {
		t.Fatalf("create workflow run: %v", err)
	}
	ctx := withWorkflowExecutionOwner(context.Background(), workflowExecutionOwner{
		WorkflowRunID: run.ID, ExecutionID: executionID, Generation: 1,
	})

	nodeExecutionIDs := []string{"child-execution-0", "child-execution-1"}
	for round, nodeExecutionID := range nodeExecutionIDs {
		data := map[string]interface{}{
			"id": nodeExecutionID, "node_id": "child-node", "node_type": "llm", "title": "Child",
			"loop_id": "loop-node", "loop_index": round, "status": "running",
		}
		if _, err := appendWorkflowRunEventPayloadResult(ctx, run.TenantID, run.AgentID, run.ID, workflowpause.EventNodeStarted, data); err != nil {
			t.Fatalf("append round %d node start: %v", round, err)
		}
		data["status"] = "succeeded"
		data["outputs"] = map[string]interface{}{"usage": map[string]interface{}{"TotalTokens": 10 + round}}
		if _, err := appendWorkflowRunEventPayloadResult(ctx, run.TenantID, run.AgentID, run.ID, workflowpause.EventNodeFinished, data); err != nil {
			t.Fatalf("append round %d node finish: %v", round, err)
		}
	}

	var events []workflowpause.RunEvent
	if err := db.Where("workflow_run_id = ?", run.ID).Order("sequence ASC").Find(&events).Error; err != nil {
		t.Fatalf("load node events: %v", err)
	}
	if len(events) != 4 {
		t.Fatalf("node event count = %d, want 4", len(events))
	}
	for index, event := range events {
		if event.Sequence != index+1 {
			t.Fatalf("event %d sequence = %d, want %d", index, event.Sequence, index+1)
		}
		var data map[string]interface{}
		if err := json.Unmarshal([]byte(event.EventData), &data); err != nil {
			t.Fatalf("decode event %d data: %v", index, err)
		}
		wantExecutionID := nodeExecutionIDs[index/2]
		if got := workflowEventString(data["node_execution_id"]); got != wantExecutionID {
			t.Fatalf("event %d node_execution_id = %q, want %q", index, got, wantExecutionID)
		}
	}

	var logs []WorkflowNodeRuntimeLog
	if err := db.Where("workflow_run_id = ?", run.ID).Order("round_index ASC").Find(&logs).Error; err != nil {
		t.Fatalf("load internal node projections: %v", err)
	}
	if len(logs) != 2 {
		t.Fatalf("internal node projection count = %d, want 2", len(logs))
	}
	for round, log := range logs {
		wantStart := int64(round*2 + 1)
		wantFinish := wantStart + 1
		if log.StartedEventSequence == nil || *log.StartedEventSequence != wantStart || log.FinishedEventSequence == nil || *log.FinishedEventSequence != wantFinish {
			t.Fatalf("round %d event sequences = start %v finish %v, want %d/%d", round, log.StartedEventSequence, log.FinishedEventSequence, wantStart, wantFinish)
		}
	}
}

func TestWorkflowRunEventDispatcherDoesNotPublishStaleOwnerEvent(t *testing.T) {
	db := openWorkflowStopV2TestDB(t)
	if err := db.AutoMigrate(&workflowStopTestRun{}, &workflowpause.RunEvent{}); err != nil {
		t.Fatalf("migrate event tables: %v", err)
	}
	oldDB := database.GetDB()
	database.SetDB(db)
	t.Cleanup(func() { database.SetDB(oldDB) })

	activeExecutionID := "00000000-0000-0000-0000-000000000081"
	run := workflowStopTestRun{
		ID: "00000000-0000-0000-0000-000000000181", TenantID: "00000000-0000-0000-0000-000000000281",
		AgentID: "00000000-0000-0000-0000-000000000381", WorkflowID: "00000000-0000-0000-0000-000000000481",
		Status: "running", RuntimeProtocolVersion: 2, ExecutionGeneration: 2,
		ActiveExecutionID: &activeExecutionID,
	}
	if err := db.Create(&run).Error; err != nil {
		t.Fatalf("create owned run: %v", err)
	}
	emitted := 0
	dispatcher := newWorkflowRunEventDispatcher(run.TenantID, run.AgentID, run.ID, false, func(string, map[string]interface{}, *workflowpause.RunEventPayload) error {
		emitted++
		return nil
	})
	t.Cleanup(func() { _ = dispatcher.Close(context.Background()) })
	staleCtx := withWorkflowExecutionOwner(context.Background(), workflowExecutionOwner{
		WorkflowRunID: run.ID,
		ExecutionID:   "00000000-0000-0000-0000-000000000082",
		Generation:    1,
	})
	err := dispatcher.Dispatch(staleCtx, workflowpause.EventNodeStarted, map[string]interface{}{
		"node_id": "node-1", "node_execution_id": "node-execution-1",
	})
	if !errors.Is(err, workflowpause.ErrExecutionOwnershipLost) {
		t.Fatalf("dispatch error = %v, want ownership lost", err)
	}
	if emitted != 0 {
		t.Fatalf("emitted events = %d, want 0", emitted)
	}
	var eventCount int64
	if err := db.Model(&workflowpause.RunEvent{}).Where("workflow_run_id = ?", run.ID).Count(&eventCount).Error; err != nil {
		t.Fatalf("count stale events: %v", err)
	}
	if eventCount != 0 {
		t.Fatalf("stale event count = %d, want 0", eventCount)
	}
}

func TestWorkflowRunEventDispatcherPreservesCommittedEnvelope(t *testing.T) {
	ctx := t.Context()
	stored := &workflowpause.RunEventPayload{
		EventID: "event-1", Sequence: 7, Event: workflowpause.EventApprovalRequested,
		Category: workflowpause.EventCategoryInteraction, SchemaVersion: 2, PayloadVersion: 1,
		ExecutionID: "execution-1", PauseID: "pause-1", CreatedAt: 100, OccurredAtMS: 101000, RecordedAtMS: 102000,
	}
	var received *workflowpause.RunEventPayload
	dispatcher := &workflowRunEventDispatcher{
		onEvent: func(_ string, data map[string]interface{}, event *workflowpause.RunEventPayload) error {
			if _, exists := data["__stored_event_payload"]; exists {
				t.Fatal("internal stored payload was exposed")
			}
			received = event
			return nil
		},
		containers: map[string]workflowRunContainerState{},
	}
	if err := dispatcher.Dispatch(ctx, workflowpause.EventApprovalRequested, map[string]interface{}{
		"node_id": "approval-1", "__stored_sequence": stored.Sequence,
		"__stored_event_id": stored.EventID, "__stored_event_payload": stored,
	}); err != nil {
		t.Fatal(err)
	}
	if received == nil || received.EventID != stored.EventID || received.Sequence != stored.Sequence ||
		received.ExecutionID != stored.ExecutionID || received.PauseID != stored.PauseID || received.RecordedAtMS != stored.RecordedAtMS {
		t.Fatalf("received envelope = %#v, want committed envelope %#v", received, stored)
	}
}

func TestWorkflowRunEventDispatcherLiveAndReplayUseSameDurableOrder(t *testing.T) {
	db := openWorkflowStopV2TestDB(t)
	if err := db.AutoMigrate(&workflowStopTestRun{}, &workflowpause.RunEvent{}); err != nil {
		t.Fatalf("migrate event tables: %v", err)
	}
	oldDB := database.GetDB()
	database.SetDB(db)
	t.Cleanup(func() { database.SetDB(oldDB) })

	executionID := "00000000-0000-0000-0000-000000000091"
	run := workflowStopTestRun{
		ID: "00000000-0000-0000-0000-000000000191", TenantID: "00000000-0000-0000-0000-000000000291",
		AgentID: "00000000-0000-0000-0000-000000000391", WorkflowID: "00000000-0000-0000-0000-000000000491",
		Status: "running", RuntimeProtocolVersion: 2, ExecutionGeneration: 1,
		ActiveExecutionID: &executionID,
	}
	if err := db.Create(&run).Error; err != nil {
		t.Fatalf("create owned run: %v", err)
	}
	ctx := withWorkflowExecutionOwner(context.Background(), workflowExecutionOwner{
		WorkflowRunID: run.ID, ExecutionID: executionID, Generation: 1,
	})
	live := make([]string, 0, 3)
	dispatcher := newWorkflowRunEventDispatcher(run.TenantID, run.AgentID, run.ID, false, func(eventType string, _ map[string]interface{}, _ *workflowpause.RunEventPayload) error {
		live = append(live, eventType)
		return nil
	})
	t.Cleanup(func() { _ = dispatcher.Close(context.Background()) })
	if err := dispatcher.Dispatch(ctx, "child_progress", map[string]interface{}{
		"node_id": "child-1", "node_execution_id": "child-execution-1", "loop_id": "loop-1", "loop_index": 0,
	}); err != nil {
		t.Fatalf("buffer child event: %v", err)
	}
	if err := dispatcher.Dispatch(ctx, "loop_started", map[string]interface{}{"node_id": "loop-1"}); err != nil {
		t.Fatalf("dispatch loop start: %v", err)
	}
	if err := dispatcher.Dispatch(ctx, "loop_next", map[string]interface{}{"node_id": "loop-1", "index": 1}); err != nil {
		t.Fatalf("dispatch loop round: %v", err)
	}
	want := []string{"loop_started", "loop_next", "child_progress"}
	if len(live) != len(want) {
		t.Fatalf("live events = %v, want %v", live, want)
	}
	var persisted []workflowpause.RunEvent
	if err := db.Where("workflow_run_id = ?", run.ID).Order("sequence ASC").Find(&persisted).Error; err != nil {
		t.Fatalf("load durable events: %v", err)
	}
	if len(persisted) != len(want) {
		t.Fatalf("durable events = %v, want %v", persisted, want)
	}
	for index := range want {
		if live[index] != want[index] || persisted[index].EventType != want[index] || persisted[index].Sequence != index+1 {
			t.Fatalf("event %d live=%q durable=%q sequence=%d want=%q", index, live[index], persisted[index].EventType, persisted[index].Sequence, want[index])
		}
	}
}

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

func TestWorkflowRunEventDispatcherFlushesFinalContainerRoundBeforeCompletion(t *testing.T) {
	tests := []struct {
		name           string
		startedEvent   string
		completedEvent string
		containerKey   string
		indexKey       string
	}{
		{
			name:           "loop",
			startedEvent:   "loop_started",
			completedEvent: "loop_completed",
			containerKey:   "loop_id",
			indexKey:       "loop_index",
		},
		{
			name:           "iteration",
			startedEvent:   "iteration_started",
			completedEvent: "iteration_completed",
			containerKey:   "iteration_id",
			indexKey:       "iteration_index",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := t.Context()
			events := make([]string, 0, 4)
			dispatcher := &workflowRunEventDispatcher{
				onEvent: func(eventType string, data map[string]interface{}, _ *workflowpause.RunEventPayload) error {
					events = append(events, eventType)
					return nil
				},
				containers: map[string]workflowRunContainerState{},
			}

			dispatcher.Dispatch(ctx, tt.startedEvent, map[string]interface{}{
				"node_id": "container-1",
			})
			dispatcher.Dispatch(ctx, "node_started", map[string]interface{}{
				"node_id":       "child-1",
				tt.containerKey: "container-1",
				tt.indexKey:     0,
			})
			dispatcher.Dispatch(ctx, "node_finished", map[string]interface{}{
				"node_id":       "child-1",
				tt.containerKey: "container-1",
				tt.indexKey:     0,
			})
			if len(events) != 1 {
				t.Fatalf("events before completion = %#v, want only %q", events, tt.startedEvent)
			}

			dispatcher.Dispatch(ctx, tt.completedEvent, map[string]interface{}{
				"node_id": "container-1",
				"steps":   1,
			})

			want := []string{tt.startedEvent, "node_started", "node_finished", tt.completedEvent}
			if len(events) != len(want) {
				t.Fatalf("events after completion = %#v, want %#v", events, want)
			}
			for index := range want {
				if events[index] != want[index] {
					t.Fatalf("events after completion = %#v, want %#v", events, want)
				}
			}
			if len(dispatcher.pending) != 0 {
				t.Fatalf("pending events after completion = %d, want 0", len(dispatcher.pending))
			}
		})
	}
}

func TestWorkflowRunEventDispatcherKeepsOutOfRangeContainerChildrenBuffered(t *testing.T) {
	ctx := t.Context()
	events := make([]string, 0, 3)
	dispatcher := &workflowRunEventDispatcher{
		onEvent: func(eventType string, data map[string]interface{}, _ *workflowpause.RunEventPayload) error {
			events = append(events, eventType)
			return nil
		},
		containers: map[string]workflowRunContainerState{},
	}

	dispatcher.Dispatch(ctx, "loop_started", map[string]interface{}{
		"node_id": "loop-1",
	})
	dispatcher.Dispatch(ctx, "node_started", map[string]interface{}{
		"node_id":    "unmatched-child",
		"loop_id":    "loop-1",
		"loop_index": 9,
	})
	dispatcher.Dispatch(ctx, "loop_completed", map[string]interface{}{
		"node_id": "loop-1",
		"steps":   2,
	})

	want := []string{"loop_started", "loop_completed"}
	if len(events) != len(want) || events[0] != want[0] || events[1] != want[1] {
		t.Fatalf("events = %#v, want %#v", events, want)
	}
	if len(dispatcher.pending) != 1 {
		t.Fatalf("pending events = %d, want 1 out-of-range child", len(dispatcher.pending))
	}
}

func TestWorkflowRunEventDispatcherFlushesUnmatchedPendingAfterTerminal(t *testing.T) {
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

	if len(events) != 3 || events[0] != "loop_started" || events[1] != "node_started" || events[2] != "workflow_finished" {
		t.Fatalf("events = %#v, want unmatched child preserved before terminal", events)
	}
}
