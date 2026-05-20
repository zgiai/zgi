package conversation

import (
	"fmt"
	"github.com/zgiai/ginext/internal/modules/app/chat"
	"time"

	"github.com/google/uuid"
	shared_dto "github.com/zgiai/ginext/internal/dto"

	"gorm.io/gorm"
)

// ConversationService conversation service interface
type ConversationService interface {
	// Completion Conversations
	GetCompletionConversations(appID string, query ConversationListRequest) (*shared_dto.PaginationResult, error)
	GetCompletionConversationDetail(appID, conversationID string) (*ConversationDetailDTO, error)
	DeleteCompletionConversation(appID, conversationID string) error

	// Chat Conversations
	GetChatConversations(appID string, query ConversationListRequest) (*shared_dto.PaginationResult, error)
	GetChatConversationDetail(appID, conversationID string) (*ConversationDetailDTO, error)
	DeleteChatConversation(appID, conversationID string) error

	// Global Chat Conversations
	GetGlobalChatConversations(appID, accountID string, query ConversationGroupQuery) (*shared_dto.PaginationResult, error)
	UpdateGlobalChatConversation(appID string, req UpdateConversationRequest) error
	DeleteGlobalChatConversation(appID string, req DeleteConversationRequest) error

	// Conversation Groups
	CreateOrUpdateConversationGroup(appID, accountID string, req CreateConversationGroupRequest) (*ConversationGroupSuccessResponse, error)
	AddConversationToGroup(appID, accountID string, req UpdateConversationGroupRequest) error
	RemoveConversationFromGroup(appID string, req DeleteConversationFromGroupRequest) error
	DeleteConversationGroup(appID, groupID string) error
	GetConversationsByGroupID(appID, groupID string) (*shared_dto.PaginationResult, error)
	DeleteConversationMessages(appID, conversationID string) error
}

// conversationService conversation service implementation
type conversationService struct {
	repo ConversationRepository
}

// NewConversationService create conversation service instance
func NewConversationService(repo ConversationRepository) ConversationService {
	return &conversationService{repo: repo}
}

// GetCompletionConversations get completion conversation list
func (s *conversationService) GetCompletionConversations(appID string, query ConversationListRequest) (*shared_dto.PaginationResult, error) {
	// Set default values
	if query.Page == 0 {
		query.Page = 1
	}
	if query.Limit == 0 {
		query.Limit = 20
	}
	if query.AnnotationStatus == "" {
		query.AnnotationStatus = "all"
	}

	// Build query parameters
	params := ConversationQueryParams{
		Keyword:          query.Keyword,
		AnnotationStatus: query.AnnotationStatus,
		Page:             query.Page,
		Limit:            query.Limit,
	}

	// Parse time parameters
	if !query.Start.IsZero() {
		params.Start = &query.Start
	}
	if !query.End.IsZero() {
		params.End = &query.End
	}

	conversations, total, err := s.repo.GetCompletionConversations(appID, params)
	if err != nil {
		return nil, err
	}

	// Convert to DTO
	data := make([]ConversationDTO, len(conversations))
	for i, conv := range conversations {
		data[i] = s.toConversationDTO(conv)
	}

	return &shared_dto.PaginationResult{
		Items:      data,
		Total:      total,
		Page:       query.Page,
		PerPage:    query.Limit,
		TotalPages: int((total + int64(query.Limit) - 1) / int64(query.Limit)),
	}, nil
}

// GetCompletionConversationDetail get completion conversation details
func (s *conversationService) GetCompletionConversationDetail(appID, conversationID string) (*ConversationDetailDTO, error) {
	conversation, err := s.repo.GetConversationByID(appID, conversationID)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("conversation not found")
		}
		return nil, err
	}

	// Mark as read (should pass current user ID here, temporarily omitted)
	// s.repo.MarkConversationAsRead(appID, conversationID, accountID)

	return s.toConversationDetailDTO(*conversation), nil
}

// DeleteCompletionConversation delete completion conversation
func (s *conversationService) DeleteCompletionConversation(appID, conversationID string) error {
	// Check if conversation exists
	_, err := s.repo.GetConversationByID(appID, conversationID)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return fmt.Errorf("conversation not found")
		}
		return err
	}

	return s.repo.DeleteConversation(appID, conversationID)
}

