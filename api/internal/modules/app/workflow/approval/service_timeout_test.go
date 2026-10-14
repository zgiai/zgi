package approval

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	workflowpause "github.com/zgiai/zgi/api/internal/modules/app/workflow/pause"
	"github.com/zgiai/zgi/api/internal/testutil/testdb"
	"gorm.io/gorm"
)

const (
	timeoutTenantID = "11111111-1111-1111-1111-111111111111"
	timeoutAppID    = "22222222-2222-2222-2222-222222222222"
	timeoutRunID    = "timeout-run"
	timeoutPauseID  = "33333333-3333-3333-3333-333333333333"
	timeoutFormID   = "44444444-4444-4444-4444-444444444444"
)

// newTimeoutTestDB holds one expired approval form. withPause adds the durable
// pause that waits on it, and withQuestion a second pending reason.
func newTimeoutTestDB(t *testing.T, protocolVersion int, withPause, withQuestion bool) *gorm.DB {
	t.Helper()
	db := newApprovalTestDB(t)
	if err := testdb.AutoMigrate(db,
		&approvalWorkflowRunScope{},
		&workflowpause.RunPause{},
		&workflowpause.RunPauseReason{},
		&workflowpause.RunEvent{},
		&workflowpause.RuntimeOutbox{},
	); err != nil {
		t.Fatalf("migrate workflow pause tables: %v", err)
	}
	now := time.Now()
	definition, err := json.Marshal(FormDefinition{Content: "Approve the request", Actions: []Action{{ID: "approve", Label: "Approve"}}})
	if err != nil {
		t.Fatal(err)
	}
	records := []interface{}{
		&approvalWorkflowRunScope{ID: timeoutRunID, RuntimeProtocolVersion: protocolVersion, NextEventSequence: 2, ExecutionGeneration: 1},
		&Form{
			ID: timeoutFormID, TenantID: timeoutTenantID, AppID: timeoutAppID, WorkflowRunID: timeoutRunID,
			NodeID: "approval-node", NodeTitle: "Approval", AccessToken: "timeout-token",
			FormDefinition: string(definition), RenderedContent: "Approve the request",
			Status: FormStatusWaiting, ExpirationTime: now.Add(-time.Minute),
		},
	}
	if withPause {
		records = append(records,
			&workflowpause.RunPause{
				ID: timeoutPauseID, TenantID: timeoutTenantID, AppID: timeoutAppID, WorkflowRunID: timeoutRunID,
				NodeID: "approval-node", Reason: "approval", StateJSON: `{}`, Generation: 1, Status: workflowpause.RunPauseStatusPaused,
			},
			&workflowpause.RunPauseReason{
				ID: "55555555-5555-5555-5555-555555555555", PauseID: timeoutPauseID, Type: workflowpause.ReasonTypeApprovalRequired,
				NodeID: "approval-node", FormID: timeoutFormID, Status: workflowpause.RunPauseReasonStatusPending, CreatedAt: now,
			},
		)
	}
	if withQuestion {
		pauseID, generation := timeoutPauseID, int64(1)
		requestKey, pausedKey := "question-requested", "workflow-paused"
		records = append(records,
			&workflowpause.RunPauseReason{
				ID: "66666666-6666-6666-6666-666666666666", PauseID: timeoutPauseID, Type: workflowpause.ReasonTypeQuestionAnswerRequired,
				NodeID: "question-node", Status: workflowpause.RunPauseReasonStatusPending, CreatedAt: now.Add(time.Millisecond),
			},
			&workflowpause.RunEvent{
				ID: "88888888-8888-8888-8888-888888888888", TenantID: timeoutTenantID, AppID: timeoutAppID,
				WorkflowRunID: timeoutRunID, Sequence: 1, EventType: workflowpause.EventQuestionAnswerRequested,
				EventData:     `{"node_id":"question-node","question":"Which option?","choices":["one","two"]}`,
				SchemaVersion: 2, Category: workflowpause.EventCategoryInteraction,
				PauseID: &pauseID, PauseGeneration: &generation, IdempotencyKey: &requestKey, OccurredAt: now,
			},
			&workflowpause.RunEvent{
				ID: "99999999-9999-9999-9999-999999999999", TenantID: timeoutTenantID, AppID: timeoutAppID,
				WorkflowRunID: timeoutRunID, Sequence: 2, EventType: workflowpause.EventWorkflowPaused,
				EventData:     `{"id":"timeout-run","status":"paused","paused_nodes":["approval-node","question-node"]}`,
				SchemaVersion: 2, Category: workflowpause.EventCategoryControl,
				PauseID: &pauseID, PauseGeneration: &generation, IdempotencyKey: &pausedKey, OccurredAt: now,
			},
		)
	}
	for _, record := range records {
		if err := db.Create(record).Error; err != nil {
			t.Fatalf("create timeout fixture %T: %v", record, err)
		}
	}
	return db
}

