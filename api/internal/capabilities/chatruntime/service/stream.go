package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	"github.com/zgiai/zgi/api/internal/capabilities/chatruntime/repository"
	llmclient "github.com/zgiai/zgi/api/internal/modules/llm/client"
	"github.com/zgiai/zgi/api/internal/modules/llm/gateway"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/pkg/logger"
	"github.com/zgiai/zgi/api/pkg/response"
	"gorm.io/gorm"
)

func (s *service) RunPreparedStream(ctx context.Context, prepared *PreparedChat, onChunk func(string) error, onEvent ...func(StreamEvent) error) (*ChatResult, error) {
	if prepared == nil || prepared.Message == nil {
		return nil, fmt.Errorf("%w: prepared chat is required", ErrInvalidInput)
	}
	eventCallback := firstStreamEventCallback(onEvent)
	persistCtx := context.WithoutCancel(ctx)
	runCtx, cancel := context.WithCancel(persistCtx)
	s.streams.Begin(prepared.Message.ID, cancel)
	defer func() {
		cancel()
		s.streams.Finish(prepared.Message.ID)
	}()
	if s.streams.IsStopped(prepared.Message.ID) {
		_ = s.persistStoppedAnswer(persistCtx, prepared, "", nil)
		return nil, ErrMessageStopped
	}
	if err := s.prepareLLMRequestForRun(runCtx, prepared, eventCallback); err != nil {
		if s.isStoppedContext(runCtx, prepared.Message.ID) {
			_ = s.persistStoppedAnswer(persistCtx, prepared, "", nil)
			return nil, ErrMessageStopped
		}
		s.finalizePreparedError(persistCtx, prepared, err, eventCallback)
		return nil, newFinalizedStreamError(err)
	}
	preflightUsage, err := s.runNativeAgentMemoryPreflight(runCtx, persistCtx, prepared, eventCallback)
	if err != nil {
		if s.isStoppedContext(runCtx, prepared.Message.ID) {
			_ = s.persistStoppedAnswer(persistCtx, prepared, "", preflightUsage)
			return nil, ErrMessageStopped
		}
		s.finalizePreparedError(persistCtx, prepared, err, eventCallback)
		return nil, newFinalizedStreamError(err)
	}

	if prepared.skillsEnabled() {
		answer, usage, err := s.runPreparedSkillStream(runCtx, persistCtx, prepared, onChunk, eventCallback)
		usage = mergeUsage(preflightUsage, usage)
		if err != nil {
			if errors.Is(err, ErrMessageStopped) {
				_ = s.clearPreparedRuntime(persistCtx, prepared)
				return nil, err
			}
			if s.isStoppedContext(runCtx, prepared.Message.ID) {
				_ = s.persistStoppedAnswer(persistCtx, prepared, answer, usage)
				return nil, ErrMessageStopped
			}
			s.finalizePreparedError(persistCtx, prepared, err, eventCallback)
			return nil, newFinalizedStreamError(err)
		}
		if s.streams.IsStopped(prepared.Message.ID) {
			_ = s.persistStoppedAnswer(persistCtx, prepared, answer, usage)
			return nil, ErrMessageStopped
		}
		metadata := preparedResultMetadata(prepared.Message.Metadata, usage)
		if err := s.completePreparedChat(persistCtx, prepared, answer, metadata); err != nil {
			_ = s.clearPreparedRuntime(persistCtx, prepared)
			return nil, err
		}
		s.appendStreamEventBestEffort(persistCtx, prepared.Message.ID, prepared.Conversation.ID, streamEventMessageEnd, messageEndPayload(prepared, metadata))
		return &ChatResult{Answer: answer, Metadata: metadata, Usage: usage}, nil
	}

	stream, err := s.openChatStream(runCtx, prepared)
	if err != nil {
		if s.isStoppedContext(runCtx, prepared.Message.ID) {
			_ = s.persistStoppedAnswer(persistCtx, prepared, "", nil)
			return nil, ErrMessageStopped
		}
		s.finalizePreparedError(persistCtx, prepared, err, eventCallback)
		return nil, newFinalizedStreamError(err)
	}
	answer, usage, err := s.collectStreamAnswer(runCtx, prepared, stream, onChunk)
	usage = mergeUsage(preflightUsage, usage)
	if err != nil {
		if errors.Is(err, ErrMessageStopped) {
			_ = s.clearPreparedRuntime(persistCtx, prepared)
			return nil, err
		}
		s.finalizePreparedError(persistCtx, prepared, err, eventCallback)
		return nil, newFinalizedStreamError(err)
	}
	if s.streams.IsStopped(prepared.Message.ID) {
		_ = s.persistStoppedAnswer(persistCtx, prepared, answer, usage)
		return nil, ErrMessageStopped
	}
	metadata := preparedResultMetadata(prepared.Message.Metadata, usage)
	if err := s.completePreparedChat(persistCtx, prepared, answer, metadata); err != nil {
		_ = s.clearPreparedRuntime(persistCtx, prepared)
		return nil, err
	}
	s.appendStreamEventBestEffort(persistCtx, prepared.Message.ID, prepared.Conversation.ID, streamEventMessageEnd, messageEndPayload(prepared, metadata))
	return &ChatResult{Answer: answer, Metadata: metadata, Usage: usage}, nil
}

