package workflow

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/app/conversation"
	"github.com/zgiai/zgi/api/pkg/database"
	"github.com/zgiai/zgi/api/pkg/logger"
	"gorm.io/gorm"
)

// UserMigrationService defines the interface for user data migration operations
type UserMigrationService interface {
	// MigrateUserData executes user data migration from virtual account to authenticated account
	MigrateUserData(ctx context.Context, virtualAccountID, authenticatedAccountID string) (*MigrationResult, error)

	// ValidateMigrationRequest validates the migration request parameters
	ValidateMigrationRequest(virtualAccountID, authenticatedAccountID string) error

	// GetMigrationStatistics retrieves migration statistics for a virtual user
	GetMigrationStatistics(ctx context.Context, virtualAccountID string) (*MigrationStatistics, error)
}

// MigrationResult represents the result of a user migration operation
type MigrationResult struct {
	ConversationsMigrated   int    `json:"conversations_migrated"`
	MessagesMigrated        int    `json:"messages_migrated"`
	WorkflowRunLogsMigrated int    `json:"workflow_run_logs_migrated"`
	NodeRuntimeLogsMigrated int    `json:"node_runtime_logs_migrated"`
	AuthenticatedAccountID  string `json:"authenticated_account_id"`
}

// MigrationStatistics represents statistics about data to be migrated
type MigrationStatistics struct {
	ConversationsCount   int
	MessagesCount        int
	WorkflowRunLogsCount int
	NodeRuntimeLogsCount int
}

// userMigrationService implements UserMigrationService
type userMigrationService struct {
	db                         *gorm.DB
	conversationRepo           conversation.AgentConversationRepository
	messageRepo                conversation.AgentMessageRepository
	workflowRunLogRepo         WorkflowRunLogRepository
	workflowNodeRuntimeLogRepo WorkflowNodeRuntimeLogRepository
}

// NewUserMigrationService creates a new UserMigrationService instance
func NewUserMigrationService(
	db *gorm.DB,
	conversationRepo conversation.AgentConversationRepository,
	messageRepo conversation.AgentMessageRepository,
	workflowRunLogRepo WorkflowRunLogRepository,
	workflowNodeRuntimeLogRepo WorkflowNodeRuntimeLogRepository,
) UserMigrationService {
	return &userMigrationService{
		db:                         db,
		conversationRepo:           conversationRepo,
		messageRepo:                messageRepo,
		workflowRunLogRepo:         workflowRunLogRepo,
		workflowNodeRuntimeLogRepo: workflowNodeRuntimeLogRepo,
	}
}

// ValidateMigrationRequest validates the migration request parameters
func (s *userMigrationService) ValidateMigrationRequest(virtualAccountID, authenticatedAccountID string) error {
	// Validate virtual account ID is a valid UUID
	if _, err := uuid.Parse(virtualAccountID); err != nil {
		return fmt.Errorf("invalid virtual user ID format: %w", err)
	}

	// Validate authenticated account ID is a valid UUID
	if _, err := uuid.Parse(authenticatedAccountID); err != nil {
		return fmt.Errorf("invalid authenticated user ID format: %w", err)
	}

	// Ensure the two IDs are different
	if virtualAccountID == authenticatedAccountID {
		return fmt.Errorf("cannot migrate user to the same account")
	}

	return nil
}

