package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type ConversationRepository interface {
	Create(ctx context.Context, conversation *runtimemodel.Conversation) error
	GetScoped(ctx context.Context, id, organizationID, accountID uuid.UUID) (*runtimemodel.Conversation, error)
	GetByCallerScoped(ctx context.Context, id, organizationID, accountID uuid.UUID, callerType string, callerID *uuid.UUID) (*runtimemodel.Conversation, error)
	GetRuntimeLogScoped(ctx context.Context, id, organizationID uuid.UUID, workspaceID *uuid.UUID, accountID uuid.UUID, callerType string, callerID *uuid.UUID, source string) (*runtimemodel.Conversation, error)
	GetBySourceConversation(ctx context.Context, sourceConversationID uuid.UUID) (*runtimemodel.Conversation, error)
	ListScoped(ctx context.Context, organizationID, accountID uuid.UUID, limit, offset int) ([]*runtimemodel.Conversation, int64, error)
	ListByCallerScoped(ctx context.Context, organizationID, accountID uuid.UUID, callerType string, callerID *uuid.UUID, limit, offset int) ([]*runtimemodel.Conversation, int64, error)
	UpdateScoped(ctx context.Context, id, organizationID, accountID uuid.UUID, updates map[string]interface{}) error
	UpdateMetadata(ctx context.Context, id uuid.UUID, metadata map[string]interface{}) error
	DeleteScoped(ctx context.Context, id, organizationID, accountID uuid.UUID) error
	StartStreaming(ctx context.Context, id, organizationID, accountID, messageID uuid.UUID) error
	ClearActiveMessage(ctx context.Context, id, messageID uuid.UUID) error
	FinishActiveMessage(ctx context.Context, id, messageID uuid.UUID) error
	FinishWaitingApprovalMessage(ctx context.Context, id, messageID uuid.UUID) error
	FinishContinuationMessage(ctx context.Context, id, messageID uuid.UUID) error
	ClearActiveMessages(ctx context.Context, messageIDs []uuid.UUID) error
	CompleteRootReplacement(ctx context.Context, id, messageID uuid.UUID) error
	UpdateAfterMessage(ctx context.Context, id uuid.UUID, leafMessageID uuid.UUID) error
	RefreshAfterMessageDelete(ctx context.Context, id uuid.UUID) error
}

type MessageRepository interface {
	Create(ctx context.Context, message *runtimemodel.Message) error
	GetScoped(ctx context.Context, id, organizationID, accountID uuid.UUID) (*runtimemodel.Message, error)
	GetBySourceMessage(ctx context.Context, sourceMessageID uuid.UUID) (*runtimemodel.Message, error)
	ListByConversationScoped(ctx context.Context, conversationID, organizationID, accountID uuid.UUID, limit, offset int) ([]*runtimemodel.Message, int64, error)
	ListByCallerScoped(ctx context.Context, organizationID, accountID uuid.UUID, callerType string, callerID *uuid.UUID, limit, offset int) ([]*runtimemodel.Message, int64, error)
	ListByCallerSourceScoped(ctx context.Context, organizationID, accountID uuid.UUID, callerType string, callerID *uuid.UUID, source string, limit, offset int) ([]*runtimemodel.Message, int64, error)
	ListByCallerLogFilterScoped(ctx context.Context, organizationID, accountID uuid.UUID, callerType string, callerID *uuid.UUID, source string, conversationID *uuid.UUID, queryText string, limit, offset int) ([]*runtimemodel.Message, int64, error)
	ListByCallerRuntimeLogScoped(ctx context.Context, organizationID uuid.UUID, workspaceID *uuid.UUID, accountID uuid.UUID, callerType string, callerID *uuid.UUID, source string, conversationID *uuid.UUID, queryText string, limit, offset int) ([]*runtimemodel.Message, int64, error)
	GetRuntimeLogScoped(ctx context.Context, id, organizationID uuid.UUID, workspaceID *uuid.UUID, accountID uuid.UUID, callerType string, callerID *uuid.UUID, source string) (*runtimemodel.Message, error)
	ListBranch(ctx context.Context, leafID uuid.UUID, maxDepth int) ([]*runtimemodel.Message, error)
	CountByConversation(ctx context.Context, conversationID uuid.UUID) (int64, error)
	ReplaceRootForStreaming(ctx context.Context, message *runtimemodel.Message) error
	UpdateCompleted(ctx context.Context, id uuid.UUID, answer string, metadata map[string]interface{}) error
	UpdateMetadata(ctx context.Context, id uuid.UUID, metadata map[string]interface{}) error
	UpdateMetadataAnyStatus(ctx context.Context, id uuid.UUID, metadata map[string]interface{}) error
	UpdateWaitingApproval(ctx context.Context, id uuid.UUID, metadata map[string]interface{}) error
	UpdateWaitingQuestion(ctx context.Context, id uuid.UUID, metadata map[string]interface{}) error
	UpdateWaitingClientAction(ctx context.Context, id uuid.UUID, metadata map[string]interface{}) error
	UpdateError(ctx context.Context, id uuid.UUID, message string) error
	MarkStopped(ctx context.Context, id uuid.UUID) error
	UpdateStoppedAnswer(ctx context.Context, id uuid.UUID, answer string, metadata map[string]interface{}) error
	DeleteSubtreeScoped(ctx context.Context, id, organizationID, accountID uuid.UUID) (*MessageDeleteResult, error)
	ListStaleActiveIDs(ctx context.Context, cutoff time.Time) ([]uuid.UUID, error)
	MarkStaleActiveAsError(ctx context.Context, cutoff time.Time, message string) (int64, error)
}

type AccessRepository interface {
	IsOrganizationMember(ctx context.Context, organizationID, accountID uuid.UUID) (bool, error)
	GetCurrentWorkspaceID(ctx context.Context, accountID uuid.UUID) (*uuid.UUID, error)
}

