package approval

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	workflowpause "github.com/zgiai/zgi/api/internal/modules/app/workflow/pause"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type approvalWorkflowRunScope struct {
	ID                     string  `gorm:"primaryKey"`
	RuntimeProtocolVersion int     `gorm:"not null"`
	NextEventSequence      int     `gorm:"not null"`
	ExecutionGeneration    int64   `gorm:"not null"`
	ActiveExecutionID      *string `gorm:"type:uuid"`
}

func (approvalWorkflowRunScope) TableName() string {
	return "workflow_run_logs"
}

func TestSubmitByTokenForWorkflowRunRejectsDifferentRunBeforeUpdate(t *testing.T) {
	db, mock, closeDB := openApprovalServiceMockDB(t)
	defer closeDB()

	now := time.Now()
	mock.ExpectQuery(`SELECT .*FROM "workflow_approval_forms".*access_token`).
		WillReturnRows(sqlmock.NewRows(approvalFormColumns()).AddRow(
			"form-1",
			"tenant-1",
			"agent-1",
			"other-run",
			"approval-node",
			"Approval",
			"approval-token",
			`{"content":"Approve","fields":[],"actions":[{"id":"approve","label":"Approve"}],"rendered_content":"Approve","default_values":{},"expiration_at":"2099-01-01T00:00:00Z"}`,
			"Approve",
			FormStatusWaiting,
			now.Add(time.Hour),
			nil,
			nil,
			nil,
			nil,
			nil,
			nil,
			now,
			now,
		))

	service := NewServiceWithDependencies(db, nil, nil, nil)
	_, err := service.SubmitByTokenForWorkflowRun(context.Background(), "approval-token", "owned-run", SubmitRequest{
		Action: "approve",
		Inputs: map[string]interface{}{},
	}, nil, nil)

	if !errors.Is(err, ErrFormNotFound) {
		t.Fatalf("error = %v, want ErrFormNotFound", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations were not met: %v", err)
	}
}

func TestEnsureFormWorkflowRunAllowsTrimmedMatch(t *testing.T) {
	err := ensureFormWorkflowRun(&Form{WorkflowRunID: " owned-run "}, "owned-run")
	if err != nil {
		t.Fatalf("ensureFormWorkflowRun returned error: %v", err)
	}
}

func TestEnsureFormWorkflowRunRejectsMissingOrDifferentRun(t *testing.T) {
	for name, form := range map[string]*Form{
		"missing_form": nil,
		"missing_run":  {WorkflowRunID: ""},
		"different":    {WorkflowRunID: "other-run"},
	} {
		t.Run(name, func(t *testing.T) {
			err := ensureFormWorkflowRun(form, "owned-run")
			if !errors.Is(err, ErrFormNotFound) {
				t.Fatalf("error = %v, want ErrFormNotFound", err)
			}
		})
	}
}

func TestEnsureFormSubmissionOptionsRequiresConfiguredWebAppChannel(t *testing.T) {
	disabled := false
	definition, err := json.Marshal(FormDefinition{
		SubmitMethods: SubmitMethods{WebApp: WebAppSubmitMethod{Enabled: &disabled}},
	})
	if err != nil {
		t.Fatalf("marshal definition: %v", err)
	}
	form := &Form{FormDefinition: string(definition)}

	err = ensureFormSubmissionOptions(form, SubmitOptions{RequireWebAppEnabled: true})
	if !errors.Is(err, ErrWebAppSubmissionDisabled) {
		t.Fatalf("error = %v, want ErrWebAppSubmissionDisabled", err)
	}
	if err := ensureFormSubmissionOptions(form, SubmitOptions{}); err != nil {
		t.Fatalf("debug submission should ignore configured channels: %v", err)
	}
}

func TestEnsureFormSubmissionOptionsPreservesLegacyWebAppDefault(t *testing.T) {
	definition, err := json.Marshal(FormDefinition{})
	if err != nil {
		t.Fatalf("marshal definition: %v", err)
	}
	if err := ensureFormSubmissionOptions(&Form{FormDefinition: string(definition)}, SubmitOptions{RequireWebAppEnabled: true}); err != nil {
		t.Fatalf("omitted webapp.enabled should remain enabled: %v", err)
	}
}

func TestSubmitApprovalProjectsPendingQuestionBeforeAgentContinuation(t *testing.T) {
	db := newApprovalTestDB(t)
	if err := db.AutoMigrate(
		&approvalWorkflowRunScope{},
		&workflowpause.RunPause{},
		&workflowpause.RunPauseReason{},
		&workflowpause.RunEvent{},
		&workflowpause.RuntimeOutbox{},
	); err != nil {
		t.Fatalf("migrate workflow pause tables: %v", err)
	}

	const (
		tenantID     = "11111111-1111-1111-1111-111111111111"
		appID        = "22222222-2222-2222-2222-222222222222"
		runID        = "mixed-pause-run"
		pauseID      = "33333333-3333-3333-3333-333333333333"
		formID       = "44444444-4444-4444-4444-444444444444"
		formToken    = "mixed-pause-token"
		approvalNode = "approval-node"
		questionNode = "question-node"
	)
	now := time.Now()
	pauseGeneration := int64(1)
	pauseIDValue := pauseID
	approvalRequestKey := "approval-requested"
	questionRequestKey := "question-requested"
	pausedKey := "workflow-paused"
	definition, err := json.Marshal(FormDefinition{
		Content: "Approve the request",
		Actions: []Action{{ID: "approve", Label: "Approve"}},
	})
	if err != nil {
		t.Fatalf("marshal approval definition: %v", err)
	}

	records := []interface{}{
		&approvalWorkflowRunScope{
			ID:                     runID,
			RuntimeProtocolVersion: 2,
			NextEventSequence:      3,
			ExecutionGeneration:    1,
		},
		&Form{
			ID: formID, TenantID: tenantID, AppID: appID, WorkflowRunID: runID,
			NodeID: approvalNode, NodeTitle: "Approval", AccessToken: formToken,
			FormDefinition: string(definition), RenderedContent: "Approve the request",
			Status: FormStatusWaiting, ExpirationTime: now.Add(time.Hour),
		},
		&workflowpause.RunPause{
			ID: pauseID, TenantID: tenantID, AppID: appID, WorkflowRunID: runID,
			NodeID: approvalNode, Reason: "mixed", StateJSON: `{}`,
			Generation: pauseGeneration, Status: workflowpause.RunPauseStatusPaused,
		},
		&workflowpause.RunPauseReason{
			ID: "55555555-5555-5555-5555-555555555555", PauseID: pauseID,
			Type: workflowpause.ReasonTypeApprovalRequired, NodeID: approvalNode,
			FormID: formID, Status: workflowpause.RunPauseReasonStatusPending, CreatedAt: now,
		},
		&workflowpause.RunPauseReason{
			ID: "66666666-6666-6666-6666-666666666666", PauseID: pauseID,
			Type: workflowpause.ReasonTypeQuestionAnswerRequired, NodeID: questionNode,
			Status: workflowpause.RunPauseReasonStatusPending, CreatedAt: now.Add(time.Millisecond),
		},
		&workflowpause.RunEvent{
			ID: "77777777-7777-7777-7777-777777777777", TenantID: tenantID, AppID: appID,
			WorkflowRunID: runID, Sequence: 1, EventType: workflowpause.EventApprovalRequested,
			EventData:     `{"node_id":"approval-node","form_id":"44444444-4444-4444-4444-444444444444"}`,
			SchemaVersion: 2, Category: workflowpause.EventCategoryInteraction,
			PauseID: &pauseIDValue, PauseGeneration: &pauseGeneration,
			IdempotencyKey: &approvalRequestKey, OccurredAt: now,
		},
		&workflowpause.RunEvent{
			ID: "88888888-8888-8888-8888-888888888888", TenantID: tenantID, AppID: appID,
			WorkflowRunID: runID, Sequence: 2, EventType: workflowpause.EventQuestionAnswerRequested,
			EventData:     `{"node_id":"question-node","question":"Which option?","choices":["one","two"]}`,
			SchemaVersion: 2, Category: workflowpause.EventCategoryInteraction,
			PauseID: &pauseIDValue, PauseGeneration: &pauseGeneration,
			IdempotencyKey: &questionRequestKey, OccurredAt: now,
		},
		&workflowpause.RunEvent{
			ID: "99999999-9999-9999-9999-999999999999", TenantID: tenantID, AppID: appID,
			WorkflowRunID: runID, Sequence: 3, EventType: workflowpause.EventWorkflowPaused,
			EventData:     `{"id":"mixed-pause-run","status":"paused","paused_nodes":["approval-node","question-node"]}`,
			SchemaVersion: 2, Category: workflowpause.EventCategoryControl,
			PauseID: &pauseIDValue, PauseGeneration: &pauseGeneration,
			IdempotencyKey: &pausedKey, OccurredAt: now,
		},
	}
	for _, record := range records {
		if err := db.Create(record).Error; err != nil {
			t.Fatalf("create mixed-pause fixture %T: %v", record, err)
		}
	}

	service := NewService(db)
	submission, err := service.SubmitByTokenForWorkflowRunWithResumeOptions(
		context.Background(),
		formToken,
		runID,
		SubmitRequest{Action: "approve", Inputs: map[string]interface{}{}},
		nil,
		nil,
		SubmitOptions{},
	)
	if err != nil {
		t.Fatalf("submit approval: %v", err)
	}
	if submission.ResumeReady {
		t.Fatal("mixed pause became resume-ready before the question was answered")
	}
	if submission.ResumeState != "waiting" {
		t.Fatalf("resume state = %q, want waiting", submission.ResumeState)
	}
	if len(submission.PendingEvents) != 2 {
		t.Fatalf("pending event count = %d, want 2", len(submission.PendingEvents))
	}
	if submission.PendingEvents[0].Event != workflowpause.EventQuestionAnswerRequested {
		t.Fatalf("first pending event = %q, want question_answer_requested", submission.PendingEvents[0].Event)
	}
	if submission.PendingEvents[0].Data["node_id"] != questionNode {
		t.Fatalf("pending question node = %#v, want %q", submission.PendingEvents[0].Data["node_id"], questionNode)
	}
	if submission.PendingEvents[1].Event != workflowpause.EventWorkflowPaused {
		t.Fatalf("second pending event = %q, want workflow_paused", submission.PendingEvents[1].Event)
	}
	if submission.Outbox != nil {
		t.Fatal("resume outbox created while a question remains pending")
	}

	replay, err := service.SubmitByTokenForWorkflowRunWithResumeOptions(
		context.Background(),
		formToken,
		runID,
		SubmitRequest{Action: "approve", Inputs: map[string]interface{}{}},
		nil,
		nil,
		SubmitOptions{},
	)
	if err != nil {
		t.Fatalf("replay approval submission: %v", err)
	}
	if !replay.IdempotentReplay {
		t.Fatal("repeat approval submission was not recognized as an idempotent replay")
	}
	if len(replay.PendingEvents) != 2 {
		t.Fatalf("replayed pending event count = %d, want 2", len(replay.PendingEvents))
	}
	if replay.PendingEvents[0].Sequence != submission.PendingEvents[0].Sequence ||
		replay.PendingEvents[1].Sequence != submission.PendingEvents[1].Sequence {
		t.Fatalf(
			"replayed pending sequences = [%d %d], want [%d %d]",
			replay.PendingEvents[0].Sequence,
			replay.PendingEvents[1].Sequence,
			submission.PendingEvents[0].Sequence,
			submission.PendingEvents[1].Sequence,
		)
	}
}

func openApprovalServiceMockDB(t *testing.T) (*gorm.DB, sqlmock.Sqlmock, func()) {
	t.Helper()

	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("create sqlmock: %v", err)
	}
	db, err := gorm.Open(postgres.New(postgres.Config{
		Conn:                 sqlDB,
		PreferSimpleProtocol: true,
	}), &gorm.Config{})
	if err != nil {
		_ = sqlDB.Close()
		t.Fatalf("open gorm sqlmock: %v", err)
	}
	return db, mock, func() {
		_ = sqlDB.Close()
	}
}

func approvalFormColumns() []string {
	return []string{
		"id",
		"tenant_id",
		"app_id",
		"workflow_run_id",
		"node_id",
		"node_title",
		"access_token",
		"form_definition",
		"rendered_content",
		"status",
		"expiration_time",
		"selected_action_id",
		"submitted_data",
		"submitted_at",
		"submission_user_id",
		"submission_end_user_id",
		"completed_by_recipient_id",
		"created_at",
		"updated_at",
	}
}