// GetChatConversations get chat conversation list
func (s *conversationService) GetChatConversations(appID string, query ConversationListRequest) (*shared_dto.PaginationResult, error) {
	// Set default values
	if query.Page == 0 {
		query.Page = 1
	}
	if query.Limit == 0 {
		query.Limit = 20
	}
	if query.AnnotationStatus == "" {
		query.AnnotationStatus = "all"
	}
	if query.SortBy == "" {
		query.SortBy = "-updated_at"
	}

	// Build query parameters
	var messageCountGte *int
	if query.MessageCountGte > 0 {
		messageCountGte = &query.MessageCountGte
	}

	params := ConversationQueryParams{
		Keyword:          query.Keyword,
		AnnotationStatus: query.AnnotationStatus,
		MessageCountGte:  messageCountGte,
		Page:             query.Page,
		Limit:            query.Limit,
		SortBy:           query.SortBy,
	}

	// Parse time parameters
	if !query.Start.IsZero() {
		params.Start = &query.Start
	}
	if !query.End.IsZero() {
		params.End = &query.End
	}

	conversations, total, err := s.repo.GetChatConversations(appID, params)
	if err != nil {
		return nil, err
	}

	// Convert to DTO
	data := make([]ConversationWithSummaryDTO, len(conversations))
	for i, conv := range conversations {
		data[i] = s.toConversationWithSummaryDTO(conv)
	}

	return &shared_dto.PaginationResult{
		Items:      data,
		Total:      total,
		Page:       query.Page,
		PerPage:    query.Limit,
		TotalPages: int((total + int64(query.Limit) - 1) / int64(query.Limit)),
	}, nil
}

// GetChatConversationDetail get chat conversation detail
func (s *conversationService) GetChatConversationDetail(appID, conversationID string) (*ConversationDetailDTO, error) {
	conversation, err := s.repo.GetConversationByID(appID, conversationID)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("conversation not found")
		}
		return nil, err
	}

	return &ConversationDetailDTO{
		ID:            conversation.ID,
		Status:        conversation.Status,
		FromSource:    conversation.FromSource,
		FromEndUserID: conversation.FromEndUserID,
		FromAccountID: conversation.FromAccountID,
		CreatedAt:     conversation.CreatedAt,
		UpdatedAt:     conversation.UpdatedAt,
		Introduction:  conversation.Introduction,
		ModelConfig:   conversation.ModelConfig,
	}, nil
}

// DeleteChatConversation delete chat conversation
func (s *conversationService) DeleteChatConversation(appID, conversationID string) error {
	// Check if conversation exists
	_, err := s.repo.GetConversationByID(appID, conversationID)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return fmt.Errorf("conversation not found")
		}
		return err
	}

	return s.repo.DeleteConversation(appID, conversationID)
}

// GetGlobalChatConversations get global chat conversations
func (s *conversationService) GetGlobalChatConversations(appID, accountID string, query ConversationGroupQuery) (*shared_dto.PaginationResult, error) {
	// Set default values
	if query.Page == 0 {
		query.Page = 1
	}
	if query.Limit == 0 {
		query.Limit = 20
	}

	groups, total, err := s.repo.GetConversationGroups(appID, accountID, query.Page, query.Limit)
	if err != nil {
		return nil, err
	}

	if len(groups) == 0 {
		return &shared_dto.PaginationResult{
			Items:      []ConversationGroupDTO{},
			Total:      total,
			Page:       query.Page,
			PerPage:    query.Limit,
			TotalPages: int((total + int64(query.Limit) - 1) / int64(query.Limit)),
		}, nil
	}

	// Get conversations from all groups
	data := make([]ConversationGroupDTO, 0, len(groups))
	for _, group := range groups {
		conversations, err := s.repo.GetConversationsByGroupID(appID, group.GroupID)
		if err != nil {
			return nil, err
		}

		groupDTO := ConversationGroupDTO{
			GroupID:        group.GroupID,
			GroupName:      group.Name,
			GroupCreatedAt: group.CreatedAt.Format(time.RFC3339),
			GroupUpdatedAt: group.UpdatedAt.Format(time.RFC3339),
			Conversations:  make([]ConversationWithSummaryDTO, len(conversations)),
		}

		for i, conv := range conversations {
			groupDTO.Conversations[i] = s.toConversationWithSummaryDTO(conv)
		}

		data = append(data, groupDTO)
	}

	return &shared_dto.PaginationResult{
		Items:      data,
		Total:      total,
		Page:       query.Page,
		PerPage:    query.Limit,
		TotalPages: int((total + int64(query.Limit) - 1) / int64(query.Limit)),
	}, nil
}

