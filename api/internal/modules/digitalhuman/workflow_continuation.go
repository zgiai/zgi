package digitalhuman

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	chatruntime "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/service"
	approvalruntime "github.com/zgiai/zgi/api/internal/modules/app/workflow/approval"
	"github.com/zgiai/zgi/api/pkg/logger"
)

var ErrDigitalHumanTaskContinuation = errors.New("digital human task continuation")

func (s *RuntimeService) WorkflowContinuationEnabled() bool {
	return s != nil && s.workflowDB != nil && s.workflowRunner != nil
}

type workflowContinuationChatRuntime interface {
	BeginWorkflowApprovalContinuation(context.Context, chatruntime.Scope, chatruntime.Caller, chatruntime.RunConfig, uuid.UUID, uuid.UUID) (*chatruntime.WorkflowApprovalContinuation, error)
	RecordWorkflowApprovalContinuationEvent(context.Context, *chatruntime.WorkflowApprovalContinuation, string, map[string]interface{}) (*chatruntime.StreamEvent, error)
	AppendWorkflowApprovalContinuationStreamEvent(context.Context, *chatruntime.WorkflowApprovalContinuation, string, map[string]interface{}) (*chatruntime.StreamEvent, error)
	PauseWorkflowApprovalContinuation(context.Context, *chatruntime.WorkflowApprovalContinuation, string) (map[string]interface{}, error)
	CompleteWorkflowApprovalContinuation(context.Context, *chatruntime.WorkflowApprovalContinuation, string, string) (map[string]interface{}, error)
	SummarizeWorkflowApprovalContinuation(context.Context, chatruntime.Scope, *chatruntime.WorkflowApprovalContinuation, chatruntime.WorkflowContinuationSummaryRequest, func(chatruntime.StreamEvent) error) (*chatruntime.ChatResult, error)
	FailWorkflowApprovalContinuation(context.Context, *chatruntime.WorkflowApprovalContinuation, string) (map[string]interface{}, error)
}

// The production ChatRuntime is chatruntime.Service; keep the continuation
// lifecycle on the same contract the Agent continuation uses.
var _ workflowContinuationChatRuntime = chatruntime.Service(nil)

// ContinueWorkflowApproval resumes a generic workflow approval or question
// continuation. Task approvals deliberately return a sentinel so the caller
// can keep using the task-specific mutation path.
func (s *RuntimeService) ContinueWorkflowApproval(
	ctx context.Context,
	scope Scope,
	digitalHumanID, conversationID, messageID uuid.UUID,
	request chatruntime.DigitalHumanTaskApprovalRequest,
	onEvent func(chatruntime.StreamEvent) error,
) error {
	_, err := s.continueWorkflowApproval(ctx, scope, digitalHumanID, conversationID, messageID, request, onEvent)
	return err
}

