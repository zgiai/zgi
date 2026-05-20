package interfaces

import (
	"context"
	"github.com/zgiai/zgi/api/internal/modules/shared/model"
)

type ConversationRepositoryInterface interface {
	GetByID(ctx context.Context, conversationID string) (*model.Conversation, error)
	GetByIDAndUser(ctx context.Context, conversationID, appID string, user interface{}) (*model.Conversation, error)
}

type MessageRepositoryInterface interface {
	GetByConversationID(ctx context.Context, conversationID string, limit, offset int) ([]*model.Message, error)
}