// UpdateGlobalChatConversation update global chat conversation
func (s *conversationService) UpdateGlobalChatConversation(appID string, req UpdateConversationRequest) error {
	conversation, err := s.repo.GetConversationByID(appID, req.ConversationID)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return fmt.Errorf("conversation not found")
		}
		return err
	}

	conversation.ModelID = &req.ModelID
	conversation.ModelProvider = &req.ModelProvider

	return s.repo.UpdateConversation(conversation)
}

// DeleteGlobalChatConversation delete global chat conversation
func (s *conversationService) DeleteGlobalChatConversation(appID string, req DeleteConversationRequest) error {
	return s.repo.DeleteConversation(appID, req.ConversationID)
}

// CreateOrUpdateConversationGroup create or update conversation group
func (s *conversationService) CreateOrUpdateConversationGroup(appID, accountID string, req CreateConversationGroupRequest) (*ConversationGroupSuccessResponse, error) {
	var groupID string

	// If no GroupID provided, generate new UUID
	if req.GroupID == nil || *req.GroupID == "" {
		groupID = uuid.New().String()
	} else {
		groupID = *req.GroupID
		// If updating, delete existing group records first
		s.repo.DeleteConversationGroupsByGroupID(groupID)
	}

	groupName := ""
	if req.GroupName != nil {
		groupName = *req.GroupName
	}

	// Create conversation group record
	groups := make([]ConversationGroup, 0)

	if len(req.ConversationIDs) == 0 {
		// Create empty group
		group := ConversationGroup{
			AppID:          appID,
			GroupID:        groupID,
			ConversationID: nil,
			Name:           groupName,
			FromAccountID:  accountID,
		}
		groups = append(groups, group)
	} else {
		// Create group record for each conversation
		for _, conversationID := range req.ConversationIDs {
			group := ConversationGroup{
				AppID:          appID,
				GroupID:        groupID,
				ConversationID: &conversationID,
				Name:           groupName,
				FromAccountID:  accountID,
			}
			groups = append(groups, group)
		}
	}

	err := s.repo.BulkCreateConversationGroups(groups)
	if err != nil {
		return nil, err
	}

	return &ConversationGroupSuccessResponse{
		Message: "Conversation group updated successfully",
		GroupID: groupID,
	}, nil
}

// AddConversationToGroup add conversation to group
func (s *conversationService) AddConversationToGroup(appID, accountID string, req UpdateConversationGroupRequest) error {
	group := ConversationGroup{
		AppID:          appID,
		GroupID:        req.GroupID,
		ConversationID: &req.ConversationID,
		Name:           "",
		FromAccountID:  accountID,
	}

	if req.GroupName != nil {
		group.Name = *req.GroupName
	}

	return s.repo.CreateConversationGroup(&group)
}

// RemoveConversationFromGroup remove conversation from group
func (s *conversationService) RemoveConversationFromGroup(appID string, req DeleteConversationFromGroupRequest) error {
	rowsAffected, err := s.repo.DeleteConversationFromGroup(req.GroupID, req.ConversationID, appID)
	if err != nil {
		return err
	}

	if rowsAffected == 0 {
		return fmt.Errorf("conversation not found in group")
	}

	return nil
}

// DeleteConversationGroup delete conversation group
func (s *conversationService) DeleteConversationGroup(appID, groupID string) error {
	rowsAffected, err := s.repo.DeleteConversationGroup(appID, groupID)
	if err != nil {
		return err
	}

	if rowsAffected == 0 {
		return fmt.Errorf("conversation group not found")
	}

	return nil
}

// GetConversationsByGroupID get conversation list by group ID
func (s *conversationService) GetConversationsByGroupID(appID, groupID string) (*shared_dto.PaginationResult, error) {
	// Get group information
	group, err := s.repo.GetConversationGroupByID(appID, groupID)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("group not found")
		}
		return nil, err
	}

	// Get conversations in group
	conversations, err := s.repo.GetConversationsByGroupID(appID, groupID)
	if err != nil {
		return nil, err
	}

	// Build response
	groupDTO := ConversationGroupDTO{
		GroupID:        groupID,
		GroupName:      group.Name,
		GroupCreatedAt: group.CreatedAt.Format(time.RFC3339),
		GroupUpdatedAt: group.UpdatedAt.Format(time.RFC3339),
		Conversations:  make([]ConversationWithSummaryDTO, len(conversations)),
	}

	for i, conv := range conversations {
		groupDTO.Conversations[i] = s.toConversationWithSummaryDTO(conv)
	}

	return &shared_dto.PaginationResult{
		Items:      []ConversationGroupDTO{groupDTO},
		Total:      1,
		Page:       1,
		PerPage:    len(conversations),
		TotalPages: 1,
	}, nil
}

