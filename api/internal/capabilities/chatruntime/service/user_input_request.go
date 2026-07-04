package service

import (
	"context"

	"github.com/zgiai/zgi/api/internal/capabilities/chatruntime/repository"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/pkg/logger"
)

func (s *service) persistUserInputRequestBestEffort(ctx context.Context, prepared *PreparedChat, payload map[string]interface{}) {
	if prepared == nil || prepared.Message == nil || len(payload) == 0 {
		return
	}
	metadata := mergeUserInputRequestMetadata(prepared.Message.Metadata, payload)
	prepared.Message.Metadata = metadata
	if s == nil || s.repos == nil || s.repos.Message == nil {
		return
	}
	if err := s.repos.Message.UpdateMetadata(ctx, prepared.Message.ID, metadata); err != nil {
		logger.WarnContext(ctx, "failed to persist aichat user input request metadata", "message_id", prepared.Message.ID.String(), err)
	}
}

func mergeUserInputRequestMetadata(source map[string]interface{}, payload map[string]interface{}) map[string]interface{} {
	metadata := copyStringAnyMap(source)
	if metadata == nil {
		metadata = map[string]interface{}{}
	}
	request := map[string]interface{}{
		"request_id": payload["request_id"],
		"questions":  payload["questions"],
		"created_at": payload["created_at"],
	}
	metadata["user_input_request"] = request
	return metadata
}

func (s *service) persistUserInputRequestPending(ctx context.Context, prepared *PreparedChat, payload map[string]interface{}, usage *adapter.Usage) map[string]interface{} {
	if prepared == nil || prepared.Message == nil || prepared.Conversation == nil {
		return map[string]interface{}{}
	}
	pendingPayload := copyStringAnyMap(payload)
	if pendingPayload == nil {
		pendingPayload = map[string]interface{}{}
	}
	pendingPayload["conversation_id"] = prepared.Conversation.ID.String()
	pendingPayload["message_id"] = prepared.Message.ID.String()

	metadata := mergeUserInputRequestMetadata(prepared.Message.Metadata, pendingPayload)
	metadata = preparedResultMetadata(metadata, usage)
	metadata["user_input_continuation"] = compactSkillInvocation(map[string]interface{}{
		"status":         "waiting_question",
		"request_id":     pendingPayload["request_id"],
		"original_query": prepared.Message.Query,
		"resume_policy":  "same_message",
	})
	prepared.Message.Metadata = metadata

	if s == nil || s.repos == nil || s.repos.Message == nil || s.repos.Conversation == nil {
		return metadata
	}
	s.persistPendingMessageAndFinishConversationBestEffort(
		ctx,
		prepared,
		"user input request",
		func(repo repository.MessageRepository) error {
			return repo.UpdateWaitingQuestion(ctx, prepared.Message.ID, metadata)
		},
		func(repo repository.ConversationRepository) error {
			return repo.FinishWaitingApprovalMessage(ctx, prepared.Conversation.ID, prepared.Message.ID)
		},
	)
	return metadata
}
