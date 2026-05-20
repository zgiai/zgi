package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	aichatmodel "github.com/zgiai/zgi/api/internal/modules/aichat/model"
	oldconversation "github.com/zgiai/zgi/api/internal/modules/app/conversation"
	"gorm.io/gorm"
)

func (s *service) MigrateWebAppConversation(ctx context.Context, scope Scope, sourceConversationID uuid.UUID) (*aichatmodel.Conversation, error) {
	if err := s.ensureMember(ctx, scope); err != nil {
		return nil, err
	}
	if sourceConversationID == uuid.Nil {
		return nil, fmt.Errorf("%w: source conversation is required", ErrInvalidInput)
	}
	existing, err := s.existingMigratedConversation(ctx, scope, sourceConversationID)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return existing, nil
	}

	source, err := s.loadLegacyConversation(ctx, scope, sourceConversationID)
	if err != nil {
		return nil, err
	}
	workspaceID, err := s.resolveWorkspaceID(ctx, scope)
	if err != nil {
		return nil, err
	}
	return s.runWebAppMigration(ctx, scope, source, workspaceID)
}

func (s *service) existingMigratedConversation(ctx context.Context, scope Scope, sourceConversationID uuid.UUID) (*aichatmodel.Conversation, error) {
	existing, err := s.repos.Conversation.GetBySourceConversation(ctx, sourceConversationID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) || strings.Contains(err.Error(), "record not found") {
			return nil, nil
		}
		return nil, err
	}
	if existing.OrganizationID != scope.OrganizationID || existing.AccountID != scope.AccountID {
		return nil, ErrPermissionDenied
	}
	return existing, nil
}

func (s *service) loadLegacyConversation(ctx context.Context, scope Scope, sourceConversationID uuid.UUID) (*oldconversation.AgentConversation, error) {
	var source oldconversation.AgentConversation
	err := s.repos.DB.WithContext(ctx).
		Where("id = ? AND deleted_at IS NULL", sourceConversationID).
		Take(&source).Error
	if err != nil {
		return nil, mapRepoError(err)
	}
	if !legacyConversationBelongsToAccount(&source, scope.AccountID) {
		return nil, ErrPermissionDenied
	}
	return &source, nil
}

func (s *service) runWebAppMigration(ctx context.Context, scope Scope, source *oldconversation.AgentConversation, workspaceID *uuid.UUID) (*aichatmodel.Conversation, error) {
	var migrated *aichatmodel.Conversation
	err := s.repos.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		conversation := newMigratedConversation(scope, source, workspaceID)
		if err := tx.Create(conversation).Error; err != nil {
			return fmt.Errorf("failed to create migrated aichat conversation: %w", err)
		}
		sourceMessages, err := loadLegacyMessages(tx, source.ID)
		if err != nil {
			return err
		}
		if err := createMigratedMessages(tx, conversation, sourceMessages); err != nil {
			return err
		}
		if err := updateMigratedConversationLeaf(tx, conversation); err != nil {
			return err
		}
		migrated = conversation
		return nil
	})
	if err != nil {
		return nil, err
	}
	return migrated, nil
}

func newMigratedConversation(scope Scope, source *oldconversation.AgentConversation, workspaceID *uuid.UUID) *aichatmodel.Conversation {
	sourceID := source.ID
	conversation := &aichatmodel.Conversation{
		ID:                   uuid.New(),
		OrganizationID:       scope.OrganizationID,
		WorkspaceID:          workspaceID,
		AccountID:            scope.AccountID,
		Title:                normalizeTitle(source.Name, defaultConversationTitle),
		Status:               aichatmodel.ConversationStatusNormal,
		RuntimeStatus:        aichatmodel.ConversationRuntimeStatusIdle,
		DialogueCount:        source.DialogueCount,
		Source:               aichatmodel.ConversationSourceWebApp,
		SourceConversationID: &sourceID,
		CreatedAt:            source.CreatedAt,
		UpdatedAt:            source.UpdatedAt,
	}
	setMigratedWebAppID(conversation, source.WebAppID)
	ensureMigratedTimestamps(&conversation.CreatedAt, &conversation.UpdatedAt)
	return conversation
}

func setMigratedWebAppID(conversation *aichatmodel.Conversation, webAppIDRaw *string) {
	if webAppIDRaw == nil || *webAppIDRaw == "" {
		return
	}
	webAppID, err := uuid.Parse(*webAppIDRaw)
	if err == nil {
		conversation.SourceWebAppID = &webAppID
	}
}

