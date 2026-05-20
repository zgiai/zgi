package conversation

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/zgiai/ginext/internal/modules/app/chat"

	"gorm.io/gorm"

	auth_model "github.com/zgiai/ginext/internal/modules/user/auth/model"
)

type ConversationRepository interface {
	GetConversationByID(appID, conversationID string) (*chat.Conversation, error)
	GetByID(ctx context.Context, conversationID string) (*chat.Conversation, error)
	GetByIDAndUser(ctx context.Context, conversationID, appID string, user interface{}) (*chat.Conversation, error)
	UpdateConversation(conversation *chat.Conversation) error
	DeleteConversation(appID, conversationID string) error
	MarkConversationAsRead(appID, conversationID, accountID string) error
	GetMessageCountByConversationID(conversationID string) (int, error)

	GetCompletionConversations(appID string, params ConversationQueryParams) ([]chat.Conversation, int64, error)
	GetChatConversations(appID string, params ConversationQueryParams) ([]chat.Conversation, int64, error)

	// Extended conversation query methods
	GetCompletionConversationsPaginated(filters ConversationFilters) (*ConversationPaginationResult, error)
	GetChatConversationsPaginated(filters ConversationFilters) (*ConversationPaginationResult, error)
	GetConversationDetailWithMessages(appID, conversationID string) (*chat.Conversation, error)

	CreateConversationGroup(group *ConversationGroup) error
	GetConversationGroups(appID, accountID string, page, limit int) ([]ConversationGroup, int64, error)
	GetConversationGroupByID(appID, groupID string) (*ConversationGroup, error)
	GetConversationsByGroupID(appID, groupID string) ([]chat.Conversation, error)
	DeleteConversationGroup(appID, groupID string) (int64, error)
	DeleteConversationFromGroup(groupID, conversationID, appID string) (int64, error)
	DeleteAllConversationsByGroupID(appID, groupID string) (int64, error)
	BulkCreateConversationGroups(groups []ConversationGroup) error
	DeleteConversationGroupsByGroupID(groupID string) (int64, error)

	DeleteMessagesByConversationID(appID, conversationID string) error
}

type ConversationQueryParams struct {
	Keyword          string
	Start            *time.Time
	End              *time.Time
	AnnotationStatus string
	MessageCountGte  *int
	Page             int
	Limit            int
	SortBy           string
	AccountID        string
	Timezone         string
}

type conversationRepository struct {
	db *gorm.DB
}

func NewConversationRepository(db *gorm.DB) ConversationRepository {
	return &conversationRepository{db: db}
}

func (r *conversationRepository) GetConversationByID(appID, conversationID string) (*chat.Conversation, error) {
	var conversation chat.Conversation
	err := r.db.Where("id = ? AND agent_id = ? AND is_deleted = ?", conversationID, appID, false).First(&conversation).Error
	if err != nil {
		return nil, err
	}
	return &conversation, nil
}

func (r *conversationRepository) GetByID(ctx context.Context, conversationID string) (*chat.Conversation, error) {
	var conversation chat.Conversation
	err := r.db.WithContext(ctx).
		Where("id = ?", conversationID).
		Preload("Messages").
		First(&conversation).Error
	if err != nil {
		return nil, err
	}
	return &conversation, nil
}

func (r *conversationRepository) GetByIDAndUser(ctx context.Context, conversationID, appID string, user interface{}) (*chat.Conversation, error) {
	query := r.db.WithContext(ctx).Where("id = ? AND agent_id = ? AND is_deleted = ?", conversationID, appID, false)

	switch u := user.(type) {
	case *auth_model.Account:
		query = query.Where("from_source = ? AND from_account_id = ?", "console", u.ID)
	case *EndUser:
		query = query.Where("from_source = ? AND from_end_user_id = ?", "api", u.ID)
	default:
		return nil, fmt.Errorf("invalid user type")
	}

	var conversation chat.Conversation
	err := query.Preload("Messages").First(&conversation).Error

	if err != nil {
		return nil, err
	}
	return &conversation, nil
}

func (r *conversationRepository) UpdateConversation(conversation *chat.Conversation) error {
	return r.db.Save(conversation).Error
}

