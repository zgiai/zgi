package workflow

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	approvalruntime "github.com/zgiai/zgi/api/internal/modules/app/workflow/approval"
	workflowpause "github.com/zgiai/zgi/api/internal/modules/app/workflow/pause"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type mixedPauseRunScope struct {
	ID                     string  `gorm:"primaryKey"`
	RuntimeProtocolVersion int     `gorm:"not null"`
	NextEventSequence      int     `gorm:"not null"`
	ExecutionGeneration    int64   `gorm:"not null"`
	ActiveExecutionID      *string `gorm:"type:uuid"`
	DeletedAt              gorm.DeletedAt
}

func (mixedPauseRunScope) TableName() string { return "workflow_run_logs" }

// A pause holds an approval (its first node) and a question. The question is
// answered first, then the approval times out: the queued question resume
// must find the pause ready and resume it without submitting the answer again,
// with the recorded answer in its durable inputs.
func TestQueuedQuestionResumeAfterTheApprovalOfAMixedPauseTimesOut(t *testing.T) {
	dsn := "file:" + strings.ReplaceAll(t.Name(), "/", "_") + "?mode=memory&cache=shared"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		if strings.Contains(err.Error(), "requires cgo") {
			t.Skipf("sqlite driver unavailable without cgo: %v", err)
		}
		t.Fatalf("open sqlite: %v", err)
	}
	if err := autoMigrateWorkflowTest(db, &approvalruntime.Form{}, &mixedPauseRunScope{},
		&workflowpause.RunPause{}, &workflowpause.RunPauseReason{}, &workflowpause.RunEvent{}, &workflowpause.RuntimeOutbox{}); err != nil {
		t.Fatalf("migrate mixed pause tables: %v", err)
	}
	const (
		tenantID = "11111111-1111-1111-1111-111111111111"
		appID    = "22222222-2222-2222-2222-222222222222"
		runID    = "mixed-run"
		pauseID  = "33333333-3333-3333-3333-333333333333"
		formID   = "44444444-4444-4444-4444-444444444444"
	)
	now := time.Now()
	generation := int64(1)
	pauseIDValue := pauseID
	approvalKey, questionKey := "approval-requested", "question-requested"
	definition, err := json.Marshal(approvalruntime.FormDefinition{Content: "Approve", Actions: []approvalruntime.Action{{ID: "approve", Label: "Approve"}}})
	if err != nil {
		t.Fatal(err)
	}
	for _, record := range []interface{}{
		&mixedPauseRunScope{ID: runID, RuntimeProtocolVersion: 2, NextEventSequence: 2, ExecutionGeneration: 1},
		&approvalruntime.Form{
			ID: formID, TenantID: tenantID, AppID: appID, WorkflowRunID: runID, NodeID: "approval-node", NodeTitle: "Approval",
			AccessToken: "mixed-token", FormDefinition: string(definition), Status: approvalruntime.FormStatusWaiting, ExpirationTime: now.Add(-time.Minute),
		},
		&workflowpause.RunPause{ID: pauseID, TenantID: tenantID, AppID: appID, WorkflowRunID: runID, NodeID: "approval-node",
			Reason: "mixed", StateJSON: `{"executor_state":{"paused_node_id":"approval-node"}}`, Generation: generation, Status: workflowpause.RunPauseStatusPaused},
		&workflowpause.RunPauseReason{ID: "55555555-5555-5555-5555-555555555555", PauseID: pauseID, Type: workflowpause.ReasonTypeApprovalRequired,
			NodeID: "approval-node", FormID: formID, Status: workflowpause.RunPauseReasonStatusPending, CreatedAt: now},
		&workflowpause.RunPauseReason{ID: "66666666-6666-6666-6666-666666666666", PauseID: pauseID, Type: workflowpause.ReasonTypeQuestionAnswerRequired,
			NodeID: "question-node", Status: workflowpause.RunPauseReasonStatusPending, CreatedAt: now.Add(time.Millisecond)},
		&workflowpause.RunEvent{ID: "77777777-7777-7777-7777-777777777777", TenantID: tenantID, AppID: appID, WorkflowRunID: runID, Sequence: 1,
			EventType: workflowpause.EventApprovalRequested, EventData: `{"node_id":"approval-node","form_id":"` + formID + `"}`, SchemaVersion: 2,
			Category: workflowpause.EventCategoryInteraction, PauseID: &pauseIDValue, PauseGeneration: &generation, IdempotencyKey: &approvalKey, OccurredAt: now},
		&workflowpause.RunEvent{ID: "88888888-8888-8888-8888-888888888888", TenantID: tenantID, AppID: appID, WorkflowRunID: runID, Sequence: 2,
			EventType: workflowpause.EventQuestionAnswerRequested, EventData: `{"node_id":"question-node","question":"Which budget?"}`, SchemaVersion: 2,
			Category: workflowpause.EventCategoryInteraction, PauseID: &pauseIDValue, PauseGeneration: &generation, IdempotencyKey: &questionKey, OccurredAt: now},
	} {
		if err := db.Create(record).Error; err != nil {
			t.Fatalf("create mixed pause fixture %T: %v", record, err)
		}
	}
	ctx := context.Background()
	pauseService := workflowpause.NewService(db)
	answer, err := pauseService.SubmitInteraction(ctx, runID, pauseID, "question-answer", workflowpause.EventQuestionAnswerSubmitted,
		map[string]interface{}{"node_id": "question-node", "answer": "Q4"}, "question-answer:mixed")
	if err != nil || answer.ResumeReady {
		t.Fatalf("answer question: %#v, %v; want the pause still waiting on its approval", answer, err)
	}

	timedOut, err := approvalruntime.NewService(db).MarkTimedOut(ctx, formID)
	if err != nil || timedOut.Outbox == nil {
		t.Fatalf("MarkTimedOut() = %#v, %v; want the pause resume queued", timedOut, err)
	}
	pause, _, _, err := pauseService.GetActiveByWorkflowRunID(ctx, runID)
	if err != nil {
		t.Fatalf("load pause: %v", err)
	}
	if !questionResumeAlreadyRecorded(pause) {
		t.Fatalf("pause status = %q; the queued question resume would submit the answer again", pause.Status)
	}
	inputs, err := loadDurableWorkflowResumeInputs(ctx, pauseService, runID, pause, map[string]interface{}{})
	if err != nil {
		t.Fatalf("load durable resume inputs: %v", err)
	}
	encoded, _ := json.Marshal(inputs)
	if !strings.Contains(string(encoded), `"answer":"Q4"`) || !strings.Contains(string(encoded), `"question-node"`) {
		t.Fatalf("durable resume inputs = %s, want the recorded answer of the question node", encoded)
	}
}
