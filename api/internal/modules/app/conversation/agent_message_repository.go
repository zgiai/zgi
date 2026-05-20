package conversation

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/ginext/pkg/logger"
	"gorm.io/gorm"
)

// AgentMessageRepository defines the interface for agent message data access
type AgentMessageRepository interface {
	Create(ctx context.Context, message *AgentMessage) error
	GetByID(ctx context.Context, id uuid.UUID) (*AgentMessage, error)
	GetByConversationID(ctx context.Context, conversationID uuid.UUID, limit, offset int) ([]*AgentMessage, int64, error)
	Update(ctx context.Context, message *AgentMessage) error
	Delete(ctx context.Context, id uuid.UUID, deletedBy uuid.UUID) error
	GetByAgentAndUser(ctx context.Context, agentID uuid.UUID, fromSource string, userID uuid.UUID, limit, offset int) ([]*AgentMessage, int64, error)
	GetLatestByConversation(ctx context.Context, conversationID uuid.UUID) (*AgentMessage, error)
	GetByWorkflowRunID(ctx context.Context, workflowRunID uuid.UUID) ([]*AgentMessage, error)
	GetFirstByWorkflowRunIDs(ctx context.Context, workflowRunIDs []string) (map[string]*AgentMessage, error)
	UpdateStatus(ctx context.Context, id uuid.UUID, status string, error *string) error
	MigrateMessagesByAccountID(ctx context.Context, tx *gorm.DB, virtualAccountID, authenticatedAccountID uuid.UUID) (int64, error)
}

// agentMessageRepository implements AgentMessageRepository
type agentMessageRepository struct {
	db *gorm.DB
}

// NewAgentMessageRepository creates a new AgentMessageRepository
func NewAgentMessageRepository(db *gorm.DB) AgentMessageRepository {
	return &agentMessageRepository{
		db: db,
	}
}

// Create creates a new agent message
func (r *agentMessageRepository) Create(ctx context.Context, message *AgentMessage) error {
	if message.ID == uuid.Nil {
		message.ID = uuid.New()
	}
	message.CreatedAt = time.Now()
	message.UpdatedAt = time.Now()

	if err := r.db.WithContext(ctx).Create(message).Error; err != nil {
		return fmt.Errorf("failed to create agent message: %w", err)
	}
	return nil
}

// GetByID retrieves an agent message by ID
func (r *agentMessageRepository) GetByID(ctx context.Context, id uuid.UUID) (*AgentMessage, error) {
	var message AgentMessage
	err := r.db.WithContext(ctx).
		Where("id = ? AND deleted_at IS NULL", id).
		First(&message).Error

	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("agent message not found")
		}
		return nil, fmt.Errorf("failed to get agent message: %w", err)
	}

	return &message, nil
}

// GetByConversationID retrieves messages by conversation ID with pagination
func (r *agentMessageRepository) GetByConversationID(ctx context.Context, conversationID uuid.UUID, limit, offset int) ([]*AgentMessage, int64, error) {
	var messages []*AgentMessage
	var total int64

	query := r.db.WithContext(ctx).
		Model(&AgentMessage{}).
		Where("conversation_id = ? AND deleted_at IS NULL", conversationID)

	// Get total count
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to count agent messages: %w", err)
	}

	// Get messages with pagination
	err := query.
		Order("created_at ASC").
		Limit(limit).
		Offset(offset).
		Find(&messages).Error

	if err != nil {
		return nil, 0, fmt.Errorf("failed to get agent messages: %w", err)
	}

	return messages, total, nil
}

// Update updates an agent message
func (r *agentMessageRepository) Update(ctx context.Context, message *AgentMessage) error {
	message.UpdatedAt = time.Now()

	err := r.db.WithContext(ctx).
		Where("id = ? AND deleted_at IS NULL", message.ID).
		Updates(message).Error

	if err != nil {
		return fmt.Errorf("failed to update agent message: %w", err)
	}
	return nil
}

// Delete soft deletes an agent message
func (r *agentMessageRepository) Delete(ctx context.Context, id uuid.UUID, deletedBy uuid.UUID) error {
	now := time.Now()
	err := r.db.WithContext(ctx).
		Model(&AgentMessage{}).
		Where("id = ? AND deleted_at IS NULL", id).
		Updates(map[string]interface{}{
			"deleted_by": deletedBy,
			"deleted_at": now,
			"updated_at": now,
		}).Error

	if err != nil {
		return fmt.Errorf("failed to delete agent message: %w", err)
	}
	return nil
}

