package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	runtimedto "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/dto"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	"github.com/zgiai/zgi/api/internal/capabilities/chatruntime/repository"
	"github.com/zgiai/zgi/api/internal/errors/failureprojection"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"gorm.io/gorm"
)

const (
	workflowContinuationStatusWaitingApproval  = "waiting_approval"
	workflowContinuationStatusWaitingQuestion  = "waiting_question"
	workflowContinuationStatusContinuing       = "continuing"
	workflowContinuationStatusSummarizing      = "summarizing"
	workflowContinuationStatusDirectOutput     = "direct_output"
	workflowContinuationStatusCompleted        = "completed"
	workflowContinuationStatusFailed           = "failed"
	workflowContinuationStatusStopped          = "stopped"
	workflowContinuationCheckpointInterval     = 750 * time.Millisecond
	workflowContinuationCheckpointBytes        = 4 * 1024
	workflowContinuationTerminalPersistTimeout = 10 * time.Second
	publishedWorkflowFailureError              = "workflow run failed"
	publishedWorkflowErrorVisibility           = "generic"
)

type WorkflowApprovalContinuation struct {
	Kind           string
	ConversationID uuid.UUID
	MessageID      uuid.UUID
	// MessageStatus is the persisted status observed while loading the
	// continuation. DigitalHuman task approvals use it to distinguish a fresh
	// waiting approval from a continuation that was already claimed before a
	// client disconnect or process restart.
	MessageStatus             string
	WorkflowRunID             string
	InvocationID              string
	InvocationMode            string
	InvocationProtocolVersion int
	AgentID                   string
	AgentType                 string
	BindingID                 string
	OriginalQuery             string
	UIApprovalAllowed         bool
	Completed                 bool
	Metadata                  map[string]interface{}
	Caller                    Caller
	RunConfig                 RunConfig

	answerMu             sync.Mutex
	answer               string
	answerObserved       bool
	lastCheckpointAnswer string
	lastCheckpointAt     time.Time
}

// DigitalHumanTaskApprovalRequest is the small, surface-neutral payload used
// by DigitalHuman task approvals. The task module owns validation and
// mutation; ChatRuntime only owns pausing/resuming the conversation stream.
type DigitalHumanTaskApprovalRequest struct {
	Type          string
	ApprovalToken string
	Action        string
	Inputs        map[string]interface{}
}

// DigitalHumanTaskApprovalHandler applies an approved task mutation and
// returns a compact tool-result summary. ChatRuntime feeds that summary back
// into the Agent Loop instead of exposing it as the final answer. It is
// deliberately a callback so ChatRuntime does not depend on the DigitalHuman
// task domain.
type DigitalHumanTaskApprovalHandler func(context.Context, *WorkflowApprovalContinuation, string, map[string]interface{}) (string, error)

// RunDigitalHumanTaskApprovalContinuation resumes a DigitalHuman task approval
// through the same waiting-message and SSE continuation lifecycle as other
// workflow approvals. The approval result is fed back into the same Agent
// Loop as authoritative tool evidence, so the model—not this transport layer—
// writes the final answer. A revision closes the previous tool call without
// applying it and asks the model to create a fresh proposal.
func (s *service) RunDigitalHumanTaskApprovalContinuation(
	ctx context.Context,
	scope Scope,
	caller Caller,
	config RunConfig,
	conversationID, messageID uuid.UUID,
	request DigitalHumanTaskApprovalRequest,
	apply DigitalHumanTaskApprovalHandler,
	onEvent func(StreamEvent) error,
) (*ChatResult, error) {
	continuation, err := s.BeginWorkflowApprovalContinuation(ctx, scope, caller, config, conversationID, messageID)
	if err != nil {
		return nil, err
	}
	if continuation.Kind != "digital_human_task" {
		return nil, fmt.Errorf("%w: continuation is not a DigitalHuman task approval", ErrInvalidInput)
	}
	if continuation.Completed {
		return &ChatResult{Answer: continuation.answer, Metadata: continuation.Metadata}, nil
	}
	state := workflowRecordFromAny(continuation.Metadata["agent_workflow_continuation"])
	expectedToken := strings.TrimSpace(firstNonEmptyString(state["approval_token"]))
	if expectedToken == "" || !secureContinuationTokenEqual(expectedToken, request.ApprovalToken) {
		return nil, fmt.Errorf("%w: approval token is invalid or expired", ErrInvalidInput)
	}
	action := strings.ToLower(strings.TrimSpace(request.Action))
	if action != "approve" && action != "decline" && action != "revise" {
		return nil, fmt.Errorf("%w: approval action is invalid", ErrInvalidInput)
	}
	feedback := strings.TrimSpace(firstNonEmptyString(request.Inputs["supplement"]))
	if action == "revise" && feedback == "" {
		return nil, fmt.Errorf("%w: revision feedback is required", ErrInvalidInput)
	}
	if action == "revise" && len([]rune(feedback)) > 4000 {
		return nil, fmt.Errorf("%w: revision feedback is too long", ErrInvalidInput)
	}
	if apply == nil {
		return nil, fmt.Errorf("%w: task approval handler is unavailable", ErrInvalidInput)
	}
	// Validate the approval before claiming the paused message. A stale token,
	// duplicate click, or malformed action must not strand the conversation in
	// streaming with no approval form left to retry.
	if continuation.MessageStatus != runtimemodel.MessageStatusStreaming {
		if err := s.claimDigitalHumanTaskApproval(ctx, scope, continuation); err != nil {
			return nil, err
		}
		continuation.MessageStatus = runtimemodel.MessageStatusStreaming
	}
	if _, err := s.UpdateWorkflowApprovalContinuationStatus(ctx, continuation, workflowContinuationStatusContinuing); err != nil {
		_, _ = s.restoreDigitalHumanTaskApproval(ctx, continuation)
		return nil, err
	}
	if event, eventErr := s.AppendWorkflowApprovalContinuationStreamEvent(ctx, continuation, "message_start", map[string]interface{}{
		"conversation_id": conversationID.String(), "message_id": messageID.String(),
		"workflow_run_id": continuation.WorkflowRunID, "continuation": true,
	}); eventErr == nil && onEvent != nil {
		if err := onEvent(*event); err != nil {
			_, _ = s.restoreDigitalHumanTaskApproval(ctx, continuation)
			return nil, err
		}
	}
	state = workflowRecordFromAny(continuation.Metadata["agent_workflow_continuation"])
	taskID := firstNonEmptyString(state["task_id"])
	taskAction := firstNonEmptyString(state["task_action"])
	answer := ""
	continuationRequest := WorkflowContinuationSummaryRequest{
		WorkflowRunID: continuation.WorkflowRunID,
		Status:        "succeeded",
		Outputs: map[string]interface{}{
			"status":         "applied",
			"applied":        true,
			"task_id":        taskID,
			"task_action":    taskAction,
			"result_summary": "",
		},
		ContinuationInstruction: "The digital-human task tool call has completed successfully and its requested change is now applied. Treat this as the tool result for the same Agent turn, then generate the final user-facing answer. Do not call the same task operation again unless the user asks for another change.",
	}
	answer, err = apply(ctx, continuation, action, copyStringAnyMap(request.Inputs))
	if err != nil {
		metadata, restoreErr := s.restoreDigitalHumanTaskApproval(ctx, continuation)
		if restoreErr == nil && onEvent != nil {
			if event, eventErr := s.AppendWorkflowApprovalContinuationStreamEvent(ctx, continuation, "error", map[string]interface{}{
				"code": "digital_human.task.approval_failed", "message": "任务审批未能完成，请刷新后重试。", "status": "error",
			}); eventErr == nil {
				_ = onEvent(*event)
			}
			if event, eventErr := s.AppendWorkflowApprovalContinuationStreamEvent(ctx, continuation, "message_end", map[string]interface{}{
				"conversation_id": conversationID.String(), "message_id": messageID.String(),
				"status": runtimemodel.MessageStatusWaitingApproval, "metadata": metadata,
			}); eventErr == nil {
				_ = onEvent(*event)
			}
		}
		if restoreErr != nil {
			return nil, fmt.Errorf("restore digital human task approval: %w", restoreErr)
		}
		return nil, err
	}
	if action == "revise" {
		// A revision is not a task mutation. The previous proposal is closed as
		// ineffective and the feedback is returned to the Agent Loop as the
		// user's next instruction so the model must produce a fresh proposal.
		continuationRequest.Status = "rejected"
		continuationRequest.Outputs = map[string]interface{}{
			"status":         "proposal_rejected",
			"applied":        false,
			"task_id":        taskID,
			"task_action":    taskAction,
			"result_summary": "用户未批准上一版任务变更，未应用请求的变更。",
			"user_feedback":  feedback,
		}
		continuationRequest.ContinuationInstruction = "The user did not approve the previous digital-human task proposal. Treat that task tool call as completed without applying the requested change. The previous proposal may have been discarded, but you must not claim that the requested change was applied. Use the user's feedback below to revise the task parameters, call the digital-human task tool again, and present the new proposal for approval. Do not apply the new proposal directly and do not answer as if the task change succeeded. User feedback: " + feedback
		continuationRequest.AllowToolReinvoke = true
	} else {
		continuationRequest.Outputs["result_summary"] = answer
		if action == "decline" {
			continuationRequest.Status = "declined"
			continuationRequest.Outputs["status"] = "not_applied"
			continuationRequest.Outputs["applied"] = false
			continuationRequest.ContinuationInstruction = "The user declined the digital-human task operation. Treat this as the completed result of the task tool call; the task change was not applied. Continue the same Agent turn and generate a concise final response without claiming success."
		}
	}
	approvalPayload := map[string]interface{}{"action": action, "status": "submitted", "submitted_at": time.Now().UTC().Format(time.RFC3339)}
	if action == "revise" {
		approvalPayload["status"] = "rejected"
		approvalPayload["reason"] = "user_requested_revision"
	}
	if event, eventErr := s.RecordWorkflowApprovalContinuationEvent(ctx, continuation, "approval_result_filled", approvalPayload); eventErr == nil && onEvent != nil {
		_ = onEvent(*event)
	}

	result, err := s.continueWorkflowTaskInvocation(ctx, scope, continuation, continuationRequest, onEvent)
	if err != nil {
		failureMessage := "任务变更已处理，但最终回答生成失败，请刷新后重试。"
		failureCode := "digital_human.task.follow_up_failed"
		if action == "revise" {
			// The old proposal was deliberately closed before regeneration. Do
			// not restore its stale approval card when the replacement proposal
			// cannot be generated; the user must start a new task instruction.
			failureMessage = "任务修改建议生成失败，请刷新后重试。"
			failureCode = "digital_human.task.revision_failed"
		}
		metadata, failErr := s.FailWorkflowApprovalContinuation(ctx, continuation, failureMessage)
		if failErr == nil {
			if event, eventErr := s.RecordWorkflowApprovalContinuationEvent(ctx, continuation, "workflow_failed", map[string]interface{}{
				"status": "error", "task_id": taskID, "task_action": taskAction, "error": failureCode,
			}); eventErr == nil && onEvent != nil {
				_ = onEvent(*event)
			}
			// RecordWorkflowApprovalContinuationEvent merges the terminal
			// workflow projection into continuation.Metadata. Use that merged
			// value for message_end; otherwise the client receives an older
			// workflow_runs entry and renders the task as still running.
			metadata = continuation.Metadata
			if onEvent != nil {
				if event, eventErr := s.AppendWorkflowApprovalContinuationStreamEvent(ctx, continuation, "error", map[string]interface{}{
					"code": failureCode, "message": failureMessage, "status": "error",
				}); eventErr == nil {
					_ = onEvent(*event)
				}
				if event, eventErr := s.AppendWorkflowApprovalContinuationStreamEvent(ctx, continuation, "message_end", map[string]interface{}{
					"conversation_id": conversationID.String(), "message_id": messageID.String(),
					"status": runtimemodel.MessageStatusError, "metadata": metadata,
				}); eventErr == nil {
					_ = onEvent(*event)
				}
			}
		}
		return nil, err
	}
	if result == nil {
		return nil, fmt.Errorf("%w: task approval continuation returned no result", ErrInvalidInput)
	}
	if result.Status != "" && result.Status != runtimemodel.MessageStatusCompleted {
		return result, nil
	}
	metadata := result.Metadata
	if event, eventErr := s.RecordWorkflowApprovalContinuationEvent(ctx, continuation, "workflow_finished", map[string]interface{}{
		"status": "completed", "task_id": taskID, "task_action": taskAction,
		"outputs": continuationRequest.Outputs, "result_summary": result.Answer,
	}); eventErr == nil && onEvent != nil {
		_ = onEvent(*event)
	}
	// The continuation event is persisted after the Agent Loop completes. The
	// message_end envelope must carry the same merged metadata so the browser
	// does not immediately overwrite the terminal event with the pre-event
	// snapshot returned by the loop.
	metadata = continuation.Metadata
	if event, eventErr := s.AppendWorkflowApprovalContinuationStreamEvent(ctx, continuation, "message_end", map[string]interface{}{
		"conversation_id": conversationID.String(), "message_id": messageID.String(),
		"status": runtimemodel.MessageStatusCompleted, "metadata": metadata,
	}); eventErr == nil && onEvent != nil {
		_ = onEvent(*event)
	}
	result.Metadata = metadata
	result.Status = runtimemodel.MessageStatusCompleted
	return result, nil
}