func (r *conversationRepository) DeleteConversation(appID, conversationID string) error {
	return r.db.Model(&chat.Conversation{}).
		Where("id = ? AND agent_id = ?", conversationID, appID).
		Update("is_deleted", true).Error
}

func (r *conversationRepository) MarkConversationAsRead(appID, conversationID, accountID string) error {
	now := time.Now()
	return r.db.Model(&chat.Conversation{}).
		Where("id = ? AND agent_id = ? AND read_at IS NULL", conversationID, appID).
		Updates(map[string]interface{}{
			"read_at":         &now,
			"read_account_id": &accountID,
		}).Error
}

func (r *conversationRepository) GetMessageCountByConversationID(conversationID string) (int, error) {
	var count int64
	err := r.db.Model(&chat.Message{}).Where("conversation_id = ?", conversationID).Count(&count).Error
	if err != nil {
		return 0, err
	}
	return int(count), nil
}

func (r *conversationRepository) GetCompletionConversationsPaginated(filters ConversationFilters) (*ConversationPaginationResult, error) {
	query := r.db.Model(&chat.Conversation{}).
		Select(`conversations.*,
		        COUNT(DISTINCT messages.id) as message_count,
		        end_users.session_id as end_user_session_id,
		        end_users.name as end_user_name,
		        end_users.is_anonymous as end_user_is_anonymous`).
		Where("conversations.agent_id = ? AND conversations.mode = ? AND conversations.is_deleted = ?",
			filters.AppID, "completion", false).
		Joins("LEFT JOIN messages ON messages.conversation_id = conversations.id").
		Joins("LEFT JOIN end_users ON end_users.id = conversations.from_end_user_id")

	query = r.applyConversationFilters(query, filters)

	query = query.Group("conversations.id, end_users.session_id, end_users.name, end_users.is_anonymous")
	query = r.applySorting(query, filters.SortBy)

	var total int64
	countQuery := r.db.Model(&chat.Conversation{}).
		Where("conversations.agent_id = ? AND conversations.mode = ? AND conversations.is_deleted = ?",
			filters.AppID, "completion", false)
	countQuery = r.applyConversationFiltersForCount(countQuery, filters)
	if err := countQuery.Count(&total).Error; err != nil {
		return nil, err
	}

	offset := (filters.Page - 1) * filters.Limit
	query = query.Offset(offset).Limit(filters.Limit)

	rows, err := query.Rows()
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var conversations []ConversationWithRelations
	for rows.Next() {
		var conv ConversationWithRelations
		var inputs string

		err := rows.Scan(
			&conv.ID, &conv.AppID, &inputs, &conv.Status,
			&conv.Introduction, &conv.CreatedAt, &conv.UpdatedAt,
			&conv.FromEndUserID, &conv.FromAccountID, &conv.FromSource,
			&conv.ReadAt, &conv.ReadAccountID, &conv.Summary,
			&conv.MessageCount, &conv.EndUserSessionID, &conv.EndUserName,
			&conv.EndUserIsAnonymous,
		)
		if err != nil {
			return nil, err
		}

		conv.Inputs = inputs
		conversations = append(conversations, conv)
	}

	hasMore := int64(filters.Page*filters.Limit) < total

	return &ConversationPaginationResult{
		Conversations: conversations,
		Total:         total,
		Page:          filters.Page,
		Limit:         filters.Limit,
		HasMore:       hasMore,
	}, nil
}