// continueWorkflowApproval is ContinueWorkflowApproval that also reports the
// final answer, so surfaces without an event stream (channels) can deliver
// the result to the user.
func (s *RuntimeService) continueWorkflowApproval(
	ctx context.Context,
	scope Scope,
	digitalHumanID, conversationID, messageID uuid.UUID,
	request chatruntime.DigitalHumanTaskApprovalRequest,
	onEvent func(chatruntime.StreamEvent) error,
) (string, error) {
	if s == nil || !s.WorkflowContinuationEnabled() {
		return "", fmt.Errorf("digital human workflow continuation is unavailable")
	}
	_, runtimeScope, caller, err := s.conversationContext(scope, digitalHumanID)
	if err != nil {
		return "", err
	}
	run, config, err := s.workflowContinuationConfig(ctx, scope, digitalHumanID, conversationID, messageID)
	if err != nil {
		return "", err
	}
	// The resumed Agent loop (summary, connector tools, workspace) is owned by
	// this run, exactly like the other DigitalHuman continuations.
	caller.RunID = cloneUUIDPointer(&run.ID)
	heldRun := workflowHeldRun(run)
	runtime, ok := s.chat.(workflowContinuationChatRuntime)
	if !ok {
		return "", fmt.Errorf("digital human workflow continuation runtime is unavailable")
	}
	continuation, err := runtime.BeginWorkflowApprovalContinuation(ctx, runtimeScope, caller, config, conversationID, messageID)
	if err != nil {
		return "", err
	}
	if continuation.Kind == "digital_human_task" {
		return "", ErrDigitalHumanTaskContinuation
	}
	if continuation.Completed {
		return "", nil
	}
	// BeginWorkflowApprovalContinuation moved the message to streaming. Every
	// exit below must leave it paused again, completed or failed, otherwise the
	// conversation stays blocked on a message no run owns.
	lifecycle := &digitalHumanWorkflowContinuation{
		ctx: context.WithoutCancel(ctx), runtime: runtime, scope: runtimeScope, continuation: continuation, onEvent: onEvent,
	}
	if heldRun != nil {
		lifecycle.settle = func(settleCtx context.Context, status, answer string, metadata map[string]interface{}) {
			s.settleWorkflowContinuationRun(settleCtx, heldRun, status, answer, metadata)
		}
		if err := s.resumeHeldWorkflowRun(ctx, heldRun); err != nil {
			return "", lifecycle.abortBeforeResume(err)
		}
	}
	if err := lifecycle.emit(chatruntime.StreamEvent{EventType: "message_start", Payload: map[string]interface{}{
		"conversation_id": conversationID.String(), "message_id": messageID.String(),
		"workflow_run_id": continuation.WorkflowRunID, "continuation": true,
	}}); err != nil {
		return "", lifecycle.abortBeforeResume(err)
	}
	streamed, err := s.resumeWorkflowContinuation(ctx, runtimeScope, continuation, request, lifecycle.handle)
	err = lifecycle.finish(streamed, err)
	return lifecycle.answer, err
}

// resumeWorkflowContinuation submits the approval or question answer and runs
// the resumed workflow. streamed reports whether workflow events were
// observed through onEvent; resumed=false with a nil error means the approval
// was recorded but the workflow is still waiting for other approvers.
func (s *RuntimeService) resumeWorkflowContinuation(
	ctx context.Context,
	runtimeScope chatruntime.Scope,
	continuation *chatruntime.WorkflowApprovalContinuation,
	request chatruntime.DigitalHumanTaskApprovalRequest,
	onEvent func(string, map[string]interface{}) error,
) (workflowContinuationResume, error) {
	typeValue := strings.ToLower(strings.TrimSpace(request.Type))
	if typeValue == "" {
		typeValue = "approval"
	}
	runner, streaming := s.workflowRunner.(workflowContinuationStreamRunner)
	if typeValue == "question_answer" {
		inputs := request.Inputs
		if len(inputs) == 0 {
			return workflowContinuationNotResumed, fmt.Errorf("%w: question answer inputs are required", chatruntime.ErrInvalidInput)
		}
		if streaming {
			return workflowContinuationStreamed, runner.ResumeQuestionAnswerWorkflowStream(ctx, continuation.WorkflowRunID, inputs, onEvent)
		}
		return workflowContinuationUnobserved, s.workflowRunner.ResumeQuestionAnswerWorkflow(ctx, continuation.WorkflowRunID, inputs)
	}
	if strings.TrimSpace(request.ApprovalToken) == "" || strings.TrimSpace(request.Action) == "" {
		return workflowContinuationNotResumed, fmt.Errorf("%w: approval token and action are required", chatruntime.ErrInvalidInput)
	}
	accountID := runtimeScope.AccountID.String()
	callerSource := strings.ToLower(strings.TrimSpace(continuation.Caller.Source))
	submission, err := approvalruntime.NewService(s.workflowDB).SubmitByTokenForWorkflowRunWithResumeOptions(
		ctx, strings.TrimSpace(request.ApprovalToken), continuation.WorkflowRunID,
		approvalruntime.SubmitRequest{Inputs: request.Inputs, Action: strings.TrimSpace(request.Action)},
		&accountID, nil, approvalruntime.SubmitOptions{
			RequireWebAppEnabled: callerSource == runtimemodel.ConversationSourceWebApp || callerSource == runtimemodel.ConversationSourceExternalAPI,
		},
	)
	if err != nil {
		return workflowContinuationNotResumed, err
	}
	if submission == nil || !submission.ResumeReady {
		return workflowContinuationNotResumed, nil
	}
	if streaming {
		return workflowContinuationStreamed, runner.ResumeApprovalWorkflowStream(ctx, submission.Form, onEvent)
	}
	return workflowContinuationUnobserved, s.workflowRunner.ResumeApprovalWorkflow(ctx, submission.Form)
}