// GetByAgentAndUser retrieves messages by agent and user with pagination
func (r *agentMessageRepository) GetByAgentAndUser(ctx context.Context, agentID uuid.UUID, fromSource string, userID uuid.UUID, limit, offset int) ([]*AgentMessage, int64, error) {
	var messages []*AgentMessage
	var total int64

	query := r.db.WithContext(ctx).
		Model(&AgentMessage{}).
		Where("agent_id = ? AND from_source = ? AND deleted_at IS NULL", agentID, fromSource)

	// Add user condition based on source type
	if fromSource == "end_user" {
		query = query.Where("from_end_user_id = ?", userID)
	} else if fromSource == "account" {
		query = query.Where("from_account_id = ?", userID)
	}

	// Get total count
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to count agent messages: %w", err)
	}

	// Get messages with pagination
	err := query.
		Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&messages).Error

	if err != nil {
		return nil, 0, fmt.Errorf("failed to get agent messages: %w", err)
	}

	return messages, total, nil
}

// GetLatestByConversation retrieves the latest message in a conversation
func (r *agentMessageRepository) GetLatestByConversation(ctx context.Context, conversationID uuid.UUID) (*AgentMessage, error) {
	var message AgentMessage
	err := r.db.WithContext(ctx).
		Where("conversation_id = ? AND deleted_at IS NULL", conversationID).
		Order("created_at DESC").
		First(&message).Error

	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("no messages found in conversation")
		}
		return nil, fmt.Errorf("failed to get latest message: %w", err)
	}

	return &message, nil
}

// GetByWorkflowRunID retrieves messages by workflow run ID
func (r *agentMessageRepository) GetByWorkflowRunID(ctx context.Context, workflowRunID uuid.UUID) ([]*AgentMessage, error) {
	var messages []*AgentMessage
	err := r.db.WithContext(ctx).
		Where("workflow_run_id = ? AND deleted_at IS NULL", workflowRunID).
		Order("created_at ASC").
		Find(&messages).Error

	if err != nil {
		return nil, fmt.Errorf("failed to get messages by workflow run ID: %w", err)
	}

	return messages, nil
}

// GetFirstByWorkflowRunIDs retrieves the earliest persisted message for each workflow run ID.
func (r *agentMessageRepository) GetFirstByWorkflowRunIDs(ctx context.Context, workflowRunIDs []string) (map[string]*AgentMessage, error) {
	result := make(map[string]*AgentMessage)
	if len(workflowRunIDs) == 0 {
		return result, nil
	}

	var messages []*AgentMessage
	err := r.db.WithContext(ctx).
		Where("workflow_run_id IN ? AND deleted_at IS NULL", workflowRunIDs).
		Order("created_at ASC").
		Find(&messages).Error
	if err != nil {
		return nil, fmt.Errorf("failed to get messages by workflow run IDs: %w", err)
	}

	for _, message := range messages {
		if message == nil || message.WorkflowRunID == nil {
			continue
		}

		runID := message.WorkflowRunID.String()
		if _, exists := result[runID]; exists {
			continue
		}
		result[runID] = message
	}

	return result, nil
}

// UpdateStatus updates the status and error of a message
func (r *agentMessageRepository) UpdateStatus(ctx context.Context, id uuid.UUID, status string, error *string) error {
	updates := map[string]interface{}{
		"status":     status,
		"updated_at": time.Now(),
	}

	if error != nil {
		updates["error"] = *error
	}

	err := r.db.WithContext(ctx).
		Model(&AgentMessage{}).
		Where("id = ? AND deleted_at IS NULL", id).
		Updates(updates).Error

	if err != nil {
		return fmt.Errorf("failed to update message status: %w", err)
	}
	return nil
}

// MigrateMessagesByAccountID migrates messages from virtual user to authenticated user
func (r *agentMessageRepository) MigrateMessagesByAccountID(ctx context.Context, tx *gorm.DB, virtualAccountID, authenticatedAccountID uuid.UUID) (int64, error) {
	logger.Debug("Migrating messages", map[string]interface{}{
		"virtual_account_id":       virtualAccountID.String(),
		"authenticated_account_id": authenticatedAccountID.String(),
	})

	result := tx.WithContext(ctx).
		Model(&AgentMessage{}).
		Where("created_by = ? AND deleted_at IS NULL", virtualAccountID).
		Updates(map[string]interface{}{
			"created_by": authenticatedAccountID,
			"updated_at": time.Now(),
		})

	if result.Error != nil {
		logger.Error("Failed to migrate messages", result.Error)
		return 0, fmt.Errorf("failed to migrate messages: %w", result.Error)
	}

	logger.Info("Messages migrated", map[string]interface{}{
		"rows_affected":            result.RowsAffected,
		"virtual_account_id":       virtualAccountID.String(),
		"authenticated_account_id": authenticatedAccountID.String(),
	})

	return result.RowsAffected, nil
}
