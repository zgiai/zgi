package conversation

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/ginext/internal/modules/app/common"
)

// AgentConversationService defines the interface for agent conversation business logic
type AgentConversationService interface {
	CreateConversation(ctx context.Context, req *CreateConversationRequest) (*AgentConversation, error)
	GetConversation(ctx context.Context, id uuid.UUID) (*AgentConversation, error)
	GetConversationByIDAndAgent(ctx context.Context, id, agentID uuid.UUID) (*AgentConversation, error)
	GetOrCreateConversation(ctx context.Context, agentID uuid.UUID, conversationID *uuid.UUID, userID uuid.UUID, fromSource string, createdBy *uuid.UUID) (*AgentConversation, error)
	UpdateConversation(ctx context.Context, conversation *AgentConversation) error
	DeleteConversation(ctx context.Context, id uuid.UUID, deletedBy uuid.UUID) error
	GetConversationsByAgent(ctx context.Context, agentID, userID uuid.UUID, versionUUID string, page, limit int) ([]*AgentConversation, int64, error)
	GetConversationHistoryByAgent(ctx context.Context, filter AgentConversationHistoryFilter) ([]*AgentConversation, int64, error)
	GetConversationsByUser(ctx context.Context, agentID uuid.UUID, fromSource string, userID uuid.UUID, limit, offset int) ([]*AgentConversation, int64, error)
	IncrementDialogueCount(ctx context.Context, conversationID uuid.UUID) error
	GetConversationWithMessages(ctx context.Context, id uuid.UUID, messageLimit int) (*AgentConversation, error)
	GenerateConversationName(ctx context.Context, query string) string
	UpdateConversationWebAppID(ctx context.Context, conversationID uuid.UUID, webAppID string) error
}

// CreateConversationRequest represents the request to create a conversation
type CreateConversationRequest struct {
	AgentID                 uuid.UUID              `json:"agent_id"`
	AgentConfigID           *uuid.UUID             `json:"agent_config_id,omitempty"`
	ModelProvider           *string                `json:"model_provider,omitempty"`
	OverrideModelConfigs    *string                `json:"override_model_configs,omitempty"`
	ModelVersionID          *string                `json:"model_version_id,omitempty"`
	Mode                    string                 `json:"mode"`
	Name                    string                 `json:"name"`
	Summary                 *string                `json:"summary,omitempty"`
	Inputs                  map[string]interface{} `json:"inputs"`
	Introduction            *string                `json:"introduction,omitempty"`
	SystemInstruction       *string                `json:"system_instruction,omitempty"`
	SystemInstructionTokens int                    `json:"system_instruction_tokens"`
	Status                  string                 `json:"status"`
	InvokeFrom              *string                `json:"invoke_from,omitempty"`
	WebAppID                *string                `json:"web_app_id,omitempty"` // Web application ID
	FromSource              string                 `json:"from_source"`
	FromEndUserID           *uuid.UUID             `json:"from_end_user_id,omitempty"`
	FromAccountID           *uuid.UUID             `json:"from_account_id,omitempty"`
	CreatedBy               *uuid.UUID             `json:"created_by,omitempty"`
}

// agentConversationService implements AgentConversationService
type agentConversationService struct {
	conversationRepo AgentConversationRepository
	messageRepo      AgentMessageRepository
}

// NewAgentConversationService creates a new AgentConversationService
func NewAgentConversationService(
	conversationRepo AgentConversationRepository,
	messageRepo AgentMessageRepository,
) AgentConversationService {
	return &agentConversationService{
		conversationRepo: conversationRepo,
		messageRepo:      messageRepo,
	}
}

// CreateConversation creates a new conversation
func (s *agentConversationService) CreateConversation(ctx context.Context, req *CreateConversationRequest) (*AgentConversation, error) {
	// Convert inputs map to JSON string
	inputsJSON, err := json.Marshal(req.Inputs)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal inputs: %w", err)
	}

	conversation := &AgentConversation{
		ID:                      uuid.New(),
		AgentID:                 req.AgentID,
		AgentConfigID:           req.AgentConfigID,
		ModelProvider:           req.ModelProvider,
		OverrideModelConfigs:    req.OverrideModelConfigs,
		ModelVersionID:          req.ModelVersionID,
		Mode:                    req.Mode,
		Name:                    req.Name,
		Summary:                 req.Summary,
		Inputs:                  string(inputsJSON),
		Introduction:            req.Introduction,
		SystemInstruction:       req.SystemInstruction,
		SystemInstructionTokens: req.SystemInstructionTokens,
		Status:                  req.Status,
		InvokeFrom:              req.InvokeFrom,
		WebAppID:                req.WebAppID,
		FromSource:              req.FromSource,
		FromEndUserID:           req.FromEndUserID,
		FromAccountID:           req.FromAccountID,
		DialogueCount:           0,
		CreatedBy:               req.CreatedBy,
	}

	err = s.conversationRepo.Create(ctx, conversation)
	if err != nil {
		return nil, fmt.Errorf("failed to create conversation: %w", err)
	}

	return conversation, nil
}