// DeleteConversationMessages delete all messages in conversation
func (s *conversationService) DeleteConversationMessages(appID, conversationID string) error {
	return s.repo.DeleteMessagesByConversationID(appID, conversationID)
}

// Helper method: convert to ConversationDTO
func (s *conversationService) toConversationDTO(conv chat.Conversation) ConversationDTO {
	conversationDTO := ConversationDTO{
		ID:                   conv.ID,
		Status:               conv.Status,
		FromSource:           conv.FromSource,
		FromEndUserID:        conv.FromEndUserID,
		FromEndUserSessionID: conv.FromEndUserSessionID,
		FromAccountID:        conv.FromAccountID,
		FromAccountName:      conv.FromAccountName,
		ReadAt:               conv.ReadAt,
		CreatedAt:            conv.CreatedAt,
		UpdatedAt:            conv.UpdatedAt,
		ModelConfig:          conv.ModelConfig,
	}

	// Handle FirstMessage
	if conv.FirstMessage != nil {
		conversationDTO.FirstMessage = &SimpleMessageDTO{
			Inputs: conv.FirstMessage.Inputs,
			Query:  conv.FirstMessage.Query,
			Answer: conv.FirstMessage.Answer,
		}
	}

	return conversationDTO
}

// Helper method: convert to ConversationWithSummaryDTO
func (s *conversationService) toConversationWithSummaryDTO(conv chat.Conversation) ConversationWithSummaryDTO {
	summary := conv.GetSummaryOrQuery()

	conversationDTO := ConversationWithSummaryDTO{
		ID:                   conv.ID,
		Status:               conv.Status,
		FromSource:           conv.FromSource,
		FromEndUserID:        conv.FromEndUserID,
		FromEndUserSessionID: conv.FromEndUserSessionID,
		FromAccountID:        conv.FromAccountID,
		FromAccountName:      conv.FromAccountName,
		Name:                 conv.Name,
		Summary:              summary,
		ReadAt:               conv.ReadAt,
		CreatedAt:            conv.CreatedAt,
		UpdatedAt:            conv.UpdatedAt,
		ModelProvider:        conv.ModelProvider,
		ModelID:              conv.ModelID,
		ModelConfig:          conv.ModelConfig,
	}

	// Handle FirstMessage
	if conv.FirstMessage != nil {
		var messageText *string
		if len(conv.FirstMessage.Message) > 0 {
			if textContent, ok := conv.FirstMessage.Message[0].(map[string]interface{}); ok {
				if text, exists := textContent["text"]; exists {
					if textStr, ok := text.(string); ok {
						messageText = &textStr
					}
				}
			}
		}

		conversationDTO.FirstMessage = &FirstMessageDTO{
			Inputs:  conv.FirstMessage.Inputs,
			Query:   conv.FirstMessage.Query,
			Message: messageText,
			Answer:  conv.FirstMessage.Answer,
		}
	}

	return conversationDTO
}

// Helper method: convert to ConversationDetailDTO
func (s *conversationService) toConversationDetailDTO(conv chat.Conversation) *ConversationDetailDTO {
	conversationDTO := &ConversationDetailDTO{
		ID:            conv.ID,
		Status:        conv.Status,
		FromSource:    conv.FromSource,
		FromEndUserID: conv.FromEndUserID,
		FromAccountID: conv.FromAccountID,
		CreatedAt:     conv.CreatedAt,
		UpdatedAt:     conv.UpdatedAt,
		Introduction:  conv.Introduction,
		ModelConfig:   conv.ModelConfig,
	}

	// Handle FirstMessage
	if conv.FirstMessage != nil {
		conversationDTO.FirstMessage = &MessageDetailDTO{
			ID:     conv.FirstMessage.ID,
			Inputs: conv.FirstMessage.Inputs,
			Query:  conv.FirstMessage.Query,
			Answer: conv.FirstMessage.Answer,
		}
	}

	return conversationDTO
}