type workflowContinuationResume int

const (
	// workflowContinuationNotResumed: the workflow did not run (validation
	// failure, or an approval that still waits for other approvers).
	workflowContinuationNotResumed workflowContinuationResume = iota
	// workflowContinuationStreamed: workflow events were delivered to handle.
	workflowContinuationStreamed
	// workflowContinuationUnobserved: the workflow ran without an event stream.
	workflowContinuationUnobserved
)

// workflowApprovalExpiredInstruction tells the summary how an approval that
// timed out ended; the workflow outputs may still carry the unanswered form.
const workflowApprovalExpiredInstruction = "The workflow approval expired before any approver approved or rejected it, and the workflow continued on its expiry path. Report that the approval timed out without a decision. Do not describe the request as approved, rejected, or still waiting for approval."

// digitalHumanWorkflowContinuation mirrors the Agent continuation lifecycle:
// it records resumed workflow events on the message, pauses again on a new
// approval or question, and completes or fails the message when the
// workflow reaches a terminal state.
type digitalHumanWorkflowContinuation struct {
	// ctx keeps request values but not cancellation, so persistence finishes
	// even when the client disconnects mid-stream.
	ctx          context.Context
	runtime      workflowContinuationChatRuntime
	scope        chatruntime.Scope
	continuation *chatruntime.WorkflowApprovalContinuation
	onEvent      func(chatruntime.StreamEvent) error

	// settle records where the message ended on the DigitalHuman run. It is nil
	// when the run is not held by this workflow approval (channel and background
	// runs keep their own lifecycle).
	settle func(ctx context.Context, status, answer string, metadata map[string]interface{})

	waitingStatus string
	terminal      bool
	// approvalExpired records that an approval timed out without a decision, so
	// the summary does not read the leftover approval form as still pending.
	approvalExpired bool
	runStatus       string
	runOutputs      map[string]interface{}
	runError        string
	// answer is the final assistant answer once the continuation completes.
	answer string
}

func (l *digitalHumanWorkflowContinuation) handle(rawEventType string, rawPayload map[string]interface{}) error {
	eventType := strings.TrimSpace(rawEventType)
	if eventType == "error" {
		eventType = "workflow_failed"
	}
	if eventType == "" {
		return nil
	}
	ctx := l.ctx
	payload := copyWorkflowContinuationPayload(rawPayload)
	payload["workflow_run_id"] = l.continuation.WorkflowRunID
	event, err := l.runtime.RecordWorkflowApprovalContinuationEvent(ctx, l.continuation, eventType, payload)
	if err != nil {
		event, err = l.runtime.AppendWorkflowApprovalContinuationStreamEvent(ctx, l.continuation, eventType, payload)
		if err != nil {
			event = &chatruntime.StreamEvent{EventType: eventType, Payload: payload}
		}
	}
	switch eventType {
	case "approval_requested":
		l.waitingStatus = runtimemodel.MessageStatusWaitingApproval
	case "question_answer_requested":
		l.waitingStatus = runtimemodel.MessageStatusWaitingQuestion
	case "approval_expired":
		l.approvalExpired = true
	case "workflow_failed":
		l.terminal = true
		if l.runStatus == "" {
			l.runStatus = "failed"
		}
		if message := workflowContinuationPayloadString(rawPayload, "message"); message != "" {
			l.runError = message
		}
	case "workflow_stopped":
		l.terminal = true
		l.runStatus = "stopped"
	case "workflow_finished":
		l.terminal = true
		if status := workflowContinuationPayloadString(rawPayload, "status"); status != "" && l.runStatus != "stopped" {
			l.runStatus = status
		}
		if outputs, ok := rawPayload["outputs"].(map[string]interface{}); ok {
			l.runOutputs = outputs
		}
		if message := workflowContinuationPayloadString(rawPayload, "error"); message != "" {
			l.runError = message
		}
	}
	// A disconnected client must not abort the resumed workflow; the events
	// are already persisted and can be replayed.
	_ = l.emit(chatruntime.ProjectWorkflowContinuationEvent(l.continuation, *event))
	return nil
}