func ensureMigratedTimestamps(createdAt, updatedAt *time.Time) {
	if createdAt.IsZero() {
		*createdAt = time.Now()
	}
	if updatedAt.IsZero() {
		*updatedAt = *createdAt
	}
}

func loadLegacyMessages(tx *gorm.DB, sourceConversationID uuid.UUID) ([]oldconversation.AgentMessage, error) {
	var sourceMessages []oldconversation.AgentMessage
	err := tx.Where("conversation_id = ? AND deleted_at IS NULL", sourceConversationID).
		Order("created_at ASC").
		Find(&sourceMessages).Error
	if err != nil {
		return nil, fmt.Errorf("failed to list webapp messages for migration: %w", err)
	}
	return sourceMessages, nil
}

func createMigratedMessages(tx *gorm.DB, conversation *aichatmodel.Conversation, sourceMessages []oldconversation.AgentMessage) error {
	var parentID *uuid.UUID
	for _, oldMessage := range sourceMessages {
		message := newMigratedMessage(conversation.ID, parentID, oldMessage)
		if err := tx.Create(message).Error; err != nil {
			return fmt.Errorf("failed to create migrated aichat message: %w", err)
		}
		nextParentID := message.ID
		parentID = &nextParentID
		conversation.CurrentLeafMessageID = &nextParentID
	}
	return nil
}

func newMigratedMessage(conversationID uuid.UUID, parentID *uuid.UUID, oldMessage oldconversation.AgentMessage) *aichatmodel.Message {
	oldID := oldMessage.ID
	message := &aichatmodel.Message{
		ID:              uuid.New(),
		ConversationID:  conversationID,
		ParentID:        parentID,
		Query:           oldMessage.Query,
		Answer:          oldMessage.Answer,
		Status:          migratedMessageStatus(oldMessage.Status, oldMessage.Error),
		Error:           oldMessage.Error,
		ModelProvider:   oldMessage.ModelProvider,
		ModelName:       migratedModelName(oldMessage.ModelVersionID),
		ModelParameters: map[string]interface{}{},
		Metadata: map[string]interface{}{
			"migrated_from":         "agents_messages",
			"system_prompt_version": systemPromptVersion,
		},
		SourceMessageID: &oldID,
		CreatedAt:       oldMessage.CreatedAt,
		UpdatedAt:       oldMessage.UpdatedAt,
	}
	ensureMigratedTimestamps(&message.CreatedAt, &message.UpdatedAt)
	return message
}

func updateMigratedConversationLeaf(tx *gorm.DB, conversation *aichatmodel.Conversation) error {
	if conversation.CurrentLeafMessageID == nil {
		return nil
	}
	err := tx.Model(&aichatmodel.Conversation{}).
		Where("id = ?", conversation.ID).
		Updates(map[string]interface{}{
			"current_leaf_message_id": conversation.CurrentLeafMessageID,
			"updated_at":              time.Now(),
		}).Error
	if err != nil {
		return fmt.Errorf("failed to update migrated aichat conversation leaf: %w", err)
	}
	return nil
}

func legacyConversationBelongsToAccount(conversation *oldconversation.AgentConversation, accountID uuid.UUID) bool {
	if conversation == nil || accountID == uuid.Nil {
		return false
	}
	if conversation.FromAccountID != nil && *conversation.FromAccountID == accountID {
		return true
	}
	if conversation.CreatedBy != nil && *conversation.CreatedBy == accountID {
		return true
	}
	return false
}

func migratedMessageStatus(status string, messageError *string) string {
	if messageError != nil && *messageError != "" {
		return aichatmodel.MessageStatusError
	}
	switch status {
	case oldconversation.AgentMessageStatusError:
		return aichatmodel.MessageStatusError
	case oldconversation.AgentMessageStatusStopped:
		return aichatmodel.MessageStatusStopped
	case oldconversation.AgentMessageStatusRunning, oldconversation.AgentMessageStatusPendingApproval:
		return aichatmodel.MessageStatusCompleted
	default:
		return aichatmodel.MessageStatusCompleted
	}
}

func migratedModelName(modelVersionID *string) string {
	if modelVersionID == nil || strings.TrimSpace(*modelVersionID) == "" {
		return "unknown"
	}
	return strings.TrimSpace(*modelVersionID)
}