func secureContinuationTokenEqual(expected, actual string) bool {
	return strings.TrimSpace(expected) != "" && strings.TrimSpace(expected) == strings.TrimSpace(actual)
}

type WorkflowContinuationSummaryRequest struct {
	WorkflowRunID        string
	Status               string
	Outputs              map[string]interface{}
	Error                string
	FailureDetailsHidden bool
	AllowToolReinvoke    bool
	// ContinuationInstruction lets a surface provide a small, authoritative
	// instruction for the next Agent Loop round. It is intentionally separate
	// from Outputs so a caller can distinguish tool evidence from the control
	// semantics of the continuation (for example, a rejected task proposal
	// must be regenerated rather than treated as applied).
	ContinuationInstruction string
}

func (s *service) BeginWorkflowApprovalContinuation(ctx context.Context, scope Scope, caller Caller, config RunConfig, conversationID, messageID uuid.UUID) (*WorkflowApprovalContinuation, error) {
	return s.beginWorkflowApprovalContinuation(ctx, scope, caller, config, conversationID, messageID, false)
}

// ExternalWorkflowApprovalContinuationBeginner is implemented by the
// production ChatRuntime; hosts assert it to observe external approvals.
type ExternalWorkflowApprovalContinuationBeginner interface {
	BeginExternalWorkflowApprovalContinuation(ctx context.Context, scope Scope, caller Caller, config RunConfig, conversationID, messageID uuid.UUID) (*WorkflowApprovalContinuation, error)
}

var _ ExternalWorkflowApprovalContinuationBeginner = (*service)(nil)

// BeginExternalWorkflowApprovalContinuation claims a message whose workflow
// approval was completed outside the conversation (an email, SMS or approval
// page approver), so the host can record how the resumed workflow went on.
// It submits nothing, so it skips the inline-approval check that guards
// conversation submissions; the caller must have verified that the workflow
// left the approval this message waits on. Only one caller can claim the message.
func (s *service) BeginExternalWorkflowApprovalContinuation(ctx context.Context, scope Scope, caller Caller, config RunConfig, conversationID, messageID uuid.UUID) (*WorkflowApprovalContinuation, error) {
	return s.beginWorkflowApprovalContinuation(ctx, scope, caller, config, conversationID, messageID, true)
}

