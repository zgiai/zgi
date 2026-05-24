package conversation

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/pkg/logger"
	"gorm.io/gorm"
)

// AgentConversationRepository defines the interface for agent conversation data access
type AgentConversationRepository interface {
	Create(ctx context.Context, conversation *AgentConversation) error
	GetByID(ctx context.Context, id uuid.UUID) (*AgentConversation, error)
	GetByIDAndAgent(ctx context.Context, id, agentID uuid.UUID) (*AgentConversation, error)
	Update(ctx context.Context, conversation *AgentConversation) error
	Delete(ctx context.Context, id uuid.UUID, deletedBy uuid.UUID) error
	GetByAgentAndUser(ctx context.Context, agentID uuid.UUID, fromSource string, userID uuid.UUID, limit, offset int) ([]*AgentConversation, int64, error)
	GetByAgent(ctx context.Context, agentID uuid.UUID, limit, offset int) ([]*AgentConversation, int64, error)
	GetByAgentWithFilters(ctx context.Context, agentID, userID uuid.UUID, versionUUID string, limit, offset int) ([]*AgentConversation, int64, error)
	GetHistoryByAgent(ctx context.Context, filter AgentConversationHistoryFilter) ([]*AgentConversation, int64, error)
	IncrementDialogueCount(ctx context.Context, id uuid.UUID) error
	GetWithMessages(ctx context.Context, id uuid.UUID, messageLimit int) (*AgentConversation, error)
	UpdateWebAppID(ctx context.Context, conversationID uuid.UUID, webAppID string) error
	UpdateNameIfCurrent(ctx context.Context, conversationID uuid.UUID, currentName, nextName string) (bool, error)
	MigrateConversationsByAccountID(ctx context.Context, tx *gorm.DB, virtualAccountID, authenticatedAccountID uuid.UUID) (int64, error)
}

type AgentConversationHistoryFilter struct {
	AgentID     uuid.UUID
	Keyword     string
	SortBy      string
	Start       *time.Time
	End         *time.Time
	InvokeFroms []string
	Limit       int
	Offset      int
}

// agentConversationRepository implements AgentConversationRepository
type agentConversationRepository struct {
	db *gorm.DB
}

// NewAgentConversationRepository creates a new AgentConversationRepository
func NewAgentConversationRepository(db *gorm.DB) AgentConversationRepository {
	return &agentConversationRepository{
		db: db,
	}
}

// Create creates a new agent conversation
func (r *agentConversationRepository) Create(ctx context.Context, conversation *AgentConversation) error {
	if conversation.ID == uuid.Nil {
		conversation.ID = uuid.New()
	}
	conversation.CreatedAt = time.Now()
	conversation.UpdatedAt = time.Now()

	if err := r.db.WithContext(ctx).Create(conversation).Error; err != nil {
		return fmt.Errorf("failed to create agent conversation: %w", err)
	}
	return nil
}

// GetByID retrieves an agent conversation by ID
func (r *agentConversationRepository) GetByID(ctx context.Context, id uuid.UUID) (*AgentConversation, error) {
	var conversation AgentConversation
	err := r.db.WithContext(ctx).
		Where("id = ? AND deleted_at IS NULL", id).
		First(&conversation).Error

	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("agent conversation not found")
		}
		return nil, fmt.Errorf("failed to get agent conversation: %w", err)
	}

	return &conversation, nil
}

// GetByIDAndAgent retrieves an agent conversation by ID and agent ID
func (r *agentConversationRepository) GetByIDAndAgent(ctx context.Context, id, agentID uuid.UUID) (*AgentConversation, error) {
	var conversation AgentConversation
	err := r.db.WithContext(ctx).
		Where("id = ? AND agent_id = ? AND deleted_at IS NULL", id, agentID).
		First(&conversation).Error

	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("agent conversation not found")
		}
		return nil, fmt.Errorf("failed to get agent conversation: %w", err)
	}

	return &conversation, nil
}

// Update updates an agent conversation
func (r *agentConversationRepository) Update(ctx context.Context, conversation *AgentConversation) error {
	conversation.UpdatedAt = time.Now()

	err := r.db.WithContext(ctx).
		Where("id = ? AND deleted_at IS NULL", conversation.ID).
		Updates(conversation).Error

	if err != nil {
		return fmt.Errorf("failed to update agent conversation: %w", err)
	}
	return nil
}