func (r *conversationRepository) GetChatConversationsPaginated(filters ConversationFilters) (*ConversationPaginationResult, error) {
	query := r.db.Model(&chat.Conversation{}).
		Select(`conversations.*,
		        COUNT(DISTINCT messages.id) as message_count,
		        COUNT(DISTINCT CASE WHEN message_feedbacks.rating = 'like' THEN message_feedbacks.id END) as like_count,
		        COUNT(DISTINCT CASE WHEN message_feedbacks.rating = 'dislike' THEN message_feedbacks.id END) as dislike_count,
		        COUNT(DISTINCT message_annotations.id) as annotation_count,
		        end_users.session_id as end_user_session_id,
		        end_users.name as end_user_name,
		        end_users.is_anonymous as end_user_is_anonymous`).
		Where("conversations.agent_id = ? AND conversations.mode IN ? AND conversations.is_deleted = ?",
			filters.AppID, []string{"chat", "agent_chat", "advanced_chat"}, false).
		Joins("LEFT JOIN messages ON messages.conversation_id = conversations.id").
		Joins("LEFT JOIN message_feedbacks ON message_feedbacks.message_id = messages.id").
		Joins("LEFT JOIN message_annotations ON message_annotations.message_id = messages.id").
		Joins("LEFT JOIN end_users ON end_users.id = conversations.from_end_user_id")

	query = r.applyConversationFilters(query, filters)

	query = query.Group("conversations.id, end_users.session_id, end_users.name, end_users.is_anonymous")
	query = r.applySorting(query, filters.SortBy)

	var total int64
	countQuery := r.db.Model(&chat.Conversation{}).
		Where("conversations.agent_id = ? AND conversations.mode IN ? AND conversations.is_deleted = ?",
			filters.AppID, []string{"chat", "agent_chat", "advanced_chat"}, false)
	countQuery = r.applyConversationFiltersForCount(countQuery, filters)
	if err := countQuery.Count(&total).Error; err != nil {
		return nil, err
	}

	offset := (filters.Page - 1) * filters.Limit
	query = query.Offset(offset).Limit(filters.Limit)

	rows, err := query.Rows()
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var conversations []ConversationWithRelations
	for rows.Next() {
		var conv ConversationWithRelations
		var inputs string
		var likeCount, dislikeCount, annotationCount int64

		err := rows.Scan(
			&conv.ID, &conv.AppID, &inputs, &conv.Status,
			&conv.Introduction, &conv.CreatedAt, &conv.UpdatedAt,
			&conv.FromEndUserID, &conv.FromAccountID, &conv.FromSource,
			&conv.ReadAt, &conv.ReadAccountID, &conv.Summary,
			&conv.MessageCount, &likeCount, &dislikeCount, &annotationCount,
			&conv.EndUserSessionID, &conv.EndUserName, &conv.EndUserIsAnonymous,
		)
		if err != nil {
			return nil, err
		}

		conv.Inputs = inputs
		conv.UserFeedbackStats = &UserFeedbackStats{
			Like:    likeCount,
			Dislike: dislikeCount,
		}
		conv.AdminFeedbackStats = &AdminFeedbackStats{
			Total: annotationCount,
		}
		conversations = append(conversations, conv)
	}

	hasMore := int64(filters.Page*filters.Limit) < total

	return &ConversationPaginationResult{
		Conversations: conversations,
		Total:         total,
		Page:          filters.Page,
		Limit:         filters.Limit,
		HasMore:       hasMore,
	}, nil
}

func (r *conversationRepository) GetConversationDetailWithMessages(appID, conversationID string) (*chat.Conversation, error) {
	// Query the agents_conversations table
	var agentConv AgentConversation
	err := r.db.Where("id = ? AND agent_id = ? AND deleted_at IS NULL", conversationID, appID).
		Preload("Messages", func(db *gorm.DB) *gorm.DB {
			return db.Order("created_at ASC")
		}).
		First(&agentConv).Error

	if err != nil {
		return nil, err
	}

	// Convert to chat.Conversation format
	return r.convertAgentConversationToChatConversation(&agentConv), nil
}

// convertAgentConversationToChatConversation converts AgentConversation to chat.Conversation
func (r *conversationRepository) convertAgentConversationToChatConversation(agentConv *AgentConversation) *chat.Conversation {
	// Convert messages
	messages := make([]chat.Message, len(agentConv.Messages))
	for i, agentMsg := range agentConv.Messages {
		messages[i] = chat.Message{
			ID:             agentMsg.ID.String(),
			ConversationID: agentMsg.ConversationID.String(),
			Query:          agentMsg.Query,
			Answer:         agentMsg.Answer,
			CreatedAt:      agentMsg.CreatedAt,
			// Other fields can be added as needed
		}
	}

	// Convert the conversation
	conv := &chat.Conversation{
		ID:        agentConv.ID.String(),
		AppID:     agentConv.AgentID.String(),
		Mode:      agentConv.Mode,
		Name:      agentConv.Name,
		Summary:   agentConv.Summary,
		Status:    agentConv.Status,
		CreatedAt: agentConv.CreatedAt,
		UpdatedAt: agentConv.UpdatedAt,
		Messages:  messages,
	}

	return conv
}

