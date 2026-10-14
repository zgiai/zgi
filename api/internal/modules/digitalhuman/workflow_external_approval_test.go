package digitalhuman

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	chatruntime "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/service"
	"github.com/zgiai/zgi/api/internal/capabilities/invocation"
	"github.com/zgiai/zgi/api/internal/capabilities/runtimeidentity"
	"github.com/zgiai/zgi/api/internal/capabilities/usersubject"
	workflowpause "github.com/zgiai/zgi/api/internal/modules/app/workflow/pause"
	"gorm.io/datatypes"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type externalApprovalRunStore struct {
	*fakeRunStore
	waiting []*Run
	calls   []string
}

func (s *externalApprovalRunStore) ListWorkflowApprovalWaitingRuns(context.Context, time.Time, int) ([]*Run, error) {
	return s.waiting, nil
}

func (s *externalApprovalRunStore) TouchWaitingRun(context.Context, uuid.UUID, time.Time) error {
	s.calls = append(s.calls, "touch")
	return nil
}

func (s *externalApprovalRunStore) MarkRunWaiting(_ context.Context, _ uuid.UUID, reason string, _ time.Time) error {
	s.calls = append(s.calls, "waiting:"+reason)
	return nil
}

func (s *externalApprovalRunStore) ResumeWaitingRun(context.Context, uuid.UUID, time.Time) error {
	s.calls = append(s.calls, "resume")
	return nil
}

func (s *externalApprovalRunStore) CancelWaitingRun(context.Context, uuid.UUID, time.Time) error {
	return nil
}

func (s *externalApprovalRunStore) CompleteRunWithDelivery(_ context.Context, _ uuid.UUID, summary string, _ datatypes.JSON, _ time.Time) error {
	s.calls = append(s.calls, "complete:"+summary)
	return nil
}

func (s *externalApprovalRunStore) FailRun(_ context.Context, _ uuid.UUID, code, _ string, _ time.Time) error {
	s.calls = append(s.calls, "fail:"+code)
	return nil
}

type externalApprovalChat struct {
	*fakeDigitalHumanChat
	*fakeWorkflowContinuationRuntime
	begun []chatruntime.Caller
}

func (c *externalApprovalChat) BeginExternalWorkflowApprovalContinuation(_ context.Context, _ chatruntime.Scope, caller chatruntime.Caller, _ chatruntime.RunConfig, conversationID, messageID uuid.UUID) (*chatruntime.WorkflowApprovalContinuation, error) {
	c.begun = append(c.begun, caller)
	return &chatruntime.WorkflowApprovalContinuation{
		ConversationID: conversationID, MessageID: messageID, WorkflowRunID: "workflow-run",
		MessageStatus: runtimemodel.MessageStatusWaitingApproval,
	}, nil
}

type fakeExternalApprovalProbe struct {
	progress externalWorkflowProgress
	formIDs  []string
}

func (p *fakeExternalApprovalProbe) Observe(_ context.Context, _ string, approvalFormID string) (externalWorkflowProgress, error) {
	p.formIDs = append(p.formIDs, approvalFormID)
	return p.progress, nil
}

