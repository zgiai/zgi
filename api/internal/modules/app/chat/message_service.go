package chat

import (
	"context"
	"fmt"
	"github.com/zgiai/ginext/internal/dto"
	"github.com/zgiai/ginext/internal/modules/shared/interface"
	"github.com/zgiai/ginext/internal/modules/shared/model"

	"github.com/zgiai/ginext/pkg/pagination"
)

// MessageService defines the interface for message-related operations
type MessageService interface {
	// Paginated query
	PaginationByFirstID(ctx context.Context, appModel *dto.AppNode, user interface{}, conversationID, firstID string, limit int) (*pagination.InfiniteScrollPagination, error)

	// CRUD operations
	CreateMessage(ctx context.Context, req *dto.CreateMessageRequest) (*model.Message, error)
	GetMessageByID(ctx context.Context, messageID string) (*model.Message, error)
	UpdateMessage(ctx context.Context, messageID string, req *dto.UpdateMessageRequest) (*model.Message, error)
	DeleteMessage(ctx context.Context, messageID string) error

	// Business methods
	GetMessagesByConversationID(ctx context.Context, conversationID string, limit, offset int) ([]*model.Message, error)
	ConvertToResponse(message *model.Message) *dto.MessageDetailResponse

	// Message feedback operations
	CreateOrUpdateFeedback(ctx context.Context, appID, messageID string, req *dto.MessageFeedbackRequest) (*dto.MessageFeedbackResultResponse, error)

	// Message annotation operations
	CreateAnnotation(ctx context.Context, appID string, req *dto.MessageAnnotationRequest) (*dto.MessageAnnotationDetailResponse, error)
	GetAnnotationsByAppID(ctx context.Context, appID string) ([]*dto.MessageAnnotationDetailResponse, error)
	CountAnnotationsByAppID(ctx context.Context, appID string) (int64, error)

	// Message API
	GetMessageByIDAndAppID(ctx context.Context, appID, messageID string) (*dto.MessageDetailResponse, error)

	// Suggested questions
	GetSuggestedQuestions(ctx context.Context, appID, messageID string) ([]string, error)
}

type messageServiceImpl struct {
	messageRepo      MessageRepository
	conversationRepo interfaces.ConversationRepositoryInterface
}

func NewMessageService(messageRepo MessageRepository, conversationRepo interfaces.ConversationRepositoryInterface) MessageService {
	return &messageServiceImpl{
		messageRepo:      messageRepo,
		conversationRepo: conversationRepo,
	}
}

func (s *messageServiceImpl) PaginationByFirstID(
	ctx context.Context,
	appModel *dto.AppNode,
	user interface{},
	conversationID, firstID string,
	limit int,
) (*pagination.InfiniteScrollPagination, error) {
	if user == nil {
		return pagination.NewInfiniteScrollPagination([]model.Message{}, limit, false), nil
	}

	if conversationID == "" {
		return pagination.NewInfiniteScrollPagination([]model.Message{}, limit, false), nil
	}

	_, err := s.conversationRepo.GetByIDAndUser(ctx, conversationID, appModel.ID, user)
	if err != nil {
		return nil, fmt.Errorf("conversation not found: %w", err)
	}

	fetchLimit := limit + 1

	var messages []*model.Message

	if firstID != "" {
		messages, err = s.messageRepo.GetByConversationIDWithFirstID(ctx, conversationID, firstID, fetchLimit)
		if err != nil {
			return nil, fmt.Errorf("first message not found: %w", err)
		}
	} else {
		messages, err = s.messageRepo.GetLatestByConversationID(ctx, conversationID, fetchLimit)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch messages: %w", err)
		}
	}

	hasMore := len(messages) > limit
	if hasMore {
		messages = messages[:limit]
	}

	// Use shared model types
	var messageValues []model.Message
	for _, msg := range messages {
		messageValues = append(messageValues, *msg)
	}

	return pagination.NewInfiniteScrollPagination(messageValues, limit, hasMore), nil
}

func (s *messageServiceImpl) CreateMessage(ctx context.Context, req *dto.CreateMessageRequest) (*model.Message, error) {
	conversation, err := s.conversationRepo.GetByID(ctx, req.ConversationID)
	if err != nil {
		return nil, fmt.Errorf("conversation not found: %w", err)
	}

	if conversation == nil {
		return nil, fmt.Errorf("conversation not found")
	}

	message := &model.Message{
		AppID:           conversation.AppID,
		ConversationID:  req.ConversationID,
		Query:           req.Query,
		Inputs:          model.JSONMap(req.Inputs),
		ModelProvider:   req.ModelProvider,
		ModelID:         req.ModelID,
		FromSource:      "console",
		Status:          model.MessageStatusNormal,
		ParentMessageID: req.ParentMessageID,
	}

	err = s.messageRepo.Create(ctx, message)
	if err != nil {
		return nil, fmt.Errorf("failed to create message: %w", err)
	}

	return message, nil
}

