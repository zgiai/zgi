package digitalhuman

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	chatruntime "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/service"
	"github.com/zgiai/zgi/api/internal/capabilities/invocation"
	"github.com/zgiai/zgi/api/internal/capabilities/runtimeidentity"
	"github.com/zgiai/zgi/api/internal/capabilities/usersubject"
	approvalruntime "github.com/zgiai/zgi/api/internal/modules/app/workflow/approval"
	workflowpause "github.com/zgiai/zgi/api/internal/modules/app/workflow/pause"
	"gorm.io/datatypes"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type externalApprovalRunStore struct {
	*fakeRunStore
	waiting *Run
	lookups []string
	calls   []string
}

func (s *externalApprovalRunStore) RunWaitingOnWorkflowApproval(_ context.Context, workflowRunID string) (*Run, error) {
	s.lookups = append(s.lookups, workflowRunID)
	return s.waiting, nil
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
	begun          []chatruntime.Caller
	beginErr       error
	failedMessages []chatruntime.FailMessageRequest
}

func (c *externalApprovalChat) FailMessage(_ context.Context, _ chatruntime.Scope, req chatruntime.FailMessageRequest) (*runtimemodel.Message, error) {
	c.failedMessages = append(c.failedMessages, req)
	return &runtimemodel.Message{ID: req.MessageID, Status: runtimemodel.MessageStatusError}, nil
}

func (c *externalApprovalChat) BeginExternalWorkflowApprovalContinuation(_ context.Context, _ chatruntime.Scope, caller chatruntime.Caller, _ chatruntime.RunConfig, conversationID, messageID uuid.UUID) (*chatruntime.WorkflowApprovalContinuation, error) {
	if c.beginErr != nil {
		return nil, c.beginErr
	}
	c.begun = append(c.begun, caller)
	return &chatruntime.WorkflowApprovalContinuation{
		ConversationID: conversationID, MessageID: messageID, WorkflowRunID: "workflow-run",
		MessageStatus: runtimemodel.MessageStatusWaitingApproval,
	}, nil
}

type workflowEvent struct {
	eventType string
	payload   map[string]interface{}
}

// streamingApprovalRunner resumes the approval by replaying workflow events.
type streamingApprovalRunner struct {
	events  []workflowEvent
	err     error
	resumed []string
}

func (r *streamingApprovalRunner) ResumeApprovalWorkflow(context.Context, *approvalruntime.Form) error {
	return nil
}

func (r *streamingApprovalRunner) ResumeQuestionAnswerWorkflow(context.Context, string, map[string]interface{}) error {
	return nil
}

func (r *streamingApprovalRunner) ResumeApprovalWorkflowStream(_ context.Context, form *approvalruntime.Form, onEvent func(string, map[string]interface{}) error) error {
	r.resumed = append(r.resumed, form.ID)
	for _, event := range r.events {
		_ = onEvent(event.eventType, event.payload)
	}
	return r.err
}

func (r *streamingApprovalRunner) ResumeQuestionAnswerWorkflowStream(context.Context, string, map[string]interface{}, func(string, map[string]interface{}) error) error {
	return nil
}

type externalApprovalFixture struct {
	service *RuntimeService
	store   *externalApprovalRunStore
	chat    *externalApprovalChat
	runner  *streamingApprovalRunner
	form    *approvalruntime.Form
}

// newExternalApprovalFixture is a WebApp run held by a workflow approval that
// belongs to email approvers.
func newExternalApprovalFixture(t *testing.T, runner *streamingApprovalRunner) externalApprovalFixture {
	t.Helper()
	store := &externalApprovalRunStore{fakeRunStore: newFakeRunStore()}
	chat := &externalApprovalChat{fakeDigitalHumanChat: &fakeDigitalHumanChat{}, fakeWorkflowContinuationRuntime: &fakeWorkflowContinuationRuntime{}}
	now := time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)
	service := NewRuntimeService(store, fakeExecutionResolver{now: now}, fakeBrainAdapter{}, chat, &fakeRunDispatcher{},
		WithChannelTransientDispatcher(&channelTransientDispatcherStub{}))
	service.now = func() time.Time { return now }
	service.workflowDB, service.workflowRunner = &gorm.DB{}, runner

	scope, digitalHumanID, subjectID := testRunScope(), uuid.New(), uuid.New()
	scope.ActorID, scope.PrincipalID = subjectID, subjectID
	scope.PrincipalType, scope.Surface = string(invocation.PrincipalSystem), SurfaceTask
	scope.UserSubjectKind, scope.UserSubjectID = string(usersubject.APIEndUser), &subjectID
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
	conversationID, messageID := uuid.New(), uuid.New()
	run.Surface, run.Status = SurfaceWebApp, RunStatusWaiting
	run.WaitingReason, run.WaitingApprovalOrigin = runtimemodel.MessageStatusWaitingApproval, workflowApprovalOrigin
	run.ConversationID, run.MessageID = &conversationID, &messageID
	store.waiting = run
	return externalApprovalFixture{
		service: service, store: store, chat: chat, runner: runner,
		form: &approvalruntime.Form{ID: "form-1", WorkflowRunID: "workflow-run"},
	}
}