func newExternalApprovalTestService(t *testing.T, progress externalWorkflowProgress, waitingRuns int) (*RuntimeService, *externalApprovalRunStore, *externalApprovalChat, *fakeExternalApprovalProbe) {
	t.Helper()
	store := &externalApprovalRunStore{fakeRunStore: newFakeRunStore()}
	chat := &externalApprovalChat{fakeDigitalHumanChat: &fakeDigitalHumanChat{}, fakeWorkflowContinuationRuntime: &fakeWorkflowContinuationRuntime{}}
	now := time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)
	service := NewRuntimeService(store, fakeExecutionResolver{now: now}, fakeBrainAdapter{}, chat, &fakeRunDispatcher{},
		WithChannelTransientDispatcher(&channelTransientDispatcherStub{}))
	service.now = func() time.Time { return now }
	service.workflowDB, service.workflowRunner = &gorm.DB{}, &channelApprovalRunnerStub{}
	probe := &fakeExternalApprovalProbe{progress: progress}
	service.externalApprovalProbe = probe

	conversationID, messageID := uuid.New(), uuid.New()
	chat.recoveryMessage = &runtimemodel.Message{
		ID: messageID, ConversationID: conversationID, Status: runtimemodel.MessageStatusWaitingApproval,
		Metadata: map[string]interface{}{
			"agent_workflow_continuation": map[string]interface{}{"workflow_run_id": "workflow-run", "ui_approval_allowed": false},
			"workflow_runs": []interface{}{map[string]interface{}{
				"workflow_run_id": "workflow-run", "approval": map[string]interface{}{"approval_form_id": "form-1"},
			}},
		},
	}
	scope, digitalHumanID, subjectID := testRunScope(), uuid.New(), uuid.New()
	scope.ActorID, scope.PrincipalID = subjectID, subjectID
	scope.PrincipalType, scope.Surface = string(invocation.PrincipalSystem), SurfaceTask
	scope.UserSubjectKind, scope.UserSubjectID = string(usersubject.APIEndUser), &subjectID
	for range waitingRuns {
		event := invocation.Event{
			ID: uuid.NewString(), Type: invocation.EventTypeSchedule,
			Origin: invocation.Origin{Kind: invocation.OriginSchedule},
			Target: runtimeidentity.SubjectRef{Type: runtimeidentity.SubjectTypeDigitalHuman, ID: digitalHumanID, OrganizationID: scope.OrganizationID, WorkspaceID: scope.WorkspaceID},
			Sender: invocation.PrincipalRef{Type: invocation.PrincipalSystem, ID: "test"}, OccurredAt: now,
		}
		run, _, err := service.SubmitInvocation(t.Context(), scope, event, "Request the purchase approval", nil)
		if err != nil {
			t.Fatalf("SubmitInvocation() error = %v", err)
		}
		run.Surface, run.Status = SurfaceWebApp, RunStatusWaiting
		run.WaitingReason, run.WaitingApprovalOrigin = runtimemodel.MessageStatusWaitingApproval, workflowApprovalOrigin
		run.ConversationID, run.MessageID = &conversationID, &messageID
		store.waiting = append(store.waiting, run)
	}
	return service, store, chat, probe
}

func TestExternalWorkflowApprovalCompletesTheMessageAndTheRun(t *testing.T) {
	service, store, chat, probe := newExternalApprovalTestService(t, externalWorkflowProgress{
		EventType: "workflow_finished", Payload: map[string]interface{}{"status": "succeeded", "outputs": map[string]interface{}{"decision": "approved"}},
	}, 1)
	chat.summaryResult = &chatruntime.ChatResult{Status: runtimemodel.MessageStatusCompleted, Answer: "The purchase was approved.", Metadata: map[string]interface{}{}}
	if err := service.resumeExternalWorkflowApprovals(t.Context(), service.now(), defaultOutboxBatch); err != nil {
		t.Fatal(err)
	}
	if len(probe.formIDs) != 1 || probe.formIDs[0] != "form-1" {
		t.Fatalf("probe form ids = %v, want the approval the message waits on", probe.formIDs)
	}
	if len(chat.begun) != 1 || chat.begun[0].Source != digitalHumanWebAppConversationSource || chat.begun[0].RunID == nil {
		t.Fatalf("external continuations = %#v, want one claimed as the WebApp run", chat.begun)
	}
	if len(chat.summarized) != 1 || chat.summarized[0].Status != "succeeded" || chat.summarized[0].Outputs["decision"] != "approved" {
		t.Fatalf("summaries = %#v, want the workflow outcome summarized", chat.summarized)
	}
	if want := []string{"resume", "complete:The purchase was approved."}; !equalStrings(store.calls, want) {
		t.Fatalf("run store calls = %v, want %v", store.calls, want)
	}
}

func TestExternalWorkflowApprovalFailsTheRunWhenTheWorkflowFailed(t *testing.T) {
	service, store, _, _ := newExternalApprovalTestService(t, externalWorkflowProgress{
		EventType: "workflow_finished", Payload: map[string]interface{}{"status": "failed", "error": "node failed"},
	}, 1)
	if err := service.resumeExternalWorkflowApprovals(t.Context(), service.now(), defaultOutboxBatch); err != nil {
		t.Fatal(err)
	}
	if want := []string{"resume", "fail:digital_human.run.execution_failed"}; !equalStrings(store.calls, want) {
		t.Fatalf("run store calls = %v, want %v", store.calls, want)
	}
}

func TestExternalWorkflowApprovalMovesToTheNextRequest(t *testing.T) {
	service, store, chat, _ := newExternalApprovalTestService(t, externalWorkflowProgress{
		EventType: "question_answer_requested", Payload: map[string]interface{}{"question": "Which budget?"},
	}, 1)
	if err := service.resumeExternalWorkflowApprovals(t.Context(), service.now(), defaultOutboxBatch); err != nil {
		t.Fatal(err)
	}
	if len(chat.paused) != 1 || chat.paused[0] != runtimemodel.MessageStatusWaitingQuestion {
		t.Fatalf("paused = %v, want the message waiting on the new question", chat.paused)
	}
	if want := []string{"resume", "waiting:" + runtimemodel.MessageStatusWaitingQuestion}; !equalStrings(store.calls, want) {
		t.Fatalf("run store calls = %v, want %v", store.calls, want)
	}
}