func (s *service) CleanupStaleActiveMessages(ctx context.Context) (int64, error) {
	cutoff := time.Now().Add(-staleActiveMessageTTL)
	var affected int64
	err := s.repos.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		txRepos := repository.NewRepositories(tx)
		ids, err := txRepos.Message.ListStaleActiveIDs(ctx, cutoff)
		if err != nil {
			return err
		}
		if len(ids) == 0 {
			return nil
		}
		affected, err = txRepos.Message.MarkStaleActiveAsError(ctx, cutoff, staleActiveMessageError)
		if err != nil {
			return err
		}
		return txRepos.Conversation.ClearActiveMessages(ctx, ids)
	})
	return affected, err
}

func (s *service) StreamConversationEvents(ctx context.Context, scope Scope, conversationID, messageID uuid.UUID, afterID string, onEvent func(StreamEvent) error) error {
	if onEvent == nil {
		return fmt.Errorf("%w: event callback is required", ErrInvalidInput)
	}
	if err := s.ensureMember(ctx, scope); err != nil {
		return err
	}
	conversation, err := s.getConversation(ctx, scope, conversationID)
	if err != nil {
		return err
	}
	message, err := s.repos.Message.GetScoped(ctx, messageID, scope.OrganizationID, scope.AccountID)
	if err != nil {
		return mapRepoError(err)
	}
	if message.ConversationID != conversation.ID {
		return fmt.Errorf("%w: message belongs to another conversation", ErrInvalidInput)
	}
	ok, err := s.ensureRecoverableEventStream(ctx, conversation, messageID, onEvent)
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}

	lastID := normalizeStreamAfterID(afterID)
	for {
		events, err := s.events.read(ctx, messageID, lastID, streamEventReadBlock)
		if err != nil {
			return err
		}
		if len(events) == 0 {
			active, err := s.isConversationMessageStreaming(ctx, scope, conversationID, messageID)
			if err != nil {
				return err
			}
			if !active {
				return nil
			}
			continue
		}
		for _, event := range events {
			lastID = event.ID
			event = hydrateStreamEventGeneratedFileURL(event)
			if err := onEvent(event); err != nil {
				return err
			}
			if isTerminalStreamEvent(event.EventType) {
				return nil
			}
		}
	}
}

func (s *service) ensureRecoverableEventStream(ctx context.Context, conversation *runtimemodel.Conversation, messageID uuid.UUID, onEvent func(StreamEvent) error) (bool, error) {
	exists, err := s.events.exists(ctx, messageID)
	if err == nil && exists {
		return true, nil
	}
	if err != nil && !errors.Is(err, ErrStreamEventsUnavailable) {
		return false, err
	}
	if !conversationHasActiveMessage(conversation, messageID) {
		return false, nil
	}
	if err := onEvent(StreamEvent{
		EventType: streamEventError,
		Payload: map[string]interface{}{
			"conversation_id": conversation.ID.String(),
			"message_id":      messageID.String(),
			"message":         streamEventsExpiredError,
		},
		CreatedAt: time.Now().Unix(),
	}); err != nil {
		return false, err
	}
	return false, nil
}

func (s *service) isConversationMessageStreaming(ctx context.Context, scope Scope, conversationID, messageID uuid.UUID) (bool, error) {
	conversation, err := s.getConversation(ctx, scope, conversationID)
	if err != nil {
		return false, err
	}
	return conversationHasActiveMessage(conversation, messageID), nil
}