// GetMigrationStatistics retrieves migration statistics for a virtual user
func (s *userMigrationService) GetMigrationStatistics(ctx context.Context, virtualAccountID string) (*MigrationStatistics, error) {
	stats := &MigrationStatistics{}

	virtualUUID, err := uuid.Parse(virtualAccountID)
	if err != nil {
		return nil, fmt.Errorf("invalid virtual account ID: %w", err)
	}

	// Count conversations
	var conversationsCount int64
	if err := s.db.WithContext(ctx).
		Model(&conversation.AgentConversation{}).
		Where("created_by = ? AND deleted_at IS NULL", virtualUUID).
		Count(&conversationsCount).Error; err != nil {
		logger.Error("Failed to count conversations for migration statistics", err)
		return nil, fmt.Errorf("failed to count conversations: %w", err)
	}
	stats.ConversationsCount = int(conversationsCount)

	// Count messages
	var messagesCount int64
	if err := s.db.WithContext(ctx).
		Model(&conversation.AgentMessage{}).
		Where("created_by = ? AND deleted_at IS NULL", virtualUUID).
		Count(&messagesCount).Error; err != nil {
		logger.Error("Failed to count messages for migration statistics", err)
		return nil, fmt.Errorf("failed to count messages: %w", err)
	}
	stats.MessagesCount = int(messagesCount)

	// Count workflow run logs
	var workflowRunLogsCount int64
	if err := s.db.WithContext(ctx).
		Model(&WorkflowRunLog{}).
		Where("created_by = ? AND deleted_at IS NULL", virtualAccountID).
		Count(&workflowRunLogsCount).Error; err != nil {
		logger.Error("Failed to count workflow run logs for migration statistics", err)
		return nil, fmt.Errorf("failed to count workflow run logs: %w", err)
	}
	stats.WorkflowRunLogsCount = int(workflowRunLogsCount)

	// Count workflow node runtime logs
	var nodeRuntimeLogsCount int64
	if err := s.db.WithContext(ctx).
		Model(&WorkflowNodeRuntimeLog{}).
		Where("created_by = ? AND deleted_at IS NULL", virtualAccountID).
		Count(&nodeRuntimeLogsCount).Error; err != nil {
		logger.Error("Failed to count node runtime logs for migration statistics", err)
		return nil, fmt.Errorf("failed to count node runtime logs: %w", err)
	}
	stats.NodeRuntimeLogsCount = int(nodeRuntimeLogsCount)

	return stats, nil
}