func TestExternalApprovalResumesTheWorkflowAndTheConversation(t *testing.T) {
	f := newExternalApprovalFixture(t, &streamingApprovalRunner{events: []workflowEvent{
		{"node_finished", map[string]interface{}{"node_id": "approval"}},
		{"workflow_finished", map[string]interface{}{"status": "succeeded", "outputs": map[string]interface{}{"decision": "approved"}}},
	}})
	f.chat.summaryResult = &chatruntime.ChatResult{Status: runtimemodel.MessageStatusCompleted, Answer: "The purchase was approved.", Metadata: map[string]interface{}{}}
	handled, err := f.service.ResumeApprovalForm(t.Context(), f.form)
	if !handled || err != nil {
		t.Fatalf("ResumeApprovalForm() = %v, %v", handled, err)
	}
	if !equalStrings(f.store.lookups, []string{"workflow-run"}) || !equalStrings(f.runner.resumed, []string{"form-1"}) {
		t.Fatalf("lookups = %v, resumed = %v", f.store.lookups, f.runner.resumed)
	}
	if len(f.chat.begun) != 1 || f.chat.begun[0].Source != digitalHumanWebAppConversationSource || f.chat.begun[0].RunID == nil {
		t.Fatalf("claimed continuations = %#v, want one as the WebApp run", f.chat.begun)
	}
	if !equalStrings(f.chat.recorded, []string{"node_finished", "workflow_finished"}) {
		t.Fatalf("recorded workflow events = %v, want them on the message as they ran", f.chat.recorded)
	}
	if len(f.chat.summarized) != 1 || f.chat.summarized[0].Outputs["decision"] != "approved" {
		t.Fatalf("summaries = %#v, want the workflow outcome summarized", f.chat.summarized)
	}
	if want := []string{"resume", "complete:The purchase was approved."}; !equalStrings(f.store.calls, want) {
		t.Fatalf("run store calls = %v, want %v", f.store.calls, want)
	}
}

func TestExternalApprovalFailsTheRunWhenTheWorkflowFailed(t *testing.T) {
	f := newExternalApprovalFixture(t, &streamingApprovalRunner{events: []workflowEvent{
		{"workflow_finished", map[string]interface{}{"status": "failed", "error": "node failed"}},
	}})
	if handled, err := f.service.ResumeApprovalForm(t.Context(), f.form); !handled || err != nil {
		t.Fatalf("ResumeApprovalForm() = %v, %v", handled, err)
	}
	if want := []string{"resume", "fail:digital_human.run.execution_failed"}; !equalStrings(f.store.calls, want) {
		t.Fatalf("run store calls = %v, want %v", f.store.calls, want)
	}
}

func TestExternalApprovalMovesTheConversationToTheNextRequest(t *testing.T) {
	f := newExternalApprovalFixture(t, &streamingApprovalRunner{events: []workflowEvent{
		{"question_answer_requested", map[string]interface{}{"question": "Which budget?"}},
	}})
	if handled, err := f.service.ResumeApprovalForm(t.Context(), f.form); !handled || err != nil {
		t.Fatalf("ResumeApprovalForm() = %v, %v", handled, err)
	}
	if len(f.chat.paused) != 1 || f.chat.paused[0] != runtimemodel.MessageStatusWaitingQuestion {
		t.Fatalf("paused = %v, want the message waiting on the new question", f.chat.paused)
	}
	if want := []string{"resume", "waiting:" + runtimemodel.MessageStatusWaitingQuestion}; !equalStrings(f.store.calls, want) {
		t.Fatalf("run store calls = %v, want %v", f.store.calls, want)
	}
}

func TestExternalApprovalKeepsWaitingWhenTheWorkflowDoesNotResume(t *testing.T) {
	// A timeout of one approver while others on the same pause are pending.
	f := newExternalApprovalFixture(t, &streamingApprovalRunner{err: workflowpause.ErrPauseNotResumeReady})
	handled, err := f.service.ResumeApprovalForm(t.Context(), f.form)
	if !handled || err == nil {
		t.Fatalf("ResumeApprovalForm() = %v, %v; want the resume error reported", handled, err)
	}
	if len(f.chat.paused) != 1 || f.chat.paused[0] != runtimemodel.MessageStatusWaitingApproval || len(f.chat.failed) != 0 {
		t.Fatalf("paused = %v, failed = %v; want the message waiting again", f.chat.paused, f.chat.failed)
	}
	if want := []string{"resume", "waiting:" + runtimemodel.MessageStatusWaitingApproval}; !equalStrings(f.store.calls, want) {
		t.Fatalf("run store calls = %v, want %v", f.store.calls, want)
	}
}

