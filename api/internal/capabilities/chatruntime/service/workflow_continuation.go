package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	"github.com/zgiai/zgi/api/internal/capabilities/chatruntime/repository"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"gorm.io/gorm"
)

const (
	workflowContinuationStatusWaitingApproval = "waiting_approval"
	workflowContinuationStatusContinuing      = "continuing"
	workflowContinuationStatusSummarizing     = "summarizing"
	workflowContinuationStatusDirectOutput    = "direct_output"
	workflowContinuationStatusCompleted       = "completed"
	workflowContinuationStatusFailed          = "failed"
)

type WorkflowApprovalContinuation struct {
	ConversationID uuid.UUID
	MessageID      uuid.UUID
	WorkflowRunID  string
	AgentType      string
	BindingID      string
	OriginalQuery  string
	Completed      bool
	Metadata       map[string]interface{}
}

type WorkflowContinuationSummaryRequest struct {
	WorkflowRunID string
	Status        string
	Outputs       map[string]interface{}
	Error         string
}

func (s *service) BeginWorkflowApprovalContinuation(ctx context.Context, scope Scope, caller Caller, conversationID, messageID uuid.UUID) (*WorkflowApprovalContinuation, error) {
	if err := s.ensureMember(ctx, scope); err != nil {
		return nil, err
	}
	conversation, err := s.repos.Conversation.GetByCallerScoped(ctx, conversationID, scope.OrganizationID, scope.AccountID, normalizeCallerType(caller.Type), normalizeCallerID(caller.ID))
	if err != nil {
		return nil, mapRepoError(err)
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
		return nil, fmt.Errorf("%w: message has no pending workflow approval continuation", ErrInvalidInput)
	}
	state.ConversationID = conversation.ID
	state.MessageID = message.ID
	state.OriginalQuery = message.Query
	state.Metadata = copyStringAnyMap(message.Metadata)
	if message.Status == runtimemodel.MessageStatusCompleted {
		state.Completed = true
		return state, nil
	}
	if message.Status != runtimemodel.MessageStatusWaitingApproval && message.Status != runtimemodel.MessageStatusStreaming {
		return nil, fmt.Errorf("%w: message is not waiting for workflow approval", ErrInvalidInput)
	}
	if message.Status == runtimemodel.MessageStatusWaitingApproval {
		err = s.repos.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			txRepos := repository.NewRepositories(tx)
			if err := txRepos.Conversation.StartStreaming(ctx, conversation.ID, scope.OrganizationID, scope.AccountID, message.ID); err != nil {
				return err
			}
			return tx.Model(&runtimemodel.Message{}).
				Where("id = ? AND deleted_at IS NULL AND status = ?", message.ID, runtimemodel.MessageStatusWaitingApproval).
				Updates(map[string]interface{}{"status": runtimemodel.MessageStatusStreaming}).Error
		})
		if err != nil {
			return nil, err
		}
	}
	return state, nil
}