// GetConversation retrieves a conversation by ID
func (s *agentConversationService) GetConversation(ctx context.Context, id uuid.UUID) (*AgentConversation, error) {
	return s.conversationRepo.GetByID(ctx, id)
}

// GetConversationByIDAndAgent retrieves a conversation by ID and agent ID
func (s *agentConversationService) GetConversationByIDAndAgent(ctx context.Context, id, agentID uuid.UUID) (*AgentConversation, error) {
	return s.conversationRepo.GetByIDAndAgent(ctx, id, agentID)
}

// GetOrCreateConversation gets an existing conversation or creates a new one
func (s *agentConversationService) GetOrCreateConversation(ctx context.Context, agentID uuid.UUID, conversationID *uuid.UUID, userID uuid.UUID, fromSource string, createdBy *uuid.UUID) (*AgentConversation, error) {
	// If conversation ID is provided, try to get existing conversation
	if conversationID != nil && *conversationID != uuid.Nil {
		conversation, err := s.conversationRepo.GetByIDAndAgent(ctx, *conversationID, agentID)
		if err == nil {
			return conversation, nil
		}
		// If conversation not found, create a new one with the provided ID
	}

	// Create new conversation
	name := s.GenerateConversationName(ctx, "")
	inputs := map[string]interface{}{
		"query": "",
		"files": []interface{}{},
	}

	req := &CreateConversationRequest{
		AgentID:    agentID,
		Mode:       "advanced-chat",
		Name:       name,
		Inputs:     inputs,
		Status:     "normal",
		InvokeFrom: stringPtr(string(common.InvokeFromWorkflow)),
		FromSource: fromSource,
		CreatedBy:  createdBy,
	}

	// Set user ID based on source
	if fromSource == "end_user" {
		req.FromEndUserID = &userID
	} else if fromSource == "account" {
		req.FromAccountID = &userID
	}

	conversation, err := s.CreateConversation(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to create new conversation: %w", err)
	}

	// If a specific conversation ID was requested, update the created conversation
	if conversationID != nil && *conversationID != uuid.Nil {
		conversation.ID = *conversationID
		err = s.conversationRepo.Update(ctx, conversation)
		if err != nil {
			return nil, fmt.Errorf("failed to update conversation ID: %w", err)
		}
	}

	return conversation, nil
}

// UpdateConversation updates a conversation
func (s *agentConversationService) UpdateConversation(ctx context.Context, conversation *AgentConversation) error {
	return s.conversationRepo.Update(ctx, conversation)
}

// DeleteConversation soft deletes a conversation
func (s *agentConversationService) DeleteConversation(ctx context.Context, id uuid.UUID, deletedBy uuid.UUID) error {
	return s.conversationRepo.Delete(ctx, id, deletedBy)
}

// GetConversationsByAgent retrieves conversations by agent with pagination and filters
func (s *agentConversationService) GetConversationsByAgent(ctx context.Context, agentID, userID uuid.UUID, versionUUID string, page, limit int) ([]*AgentConversation, int64, error) {
	offset := (page - 1) * limit
	return s.conversationRepo.GetByAgentWithFilters(ctx, agentID, userID, versionUUID, limit, offset)
}

// GetConversationHistoryByAgent retrieves agent conversations for console history views.
func (s *agentConversationService) GetConversationHistoryByAgent(ctx context.Context, filter AgentConversationHistoryFilter) ([]*AgentConversation, int64, error) {
	return s.conversationRepo.GetHistoryByAgent(ctx, filter)
}

// GetConversationsByUser retrieves conversations by user with pagination
func (s *agentConversationService) GetConversationsByUser(ctx context.Context, agentID uuid.UUID, fromSource string, userID uuid.UUID, limit, offset int) ([]*AgentConversation, int64, error) {
	return s.conversationRepo.GetByAgentAndUser(ctx, agentID, fromSource, userID, limit, offset)
}

// IncrementDialogueCount increments the dialogue count for a conversation
func (s *agentConversationService) IncrementDialogueCount(ctx context.Context, conversationID uuid.UUID) error {
	return s.conversationRepo.IncrementDialogueCount(ctx, conversationID)
}

// GetConversationWithMessages retrieves a conversation with its messages
func (s *agentConversationService) GetConversationWithMessages(ctx context.Context, id uuid.UUID, messageLimit int) (*AgentConversation, error) {
	return s.conversationRepo.GetWithMessages(ctx, id, messageLimit)
}

// GenerateConversationName generates a name for a new conversation
func (s *agentConversationService) GenerateConversationName(ctx context.Context, query string) string {
	if query != "" && len(query) > 0 {
		// Use first 50 characters of query as name
		if len(query) > 50 {
			return query[:50] + "..."
		}
		return query
	}

	// Generate default name with timestamp
	return fmt.Sprintf("Conversation %s", time.Now().Format("2006-01-02 15:04:05"))
}

// UpdateConversationWebAppID updates the web_app_id for an existing conversation
func (s *agentConversationService) UpdateConversationWebAppID(ctx context.Context, conversationID uuid.UUID, webAppID string) error {
	return s.conversationRepo.UpdateWebAppID(ctx, conversationID, webAppID)
}

// Helper function to create string pointer
func stringPtr(s string) *string {
	return &s
}
