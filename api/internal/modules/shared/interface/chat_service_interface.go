package interfaces

import "context"

// ChatService defines the interface for chat-related operations
type ChatService interface {
	SendMessage(ctx context.Context, appID, accountID string, req interface{}) (interface{}, error)
	GetMessages(ctx context.Context, appID, accountID string, req interface{}) (interface{}, error)
	GetMessage(ctx context.Context, appID, messageID, accountID string) (interface{}, error)
	UpdateMessage(ctx context.Context, appID, messageID, accountID string, req interface{}) (interface{}, error)
	DeleteMessage(ctx context.Context, appID, messageID, accountID string) error
	AddMessageFeedback(ctx context.Context, appID, messageID, accountID string, req interface{}) (interface{}, error)
}