func (s *service) RecordWorkflowApprovalContinuationEvent(ctx context.Context, continuation *WorkflowApprovalContinuation, eventType string, payload map[string]interface{}) (map[string]interface{}, error) {
	if continuation == nil || continuation.MessageID == uuid.Nil {
		return nil, fmt.Errorf("%w: workflow continuation is required", ErrInvalidInput)
	}
	eventPayload := copyStringAnyMap(payload)
	if eventPayload == nil {
		eventPayload = map[string]interface{}{}
	}
	eventPayload["conversation_id"] = continuation.ConversationID.String()
	eventPayload["message_id"] = continuation.MessageID.String()
	if _, ok := eventPayload["workflow_run_id"]; !ok && continuation.WorkflowRunID != "" {
		eventPayload["workflow_run_id"] = continuation.WorkflowRunID
	}
	metadata := mergeWorkflowRunMetadata(continuation.Metadata, eventType, eventPayload)
	metadata["agent_workflow_continuation"] = mergeWorkflowMap(
		workflowRecordFromAny(metadata["agent_workflow_continuation"]),
		map[string]interface{}{"status": workflowContinuationStatusFromEvent(eventType)},
	)
	continuation.Metadata = metadata
	if err := s.repos.Message.UpdateMetadata(ctx, continuation.MessageID, metadata); err != nil {
		return nil, err
	}
	return eventPayload, nil
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

func (s *service) SummarizeWorkflowApprovalContinuation(ctx context.Context, scope Scope, continuation *WorkflowApprovalContinuation, req WorkflowContinuationSummaryRequest, onChunk func(string) error) (*ChatResult, error) {
	if continuation == nil || continuation.MessageID == uuid.Nil || continuation.ConversationID == uuid.Nil {
		return nil, fmt.Errorf("%w: workflow continuation is required", ErrInvalidInput)
	}
	if len(req.Outputs) == 0 {
		answer := workflowNoDisplayableOutputAnswer(req.WorkflowRunID)
		metadata, err := s.CompleteWorkflowApprovalContinuation(ctx, continuation, answer, workflowContinuationStatusCompleted)
		if err != nil {
			return nil, err
		}
		if onChunk != nil && answer != "" {
			if err := onChunk(answer); err != nil {
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
	}
	runCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	s.streams.Begin(message.ID, cancel)
	defer func() {
		cancel()
		s.streams.Finish(message.ID)
	}()
	if s.streams.IsStopped(message.ID) {
		_ = s.persistStoppedAnswer(context.WithoutCancel(ctx), prepared, "", nil)
		return nil, ErrMessageStopped
	}
	stream, err := s.openChatStream(runCtx, prepared)
	if err != nil {
		return nil, err
	}
	answer, usage, err := s.collectStreamAnswer(runCtx, prepared, stream, onChunk)
	if err != nil {
		return nil, err
	}
	if s.streams.IsStopped(message.ID) {
		_ = s.persistStoppedAnswer(context.WithoutCancel(ctx), prepared, answer, usage)
		return nil, ErrMessageStopped
	}
	metadata = preparedResultMetadata(continuation.Metadata, usage)
	continuation.Metadata = metadata
	metadata, err = s.CompleteWorkflowApprovalContinuation(context.WithoutCancel(ctx), continuation, answer, workflowContinuationStatusCompleted)
	if err != nil {
		return nil, err
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
	metadata = workflowContinuationMetadataWithStatus(metadata, status)
	continuation.Metadata = metadata
	if err := s.repos.Message.UpdateCompleted(ctx, continuation.MessageID, answer, metadata); err != nil {
		return nil, err
	}
	if err := s.repos.Conversation.FinishContinuationMessage(ctx, continuation.ConversationID, continuation.MessageID); err != nil {
		return nil, err
	}
	return metadata, nil
}

func (s *service) FailWorkflowApprovalContinuation(ctx context.Context, continuation *WorkflowApprovalContinuation, message string) (map[string]interface{}, error) {
	if continuation == nil || continuation.MessageID == uuid.Nil || continuation.ConversationID == uuid.Nil {
		return nil, fmt.Errorf("%w: workflow continuation is required", ErrInvalidInput)
	}
	message = strings.TrimSpace(message)
	if message == "" {
		message = "workflow approval continuation failed"
	}
	metadata := workflowContinuationMetadataWithStatus(continuation.Metadata, workflowContinuationStatusFailed)
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
	return metadata, nil
}

func workflowApprovalContinuationFromMetadata(metadata map[string]interface{}) *WorkflowApprovalContinuation {
	state := workflowRecordFromAny(metadata["agent_workflow_continuation"])
	return &WorkflowApprovalContinuation{
		WorkflowRunID: firstNonEmptyString(state["workflow_run_id"]),
		AgentType:     firstNonEmptyString(state["agent_type"]),
		BindingID:     firstNonEmptyString(state["binding_id"]),
		OriginalQuery: firstNonEmptyString(state["original_query"]),
		Metadata:      copyStringAnyMap(metadata),
	}
}

func workflowContinuationStatusFromEvent(eventType string) string {
	switch strings.TrimSpace(eventType) {
	case "workflow_finished":
		return "finishing"
	case "workflow_failed", "error":
		return workflowContinuationStatusFailed
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

func workflowSummaryLLMRequest(message *runtimemodel.Message, continuation *WorkflowApprovalContinuation, req WorkflowContinuationSummaryRequest) *adapter.ChatRequest {
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
	chatReq := &adapter.ChatRequest{
		Provider: provider,
		Model:    model,
		Stream:   true,
		Messages: []adapter.Message{
			{
				Role:    "system",
				Content: "You are writing the final response for an Agent after a task workflow completed. Use only the workflow outputs, workflow status, workflow error, and workflow_run_id provided by the user message. Do not invent results, files, approvals, or data that are not present. Do not answer the original user request yourself; treat it only as context for explaining the workflow result. If the workflow outputs do not contain the requested business result, say what the workflow actually returned and include the workflow_run_id. If the workflow failed, explain the failure briefly and include the workflow_run_id. If the workflow outputs are enough, summarize them clearly for the user.",
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
