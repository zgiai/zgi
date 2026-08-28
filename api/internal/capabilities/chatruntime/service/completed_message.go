package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	"github.com/zgiai/zgi/api/internal/capabilities/chatruntime/repository"
	"gorm.io/gorm"
)

func (s *service) CreateCompletedMessage(ctx context.Context, scope Scope, req CreateCompletedMessageRequest) (*runtimemodel.Message, error) {
	if req.ConversationID == uuid.Nil {
		return nil, ErrConversationMissing
	}
	conversation, err := s.getConversation(ctx, scope, req.ConversationID)
	if err != nil {
		return nil, err
	}

	message := completedMessageForConversation(conversation, req)

	if err := s.repos.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		txRepos := repository.NewRepositories(tx)
		if err := txRepos.Message.Create(ctx, message); err != nil {
			return err
		}
		return txRepos.Conversation.UpdateAfterMessage(ctx, conversation.ID, message.ID)
	}); err != nil {
		return nil, err
	}
	return message, nil
}

func (s *service) CreatePendingMessage(ctx context.Context, scope Scope, req CreatePendingMessageRequest) (*runtimemodel.Message, error) {
	if req.ConversationID == uuid.Nil {
		return nil, ErrConversationMissing
	}
	conversation, err := s.getConversation(ctx, scope, req.ConversationID)
	if err != nil {
		return nil, err
	}

	message := pendingMessageForConversation(conversation, req)
	if err := s.repos.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		txRepos := repository.NewRepositories(tx)
		if err := txRepos.Message.Create(ctx, message); err != nil {
			return err
		}
		return startVisiblePendingMessage(ctx, tx, scope, conversation.ID, message.ID)
	}); err != nil {
		return nil, err
	}
	return message, nil
}

func (s *service) CreateConversationWithCompletedMessage(ctx context.Context, scope Scope, caller Caller, req CreateConversationWithCompletedMessageRequest) (*runtimemodel.Conversation, *runtimemodel.Message, error) {
	if err := s.ensureMember(ctx, scope); err != nil {
		return nil, nil, err
	}
	workspaceID, err := s.resolveWorkspaceID(ctx, scope)
	if err != nil {
		return nil, nil, err
	}
	source := normalizeConversationSource(caller.Source)
	sourceWebAppID := normalizeCallerID(caller.SourceWebAppID)
	if source == runtimemodel.ConversationSourceWebApp && sourceWebAppID == nil {
		return nil, nil, fmt.Errorf("%w: source_web_app_id is required for webapp conversations", ErrInvalidInput)
	}
	conversationID := req.ConversationID
	if conversationID == uuid.Nil {
		conversationID = uuid.New()
	}
	conversation := &runtimemodel.Conversation{
		ID:               conversationID,
		OrganizationID:   scope.OrganizationID,
		WorkspaceID:      workspaceID,
		AccountID:        scope.AccountID,
		CallerType:       normalizeCallerType(caller.Type),
		CallerID:         normalizeCallerID(caller.ID),
		ConversationType: normalizeConversationType(caller.ConversationType),
		Title:            normalizeTitle(req.Title, defaultConversationTitle),
		Status:           runtimemodel.ConversationStatusNormal,
		Source:           source,
		SourceWebAppID:   sourceWebAppID,
	}
	if s.titleGen != nil {
		conversation.Metadata = conversationMetadataWithTitleStatus(conversation.Metadata, conversationTitleStatusPending)
	}
	messageReq := req.Message
	messageReq.ConversationID = conversationID
	message := completedMessageForConversation(conversation, messageReq)

	if err := s.repos.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		txRepos := repository.NewRepositories(tx)
		if err := txRepos.Conversation.Create(ctx, conversation); err != nil {
			return err
		}
		if err := txRepos.Message.Create(ctx, message); err != nil {
			return err
		}
		return txRepos.Conversation.UpdateAfterMessage(ctx, conversation.ID, message.ID)
	}); err != nil {
		return nil, nil, err
	}
	if s.titleGen != nil {
		s.enqueueConversationTitleGeneration(ctx, scope, caller, conversation, conversationTitleGenerationInput{
			Messages:      conversationTitleMessagesFromQuery(message.Query),
			FallbackTitle: conversation.Title,
		})
	}
	return conversation, message, nil
}

