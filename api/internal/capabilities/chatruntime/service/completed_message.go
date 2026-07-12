package service

import (
	"context"
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

func optionalStringPtr(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}
