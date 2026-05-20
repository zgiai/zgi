package interfaces

import (
	"context"
	"github.com/zgiai/ginext/internal/dto"
	"github.com/zgiai/ginext/internal/modules/shared/model"
	"github.com/zgiai/ginext/pkg/pagination"
)

type MessageService interface {
	PaginationByFirstID(ctx context.Context, appModel *dto.AppNode, user interface{}, conversationID, firstID string, limit int) (*pagination.InfiniteScrollPagination, error)

	CreateMessage(ctx context.Context, req *dto.CreateMessageRequest) (*model.Message, error)
	GetMessageByID(ctx context.Context, messageID string) (*model.Message, error)
	UpdateMessage(ctx context.Context, messageID string, req *dto.UpdateMessageRequest) (*model.Message, error)
	DeleteMessage(ctx context.Context, messageID string) error

	GetMessagesByConversationID(ctx context.Context, conversationID string, limit, offset int) ([]*model.Message, error)
	ConvertToResponse(message *model.Message) *dto.MessageDetailResponse

	CreateOrUpdateFeedback(ctx context.Context, appID, messageID string, req *dto.MessageFeedbackRequest) (*dto.MessageFeedbackResultResponse, error)

	CreateAnnotation(ctx context.Context, appID string, req *dto.MessageAnnotationRequest) (*dto.MessageAnnotationDetailResponse, error)
	GetAnnotationsByAppID(ctx context.Context, appID string) ([]*dto.MessageAnnotationDetailResponse, error)
	CountAnnotationsByAppID(ctx context.Context, appID string) (int64, error)

	GetMessageByIDAndAppID(ctx context.Context, appID, messageID string) (*dto.MessageDetailResponse, error)

	GetSuggestedQuestions(ctx context.Context, appID, messageID string) ([]string, error)
}