func (s *service) beginWorkflowApprovalContinuation(ctx context.Context, scope Scope, caller Caller, config RunConfig, conversationID, messageID uuid.UUID, external bool) (*WorkflowApprovalContinuation, error) {
	if err := s.ensureMember(ctx, scope); err != nil {
		return nil, err
	}
	conversation, err := s.getConversationByCallerScoped(ctx, scope, caller, conversationID)
	if err != nil {
		return nil, err
	}
	message, err := s.repos.Message.GetScoped(ctx, messageID, scope.OrganizationID, scope.AccountID)
	if err != nil {
		return nil, mapRepoError(err)
	}
	if message.ConversationID != conversation.ID {
		return nil, fmt.Errorf("%w: message belongs to another conversation", ErrInvalidInput)
	}
	state := workflowApprovalContinuationFromMetadata(message.Metadata)
	if state.WorkflowRunID == "" {
		return nil, fmt.Errorf("%w: message has no pending workflow continuation", ErrInvalidInput)
	}
	state.ConversationID = conversation.ID
	state.MessageID = message.ID
	state.MessageStatus = message.Status
	state.OriginalQuery = message.Query
	state.Metadata = copyStringAnyMap(message.Metadata)
	state.answer = message.Answer
	state.lastCheckpointAnswer = message.Answer
	state.Caller = caller
	state.RunConfig = config
	if message.Status == runtimemodel.MessageStatusCompleted {
		state.Completed = true
		return state, nil
	}
	if external && state.Kind != "digital_human_task" && message.Status == runtimemodel.MessageStatusStreaming &&
		workflowContinuationExternallyResumed(state.Metadata) {
		// An external continuation already claimed this message and its worker
		// was lost; the retried resume takes it over again. The workflow's own
		// resume claim decides which worker actually runs it.
		if err := validateWorkflowContinuationBinding(state, config.WorkflowBindings); err != nil {
			return nil, err
		}
		return state, nil
	}
	if external && (state.Kind == "digital_human_task" || message.Status != runtimemodel.MessageStatusWaitingApproval) {
		return nil, fmt.Errorf("%w: message is not waiting for an external workflow approval", ErrInvalidInput)
	}
	if state.Kind != "digital_human_task" {
		if err := validateWorkflowContinuationBinding(state, config.WorkflowBindings); err != nil {
			return nil, err
		}
	}
	if state.Kind == "digital_human_task" {
		state.UIApprovalAllowed = true
		if message.Status != runtimemodel.MessageStatusWaitingApproval && message.Status != runtimemodel.MessageStatusStreaming {
			return nil, fmt.Errorf("%w: message is not waiting for DigitalHuman task approval", ErrInvalidInput)
		}
		// DigitalHuman task approvals validate token/action in
		// RunDigitalHumanTaskApprovalContinuation before claiming the message.
		// Generic workflow approvals retain the existing begin-and-stream path.
		return state, nil
	}
	if message.Status == runtimemodel.MessageStatusWaitingApproval && !external &&
		!workflowContinuationAllowsInlineApproval(caller, state) {
		return nil, fmt.Errorf("%w: this workflow approval must be completed through its configured approval channel", ErrInvalidInput)
	}
	if message.Status != runtimemodel.MessageStatusWaitingApproval && message.Status != runtimemodel.MessageStatusWaitingQuestion && message.Status != runtimemodel.MessageStatusStreaming {
		return nil, fmt.Errorf("%w: message is not waiting for workflow continuation", ErrInvalidInput)
	}
	if message.Status == runtimemodel.MessageStatusWaitingApproval || message.Status == runtimemodel.MessageStatusWaitingQuestion {
		metadataChanged := false
		if message.Status == runtimemodel.MessageStatusWaitingQuestion {
			state.Metadata = workflowContinuationMetadataWithoutUserInputRequest(state.Metadata)
			metadataChanged = true
		}
		// Record whether an external continuation holds the message, so a retried
		// external resume can take it over again but never takes over one the
		// conversation started.
		if markWorkflowContinuationExternalResume(state.Metadata, external) {
			metadataChanged = true
		}
		err = s.repos.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			txRepos := repository.NewRepositories(tx)
			if err := txRepos.Conversation.StartStreaming(ctx, conversation.ID, scope.OrganizationID, scope.AccountID, message.ID); err != nil {
				return err
			}
			if metadataChanged {
				if err := txRepos.Message.UpdateMetadata(ctx, message.ID, state.Metadata); err != nil {
					return err
				}
			}
			result := tx.Model(&runtimemodel.Message{}).
				Where("id = ? AND deleted_at IS NULL AND status IN ?", message.ID, []string{runtimemodel.MessageStatusWaitingApproval, runtimemodel.MessageStatusWaitingQuestion}).
				Updates(map[string]interface{}{"status": runtimemodel.MessageStatusStreaming})
			if result.Error == nil && external && result.RowsAffected != 1 {
				return fmt.Errorf("%w: workflow continuation message was already claimed", ErrInvalidInput)
			}
			return result.Error
		})
		if err != nil {
			return nil, err
		}
	}
	return state, nil
}

// workflowContinuationExternalResumeKey marks a continuation an external
// approval resume claimed, in the message's agent_workflow_continuation.
const workflowContinuationExternalResumeKey = "external_resume"

func workflowContinuationExternallyResumed(metadata map[string]interface{}) bool {
	record, _ := metadata["agent_workflow_continuation"].(map[string]interface{})
	resumed, _ := record[workflowContinuationExternalResumeKey].(bool)
	return resumed
}

// markWorkflowContinuationExternalResume sets or clears the marker and
// reports whether the metadata changed.
func markWorkflowContinuationExternalResume(metadata map[string]interface{}, external bool) bool {
	record, ok := metadata["agent_workflow_continuation"].(map[string]interface{})
	if !ok {
		return false
	}
	if external {
		if workflowContinuationExternallyResumed(metadata) {
			return false
		}
		record = copyStringAnyMap(record)
		record[workflowContinuationExternalResumeKey] = true
		metadata["agent_workflow_continuation"] = record
		return true
	}
	if _, present := record[workflowContinuationExternalResumeKey]; !present {
		return false
	}
	record = copyStringAnyMap(record)
	delete(record, workflowContinuationExternalResumeKey)
	metadata["agent_workflow_continuation"] = record
	return true
}

// claimDigitalHumanTaskApproval atomically claims a waiting task approval.
// Validation is deliberately performed by the caller before this method so a
// bad token or action cannot leave the message in streaming state.
func (s *service) claimDigitalHumanTaskApproval(ctx context.Context, scope Scope, continuation *WorkflowApprovalContinuation) error {
	if continuation == nil || continuation.MessageID == uuid.Nil || continuation.ConversationID == uuid.Nil {
		return fmt.Errorf("%w: workflow continuation is required", ErrInvalidInput)
	}
	return s.repos.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		txRepos := repository.NewRepositories(tx)
		if err := txRepos.Conversation.StartStreaming(ctx, continuation.ConversationID, scope.OrganizationID, scope.AccountID, continuation.MessageID); err != nil {
			return err
		}
		result := tx.Model(&runtimemodel.Message{}).
			Where("id = ? AND deleted_at IS NULL AND status = ?", continuation.MessageID, runtimemodel.MessageStatusWaitingApproval).
			Updates(map[string]interface{}{"status": runtimemodel.MessageStatusStreaming})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return fmt.Errorf("%w: message is no longer waiting for DigitalHuman task approval", ErrInvalidInput)
		}
		return nil
	})
}

// restoreDigitalHumanTaskApproval makes a failed or disconnected task
// approval retryable. The generic workflow failure path is terminal by design;
// task mutations are different because the user must be able to retry after a
// transient task, revision, or persistence failure.
func (s *service) restoreDigitalHumanTaskApproval(ctx context.Context, continuation *WorkflowApprovalContinuation) (map[string]interface{}, error) {
	if continuation == nil || continuation.MessageID == uuid.Nil || continuation.ConversationID == uuid.Nil {
		return nil, fmt.Errorf("%w: workflow continuation is required", ErrInvalidInput)
	}
	metadata := workflowContinuationMetadataWithoutUserInputRequest(continuation.Metadata)
	metadata = workflowContinuationMetadataWithStatus(metadata, workflowContinuationStatusWaitingApproval)
	metadata["digital_human_task_approval_error"] = "任务审批未完成，请重试。"
	continuation.Metadata = metadata
	err := s.repos.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		txRepos := repository.NewRepositories(tx)
		if err := txRepos.Message.UpdateWaitingApproval(ctx, continuation.MessageID, metadata); err != nil {
			return err
		}
		return txRepos.Conversation.FinishContinuationMessage(ctx, continuation.ConversationID, continuation.MessageID)
	})
	return metadata, err
}

func validateWorkflowContinuationBinding(continuation *WorkflowApprovalContinuation, bindings []AgentWorkflowBinding) error {
	if continuation == nil {
		return fmt.Errorf("%w: continuation is missing", ErrWorkflowBindingUnavailable)
	}
	bindingID := strings.TrimSpace(continuation.BindingID)
	if bindingID == "" {
		return fmt.Errorf("%w: continuation binding id is missing", ErrWorkflowBindingUnavailable)
	}
	agentID := strings.TrimSpace(continuation.AgentID)
	for _, binding := range bindings {
		if !strings.EqualFold(strings.TrimSpace(binding.BindingID), bindingID) {
			continue
		}
		if agentID != "" && !strings.EqualFold(strings.TrimSpace(binding.AgentID), agentID) {
			continue
		}
		return nil
	}
	return fmt.Errorf("%w: workflow binding %q is not active", ErrWorkflowBindingUnavailable, bindingID)
}

