package service

import (
	"context"
	"fmt"
	"strings"

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
	return conversation, message, nil
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

func optionalStringPtr(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}