func (r *conversationRepository) applyConversationFilters(query *gorm.DB, filters ConversationFilters) *gorm.DB {
	if filters.Keyword != "" {
		keyword := "%" + filters.Keyword + "%"
		subquery := r.db.Model(&chat.Conversation{}).
			Select("conversations.id").
			Joins("LEFT JOIN messages ON messages.conversation_id = conversations.id").
			Joins("LEFT JOIN end_users ON end_users.id = conversations.from_end_user_id").
			Where(`messages.query ILIKE ? OR messages.answer ILIKE ? OR
			       conversations.name ILIKE ? OR conversations.introduction ILIKE ? OR
			       end_users.session_id ILIKE ?`,
				keyword, keyword, keyword, keyword, keyword)

		query = query.Where("conversations.id IN (?)", subquery)
	}

	if filters.Start != nil {
		if strings.Contains(filters.SortBy, "updated_at") {
			query = query.Where("conversations.updated_at >= ?", filters.Start)
		} else {
			query = query.Where("conversations.created_at >= ?", filters.Start)
		}
	}

	if filters.End != nil {
		if strings.Contains(filters.SortBy, "updated_at") {
			query = query.Where("conversations.updated_at < ?", filters.End)
		} else {
			query = query.Where("conversations.created_at < ?", filters.End)
		}
	}

	switch filters.AnnotationStatus {
	case AnnotationStatusAnnotated:
		query = query.Joins("INNER JOIN message_annotations ON message_annotations.conversation_id = conversations.id")
	case AnnotationStatusNotAnnotated:
		subquery := r.db.Model(&chat.MessageAnnotation{}).
			Select("DISTINCT conversation_id").
			Where("conversation_id IS NOT NULL")
		query = query.Where("conversations.id NOT IN (?)", subquery)
	}

	if filters.MessageCountGte > 0 {
		query = query.Having("COUNT(DISTINCT messages.id) >= ?", filters.MessageCountGte)
	}

	if filters.FromSource != "" {
		query = query.Where("conversations.from_source = ?", filters.FromSource)
	}

	if filters.FromEndUserID != nil {
		query = query.Where("conversations.from_end_user_id = ?", *filters.FromEndUserID)
	}

	if filters.FromAccountID != nil {
		query = query.Where("conversations.from_account_id = ?", *filters.FromAccountID)
	}

	return query
}

func (r *conversationRepository) applyConversationFiltersForCount(query *gorm.DB, filters ConversationFilters) *gorm.DB {
	if filters.Keyword != "" {
		keyword := "%" + filters.Keyword + "%"
		subquery := r.db.Model(&chat.Conversation{}).
			Select("conversations.id").
			Joins("LEFT JOIN messages ON messages.conversation_id = conversations.id").
			Joins("LEFT JOIN end_users ON end_users.id = conversations.from_end_user_id").
			Where(`messages.query ILIKE ? OR messages.answer ILIKE ? OR
			       conversations.name ILIKE ? OR conversations.introduction ILIKE ? OR
			       end_users.session_id ILIKE ?`,
				keyword, keyword, keyword, keyword, keyword)

		query = query.Where("conversations.id IN (?)", subquery)
	}

	if filters.Start != nil {
		if strings.Contains(filters.SortBy, "updated_at") {
			query = query.Where("conversations.updated_at >= ?", filters.Start)
		} else {
			query = query.Where("conversations.created_at >= ?", filters.Start)
		}
	}

	if filters.End != nil {
		if strings.Contains(filters.SortBy, "updated_at") {
			query = query.Where("conversations.updated_at < ?", filters.End)
		} else {
			query = query.Where("conversations.created_at < ?", filters.End)
		}
	}

	switch filters.AnnotationStatus {
	case AnnotationStatusAnnotated:
		query = query.Joins("INNER JOIN message_annotations ON message_annotations.conversation_id = conversations.id")
	case AnnotationStatusNotAnnotated:
		subquery := r.db.Model(&chat.MessageAnnotation{}).
			Select("DISTINCT conversation_id").
			Where("conversation_id IS NOT NULL")
		query = query.Where("conversations.id NOT IN (?)", subquery)
	}

	if filters.MessageCountGte > 0 {
		subquery := r.db.Model(&chat.Conversation{}).
			Select("conversations.id").
			Joins("LEFT JOIN messages ON messages.conversation_id = conversations.id").
			Group("conversations.id").
			Having("COUNT(messages.id) >= ?", filters.MessageCountGte)

		query = query.Where("conversations.id IN (?)", subquery)
	}

	if filters.FromSource != "" {
		query = query.Where("conversations.from_source = ?", filters.FromSource)
	}

	if filters.FromEndUserID != nil {
		query = query.Where("conversations.from_end_user_id = ?", *filters.FromEndUserID)
	}

	if filters.FromAccountID != nil {
		query = query.Where("conversations.from_account_id = ?", *filters.FromAccountID)
	}

	return query
}