func (s *service) RecordWorkflowApprovalContinuationEvent(ctx context.Context, continuation *WorkflowApprovalContinuation, eventType string, payload map[string]interface{}) (*StreamEvent, error) {
	if continuation == nil || continuation.MessageID == uuid.Nil {
		return nil, fmt.Errorf("%w: workflow continuation is required", ErrInvalidInput)
	}
	eventPayload := projectWorkflowContinuationEventPayload(continuation.Caller, eventType, workflowContinuationEventPayload(continuation, payload))
	if eventType == "approval_requested" {
		// An approval the workflow asks for while it continues (another approval
		// node, or the approval left after a mixed pause's question) is exposed
		// by the same rule as the workflow tool's first one.
		eventPayload["ui_approval_allowed"] = workflowContinuationApprovalUIAllowed(continuation.Caller, eventPayload)
	}
	eventPayload["conversation_id"] = continuation.ConversationID.String()
	eventPayload["message_id"] = continuation.MessageID.String()
	if _, ok := eventPayload["workflow_run_id"]; !ok && continuation.WorkflowRunID != "" {
		eventPayload["workflow_run_id"] = continuation.WorkflowRunID
	}
	// Terminal persistence happens before the SSE event is emitted. Once the
	// message is completed/failed/stopped, trying to write a partial answer can
	// race the terminal message update and make the otherwise valid workflow
	// event disappear. The answer was already checkpointed by the loop.
	if !workflowContinuationMessageTerminal(continuation) {
		if err := s.checkpointWorkflowContinuationAnswer(ctx, continuation, eventType, eventPayload, workflowContinuationEventForcesAnswerCheckpoint(eventType)); err != nil {
			return nil, err
		}
	}
	if !shouldPersistWorkflowRunMetadataEvent(eventType) {
		return s.AppendWorkflowApprovalContinuationStreamEvent(ctx, continuation, eventType, eventPayload)
	}
	metadata := mergeWorkflowRunMetadata(continuation.Metadata, eventType, eventPayload)
	update := map[string]interface{}{"status": workflowContinuationStatusFromEvent(eventType)}
	if eventType == "approval_requested" {
		// The continuation now waits on this approval: an in-conversation answer
		// is checked against its exposure and submitted with its token.
		allowed, _ := eventPayload["ui_approval_allowed"].(bool)
		update["ui_approval_allowed"] = allowed
		update["approval_token"] = firstNonEmptyString(eventPayload["approval_token"])
		update["approval_form"] = eventPayload["approval_form"]
		continuation.UIApprovalAllowed = allowed
	}
	metadata["agent_workflow_continuation"] = mergeWorkflowMap(workflowRecordFromAny(metadata["agent_workflow_continuation"]), update)
	continuation.Metadata = metadata
	var updateErr error
	if workflowContinuationMessageTerminal(continuation) {
		// The terminal message write happens before the final workflow event is
		// emitted. Preserve the workflow terminal projection even after the
		// message has become immutable to normal runtime updates.
		updateErr = s.repos.Message.UpdateMetadataAnyStatus(ctx, continuation.MessageID, metadata)
	} else {
		updateErr = s.repos.Message.UpdateMetadata(ctx, continuation.MessageID, metadata)
	}
	if updateErr != nil {
		return nil, updateErr
	}
	return s.AppendWorkflowApprovalContinuationStreamEvent(ctx, continuation, eventType, eventPayload)
}

func workflowContinuationMessageTerminal(continuation *WorkflowApprovalContinuation) bool {
	if continuation == nil {
		return false
	}
	switch continuation.MessageStatus {
	case runtimemodel.MessageStatusCompleted, runtimemodel.MessageStatusError, runtimemodel.MessageStatusStopped:
		return true
	default:
		return false
	}
}

func shouldPersistWorkflowRunMetadataEvent(eventType string) bool {
	switch strings.TrimSpace(eventType) {
	case streamEventMessage, "text_chunk", streamEventMessageEnd:
		return false
	default:
		return true
	}
}

func (s *service) AppendWorkflowApprovalContinuationStreamEvent(ctx context.Context, continuation *WorkflowApprovalContinuation, eventType string, payload map[string]interface{}) (*StreamEvent, error) {
	if continuation == nil || continuation.MessageID == uuid.Nil || continuation.ConversationID == uuid.Nil {
		return nil, fmt.Errorf("%w: workflow continuation is required", ErrInvalidInput)
	}
	eventPayload := projectWorkflowContinuationEventPayload(continuation.Caller, eventType, workflowContinuationEventPayload(continuation, payload))
	if eventType == "approval_requested" {
		// An approval the workflow asks for while it continues (another approval
		// node, or the approval left after a mixed pause's question) is exposed
		// by the same rule as the workflow tool's first one.
		eventPayload["ui_approval_allowed"] = workflowContinuationApprovalUIAllowed(continuation.Caller, eventPayload)
	}
	eventPayload["conversation_id"] = continuation.ConversationID.String()
	eventPayload["message_id"] = continuation.MessageID.String()
	if _, ok := eventPayload["workflow_run_id"]; !ok && continuation.WorkflowRunID != "" {
		eventPayload["workflow_run_id"] = continuation.WorkflowRunID
	}
	event := s.appendStreamEventBestEffort(ctx, continuation.MessageID, continuation.ConversationID, eventType, eventPayload)
	if event != nil {
		return event, nil
	}
	return &StreamEvent{
		EventType: eventType,
		Payload:   eventPayload,
		CreatedAt: time.Now().Unix(),
	}, nil
}

// workflowContinuationEventPayload adds the stable task identity to every
// DigitalHuman task continuation event. The frontend can then keep task
// approvals in the task workflow lane instead of falling back to a generic
// tool-governance row when an intermediate event has no skill/tool fields.
func workflowContinuationEventPayload(continuation *WorkflowApprovalContinuation, payload map[string]interface{}) map[string]interface{} {
	eventPayload := copyStringAnyMap(payload)
	if eventPayload == nil {
		eventPayload = map[string]interface{}{}
	}
	if continuation == nil || continuation.Kind != "digital_human_task" {
		return eventPayload
	}
	state := workflowRecordFromAny(continuation.Metadata["agent_workflow_continuation"])
	eventPayload["kind"] = "digital_human_task"
	if continuation.WorkflowRunID != "" {
		eventPayload["workflow_run_id"] = continuation.WorkflowRunID
	}
	for _, key := range []string{"task_id", "task_action", "operation_id", "task_revision", "owner_kind"} {
		if _, exists := eventPayload[key]; exists {
			continue
		}
		if value, ok := state[key]; ok && value != nil {
			eventPayload[key] = value
		}
	}
	return eventPayload
}

func (s *service) UpdateWorkflowApprovalContinuationStatus(ctx context.Context, continuation *WorkflowApprovalContinuation, status string) (map[string]interface{}, error) {
	if continuation == nil || continuation.MessageID == uuid.Nil {
		return nil, fmt.Errorf("%w: workflow continuation is required", ErrInvalidInput)
	}
	status = strings.TrimSpace(status)
	if status == "" {
		return copyStringAnyMap(continuation.Metadata), nil
	}
	metadata := workflowContinuationMetadataWithStatus(continuation.Metadata, status)
	continuation.Metadata = metadata
	if err := s.repos.Message.UpdateMetadata(ctx, continuation.MessageID, metadata); err != nil {
		return nil, err
	}
	return metadata, nil
}

func (s *service) PauseWorkflowApprovalContinuation(ctx context.Context, continuation *WorkflowApprovalContinuation, status string) (map[string]interface{}, error) {
	if continuation == nil || continuation.MessageID == uuid.Nil || continuation.ConversationID == uuid.Nil {
		return nil, fmt.Errorf("%w: workflow continuation is required", ErrInvalidInput)
	}
	if err := s.checkpointWorkflowContinuationAnswer(ctx, continuation, "workflow_paused", nil, true); err != nil {
		return nil, err
	}
	status = strings.TrimSpace(status)
	if status == "" {
		status = workflowContinuationStatusWaitingApproval
	}
	metadata := workflowContinuationMetadataWithStatus(continuation.Metadata, status)
	continuation.Metadata = metadata
	switch status {
	case workflowContinuationStatusWaitingQuestion:
		if err := s.repos.Message.UpdateWaitingQuestion(ctx, continuation.MessageID, metadata); err != nil {
			return nil, err
		}
	default:
		if err := s.repos.Message.UpdateWaitingApproval(ctx, continuation.MessageID, metadata); err != nil {
			return nil, err
		}
	}
	if err := s.repos.Conversation.FinishContinuationMessage(ctx, continuation.ConversationID, continuation.MessageID); err != nil {
		return nil, err
	}
	return metadata, nil
}