// Add the missing ConvertToResponse method
func (s *messageServiceImpl) ConvertToResponse(message *model.Message) *dto.MessageDetailResponse {
	if message == nil {
		return nil
	}

	response := &dto.MessageDetailResponse{
		ID:                      message.ID,
		ConversationID:          message.ConversationID,
		Inputs:                  map[string]interface{}(message.Inputs),
		Query:                   message.Query,
		Message:                 []interface{}(message.Message),
		MessageTokens:           message.MessageTokens,
		Answer:                  message.Answer,
		AnswerTokens:            message.AnswerTokens,
		ProviderResponseLatency: message.ProviderResponseLatency,
		FromSource:              message.FromSource,
		FromEndUserID:           message.FromEndUserID,
		FromAccountID:           message.FromAccountID,
		CreatedAt:               message.CreatedAt.Unix(),
		Status:                  string(message.Status),
		Error:                   message.Error,
		ParentMessageID:         message.ParentMessageID,
		WorkflowRunID:           message.WorkflowRunID,
		Metadata:                map[string]interface{}(message.MessageMetadata),
	}

	// Convert feedbacks
	for _, feedback := range message.Feedbacks {
		response.Feedbacks = append(response.Feedbacks, dto.MessageFeedbackResponse{
			ID:      feedback.ID,
			Rating:  feedback.Rating,
			Content: feedback.Content,
		})
	}

	// Convert annotations
	if len(message.Annotations) > 0 {
		annotation := message.Annotations[0] // Take first annotation
		response.Annotation = &dto.MessageAnnotationResponse{
			ID:       annotation.ID,
			Question: annotation.Question,
			Content:  annotation.Content,
		}
	}

	// Convert message files
	for _, file := range message.MessageFiles {
		response.MessageFiles = append(response.MessageFiles, dto.MessageFileResponse{
			ID:             file.ID,
			Type:           file.Type,
			TransferMethod: file.TransferMethod,
			URL:            file.URL,
			BelongsTo:      file.BelongsTo,
		})
	}

	return response
}

// Add other missing interface methods
func (s *messageServiceImpl) GetMessageByID(ctx context.Context, messageID string) (*model.Message, error) {
	return s.messageRepo.GetByID(ctx, messageID)
}

func (s *messageServiceImpl) UpdateMessage(ctx context.Context, messageID string, req *dto.UpdateMessageRequest) (*model.Message, error) {
	// Implementation needed
	return nil, fmt.Errorf("not implemented")
}

func (s *messageServiceImpl) DeleteMessage(ctx context.Context, messageID string) error {
	return s.messageRepo.Delete(ctx, messageID)
}

func (s *messageServiceImpl) GetMessagesByConversationID(ctx context.Context, conversationID string, limit, offset int) ([]*model.Message, error) {
	return s.messageRepo.GetByConversationID(ctx, conversationID, limit, offset)
}

func (s *messageServiceImpl) CreateOrUpdateFeedback(ctx context.Context, appID, messageID string, req *dto.MessageFeedbackRequest) (*dto.MessageFeedbackResultResponse, error) {
	// Implementation needed
	return nil, fmt.Errorf("not implemented")
}

func (s *messageServiceImpl) CreateAnnotation(ctx context.Context, appID string, req *dto.MessageAnnotationRequest) (*dto.MessageAnnotationDetailResponse, error) {
	// Implementation needed
	return nil, fmt.Errorf("not implemented")
}

func (s *messageServiceImpl) GetAnnotationsByAppID(ctx context.Context, appID string) ([]*dto.MessageAnnotationDetailResponse, error) {
	// Implementation needed
	return nil, fmt.Errorf("not implemented")
}

func (s *messageServiceImpl) CountAnnotationsByAppID(ctx context.Context, appID string) (int64, error) {
	return s.messageRepo.CountAnnotationsByAppID(ctx, appID)
}

func (s *messageServiceImpl) GetMessageByIDAndAppID(ctx context.Context, appID, messageID string) (*dto.MessageDetailResponse, error) {
	message, err := s.messageRepo.GetByIDAndAppID(ctx, messageID, appID)
	if err != nil {
		return nil, err
	}
	return s.ConvertToResponse(message), nil
}

func (s *messageServiceImpl) GetSuggestedQuestions(ctx context.Context, appID, messageID string) ([]string, error) {
	// Implementation needed
	return []string{}, nil
}

func (s *messageServiceImpl) reverseMessages(messages []model.Message) {
	for i, j := 0, len(messages)-1; i < j; i, j = i+1, j-1 {
		messages[i], messages[j] = messages[j], messages[i]
	}
}
