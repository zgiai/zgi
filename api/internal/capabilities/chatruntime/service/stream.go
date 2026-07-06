package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	"github.com/zgiai/zgi/api/internal/capabilities/chatruntime/repository"
	"github.com/zgiai/zgi/api/internal/capabilities/chatruntime/skillloop"
	llmclient "github.com/zgiai/zgi/api/internal/modules/llm/client"
	"github.com/zgiai/zgi/api/internal/modules/llm/gateway"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/pkg/logger"
	"github.com/zgiai/zgi/api/pkg/response"
	"go.uber.org/zap"
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
	userMemoryUsage, err := s.runUserMemoryPreflight(runCtx, persistCtx, prepared, eventCallback)
	if err != nil {
		if s.isStoppedContext(runCtx, prepared.Message.ID) {
			_ = s.persistStoppedAnswer(persistCtx, prepared, "", userMemoryUsage)
			return nil, ErrMessageStopped
		}
		s.finalizePreparedError(persistCtx, prepared, err, eventCallback)
		return nil, newFinalizedStreamError(err)
	}
	agentMemoryUsage, err := s.runNativeAgentMemoryPreflight(runCtx, persistCtx, prepared, eventCallback)
	preflightUsage := mergeUsage(userMemoryUsage, agentMemoryUsage)
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
			var pendingApproval *skillloop.WorkflowApprovalPendingError
			if errors.As(err, &pendingApproval) {
				metadata := s.persistWorkflowApprovalPending(persistCtx, prepared, pendingApproval.Payload, usage)
				s.emitPreparedEvent(persistCtx, prepared, streamEventMessageEnd, messageEndPayloadWithStatus(prepared, metadata, runtimemodel.MessageStatusWaitingApproval), eventCallback)
				return &ChatResult{Answer: answer, Metadata: metadata, Usage: usage}, nil
			}
			var pendingQuestion *skillloop.WorkflowQuestionPendingError
			if errors.As(err, &pendingQuestion) {
				metadata := s.persistWorkflowQuestionPending(persistCtx, prepared, pendingQuestion.Payload, usage)
				s.emitPreparedEvent(persistCtx, prepared, streamEventMessageEnd, messageEndPayloadWithStatus(prepared, metadata, runtimemodel.MessageStatusWaitingQuestion), eventCallback)
				return &ChatResult{Answer: answer, Metadata: metadata, Usage: usage}, nil
			}
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

	finalCallStartedAt := time.Now()
	stream, err := s.openChatStream(runCtx, prepared)
	if err != nil {
		s.persistModelInvocationBestEffort(persistCtx, prepared, skillloop.ModelInvocationTrace{
			Phase:      "final_answer",
			Round:      -1,
			Streaming:  true,
			StartedAt:  finalCallStartedAt,
			DurationMS: time.Since(finalCallStartedAt).Milliseconds(),
			Request:    prepared.LLMRequest,
			Error:      err.Error(),
		})
		if s.isStoppedContext(runCtx, prepared.Message.ID) {
			_ = s.persistStoppedAnswer(persistCtx, prepared, "", nil)
			return nil, ErrMessageStopped
		}
		s.finalizePreparedError(persistCtx, prepared, err, eventCallback)
		return nil, newFinalizedStreamError(err)
	}
	modelChunkCallback := modelStreamChunkCallback(eventCallback, onChunk)
	answer, callUsage, err := s.collectStreamAnswerWithEvents(runCtx, prepared, stream, eventCallback, modelChunkCallback)
	usage := mergeUsage(preflightUsage, callUsage)
	if err != nil {
		s.persistModelInvocationBestEffort(persistCtx, prepared, skillloop.ModelInvocationTrace{
			Phase:      "final_answer",
			Round:      -1,
			Streaming:  true,
			StartedAt:  finalCallStartedAt,
			DurationMS: time.Since(finalCallStartedAt).Milliseconds(),
			Request:    prepared.LLMRequest,
			Response:   &adapter.Message{Role: "assistant", Content: answer},
			Usage:      callUsage,
			Error:      err.Error(),
		})
		if errors.Is(err, ErrMessageStopped) {
			_ = s.clearPreparedRuntime(persistCtx, prepared)
			return nil, err
		}
		s.finalizePreparedError(persistCtx, prepared, err, eventCallback)
		return nil, newFinalizedStreamError(err)
	}
	s.persistModelInvocationBestEffort(persistCtx, prepared, skillloop.ModelInvocationTrace{
		Phase:      "final_answer",
		Round:      -1,
		Streaming:  true,
		StartedAt:  finalCallStartedAt,
		DurationMS: time.Since(finalCallStartedAt).Milliseconds(),
		Request:    prepared.LLMRequest,
		Response:   &adapter.Message{Role: "assistant", Content: answer},
		Usage:      callUsage,
	})
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
	return s.StreamConversationEventsForCaller(ctx, scope, Caller{Type: runtimemodel.ConversationCallerAIChat}, conversationID, messageID, afterID, onEvent)
}

