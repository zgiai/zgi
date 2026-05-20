package workflow_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	graphentities "github.com/zgiai/ginext/internal/modules/app/workflow/graph_engine/entities"
	workflowpause "github.com/zgiai/ginext/internal/modules/app/workflow/pause"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestPauseServiceSaveAndLoadVersionedState(t *testing.T) {
	ctx := context.Background()
	db := newPauseServiceTestDB(t)
	service := workflowpause.NewService(db)

	pool := graphentities.NewVariablePool()
	pool.SystemVariables.WorkflowRunID = "run-" + uuid.NewString()
	pool.SystemVariables.AppID = uuid.NewString()
	pool.UserInputs["test"] = "input"
	pool.Add([]string{"start", "test"}, "input")
	pool.Add([]string{"node-1", "answer"}, map[string]interface{}{"value": "approved"})

	state := workflowpause.State{
		Version:       workflowpause.StateVersion,
		WorkflowRunID: pool.SystemVariables.WorkflowRunID,
		WorkflowID:    uuid.NewString(),
		AppID:         pool.SystemVariables.AppID,
		TenantID:      uuid.NewString(),
		RunType:       "WORKFLOW",
		TriggeredFrom: "debugging",
		Request: workflowpause.RequestState{
			Inputs:       map[string]interface{}{"test": "input"},
			ResponseMode: "streaming",
		},
		ExecutorState: workflowpause.ExecutorState{
			PausedNodeID:   "approval-1",
			NodeQueue:      []string{"answer-1"},
			CompletedNodes: map[string]bool{"start": true},
			FailedNodes:    map[string]string{},
			ExecutionOutputs: map[string]map[string]interface{}{
				"start": map[string]interface{}{"test": "input"},
			},
			AllNodeOutputs: map[string]interface{}{"test": "input"},
			NodeIndex:      2,
			TotalTokens:    3,
		},
		VariablePool: workflowpause.SnapshotVariablePool(pool),
	}

	pause, err := service.Save(ctx, workflowpause.SaveParams{
		TenantID:      state.TenantID,
		AppID:         state.AppID,
		WorkflowRunID: state.WorkflowRunID,
		NodeID:        state.ExecutorState.PausedNodeID,
		Reason:        workflowpause.ReasonTypeApprovalRequired,
		State:         state,
		Reasons: []workflowpause.Reason{
			{
				Type:   workflowpause.ReasonTypeApprovalRequired,
				NodeID: "approval-1",
				FormID: uuid.NewString(),
			},
		},
	})
	if err != nil {
		t.Fatalf("Save returned error: %v", err)
	}
	if pause.ID == "" {
		t.Fatal("expected pause id")
	}

	_, reasons, loaded, err := service.GetActiveByWorkflowRunID(ctx, state.WorkflowRunID)
	if err != nil {
		t.Fatalf("GetActiveByWorkflowRunID returned error: %v", err)
	}
	if loaded.Version != workflowpause.StateVersion {
		t.Fatalf("state version = %s, want %s", loaded.Version, workflowpause.StateVersion)
	}
	if loaded.ExecutorState.PausedNodeID != "approval-1" {
		t.Fatalf("paused node = %s, want approval-1", loaded.ExecutorState.PausedNodeID)
	}
	if len(reasons) != 1 {
		t.Fatalf("reasons = %d, want 1", len(reasons))
	}
	if reasons[0].Type != workflowpause.ReasonTypeApprovalRequired {
		t.Fatalf("reason type = %s, want approval_required", reasons[0].Type)
	}

	restoredPool := graphentities.NewVariablePool()
	workflowpause.RestoreVariablePoolSnapshot(restoredPool, loaded.VariablePool)
	if got := restoredPool.GetWithPath([]string{"node-1", "answer", "value"}); got == nil || got.ToObject() != "approved" {
		t.Fatalf("restored variable = %v, want approved", got)
	}
	if restoredPool.UserInputs["test"] != "input" {
		t.Fatalf("restored user input = %v, want input", restoredPool.UserInputs["test"])
	}
}