// Delete soft deletes an agent conversation
func (r *agentConversationRepository) Delete(ctx context.Context, id uuid.UUID, deletedBy uuid.UUID) error {
	now := time.Now()
	err := r.db.WithContext(ctx).
		Model(&AgentConversation{}).
		Where("id = ? AND deleted_at IS NULL", id).
		Updates(map[string]interface{}{
			"deleted_by": deletedBy,
			"deleted_at": now,
			"updated_at": now,
		}).Error

	if err != nil {
		return fmt.Errorf("failed to delete agent conversation: %w", err)
	}
	return nil
}

// GetByAgentAndUser retrieves conversations by agent and user with pagination
func (r *agentConversationRepository) GetByAgentAndUser(ctx context.Context, agentID uuid.UUID, fromSource string, userID uuid.UUID, limit, offset int) ([]*AgentConversation, int64, error) {
	var conversations []*AgentConversation
	var total int64

	query := r.db.WithContext(ctx).
		Model(&AgentConversation{}).
		Where("agent_id = ? AND from_source = ? AND deleted_at IS NULL", agentID, fromSource)

	// Add user condition based on source type
	if fromSource == "end_user" {
		query = query.Where("from_end_user_id = ?", userID)
	} else if fromSource == "account" {
		query = query.Where("from_account_id = ?", userID)
	}

	// Get total count
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to count agent conversations: %w", err)
	}

	// Get conversations with pagination
	err := query.
		Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&conversations).Error

	if err != nil {
		return nil, 0, fmt.Errorf("failed to get agent conversations: %w", err)
	}

	return conversations, total, nil
}

// GetByAgent retrieves conversations by agent with pagination
func (r *agentConversationRepository) GetByAgent(ctx context.Context, agentID uuid.UUID, limit, offset int) ([]*AgentConversation, int64, error) {
	var conversations []*AgentConversation
	var total int64

	query := r.db.WithContext(ctx).
		Model(&AgentConversation{}).
		Where("agent_id = ? AND deleted_at IS NULL", agentID)

	// Get total count
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to count agent conversations: %w", err)
	}

	// Get conversations with pagination
	err := query.
		Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&conversations).Error

	if err != nil {
		return nil, 0, fmt.Errorf("failed to get agent conversations: %w", err)
	}

	return conversations, total, nil
}

// GetByAgentWithFilters retrieves conversations by agent with filters and pagination
func (r *agentConversationRepository) GetByAgentWithFilters(ctx context.Context, agentID, userID uuid.UUID, versionUUID string, limit, offset int) ([]*AgentConversation, int64, error) {
	var conversations []*AgentConversation
	var total int64

	query := r.db.WithContext(ctx).
		Model(&AgentConversation{}).
		Where("agent_id = ? AND deleted_at IS NULL", agentID)

	// Filter by invoke_from = 'web-app' to only return web app conversations
	query = query.Where("invoke_from = ?", "web-app")

	// Filter by user (created_by)
	if userID != uuid.Nil {
		query = query.Where("(created_by = ?)", userID)
	}

	// Get total count
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to count agent conversations: %w", err)
	}

	// Get conversations with pagination
	err := query.
		Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&conversations).Error

	if err != nil {
		return nil, 0, fmt.Errorf("failed to get agent conversations: %w", err)
	}

	return conversations, total, nil
}

// GetHistoryByAgent retrieves agent conversations for console history views.
func (r *agentConversationRepository) GetHistoryByAgent(ctx context.Context, filter AgentConversationHistoryFilter) ([]*AgentConversation, int64, error) {
	var conversations []*AgentConversation
	var total int64

	query := r.db.WithContext(ctx).
		Model(&AgentConversation{}).
		Where("agent_id = ? AND deleted_at IS NULL", filter.AgentID)

	if len(filter.InvokeFroms) > 0 {
		query = query.Where("invoke_from IN ?", filter.InvokeFroms)
	}
	if filter.Keyword != "" {
		keyword := "%" + filter.Keyword + "%"
		query = query.Where("LOWER(name) LIKE LOWER(?) OR LOWER(COALESCE(summary, '')) LIKE LOWER(?)", keyword, keyword)
	}
	if filter.Start != nil {
		query = query.Where("created_at >= ?", *filter.Start)
	}
	if filter.End != nil {
		query = query.Where("created_at <= ?", *filter.End)
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to count agent conversation history: %w", err)
	}

	sortBy := "created_at ASC"
	switch filter.SortBy {
	case "-created_at":
		sortBy = "created_at DESC"
	case "updated_at":
		sortBy = "updated_at ASC"
	case "-updated_at":
		sortBy = "updated_at DESC"
	}

	err := query.
		Order(sortBy).
		Limit(filter.Limit).
		Offset(filter.Offset).
		Find(&conversations).Error
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get agent conversation history: %w", err)
	}

	return conversations, total, nil
}