func TestExternalWorkflowApprovalStillPendingOnlyTouchesTheRun(t *testing.T) {
	service, store, chat, _ := newExternalApprovalTestService(t, externalWorkflowProgress{}, 1)
	if err := service.resumeExternalWorkflowApprovals(t.Context(), service.now(), defaultOutboxBatch); err != nil {
		t.Fatal(err)
	}
	if len(chat.begun) != 0 || !equalStrings(store.calls, []string{"touch"}) {
		t.Fatalf("begun = %d, run store calls = %v, want the run left waiting and rotated", len(chat.begun), store.calls)
	}
}

func TestExternalWorkflowApprovalIgnoresAMessageNoLongerWaiting(t *testing.T) {
	service, store, chat, probe := newExternalApprovalTestService(t, externalWorkflowProgress{EventType: "workflow_finished", Payload: map[string]interface{}{"status": "succeeded"}}, 1)
	chat.recoveryMessage.Status = runtimemodel.MessageStatusStreaming
	if err := service.resumeExternalWorkflowApprovals(t.Context(), service.now(), defaultOutboxBatch); err != nil {
		t.Fatal(err)
	}
	if len(probe.formIDs) != 0 || len(chat.begun) != 0 || !equalStrings(store.calls, []string{"touch"}) {
		t.Fatalf("probe = %v, begun = %d, calls = %v, want an in-flight continuation left alone", probe.formIDs, len(chat.begun), store.calls)
	}
}

func TestExternalWorkflowApprovalBoundsClaimsPerPass(t *testing.T) {
	service, _, chat, _ := newExternalApprovalTestService(t, externalWorkflowProgress{EventType: "workflow_finished", Payload: map[string]interface{}{"status": "succeeded"}}, externalWorkflowApprovalClaimsPerPass+1)
	if err := service.resumeExternalWorkflowApprovals(t.Context(), service.now(), defaultOutboxBatch); err != nil {
		t.Fatal(err)
	}
	if len(chat.begun) != externalWorkflowApprovalClaimsPerPass {
		t.Fatalf("claimed = %d, want %d per pass", len(chat.begun), externalWorkflowApprovalClaimsPerPass)
	}
}

func TestTerminalWorkflowProgressFollowsTheRunLog(t *testing.T) {
	outputs, runError := `{"decision":"approved"}`, "node failed"
	for status, want := range map[string]string{
		"succeeded": "succeeded", "partial-succeeded": "partial-succeeded", "failed": "failed", "exception": "failed", "expired": "failed",
	} {
		progress, terminal := terminalWorkflowProgress(status, &outputs, &runError)
		if !terminal || progress.EventType != "workflow_finished" || progress.Payload["status"] != want {
			t.Fatalf("%s: progress = %#v, terminal = %v", status, progress, terminal)
		}
	}
	if progress, terminal := terminalWorkflowProgress("stopped", nil, nil); !terminal || progress.EventType != "workflow_stopped" {
		t.Fatalf("stopped: progress = %#v, terminal = %v", progress, terminal)
	}
	for _, status := range []string{"running", "paused", ""} {
		if _, terminal := terminalWorkflowProgress(status, nil, nil); terminal {
			t.Fatalf("%q must not end the continuation", status)
		}
	}
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func TestWorkflowApprovalWaitingRunsListAndRotate(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{DisableForeignKeyConstraintWhenMigrating: true})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.AutoMigrate(&Run{}); err != nil {
		t.Fatal(err)
	}
	checked := time.Now().UTC().Add(-time.Minute)
	held, _ := platformWaitingRunFixture(t)
	held.Status, held.WaitingReason, held.WaitingApprovalOrigin, held.UpdatedAt = RunStatusWaiting, runtimemodel.MessageStatusWaitingApproval, workflowApprovalOrigin, checked
	platform, _ := platformWaitingRunFixture(t)
	platform.Status, platform.WaitingReason, platform.WaitingApprovalOrigin, platform.UpdatedAt = RunStatusWaiting, runtimemodel.MessageStatusWaitingApproval, platformCLIApprovalOrigin, checked
	for _, run := range []*Run{held, platform} {
		run.IdempotencyKey = run.ID.String()
		run.InvocationEvent, run.RunSpec, run.ExecutionSnapshot, run.ExecutionTrace = datatypes.JSON(`{}`), datatypes.JSON(`{}`), datatypes.JSON(`{}`), datatypes.JSON(`{}`)
		if err := db.Create(run).Error; err != nil {
			t.Fatal(err)
		}
	}
	repo, now := NewRepository(db), time.Now().UTC()
	runs, err := repo.ListWorkflowApprovalWaitingRuns(t.Context(), now, 10)
	if err != nil || len(runs) != 1 || runs[0].ID != held.ID {
		t.Fatalf("waiting runs = %#v, err %v; want only the workflow approval hold", runs, err)
	}
	if err := repo.TouchWaitingRun(t.Context(), held.ID, now); err != nil {
		t.Fatal(err)
	}
	if runs, err := repo.ListWorkflowApprovalWaitingRuns(t.Context(), now.Add(-time.Second), 10); err != nil || len(runs) != 0 {
		t.Fatalf("waiting runs after touch = %#v, err %v; want the checked run rotated out", runs, err)
	}
}

