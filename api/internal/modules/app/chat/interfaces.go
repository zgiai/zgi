package chat

import (
	"context"
)

type ConversationRepositoryInterface interface {
	GetByID(ctx context.Context, conversationID string) (*Conversation, error)
	GetByIDAndUser(ctx context.Context, conversationID, appID string, user interface{}) (*Conversation, error)
}
