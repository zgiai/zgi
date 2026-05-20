package app

import (
	"context"
	"github.com/zgiai/ginext/internal/modules/app/chat"
	"github.com/zgiai/ginext/internal/modules/app/conversation"
	"github.com/zgiai/ginext/internal/modules/shared/model"
	"gorm.io/gorm"
)

type Services struct {
	MessageService chat.MessageService
	db             *gorm.DB
}

type conversationRepoAdapter struct {
	repo conversation.ConversationRepository
}

func (a *conversationRepoAdapter) GetByID(ctx context.Context, conversationID string) (*model.Conversation, error) {
	chatConv, err := a.repo.GetByID(ctx, conversationID)
	if err != nil {
		return nil, err
	}

	return &model.Conversation{
		ID:                      chatConv.ID,
		AppID:                   chatConv.AppID,
		ModelConfig:             chatConv.ModelConfig,
		OverrideModelConfigs:    chatConv.OverrideModelConfigs,
		Mode:                    chatConv.Mode,
		Name:                    chatConv.Name,
		Summary:                 chatConv.Summary,
		Inputs:                  chatConv.Inputs,
		Introduction:            chatConv.Introduction,
		SystemInstruction:       chatConv.SystemInstruction,
		SystemInstructionTokens: chatConv.SystemInstructionTokens,
		Status:                  chatConv.Status,
		FromSource:              chatConv.FromSource,
		FromEndUserID:           chatConv.FromEndUserID,
		FromAccountID:           chatConv.FromAccountID,
		ReadAt:                  chatConv.ReadAt,
		ReadAccountID:           chatConv.ReadAccountID,
		CreatedAt:               chatConv.CreatedAt,
		UpdatedAt:               chatConv.UpdatedAt,
		IsDeleted:               chatConv.IsDeleted,
	}, nil
}

func (a *conversationRepoAdapter) GetByIDAndUser(ctx context.Context, conversationID, appID string, user interface{}) (*model.Conversation, error) {
	chatConv, err := a.repo.GetByIDAndUser(ctx, conversationID, appID, user)
	if err != nil {
		return nil, err
	}

	return &model.Conversation{
		ID:                      chatConv.ID,
		AppID:                   chatConv.AppID,
		ModelConfig:             chatConv.ModelConfig,
		OverrideModelConfigs:    chatConv.OverrideModelConfigs,
		Mode:                    chatConv.Mode,
		Name:                    chatConv.Name,
		Summary:                 chatConv.Summary,
		Inputs:                  chatConv.Inputs,
		Introduction:            chatConv.Introduction,
		SystemInstruction:       chatConv.SystemInstruction,
		SystemInstructionTokens: chatConv.SystemInstructionTokens,
		Status:                  chatConv.Status,
		FromSource:              chatConv.FromSource,
		FromEndUserID:           chatConv.FromEndUserID,
		FromAccountID:           chatConv.FromAccountID,
		ReadAt:                  chatConv.ReadAt,
		ReadAccountID:           chatConv.ReadAccountID,
		CreatedAt:               chatConv.CreatedAt,
		UpdatedAt:               chatConv.UpdatedAt,
		IsDeleted:               chatConv.IsDeleted,
	}, nil
}

func (s *Services) initMessageService() {
	conversationRepo := conversation.NewConversationRepository(s.db)

	adapter := &conversationRepoAdapter{repo: conversationRepo}

	messageRepo := chat.NewMessageRepository(s.db)

	s.MessageService = chat.NewMessageService(messageRepo, adapter)
}

func NewServices(db *gorm.DB) *Services {
	services := &Services{db: db}
	services.initMessageService()
	return services
}
