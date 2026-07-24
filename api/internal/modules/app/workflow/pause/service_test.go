package pause

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type pauseTestWorkflowRun struct {
	ID                      string `gorm:"primaryKey"`
	TenantID                string
	AgentID                 string
	WorkflowID              string
	RuntimeProtocolVersion  int
	NextEventSequence       int64
	ExecutionGeneration     int64
	ActiveExecutionID       *string
	ExecutionLeaseExpiresAt *time.Time
	StateRevision           int64
	Status                  string
	Error                   *string
	ExceptionsCount         int
	FinishedAt              *time.Time
	ConversationID          *string
	DeletedAt               *time.Time
}

func (pauseTestWorkflowRun) TableName() string { return "workflow_run_logs" }

type pauseTestMessage struct {
	ID                  string `gorm:"primaryKey"`
	WorkflowRunID       string
	ConversationID      string
	Answer              string
	Status              string
	Error               *string
	ExecutionGeneration int64
	ProjectionRevision  int64
	UpdatedAt           time.Time
	DeletedAt           *time.Time
}

func (pauseTestMessage) TableName() string { return "agents_messages" }

type pauseTestConversation struct {
	ID                  string `gorm:"primaryKey"`
	RuntimeStatus       string
	ActiveWorkflowRunID *string
	RuntimeRevision     int64
	DialogueCount       int
}

func (pauseTestConversation) TableName() string { return "agents_conversations" }

func openPauseServiceTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := "file:" + strings.ReplaceAll(t.Name(), "/", "_") + "?mode=memory&cache=shared"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		if strings.Contains(err.Error(), "requires cgo") {
			t.Skipf("sqlite driver unavailable without cgo: %v", err)
		}
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&pauseTestWorkflowRun{}, &pauseTestMessage{}, &pauseTestConversation{}, &RunPause{}, &RunPauseReason{}, &RunEvent{}, &RuntimeOutbox{}); err != nil {
		t.Fatalf("migrate pause service tables: %v", err)
	}
	return db
}

func TestPauseTransactionPersistsFinalAnswerProjection(t *testing.T) {
	db := openPauseServiceTestDB(t)
	service := NewService(db)
	executionID := "00000000-0000-0000-0000-000000000011"
	run := pauseTestWorkflowRun{
		ID: "run-pause-answer", RuntimeProtocolVersion: 2, ExecutionGeneration: 1,
		ActiveExecutionID: &executionID, Status: "running",
	}
	if err := db.Create(&run).Error; err != nil {
		t.Fatalf("create run: %v", err)
	}
	message := pauseTestMessage{ID: "message-pause-answer", WorkflowRunID: run.ID, Status: "running", ExecutionGeneration: 1}
	if err := db.Create(&message).Error; err != nil {
		t.Fatalf("create message: %v", err)
	}

	pauseRecord, err := service.Save(context.Background(), SaveParams{
		TenantID: "tenant-1", AppID: "app-1", WorkflowRunID: run.ID, NodeID: "question-1",
		Reason:      ReasonTypeQuestionAnswerRequired,
		State:       State{Version: StateVersion, WorkflowRunID: run.ID},
		Reasons:     []Reason{{Type: ReasonTypeQuestionAnswerRequired, NodeID: "question-1"}},
		ExecutionID: executionID, Generation: 1,
		MessageStatus: "waiting_question", MessageAnswer: "partial answer", UpdateMessageAnswer: true,
	})
	if err != nil {
		t.Fatalf("save pause: %v", err)
	}
	if pauseRecord == nil || pauseRecord.Status != RunPauseStatusPaused || pauseRecord.WorkflowRunID != run.ID {
		t.Fatalf("returned pause = %#v, want committed paused record", pauseRecord)
	}
	var persisted pauseTestMessage
	if err := db.First(&persisted, "id = ?", message.ID).Error; err != nil {
		t.Fatalf("load message: %v", err)
	}
	if persisted.Answer != "partial answer" || persisted.Status != "waiting_question" {
		t.Fatalf("message projection = answer %q status %q", persisted.Answer, persisted.Status)
	}
	if persisted.ProjectionRevision != 1 {
		t.Fatalf("projection revision = %d, want 1", persisted.ProjectionRevision)
	}
}