func conversationHasActiveMessage(conversation *runtimemodel.Conversation, messageID uuid.UUID) bool {
	if conversation == nil || conversation.ActiveMessageID == nil {
		return false
	}
	return conversation.RuntimeStatus == runtimemodel.ConversationRuntimeStatusStreaming && *conversation.ActiveMessageID == messageID
}

func isTerminalStreamEvent(eventType string) bool {
	return eventType == streamEventMessageEnd || eventType == streamEventError
}

func (s *service) openChatStream(ctx context.Context, prepared *PreparedChat) (<-chan adapter.StreamResponse, error) {
	if prepared == nil || prepared.Message == nil || prepared.LLMRequest == nil || prepared.Conversation == nil {
		return nil, fmt.Errorf("%w: prepared chat is required", ErrInvalidInput)
	}
	if s.llmClient == nil {
		err := fmt.Errorf("llm client is not configured")
		_ = s.repos.Message.UpdateError(ctx, prepared.Message.ID, err.Error())
		return nil, err
	}
	stream, err := s.llmClient.AppChatStream(ctx, newBillingAppContext(prepared), prepared.LLMRequest)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		_ = s.repos.Message.UpdateError(ctx, prepared.Message.ID, err.Error())
		return nil, err
	}
	return stream, nil
}

func (s *service) prepareLLMRequestForRun(ctx context.Context, prepared *PreparedChat, onEvent func(StreamEvent) error) error {
	if prepared.LLMRequest != nil {
		return nil
	}
	if prepared.parts == nil {
		return fmt.Errorf("%w: prepared chat parts are required", ErrInvalidInput)
	}
	if err := s.extractPreparedAttachments(ctx, prepared, onEvent); err != nil {
		return err
	}
	metadata := streamingMessageMetadata(prepared.parts)
	prepared.Message.Metadata = metadata
	if err := s.repos.Message.UpdateMetadata(ctx, prepared.Message.ID, metadata); err != nil {
		return err
	}
	contextResult, err := s.buildUpstreamMessages(ctx, prepared.Scope, prepared.ParentID, prepared.parts)
	if err != nil {
		return err
	}
	prepared.parts.ContextControl = contextResult.Metadata
	metadata = streamingMessageMetadata(prepared.parts)
	prepared.Message.Metadata = metadata
	if err := s.repos.Message.UpdateMetadata(ctx, prepared.Message.ID, metadata); err != nil {
		return err
	}
	prepared.LLMRequest = newLLMChatRequest(prepared.parts, contextResult.Messages)
	return nil
}

func firstStreamEventCallback(callbacks []func(StreamEvent) error) func(StreamEvent) error {
	if len(callbacks) == 0 {
		return nil
	}
	return callbacks[0]
}

func newBillingAppContext(prepared *PreparedChat) *llmclient.AppContext {
	source := ""
	if prepared.Message.BillingReasonSource != nil {
		source = strings.TrimSpace(*prepared.Message.BillingReasonSource)
	}
	if source == "" {
		source = runtimemodel.MessageBillingReasonSourceAIChat
	}
	appID := prepared.Conversation.ID.String()
	if strings.TrimSpace(prepared.RunConfig.BillingAppID) != "" {
		appID = strings.TrimSpace(prepared.RunConfig.BillingAppID)
	}
	appType := source
	if strings.TrimSpace(prepared.RunConfig.BillingAppType) != "" {
		appType = strings.TrimSpace(prepared.RunConfig.BillingAppType)
	}
	appCtx := &llmclient.AppContext{
		OrganizationID:     prepared.Conversation.OrganizationID.String(),
		BillingSubjectType: llmclient.BillingSubjectTypeOrganization,
		AppID:              appID,
		AppType:            appType,
		AccountID:          prepared.Conversation.AccountID.String(),
		SessionID:          prepared.Conversation.ID.String(),
		ConversationID:     prepared.Conversation.ID.String(),
	}
	if prepared.Conversation.WorkspaceID != nil {
		appCtx.WorkspaceID = prepared.Conversation.WorkspaceID.String()
	}
	return appCtx
}