func (r *conversationRepository) applySorting(query *gorm.DB, sortBy string) *gorm.DB {
	switch sortBy {
	case SortByCreatedAtAsc:
		return query.Order("conversations.created_at ASC")
	case SortByCreatedAtDesc:
		return query.Order("conversations.created_at DESC")
	case SortByUpdatedAtAsc:
		return query.Order("conversations.updated_at ASC")
	case SortByUpdatedAtDesc:
		return query.Order("conversations.updated_at DESC")
	default:
		return query.Order("conversations.updated_at DESC")
	}
}

// Legacy methods for backward compatibility
func (r *conversationRepository) GetCompletionConversations(appID string, params ConversationQueryParams) ([]chat.Conversation, int64, error) {
	query := r.db.Model(&chat.Conversation{}).
		Where("agent_id = ? AND mode = ? AND is_deleted = ?", appID, "completion", false)

	query = r.applyLegacyConversationFilters(query, params)

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	query = query.Order("created_at DESC").
		Offset((params.Page - 1) * params.Limit).
		Limit(params.Limit)

	var conversations []chat.Conversation
	err := query.Find(&conversations).Error
	return conversations, total, err
}

func (r *conversationRepository) GetChatConversations(appID string, params ConversationQueryParams) ([]chat.Conversation, int64, error) {
	query := r.db.Model(&chat.Conversation{}).
		Where("agent_id = ? AND is_deleted = ?", appID, false)

	query = r.applyLegacyConversationFilters(query, params)
	query = r.applyChatSorting(query, params.SortBy)

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	query = query.Offset((params.Page - 1) * params.Limit).
		Limit(params.Limit)

	var conversations []chat.Conversation
	err := query.Find(&conversations).Error
	return conversations, total, err
}

func (r *conversationRepository) applyLegacyConversationFilters(query *gorm.DB, params ConversationQueryParams) *gorm.DB {
	if params.Keyword != "" {
		keyword := "%" + params.Keyword + "%"
		query = query.Joins("LEFT JOIN messages ON messages.conversation_id = conversations.id").
			Where("messages.query ILIKE ? OR messages.answer ILIKE ? OR conversations.name ILIKE ? OR conversations.introduction ILIKE ?",
				keyword, keyword, keyword, keyword)
	}

	if params.Start != nil {
		if strings.Contains(params.SortBy, "updated_at") {
			query = query.Where("conversations.updated_at >= ?", params.Start)
		} else {
			query = query.Where("conversations.created_at >= ?", params.Start)
		}
	}

	if params.End != nil {
		if strings.Contains(params.SortBy, "updated_at") {
			query = query.Where("conversations.updated_at <= ?", params.End)
		} else {
			query = query.Where("conversations.created_at <= ?", params.End)
		}
	}

	switch params.AnnotationStatus {
	case "annotated":
		query = query.Joins("INNER JOIN message_annotations ON message_annotations.conversation_id = conversations.id")
	case "not_annotated":
		query = query.Where("conversations.id NOT IN (SELECT DISTINCT conversation_id FROM message_annotations WHERE conversation_id IS NOT NULL)")
	}

	if params.MessageCountGte != nil {
		query = query.Joins("LEFT JOIN messages ON messages.conversation_id = conversations.id").
			Group("conversations.id").
			Having("COUNT(messages.id) >= ?", *params.MessageCountGte)
	}

	return query
}

