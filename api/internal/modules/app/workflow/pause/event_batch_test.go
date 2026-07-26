package pause

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

func TestAppendEventBatchAllocatesOneContinuousRangeAndDeduplicates(t *testing.T) {
	db := openPauseServiceTestDB(t)
	executionID := "execution-batch-1"
	run := pauseTestWorkflowRun{
		ID: "run-event-batch", RuntimeProtocolVersion: 2, ExecutionGeneration: 3,
		ActiveExecutionID: &executionID, Status: "running",
	}
	if err := db.Create(&run).Error; err != nil {
		t.Fatal(err)
	}
	service := NewService(db)
	request := AppendEventBatchRequest{
		TenantID: "tenant-1", AppID: "app-1", WorkflowRunID: run.ID,
		Fence: RuntimeFence{ExpectedExecutionID: executionID, ExpectedExecutionGeneration: 3},
		Events: []EventDraft{
			{EventType: EventNodeStarted, IdempotencyKey: "node:a:started", EventData: map[string]interface{}{"node_id": "a"}},
			{EventType: EventNodeFinished, IdempotencyKey: "node:a:finished", EventData: map[string]interface{}{"node_id": "a"}},
			{EventType: EventNodeFinished, IdempotencyKey: "node:a:finished", EventData: map[string]interface{}{"node_id": "a"}},
		},
	}
	stored, err := service.AppendEventBatch(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if len(stored) != 3 {
		t.Fatalf("stored event count = %d, want 3", len(stored))
	}
	if stored[0].Payload.Sequence != 1 || stored[1].Payload.Sequence != 2 || stored[2].Payload.Sequence != 2 {
		t.Fatalf("sequences = %d,%d,%d, want 1,2,2", stored[0].Payload.Sequence, stored[1].Payload.Sequence, stored[2].Payload.Sequence)
	}
	if !stored[0].Inserted || !stored[1].Inserted || stored[2].Inserted {
		t.Fatalf("insert flags = %v,%v,%v", stored[0].Inserted, stored[1].Inserted, stored[2].Inserted)
	}
	var persisted int64
	if err := db.Model(&RunEvent{}).Where("workflow_run_id = ?", run.ID).Count(&persisted).Error; err != nil {
		t.Fatal(err)
	}
	if persisted != 2 {
		t.Fatalf("persisted events = %d, want 2", persisted)
	}
	if err := db.First(&run, "id = ?", run.ID).Error; err != nil {
		t.Fatal(err)
	}
	if run.NextEventSequence != 2 {
		t.Fatalf("next sequence = %d, want 2", run.NextEventSequence)
	}

	replayed, err := service.AppendEventBatch(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	for index, event := range replayed {
		if event.Inserted {
			t.Fatalf("replayed event %d was inserted", index)
		}
	}
}

func TestAppendEventBatchRejectsStaleExecutionOwner(t *testing.T) {
	db := openPauseServiceTestDB(t)
	activeExecutionID := "execution-current"
	run := pauseTestWorkflowRun{
		ID: "run-event-batch-fence", RuntimeProtocolVersion: 2, ExecutionGeneration: 4,
		ActiveExecutionID: &activeExecutionID, Status: "running",
	}
	if err := db.Create(&run).Error; err != nil {
		t.Fatal(err)
	}
	_, err := NewService(db).AppendEventBatch(context.Background(), AppendEventBatchRequest{
		TenantID: "tenant-1", AppID: "app-1", WorkflowRunID: run.ID,
		Fence:  RuntimeFence{ExpectedExecutionID: "execution-stale", ExpectedExecutionGeneration: 3},
		Events: []EventDraft{{EventType: EventNodeStarted, EventData: map[string]interface{}{"node_id": "a"}}},
	})
	if !errors.Is(err, ErrExecutionOwnershipLost) {
		t.Fatalf("error = %v, want execution ownership lost", err)
	}
	var count int64
	if err := db.Model(&RunEvent{}).Where("workflow_run_id = ?", run.ID).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("event count = %d, want 0", count)
	}
}

func TestAppendEventBatchSplitsOversizedBatch(t *testing.T) {
	db := openPauseServiceTestDB(t)
	run := pauseTestWorkflowRun{ID: "run-oversized", RuntimeProtocolVersion: 2, ExecutionGeneration: 1, Status: "running"}
	if err := db.Create(&run).Error; err != nil {
		t.Fatal(err)
	}
	events := make([]EventDraft, maximumEventBatchCount+1)
	for index := range events {
		events[index] = EventDraft{EventType: EventNodeStarted, EventData: map[string]interface{}{"index": index}, IdempotencyKey: fmt.Sprintf("event-%d", index)}
	}
	stored, err := NewService(db).AppendEventBatch(context.Background(), AppendEventBatchRequest{
		TenantID: "tenant-1", AppID: "app-1", WorkflowRunID: run.ID, Events: events,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(stored) != len(events) || stored[len(stored)-1].Payload.Sequence != len(events) {
		t.Fatalf("stored batch = %d last sequence = %d", len(stored), stored[len(stored)-1].Payload.Sequence)
	}
}
