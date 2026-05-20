package chat

import (
	"context"
	"github.com/zgiai/ginext/internal/modules/shared/model"
	"gorm.io/gorm"
)

// Remove the duplicate MessageRepository interface - it's already defined in repository_interfaces.go

type messageRepository struct {
	db *gorm.DB
}

func NewMessageRepository(db *gorm.DB) MessageRepository {
	return &messageRepository{
		db: db,
	}
}

func (r *messageRepository) Create(ctx context.Context, message *model.Message) error {
	return r.db.WithContext(ctx).Create(message).Error
}

func (r *messageRepository) GetByID(ctx context.Context, id string) (*model.Message, error) {
	var message model.Message
	err := r.db.WithContext(ctx).
		Where("id = ?", id).
		Preload("Feedbacks").
		Preload("Annotations").
		Preload("MessageFiles").
		Preload("Conversation").
		First(&message).Error

	if err != nil {
		return nil, err
	}
	return &message, nil
}

func (r *messageRepository) Update(ctx context.Context, message *model.Message) error {
	return r.db.WithContext(ctx).Save(message).Error
}

func (r *messageRepository) Delete(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Where("id = ?", id).Delete(&model.Message{}).Error
}

func (r *messageRepository) GetByConversationID(ctx context.Context, conversationID string, limit, offset int) ([]*model.Message, error) {
	var messages []*model.Message
	err := r.db.WithContext(ctx).
		Where("conversation_id = ?", conversationID).
		Order("created_at ASC").
		Limit(limit).
		Offset(offset).
		Preload("Feedbacks").
		Preload("Annotations").
		Preload("MessageFiles").
		Find(&messages).Error

	return messages, err
}

func (r *messageRepository) GetByConversationIDWithFirstID(ctx context.Context, conversationID, firstID string, limit int) ([]*model.Message, error) {
	firstMessage, err := r.GetFirstMessage(ctx, conversationID, firstID)
	if err != nil {
		return nil, err
	}

	var messages []*model.Message
	err = r.db.WithContext(ctx).
		Where("conversation_id = ? AND created_at < ? AND id != ?",
			conversationID, firstMessage.CreatedAt, firstID).
		Order("created_at DESC").
		Limit(limit).
		Preload("Feedbacks").
		Preload("Annotations").
		Preload("MessageFiles").
		Find(&messages).Error

	return messages, err
}

func (r *messageRepository) GetLatestByConversationID(ctx context.Context, conversationID string, limit int) ([]*model.Message, error) {
	var messages []*model.Message
	err := r.db.WithContext(ctx).
		Where("conversation_id = ?", conversationID).
		Order("created_at DESC").
		Limit(limit).
		Preload("Feedbacks").
		Preload("Annotations").
		Preload("MessageFiles").
		Find(&messages).Error

	return messages, err
}

func (r *messageRepository) GetFirstMessage(ctx context.Context, conversationID, firstID string) (*model.Message, error) {
	var message model.Message
	err := r.db.WithContext(ctx).
		Where("conversation_id = ? AND id = ?", conversationID, firstID).
		First(&message).Error

	if err != nil {
		return nil, err
	}
	return &message, nil
}

func (r *messageRepository) CountByConversationID(ctx context.Context, conversationID string) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&model.Message{}).
		Where("conversation_id = ?", conversationID).
		Count(&count).Error

	return count, err
}

func (r *messageRepository) GetByIDAndAppID(ctx context.Context, messageID, appID string) (*model.Message, error) {
	var message model.Message
	err := r.db.WithContext(ctx).
		Where("id = ? AND app_id = ?", messageID, appID).
		Preload("Feedbacks").
		Preload("Annotations").
		Preload("MessageFiles").
		Preload("Conversation").
		First(&message).Error

	if err != nil {
		return nil, err
	}
	return &message, nil
}

// Message Feedback operations
func (r *messageRepository) CreateFeedback(ctx context.Context, feedback *model.MessageFeedback) error {
	return r.db.WithContext(ctx).Create(feedback).Error
}

func (r *messageRepository) UpdateFeedback(ctx context.Context, feedback *model.MessageFeedback) error {
	return r.db.WithContext(ctx).Save(feedback).Error
}

func (r *messageRepository) DeleteFeedback(ctx context.Context, messageID, appID string) error {
	return r.db.WithContext(ctx).
		Where("message_id = ? AND app_id = ?", messageID, appID).
		Delete(&model.MessageFeedback{}).Error
}

func (r *messageRepository) GetFeedbackByMessageID(ctx context.Context, messageID string) (*model.MessageFeedback, error) {
	var feedback model.MessageFeedback
	err := r.db.WithContext(ctx).
		Where("message_id = ?", messageID).
		First(&feedback).Error

	if err != nil {
		return nil, err
	}
	return &feedback, nil
}

// Message Annotation operations
func (r *messageRepository) CreateAnnotation(ctx context.Context, annotation *model.MessageAnnotation) error {
	return r.db.WithContext(ctx).Create(annotation).Error
}

func (r *messageRepository) GetAnnotationsByAppID(ctx context.Context, appID string) ([]*model.MessageAnnotation, error) {
	var annotations []*model.MessageAnnotation
	err := r.db.WithContext(ctx).
		Where("app_id = ?", appID).
		Find(&annotations).Error

	return annotations, err
}

func (r *messageRepository) CountAnnotationsByAppID(ctx context.Context, appID string) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&model.MessageAnnotation{}).
		Where("app_id = ?", appID).
		Count(&count).Error

	return count, err
}