func TestPauseTransactionRollsBackProjectionWhenDurableEventFails(t *testing.T) {
	db := openPauseServiceTestDB(t)
	service := NewService(db)
	executionID := "00000000-0000-0000-0000-000000000012"
	run := pauseTestWorkflowRun{
		ID: "run-pause-rollback", RuntimeProtocolVersion: 2, ExecutionGeneration: 1,
		ActiveExecutionID: &executionID, Status: "running",
	}
	if err := db.Create(&run).Error; err != nil {
		t.Fatalf("create run: %v", err)
	}
	message := pauseTestMessage{
		ID: "message-pause-rollback", WorkflowRunID: run.ID, Answer: "before", Status: "running", ExecutionGeneration: 1,
	}
	if err := db.Create(&message).Error; err != nil {
		t.Fatalf("create message: %v", err)
	}
	_, err := service.Save(context.Background(), SaveParams{
		TenantID: "tenant-1", AppID: "app-1", WorkflowRunID: run.ID, NodeID: "question-1",
		Reason: ReasonTypeQuestionAnswerRequired, State: State{Version: StateVersion, WorkflowRunID: run.ID},
		Reasons: []Reason{{Type: ReasonTypeQuestionAnswerRequired, NodeID: "question-1"}},
		Events: []AppendEventParams{{
			EventType: EventWorkflowPaused, IdempotencyKey: "pause:rollback",
			EventData: map[string]interface{}{"not_json": func() {}},
		}},
		ExecutionID: executionID, Generation: 1,
		MessageStatus: "waiting_question", MessageAnswer: "partial", UpdateMessageAnswer: true,
	})
	if err == nil {
		t.Fatal("save pause succeeded despite invalid durable event payload")
	}

	if err := db.First(&run, "id = ?", run.ID).Error; err != nil {
		t.Fatalf("reload run: %v", err)
	}
	if run.Status != "running" || run.ActiveExecutionID == nil || *run.ActiveExecutionID != executionID {
		t.Fatalf("run changed after rolled back pause: %+v", run)
	}
	if err := db.First(&message, "id = ?", message.ID).Error; err != nil {
		t.Fatalf("reload message: %v", err)
	}
	if message.Status != "running" || message.Answer != "before" || message.ProjectionRevision != 0 {
		t.Fatalf("message changed after rolled back pause: %+v", message)
	}
	var pauseCount int64
	if err := db.Model(&RunPause{}).Where("workflow_run_id = ?", run.ID).Count(&pauseCount).Error; err != nil {
		t.Fatalf("count rolled back pauses: %v", err)
	}
	if pauseCount != 0 {
		t.Fatalf("pause count after rollback = %d, want 0", pauseCount)
	}
}

func TestFinalizeExpiredExecutionsFailsOrphanExactlyOnce(t *testing.T) {
	db := openPauseServiceTestDB(t)
	service := NewService(db)
	executionID := "00000000-0000-0000-0000-000000000099"
	expiredAt := time.Now().Add(-time.Minute)
	run := pauseTestWorkflowRun{
		ID: "run-expired-owner", TenantID: "tenant-1", AgentID: "app-1", WorkflowID: "workflow-1",
		RuntimeProtocolVersion: 2, ExecutionGeneration: 3, ActiveExecutionID: &executionID,
		ExecutionLeaseExpiresAt: &expiredAt, Status: "running",
	}
	conversationID := "conversation-1"
	run.ConversationID = &conversationID
	if err := db.Create(&run).Error; err != nil {
		t.Fatalf("create expired run: %v", err)
	}
	message := pauseTestMessage{
		ID: "message-expired-owner", WorkflowRunID: run.ID, ConversationID: conversationID,
		Status: "running", ExecutionGeneration: 3,
	}
	if err := db.Create(&message).Error; err != nil {
		t.Fatalf("create expired run message: %v", err)
	}
	if err := db.Create(&pauseTestConversation{ID: message.ConversationID, RuntimeStatus: "running", ActiveWorkflowRunID: &run.ID}).Error; err != nil {
		t.Fatalf("create expired run conversation: %v", err)
	}

	finalized, err := service.FinalizeExpiredExecutions(context.Background(), time.Now().Add(-15*time.Second), 10)
	if err != nil {
		t.Fatalf("finalize expired executions: %v", err)
	}
	if len(finalized) != 1 || finalized[0] != run.ID {
		t.Fatalf("finalized runs = %v, want [%s]", finalized, run.ID)
	}
	if err := db.First(&run, "id = ?", run.ID).Error; err != nil {
		t.Fatalf("reload expired run: %v", err)
	}
	if run.Status != "failed" || run.ActiveExecutionID != nil || run.Error == nil || *run.Error != "workflow_execution_interrupted" {
		t.Fatalf("unexpected expired run projection: %+v", run)
	}
	if err := db.First(&message, "id = ?", message.ID).Error; err != nil {
		t.Fatalf("reload expired message: %v", err)
	}
	if message.Status != "error" || message.Error == nil || *message.Error != "workflow_execution_interrupted" {
		t.Fatalf("unexpected expired message projection: %+v", message)
	}
	var eventCount int64
	if err := db.Model(&RunEvent{}).Where("workflow_run_id = ?", run.ID).Count(&eventCount).Error; err != nil {
		t.Fatalf("count interrupted events: %v", err)
	}
	if eventCount != 3 {
		t.Fatalf("interrupted event count = %d, want 3", eventCount)
	}

	again, err := service.FinalizeExpiredExecutions(context.Background(), time.Now(), 10)
	if err != nil {
		t.Fatalf("repeat expired finalization: %v", err)
	}
	if len(again) != 0 {
		t.Fatalf("repeat finalized runs = %v, want none", again)
	}
	if err := db.Model(&RunEvent{}).Where("workflow_run_id = ?", run.ID).Count(&eventCount).Error; err != nil {
		t.Fatalf("recount interrupted events: %v", err)
	}
	if eventCount != 3 {
		t.Fatalf("repeated interrupted event count = %d, want 3", eventCount)
	}
}