func (s *service) SummarizeWorkflowApprovalContinuation(ctx context.Context, scope Scope, continuation *WorkflowApprovalContinuation, req WorkflowContinuationSummaryRequest, onEvent func(StreamEvent) error) (*ChatResult, error) {
	if continuation == nil || continuation.MessageID == uuid.Nil || continuation.ConversationID == uuid.Nil {
		return nil, fmt.Errorf("%w: workflow continuation is required", ErrInvalidInput)
	}
	req = publishedWorkflowContinuationSummaryRequest(continuation.Caller, req)
	if workflowContinuationInvocationMode(continuation) == "agent_task_tool" || workflowContinuationStatusFailedValue(req.Status) {
		return s.continueWorkflowTaskInvocation(ctx, scope, continuation, req, onEvent)
	}
	if len(req.Outputs) == 0 && !(req.FailureDetailsHidden && workflowContinuationStatusFailedValue(req.Status)) {
		answer := workflowNoDisplayableOutputAnswer(req.WorkflowRunID)
		metadata, err := s.CompleteWorkflowApprovalContinuation(ctx, continuation, answer, workflowContinuationStatusCompleted)
		if err != nil {
			return nil, err
		}
		if onEvent != nil && answer != "" {
			event, eventErr := s.AppendWorkflowApprovalContinuationStreamEvent(ctx, continuation, streamEventMessage, map[string]interface{}{
				"answer": answer,
			})
			if eventErr != nil {
				return nil, eventErr
			}
			if err := onEvent(*event); err != nil {
				return nil, err
			}
		}
		return &ChatResult{Answer: answer, Metadata: metadata}, nil
	}
	conversation, err := s.repos.Conversation.GetScoped(ctx, continuation.ConversationID, scope.OrganizationID, scope.AccountID)
	if err != nil {
		return nil, mapRepoError(err)
	}
	message, err := s.repos.Message.GetScoped(ctx, continuation.MessageID, scope.OrganizationID, scope.AccountID)
	if err != nil {
		return nil, mapRepoError(err)
	}
	if message.ConversationID != conversation.ID {
		return nil, fmt.Errorf("%w: message belongs to another conversation", ErrInvalidInput)
	}
	metadata, err := s.UpdateWorkflowApprovalContinuationStatus(ctx, continuation, workflowContinuationStatusSummarizing)
	if err != nil {
		return nil, err
	}
	message.Metadata = metadata
	prepared := &PreparedChat{
		Conversation: conversation,
		Message:      message,
		Scope:        scope,
		LLMRequest:   workflowSummaryLLMRequest(message, continuation, req),

		usageContinuation: true,
	}
	execution, err := s.beginRuntimeExecution(ctx, message.ID)
	if err != nil {
		return nil, err
	}
	defer execution.Finish()
	runCtx := execution.Context
	persistCtx := execution.PersistContext
	if s.streams.IsStopped(message.ID, execution.runID) {
		_ = s.persistStoppedAnswer(persistCtx, prepared, "", nil)
		return nil, ErrMessageStopped
	}
	stream, err := s.openChatStream(runCtx, prepared)
	if err != nil {
		if finalizeErr := s.finalizePreparedError(persistCtx, prepared, err, onEvent); finalizeErr != nil {
			return nil, finalizedRuntimePersistenceError(finalizeErr)
		}
		return nil, newFinalizedStreamError(err)
	}
	answer, usage, err := s.collectStreamAnswerWithEvents(runCtx, prepared, stream, onEvent, nil)
	if err != nil {
		if errors.Is(err, ErrMessageStopped) {
			_ = s.persistStoppedAnswer(persistCtx, prepared, answer, usage)
			return nil, err
		}
		if finalizeErr := s.finalizePreparedError(persistCtx, prepared, err, onEvent); finalizeErr != nil {
			return nil, finalizedRuntimePersistenceError(finalizeErr)
		}
		return nil, newFinalizedStreamError(err)
	}
	if s.streams.IsStopped(message.ID, execution.runID) {
		_ = s.persistStoppedAnswer(persistCtx, prepared, answer, usage)
		return nil, ErrMessageStopped
	}
	metadata = preparedResultMetadataForPrepared(prepared, continuation.Metadata, usage)
	continuation.Metadata = metadata
	metadata, err = s.CompleteWorkflowApprovalContinuation(persistCtx, continuation, answer, workflowContinuationStatusCompleted)
	if err != nil {
		return nil, finalizedRuntimePersistenceError(err)
	}
	return &ChatResult{Answer: answer, Metadata: metadata, Usage: usage}, nil
}

func (s *service) CompleteWorkflowApprovalContinuation(ctx context.Context, continuation *WorkflowApprovalContinuation, answer string, status string) (map[string]interface{}, error) {
	if continuation == nil || continuation.MessageID == uuid.Nil || continuation.ConversationID == uuid.Nil {
		return nil, fmt.Errorf("%w: workflow continuation is required", ErrInvalidInput)
	}
	metadata := copyStringAnyMap(continuation.Metadata)
	if metadata == nil {
		metadata = map[string]interface{}{}
	}
	if strings.TrimSpace(status) == "" {
		status = runtimemodel.MessageStatusCompleted
	}
	metadata = workflowContinuationMetadataWithoutUserInputRequest(metadata)
	metadata = workflowContinuationMetadataWithStatus(metadata, status)
	continuation.Metadata = metadata
	if status == runtimemodel.MessageStatusStopped || status == workflowContinuationStatusStopped {
		markOperationPlanStopped(metadata)
		continuation.answerMu.Lock()
		answerObserved := continuation.answerObserved
		if answerObserved {
			answer = continuation.answer
		}
		continuation.answerMu.Unlock()
		persistCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), workflowContinuationTerminalPersistTimeout)
		defer cancel()
		var err error
		if answerObserved || answer != "" {
			err = s.repos.Message.UpdateStoppedAnswer(persistCtx, continuation.MessageID, answer, metadata)
		} else {
			err = updateStoppedContinuationPreservingAnswer(persistCtx, s.repos, continuation.MessageID, metadata)
		}
		if err != nil {
			return nil, err
		}
		if err := s.repos.Conversation.FinishContinuationMessage(persistCtx, continuation.ConversationID, continuation.MessageID); err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
		continuation.MessageStatus = runtimemodel.MessageStatusStopped
		return metadata, nil
	}
	if err := s.repos.Message.UpdateCompleted(ctx, continuation.MessageID, answer, metadata); err != nil {
		return nil, err
	}
	if err := s.repos.Conversation.FinishContinuationMessage(ctx, continuation.ConversationID, continuation.MessageID); err != nil {
		return nil, err
	}
	continuation.MessageStatus = runtimemodel.MessageStatusCompleted
	return metadata, nil
}

type stoppedAnswerPreservingMessageRepository interface {
	UpdateStoppedPreservingAnswer(context.Context, uuid.UUID, map[string]interface{}) error
}

func updateStoppedContinuationPreservingAnswer(ctx context.Context, repos *repository.Repositories, messageID uuid.UUID, metadata map[string]interface{}) error {
	if repos == nil {
		return fmt.Errorf("workflow continuation repositories are not configured")
	}
	if messageRepo, ok := repos.Message.(stoppedAnswerPreservingMessageRepository); ok {
		return messageRepo.UpdateStoppedPreservingAnswer(ctx, messageID, metadata)
	}
	if repos.DB == nil {
		return fmt.Errorf("workflow continuation stopped answer persistence is not configured")
	}
	return repository.UpdateMessageStoppedPreservingAnswer(ctx, repos.DB, messageID, metadata)
}