func TestTimeoutCompletesTheDurablePauseReasonAndQueuesItsResume(t *testing.T) {
	db := newTimeoutTestDB(t, 2, true, false)
	results, err := NewService(db).TimeoutExpiredForms(context.Background(), 10)
	if err != nil {
		t.Fatalf("TimeoutExpiredForms() error = %v", err)
	}
	if len(results) != 1 || results[0].Form.Status != FormStatusTimeout || results[0].LegacyResume {
		t.Fatalf("results = %#v, want one timed-out form of a durable pause", results)
	}
	if results[0].Outbox == nil {
		t.Fatal("timed-out approval did not queue its workflow resume")
	}
	var reason workflowpause.RunPauseReason
	if err := db.First(&reason, "pause_id = ? AND form_id = ?", timeoutPauseID, timeoutFormID).Error; err != nil || reason.Status != workflowpause.RunPauseReasonStatusCompleted {
		t.Fatalf("pause reason = %#v, err %v; want it completed so the pause can resume", reason, err)
	}
	var event workflowpause.RunEvent
	if err := db.Select("event_type", "event_data").First(&event, "workflow_run_id = ? AND event_type = ?", timeoutRunID, workflowpause.EventApprovalExpired).Error; err != nil {
		t.Fatalf("approval_expired event missing: %v", err)
	}
	var data map[string]interface{}
	if err := json.Unmarshal([]byte(event.EventData), &data); err != nil || data["form_id"] != timeoutFormID || data["reason"] != FormStatusTimeout {
		t.Fatalf("approval_expired data = %#v, err %v", data, err)
	}

	again, err := NewService(db).TimeoutExpiredForms(context.Background(), 10)
	if err != nil || len(again) != 0 {
		t.Fatalf("second scan = %#v, err %v; want the timed-out form left alone", again, err)
	}
}

func TestTimeoutWaitsForTheOtherReasonsOfTheSamePause(t *testing.T) {
	db := newTimeoutTestDB(t, 2, true, true)
	results, err := NewService(db).TimeoutExpiredForms(context.Background(), 10)
	if err != nil {
		t.Fatalf("TimeoutExpiredForms() error = %v", err)
	}
	if len(results) != 1 || results[0].Outbox != nil || results[0].LegacyResume {
		t.Fatalf("results = %#v, want the form timed out but the pause still waiting on its question", results)
	}
	var reason workflowpause.RunPauseReason
	if err := db.First(&reason, "pause_id = ? AND form_id = ?", timeoutPauseID, timeoutFormID).Error; err != nil || reason.Status != workflowpause.RunPauseReasonStatusCompleted {
		t.Fatalf("approval reason = %#v, err %v; want it completed", reason, err)
	}
}

func TestTimeoutResumesAWorkflowWithoutADurablePauseDirectly(t *testing.T) {
	results, err := NewService(newTimeoutTestDB(t, 1, false, false)).TimeoutExpiredForms(context.Background(), 10)
	if err != nil {
		t.Fatalf("TimeoutExpiredForms() error = %v", err)
	}
	if len(results) != 1 || !results[0].LegacyResume || results[0].Outbox != nil {
		t.Fatalf("results = %#v, want a direct resume of the legacy workflow", results)
	}
	durable, err := NewService(newTimeoutTestDB(t, 2, false, false)).TimeoutExpiredForms(context.Background(), 10)
	if err != nil || len(durable) != 1 || durable[0].LegacyResume || durable[0].Outbox != nil {
		t.Fatalf("durable run without its pause = %#v, err %v; want nothing to resume", durable, err)
	}
}

func TestTimeoutLeavesASubmittedFormAlone(t *testing.T) {
	db := newTimeoutTestDB(t, 2, true, false)
	if err := db.Model(&Form{}).Where("id = ?", timeoutFormID).Update("status", FormStatusSubmitted).Error; err != nil {
		t.Fatal(err)
	}
	result, err := NewService(db).MarkTimedOut(context.Background(), timeoutFormID)
	if err != nil || result.TimedOut || result.Outbox != nil || result.Form.Status != FormStatusSubmitted {
		t.Fatalf("MarkTimedOut() = %#v, %v; want the submitted form unchanged", result, err)
	}
}