func TestAppendEventPayloadSerializesSequencesPerWorkflowRun(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		if strings.Contains(err.Error(), "requires cgo") {
			t.Skipf("sqlite driver unavailable without cgo: %v", err)
		}
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&RunEvent{}); err != nil {
		t.Fatalf("migrate run events: %v", err)
	}
	service := NewService(db)
	const count = 20
	var wg sync.WaitGroup
	errs := make(chan error, count)
	for i := 0; i < count; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			_, err := service.AppendEventPayload(context.Background(), AppendEventParams{
				TenantID:      "tenant-1",
				AppID:         "app-1",
				WorkflowRunID: "run-1",
				EventType:     EventNodeStarted,
				EventData:     map[string]interface{}{"index": index},
			})
			errs <- err
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("append event: %v", err)
		}
	}
	payload, err := service.ListEvents(context.Background(), "tenant-1", "run-1", 0, count)
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	if len(payload.Events) != count {
		t.Fatalf("events = %d, want %d", len(payload.Events), count)
	}
	for index, event := range payload.Events {
		want := index + 1
		if event.Sequence != want {
			t.Fatalf("event[%d].sequence = %d, want %d; events=%s", index, event.Sequence, want, sequencesDebug(payload.Events))
		}
	}
}

func sequencesDebug(events []RunEventPayload) string {
	values := make([]string, 0, len(events))
	for _, event := range events {
		values = append(values, fmt.Sprintf("%d", event.Sequence))
	}
	return "[" + strings.Join(values, ",") + "]"
}

func TestV2ClaimResumeOwnsOneGenerationAndRejectsOldLease(t *testing.T) {
	db := openPauseServiceTestDB(t)
	service := NewService(db)
	run := pauseTestWorkflowRun{
		ID: "run-claim", RuntimeProtocolVersion: 2, ExecutionGeneration: 1,
		Status: "paused",
	}
	conversationID := "00000000-0000-0000-0000-000000000777"
	run.ConversationID = &conversationID
	if err := db.Create(&run).Error; err != nil {
		t.Fatalf("create run: %v", err)
	}
	pauseRecord := RunPause{
		ID: "pause-claim", TenantID: "tenant-1", AppID: "app-1", WorkflowRunID: run.ID,
		NodeID: "approval", Reason: ReasonTypeApprovalRequired, StateJSON: `{"version":"2"}`,
		Generation: 1, Status: RunPauseStatusResumeReady,
	}
	pauseRecord.ConversationID = &conversationID
	if err := db.Create(&pauseRecord).Error; err != nil {
		t.Fatalf("create pause: %v", err)
	}
	message := pauseTestMessage{
		ID: "message-claim", WorkflowRunID: run.ID, ConversationID: conversationID,
		Status: "pending_approval", ExecutionGeneration: 1,
	}
	if err := db.Create(&message).Error; err != nil {
		t.Fatalf("create paused message: %v", err)
	}
	if err := db.Create(&pauseTestConversation{ID: conversationID, RuntimeStatus: "pending_approval", ActiveWorkflowRunID: &run.ID}).Error; err != nil {
		t.Fatalf("create paused conversation: %v", err)
	}
	if err := db.Create(&RunPauseReason{
		ID: "reason-submit", PauseID: pauseRecord.ID, Type: ReasonTypeQuestionAnswerRequired,
		NodeID: "question", Status: RunPauseReasonStatusPending,
	}).Error; err != nil {
		t.Fatalf("create pause reason: %v", err)
	}
	if _, err := service.PrepareResume(context.Background(), run.ID, pauseRecord.ID, "form-1"); err != nil {
		t.Fatalf("prepare resume: %v", err)
	}

	claim, err := service.ClaimResume(context.Background(), run.ID, pauseRecord.ID, time.Minute)
	if err != nil {
		t.Fatalf("claim resume: %v", err)
	}
	if claim.ExecutionID == "" || claim.Generation != 2 || claim.PauseID != pauseRecord.ID {
		t.Fatalf("unexpected claim: %#v", claim)
	}
	if claim.Event == nil || claim.Event.Event != EventWorkflowResumed || claim.Event.Sequence != claim.EventCursor {
		t.Fatalf("claim resume event = %#v, cursor=%d", claim.Event, claim.EventCursor)
	}
	if err := db.First(&message, "id = ?", message.ID).Error; err != nil {
		t.Fatalf("reload claimed message: %v", err)
	}
	if message.Status != "running" || message.ExecutionGeneration != claim.Generation || message.ProjectionRevision != 1 {
		t.Fatalf("claimed message projection = %+v", message)
	}
	if _, err := service.ClaimResume(context.Background(), run.ID, pauseRecord.ID, time.Minute); !errors.Is(err, ErrResumeAlreadyRunning) {
		t.Fatalf("second claim error = %v, want ErrResumeAlreadyRunning", err)
	}

	if err := db.Model(&RunPause{}).Where("id = ?", pauseRecord.ID).
		Update("lease_expires_at", time.Now().Add(-time.Second)).Error; err != nil {
		t.Fatalf("expire pause lease: %v", err)
	}
	takeover, err := service.ClaimResume(context.Background(), run.ID, pauseRecord.ID, time.Minute)
	if err != nil {
		t.Fatalf("take over expired claim: %v", err)
	}
	if takeover.Generation != 3 || takeover.ExecutionID == claim.ExecutionID {
		t.Fatalf("unexpected takeover: %#v", takeover)
	}
	var resumedEvents []RunEvent
	if err := db.Where("workflow_run_id = ? AND event_type = ?", run.ID, EventWorkflowResumed).
		Order("sequence ASC").Find(&resumedEvents).Error; err != nil {
		t.Fatalf("load resumed events: %v", err)
	}
	if len(resumedEvents) != 2 || resumedEvents[0].Sequence >= resumedEvents[1].Sequence {
		t.Fatalf("resumed events = %#v, want two ordered execution boundaries", resumedEvents)
	}
	if resumedEvents[0].IdempotencyKey == nil || resumedEvents[1].IdempotencyKey == nil ||
		*resumedEvents[0].IdempotencyKey == *resumedEvents[1].IdempotencyKey {
		t.Fatalf("takeover must use a distinct resume idempotency key: %#v", resumedEvents)
	}
	if _, err := service.RenewExecutionLease(context.Background(), *claim, time.Minute); !errors.Is(err, ErrExecutionOwnershipLost) {
		t.Fatalf("old claim renewal = %v, want ownership lost", err)
	}
}