// IncrementDialogueCount increments the dialogue count for a conversation
func (r *agentConversationRepository) IncrementDialogueCount(ctx context.Context, id uuid.UUID) error {
	err := r.db.WithContext(ctx).
		Model(&AgentConversation{}).
		Where("id = ? AND deleted_at IS NULL", id).
		Updates(map[string]interface{}{
			"dialogue_count": gorm.Expr("dialogue_count + 1"),
			"updated_at":     time.Now(),
		}).Error

	if err != nil {
		return fmt.Errorf("failed to increment dialogue count: %w", err)
	}
	return nil
}

// GetWithMessages retrieves a conversation with its messages
func (r *agentConversationRepository) GetWithMessages(ctx context.Context, id uuid.UUID, messageLimit int) (*AgentConversation, error) {
	var conversation AgentConversation

	query := r.db.WithContext(ctx).
		Where("id = ? AND deleted_at IS NULL", id)

	if messageLimit > 0 {
		query = query.Preload("Messages", func(db *gorm.DB) *gorm.DB {
			return db.Where("deleted_at IS NULL").
				Order("created_at DESC").
				Limit(messageLimit)
		})
	} else {
		query = query.Preload("Messages", "deleted_at IS NULL")
	}

	err := query.First(&conversation).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("agent conversation not found")
		}
		return nil, fmt.Errorf("failed to get agent conversation with messages: %w", err)
	}

	return &conversation, nil
}

// UpdateWebAppID updates the web_app_id for an existing conversation
func (r *agentConversationRepository) UpdateWebAppID(ctx context.Context, conversationID uuid.UUID, webAppID string) error {
	err := r.db.WithContext(ctx).
		Model(&AgentConversation{}).
		Where("id = ? AND deleted_at IS NULL", conversationID).
		Update("web_app_id", webAppID).Error

	if err != nil {
		return fmt.Errorf("failed to update conversation web_app_id: %w", err)
	}

	return nil
}

// UpdateNameIfCurrent updates the conversation name only when the caller still sees the same current name.
func (r *agentConversationRepository) UpdateNameIfCurrent(ctx context.Context, conversationID uuid.UUID, currentName, nextName string) (bool, error) {
	result := r.db.WithContext(ctx).
		Model(&AgentConversation{}).
		Where("id = ? AND name = ? AND deleted_at IS NULL", conversationID, currentName).
		Updates(map[string]interface{}{
			"name":       nextName,
			"updated_at": time.Now(),
		})
	if result.Error != nil {
		return false, fmt.Errorf("failed to update conversation name: %w", result.Error)
	}
	return result.RowsAffected > 0, nil
}

// MigrateConversationsByAccountID migrates conversations from virtual user to authenticated user
func (r *agentConversationRepository) MigrateConversationsByAccountID(ctx context.Context, tx *gorm.DB, virtualAccountID, authenticatedAccountID uuid.UUID) (int64, error) {
	logger.Debug("Migrating conversations", map[string]interface{}{
		"virtual_account_id":       virtualAccountID.String(),
		"authenticated_account_id": authenticatedAccountID.String(),
	})

	result := tx.WithContext(ctx).
		Model(&AgentConversation{}).
		Where("created_by = ? AND deleted_at IS NULL", virtualAccountID).
		Updates(map[string]interface{}{
			"created_by": authenticatedAccountID,
			"updated_at": time.Now(),
		})

	if result.Error != nil {
		logger.Error("Failed to migrate conversations", result.Error)
		return 0, fmt.Errorf("failed to migrate conversations: %w", result.Error)
	}

	logger.Info("Conversations migrated", map[string]interface{}{
		"rows_affected":            result.RowsAffected,
		"virtual_account_id":       virtualAccountID.String(),
		"authenticated_account_id": authenticatedAccountID.String(),
	})

	return result.RowsAffected, nil
}
