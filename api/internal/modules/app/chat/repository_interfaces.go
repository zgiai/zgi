package chat

import (
	"context"
	"github.com/zgiai/ginext/internal/modules/shared/model"
)

type MessageRepository interface {
	Create(ctx context.Context, message *model.Message) error
	GetByID(ctx context.Context, messageID string) (*model.Message, error)
	Update(ctx context.Context, message *model.Message) error
	Delete(ctx context.Context, messageID string) error
	GetByConversationID(ctx context.Context, conversationID string, limit, offset int) ([]*model.Message, error)
	GetByConversationIDWithFirstID(ctx context.Context, conversationID, firstID string, limit int) ([]*model.Message, error)
	GetLatestByConversationID(ctx context.Context, conversationID string, limit int) ([]*model.Message, error)
	GetByIDAndAppID(ctx context.Context, messageID, appID string) (*model.Message, error)

	CreateFeedback(ctx context.Context, feedback *model.MessageFeedback) error
	GetFeedbackByMessageID(ctx context.Context, messageID string) (*model.MessageFeedback, error)
	UpdateFeedback(ctx context.Context, feedback *model.MessageFeedback) error

	CreateAnnotation(ctx context.Context, annotation *model.MessageAnnotation) error
	GetAnnotationsByAppID(ctx context.Context, appID string) ([]*model.MessageAnnotation, error)
	CountAnnotationsByAppID(ctx context.Context, appID string) (int64, error)
}

type ConversationRepository interface {
	GetByID(ctx context.Context, conversationID string) (*model.Conversation, error)
	GetByIDAndUser(ctx context.Context, conversationID, appID string, user interface{}) (*model.Conversation, error)
}