func (r *conversationRepository) applyChatSorting(query *gorm.DB, sortBy string) *gorm.DB {
	switch sortBy {
	case "created_at":
		return query.Order("conversations.created_at ASC")
	case "-created_at":
		return query.Order("conversations.created_at DESC")
	case "updated_at":
		return query.Order("conversations.updated_at ASC")
	case "-updated_at":
		return query.Order("conversations.updated_at DESC")
	default:
		return query.Order("conversations.created_at DESC")
	}
}

// Rest of the existing methods remain the same...
func (r *conversationRepository) CreateConversationGroup(group *ConversationGroup) error {
	return r.db.Create(group).Error
}

func (r *conversationRepository) GetConversationGroups(appID, accountID string, page, limit int) ([]ConversationGroup, int64, error) {
	subQuery := r.db.Model(&ConversationGroup{}).
		Select("group_id, MAX(created_at) as max_created_at").
		Where("agent_id = ? AND from_account_id = ?", appID, accountID).
		Group("group_id")

	query := r.db.Model(&ConversationGroup{}).
		Joins("INNER JOIN (?) as latest ON conversation_group.group_id = latest.group_id AND conversation_group.created_at = latest.max_created_at", subQuery).
		Order("conversation_group.created_at DESC")

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	query = query.Offset((page - 1) * limit).Limit(limit)

	var groups []ConversationGroup
	err := query.Find(&groups).Error
	return groups, total, err
}

func (r *conversationRepository) GetConversationGroupByID(appID, groupID string) (*ConversationGroup, error) {
	var group ConversationGroup
	err := r.db.Where("agent_id = ? AND group_id = ?", appID, groupID).
		Order("created_at DESC").
		First(&group).Error
	if err != nil {
		return nil, err
	}
	return &group, nil
}

func (r *conversationRepository) GetConversationsByGroupID(appID, groupID string) ([]chat.Conversation, error) {
	var conversationIDs []string

	err := r.db.Model(&ConversationGroup{}).
		Select("conversation_id").
		Where("agent_id = ? AND group_id = ? AND conversation_id IS NOT NULL", appID, groupID).
		Pluck("conversation_id", &conversationIDs).Error
	if err != nil {
		return nil, err
	}

	if len(conversationIDs) == 0 {
		return []chat.Conversation{}, nil
	}

	var conversations []chat.Conversation
	err = r.db.Where("id IN ? AND agent_id = ? AND is_deleted = ?", conversationIDs, appID, false).
		Order("created_at DESC").
		Find(&conversations).Error

	return conversations, err
}

func (r *conversationRepository) DeleteConversationGroup(appID, groupID string) (int64, error) {
	result := r.db.Where("agent_id = ? AND group_id = ?", appID, groupID).
		Delete(&ConversationGroup{})
	return result.RowsAffected, result.Error
}

func (r *conversationRepository) DeleteConversationFromGroup(groupID, conversationID, appID string) (int64, error) {
	result := r.db.Where("group_id = ? AND conversation_id = ? AND agent_id = ?", groupID, conversationID, appID).
		Delete(&ConversationGroup{})
	return result.RowsAffected, result.Error
}

func (r *conversationRepository) DeleteAllConversationsByGroupID(appID, groupID string) (int64, error) {
	result := r.db.Where("agent_id = ? AND group_id = ?", appID, groupID).
		Delete(&ConversationGroup{})
	return result.RowsAffected, result.Error
}

func (r *conversationRepository) BulkCreateConversationGroups(groups []ConversationGroup) error {
	return r.db.CreateInBatches(groups, 100).Error
}

func (r *conversationRepository) DeleteConversationGroupsByGroupID(groupID string) (int64, error) {
	result := r.db.Where("group_id = ?", groupID).Delete(&ConversationGroup{})
	return result.RowsAffected, result.Error
}

func (r *conversationRepository) DeleteMessagesByConversationID(appID, conversationID string) error {
	return r.db.Where("agent_id = ? AND conversation_id = ?", appID, conversationID).
		Delete(&chat.Message{}).Error
}

type EndUser struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}