// MigrateUserData executes user data migration from virtual account to authenticated account
// This method implements idempotency - it can be safely called multiple times with the same parameters
func (s *userMigrationService) MigrateUserData(ctx context.Context, virtualAccountID, authenticatedAccountID string) (*MigrationResult, error) {
	startTime := time.Now()

	logger.Info("Starting user migration", "virtual_account_id", virtualAccountID, "authenticated_account_id", authenticatedAccountID)

	// Validate migration request
	if err := s.ValidateMigrationRequest(virtualAccountID, authenticatedAccountID); err != nil {
		logger.Error("Migration validation failed", err)
		return nil, err
	}

	// Get migration statistics before starting (idempotency check)
	// This counts only records that still belong to the virtual user
	stats, err := s.GetMigrationStatistics(ctx, virtualAccountID)
	if err != nil {
		logger.Error("Failed to get migration statistics", err)
		return nil, fmt.Errorf("failed to get migration statistics: %w", err)
	}

	// Idempotency: If there's no data to migrate, return success with zero counts
	// This handles the case where migration was already completed or the virtual user has no data
	if stats.ConversationsCount == 0 && stats.MessagesCount == 0 &&
		stats.WorkflowRunLogsCount == 0 && stats.NodeRuntimeLogsCount == 0 {
		logger.Info("No data to migrate for virtual user (already migrated or no data exists)",
			"virtual_account_id", virtualAccountID)
		return &MigrationResult{
			ConversationsMigrated:   0,
			MessagesMigrated:        0,
			WorkflowRunLogsMigrated: 0,
			NodeRuntimeLogsMigrated: 0,
			AuthenticatedAccountID:  authenticatedAccountID,
		}, nil
	}

	// Start database transaction for atomic migration
	// All migration operations must succeed or all will be rolled back
	tx := s.db.WithContext(ctx).Begin()
	if tx.Error != nil {
		logger.Error("Failed to start transaction for user migration", tx.Error)
		return nil, fmt.Errorf("failed to start transaction: %w", tx.Error)
	}

	// Ensure transaction is rolled back on error or panic
	// This provides transaction safety and prevents partial migrations
	var committed bool
	defer func() {
		if r := recover(); r != nil {
			if !committed {
				tx.Rollback()
				logger.Error(fmt.Sprintf("Panic during migration (virtual: %s, authenticated: %s), transaction rolled back",
					virtualAccountID, authenticatedAccountID), fmt.Errorf("%v", r))
			}
		}
	}()

	result := &MigrationResult{
		AuthenticatedAccountID: authenticatedAccountID,
	}

	// Migrate conversations
	// Transaction will be rolled back if this fails
	conversationsMigrated, err := s.migrateConversations(ctx, tx, virtualAccountID, authenticatedAccountID)
	if err != nil {
		tx.Rollback()
		logger.Error(fmt.Sprintf("Failed to migrate conversations (virtual: %s, authenticated: %s), rolling back transaction",
			virtualAccountID, authenticatedAccountID), err)
		return nil, fmt.Errorf("failed to migrate conversations: %w", err)
	}
	result.ConversationsMigrated = int(conversationsMigrated)
	logger.Debug("Migrated conversations in transaction", "count", conversationsMigrated)

	// Migrate messages
	// Transaction will be rolled back if this fails
	messagesMigrated, err := s.migrateMessages(ctx, tx, virtualAccountID, authenticatedAccountID)
	if err != nil {
		tx.Rollback()
		logger.Error(fmt.Sprintf("Failed to migrate messages (virtual: %s, authenticated: %s), rolling back transaction",
			virtualAccountID, authenticatedAccountID), err)
		return nil, fmt.Errorf("failed to migrate messages: %w", err)
	}
	result.MessagesMigrated = int(messagesMigrated)
	logger.Debug("Migrated messages in transaction", "count", messagesMigrated)

	// Migrate workflow run logs
	// Transaction will be rolled back if this fails
	workflowRunLogsMigrated, err := s.migrateWorkflowRunLogs(ctx, tx, virtualAccountID, authenticatedAccountID)
	if err != nil {
		tx.Rollback()
		logger.Error(fmt.Sprintf("Failed to migrate workflow run logs (virtual: %s, authenticated: %s), rolling back transaction",
			virtualAccountID, authenticatedAccountID), err)
		return nil, fmt.Errorf("failed to migrate workflow run logs: %w", err)
	}
	result.WorkflowRunLogsMigrated = int(workflowRunLogsMigrated)
	logger.Debug("Migrated workflow run logs in transaction", "count", workflowRunLogsMigrated)

	// Migrate workflow node runtime logs
	// Transaction will be rolled back if this fails
	nodeRuntimeLogsMigrated, err := s.migrateNodeRuntimeLogs(ctx, tx, virtualAccountID, authenticatedAccountID)
	if err != nil {
		tx.Rollback()
		logger.Error(fmt.Sprintf("Failed to migrate node runtime logs (virtual: %s, authenticated: %s), rolling back transaction",
			virtualAccountID, authenticatedAccountID), err)
		return nil, fmt.Errorf("failed to migrate node runtime logs: %w", err)
	}
	result.NodeRuntimeLogsMigrated = int(nodeRuntimeLogsMigrated)
	logger.Debug("Migrated node runtime logs in transaction", "count", nodeRuntimeLogsMigrated)

	// Commit transaction - all migrations succeeded
	if err := tx.Commit().Error; err != nil {
		logger.Error(fmt.Sprintf("Failed to commit migration transaction (virtual: %s, authenticated: %s)",
			virtualAccountID, authenticatedAccountID), err)
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}
	committed = true

	elapsedTime := time.Since(startTime)

	logger.Info("User migration completed successfully",
		"virtual_account_id", virtualAccountID,
		"authenticated_account_id", authenticatedAccountID,
		"conversations_migrated", result.ConversationsMigrated,
		"messages_migrated", result.MessagesMigrated,
		"workflow_runs_migrated", result.WorkflowRunLogsMigrated,
		"node_logs_migrated", result.NodeRuntimeLogsMigrated,
		"duration_ms", elapsedTime.Milliseconds(),
	)

	return result, nil
}