func (s *service) collectStreamAnswer(ctx context.Context, prepared *PreparedChat, stream <-chan adapter.StreamResponse, onChunk func(string) error) (string, *adapter.Usage, error) {
	var builder strings.Builder
	var usage *adapter.Usage
	eventBuffer := newStreamMessageEventBuffer(s.events, prepared.Conversation.ID, prepared.Message.ID)
	for {
		select {
		case <-ctx.Done():
			answer := builder.String()
			if s.isStoppedContext(ctx, prepared.Message.ID) {
				_ = eventBuffer.flush(context.WithoutCancel(ctx))
				_ = s.persistStoppedAnswer(context.WithoutCancel(ctx), prepared, answer, usage)
				return answer, usage, ErrMessageStopped
			}
			_ = s.repos.Message.UpdateError(context.WithoutCancel(ctx), prepared.Message.ID, ctx.Err().Error())
			return "", nil, ctx.Err()
		case chunk, ok := <-stream:
			if !ok {
				answer := builder.String()
				_ = eventBuffer.flush(context.WithoutCancel(ctx))
				if s.streams.IsStopped(prepared.Message.ID) {
					_ = s.persistStoppedAnswer(context.WithoutCancel(ctx), prepared, answer, usage)
					return answer, usage, ErrMessageStopped
				}
				return answer, usage, nil
			}
			if chunk.Error != nil {
				answer := builder.String()
				_ = eventBuffer.flush(context.WithoutCancel(ctx))
				if s.streams.IsStopped(prepared.Message.ID) {
					_ = s.persistStoppedAnswer(context.WithoutCancel(ctx), prepared, answer, usage)
					return answer, usage, ErrMessageStopped
				}
				_ = s.repos.Message.UpdateError(context.WithoutCancel(ctx), prepared.Message.ID, chunk.Error.Error())
				return "", nil, chunk.Error
			}
			if chunk.Usage != nil {
				usage = chunk.Usage
			}
			if chunk.Done {
				_ = eventBuffer.flush(context.WithoutCancel(ctx))
				return builder.String(), usage, nil
			}
			text := streamChunkText(chunk)
			if text == "" {
				continue
			}
			builder.WriteString(text)
			if err := eventBuffer.add(ctx, text); err != nil {
				logger.WarnContext(ctx, "failed to append aichat stream message event", "message_id", prepared.Message.ID.String(), err)
			}
			if onChunk != nil {
				if err := onChunk(text); err != nil {
					logger.WarnContext(context.WithoutCancel(ctx), "failed to deliver aichat stream chunk to client", "message_id", prepared.Message.ID.String(), err)
				}
			}
		}
	}
}

func (s *service) completePreparedChat(ctx context.Context, prepared *PreparedChat, answer string, metadata map[string]interface{}) error {
	if err := s.repos.Message.UpdateCompleted(ctx, prepared.Message.ID, answer, metadata); err != nil {
		return err
	}
	if prepared.ReplaceRoot {
		return s.repos.Conversation.CompleteRootReplacement(ctx, prepared.Conversation.ID, prepared.Message.ID)
	}
	if err := s.repos.Conversation.UpdateAfterMessage(ctx, prepared.Conversation.ID, prepared.Message.ID); err != nil {
		return err
	}
	return nil
}

func (s *service) finalizePreparedError(ctx context.Context, prepared *PreparedChat, cause error, onEvent ...func(StreamEvent) error) {
	if prepared == nil || prepared.Message == nil || prepared.Conversation == nil || cause == nil {
		return
	}
	eventCallback := firstStreamEventCallback(onEvent)
	if err := s.completePreparedError(ctx, prepared, cause.Error()); err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			logger.WarnContext(ctx, "failed to finalize aichat message error", "message_id", prepared.Message.ID.String(), err)
		}
		if clearErr := s.clearPreparedRuntime(ctx, prepared); clearErr != nil {
			logger.WarnContext(ctx, "failed to clear aichat conversation runtime", "conversation_id", prepared.Conversation.ID.String(), clearErr)
		}
	}
	s.emitPreparedEvent(ctx, prepared, streamEventError, BuildStreamErrorPayload(prepared, cause), eventCallback)
}

func (s *service) completePreparedError(ctx context.Context, prepared *PreparedChat, message string) error {
	if err := s.repos.Message.UpdateError(ctx, prepared.Message.ID, message); err != nil {
		return err
	}
	if prepared.ReplaceRoot {
		return s.repos.Conversation.CompleteRootReplacement(ctx, prepared.Conversation.ID, prepared.Message.ID)
	}
	return s.repos.Conversation.UpdateAfterMessage(ctx, prepared.Conversation.ID, prepared.Message.ID)
}