type OrganizationSkillConfigRepository interface {
	ListByOrganization(ctx context.Context, organizationID uuid.UUID) ([]*runtimemodel.OrganizationSkillConfig, error)
	ReplaceForOrganization(ctx context.Context, organizationID uuid.UUID, configs []*runtimemodel.OrganizationSkillConfig) error
	DeleteByOrganizationAndSkill(ctx context.Context, organizationID uuid.UUID, skillID string) error
}

type AccountSkillPreferenceRepository interface {
	Get(ctx context.Context, organizationID, accountID uuid.UUID, callerType string) (*runtimemodel.AccountSkillPreference, error)
	Upsert(ctx context.Context, pref *runtimemodel.AccountSkillPreference) error
}

type CustomSkillRepository interface {
	ListByOrganization(ctx context.Context, organizationID uuid.UUID) ([]*runtimemodel.CustomSkill, error)
	ListManageableByOrganization(ctx context.Context, organizationID uuid.UUID) ([]*runtimemodel.CustomSkill, error)
	GetBySkillID(ctx context.Context, organizationID uuid.UUID, skillID string) (*runtimemodel.CustomSkill, error)
	Upsert(ctx context.Context, skill *runtimemodel.CustomSkill) error
	DeleteBySkillID(ctx context.Context, organizationID uuid.UUID, skillID string) error
}

type Repositories struct {
	Conversation ConversationRepository
	Message      MessageRepository
	Access       AccessRepository
	SkillConfig  OrganizationSkillConfigRepository
	SkillPref    AccountSkillPreferenceRepository
	CustomSkill  CustomSkillRepository
	DB           *gorm.DB
}

type MessageDeleteResult struct {
	ConversationID uuid.UUID
	DeletedCount   int64
}

func NewRepositories(db *gorm.DB) *Repositories {
	return &Repositories{
		Conversation: NewConversationRepository(db),
		Message:      NewMessageRepository(db),
		Access:       NewAccessRepository(db),
		SkillConfig:  NewOrganizationSkillConfigRepository(db),
		SkillPref:    NewAccountSkillPreferenceRepository(db),
		CustomSkill:  NewCustomSkillRepository(db),
		DB:           db,
	}
}

type conversationRepository struct {
	db *gorm.DB
}

func NewConversationRepository(db *gorm.DB) ConversationRepository {
	return &conversationRepository{db: db}
}

func (r *conversationRepository) Create(ctx context.Context, conversation *runtimemodel.Conversation) error {
	now := time.Now()
	if conversation.ID == uuid.Nil {
		conversation.ID = uuid.New()
	}
	if conversation.Status == "" {
		conversation.Status = runtimemodel.ConversationStatusNormal
	}
	if conversation.RuntimeStatus == "" {
		conversation.RuntimeStatus = runtimemodel.ConversationRuntimeStatusIdle
	}
	if conversation.CallerType == "" {
		conversation.CallerType = runtimemodel.ConversationCallerAIChat
	}
	if conversation.Source == "" {
		conversation.Source = runtimemodel.ConversationSourceConsole
	}
	if conversation.Metadata == nil {
		conversation.Metadata = map[string]interface{}{}
	}
	conversation.CreatedAt = now
	conversation.UpdatedAt = now
	if err := r.db.WithContext(ctx).Create(conversation).Error; err != nil {
		return fmt.Errorf("failed to create aichat conversation: %w", err)
	}
	return nil
}

func (r *conversationRepository) GetScoped(ctx context.Context, id, organizationID, accountID uuid.UUID) (*runtimemodel.Conversation, error) {
	var conversation runtimemodel.Conversation
	err := r.db.WithContext(ctx).
		Where("id = ? AND organization_id = ? AND account_id = ? AND deleted_at IS NULL", id, organizationID, accountID).
		Take(&conversation).Error
	if err != nil {
		return nil, wrapNotFound(err, "aichat conversation")
	}
	return &conversation, nil
}

func (r *conversationRepository) GetByCallerScoped(ctx context.Context, id, organizationID, accountID uuid.UUID, callerType string, callerID *uuid.UUID) (*runtimemodel.Conversation, error) {
	var conversation runtimemodel.Conversation
	query := applyCallerFilter(r.db.WithContext(ctx).
		Where("id = ? AND organization_id = ? AND account_id = ? AND deleted_at IS NULL", id, organizationID, accountID), callerType, callerID)
	if err := query.Take(&conversation).Error; err != nil {
		return nil, wrapNotFound(err, "chat runtime conversation")
	}
	return &conversation, nil
}

func (r *conversationRepository) GetRuntimeLogScoped(ctx context.Context, id, organizationID uuid.UUID, workspaceID *uuid.UUID, accountID uuid.UUID, callerType string, callerID *uuid.UUID, source string) (*runtimemodel.Conversation, error) {
	var conversation runtimemodel.Conversation
	query := applyRuntimeLogSourceFilter(applyCallerFilter(r.db.WithContext(ctx).
		Where("id = ? AND organization_id = ? AND deleted_at IS NULL", id, organizationID), callerType, callerID), accountID, source, "")
	if workspaceID != nil && *workspaceID != uuid.Nil {
		query = query.Where("workspace_id = ?", *workspaceID)
	}
	if err := query.Take(&conversation).Error; err != nil {
		return nil, wrapNotFound(err, "chat runtime conversation")
	}
	return &conversation, nil
}

func (r *conversationRepository) GetBySourceConversation(ctx context.Context, sourceConversationID uuid.UUID) (*runtimemodel.Conversation, error) {
	var conversation runtimemodel.Conversation
	err := r.db.WithContext(ctx).
		Where("source_conversation_id = ? AND deleted_at IS NULL", sourceConversationID).
		Take(&conversation).Error
	if err != nil {
		return nil, wrapNotFound(err, "aichat conversation")
	}
	return &conversation, nil
}