func (s *service) checkpointWorkflowContinuationAnswer(ctx context.Context, continuation *WorkflowApprovalContinuation, eventType string, payload map[string]interface{}, force bool) error {
	if continuation == nil || continuation.MessageID == uuid.Nil {
		return nil
	}
	chunk, observed := workflowContinuationAnswerChunk(eventType, payload)
	continuation.answerMu.Lock()
	defer continuation.answerMu.Unlock()
	if observed {
		continuation.answerObserved = true
		if strings.TrimSpace(eventType) == "text_replace" {
			continuation.answer = chunk
		} else {
			continuation.answer += chunk
		}
	}
	if continuation.answer == continuation.lastCheckpointAnswer {
		return nil
	}
	now := time.Now()
	answerGrowth := len(continuation.answer) - len(continuation.lastCheckpointAnswer)
	if answerGrowth < 0 {
		answerGrowth = -answerGrowth
	}
	if !force && !continuation.lastCheckpointAt.IsZero() && now.Sub(continuation.lastCheckpointAt) < workflowContinuationCheckpointInterval && answerGrowth < workflowContinuationCheckpointBytes {
		return nil
	}
	metadata := copyStringAnyMap(continuation.Metadata)
	if err := s.repos.Message.UpdatePartialAnswer(ctx, continuation.MessageID, continuation.answer, metadata); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return fmt.Errorf("checkpoint workflow continuation answer: %w", err)
	}
	continuation.lastCheckpointAnswer = continuation.answer
	continuation.lastCheckpointAt = now
	return nil
}

func workflowContinuationAnswerChunk(eventType string, payload map[string]interface{}) (string, bool) {
	switch strings.TrimSpace(eventType) {
	case streamEventMessage, "text_chunk", "text_replace":
	default:
		return "", false
	}
	if payload == nil {
		return "", false
	}
	for _, key := range []string{"answer", "text", "answer_delta"} {
		if value, ok := payload[key].(string); ok {
			return value, true
		}
	}
	if data, ok := payload["data"].(map[string]interface{}); ok {
		return workflowContinuationAnswerChunk(eventType, data)
	}
	return "", false
}

func workflowContinuationEventForcesAnswerCheckpoint(eventType string) bool {
	switch strings.TrimSpace(eventType) {
	case "text_replace", "workflow_finished", "workflow_failed", "workflow_stopped", "workflow_paused", "approval_requested", "question_answer_requested", streamEventMessageEnd:
		return true
	default:
		return false
	}
}

func (s *service) FailWorkflowApprovalContinuation(ctx context.Context, continuation *WorkflowApprovalContinuation, message string) (map[string]interface{}, error) {
	if continuation == nil || continuation.MessageID == uuid.Nil || continuation.ConversationID == uuid.Nil {
		return nil, fmt.Errorf("%w: workflow continuation is required", ErrInvalidInput)
	}
	if err := s.checkpointWorkflowContinuationAnswer(ctx, continuation, "workflow_failed", nil, true); err != nil {
		return nil, err
	}
	message = strings.TrimSpace(message)
	if message == "" {
		message = "workflow approval continuation failed"
	}
	message = ProjectWorkflowContinuationFailureMessage(continuation, message)
	metadata := workflowContinuationMetadataWithoutUserInputRequest(continuation.Metadata)
	metadata = workflowContinuationMetadataWithStatus(metadata, workflowContinuationStatusFailed)
	continuation.Metadata = metadata
	if err := s.repos.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		txRepos := repository.NewRepositories(tx)
		if err := txRepos.Message.UpdateMetadata(ctx, continuation.MessageID, metadata); err != nil {
			return err
		}
		if err := txRepos.Message.UpdateError(ctx, continuation.MessageID, message); err != nil {
			return err
		}
		return txRepos.Conversation.FinishContinuationMessage(ctx, continuation.ConversationID, continuation.MessageID)
	}); err != nil {
		return nil, err
	}
	continuation.MessageStatus = runtimemodel.MessageStatusError
	return metadata, nil
}

func workflowApprovalContinuationFromMetadata(metadata map[string]interface{}) *WorkflowApprovalContinuation {
	state := workflowRecordFromAny(metadata["agent_workflow_continuation"])
	return &WorkflowApprovalContinuation{
		Kind:                      firstNonEmptyString(state["kind"]),
		WorkflowRunID:             firstNonEmptyString(state["workflow_run_id"]),
		InvocationID:              firstNonEmptyString(state["invocation_id"]),
		InvocationMode:            firstNonEmptyString(state["invocation_mode"]),
		InvocationProtocolVersion: intFromWorkflowContinuation(state["invocation_protocol_version"]),
		AgentID:                   firstNonEmptyString(state["agent_id"]),
		AgentType:                 firstNonEmptyString(state["agent_type"]),
		BindingID:                 firstNonEmptyString(state["binding_id"]),
		OriginalQuery:             firstNonEmptyString(state["original_query"]),
		UIApprovalAllowed:         boolFromWorkflowContinuation(state["ui_approval_allowed"]),
		Metadata:                  copyStringAnyMap(metadata),
	}
}

func boolFromWorkflowContinuation(value interface{}) bool {
	allowed, _ := value.(bool)
	return allowed
}

// workflowContinuationApprovalUIAllowed decides whether the conversation may
// answer an approval requested while a workflow continues, the same way the
// workflow tool exposes the first one: the console always may, a published
// surface only when the approval accepts WebApp submissions, and a channel
// only for a DigitalHuman conversation. An exposure the event already carries
// is kept.
func workflowContinuationApprovalUIAllowed(caller Caller, payload map[string]interface{}) bool {
	if allowed, ok := payload["ui_approval_allowed"].(bool); ok {
		return allowed
	}
	switch strings.ToLower(strings.TrimSpace(caller.Source)) {
	case runtimemodel.ConversationSourceConsole:
		return true
	case runtimemodel.ConversationSourceWebApp, runtimemodel.ConversationSourceExternalAPI:
		return workflowApprovalAcceptsWebApp(payload)
	case runtimemodel.ConversationSourceChannel:
		return caller.Type == runtimemodel.ConversationCallerDigitalHuman && workflowApprovalAcceptsWebApp(payload)
	default:
		return false
	}
}

// workflowApprovalAcceptsWebApp reports whether the approval form accepts
// WebApp submissions; an omitted webapp.enabled defaults to true for legacy
// definitions.
func workflowApprovalAcceptsWebApp(payload map[string]interface{}) bool {
	methods := workflowApprovalRecordValue(payload["submit_methods"])
	if len(methods) == 0 {
		methods = workflowApprovalRecordValue(workflowApprovalRecordValue(payload["approval_form"])["submit_methods"])
	}
	rawWebApp, present := methods["webapp"]
	if !present || rawWebApp == nil {
		return false
	}
	webApp := workflowApprovalRecordValue(rawWebApp)
	enabled, exists := webApp["enabled"]
	if !exists || enabled == nil {
		return true
	}
	value, ok := enabled.(bool)
	return ok && value
}

// workflowApprovalRecordValue reads an approval record that live events may
// carry as a typed struct and stored events as a decoded map.
func workflowApprovalRecordValue(value interface{}) map[string]interface{} {
	if value == nil {
		return map[string]interface{}{}
	}
	if record, ok := value.(map[string]interface{}); ok {
		return record
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return map[string]interface{}{}
	}
	record := map[string]interface{}{}
	if err := json.Unmarshal(encoded, &record); err != nil {
		return map[string]interface{}{}
	}
	return record
}

func workflowContinuationAllowsInlineApproval(caller Caller, continuation *WorkflowApprovalContinuation) bool {
	if continuation != nil && continuation.Kind == "digital_human_task" {
		// Channel conversations have no browser composer, but DigitalHuman task
		// approvals are still completed through the same durable continuation
		// after the channel adapter recognizes an explicit confirmation message.
		return true
	}
	switch strings.ToLower(strings.TrimSpace(caller.Source)) {
	case runtimemodel.ConversationSourceConsole:
		return true
	case runtimemodel.ConversationSourceWebApp, runtimemodel.ConversationSourceExternalAPI:
		return continuation != nil && continuation.UIApprovalAllowed
	case runtimemodel.ConversationSourceChannel:
		// A DigitalHuman channel adapter only resumes after recognizing an
		// explicit confirmation in the bound channel conversation, and only for
		// an approval the workflow lets the conversation user complete; one
		// assigned to designated approvers stays with them.
		return caller.Type == runtimemodel.ConversationCallerDigitalHuman && continuation != nil && continuation.UIApprovalAllowed
	default:
		return false
	}
}

func publishedWorkflowContinuationSummaryRequest(caller Caller, req WorkflowContinuationSummaryRequest) WorkflowContinuationSummaryRequest {
	if !publishedWorkflowContinuationFailureDetailsHidden(caller) || !workflowContinuationStatusFailedValue(req.Status) {
		return req
	}
	req.Error = publishedWorkflowFailureError
	req.Outputs = map[string]interface{}{}
	req.FailureDetailsHidden = true
	return req
}

