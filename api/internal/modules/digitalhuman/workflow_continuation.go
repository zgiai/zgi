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
)

var ErrDigitalHumanTaskContinuation = errors.New("digital human task continuation")

func (s *RuntimeService) WorkflowContinuationEnabled() bool {
	return s != nil && s.workflowDB != nil && s.workflowRunner != nil
}

type workflowContinuationChatRuntime interface {
	BeginWorkflowApprovalContinuation(context.Context, chatruntime.Scope, chatruntime.Caller, chatruntime.RunConfig, uuid.UUID, uuid.UUID) (*chatruntime.WorkflowApprovalContinuation, error)
}

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
	typeValue := strings.ToLower(strings.TrimSpace(request.Type))
	if typeValue == "" {
		typeValue = "approval"
	}
	if typeValue == "question_answer" {
		inputs := request.Inputs
		if len(inputs) == 0 {
			return fmt.Errorf("%w: question answer inputs are required", chatruntime.ErrInvalidInput)
		}
		if runner, ok := s.workflowRunner.(workflowContinuationStreamRunner); ok {
			return runner.ResumeQuestionAnswerWorkflowStream(ctx, continuation.WorkflowRunID, inputs, func(event string, payload map[string]interface{}) error {
				if onEvent == nil {
					return nil
				}
				return onEvent(chatruntime.StreamEvent{EventType: event, Payload: payload})
			})
		}
		return s.workflowRunner.ResumeQuestionAnswerWorkflow(ctx, continuation.WorkflowRunID, inputs)
	}
	if strings.TrimSpace(request.ApprovalToken) == "" || strings.TrimSpace(request.Action) == "" {
		return fmt.Errorf("%w: approval token and action are required", chatruntime.ErrInvalidInput)
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
		return err
	}
	if submission == nil || !submission.ResumeReady {
		return nil
	}
	if runner, ok := s.workflowRunner.(workflowContinuationStreamRunner); ok {
		return runner.ResumeApprovalWorkflowStream(ctx, submission.Form, func(event string, payload map[string]interface{}) error {
			if onEvent == nil {
				return nil
			}
			return onEvent(chatruntime.StreamEvent{EventType: event, Payload: payload})
		})
	}
	return s.workflowRunner.ResumeApprovalWorkflow(ctx, submission.Form)
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