func (r *conversationRepository) ListScoped(ctx context.Context, organizationID, accountID uuid.UUID, limit, offset int) ([]*runtimemodel.Conversation, int64, error) {
	var conversations []*runtimemodel.Conversation
	var total int64
	query := r.db.WithContext(ctx).Model(&runtimemodel.Conversation{}).
		Where("organization_id = ? AND account_id = ? AND deleted_at IS NULL", organizationID, accountID)
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to count aichat conversations: %w", err)
	}
	if err := query.Order("updated_at DESC, created_at DESC, id DESC").Limit(limit).Offset(offset).Find(&conversations).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to list aichat conversations: %w", err)
	}
	return conversations, total, nil
}

func (r *conversationRepository) ListByCallerScoped(ctx context.Context, organizationID, accountID uuid.UUID, callerType string, callerID *uuid.UUID, limit, offset int) ([]*runtimemodel.Conversation, int64, error) {
	var conversations []*runtimemodel.Conversation
	var total int64
	query := applyCallerFilter(r.db.WithContext(ctx).Model(&runtimemodel.Conversation{}).
		Where("organization_id = ? AND account_id = ? AND deleted_at IS NULL", organizationID, accountID), callerType, callerID)
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to count chat runtime conversations: %w", err)
	}
	if err := query.Order("updated_at DESC, created_at DESC, id DESC").Limit(limit).Offset(offset).Find(&conversations).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to list chat runtime conversations: %w", err)
	}
	return conversations, total, nil
}

func applyCallerFilter(query *gorm.DB, callerType string, callerID *uuid.UUID) *gorm.DB {
	if callerType == "" {
		callerType = runtimemodel.ConversationCallerAIChat
	}
	query = query.Where("caller_type = ?", callerType)
	if callerID != nil && *callerID != uuid.Nil {
		return query.Where("caller_id = ?", *callerID)
	}
	return query.Where("caller_id IS NULL")
}

func applyRuntimeLogCallerFilter(query *gorm.DB, callerType string, callerID *uuid.UUID, alias string) *gorm.DB {
	if callerType == "" {
		callerType = runtimemodel.ConversationCallerAIChat
	}
	callerTypeColumn := qualifiedColumn(alias, "caller_type")
	callerIDColumn := qualifiedColumn(alias, "caller_id")
	query = query.Where(callerTypeColumn+" = ?", callerType)
	if callerID != nil && *callerID != uuid.Nil {
		return query.Where(callerIDColumn+" = ?", *callerID)
	}
	return query.Where(callerIDColumn + " IS NULL")
}

func applyRuntimeLogSourceFilter(query *gorm.DB, accountID uuid.UUID, source string, alias string) *gorm.DB {
	sourceColumn := qualifiedColumn(alias, "source")
	sourceWebAppIDColumn := qualifiedColumn(alias, "source_web_app_id")
	accountIDColumn := qualifiedColumn(alias, "account_id")
	switch strings.TrimSpace(source) {
	case runtimemodel.ConversationSourceWebApp:
		return query.Where(sourceColumn+" = ? AND "+sourceWebAppIDColumn+" IS NOT NULL", runtimemodel.ConversationSourceWebApp)
	case runtimemodel.ConversationSourceConsole:
		return query.Where(sourceColumn+" = ? AND "+accountIDColumn+" = ?", runtimemodel.ConversationSourceConsole, accountID)
	case "":
		return query.Where(accountIDColumn+" = ?", accountID)
	default:
		return query.Where(sourceColumn+" = ? AND "+accountIDColumn+" = ?", source, accountID)
	}
}

func qualifiedColumn(alias string, column string) string {
	if strings.TrimSpace(alias) == "" {
		return column
	}
	return alias + "." + column
}

