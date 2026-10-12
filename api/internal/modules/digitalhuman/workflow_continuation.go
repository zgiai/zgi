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
	if s == nil || !s.WorkflowContinuationEnabled() {
		return fmt.Errorf("digital human workflow continuation is unavailable")
	}
	_, runtimeScope, caller, err := s.conversationContext(scope, digitalHumanID)
	if err != nil {
		return err
	}
	config, err := s.workflowContinuationConfig(ctx, scope, digitalHumanID, messageID)
	if err != nil {
		return err
	}
	runtime, ok := s.chat.(workflowContinuationChatRuntime)
	if !ok {
		return fmt.Errorf("digital human workflow continuation runtime is unavailable")
	}
	continuation, err := runtime.BeginWorkflowApprovalContinuation(ctx, runtimeScope, caller, config, conversationID, messageID)
	if err != nil {
		return err
	}
	if continuation.Kind == "digital_human_task" {
		return ErrDigitalHumanTaskContinuation
	}
	if continuation.Completed {
		return nil
	}
	if onEvent != nil {
		if err := onEvent(chatruntime.StreamEvent{EventType: "message_start", Payload: map[string]interface{}{
			"conversation_id": conversationID.String(), "message_id": messageID.String(),
			"workflow_run_id": continuation.WorkflowRunID, "continuation": true,
		}}); err != nil {
			return err
		}
	}
	// BeginWorkflowApprovalContinuation moved the message to streaming. Every
	// exit below must leave it paused again, completed or failed, otherwise the
	// conversation stays blocked on a message no run owns.
	lifecycle := &digitalHumanWorkflowContinuation{
		ctx: context.WithoutCancel(ctx), runtime: runtime, scope: runtimeScope, continuation: continuation, onEvent: onEvent,
	}
	streamed, err := s.resumeWorkflowContinuation(ctx, runtimeScope, continuation, request, lifecycle.handle)
	return lifecycle.finish(streamed, err)
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

	waitingStatus string
	terminal      bool
	runStatus     string
	runOutputs    map[string]interface{}
	runError      string
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
	result, err := l.runtime.SummarizeWorkflowApprovalContinuation(ctx, l.scope, l.continuation, chatruntime.WorkflowContinuationSummaryRequest{
		WorkflowRunID: l.continuation.WorkflowRunID,
		Status:        status,
		Outputs:       l.runOutputs,
		Error:         l.runError,
	}, func(event chatruntime.StreamEvent) error {
		return l.emit(chatruntime.ProjectWorkflowContinuationEvent(l.continuation, event))
	})
	if err != nil {
		if chatruntime.IsFinalizedStreamError(err) {
			return err
		}
		l.fail(ctx, err)
		return err
	}
	if result != nil && result.Status != "" && result.Status != runtimemodel.MessageStatusCompleted {
		return nil
	}
	metadata := map[string]interface{}{}
	if result != nil {
		metadata = result.Metadata
	}
	return l.emitMessageEnd(ctx, runtimemodel.MessageStatusCompleted, metadata)
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
		return
	}
	_ = l.appendAndEmit(ctx, "message_end", map[string]interface{}{
		"conversation_id": l.continuation.ConversationID.String(),
		"message_id":      l.continuation.MessageID.String(),
		"status":          runtimemodel.MessageStatusError,
		"metadata":        metadata,
	})
}

func (l *digitalHumanWorkflowContinuation) emitMessageEnd(ctx context.Context, status string, metadata map[string]interface{}) error {
	return l.appendAndEmit(ctx, "message_end", map[string]interface{}{
		"conversation_id": l.continuation.ConversationID.String(),
		"message_id":      l.continuation.MessageID.String(),
		"status":          status,
		"metadata":        metadata,
	})
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

func (s *RuntimeService) workflowContinuationConfig(ctx context.Context, scope Scope, digitalHumanID, messageID uuid.UUID) (chatruntime.RunConfig, error) {
	lookup, ok := s.store.(channelApprovalRunLookup)
	if !ok {
		return chatruntime.RunConfig{}, ErrRunNotFound
	}
	run, err := lookup.RunByMessageID(ctx, scope.OrganizationID, scope.WorkspaceID, digitalHumanID, messageID)
	if err != nil {
		return chatruntime.RunConfig{}, err
	}
	if run == nil {
		return chatruntime.RunConfig{}, ErrRunNotFound
	}
	resolved, event, err := decodeAndVerifyRun(run)
	if err != nil {
		return chatruntime.RunConfig{}, err
	}
	config, err := s.brain.RunConfig(resolved)
	if err != nil {
		return chatruntime.RunConfig{}, err
	}
	s.attachAttachmentReader(&config, run, resolved, event, nil)
	return config, nil
}