func TestInitialExecutionLeaseDoesNotRequirePause(t *testing.T) {
	db := openPauseServiceTestDB(t)
	executionID := "11111111-1111-1111-1111-111111111111"
	run := pauseTestWorkflowRun{
		ID: "run-initial", RuntimeProtocolVersion: 2, ExecutionGeneration: 1,
		ActiveExecutionID: &executionID, Status: "running",
	}
	if err := db.Create(&run).Error; err != nil {
		t.Fatalf("create run: %v", err)
	}
	_, err := NewService(db).RenewExecutionLease(context.Background(), ExecutionClaim{
		WorkflowRunID: run.ID, Generation: 1, ExecutionID: executionID,
	}, time.Minute)
	if err != nil {
		t.Fatalf("renew initial execution lease: %v", err)
	}
}

func TestAppendEventPayloadRejectsStaleExecutionOwner(t *testing.T) {
	db := openPauseServiceTestDB(t)
	service := NewService(db)
	firstExecutionID := "11111111-1111-1111-1111-111111111111"
	secondExecutionID := "22222222-2222-2222-2222-222222222222"
	run := pauseTestWorkflowRun{
		ID: "run-event-fence", RuntimeProtocolVersion: 2, ExecutionGeneration: 1,
		ActiveExecutionID: &firstExecutionID, Status: "running",
	}
	if err := db.Create(&run).Error; err != nil {
		t.Fatalf("create run: %v", err)
	}

	first, err := service.AppendEventPayload(context.Background(), AppendEventParams{
		TenantID: "tenant-1", AppID: "app-1", WorkflowRunID: run.ID,
		EventType: "node_started", ExecutionID: firstExecutionID,
		ExpectedExecutionID: firstExecutionID, ExpectedExecutionGeneration: 1,
		IdempotencyKey: "node:first:started", EventData: map[string]interface{}{"node_id": "first"},
	})
	if err != nil {
		t.Fatalf("append first owner event: %v", err)
	}
	if first.Sequence != 1 {
		t.Fatalf("first sequence = %d, want 1", first.Sequence)
	}

	if err := db.Model(&pauseTestWorkflowRun{}).Where("id = ?", run.ID).Updates(map[string]interface{}{
		"execution_generation": 2,
		"active_execution_id":  secondExecutionID,
	}).Error; err != nil {
		t.Fatalf("take over execution: %v", err)
	}
	_, err = service.AppendEventPayload(context.Background(), AppendEventParams{
		TenantID: "tenant-1", AppID: "app-1", WorkflowRunID: run.ID,
		EventType: "node_finished", ExecutionID: firstExecutionID,
		ExpectedExecutionID: firstExecutionID, ExpectedExecutionGeneration: 1,
		IdempotencyKey: "node:first:finished", EventData: map[string]interface{}{"node_id": "first"},
	})
	if !errors.Is(err, ErrExecutionOwnershipLost) {
		t.Fatalf("stale append error = %v, want ErrExecutionOwnershipLost", err)
	}

	second, err := service.AppendEventPayload(context.Background(), AppendEventParams{
		TenantID: "tenant-1", AppID: "app-1", WorkflowRunID: run.ID,
		EventType: "node_started", ExecutionID: secondExecutionID,
		ExpectedExecutionID: secondExecutionID, ExpectedExecutionGeneration: 2,
		IdempotencyKey: "node:second:started", EventData: map[string]interface{}{"node_id": "second"},
	})
	if err != nil {
		t.Fatalf("append second owner event: %v", err)
	}
	if second.Sequence != 2 || second.ExecutionID != secondExecutionID {
		t.Fatalf("second event = %#v, want sequence 2 owned by takeover", second)
	}
	var count int64
	if err := db.Model(&RunEvent{}).Where("workflow_run_id = ?", run.ID).Count(&count).Error; err != nil {
		t.Fatalf("count events: %v", err)
	}
	if count != 2 {
		t.Fatalf("event count = %d, want 2", count)
	}
}