func TestExternalApprovalLeavesUnownedOrClaimedFormsToTheWorkflow(t *testing.T) {
	unowned := newExternalApprovalFixture(t, &streamingApprovalRunner{})
	unowned.store.waiting = nil
	if handled, err := unowned.service.ResumeApprovalForm(t.Context(), unowned.form); handled || err != nil {
		t.Fatalf("no waiting run: ResumeApprovalForm() = %v, %v", handled, err)
	}
	claimed := newExternalApprovalFixture(t, &streamingApprovalRunner{})
	claimed.chat.beginErr = chatruntime.ErrInvalidInput
	if handled, err := claimed.service.ResumeApprovalForm(t.Context(), claimed.form); handled || err != nil {
		t.Fatalf("claimed in the conversation: ResumeApprovalForm() = %v, %v", handled, err)
	}
	for name, f := range map[string]externalApprovalFixture{"unowned": unowned, "claimed": claimed} {
		if len(f.runner.resumed) != 0 || len(f.store.calls) != 0 {
			t.Fatalf("%s: resumed = %v, calls = %v; want the workflow left to its own resume", name, f.runner.resumed, f.store.calls)
		}
	}
}

func TestExternalApprovalReportsATemporaryTakeoverFailure(t *testing.T) {
	f := newExternalApprovalFixture(t, &streamingApprovalRunner{})
	f.chat.beginErr = errors.New("connection reset")
	handled, err := f.service.ResumeApprovalForm(t.Context(), f.form)
	if handled || err == nil {
		t.Fatalf("ResumeApprovalForm() = %v, %v; want the failure reported so the resume queue retries", handled, err)
	}
	if len(f.runner.resumed) != 0 || len(f.store.calls) != 0 {
		t.Fatalf("resumed = %v, calls = %v; want nothing resumed or settled", f.runner.resumed, f.store.calls)
	}
}

func TestAbandonedExternalApprovalFailsTheWaitingConversation(t *testing.T) {
	f := newExternalApprovalFixture(t, &streamingApprovalRunner{})
	f.service.AbandonApprovalForm(t.Context(), f.form, errors.New("model unavailable"))
	if len(f.chat.failedMessages) != 1 || f.chat.failedMessages[0].MessageID != *f.store.waiting.MessageID ||
		f.chat.failedMessages[0].ErrorMessage != workflowApprovalAbandonedMessage {
		t.Fatalf("failed messages = %#v, want the waiting message failed with the public notice", f.chat.failedMessages)
	}
	if want := []string{"fail:digital_human.run.execution_failed"}; !equalStrings(f.store.calls, want) {
		t.Fatalf("run store calls = %v, want %v", f.store.calls, want)
	}
	if len(f.runner.resumed) != 0 {
		t.Fatalf("resumed = %v; the workflow resumes through the approval fallback, not the host", f.runner.resumed)
	}

	unowned := newExternalApprovalFixture(t, &streamingApprovalRunner{})
	unowned.store.waiting = nil
	unowned.service.AbandonApprovalForm(t.Context(), unowned.form, errors.New("model unavailable"))
	if len(unowned.chat.failedMessages) != 0 || len(unowned.store.calls) != 0 {
		t.Fatalf("no waiting run: failed = %v, calls = %v", unowned.chat.failedMessages, unowned.store.calls)
	}
}

func TestRunWaitingOnWorkflowApprovalMatchesTheMessageWorkflowRun(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{DisableForeignKeyConstraintWhenMigrating: true})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.AutoMigrate(&Run{}, &runtimemodel.Message{}); err != nil {
		t.Fatal(err)
	}
	create := func(origin, workflowRunID string) *Run {
		run, message := platformWaitingRunFixture(t)
		run.Status, run.WaitingReason, run.WaitingApprovalOrigin = RunStatusWaiting, runtimemodel.MessageStatusWaitingApproval, origin
		run.IdempotencyKey = run.ID.String()
		run.InvocationEvent, run.RunSpec, run.ExecutionSnapshot, run.ExecutionTrace = datatypes.JSON(`{}`), datatypes.JSON(`{}`), datatypes.JSON(`{}`), datatypes.JSON(`{}`)
		message.Metadata = map[string]interface{}{"agent_workflow_continuation": map[string]interface{}{"workflow_run_id": workflowRunID}}
		if err := db.Create(run).Error; err != nil {
			t.Fatal(err)
		}
		if err := db.Select("id", "conversation_id", "status", "metadata", "query", "model_name", "updated_at").Create(message).Error; err != nil {
			t.Fatal(err)
		}
		return run
	}
	held := create(workflowApprovalOrigin, "workflow-run-1")
	create(workflowApprovalOrigin, "workflow-run-2")
	create(platformCLIApprovalOrigin, "workflow-run-3")
	repo := NewRepository(db)
	run, err := repo.RunWaitingOnWorkflowApproval(t.Context(), "workflow-run-1")
	if err != nil || run == nil || run.ID != held.ID {
		t.Fatalf("RunWaitingOnWorkflowApproval() = %#v, %v; want the run of that workflow", run, err)
	}
	for _, workflowRunID := range []string{"workflow-run-3", "missing", ""} {
		if run, err := repo.RunWaitingOnWorkflowApproval(t.Context(), workflowRunID); err != nil || run != nil {
			t.Fatalf("%q: run = %#v, err %v; want none", workflowRunID, run, err)
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