func TestPauseServiceSaveReplacesActiveReasons(t *testing.T) {
	ctx := context.Background()
	db := newPauseServiceTestDB(t)
	service := workflowpause.NewService(db)

	runID := "run-" + uuid.NewString()
	state := workflowpause.State{
		Version:       workflowpause.StateVersion,
		WorkflowRunID: runID,
		ExecutorState: workflowpause.ExecutorState{
			PausedNodeID: "approval-1",
		},
	}

	if _, err := service.Save(ctx, workflowpause.SaveParams{
		WorkflowRunID: runID,
		NodeID:        "approval-1",
		Reason:        workflowpause.ReasonTypeApprovalRequired,
		State:         state,
		Reasons: []workflowpause.Reason{
			{Type: workflowpause.ReasonTypeApprovalRequired, NodeID: "old", FormID: uuid.NewString()},
		},
	}); err != nil {
		t.Fatalf("first Save returned error: %v", err)
	}
	if _, err := service.Save(ctx, workflowpause.SaveParams{
		WorkflowRunID: runID,
		NodeID:        "approval-1",
		Reason:        workflowpause.ReasonTypeApprovalRequired,
		State:         state,
		Reasons: []workflowpause.Reason{
			{Type: workflowpause.ReasonTypeApprovalRequired, NodeID: "new", FormID: uuid.NewString()},
		},
	}); err != nil {
		t.Fatalf("second Save returned error: %v", err)
	}

	_, reasons, _, err := service.GetActiveByWorkflowRunID(ctx, runID)
	if err != nil {
		t.Fatalf("GetActiveByWorkflowRunID returned error: %v", err)
	}
	if len(reasons) != 1 {
		t.Fatalf("reasons = %d, want 1", len(reasons))
	}
	if reasons[0].NodeID != "new" {
		t.Fatalf("reason node = %s, want new", reasons[0].NodeID)
	}
}

func TestPauseServiceListEventsAfterSequence(t *testing.T) {
	ctx := context.Background()
	db := newPauseServiceTestDB(t)
	service := workflowpause.NewService(db)
	runID := "run-" + uuid.NewString()
	tenantID := uuid.NewString()
	appID := uuid.NewString()

	for _, eventType := range []string{
		workflowpause.EventWorkflowStarted,
		workflowpause.EventApprovalRequested,
		workflowpause.EventWorkflowPaused,
	} {
		if err := service.AppendEvent(ctx, workflowpause.AppendEventParams{
			TenantID:      tenantID,
			AppID:         appID,
			WorkflowRunID: runID,
			EventType:     eventType,
			EventData:     map[string]interface{}{"event": eventType},
		}); err != nil {
			t.Fatalf("AppendEvent(%s) returned error: %v", eventType, err)
		}
	}

	latest, err := service.LatestEventSequence(ctx, tenantID, runID)
	if err != nil {
		t.Fatalf("LatestEventSequence returned error: %v", err)
	}
	if latest != 3 {
		t.Fatalf("latest sequence = %d, want 3", latest)
	}

	payload, err := service.ListEvents(ctx, tenantID, runID, 1, 10)
	if err != nil {
		t.Fatalf("ListEvents returned error: %v", err)
	}
	if len(payload.Events) != 2 {
		t.Fatalf("events length = %d, want 2", len(payload.Events))
	}
	if payload.Events[0].Event != workflowpause.EventApprovalRequested {
		t.Fatalf("first event = %s, want approval_requested", payload.Events[0].Event)
	}
	if payload.Events[1].Sequence != 3 {
		t.Fatalf("second sequence = %d, want 3", payload.Events[1].Sequence)
	}
}

func newPauseServiceTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open("file:"+uuid.NewString()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(
		&workflowpause.RunPause{},
		&workflowpause.RunPauseReason{},
		&workflowpause.RunEvent{},
	); err != nil {
		t.Fatalf("auto migrate pause tables: %v", err)
	}
	return db
}