func (s *service) clearPreparedRuntime(ctx context.Context, prepared *PreparedChat) error {
	if prepared == nil || prepared.Conversation == nil || prepared.Message == nil {
		return nil
	}
	return s.repos.Conversation.FinishActiveMessage(ctx, prepared.Conversation.ID, prepared.Message.ID)
}

func (s *service) persistStoppedAnswer(ctx context.Context, prepared *PreparedChat, answer string, usage *adapter.Usage) error {
	metadata := preparedResultMetadata(prepared.Message.Metadata, usage)
	metadata["stopped"] = true
	if err := s.repos.Message.UpdateStoppedAnswer(ctx, prepared.Message.ID, answer, metadata); err != nil {
		return err
	}
	if prepared.ReplaceRoot {
		if err := s.repos.Conversation.CompleteRootReplacement(ctx, prepared.Conversation.ID, prepared.Message.ID); err != nil {
			return err
		}
		s.appendStreamEventBestEffort(ctx, prepared.Message.ID, prepared.Conversation.ID, streamEventMessageEnd, messageEndPayload(prepared, metadata))
		return nil
	}
	if err := s.repos.Conversation.UpdateAfterMessage(ctx, prepared.Conversation.ID, prepared.Message.ID); err != nil {
		return err
	}
	s.appendStreamEventBestEffort(ctx, prepared.Message.ID, prepared.Conversation.ID, streamEventMessageEnd, messageEndPayload(prepared, metadata))
	return nil
}

func (s *service) appendStreamEventBestEffort(ctx context.Context, messageID uuid.UUID, conversationID uuid.UUID, eventType string, payload map[string]interface{}) *StreamEvent {
	if s.events == nil {
		return nil
	}
	event, err := s.events.append(ctx, messageID, conversationID, eventType, payload)
	if err != nil {
		if errors.Is(err, ErrStreamEventsUnavailable) {
			return nil
		}
		logger.WarnContext(ctx, "failed to append aichat stream event", "message_id", messageID.String(), "event_type", eventType, err)
		return nil
	}
	return event
}

func (s *service) emitPreparedEvent(ctx context.Context, prepared *PreparedChat, eventType string, payload map[string]interface{}, onEvent func(StreamEvent) error) {
	if prepared == nil || prepared.Message == nil || prepared.Conversation == nil {
		return
	}
	event := s.appendStreamEventBestEffort(ctx, prepared.Message.ID, prepared.Conversation.ID, eventType, payload)
	if onEvent == nil {
		return
	}
	if event == nil {
		event = &StreamEvent{
			EventType: eventType,
			Payload:   payload,
			CreatedAt: time.Now().Unix(),
		}
	}
	if err := onEvent(StreamEvent{
		ID:        event.ID,
		EventType: event.EventType,
		Payload:   event.Payload,
		CreatedAt: event.CreatedAt,
	}); err != nil {
		logger.WarnContext(ctx, "failed to deliver aichat stream event", "message_id", prepared.Message.ID.String(), "event_type", eventType, err)
	}
}

func (s *service) resetStreamEventsBestEffort(ctx context.Context, messageID uuid.UUID) {
	if err := s.events.reset(ctx, messageID); err != nil {
		if errors.Is(err, ErrStreamEventsUnavailable) {
			return
		}
		logger.WarnContext(ctx, "failed to reset aichat stream events", "message_id", messageID.String(), err)
	}
}

func messageStartPayload(conversation *runtimemodel.Conversation, message *runtimemodel.Message, replace bool) map[string]interface{} {
	return map[string]interface{}{
		"conversation_id": conversation.ID.String(),
		"message_id":      message.ID.String(),
		"parent_id":       uuidStringValue(message.ParentID),
		"title":           conversation.Title,
		"model":           message.ModelName,
		"replace":         replace,
		"created_at":      message.CreatedAt.Unix(),
	}
}

func fileParseStartPayload(prepared *PreparedChat, file attachmentFile, index, total int) map[string]interface{} {
	payload := baseFileParsePayload(prepared, file, index, total)
	payload["status"] = "parsing"
	return payload
}

func fileParseEndPayload(prepared *PreparedChat, file attachmentFile, index, total int) map[string]interface{} {
	payload := baseFileParsePayload(prepared, file, index, total)
	payload["status"] = "completed"
	payload["content_status"] = file.ContentStatus
	payload["content_chars"] = file.ContentChars
	payload["from_cache"] = file.FromCache
	if strings.TrimSpace(file.VisionDetail) != "" {
		payload["vision_detail"] = file.VisionDetail
	}
	if strings.TrimSpace(file.FilteredReason) != "" {
		payload["filtered_reason"] = file.FilteredReason
	}
	return payload
}