func (l *digitalHumanWorkflowContinuation) finish(resume workflowContinuationResume, runErr error) error {
	ctx := l.ctx
	if runErr != nil && resume == workflowContinuationNotResumed {
		// Invalid input or a rejected approval submission: the workflow never
		// resumed, so the user can still answer the original request.
		return l.abortBeforeResume(runErr)
	}
	if runErr != nil {
		l.fail(ctx, runErr)
		return runErr
	}
	switch {
	case resume == workflowContinuationNotResumed:
		return l.pause(ctx, runtimemodel.MessageStatusWaitingApproval)
	case resume == workflowContinuationStreamed && !l.terminal && l.waitingStatus != "":
		return l.pause(ctx, l.waitingStatus)
	case resume == workflowContinuationStreamed && !l.terminal:
		err := fmt.Errorf("digital human workflow continuation ended without a terminal workflow event")
		l.fail(ctx, err)
		return err
	}
	if strings.EqualFold(strings.TrimSpace(l.runStatus), "stopped") {
		metadata, err := l.runtime.CompleteWorkflowApprovalContinuation(ctx, l.continuation, "", runtimemodel.MessageStatusStopped)
		if err != nil {
			l.fail(ctx, err)
			return err
		}
		return l.emitMessageEnd(ctx, runtimemodel.MessageStatusStopped, metadata)
	}
	status := strings.TrimSpace(l.runStatus)
	if status == "" {
		status = "succeeded"
	}
	summary := chatruntime.WorkflowContinuationSummaryRequest{
		WorkflowRunID: l.continuation.WorkflowRunID,
		Status:        status,
		Outputs:       l.runOutputs,
		Error:         l.runError,
	}
	if l.approvalExpired {
		summary.ContinuationInstruction = workflowApprovalExpiredInstruction
	}
	result, err := l.runtime.SummarizeWorkflowApprovalContinuation(ctx, l.scope, l.continuation, summary, func(event chatruntime.StreamEvent) error {
		return l.emit(chatruntime.ProjectWorkflowContinuationEvent(l.continuation, event))
	})
	if err != nil {
		if chatruntime.IsFinalizedStreamError(err) {
			// The runtime already finalized the message as failed.
			l.settleRun(ctx, runtimemodel.MessageStatusError, nil)
			return err
		}
		l.fail(ctx, err)
		return err
	}
	if result != nil && result.Status != "" && result.Status != runtimemodel.MessageStatusCompleted {
		l.settleRun(ctx, result.Status, result.Metadata)
		return nil
	}
	metadata := map[string]interface{}{}
	if result != nil {
		metadata = result.Metadata
		l.answer = result.Answer
	}
	return l.emitMessageEnd(ctx, runtimemodel.MessageStatusCompleted, metadata)
}

// abortBeforeResume restores the waiting state when the client is gone
// before anything was submitted, so the approval or question can be answered
// again. A continuation that was not waiting before Begin is failed instead.
func (l *digitalHumanWorkflowContinuation) abortBeforeResume(cause error) error {
	switch l.continuation.MessageStatus {
	case runtimemodel.MessageStatusWaitingApproval, runtimemodel.MessageStatusWaitingQuestion:
		if _, err := l.runtime.PauseWorkflowApprovalContinuation(l.ctx, l.continuation, l.continuation.MessageStatus); err != nil {
			l.fail(l.ctx, errors.Join(cause, err))
		} else {
			l.settleRun(l.ctx, l.continuation.MessageStatus, nil)
		}
	default:
		l.fail(l.ctx, cause)
	}
	return cause
}

func (l *digitalHumanWorkflowContinuation) pause(ctx context.Context, status string) error {
	metadata, err := l.runtime.PauseWorkflowApprovalContinuation(ctx, l.continuation, status)
	if err != nil {
		l.fail(ctx, err)
		return err
	}
	return l.emitMessageEnd(ctx, status, metadata)
}