func TestGormWorkflowApprovalProbeReadsDurableWorkflowState(t *testing.T) {
	newDB := func(t *testing.T, logStatus string) *gorm.DB {
		db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{DisableForeignKeyConstraintWhenMigrating: true})
		if err != nil {
			t.Fatal(err)
		}
		sqlDB, _ := db.DB()
		t.Cleanup(func() { _ = sqlDB.Close() })
		if err := db.AutoMigrate(&workflowpause.RunPause{}, &workflowpause.RunPauseReason{}, &workflowpause.RunEvent{}); err != nil {
			t.Fatal(err)
		}
		if err := db.Exec(`CREATE TABLE workflow_run_logs (id text, status text, outputs text, error text, deleted_at datetime)`).Error; err != nil {
			t.Fatal(err)
		}
		if err := db.Exec(`INSERT INTO workflow_run_logs (id, status, outputs) VALUES ('workflow-run', ?, '{"decision":"approved"}')`, logStatus).Error; err != nil {
			t.Fatal(err)
		}
		return db
	}
	pauseOn := func(t *testing.T, db *gorm.DB, formID string) {
		pauseID := uuid.NewString()
		if err := db.Create(&workflowpause.RunPause{ID: pauseID, TenantID: uuid.NewString(), AppID: uuid.NewString(), WorkflowRunID: "workflow-run", NodeID: "approval", Reason: "approval", StateJSON: `{}`, CreatedAt: time.Now()}).Error; err != nil {
			t.Fatal(err)
		}
		if err := db.Create(&workflowpause.RunPauseReason{ID: uuid.NewString(), PauseID: pauseID, Type: "approval", FormID: formID, CreatedAt: time.Now()}).Error; err != nil {
			t.Fatal(err)
		}
		if err := db.Exec(`INSERT INTO workflow_run_events (id, tenant_id, app_id, workflow_run_id, sequence, event_type, event_data, category, occurred_at) VALUES (?, ?, ?, 'workflow-run', 7, 'approval_requested', ?, 'execution', CURRENT_TIMESTAMP)`,
			uuid.NewString(), uuid.NewString(), uuid.NewString(), `{"approval_form_id":"`+formID+`"}`).Error; err != nil {
			t.Fatal(err)
		}
	}

	finished, err := gormWorkflowApprovalProbe{db: newDB(t, "succeeded")}.Observe(t.Context(), "workflow-run", "form-1")
	if err != nil || finished.EventType != "workflow_finished" || finished.Payload["outputs"].(map[string]interface{})["decision"] != "approved" {
		t.Fatalf("finished = %#v, err %v", finished, err)
	}
	running, err := gormWorkflowApprovalProbe{db: newDB(t, "running")}.Observe(t.Context(), "workflow-run", "form-1")
	if err != nil || running.EventType != "" {
		t.Fatalf("running = %#v, err %v; want the continuation left waiting", running, err)
	}
	samePause := newDB(t, "paused")
	pauseOn(t, samePause, "form-1")
	if waiting, err := (gormWorkflowApprovalProbe{db: samePause}).Observe(t.Context(), "workflow-run", "form-1"); err != nil || waiting.EventType != "" {
		t.Fatalf("same approval = %#v, err %v; want it still waiting on its approvers", waiting, err)
	}
	nextPause := newDB(t, "paused")
	pauseOn(t, nextPause, "form-2")
	next, err := gormWorkflowApprovalProbe{db: nextPause}.Observe(t.Context(), "workflow-run", "form-1")
	if err != nil || next.EventType != "approval_requested" || next.Payload["approval_form_id"] != "form-2" {
		t.Fatalf("next approval = %#v, err %v; want the new approval request", next, err)
	}
}