func TestAppendEventPayloadRejectsStalePauseRevision(t *testing.T) {
	db := openPauseServiceTestDB(t)
	service := NewService(db)
	run := pauseTestWorkflowRun{ID: "run-pause-event-fence", RuntimeProtocolVersion: 2, ExecutionGeneration: 1, Status: "paused"}
	if err := db.Create(&run).Error; err != nil {
		t.Fatalf("create run: %v", err)
	}
	pauseRecord := RunPause{
		ID: "pause-event-fence", TenantID: "tenant-1", AppID: "app-1", WorkflowRunID: run.ID,
		NodeID: "approval", Reason: ReasonTypeApprovalRequired, StateJSON: `{"version":"2"}`,
		Generation: 1, Status: RunPauseStatusPaused, Revision: 3,
	}
	if err := db.Create(&pauseRecord).Error; err != nil {
		t.Fatalf("create pause: %v", err)
	}
	generation := pauseRecord.Generation
	staleRevision := pauseRecord.Revision - 1
	_, err := service.AppendEventPayload(context.Background(), AppendEventParams{
		TenantID: "tenant-1", AppID: "app-1", WorkflowRunID: run.ID,
		EventType: EventApprovalResultFilled, PauseID: pauseRecord.ID, PauseGeneration: &generation,
		ExpectedPauseID: pauseRecord.ID, ExpectedPauseGeneration: &generation, ExpectedPauseRevision: &staleRevision,
		IdempotencyKey: "approval-result:stale", EventData: map[string]interface{}{"form_id": "form-1"},
	})
	if !errors.Is(err, ErrPauseNotResumeReady) {
		t.Fatalf("stale pause append error = %v, want ErrPauseNotResumeReady", err)
	}

	currentRevision := pauseRecord.Revision
	stored, err := service.AppendEventPayload(context.Background(), AppendEventParams{
		TenantID: "tenant-1", AppID: "app-1", WorkflowRunID: run.ID,
		EventType: EventApprovalResultFilled, PauseID: pauseRecord.ID, PauseGeneration: &generation,
		ExpectedPauseID: pauseRecord.ID, ExpectedPauseGeneration: &generation, ExpectedPauseRevision: &currentRevision,
		IdempotencyKey: "approval-result:current", EventData: map[string]interface{}{"form_id": "form-1"},
	})
	if err != nil {
		t.Fatalf("append current pause event: %v", err)
	}
	if stored.Sequence != 1 {
		t.Fatalf("current pause event sequence = %d, want 1", stored.Sequence)
	}
}

func TestClaimResumeRollsBackWhenMessageProjectionIsMissing(t *testing.T) {
	db := openPauseServiceTestDB(t)
	service := NewService(db)
	run := pauseTestWorkflowRun{ID: "run-claim-rollback", RuntimeProtocolVersion: 2, ExecutionGeneration: 1, Status: "paused"}
	if err := db.Create(&run).Error; err != nil {
		t.Fatalf("create run: %v", err)
	}
	conversationID := "conversation-claim-rollback"
	pauseRecord := RunPause{
		ID: "pause-claim-rollback", TenantID: "tenant-1", AppID: "app-1", WorkflowRunID: run.ID,
		NodeID: "approval", Reason: ReasonTypeApprovalRequired, StateJSON: `{"version":"2"}`,
		ConversationID: &conversationID, Generation: 1, Status: RunPauseStatusResumeReady, Revision: 2,
	}
	if err := db.Create(&pauseRecord).Error; err != nil {
		t.Fatalf("create pause: %v", err)
	}

	if _, err := service.ClaimResume(context.Background(), run.ID, pauseRecord.ID, time.Minute); err == nil {
		t.Fatal("claim resume succeeded without the required message projection")
	}
	if err := db.First(&run, "id = ?", run.ID).Error; err != nil {
		t.Fatalf("reload run: %v", err)
	}
	if run.Status != "paused" || run.ExecutionGeneration != 1 || run.ActiveExecutionID != nil {
		t.Fatalf("run changed after rolled back claim: %+v", run)
	}
	if err := db.First(&pauseRecord, "id = ?", pauseRecord.ID).Error; err != nil {
		t.Fatalf("reload pause: %v", err)
	}
	if pauseRecord.Status != RunPauseStatusResumeReady || pauseRecord.Revision != 2 || pauseRecord.ResumeExecutionID != nil {
		t.Fatalf("pause changed after rolled back claim: %+v", pauseRecord)
	}
	var eventCount int64
	if err := db.Model(&RunEvent{}).Where("workflow_run_id = ?", run.ID).Count(&eventCount).Error; err != nil {
		t.Fatalf("count resume events: %v", err)
	}
	if eventCount != 0 {
		t.Fatalf("resume event count after rollback = %d, want 0", eventCount)
	}
}