// fail marks the message failed with a generic, public message. The cause is
// returned to the caller for logging and boundary projection only.
func (l *digitalHumanWorkflowContinuation) fail(ctx context.Context, cause error) {
	logger.WarnContext(ctx, "digital human workflow continuation failed", "workflow_run_id", l.continuation.WorkflowRunID, "cause", cause)
	message := chatruntime.ProjectWorkflowContinuationFailureMessage(l.continuation, "workflow continuation stopped before completion")
	metadata, err := l.runtime.FailWorkflowApprovalContinuation(ctx, l.continuation, message)
	if err != nil {
		logger.WarnContext(ctx, "failed to mark digital human workflow continuation failed", "workflow_run_id", l.continuation.WorkflowRunID, err)
		l.settleRun(ctx, runtimemodel.MessageStatusError, nil)
		return
	}
	_ = l.appendAndEmit(ctx, "message_end", map[string]interface{}{
		"conversation_id": l.continuation.ConversationID.String(),
		"message_id":      l.continuation.MessageID.String(),
		"status":          runtimemodel.MessageStatusError,
		"metadata":        metadata,
	})
	l.settleRun(ctx, runtimemodel.MessageStatusError, metadata)
}

func (l *digitalHumanWorkflowContinuation) emitMessageEnd(ctx context.Context, status string, metadata map[string]interface{}) error {
	err := l.appendAndEmit(ctx, "message_end", map[string]interface{}{
		"conversation_id": l.continuation.ConversationID.String(),
		"message_id":      l.continuation.MessageID.String(),
		"status":          status,
		"metadata":        metadata,
	})
	// The message is already persisted in its final state; the run follows it
	// even when the client is gone and the event could not be delivered.
	l.settleRun(ctx, status, metadata)
	return err
}

func (l *digitalHumanWorkflowContinuation) settleRun(ctx context.Context, status string, metadata map[string]interface{}) {
	if l.settle == nil {
		return
	}
	if status == runtimemodel.MessageStatusCompleted && strings.EqualFold(strings.TrimSpace(l.runStatus), "failed") {
		// The summary of a failed workflow completes the message, but the run
		// still reports the workflow outcome.
		status = runtimemodel.MessageStatusError
	}
	l.settle(ctx, status, l.answer, metadata)
}

func (l *digitalHumanWorkflowContinuation) appendAndEmit(ctx context.Context, eventType string, payload map[string]interface{}) error {
	event, err := l.runtime.AppendWorkflowApprovalContinuationStreamEvent(ctx, l.continuation, eventType, payload)
	if err != nil || event == nil {
		event = &chatruntime.StreamEvent{EventType: eventType, Payload: payload}
	}
	return l.emit(chatruntime.ProjectWorkflowContinuationEvent(l.continuation, *event))
}

func (l *digitalHumanWorkflowContinuation) emit(event chatruntime.StreamEvent) error {
	if l.onEvent == nil {
		return nil
	}
	return l.onEvent(event)
}

func copyWorkflowContinuationPayload(payload map[string]interface{}) map[string]interface{} {
	copied := make(map[string]interface{}, len(payload)+1)
	for key, value := range payload {
		copied[key] = value
	}
	return copied
}

func workflowContinuationPayloadString(payload map[string]interface{}, key string) string {
	value, _ := payload[key].(string)
	return strings.TrimSpace(value)
}