func (s *service) CreateConversationWithPendingMessage(ctx context.Context, scope Scope, caller Caller, req CreateConversationWithPendingMessageRequest) (*runtimemodel.Conversation, *runtimemodel.Message, error) {
	if err := s.ensureMember(ctx, scope); err != nil {
		return nil, nil, err
	}
	workspaceID, err := s.resolveWorkspaceID(ctx, scope)
	if err != nil {
		return nil, nil, err
	}
	source := normalizeConversationSource(caller.Source)
	sourceWebAppID := normalizeCallerID(caller.SourceWebAppID)
	if source == runtimemodel.ConversationSourceWebApp && sourceWebAppID == nil {
		return nil, nil, fmt.Errorf("%w: source_web_app_id is required for webapp conversations", ErrInvalidInput)
	}
	conversationID := req.ConversationID
	if conversationID == uuid.Nil {
		conversationID = uuid.New()
	}
	conversation := &runtimemodel.Conversation{
		ID:               conversationID,
		OrganizationID:   scope.OrganizationID,
		WorkspaceID:      workspaceID,
		AccountID:        scope.AccountID,
		CallerType:       normalizeCallerType(caller.Type),
		CallerID:         normalizeCallerID(caller.ID),
		ConversationType: normalizeConversationType(caller.ConversationType),
		Title:            normalizeTitle(req.Title, defaultConversationTitle),
		Status:           runtimemodel.ConversationStatusNormal,
		Source:           source,
		SourceWebAppID:   sourceWebAppID,
	}
	if s.titleGen != nil {
		conversation.Metadata = conversationMetadataWithTitleStatus(conversation.Metadata, conversationTitleStatusPending)
	}
	messageReq := req.Message
	messageReq.ConversationID = conversationID
	message := pendingMessageForConversation(conversation, messageReq)

	if err := s.repos.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		txRepos := repository.NewRepositories(tx)
		if err := txRepos.Conversation.Create(ctx, conversation); err != nil {
			return err
		}
		if err := txRepos.Message.Create(ctx, message); err != nil {
			return err
		}
		return startVisiblePendingMessage(ctx, tx, scope, conversation.ID, message.ID)
	}); err != nil {
		return nil, nil, err
	}
	if s.titleGen != nil {
		s.enqueueConversationTitleGeneration(ctx, scope, caller, conversation, conversationTitleGenerationInput{
			Messages:      conversationTitleMessagesFromQuery(message.Query),
			FallbackTitle: conversation.Title,
		})
	}
	return conversation, message, nil
}

func (s *service) CompleteMessage(ctx context.Context, scope Scope, req CompleteMessageRequest) (*runtimemodel.Message, error) {
	if req.ConversationID == uuid.Nil || req.MessageID == uuid.Nil {
		return nil, ErrConversationMissing
	}
	var completed *runtimemodel.Message
	err := s.repos.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		txRepos := repository.NewRepositories(tx)
		message, err := repository.GetMessageScopedForUpdate(ctx, tx, req.MessageID, scope.OrganizationID, scope.AccountID)
		if err != nil {
			return err
		}
		if message.ConversationID != req.ConversationID {
			return gorm.ErrRecordNotFound
		}
		if err := txRepos.Message.UpdateCompleted(ctx, req.MessageID, strings.TrimSpace(req.Answer), req.Metadata); err != nil {
			return err
		}
		if err := txRepos.Conversation.FinishActiveMessage(ctx, req.ConversationID, req.MessageID); err != nil {
			return err
		}
		completed, err = txRepos.Message.GetScoped(ctx, req.MessageID, scope.OrganizationID, scope.AccountID)
		return err
	})
	if err != nil {
		return nil, mapRepoError(err)
	}
	hydrateMessageGeneratedFileState(ctx, completed)
	return completed, nil
}

func (s *service) FailMessage(ctx context.Context, scope Scope, req FailMessageRequest) (*runtimemodel.Message, error) {
	if req.ConversationID == uuid.Nil || req.MessageID == uuid.Nil {
		return nil, ErrConversationMissing
	}
	messageText := strings.TrimSpace(req.ErrorMessage)
	if messageText == "" {
		messageText = "message failed"
	}
	var failed *runtimemodel.Message
	err := s.repos.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		txRepos := repository.NewRepositories(tx)
		message, err := repository.GetMessageScopedForUpdate(ctx, tx, req.MessageID, scope.OrganizationID, scope.AccountID)
		if err != nil {
			return err
		}
		if message.ConversationID != req.ConversationID {
			return gorm.ErrRecordNotFound
		}
		if req.Metadata != nil {
			if err := txRepos.Message.UpdateMetadata(ctx, req.MessageID, req.Metadata); err != nil {
				return err
			}
		}
		if err := txRepos.Message.UpdateError(ctx, req.MessageID, messageText); err != nil {
			return err
		}
		if err := txRepos.Conversation.FinishActiveMessage(ctx, req.ConversationID, req.MessageID); err != nil {
			return err
		}
		failed, err = txRepos.Message.GetScoped(ctx, req.MessageID, scope.OrganizationID, scope.AccountID)
		return err
	})
	if err != nil {
		return nil, mapRepoError(err)
	}
	hydrateMessagePublicError(failed)
	return failed, nil
}