// migrateConversations migrates conversations from virtual user to authenticated user
func (s *userMigrationService) migrateConversations(ctx context.Context, tx *gorm.DB, virtualAccountID, authenticatedAccountID string) (int64, error) {
	virtualUUID, _ := uuid.Parse(virtualAccountID)
	authenticatedUUID, _ := uuid.Parse(authenticatedAccountID)

	result := tx.WithContext(ctx).
		Model(&conversation.AgentConversation{}).
		Where("created_by = ? AND deleted_at IS NULL", virtualUUID).
		Updates(map[string]interface{}{
			"created_by": authenticatedUUID,
			"updated_at": time.Now(),
		})

	if result.Error != nil {
		return 0, fmt.Errorf("failed to migrate conversations: %w", result.Error)
	}

	return result.RowsAffected, nil
}

// migrateMessages migrates messages from virtual user to authenticated user
func (s *userMigrationService) migrateMessages(ctx context.Context, tx *gorm.DB, virtualAccountID, authenticatedAccountID string) (int64, error) {
	virtualUUID, _ := uuid.Parse(virtualAccountID)
	authenticatedUUID, _ := uuid.Parse(authenticatedAccountID)

	result := tx.WithContext(ctx).
		Model(&conversation.AgentMessage{}).
		Where("created_by = ? AND deleted_at IS NULL", virtualUUID).
		Updates(map[string]interface{}{
			"created_by": authenticatedUUID,
			"updated_at": time.Now(),
		})

	if result.Error != nil {
		return 0, fmt.Errorf("failed to migrate messages: %w", result.Error)
	}

	return result.RowsAffected, nil
}

// migrateWorkflowRunLogs migrates workflow run logs from virtual user to authenticated user
func (s *userMigrationService) migrateWorkflowRunLogs(ctx context.Context, tx *gorm.DB, virtualAccountID, authenticatedAccountID string) (int64, error) {
	result := tx.WithContext(ctx).
		Model(&WorkflowRunLog{}).
		Where("created_by = ? AND deleted_at IS NULL", virtualAccountID).
		Update("created_by", authenticatedAccountID)

	if result.Error != nil {
		return 0, fmt.Errorf("failed to migrate workflow run logs: %w", result.Error)
	}

	return result.RowsAffected, nil
}

// migrateNodeRuntimeLogs migrates workflow node runtime logs from virtual user to authenticated user
func (s *userMigrationService) migrateNodeRuntimeLogs(ctx context.Context, tx *gorm.DB, virtualAccountID, authenticatedAccountID string) (int64, error) {
	result := tx.WithContext(ctx).
		Model(&WorkflowNodeRuntimeLog{}).
		Where("created_by = ? AND deleted_at IS NULL", virtualAccountID).
		Update("created_by", authenticatedAccountID)

	if result.Error != nil {
		return 0, fmt.Errorf("failed to migrate node runtime logs: %w", result.Error)
	}

	return result.RowsAffected, nil
}

// NewUserMigrationServiceFromDB creates a new UserMigrationService with database connection
func NewUserMigrationServiceFromDB() UserMigrationService {
	db := database.GetDB()
	conversationRepo := conversation.NewAgentConversationRepository(db)
	messageRepo := conversation.NewAgentMessageRepository(db)

	// Note: WorkflowRunLogRepository and WorkflowNodeRuntimeLogRepository will be created in task 3
	// For now, we pass nil and handle them directly in the service
	return NewUserMigrationService(db, conversationRepo, messageRepo, nil, nil)
}