func fileParseErrorPayload(prepared *PreparedChat, file attachmentFile, index, total int, message string) map[string]interface{} {
	payload := baseFileParsePayload(prepared, file, index, total)
	payload["status"] = "error"
	payload["message"] = message
	return payload
}

func baseFileParsePayload(prepared *PreparedChat, file attachmentFile, index, total int) map[string]interface{} {
	return map[string]interface{}{
		"conversation_id": prepared.Conversation.ID.String(),
		"message_id":      prepared.Message.ID.String(),
		"file_id":         file.ID,
		"name":            file.Name,
		"kind":            file.kind(),
		"index":           index + 1,
		"total":           total,
	}
}

func messageEndPayload(prepared *PreparedChat, metadata map[string]interface{}) map[string]interface{} {
	return map[string]interface{}{
		"conversation_id": prepared.Conversation.ID.String(),
		"message_id":      prepared.Message.ID.String(),
		"status":          completedStatusFromMetadata(metadata),
		"metadata": map[string]interface{}{
			"usage": metadata["usage"],
		},
	}
}

// BuildStreamErrorPayload returns the public SSE error payload for an AIChat turn.
func BuildStreamErrorPayload(prepared *PreparedChat, err error) map[string]interface{} {
	message := streamFallbackErrorMessage(err)
	payload := map[string]interface{}{
		"conversation_id": prepared.Conversation.ID.String(),
		"message_id":      prepared.Message.ID.String(),
		"message":         message,
	}
	if code, billingMessage, ok := aichatBillingErrorCodeAndMessage(err); ok {
		payload["message"] = billingMessage
		payload["code"] = code
		payload["params"] = map[string]interface{}{}
	}
	return payload
}

func streamFallbackErrorMessage(err error) string {
	if err == nil {
		return "unknown error"
	}
	return err.Error()
}

func aichatBillingErrorCodeAndMessage(err error) (int, string, bool) {
	var userErr *gateway.BillingUserError
	if !errors.As(err, &userErr) || userErr == nil {
		return 0, "", false
	}

	switch userErr.Kind {
	case gateway.BillingUserErrorKindOrganizationBalanceInsufficient:
		return response.ErrWorkflowOrganizationBalanceInsufficient.Code, response.ErrWorkflowOrganizationBalanceInsufficient.Message, true
	case gateway.BillingUserErrorKindWorkspaceQuotaInsufficient:
		return response.ErrWorkflowWorkspaceQuotaInsufficient.Code, response.ErrWorkflowWorkspaceQuotaInsufficient.Message, true
	case gateway.BillingUserErrorKindPrivateChannelBalanceInsufficient:
		return response.ErrWorkflowPrivateChannelBalanceInsufficient.Code, response.ErrWorkflowPrivateChannelBalanceInsufficient.Message, true
	default:
		return 0, "", false
	}
}

func completedStatusFromMetadata(metadata map[string]interface{}) string {
	if stopped, ok := metadata["stopped"].(bool); ok && stopped {
		return runtimemodel.MessageStatusStopped
	}
	return runtimemodel.MessageStatusCompleted
}

func uuidStringValue(value *uuid.UUID) interface{} {
	if value == nil {
		return nil
	}
	return value.String()
}

func (s *service) isStoppedContext(ctx context.Context, messageID uuid.UUID) bool {
	return s.streams.IsStopped(messageID) || (errors.Is(ctx.Err(), context.Canceled) && s.streams.IsStopped(messageID))
}

func preparedResultMetadata(source map[string]interface{}, usage *adapter.Usage) map[string]interface{} {
	metadata := copyStringAnyMap(source)
	if metadata == nil {
		metadata = map[string]interface{}{}
	}
	metadata["usage"] = usageMetadata(usage)
	metadata["system_prompt_version"] = systemPromptVersion
	return metadata
}

func streamChunkText(resp adapter.StreamResponse) string {
	if len(resp.Choices) == 0 {
		return ""
	}
	content := resp.Choices[0].Delta.Content
	switch typed := content.(type) {
	case string:
		return typed
	default:
		return ""
	}
}