// workflowContinuationConfig verifies the run belongs to this conversation
// message and restores its execution snapshot, tools, connector identity and
// workspace, so the resumed Agent loop behaves like the original turn.
func (s *RuntimeService) workflowContinuationConfig(ctx context.Context, scope Scope, digitalHumanID, conversationID, messageID uuid.UUID) (*Run, chatruntime.RunConfig, error) {
	lookup, ok := s.store.(channelApprovalRunLookup)
	if !ok {
		return nil, chatruntime.RunConfig{}, ErrRunNotFound
	}
	run, err := lookup.RunByMessageID(ctx, scope.OrganizationID, scope.WorkspaceID, digitalHumanID, messageID)
	if err != nil {
		return nil, chatruntime.RunConfig{}, err
	}
	if run == nil {
		return nil, chatruntime.RunConfig{}, ErrRunNotFound
	}
	if run.DigitalHumanID != digitalHumanID || run.OrganizationID != scope.OrganizationID || run.WorkspaceID != scope.WorkspaceID ||
		run.ConversationID == nil || *run.ConversationID != conversationID || run.MessageID == nil || *run.MessageID != messageID {
		return nil, chatruntime.RunConfig{}, ErrConversationNotFound
	}
	resolved, event, err := decodeAndVerifyRun(run)
	if err != nil {
		return nil, chatruntime.RunConfig{}, err
	}
	config, _, err := s.userInputContinuationConfig(ctx, run, resolved, event, scope)
	if err != nil {
		return nil, chatruntime.RunConfig{}, err
	}
	return run, config, nil
}

// workflowHeldRun returns the run only when this workflow continuation holds it:
// waiting on a generic workflow approval (workflow origin) or on a workflow
// question. The caller has already verified the run owns this conversation
// message. Channel and background runs keep their own lifecycle (a channel
// continuation delivers its own reply) and are left untouched.
func workflowHeldRun(run *Run) *Run {
	if run == nil || run.Status != RunStatusWaiting || !workflowContinuationSurface(run.Surface) {
		return nil
	}
	switch {
	case run.WaitingReason == runtimemodel.MessageStatusWaitingApproval && run.WaitingApprovalOrigin == workflowApprovalOrigin:
		return run
	case run.WaitingReason == runtimemodel.MessageStatusWaitingQuestion && run.WaitingApprovalOrigin == "":
		return run
	default:
		return nil
	}
}

// workflowContinuationSurface reports whether a run's surface resumes a
// workflow continuation interactively (console, WebApp, API).
func workflowContinuationSurface(surface string) bool {
	return surface == SurfaceConsole || surface == SurfaceWebApp || surface == SurfaceAPI
}

// resumeHeldWorkflowRun moves the waiting run back to running. A run that is
// already running is a retry of an in-flight continuation and just observes it.
func (s *RuntimeService) resumeHeldWorkflowRun(ctx context.Context, run *Run) error {
	waiter, ok := s.store.(runWaitingStore)
	if !ok {
		return fmt.Errorf("digital human waiting run resume is unavailable")
	}
	if err := waiter.ResumeWaitingRun(ctx, run.ID, s.now()); err != nil && !errors.Is(err, ErrRunConflict) {
		return err
	}
	return nil
}

// settleWorkflowContinuationRun records where the continuation ended on the run:
// completed, waiting again for another approval or question, or stopped/failed.
func (s *RuntimeService) settleWorkflowContinuationRun(ctx context.Context, run *Run, status, answer string, metadata map[string]interface{}) {
	ctx = context.WithoutCancel(ctx)
	var err error
	switch status {
	case runtimemodel.MessageStatusCompleted:
		err = s.store.CompleteRunWithDelivery(ctx, run.ID, answer, executionTraceFromMetadata(metadata), s.now())
	case runtimemodel.MessageStatusWaitingApproval, runtimemodel.MessageStatusWaitingQuestion,
		runtimemodel.MessageStatusWaitingBudget, runtimemodel.MessageStatusWaitingClientAction:
		// The resumed Agent loop can also stop on a budget or client action; the
		// run must stay waiting so those continuations can pick it up.
		waiter, ok := s.store.(runWaitingStore)
		if !ok {
			err = fmt.Errorf("digital human waiting run resume is unavailable")
			break
		}
		err = waiter.MarkRunWaiting(ctx, run.ID, status, s.now())
	case runtimemodel.MessageStatusStopped:
		err = s.store.FailRun(ctx, run.ID, "digital_human.run.canceled", "The DigitalHuman run was stopped.", s.now())
	default:
		err = s.store.FailRun(ctx, run.ID, "digital_human.run.execution_failed", "The DigitalHuman continuation failed.", s.now())
	}
	if err != nil {
		logger.WarnContext(ctx, "failed to settle digital human run after workflow continuation", "run_id", run.ID.String(), "status", status, err)
	}
}