func (s *service) StreamConversationEventsForCaller(ctx context.Context, scope Scope, caller Caller, conversationID, messageID uuid.UUID, afterID string, onEvent func(StreamEvent) error) error {
	if onEvent == nil {
		return fmt.Errorf("%w: event callback is required", ErrInvalidInput)
	}
	if err := s.ensureMember(ctx, scope); err != nil {
		return err
	}
	conversation, err := s.repos.Conversation.GetByCallerScoped(ctx, conversationID, scope.OrganizationID, scope.AccountID, normalizeCallerType(caller.Type), normalizeCallerID(caller.ID))
	if err != nil {
		return mapRepoError(err)
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
			active, err := s.isConversationMessageStreamingForCaller(ctx, scope, caller, conversationID, messageID)
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
			if isTerminalStreamEvent(event) {
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
	return s.isConversationMessageStreamingForCaller(ctx, scope, Caller{Type: runtimemodel.ConversationCallerAIChat}, conversationID, messageID)
}

func (s *service) isConversationMessageStreamingForCaller(ctx context.Context, scope Scope, caller Caller, conversationID, messageID uuid.UUID) (bool, error) {
	conversation, err := s.repos.Conversation.GetByCallerScoped(ctx, conversationID, scope.OrganizationID, scope.AccountID, normalizeCallerType(caller.Type), normalizeCallerID(caller.ID))
	if err != nil {
		return false, mapRepoError(err)
	}
	return conversationHasActiveMessage(conversation, messageID), nil
}

func conversationHasActiveMessage(conversation *runtimemodel.Conversation, messageID uuid.UUID) bool {
	if conversation == nil || conversation.ActiveMessageID == nil {
		return false
	}
	return conversation.RuntimeStatus == runtimemodel.ConversationRuntimeStatusStreaming && *conversation.ActiveMessageID == messageID
}

func isTerminalStreamEvent(event StreamEvent) bool {
	switch event.EventType {
	case streamEventError:
		return true
	case streamEventMessageEnd:
		status := strings.ToLower(strings.TrimSpace(fmt.Sprint(event.Payload["status"])))
		switch status {
		case runtimemodel.MessageStatusCompleted, runtimemodel.MessageStatusStopped, runtimemodel.MessageStatusError, "failed":
			return true
		default:
			return false
		}
	default:
		return false
	}
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
	return s.collectStreamAnswerWithEvents(ctx, prepared, stream, nil, onChunk)
}

func (s *service) collectStreamAnswerWithEvents(ctx context.Context, prepared *PreparedChat, stream <-chan adapter.StreamResponse, onEvent func(StreamEvent) error, onChunk func(string) error) (string, *adapter.Usage, error) {
	var builder strings.Builder
	var usage *adapter.Usage
	serviceChunkIndex := 0
	eventBuffer := newStreamMessageEventBuffer(s.events, prepared.Conversation.ID, prepared.Message.ID)
	for {
		select {
		case <-ctx.Done():
			answer := builder.String()
			if s.isStoppedContext(ctx, prepared.Message.ID) {
				s.flushStreamMessageEventBuffer(context.WithoutCancel(ctx), prepared.Message.ID, eventBuffer, onEvent)
				_ = s.persistStoppedAnswer(context.WithoutCancel(ctx), prepared, answer, usage)
				return answer, usage, ErrMessageStopped
			}
			_ = s.repos.Message.UpdateError(context.WithoutCancel(ctx), prepared.Message.ID, ctx.Err().Error())
			return "", nil, ctx.Err()
		case chunk, ok := <-stream:
			serviceChunkIndex++
			if qwenRuntimeStreamDebugEnabled() {
				logger.InfoContext(ctx, "aichat runtime stream chunk",
					zap.String("model", prepared.Message.ModelName),
					zap.String("message_id", prepared.Message.ID.String()),
					zap.Int("chunk_index", serviceChunkIndex),
					zap.Int("choices", len(chunk.Choices)),
					zap.Int("text_len", runtimeStreamResponseTextLen(chunk)),
					zap.Bool("done", chunk.Done),
					zap.Bool("has_usage", chunk.Usage != nil),
					zap.Bool("has_error", chunk.Error != nil),
				)
			}
			if !ok {
				answer := builder.String()
				s.flushStreamMessageEventBuffer(context.WithoutCancel(ctx), prepared.Message.ID, eventBuffer, onEvent)
				if s.streams.IsStopped(prepared.Message.ID) {
					_ = s.persistStoppedAnswer(context.WithoutCancel(ctx), prepared, answer, usage)
					return answer, usage, ErrMessageStopped
				}
				return answer, usage, nil
			}
			if chunk.Error != nil {
				answer := builder.String()
				s.flushStreamMessageEventBuffer(context.WithoutCancel(ctx), prepared.Message.ID, eventBuffer, onEvent)
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
				s.flushStreamMessageEventBuffer(context.WithoutCancel(ctx), prepared.Message.ID, eventBuffer, onEvent)
				return builder.String(), usage, nil
			}
			reasoning := streamChunkReasoningContent(chunk)
			if reasoning != "" {
				appendPreparedReasoningContent(prepared, reasoning)
				s.flushStreamMessageEventBuffer(ctx, prepared.Message.ID, eventBuffer, onEvent)
				event, err := s.appendStreamReasoningEvent(ctx, prepared, reasoning)
				if err != nil {
					logger.WarnContext(ctx, "failed to append aichat stream reasoning event", "message_id", prepared.Message.ID.String(), err)
				}
				s.deliverStreamEvent(ctx, prepared.Message.ID, event, onEvent)
			}
			text := streamChunkText(chunk)
			if text == "" {
				continue
			}
			builder.WriteString(text)
			event, err := eventBuffer.add(ctx, text)
			if err != nil {
				logger.WarnContext(ctx, "failed to append aichat stream message event", "message_id", prepared.Message.ID.String(), err)
			}
			s.deliverStreamEvent(ctx, prepared.Message.ID, event, onEvent)
			if onChunk != nil {
				if err := onChunk(text); err != nil {
					logger.WarnContext(context.WithoutCancel(ctx), "failed to deliver aichat stream chunk to client", "message_id", prepared.Message.ID.String(), err)
				}
			}
		}
	}
}

func (s *service) flushStreamMessageEventBuffer(ctx context.Context, messageID uuid.UUID, eventBuffer *streamMessageEventBuffer, onEvent func(StreamEvent) error) {
	event, err := eventBuffer.flush(ctx)
	if err != nil {
		logger.WarnContext(ctx, "failed to append aichat stream message event", "message_id", messageID.String(), err)
		return
	}
	s.deliverStreamEvent(ctx, messageID, event, onEvent)
}

func (s *service) deliverStreamEvent(ctx context.Context, messageID uuid.UUID, event *StreamEvent, onEvent func(StreamEvent) error) {
	if event == nil || onEvent == nil {
		return
	}
	if err := onEvent(StreamEvent{
		ID:        event.ID,
		EventType: event.EventType,
		Payload:   event.Payload,
		CreatedAt: event.CreatedAt,
	}); err != nil {
		logger.WarnContext(context.WithoutCancel(ctx), "failed to deliver aichat stream event to client", "message_id", messageID.String(), "event_type", event.EventType, err)
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
	return messageEndPayloadWithStatus(prepared, metadata, completedStatusFromMetadata(metadata))
}

func messageEndPayloadWithStatus(prepared *PreparedChat, metadata map[string]interface{}, status string) map[string]interface{} {
	return map[string]interface{}{
		"conversation_id": prepared.Conversation.ID.String(),
		"message_id":      prepared.Message.ID.String(),
		"status":          strings.TrimSpace(status),
		"metadata":        copyStringAnyMap(metadata),
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
		payload["params"] = aichatBillingErrorParams(err)
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
	case gateway.BillingUserErrorKindModelPricingNotConfigured:
		return response.ErrWorkflowModelPricingNotConfigured.Code, response.ErrWorkflowModelPricingNotConfigured.Message, true
	default:
		return 0, "", false
	}
}

func aichatBillingErrorParams(err error) map[string]interface{} {
	var userErr *gateway.BillingUserError
	if !errors.As(err, &userErr) || userErr == nil || len(userErr.Params) == 0 {
		return map[string]interface{}{}
	}
	params := make(map[string]interface{}, len(userErr.Params))
	for key, value := range userErr.Params {
		params[key] = value
	}
	return params
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

func appendPreparedReasoningContent(prepared *PreparedChat, reasoning string) {
	if prepared == nil || prepared.Message == nil || reasoning == "" {
		return
	}
	metadata := copyStringAnyMap(prepared.Message.Metadata)
	if metadata == nil {
		metadata = map[string]interface{}{}
	}
	metadata["reasoning_content"] = stringFromAny(metadata["reasoning_content"]) + reasoning
	prepared.Message.Metadata = metadata
}

func (s *service) appendStreamReasoningEvent(ctx context.Context, prepared *PreparedChat, reasoning string) (*StreamEvent, error) {
	if s == nil || prepared == nil || prepared.Message == nil || prepared.Conversation == nil || reasoning == "" {
		return nil, nil
	}
	payload := map[string]interface{}{
		"conversation_id":   prepared.Conversation.ID.String(),
		"message_id":        prepared.Message.ID.String(),
		"answer":            "",
		"reasoning_content": reasoning,
	}
	if !s.events.available() {
		return &StreamEvent{EventType: streamEventMessage, Payload: payload, CreatedAt: time.Now().Unix()}, nil
	}
	return s.events.append(ctx, prepared.Message.ID, prepared.Conversation.ID, streamEventMessage, payload)
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

func modelStreamChunkCallback(eventCallback func(StreamEvent) error, onChunk func(string) error) func(string) error {
	if eventCallback != nil {
		return nil
	}
	return onChunk
}

func streamChunkReasoningContent(resp adapter.StreamResponse) string {
	if len(resp.Choices) == 0 {
		return ""
	}
	return resp.Choices[0].Delta.ReasoningContent
}

func qwenRuntimeStreamDebugEnabled() bool {
	return strings.TrimSpace(os.Getenv("ZGI_DEBUG_ALIYUN_STREAM")) == "1"
}

func runtimeStreamResponseTextLen(resp adapter.StreamResponse) int {
	return len(streamChunkText(resp))
}