func TestSubmitInteractionIsIdempotentAndCreatesOneResumeOutbox(t *testing.T) {
	db := openPauseServiceTestDB(t)
	service := NewService(db)
	run := pauseTestWorkflowRun{ID: "run-submit", RuntimeProtocolVersion: 2, ExecutionGeneration: 1, Status: "paused"}
	if err := db.Create(&run).Error; err != nil {
		t.Fatalf("create run: %v", err)
	}
	pauseRecord := RunPause{
		ID: "pause-submit", TenantID: "tenant-1", AppID: "app-1", WorkflowRunID: run.ID,
		NodeID: "question", Reason: ReasonTypeQuestionAnswerRequired, StateJSON: `{"version":"2"}`,
		Generation: 1, Status: RunPauseStatusPaused,
	}
	if err := db.Create(&pauseRecord).Error; err != nil {
		t.Fatalf("create pause: %v", err)
	}

	for attempt := 0; attempt < 2; attempt++ {
		result, err := service.SubmitInteraction(context.Background(), run.ID, pauseRecord.ID, "question", EventQuestionAnswerSubmitted,
			map[string]interface{}{"node_id": "question", "answer": "yes"}, "question-submit:1")
		if err != nil {
			t.Fatalf("submit interaction attempt %d: %v", attempt, err)
		}
		if result.Event == nil || result.Outbox == nil {
			t.Fatalf("submit interaction attempt %d returned incomplete result: %#v", attempt, result)
		}
	}
	var eventCount int64
	if err := db.Model(&RunEvent{}).Where("workflow_run_id = ?", run.ID).Count(&eventCount).Error; err != nil {
		t.Fatal(err)
	}
	if eventCount != 1 {
		t.Fatalf("event count = %d, want 1", eventCount)
	}
	var outboxCount int64
	if err := db.Model(&RuntimeOutbox{}).Where("workflow_run_id = ?", run.ID).Count(&outboxCount).Error; err != nil {
		t.Fatal(err)
	}
	if outboxCount != 1 {
		t.Fatalf("outbox count = %d, want 1", outboxCount)
	}
	claim, err := service.ClaimResume(context.Background(), run.ID, pauseRecord.ID, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.ClosePause(context.Background(), *claim); err != nil {
		t.Fatal(err)
	}
	replay, err := service.SubmitInteraction(context.Background(), run.ID, pauseRecord.ID, "question", EventQuestionAnswerSubmitted,
		map[string]interface{}{"node_id": "question", "answer": "yes"}, "question-submit:1")
	if err != nil {
		t.Fatal(err)
	}
	if replay.Event == nil || replay.Outbox == nil || !replay.ResumeReady {
		t.Fatalf("closed pause replay = %#v", replay)
	}
}

func TestSubmitInteractionWaitsForEveryPauseReason(t *testing.T) {
	db := openPauseServiceTestDB(t)
	service := NewService(db)
	executionID := "22222222-2222-2222-2222-222222222222"
	run := pauseTestWorkflowRun{
		ID: "run-mixed-reasons", RuntimeProtocolVersion: 2, ExecutionGeneration: 1,
		ActiveExecutionID: &executionID, Status: "running",
	}
	if err := db.Create(&run).Error; err != nil {
		t.Fatal(err)
	}
	pauseRecord, err := service.Save(context.Background(), SaveParams{
		TenantID: "tenant-1", AppID: "app-1", WorkflowRunID: run.ID,
		NodeID: "question", Reason: ReasonTypeQuestionAnswerRequired, State: State{Version: StateVersion, WorkflowRunID: run.ID},
		ExecutionID: executionID, Generation: run.ExecutionGeneration,
		Reasons: []Reason{
			{Type: ReasonTypeQuestionAnswerRequired, NodeID: "question"},
			{Type: ReasonTypeApprovalRequired, NodeID: "approval", FormID: "form-1"},
		},
		Events: []AppendEventParams{
			{
				EventType: EventQuestionAnswerRequested,
				EventData: map[string]interface{}{
					"workflow_run_id": run.ID,
					"node_id":         "question",
					"question":        "Choose a branch",
					"choices":         []interface{}{"yes", "no"},
				},
				IdempotencyKey: "question-requested:question",
			},
			{
				EventType: EventApprovalRequested,
				EventData: map[string]interface{}{
					"workflow_run_id": run.ID,
					"node_id":         "approval",
					"form_id":         "form-1",
					"content":         "Approve the next step",
				},
				IdempotencyKey: "approval-requested:approval",
			},
			{
				EventType: EventWorkflowPaused,
				EventData: map[string]interface{}{
					"id":           run.ID,
					"status":       "paused",
					"paused_nodes": []interface{}{"question", "approval"},
					"reasons": []interface{}{
						map[string]interface{}{
							"type":     ReasonTypeQuestionAnswerRequired,
							"node_id":  "question",
							"question": "Choose a branch",
							"choices":  []interface{}{"yes", "no"},
						},
						map[string]interface{}{
							"type":    ReasonTypeApprovalRequired,
							"node_id": "approval",
							"form_id": "form-1",
						},
					},
				},
				IdempotencyKey: "workflow-paused",
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	submission, err := service.SubmitInteraction(context.Background(), run.ID, pauseRecord.ID, "question", EventQuestionAnswerSubmitted,
		map[string]interface{}{"node_id": "question", "answer": "yes"}, "question-answer:"+pauseRecord.ID)
	if err != nil {
		t.Fatal(err)
	}
	if submission.ResumeReady || submission.Outbox != nil {
		t.Fatalf("question submission resumed pause with pending approval: %#v", submission)
	}
	if len(submission.PendingEvents) != 2 {
		t.Fatalf("pending events = %#v, want approval request and paused projection", submission.PendingEvents)
	}
	if submission.PendingEvents[0].Event != EventApprovalRequested {
		t.Fatalf("next pending event = %q, want %q", submission.PendingEvents[0].Event, EventApprovalRequested)
	}
	if submission.PendingEvents[0].Data["form_id"] != "form-1" {
		t.Fatalf("next pending approval = %#v", submission.PendingEvents[0].Data)
	}
	if submission.PendingEvents[1].Event != EventWorkflowPaused {
		t.Fatalf("pending terminal event = %q, want %q", submission.PendingEvents[1].Event, EventWorkflowPaused)
	}

	var stored RunPause
	if err := db.First(&stored, "id = ?", pauseRecord.ID).Error; err != nil {
		t.Fatal(err)
	}
	if stored.Status != RunPauseStatusPaused {
		t.Fatalf("pause status = %q, want %q", stored.Status, RunPauseStatusPaused)
	}
	var reasons []RunPauseReason
	if err := db.Where("pause_id = ?", pauseRecord.ID).Order("type ASC").Find(&reasons).Error; err != nil {
		t.Fatal(err)
	}
	statuses := map[string]string{}
	for _, reason := range reasons {
		statuses[reason.Type] = reason.Status
	}
	if statuses[ReasonTypeQuestionAnswerRequired] != RunPauseReasonStatusCompleted || statuses[ReasonTypeApprovalRequired] != RunPauseReasonStatusPending {
		t.Fatalf("reason statuses = %#v", statuses)
	}
}

func TestSubmitInteractionCompletesOnlyTheTargetQuestionReason(t *testing.T) {
	db := openPauseServiceTestDB(t)
	service := NewService(db)
	executionID := "33333333-3333-3333-3333-333333333333"
	run := pauseTestWorkflowRun{
		ID: "run-two-questions", RuntimeProtocolVersion: 2, ExecutionGeneration: 1,
		ActiveExecutionID: &executionID, Status: "running",
	}
	if err := db.Create(&run).Error; err != nil {
		t.Fatal(err)
	}
	pauseRecord, err := service.Save(context.Background(), SaveParams{
		TenantID: "tenant-1", AppID: "app-1", WorkflowRunID: run.ID,
		NodeID: "question-1", Reason: ReasonTypeQuestionAnswerRequired,
		State:       State{Version: StateVersion, WorkflowRunID: run.ID},
		ExecutionID: executionID, Generation: run.ExecutionGeneration,
		Reasons: []Reason{
			{Type: ReasonTypeQuestionAnswerRequired, NodeID: "question-1"},
			{Type: ReasonTypeQuestionAnswerRequired, NodeID: "question-2"},
		},
		Events: []AppendEventParams{
			{
				EventType: EventQuestionAnswerRequested,
				EventData: map[string]interface{}{
					"workflow_run_id": run.ID,
					"node_id":         "question-1",
					"round":           1,
					"question":        "First question",
					"choices":         []interface{}{"first-a", "first-b"},
				},
				IdempotencyKey: "question-requested:question-1",
			},
			{
				EventType: EventQuestionAnswerRequested,
				EventData: map[string]interface{}{
					"workflow_run_id": run.ID,
					"node_id":         "question-2",
					"round":           2,
					"question":        "Second question",
					"choices":         []interface{}{"second-a", "second-b"},
				},
				IdempotencyKey: "question-requested:question-2",
			},
			{
				EventType: EventWorkflowPaused,
				EventData: map[string]interface{}{
					"id":           run.ID,
					"status":       "paused",
					"paused_nodes": []interface{}{"question-1", "question-2"},
					"reasons": []interface{}{
						map[string]interface{}{
							"type":     ReasonTypeQuestionAnswerRequired,
							"node_id":  "question-1",
							"round":    1,
							"question": "First question",
							"choices":  []interface{}{"first-a", "first-b"},
						},
						map[string]interface{}{
							"type":     ReasonTypeQuestionAnswerRequired,
							"node_id":  "question-2",
							"round":    2,
							"question": "Second question",
							"choices":  []interface{}{"second-a", "second-b"},
						},
					},
				},
				IdempotencyKey: "workflow-paused",
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	first, err := service.SubmitInteraction(context.Background(), run.ID, pauseRecord.ID, "question-1", EventQuestionAnswerSubmitted,
		map[string]interface{}{"node_id": "question-1", "answer": "first"}, "question-answer:"+pauseRecord.ID+":1")
	if err != nil {
		t.Fatal(err)
	}
	if first.ResumeReady || first.Outbox != nil {
		t.Fatalf("first question unexpectedly resumed run: %#v", first)
	}
	if len(first.PendingEvents) != 2 {
		t.Fatalf("pending events = %#v, want next question and paused projection", first.PendingEvents)
	}
	nextQuestion := first.PendingEvents[0]
	if nextQuestion.Event != EventQuestionAnswerRequested {
		t.Fatalf("next pending event = %q, want %q", nextQuestion.Event, EventQuestionAnswerRequested)
	}
	if nextQuestion.Data["node_id"] != "question-2" ||
		nextQuestion.Data["question"] != "Second question" ||
		nextQuestion.Data["round"] != float64(2) {
		t.Fatalf("next pending question data = %#v", nextQuestion.Data)
	}
	choices, ok := nextQuestion.Data["choices"].([]interface{})
	if !ok || len(choices) != 2 || choices[0] != "second-a" {
		t.Fatalf("next pending question choices = %#v", nextQuestion.Data["choices"])
	}
	if first.PendingEvents[1].Event != EventWorkflowPaused {
		t.Fatalf("pending terminal event = %q, want %q", first.PendingEvents[1].Event, EventWorkflowPaused)
	}
	replayed, err := service.SubmitInteraction(context.Background(), run.ID, pauseRecord.ID, "question-1", EventQuestionAnswerSubmitted,
		map[string]interface{}{"node_id": "question-1", "answer": "first"}, "question-answer:"+pauseRecord.ID+":1")
	if err != nil {
		t.Fatal(err)
	}
	if len(replayed.PendingEvents) != 2 ||
		replayed.PendingEvents[0].Sequence != first.PendingEvents[0].Sequence ||
		replayed.PendingEvents[1].Sequence != first.PendingEvents[1].Sequence {
		t.Fatalf("idempotent pending projection = %#v, want sequences %d and %d", replayed.PendingEvents, first.PendingEvents[0].Sequence, first.PendingEvents[1].Sequence)
	}
	var storedReasons []RunPauseReason
	if err := db.Where("pause_id = ?", pauseRecord.ID).Order("node_id ASC").Find(&storedReasons).Error; err != nil {
		t.Fatal(err)
	}
	if len(storedReasons) != 2 || storedReasons[0].Status != RunPauseReasonStatusCompleted || storedReasons[1].Status != RunPauseReasonStatusPending {
		t.Fatalf("question reason statuses = %#v", storedReasons)
	}

	second, err := service.SubmitInteraction(context.Background(), run.ID, pauseRecord.ID, "question-2", EventQuestionAnswerSubmitted,
		map[string]interface{}{"node_id": "question-2", "answer": "second"}, "question-answer:"+pauseRecord.ID+":2")
	if err != nil {
		t.Fatal(err)
	}
	if !second.ResumeReady || second.Outbox == nil {
		t.Fatalf("second question did not make pause resume-ready: %#v", second)
	}
	var payload RuntimeOutboxPayload
	if err := json.Unmarshal([]byte(second.Outbox.PayloadJSON), &payload); err != nil {
		t.Fatalf("decode resume outbox payload: %v", err)
	}
	submissions, ok := payload.ResumeInputs["interaction_submissions"].([]interface{})
	if !ok || len(submissions) != 2 {
		t.Fatalf("interaction submissions = %#v, want two node-scoped answers", payload.ResumeInputs["interaction_submissions"])
	}
	firstSubmission, _ := submissions[0].(map[string]interface{})
	firstData, _ := firstSubmission["data"].(map[string]interface{})
	secondSubmission, _ := submissions[1].(map[string]interface{})
	secondData, _ := secondSubmission["data"].(map[string]interface{})
	if firstSubmission["node_id"] != "question-1" || firstData["answer"] != "first" {
		t.Fatalf("first interaction submission = %#v, want question-1/first", firstSubmission)
	}
	if secondSubmission["node_id"] != "question-2" || secondData["answer"] != "second" {
		t.Fatalf("second interaction submission = %#v, want question-2/second", secondSubmission)
	}
}
