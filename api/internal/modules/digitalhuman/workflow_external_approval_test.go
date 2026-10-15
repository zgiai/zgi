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
	retaken bool
	lookups []string
	calls   []string
}

func (s *externalApprovalRunStore) RunHeldByWorkflowApproval(_ context.Context, workflowRunID string) (*Run, bool, error) {
	s.lookups = append(s.lookups, workflowRunID)
	return s.waiting, s.retaken, nil
}

type fakeWorkflowProgressReader struct {
	progress []externalWorkflowProgress
}

func (r fakeWorkflowProgressReader) Read(context.Context, string) ([]externalWorkflowProgress, error) {
	return r.progress, nil
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
	events          []workflowEvent
	err             error
	resumed         []string
	questionResumes []map[string]interface{}
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

func (r *streamingApprovalRunner) ResumeQuestionAnswerWorkflowStream(_ context.Context, workflowRunID string, inputs map[string]interface{}, onEvent func(string, map[string]interface{}) error) error {
	r.questionResumes = append(r.questionResumes, inputs)
	for _, event := range r.events {
		_ = onEvent(event.eventType, event.payload)
	}
	return r.err
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

func (f externalApprovalFixture) request() approvalruntime.ResumeRequest {
	return approvalruntime.ResumeRequest{WorkflowRunID: f.form.WorkflowRunID, Form: f.form}
}

func TestExternalApprovalResumesTheWorkflowAndTheConversation(t *testing.T) {
	f := newExternalApprovalFixture(t, &streamingApprovalRunner{events: []workflowEvent{
		{"node_finished", map[string]interface{}{"node_id": "approval"}},
		{"workflow_finished", map[string]interface{}{"status": "succeeded", "outputs": map[string]interface{}{"decision": "approved"}}},
	}})
	f.chat.summaryResult = &chatruntime.ChatResult{Status: runtimemodel.MessageStatusCompleted, Answer: "The purchase was approved.", Metadata: map[string]interface{}{}}
	handled, err := f.service.ResumeWorkflow(t.Context(), f.request())
	if !handled || err != nil {
		t.Fatalf("ResumeWorkflow() = %v, %v", handled, err)
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

func TestQuestionResumeOfAMixedPauseIsFollowedByTheConversation(t *testing.T) {
	// The question was answered first; the approval then completed the pause,
	// so the workflow resumes as a question resume carrying the answer.
	f := newExternalApprovalFixture(t, &streamingApprovalRunner{events: []workflowEvent{
		{"workflow_finished", map[string]interface{}{"status": "succeeded", "outputs": map[string]interface{}{"budget": "Q4"}}},
	}})
	f.chat.summaryResult = &chatruntime.ChatResult{Status: runtimemodel.MessageStatusCompleted, Answer: "Approved for Q4.", Metadata: map[string]interface{}{}}
	inputs := map[string]interface{}{"query": "Q4", "question_answer_node_id": "question-node"}
	handled, err := f.service.ResumeWorkflow(t.Context(), approvalruntime.ResumeRequest{WorkflowRunID: "workflow-run", QuestionInputs: inputs})
	if !handled || err != nil {
		t.Fatalf("ResumeWorkflow() = %v, %v", handled, err)
	}
	if len(f.runner.resumed) != 0 || len(f.runner.questionResumes) != 1 || f.runner.questionResumes[0]["query"] != "Q4" {
		t.Fatalf("approval resumes = %v, question resumes = %v; want the question answers kept", f.runner.resumed, f.runner.questionResumes)
	}
	if want := []string{"resume", "complete:Approved for Q4."}; !equalStrings(f.store.calls, want) {
		t.Fatalf("run store calls = %v, want %v", f.store.calls, want)
	}
}

func TestExternalApprovalFailsTheRunWhenTheWorkflowFailed(t *testing.T) {
	f := newExternalApprovalFixture(t, &streamingApprovalRunner{events: []workflowEvent{
		{"workflow_finished", map[string]interface{}{"status": "failed", "error": "node failed"}},
	}})
	if handled, err := f.service.ResumeWorkflow(t.Context(), f.request()); !handled || err != nil {
		t.Fatalf("ResumeWorkflow() = %v, %v", handled, err)
	}
	if want := []string{"resume", "fail:digital_human.run.execution_failed"}; !equalStrings(f.store.calls, want) {
		t.Fatalf("run store calls = %v, want %v", f.store.calls, want)
	}
}

func TestExternalApprovalMovesTheConversationToTheNextRequest(t *testing.T) {
	f := newExternalApprovalFixture(t, &streamingApprovalRunner{events: []workflowEvent{
		{"question_answer_requested", map[string]interface{}{"question": "Which budget?"}},
	}})
	if handled, err := f.service.ResumeWorkflow(t.Context(), f.request()); !handled || err != nil {
		t.Fatalf("ResumeWorkflow() = %v, %v", handled, err)
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
	handled, err := f.service.ResumeWorkflow(t.Context(), f.request())
	if !handled || err == nil {
		t.Fatalf("ResumeWorkflow() = %v, %v; want the resume error reported", handled, err)
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
	if handled, err := unowned.service.ResumeWorkflow(t.Context(), unowned.request()); handled || err != nil {
		t.Fatalf("no waiting run: ResumeWorkflow() = %v, %v", handled, err)
	}
	claimed := newExternalApprovalFixture(t, &streamingApprovalRunner{})
	claimed.chat.beginErr = chatruntime.ErrInvalidInput
	if handled, err := claimed.service.ResumeWorkflow(t.Context(), claimed.request()); handled || err != nil {
		t.Fatalf("claimed in the conversation: ResumeWorkflow() = %v, %v", handled, err)
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
	handled, err := f.service.ResumeWorkflow(t.Context(), f.request())
	if handled || err == nil {
		t.Fatalf("ResumeWorkflow() = %v, %v; want the failure reported so the resume queue retries", handled, err)
	}
	if len(f.runner.resumed) != 0 || len(f.store.calls) != 0 {
		t.Fatalf("resumed = %v, calls = %v; want nothing resumed or settled", f.runner.resumed, f.store.calls)
	}
}

func TestAbandonedExternalApprovalFailsTheWaitingConversation(t *testing.T) {
	f := newExternalApprovalFixture(t, &streamingApprovalRunner{})
	f.service.AbandonWorkflowResume(t.Context(), f.request(), errors.New("model unavailable"))
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
	unowned.service.AbandonWorkflowResume(t.Context(), unowned.request(), errors.New("model unavailable"))
	if len(unowned.chat.failedMessages) != 0 || len(unowned.store.calls) != 0 {
		t.Fatalf("no waiting run: failed = %v, calls = %v", unowned.chat.failedMessages, unowned.store.calls)
	}
}

func TestRunHeldByWorkflowApprovalFindsWaitingAndClaimedRuns(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{DisableForeignKeyConstraintWhenMigrating: true})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.AutoMigrate(&Run{}, &runtimemodel.Message{}); err != nil {
		t.Fatal(err)
	}
	create := func(runStatus RunStatus, origin, messageStatus, workflowRunID string, externalResume bool) *Run {
		run, message := platformWaitingRunFixture(t)
		run.Status, run.WaitingApprovalOrigin = runStatus, origin
		if runStatus == RunStatusWaiting {
			run.WaitingReason = runtimemodel.MessageStatusWaitingApproval
		}
		run.IdempotencyKey = run.ID.String()
		run.InvocationEvent, run.RunSpec, run.ExecutionSnapshot, run.ExecutionTrace = datatypes.JSON(`{}`), datatypes.JSON(`{}`), datatypes.JSON(`{}`), datatypes.JSON(`{}`)
		continuation := map[string]interface{}{"workflow_run_id": workflowRunID}
		if externalResume {
			continuation["external_resume"] = true
		}
		message.Status, message.Metadata = messageStatus, map[string]interface{}{"agent_workflow_continuation": continuation}
		if err := db.Create(run).Error; err != nil {
			t.Fatal(err)
		}
		if err := db.Select("id", "conversation_id", "status", "metadata", "query", "model_name", "updated_at").Create(message).Error; err != nil {
			t.Fatal(err)
		}
		return run
	}
	waiting := create(RunStatusWaiting, workflowApprovalOrigin, runtimemodel.MessageStatusWaitingApproval, "workflow-run-1", false)
	create(RunStatusWaiting, workflowApprovalOrigin, runtimemodel.MessageStatusWaitingApproval, "workflow-run-2", false)
	create(RunStatusWaiting, platformCLIApprovalOrigin, runtimemodel.MessageStatusWaitingApproval, "workflow-run-3", false)
	claimed := create(RunStatusRunning, "", runtimemodel.MessageStatusStreaming, "workflow-run-4", true)
	create(RunStatusRunning, "", runtimemodel.MessageStatusStreaming, "workflow-run-5", false)
	repo := NewRepository(db)
	if run, retaken, err := repo.RunHeldByWorkflowApproval(t.Context(), "workflow-run-1"); err != nil || run == nil || run.ID != waiting.ID || retaken {
		t.Fatalf("waiting run = %#v, retaken %v, err %v", run, retaken, err)
	}
	if run, retaken, err := repo.RunHeldByWorkflowApproval(t.Context(), "workflow-run-4"); err != nil || run == nil || run.ID != claimed.ID || !retaken {
		t.Fatalf("externally claimed run = %#v, retaken %v, err %v; want it retaken", run, retaken, err)
	}
	for _, workflowRunID := range []string{"workflow-run-3", "workflow-run-5", "missing", ""} {
		if run, _, err := repo.RunHeldByWorkflowApproval(t.Context(), workflowRunID); err != nil || run != nil {
			t.Fatalf("%q: run = %#v, err %v; want none (a platform approval, or a continuation the conversation runs)", workflowRunID, run, err)
		}
	}
}

func TestRetakenExternalApprovalFollowsTheWorkflowWithoutReleasingTheMessage(t *testing.T) {
	retake := func(t *testing.T, runner *streamingApprovalRunner, progress []externalWorkflowProgress) (externalApprovalFixture, bool, error) {
		f := newExternalApprovalFixture(t, runner)
		f.store.waiting.Status = RunStatusRunning
		f.store.retaken = true
		f.service.progressReader = fakeWorkflowProgressReader{progress: progress}
		f.chat.summaryResult = &chatruntime.ChatResult{Status: runtimemodel.MessageStatusCompleted, Answer: "Done.", Metadata: map[string]interface{}{}}
		handled, err := f.service.ResumeWorkflow(t.Context(), f.request())
		return f, handled, err
	}

	f, handled, err := retake(t, &streamingApprovalRunner{events: []workflowEvent{{"workflow_finished", map[string]interface{}{"status": "succeeded"}}}}, nil)
	if !handled || err != nil || !equalStrings(f.store.calls, []string{"complete:Done."}) {
		t.Fatalf("retaken resume: handled %v, err %v, calls %v; want it completed without claiming the run again", handled, err, f.store.calls)
	}

	f, handled, err = retake(t, &streamingApprovalRunner{err: workflowpause.ErrResumeAlreadyRunning}, nil)
	if !handled || !errors.Is(err, workflowpause.ErrResumeAlreadyRunning) || len(f.store.calls) != 0 || len(f.chat.paused) != 0 {
		t.Fatalf("original worker alive: handled %v, err %v, calls %v, paused %v; want a retry with nothing touched", handled, err, f.store.calls, f.chat.paused)
	}

	f, handled, err = retake(t, &streamingApprovalRunner{err: workflowpause.ErrPauseNotFound},
		[]externalWorkflowProgress{{EventType: "workflow_finished", Payload: map[string]interface{}{"status": "succeeded"}}})
	if !handled || err != nil || !equalStrings(f.store.calls, []string{"complete:Done."}) {
		t.Fatalf("workflow already finished: handled %v, err %v, calls %v; want the conversation settled from the workflow", handled, err, f.store.calls)
	}

	f, handled, err = retake(t, &streamingApprovalRunner{err: workflowpause.ErrPauseNotFound}, nil)
	if !handled || !errors.Is(err, errWorkflowResumedElsewhere) || errors.Is(err, workflowpause.ErrPauseNotFound) || len(f.store.calls) != 0 || len(f.chat.paused) != 0 {
		t.Fatalf("workflow running elsewhere: handled %v, err %v, calls %v; want a retried error the queue does not drop", handled, err, f.store.calls)
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

type recoveringRunStore struct {
	*externalApprovalRunStore
	recoverable []*Run
}

func (s *recoveringRunStore) ListRecoverableRuns(context.Context, time.Time, int) ([]*Run, error) {
	return s.recoverable, nil
}

func TestRecoveredFailedWorkflowContinuationFailsTheRun(t *testing.T) {
	for name, test := range map[string]struct {
		status string
		want   string
	}{
		"failed workflow":    {"failed", "fail:" + workflowContinuationFailedCode},
		"succeeded workflow": {"succeeded", "complete:Summary of the workflow."},
	} {
		t.Run(name, func(t *testing.T) {
			f := newExternalApprovalFixture(t, &streamingApprovalRunner{})
			run := f.store.waiting
			actorID := uuid.New()
			run.ActorID, run.Status = &actorID, RunStatusRunning
			store := &recoveringRunStore{externalApprovalRunStore: f.store, recoverable: []*Run{run}}
			f.service.store = store
			f.chat.recoveryMessage = &runtimemodel.Message{
				ID: *run.MessageID, ConversationID: *run.ConversationID, Status: runtimemodel.MessageStatusCompleted, Answer: "Summary of the workflow.",
				Metadata: map[string]interface{}{
					"agent_workflow_continuation": map[string]interface{}{"workflow_run_id": "workflow-run"},
					"workflow_runs":               []interface{}{map[string]interface{}{"workflow_run_id": "workflow-run", "status": test.status}},
				},
			}
			if err := f.service.recoverCompletedRuns(t.Context(), time.Now(), 10); err != nil {
				t.Fatal(err)
			}
			if !equalStrings(f.store.calls, []string{test.want}) {
				t.Fatalf("run store calls = %v, want [%s]", f.store.calls, test.want)
			}
		})
	}
}