func (s *service) StopRuntimeMessage(ctx context.Context, scope Scope, req StopRuntimeMessageRequest) (*runtimemodel.Message, error) {
	if req.ConversationID == uuid.Nil || req.MessageID == uuid.Nil {
		return nil, ErrConversationMissing
	}
	var stopped *runtimemodel.Message
	err := s.repos.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		txRepos := repository.NewRepositories(tx)
		message, err := repository.GetMessageScopedForUpdate(ctx, tx, req.MessageID, scope.OrganizationID, scope.AccountID)
		if err != nil {
			return err
		}
		if message.ConversationID != req.ConversationID {
			return gorm.ErrRecordNotFound
		}
		if !isStoppableMessageStatus(message.Status) {
			stopped = message
			return nil
		}
		if err := txRepos.Message.UpdateStoppedAnswer(ctx, req.MessageID, strings.TrimSpace(req.Answer), req.Metadata); err != nil {
			return err
		}
		if err := txRepos.Conversation.FinishActiveMessage(ctx, req.ConversationID, req.MessageID); err != nil {
			return err
		}
		stopped, err = txRepos.Message.GetScoped(ctx, req.MessageID, scope.OrganizationID, scope.AccountID)
		return err
	})
	if err != nil {
		return nil, mapRepoError(err)
	}
	hydrateMessageGeneratedFileState(ctx, stopped)
	return stopped, nil
}

func startVisiblePendingMessage(ctx context.Context, tx *gorm.DB, scope Scope, conversationID, messageID uuid.UUID) error {
	result := tx.WithContext(ctx).Model(&runtimemodel.Conversation{}).
		Where("id = ? AND organization_id = ? AND account_id = ? AND deleted_at IS NULL", conversationID, scope.OrganizationID, scope.AccountID).
		UpdateColumns(map[string]interface{}{
			"current_leaf_message_id": messageID,
			"runtime_status":          runtimemodel.ConversationRuntimeStatusStreaming,
			"active_message_id":       messageID,
			"dialogue_count":          gorm.Expr("CASE WHEN current_leaf_message_id = ? THEN dialogue_count ELSE dialogue_count + 1 END", messageID),
			"updated_at":              time.Now(),
		})
	if result.Error != nil {
		return fmt.Errorf("failed to mark pending aichat message visible: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func completedMessageForConversation(conversation *runtimemodel.Conversation, req CreateCompletedMessageRequest) *runtimemodel.Message {
	message := &runtimemodel.Message{
		ConversationID:      conversation.ID,
		ParentID:            conversation.CurrentLeafMessageID,
		Query:               strings.TrimSpace(req.Query),
		Answer:              strings.TrimSpace(req.Answer),
		Status:              runtimemodel.MessageStatusCompleted,
		ModelProvider:       optionalStringPtr(req.ModelProvider),
		ModelName:           strings.TrimSpace(req.ModelName),
		BillingReasonSource: optionalStringPtr(runtimemodel.MessageBillingReasonSourceAIChat),
		ModelParameters:     req.ModelParameters,
		Metadata:            req.Metadata,
	}
	if message.ModelParameters == nil {
		message.ModelParameters = map[string]interface{}{}
	}
	if message.Metadata == nil {
		message.Metadata = map[string]interface{}{}
	}
	return message
}

func pendingMessageForConversation(conversation *runtimemodel.Conversation, req CreatePendingMessageRequest) *runtimemodel.Message {
	message := &runtimemodel.Message{
		ConversationID:      conversation.ID,
		ParentID:            conversation.CurrentLeafMessageID,
		Query:               strings.TrimSpace(req.Query),
		Answer:              strings.TrimSpace(req.Answer),
		Status:              runtimemodel.MessageStatusStreaming,
		ModelProvider:       optionalStringPtr(req.ModelProvider),
		ModelName:           strings.TrimSpace(req.ModelName),
		BillingReasonSource: optionalStringPtr(runtimemodel.MessageBillingReasonSourceAIChat),
		ModelParameters:     req.ModelParameters,
		Metadata:            req.Metadata,
	}
	if message.ModelParameters == nil {
		message.ModelParameters = map[string]interface{}{}
	}
	if message.Metadata == nil {
		message.Metadata = map[string]interface{}{}
	}
	if _, ok := message.Metadata["started_at"]; !ok {
		message.Metadata["started_at"] = time.Now().Unix()
	}
	return message
}

func optionalStringPtr(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}