func (r *conversationRepository) UpdateScoped(ctx context.Context, id, organizationID, accountID uuid.UUID, updates map[string]interface{}) error {
	if len(updates) == 0 {
		return nil
	}
	updates["updated_at"] = time.Now()
	result := r.db.WithContext(ctx).Model(&runtimemodel.Conversation{}).
		Where("id = ? AND organization_id = ? AND account_id = ? AND deleted_at IS NULL", id, organizationID, accountID).
		Updates(updates)
	if result.Error != nil {
		return fmt.Errorf("failed to update aichat conversation: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (r *conversationRepository) UpdateMetadata(ctx context.Context, id uuid.UUID, metadata map[string]interface{}) error {
	if metadata == nil {
		metadata = map[string]interface{}{}
	}
	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal aichat conversation metadata: %w", err)
	}
	result := r.db.WithContext(ctx).Model(&runtimemodel.Conversation{}).
		Where("id = ? AND deleted_at IS NULL", id).
		Updates(map[string]interface{}{
			"metadata":   datatypes.JSON(metadataJSON),
			"updated_at": time.Now(),
		})
	if result.Error != nil {
		return fmt.Errorf("failed to update aichat conversation metadata: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (r *conversationRepository) DeleteScoped(ctx context.Context, id, organizationID, accountID uuid.UUID) error {
	now := time.Now()
	result := r.db.WithContext(ctx).Model(&runtimemodel.Conversation{}).
		Where("id = ? AND organization_id = ? AND account_id = ? AND deleted_at IS NULL", id, organizationID, accountID).
		Updates(map[string]interface{}{"deleted_at": now, "updated_at": now})
	if result.Error != nil {
		return fmt.Errorf("failed to delete aichat conversation: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (r *conversationRepository) StartStreaming(ctx context.Context, id, organizationID, accountID, messageID uuid.UUID) error {
	now := time.Now()
	result := r.db.WithContext(ctx).Model(&runtimemodel.Conversation{}).
		Where("id = ? AND organization_id = ? AND account_id = ? AND deleted_at IS NULL AND runtime_status <> ?", id, organizationID, accountID, runtimemodel.ConversationRuntimeStatusStreaming).
		Updates(map[string]interface{}{
			"runtime_status":    runtimemodel.ConversationRuntimeStatusStreaming,
			"active_message_id": messageID,
			"updated_at":        now,
		})
	if result.Error != nil {
		return fmt.Errorf("failed to start aichat conversation stream: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (r *conversationRepository) ClearActiveMessage(ctx context.Context, id, messageID uuid.UUID) error {
	result := r.db.WithContext(ctx).Model(&runtimemodel.Conversation{}).
		Where("id = ? AND active_message_id = ? AND deleted_at IS NULL", id, messageID).
		Updates(map[string]interface{}{
			"runtime_status":    runtimemodel.ConversationRuntimeStatusIdle,
			"active_message_id": nil,
			"updated_at":        time.Now(),
		})
	if result.Error != nil {
		return fmt.Errorf("failed to clear aichat conversation active message: %w", result.Error)
	}
	return nil
}

func (r *conversationRepository) FinishActiveMessage(ctx context.Context, id, messageID uuid.UUID) error {
	result := r.db.WithContext(ctx).Model(&runtimemodel.Conversation{}).
		Where("id = ? AND active_message_id = ? AND deleted_at IS NULL", id, messageID).
		UpdateColumns(map[string]interface{}{
			"runtime_status":    runtimemodel.ConversationRuntimeStatusIdle,
			"active_message_id": nil,
		})
	if result.Error != nil {
		return fmt.Errorf("failed to finish aichat conversation active message: %w", result.Error)
	}
	return nil
}

func (r *conversationRepository) FinishWaitingApprovalMessage(ctx context.Context, id, messageID uuid.UUID) error {
	result := r.db.WithContext(ctx).Model(&runtimemodel.Conversation{}).
		Where("id = ? AND active_message_id = ? AND deleted_at IS NULL", id, messageID).
		UpdateColumns(map[string]interface{}{
			"current_leaf_message_id": messageID,
			"runtime_status":          runtimemodel.ConversationRuntimeStatusIdle,
			"active_message_id":       nil,
			"dialogue_count":          gorm.Expr("dialogue_count + 1"),
			"updated_at":              time.Now(),
		})
	if result.Error != nil {
		return fmt.Errorf("failed to finish waiting approval aichat message: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (r *conversationRepository) FinishContinuationMessage(ctx context.Context, id, messageID uuid.UUID) error {
	result := r.db.WithContext(ctx).Model(&runtimemodel.Conversation{}).
		Where("id = ? AND active_message_id = ? AND deleted_at IS NULL", id, messageID).
		UpdateColumns(map[string]interface{}{
			"current_leaf_message_id": messageID,
			"runtime_status":          runtimemodel.ConversationRuntimeStatusIdle,
			"active_message_id":       nil,
			"updated_at":              time.Now(),
		})
	if result.Error != nil {
		return fmt.Errorf("failed to finish aichat continuation message: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (r *conversationRepository) ClearActiveMessages(ctx context.Context, messageIDs []uuid.UUID) error {
	if len(messageIDs) == 0 {
		return nil
	}
	result := r.db.WithContext(ctx).Model(&runtimemodel.Conversation{}).
		Where("active_message_id IN ? AND deleted_at IS NULL", messageIDs).
		UpdateColumns(map[string]interface{}{
			"runtime_status":    runtimemodel.ConversationRuntimeStatusIdle,
			"active_message_id": nil,
		})
	if result.Error != nil {
		return fmt.Errorf("failed to clear stale aichat conversation streams: %w", result.Error)
	}
	return nil
}

func (r *conversationRepository) CompleteRootReplacement(ctx context.Context, id, messageID uuid.UUID) error {
	result := r.db.WithContext(ctx).Model(&runtimemodel.Conversation{}).
		Where("id = ? AND active_message_id = ? AND deleted_at IS NULL", id, messageID).
		UpdateColumns(map[string]interface{}{
			"current_leaf_message_id": messageID,
			"runtime_status":          runtimemodel.ConversationRuntimeStatusIdle,
			"active_message_id":       nil,
			"dialogue_count":          1,
		})
	if result.Error != nil {
		return fmt.Errorf("failed to complete aichat root replacement: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (r *conversationRepository) UpdateAfterMessage(ctx context.Context, id uuid.UUID, leafMessageID uuid.UUID) error {
	result := r.db.WithContext(ctx).Model(&runtimemodel.Conversation{}).
		Where("id = ? AND deleted_at IS NULL", id).
		UpdateColumns(map[string]interface{}{
			"current_leaf_message_id": leafMessageID,
			"runtime_status":          runtimemodel.ConversationRuntimeStatusIdle,
			"active_message_id":       nil,
			"dialogue_count":          gorm.Expr("dialogue_count + 1"),
		})
	if result.Error != nil {
		return fmt.Errorf("failed to update aichat conversation after message: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (r *conversationRepository) RefreshAfterMessageDelete(ctx context.Context, id uuid.UUID) error {
	var latest runtimemodel.Message
	var leafID *uuid.UUID
	err := r.db.WithContext(ctx).
		Where("conversation_id = ? AND deleted_at IS NULL", id).
		Order("created_at DESC, updated_at DESC").
		Take(&latest).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return fmt.Errorf("failed to get latest aichat message: %w", err)
	}
	if err == nil {
		value := latest.ID
		leafID = &value
	}

	var count int64
	if err := r.db.WithContext(ctx).Model(&runtimemodel.Message{}).
		Where("conversation_id = ? AND deleted_at IS NULL", id).
		Count(&count).Error; err != nil {
		return fmt.Errorf("failed to count aichat messages after delete: %w", err)
	}

	result := r.db.WithContext(ctx).Model(&runtimemodel.Conversation{}).
		Where("id = ? AND deleted_at IS NULL", id).
		Updates(map[string]interface{}{
			"current_leaf_message_id": leafID,
			"dialogue_count":          count,
			"updated_at":              time.Now(),
		})
	if result.Error != nil {
		return fmt.Errorf("failed to refresh aichat conversation after delete: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

type messageRepository struct {
	db *gorm.DB
}

func NewMessageRepository(db *gorm.DB) MessageRepository {
	return &messageRepository{db: db}
}

func (r *messageRepository) Create(ctx context.Context, message *runtimemodel.Message) error {
	now := time.Now()
	if message.ID == uuid.Nil {
		message.ID = uuid.New()
	}
	if message.Status == "" {
		message.Status = runtimemodel.MessageStatusPending
	}
	if message.ModelParameters == nil {
		message.ModelParameters = map[string]interface{}{}
	}
	if message.Metadata == nil {
		message.Metadata = map[string]interface{}{}
	}
	message.CreatedAt = now
	message.UpdatedAt = now
	if err := r.db.WithContext(ctx).Create(message).Error; err != nil {
		return fmt.Errorf("failed to create aichat message: %w", err)
	}
	return nil
}

func (r *messageRepository) GetScoped(ctx context.Context, id, organizationID, accountID uuid.UUID) (*runtimemodel.Message, error) {
	var message runtimemodel.Message
	err := r.db.WithContext(ctx).Table("chat_runtime_messages AS m").
		Select("m.*").
		Joins("JOIN chat_runtime_conversations AS c ON c.id = m.conversation_id").
		Where("m.id = ? AND c.organization_id = ? AND c.account_id = ? AND m.deleted_at IS NULL AND c.deleted_at IS NULL", id, organizationID, accountID).
		Take(&message).Error
	if err != nil {
		return nil, wrapNotFound(err, "aichat message")
	}
	return &message, nil
}

func (r *messageRepository) GetRuntimeLogScoped(ctx context.Context, id, organizationID uuid.UUID, workspaceID *uuid.UUID, accountID uuid.UUID, callerType string, callerID *uuid.UUID, source string) (*runtimemodel.Message, error) {
	var message runtimemodel.Message
	query := applyRuntimeLogSourceFilter(applyRuntimeLogCallerFilter(r.db.WithContext(ctx).Table("chat_runtime_messages AS m").
		Select("m.*").
		Joins("JOIN chat_runtime_conversations AS c ON c.id = m.conversation_id").
		Where("m.id = ? AND c.organization_id = ? AND m.deleted_at IS NULL AND c.deleted_at IS NULL", id, organizationID), callerType, callerID, "c"), accountID, source, "c")
	if workspaceID != nil && *workspaceID != uuid.Nil {
		query = query.Where("c.workspace_id = ?", *workspaceID)
	}
	if err := query.Take(&message).Error; err != nil {
		return nil, wrapNotFound(err, "aichat message")
	}
	return &message, nil
}

func (r *messageRepository) GetBySourceMessage(ctx context.Context, sourceMessageID uuid.UUID) (*runtimemodel.Message, error) {
	var message runtimemodel.Message
	err := r.db.WithContext(ctx).
		Where("source_message_id = ? AND deleted_at IS NULL", sourceMessageID).
		Take(&message).Error
	if err != nil {
		return nil, wrapNotFound(err, "aichat message")
	}
	return &message, nil
}

func (r *messageRepository) ListByConversationScoped(ctx context.Context, conversationID, organizationID, accountID uuid.UUID, limit, offset int) ([]*runtimemodel.Message, int64, error) {
	var messages []*runtimemodel.Message
	var total int64
	query := r.db.WithContext(ctx).Table("chat_runtime_messages AS m").
		Joins("JOIN chat_runtime_conversations AS c ON c.id = m.conversation_id").
		Where("m.conversation_id = ? AND c.organization_id = ? AND c.account_id = ? AND m.deleted_at IS NULL AND c.deleted_at IS NULL", conversationID, organizationID, accountID)
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to count aichat messages: %w", err)
	}
	if err := query.Select("m.*").Order("m.created_at DESC, m.updated_at DESC").Limit(limit).Offset(offset).Find(&messages).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to list aichat messages: %w", err)
	}
	return messages, total, nil
}

func (r *messageRepository) ListByCallerScoped(ctx context.Context, organizationID, accountID uuid.UUID, callerType string, callerID *uuid.UUID, limit, offset int) ([]*runtimemodel.Message, int64, error) {
	var messages []*runtimemodel.Message
	var total int64
	query := applyCallerFilter(r.db.WithContext(ctx).Table("chat_runtime_messages AS m").
		Joins("JOIN chat_runtime_conversations AS c ON c.id = m.conversation_id").
		Where("c.organization_id = ? AND c.account_id = ? AND m.deleted_at IS NULL AND c.deleted_at IS NULL", organizationID, accountID), callerType, callerID)
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to count chat runtime messages: %w", err)
	}
	if err := query.Select("m.*").Order("m.created_at DESC, m.updated_at DESC").Limit(limit).Offset(offset).Find(&messages).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to list chat runtime messages: %w", err)
	}
	return messages, total, nil
}

func (r *messageRepository) ListByCallerSourceScoped(ctx context.Context, organizationID, accountID uuid.UUID, callerType string, callerID *uuid.UUID, source string, limit, offset int) ([]*runtimemodel.Message, int64, error) {
	var messages []*runtimemodel.Message
	var total int64
	query := applyCallerFilter(r.db.WithContext(ctx).Table("chat_runtime_messages AS m").
		Joins("JOIN chat_runtime_conversations AS c ON c.id = m.conversation_id").
		Where("c.organization_id = ? AND c.account_id = ? AND c.source = ? AND m.deleted_at IS NULL AND c.deleted_at IS NULL", organizationID, accountID, source), callerType, callerID)
	if source == runtimemodel.ConversationSourceWebApp {
		query = query.Where("c.source_web_app_id IS NOT NULL")
	}
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to count chat runtime messages by source: %w", err)
	}
	if err := query.Select("m.*").Order("m.created_at DESC, m.updated_at DESC").Limit(limit).Offset(offset).Find(&messages).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to list chat runtime messages by source: %w", err)
	}
	return messages, total, nil
}

func (r *messageRepository) ListByCallerLogFilterScoped(ctx context.Context, organizationID, accountID uuid.UUID, callerType string, callerID *uuid.UUID, source string, conversationID *uuid.UUID, queryText string, limit, offset int) ([]*runtimemodel.Message, int64, error) {
	var messages []*runtimemodel.Message
	var total int64
	query := applyCallerFilter(r.db.WithContext(ctx).Table("chat_runtime_messages AS m").
		Joins("JOIN chat_runtime_conversations AS c ON c.id = m.conversation_id").
		Where("c.organization_id = ? AND c.account_id = ? AND m.deleted_at IS NULL AND c.deleted_at IS NULL", organizationID, accountID), callerType, callerID)

	switch strings.TrimSpace(source) {
	case runtimemodel.ConversationSourceWebApp:
		query = query.Where("c.source = ? AND c.source_web_app_id IS NOT NULL", runtimemodel.ConversationSourceWebApp)
	case runtimemodel.ConversationSourceConsole:
		query = query.Where("c.source = ?", runtimemodel.ConversationSourceConsole)
	case "":
	default:
		query = query.Where("c.source = ?", source)
	}
	if conversationID != nil && *conversationID != uuid.Nil {
		query = query.Where("m.conversation_id = ?", *conversationID)
	}
	if keyword := strings.TrimSpace(queryText); keyword != "" {
		pattern := "%" + strings.ToLower(keyword) + "%"
		query = query.Where("(LOWER(COALESCE(m.query, '')) LIKE ? OR LOWER(COALESCE(m.answer, '')) LIKE ?)", pattern, pattern)
	}
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to count chat runtime messages by log filters: %w", err)
	}
	if err := query.Select("m.*").Order("m.created_at DESC, m.updated_at DESC").Limit(limit).Offset(offset).Find(&messages).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to list chat runtime messages by log filters: %w", err)
	}
	return messages, total, nil
}

func (r *messageRepository) ListByCallerRuntimeLogScoped(ctx context.Context, organizationID uuid.UUID, workspaceID *uuid.UUID, accountID uuid.UUID, callerType string, callerID *uuid.UUID, source string, conversationID *uuid.UUID, queryText string, limit, offset int) ([]*runtimemodel.Message, int64, error) {
	var messages []*runtimemodel.Message
	var total int64
	query := applyRuntimeLogSourceFilter(applyRuntimeLogCallerFilter(r.db.WithContext(ctx).Table("chat_runtime_messages AS m").
		Joins("JOIN chat_runtime_conversations AS c ON c.id = m.conversation_id").
		Where("c.organization_id = ? AND m.deleted_at IS NULL AND c.deleted_at IS NULL", organizationID), callerType, callerID, "c"), accountID, source, "c")
	if workspaceID != nil && *workspaceID != uuid.Nil {
		query = query.Where("c.workspace_id = ?", *workspaceID)
	}
	if conversationID != nil && *conversationID != uuid.Nil {
		query = query.Where("m.conversation_id = ?", *conversationID)
	}
	if keyword := strings.TrimSpace(queryText); keyword != "" {
		pattern := "%" + strings.ToLower(keyword) + "%"
		query = query.Where("(LOWER(COALESCE(m.query, '')) LIKE ? OR LOWER(COALESCE(m.answer, '')) LIKE ?)", pattern, pattern)
	}
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to count chat runtime messages by runtime log filters: %w", err)
	}
	if err := query.Select("m.*").Order("m.created_at DESC, m.updated_at DESC").Limit(limit).Offset(offset).Find(&messages).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to list chat runtime messages by runtime log filters: %w", err)
	}
	return messages, total, nil
}

func (r *messageRepository) ListBranch(ctx context.Context, leafID uuid.UUID, maxDepth int) ([]*runtimemodel.Message, error) {
	if leafID == uuid.Nil || maxDepth <= 0 {
		return []*runtimemodel.Message{}, nil
	}

	out := make([]*runtimemodel.Message, 0, maxDepth)
	seen := make(map[uuid.UUID]bool, maxDepth)
	currentID := leafID
	for len(out) < maxDepth && currentID != uuid.Nil {
		if seen[currentID] {
			return nil, fmt.Errorf("cycle detected in aichat message branch")
		}
		seen[currentID] = true

		var message runtimemodel.Message
		err := r.db.WithContext(ctx).
			Where("id = ? AND deleted_at IS NULL", currentID).
			Take(&message).Error
		if err != nil {
			return nil, wrapNotFound(err, "aichat message branch")
		}
		out = append(out, &message)
		if message.ParentID == nil {
			break
		}
		currentID = *message.ParentID
	}

	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out, nil
}

func (r *messageRepository) CountByConversation(ctx context.Context, conversationID uuid.UUID) (int64, error) {
	var count int64
	if err := r.db.WithContext(ctx).Model(&runtimemodel.Message{}).
		Where("conversation_id = ? AND deleted_at IS NULL", conversationID).
		Count(&count).Error; err != nil {
		return 0, fmt.Errorf("failed to count aichat conversation messages: %w", err)
	}
	return count, nil
}

func (r *messageRepository) ReplaceRootForStreaming(ctx context.Context, message *runtimemodel.Message) error {
	if message == nil {
		return fmt.Errorf("aichat message is required")
	}
	if message.ModelParameters == nil {
		message.ModelParameters = map[string]interface{}{}
	}
	if message.Metadata == nil {
		message.Metadata = map[string]interface{}{}
	}
	parametersJSON, err := json.Marshal(message.ModelParameters)
	if err != nil {
		return fmt.Errorf("failed to marshal aichat model parameters: %w", err)
	}
	metadataJSON, err := json.Marshal(message.Metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal aichat message metadata: %w", err)
	}
	result := r.db.WithContext(ctx).Model(&runtimemodel.Message{}).
		Where("id = ? AND deleted_at IS NULL AND parent_id IS NULL", message.ID).
		Updates(map[string]interface{}{
			"query":                 message.Query,
			"answer":                "",
			"status":                runtimemodel.MessageStatusStreaming,
			"error":                 nil,
			"model_provider":        message.ModelProvider,
			"model_name":            message.ModelName,
			"billing_reason_source": message.BillingReasonSource,
			"model_parameters":      datatypes.JSON(parametersJSON),
			"metadata":              datatypes.JSON(metadataJSON),
			"updated_at":            time.Now(),
		})
	if result.Error != nil {
		return fmt.Errorf("failed to prepare aichat root replacement: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (r *messageRepository) UpdateCompleted(ctx context.Context, id uuid.UUID, answer string, metadata map[string]interface{}) error {
	if metadata == nil {
		metadata = map[string]interface{}{}
	}
	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal aichat message metadata: %w", err)
	}
	result := r.db.WithContext(ctx).Model(&runtimemodel.Message{}).
		Where("id = ? AND deleted_at IS NULL AND status IN ?", id, completableMessageStatuses()).
		Updates(map[string]interface{}{
			"answer":     answer,
			"status":     runtimemodel.MessageStatusCompleted,
			"error":      nil,
			"metadata":   datatypes.JSON(metadataJSON),
			"updated_at": time.Now(),
		})
	if result.Error != nil {
		return fmt.Errorf("failed to complete aichat message: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (r *messageRepository) UpdateMetadata(ctx context.Context, id uuid.UUID, metadata map[string]interface{}) error {
	if metadata == nil {
		metadata = map[string]interface{}{}
	}
	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal aichat message metadata: %w", err)
	}
	result := r.db.WithContext(ctx).Model(&runtimemodel.Message{}).
		Where("id = ? AND deleted_at IS NULL AND status IN ?", id, mutableMessageStatuses()).
		Updates(map[string]interface{}{
			"metadata":   datatypes.JSON(metadataJSON),
			"updated_at": time.Now(),
		})
	if result.Error != nil {
		return fmt.Errorf("failed to update aichat message metadata: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (r *messageRepository) UpdateMetadataAnyStatus(ctx context.Context, id uuid.UUID, metadata map[string]interface{}) error {
	if metadata == nil {
		metadata = map[string]interface{}{}
	}
	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal aichat message metadata: %w", err)
	}
	result := r.db.WithContext(ctx).Model(&runtimemodel.Message{}).
		Where("id = ? AND deleted_at IS NULL", id).
		Updates(map[string]interface{}{
			"metadata":   datatypes.JSON(metadataJSON),
			"updated_at": time.Now(),
		})
	if result.Error != nil {
		return fmt.Errorf("failed to update aichat message metadata: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (r *messageRepository) UpdateWaitingApproval(ctx context.Context, id uuid.UUID, metadata map[string]interface{}) error {
	if metadata == nil {
		metadata = map[string]interface{}{}
	}
	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal waiting approval aichat message metadata: %w", err)
	}
	result := r.db.WithContext(ctx).Model(&runtimemodel.Message{}).
		Where("id = ? AND deleted_at IS NULL AND status IN ?", id, activeMessageStatuses()).
		Updates(map[string]interface{}{
			"status":     runtimemodel.MessageStatusWaitingApproval,
			"error":      nil,
			"metadata":   datatypes.JSON(metadataJSON),
			"updated_at": time.Now(),
		})
	if result.Error != nil {
		return fmt.Errorf("failed to mark aichat message waiting approval: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (r *messageRepository) UpdateWaitingQuestion(ctx context.Context, id uuid.UUID, metadata map[string]interface{}) error {
	if metadata == nil {
		metadata = map[string]interface{}{}
	}
	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal waiting question aichat message metadata: %w", err)
	}
	result := r.db.WithContext(ctx).Model(&runtimemodel.Message{}).
		Where("id = ? AND deleted_at IS NULL AND status IN ?", id, activeMessageStatuses()).
		Updates(map[string]interface{}{
			"status":     runtimemodel.MessageStatusWaitingQuestion,
			"error":      nil,
			"metadata":   datatypes.JSON(metadataJSON),
			"updated_at": time.Now(),
		})
	if result.Error != nil {
		return fmt.Errorf("failed to mark aichat message waiting question: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (r *messageRepository) UpdateWaitingClientAction(ctx context.Context, id uuid.UUID, metadata map[string]interface{}) error {
	if metadata == nil {
		metadata = map[string]interface{}{}
	}
	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal waiting client action aichat message metadata: %w", err)
	}
	result := r.db.WithContext(ctx).Model(&runtimemodel.Message{}).
		Where("id = ? AND deleted_at IS NULL AND status IN ?", id, activeMessageStatuses()).
		Updates(map[string]interface{}{
			"status":     runtimemodel.MessageStatusWaitingClientAction,
			"error":      nil,
			"metadata":   datatypes.JSON(metadataJSON),
			"updated_at": time.Now(),
		})
	if result.Error != nil {
		return fmt.Errorf("failed to mark aichat message waiting client action: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (r *messageRepository) UpdateError(ctx context.Context, id uuid.UUID, message string) error {
	result := r.db.WithContext(ctx).Model(&runtimemodel.Message{}).
		Where("id = ? AND deleted_at IS NULL AND status IN ?", id, activeMessageStatuses()).
		Updates(map[string]interface{}{
			"status":     runtimemodel.MessageStatusError,
			"error":      message,
			"updated_at": time.Now(),
		})
	if result.Error != nil {
		return fmt.Errorf("failed to mark aichat message error: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (r *messageRepository) MarkStopped(ctx context.Context, id uuid.UUID) error {
	result := r.db.WithContext(ctx).Model(&runtimemodel.Message{}).
		Where("id = ? AND deleted_at IS NULL AND status IN ?", id, activeMessageStatuses()).
		Updates(map[string]interface{}{
			"status":     runtimemodel.MessageStatusStopped,
			"error":      nil,
			"updated_at": time.Now(),
		})
	if result.Error != nil {
		return fmt.Errorf("failed to stop aichat message: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (r *messageRepository) UpdateStoppedAnswer(ctx context.Context, id uuid.UUID, answer string, metadata map[string]interface{}) error {
	if metadata == nil {
		metadata = map[string]interface{}{}
	}
	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal stopped aichat metadata: %w", err)
	}
	result := r.db.WithContext(ctx).Model(&runtimemodel.Message{}).
		Where("id = ? AND deleted_at IS NULL AND status IN ?", id, append(mutableMessageStatuses(), runtimemodel.MessageStatusStopped)).
		Updates(map[string]interface{}{
			"answer":     answer,
			"status":     runtimemodel.MessageStatusStopped,
			"error":      nil,
			"metadata":   datatypes.JSON(metadataJSON),
			"updated_at": time.Now(),
		})
	if result.Error != nil {
		return fmt.Errorf("failed to persist stopped aichat message: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (r *messageRepository) DeleteSubtreeScoped(ctx context.Context, id, organizationID, accountID uuid.UUID) (*MessageDeleteResult, error) {
	message, err := r.GetScoped(ctx, id, organizationID, accountID)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	result := r.db.WithContext(ctx).Exec(`
		WITH RECURSIVE subtree(id) AS (
			SELECT id
			FROM chat_runtime_messages
			WHERE id = ? AND conversation_id = ? AND deleted_at IS NULL
			UNION ALL
			SELECT child.id
			FROM chat_runtime_messages child
			JOIN subtree ON child.parent_id = subtree.id
			WHERE child.conversation_id = ? AND child.deleted_at IS NULL
		)
		UPDATE chat_runtime_messages
		SET deleted_at = ?, updated_at = ?
		WHERE id IN (SELECT id FROM subtree) AND deleted_at IS NULL
	`, id, message.ConversationID, message.ConversationID, now, now)
	if result.Error != nil {
		return nil, fmt.Errorf("failed to delete aichat message subtree: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return nil, gorm.ErrRecordNotFound
	}
	return &MessageDeleteResult{ConversationID: message.ConversationID, DeletedCount: result.RowsAffected}, nil
}

func (r *messageRepository) ListStaleActiveIDs(ctx context.Context, cutoff time.Time) ([]uuid.UUID, error) {
	var ids []uuid.UUID
	if err := r.db.WithContext(ctx).Model(&runtimemodel.Message{}).
		Where("deleted_at IS NULL AND status IN ? AND updated_at < ?", activeMessageStatuses(), cutoff).
		Pluck("id", &ids).Error; err != nil {
		return nil, fmt.Errorf("failed to list stale aichat active messages: %w", err)
	}
	return ids, nil
}

func (r *messageRepository) MarkStaleActiveAsError(ctx context.Context, cutoff time.Time, message string) (int64, error) {
	result := r.db.WithContext(ctx).Model(&runtimemodel.Message{}).
		Where("deleted_at IS NULL AND status IN ? AND updated_at < ?", activeMessageStatuses(), cutoff).
		Updates(map[string]interface{}{
			"status":     runtimemodel.MessageStatusError,
			"error":      message,
			"updated_at": time.Now(),
		})
	if result.Error != nil {
		return 0, fmt.Errorf("failed to mark stale aichat messages error: %w", result.Error)
	}
	return result.RowsAffected, nil
}

type accessRepository struct {
	db *gorm.DB
}

func NewAccessRepository(db *gorm.DB) AccessRepository {
	return &accessRepository{db: db}
}

func (r *accessRepository) IsOrganizationMember(ctx context.Context, organizationID, accountID uuid.UUID) (bool, error) {
	var count int64
	if err := r.db.WithContext(ctx).Table("members").
		Where("organization_id = ? AND account_id = ?", organizationID, accountID).
		Count(&count).Error; err != nil {
		return false, fmt.Errorf("failed to check organization membership: %w", err)
	}
	return count > 0, nil
}

func (r *accessRepository) GetCurrentWorkspaceID(ctx context.Context, accountID uuid.UUID) (*uuid.UUID, error) {
	var row struct {
		CurrentWorkspaceID *string `gorm:"column:current_workspace_id"`
	}
	if err := r.db.WithContext(ctx).Table("account_contexts").
		Select("current_workspace_id").
		Where("account_id = ?", accountID).
		Take(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get current workspace: %w", err)
	}
	if row.CurrentWorkspaceID == nil || *row.CurrentWorkspaceID == "" {
		return nil, nil
	}
	workspaceID, err := uuid.Parse(*row.CurrentWorkspaceID)
	if err != nil {
		return nil, nil
	}
	return &workspaceID, nil
}

func activeMessageStatuses() []string {
	return []string{runtimemodel.MessageStatusPending, runtimemodel.MessageStatusStreaming}
}

func mutableMessageStatuses() []string {
	return []string{
		runtimemodel.MessageStatusPending,
		runtimemodel.MessageStatusStreaming,
		runtimemodel.MessageStatusWaitingApproval,
		runtimemodel.MessageStatusWaitingQuestion,
		runtimemodel.MessageStatusWaitingClientAction,
	}
}

func completableMessageStatuses() []string {
	return mutableMessageStatuses()
}

func wrapNotFound(err error, name string) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return fmt.Errorf("%s not found: %w", name, gorm.ErrRecordNotFound)
	}
	return fmt.Errorf("failed to get %s: %w", name, err)
}