// ProjectWorkflowContinuationEvent applies the published-surface failure policy
// at the final stream boundary. Callers should use it even when persistence has
// already projected the event so fallback emission cannot bypass redaction.
func ProjectWorkflowContinuationEvent(continuation *WorkflowApprovalContinuation, event StreamEvent) StreamEvent {
	projected := event
	caller := Caller{}
	if continuation != nil {
		caller = continuation.Caller
	}
	projected.Payload = projectWorkflowContinuationEventPayload(caller, event.EventType, event.Payload)
	return projected
}

// ProjectWorkflowContinuationFailureMessage returns the message safe to persist
// and expose for a workflow continuation on the current runtime surface.
func ProjectWorkflowContinuationFailureMessage(continuation *WorkflowApprovalContinuation, message string) string {
	if WorkflowContinuationFailureDetailsHidden(continuation) {
		return publishedWorkflowFailureError
	}
	return strings.TrimSpace(message)
}

// WorkflowContinuationFailureDetailsHidden reports whether the continuation is
// running on a published surface where workflow diagnostics are private.
func WorkflowContinuationFailureDetailsHidden(continuation *WorkflowApprovalContinuation) bool {
	return continuation != nil && publishedWorkflowContinuationFailureDetailsHidden(continuation.Caller)
}

func projectWorkflowContinuationEventPayload(caller Caller, eventType string, payload map[string]interface{}) map[string]interface{} {
	if !publishedWorkflowContinuationFailureDetailsHidden(caller) || !workflowContinuationEventReportsFailure(eventType, payload) {
		eventPayload := copyStringAnyMap(payload)
		if eventPayload == nil {
			eventPayload = map[string]interface{}{}
		}
		return eventPayload
	}
	eventPayload := failureprojection.ProjectPublicPayload(
		payload,
		publishedWorkflowFailureError,
		true,
	)
	eventPayload["error_visibility"] = publishedWorkflowErrorVisibility
	return eventPayload
}

func publishedWorkflowContinuationFailureDetailsHidden(caller Caller) bool {
	switch strings.ToLower(strings.TrimSpace(caller.Source)) {
	case runtimemodel.ConversationSourceWebApp, runtimemodel.ConversationSourceExternalAPI:
		return true
	default:
		return false
	}
}

func workflowContinuationEventReportsFailure(eventType string, payload map[string]interface{}) bool {
	eventType = strings.ToLower(strings.TrimSpace(eventType))
	if eventType == "error" || strings.HasSuffix(eventType, "_failed") {
		return true
	}
	return failureprojection.IsFailureStatus(firstNonEmptyString(payload["status"]))
}

func workflowContinuationStatusFailedValue(status string) bool {
	if failureprojection.IsFailureStatus(status) {
		return true
	}
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "expired":
		return true
	default:
		return false
	}
}

func workflowContinuationInvocationMode(continuation *WorkflowApprovalContinuation) string {
	if continuation == nil {
		return ""
	}
	if mode := strings.ToLower(strings.TrimSpace(continuation.InvocationMode)); mode != "" {
		return mode
	}
	if strings.EqualFold(strings.TrimSpace(continuation.AgentType), "CONVERSATIONAL_WORKFLOW") {
		return "agent_conversation_delegate"
	}
	return "agent_task_tool"
}

func intFromWorkflowContinuation(value interface{}) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	default:
		return 0
	}
}

func (s *service) continueWorkflowTaskInvocation(ctx context.Context, scope Scope, continuation *WorkflowApprovalContinuation, req WorkflowContinuationSummaryRequest, onEvent func(StreamEvent) error) (*ChatResult, error) {
	conversation, err := s.repos.Conversation.GetScoped(ctx, continuation.ConversationID, scope.OrganizationID, scope.AccountID)
	if err != nil {
		return nil, mapRepoError(err)
	}
	message, err := s.repos.Message.GetScoped(ctx, continuation.MessageID, scope.OrganizationID, scope.AccountID)
	if err != nil {
		return nil, mapRepoError(err)
	}
	if message.ConversationID != conversation.ID {
		return nil, fmt.Errorf("%w: message belongs to another conversation", ErrInvalidInput)
	}

	prepared, err := s.prepareWorkflowTaskContinuationChat(ctx, scope, continuation, conversation, message, req)
	if err != nil {
		return nil, err
	}
	execution, err := s.beginRuntimeExecution(ctx, message.ID)
	if err != nil {
		return nil, err
	}
	defer execution.Finish()
	runCtx := execution.Context
	persistCtx := execution.PersistContext
	if s.streams.IsStopped(message.ID, execution.runID) {
		_ = s.persistStoppedAnswer(persistCtx, prepared, "", nil)
		return nil, ErrMessageStopped
	}

	previousTaskToolCalls := digitalHumanTaskToolInvocationCount(prepared.Message.Metadata)
	answer, usage, err := s.runPreparedToolLoop(runCtx, persistCtx, prepared, nil, onEvent)
	if err == nil {
		if result, handled, budgetErr := s.handleTaskTokenBudgetPending(persistCtx, prepared, answer, usage, nil, onEvent); handled {
			return result, budgetErr
		}
	}
	if err == nil && req.AllowToolReinvoke && digitalHumanTaskToolInvocationCount(prepared.Message.Metadata) <= previousTaskToolCalls {
		// A revision is a control-flow transition, not a normal assistant turn.
		// If the model ignored the first instruction and emitted prose, give it
		// one bounded retry with an explicit tool-call requirement. Never let
		// that prose complete the continuation as if the user had to ask again.
		prepared.LLMRequest.Messages = append(prepared.LLMRequest.Messages, adapter.Message{
			Role:    "user",
			Content: "Internal continuation requirement: the previous task proposal was rejected. Do not ask the user for another message. Immediately call manage_digital_human_task with the current task_id, latest revision, and the supplied user_feedback to create the replacement proposal. Do not answer with prose until that tool call has been made.",
		})
		answer, usage, err = s.runPreparedToolLoop(runCtx, persistCtx, prepared, nil, onEvent)
	}
	if err == nil && req.AllowToolReinvoke && digitalHumanTaskToolInvocationCount(prepared.Message.Metadata) <= previousTaskToolCalls {
		err = fmt.Errorf("%w: revised digital-human task proposal was not regenerated", ErrInvalidInput)
	}
	if err != nil {
		if result, handled, budgetErr := s.handleTaskTokenBudgetPending(persistCtx, prepared, answer, usage, err, onEvent); handled {
			return result, budgetErr
		}
		return s.finishUserInputContinuationPendingOrError(persistCtx, prepared, answer, usage, err, onEvent)
	}
	if result, handled, budgetErr := s.handleTaskTokenBudgetPending(persistCtx, prepared, answer, usage, nil, onEvent); handled {
		return result, budgetErr
	}
	if s.streams.IsStopped(message.ID, execution.runID) {
		_ = s.persistStoppedAnswer(persistCtx, prepared, answer, usage)
		return nil, ErrMessageStopped
	}
	metadata := preparedResultMetadataForPrepared(prepared, prepared.Message.Metadata, usage)
	continuation.Metadata = metadata
	metadata, err = s.CompleteWorkflowApprovalContinuation(persistCtx, continuation, answer, workflowContinuationStatusCompleted)
	if err != nil {
		return nil, finalizedRuntimePersistenceError(err)
	}
	return &ChatResult{Answer: answer, Metadata: metadata, Usage: usage, Status: runtimemodel.MessageStatusCompleted}, nil
}

func digitalHumanTaskToolInvocationCount(metadata map[string]interface{}) int {
	count := 0
	for _, invocation := range skillInvocationsFromMetadata(metadataValue(metadata, "skill_invocations")) {
		if strings.EqualFold(strings.TrimSpace(stringFromAny(invocation["tool_name"])), "manage_digital_human_task") {
			count++
		}
	}
	return count
}

func (s *service) prepareWorkflowTaskContinuationChat(ctx context.Context, scope Scope, continuation *WorkflowApprovalContinuation, conversation *runtimemodel.Conversation, message *runtimemodel.Message, req WorkflowContinuationSummaryRequest) (*PreparedChat, error) {
	regenerateReq := applyRunConfigToRegenerateRequest(continuation.RunConfig, runtimedto.RegenerateMessageRequest{})
	parts, err := normalizeRegenerateRequest(regenerateReq, message)
	if err != nil {
		return nil, err
	}
	applyRunConfigToParts(continuation.RunConfig, parts)
	applyCallerRuntimeSurfacePolicy(continuation.Caller, parts)
	applyPersistedConversationSurface(conversation, parts)
	restoreExecutionModeFromMetadata(parts, message.Metadata)
	restoreConsoleFilesContextFromMetadata(parts, message.Metadata, nil)
	restoreConsoleAgentsContextFromMetadata(parts, message.Metadata, nil)
	restoreTurnInitialContextFromMetadata(parts, message.Metadata)
	restoreCurrentPageContextFromMetadata(parts, message.Metadata)
	parts.Attachments = attachmentBundleFromMessageMetadata(message.Metadata)
	if err := s.applyModelCapabilities(ctx, scope, continuation.Caller, parts); err != nil {
		return nil, err
	}
	applyProtocolToolsPolicy(continuation.Caller, parts)
	applyCallerMemoryPolicy(continuation.Caller, parts)
	if err := s.applySkillConfig(ctx, scope, continuation.Caller, &continuation.RunConfig, parts); err != nil {
		return nil, err
	}
	applyAgentMemoryToolsPolicy(parts)
	contextResult, err := s.buildUpstreamMessages(ctx, scope, message.ParentID, parts, message.ConversationID)
	if err != nil {
		return nil, err
	}
	parts.ContextControl = contextResult.Metadata
	llmRequest := newLLMChatRequest(parts, contextResult.Messages)
	s.appendPersistedAgentTranscript(ctx, llmRequest, message)
	if stateMessage := currentTurnAuthoritativeStateMessage(message); stateMessage != nil {
		llmRequest.Messages = append(llmRequest.Messages, *stateMessage)
	}
	llmRequest.Messages = append(llmRequest.Messages, continuationMessageForExecutionMode(workflowTaskContinuationMessage(continuation, req), parts.ExecutionMode))
	return &PreparedChat{
		Conversation:     conversation,
		Message:          message,
		LLMRequest:       llmRequest,
		Scope:            scope,
		Caller:           continuation.Caller,
		RunConfig:        continuation.RunConfig,
		ParentID:         message.ParentID,
		Continuation:     true,
		ContinuationType: "agent_workflow_task",
		parts:            parts,
		contextBudget:    contextResult,
		taskTokenBudget:  taskTokenBudgetFromMetadata(message.Metadata),
	}, nil
}

func workflowTaskContinuationMessage(continuation *WorkflowApprovalContinuation, req WorkflowContinuationSummaryRequest) adapter.Message {
	if continuation != nil {
		req = publishedWorkflowContinuationSummaryRequest(continuation.Caller, req)
	}
	payload := map[string]interface{}{
		"invocation_id":   continuation.InvocationID,
		"workflow_run_id": firstNonEmptyString(req.WorkflowRunID, continuation.WorkflowRunID),
		"status":          strings.TrimSpace(req.Status),
		"outputs":         req.Outputs,
		"error":           strings.TrimSpace(req.Error),
	}
	instructions := []string{
		"The task workflow tool invocation for this same Agent turn has reached a terminal state.",
		"You may answer the user if the task is complete, or use other already-authorized tools when the original request still requires work.",
	}
	if req.AllowToolReinvoke {
		instructions = append(instructions,
			"The previous invocation is closed without applying its proposal. This continuation is not complete until you call manage_digital_human_task again with revised inputs and receive its result.",
			"The user's feedback in user_feedback is already the next instruction for this same Agent turn. Do not ask the user to send another message, repeat the request, or wait for a future turn.",
			"Immediately use the current task_id and latest revision to create a new proposal. Do not apply the revised task directly; return the new proposal through the normal approval flow.",
		)
	} else {
		instructions = append(instructions, "Treat the JSON below as authoritative tool evidence. Continue the original Agent plan from this result; do not rerun the same workflow invocation.")
	}
	if instruction := strings.TrimSpace(req.ContinuationInstruction); instruction != "" {
		instructions = append(instructions, instruction)
	}
	if req.FailureDetailsHidden {
		payload["error_visibility"] = publishedWorkflowErrorVisibility
		instructions = append(instructions, "The failure reason is intentionally hidden. Tell the user only that the workflow run failed; do not include identifiers and do not state or infer a specific reason.")
	}
	instructions = append(instructions, "Workflow task result JSON:\n"+compactJSONForPrompt(payload, 24000))
	return adapter.Message{
		Role:    "user",
		Content: strings.Join(instructions, "\n"),
	}
}

func workflowContinuationStatusFromEvent(eventType string) string {
	switch strings.TrimSpace(eventType) {
	case "workflow_finished":
		return workflowContinuationStatusCompleted
	case "workflow_failed", "error":
		return workflowContinuationStatusFailed
	case "workflow_stopped":
		return workflowContinuationStatusFailed
	case "approval_requested":
		return workflowContinuationStatusWaitingApproval
	case "question_answer_requested":
		return workflowContinuationStatusWaitingQuestion
	default:
		return workflowContinuationStatusContinuing
	}
}

func workflowContinuationMetadataWithStatus(metadata map[string]interface{}, status string) map[string]interface{} {
	next := copyStringAnyMap(metadata)
	if next == nil {
		next = map[string]interface{}{}
	}
	next["agent_workflow_continuation"] = mergeWorkflowMap(
		workflowRecordFromAny(next["agent_workflow_continuation"]),
		map[string]interface{}{"status": strings.TrimSpace(status)},
	)
	return next
}

func workflowContinuationMetadataWithoutUserInputRequest(metadata map[string]interface{}) map[string]interface{} {
	next := copyStringAnyMap(metadata)
	if next == nil {
		return map[string]interface{}{}
	}
	delete(next, "user_input_request")
	return next
}

func workflowSummaryLLMRequest(message *runtimemodel.Message, continuation *WorkflowApprovalContinuation, req WorkflowContinuationSummaryRequest) *adapter.ChatRequest {
	if continuation != nil {
		req = publishedWorkflowContinuationSummaryRequest(continuation.Caller, req)
	}
	provider := ""
	if message != nil && message.ModelProvider != nil {
		provider = strings.TrimSpace(*message.ModelProvider)
	}
	model := ""
	if message != nil {
		model = strings.TrimSpace(message.ModelName)
	}
	outputsJSON := workflowOutputsJSON(req.Outputs)
	errorText := strings.TrimSpace(req.Error)
	status := strings.TrimSpace(req.Status)
	if status == "" {
		status = "succeeded"
	}
	workflowRunID := strings.TrimSpace(req.WorkflowRunID)
	if workflowRunID == "" && continuation != nil {
		workflowRunID = continuation.WorkflowRunID
	}
	userQuery := ""
	if continuation != nil {
		userQuery = strings.TrimSpace(continuation.OriginalQuery)
	}
	content := fmt.Sprintf("Original user request:\n%s\n\nWorkflow run id:\n%s\n\nWorkflow status:\n%s\n\nWorkflow error:\n%s\n\nWorkflow outputs JSON:\n%s", userQuery, workflowRunID, status, errorText, outputsJSON)
	systemPrompt := "You are writing the final response for an Agent after a task workflow completed. Use only the workflow outputs, workflow status, workflow error, and workflow_run_id provided by the user message. Do not invent results, files, approvals, or data that are not present. Do not answer the original user request yourself; treat it only as context for explaining the workflow result. If the workflow outputs do not contain the requested business result, say what the workflow actually returned and include the workflow_run_id. If the workflow failed, explain the failure briefly and include the workflow_run_id. If the workflow outputs are enough, summarize them clearly for the user."
	if req.FailureDetailsHidden {
		systemPrompt = "You are writing the final response for an Agent after a workflow failed. The failure reason is intentionally hidden. Respond in the user's language and tell the user only that the workflow run failed. Do not include identifiers and do not state or infer a specific reason."
	}
	chatReq := &adapter.ChatRequest{
		Provider: provider,
		Model:    model,
		Stream:   true,
		Messages: []adapter.Message{
			{
				Role:    "system",
				Content: systemPrompt,
			},
			{Role: "user", Content: content},
		},
	}
	if message != nil {
		applyModelParameters(chatReq, message.ModelParameters)
	}
	return chatReq
}

func workflowOutputsJSON(outputs map[string]interface{}) string {
	if len(outputs) == 0 {
		return "{}"
	}
	data, err := json.MarshalIndent(outputs, "", "  ")
	if err != nil {
		return "{}"
	}
	return string(data)
}

func workflowNoDisplayableOutputAnswer(workflowRunID string) string {
	workflowRunID = strings.TrimSpace(workflowRunID)
	if workflowRunID == "" {
		return "工作流已运行，但未返回可展示输出。"
	}
	return fmt.Sprintf("工作流已运行，但未返回可展示输出。workflow_run_id: %s", workflowRunID)
}
