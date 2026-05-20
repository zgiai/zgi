package conversation

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/pkg/logger"
)

// AgentMessageService defines the interface for agent message business logic
type AgentMessageService interface {
	CreateMessage(ctx context.Context, req *CreateMessageRequest) (*AgentMessage, error)
	GetMessage(ctx context.Context, id uuid.UUID) (*AgentMessage, error)
	GetMessagesByConversation(ctx context.Context, conversationID uuid.UUID, limit, offset int) ([]*AgentMessage, int64, error)
	UpdateMessage(ctx context.Context, message *AgentMessage) error
	DeleteMessage(ctx context.Context, id uuid.UUID, deletedBy uuid.UUID) error
	GetMessagesByUser(ctx context.Context, agentID uuid.UUID, fromSource string, userID uuid.UUID, limit, offset int) ([]*AgentMessage, int64, error)
	GetLatestMessageByConversation(ctx context.Context, conversationID uuid.UUID) (*AgentMessage, error)
	GetMessagesByWorkflowRun(ctx context.Context, workflowRunID uuid.UUID) ([]*AgentMessage, error)
	GetFirstMessagesByWorkflowRunIDs(ctx context.Context, workflowRunIDs []string) (map[string]*AgentMessage, error)
	UpdateMessageStatus(ctx context.Context, id uuid.UUID, status string, error *string) error
	CreateWorkflowMessage(ctx context.Context, req *CreateWorkflowMessageRequest) (*AgentMessage, error)
	GetConversationMessages(ctx context.Context, conversationID uuid.UUID) ([]*AgentMessage, error)
}

// CreateMessageRequest represents the request to create a message
type CreateMessageRequest struct {
	AgentID                 uuid.UUID              `json:"agent_id"`
	ModelProvider           *string                `json:"model_provider,omitempty"`
	ModelVersionID          *string                `json:"model_version_id,omitempty"`
	OverrideModelConfigs    *string                `json:"override_model_configs,omitempty"`
	ConversationID          uuid.UUID              `json:"conversation_id"`
	Inputs                  map[string]interface{} `json:"inputs"`
	Query                   string                 `json:"query"`
	Message                 []interface{}          `json:"message"`
	MessageTokens           int                    `json:"message_tokens"`
	MessageUnitPrice        *float64               `json:"message_unit_price,omitempty"`
	MessagePriceUnit        *float64               `json:"message_price_unit,omitempty"`
	Answer                  string                 `json:"answer"`
	AnswerTokens            int                    `json:"answer_tokens"`
	AnswerUnitPrice         *float64               `json:"answer_unit_price,omitempty"`
	AnswerPriceUnit         *float64               `json:"answer_price_unit,omitempty"`
	ParentMessageID         *uuid.UUID             `json:"parent_message_id,omitempty"`
	ProviderResponseLatency float64                `json:"provider_response_latency"`
	Currency                string                 `json:"currency"`
	Status                  string                 `json:"status"`
	Error                   *string                `json:"error,omitempty"`
	MessageMetadata         map[string]interface{} `json:"message_metadata,omitempty"`
	InvokeFrom              *string                `json:"invoke_from,omitempty"`
	FromSource              string                 `json:"from_source"`
	FromEndUserID           *uuid.UUID             `json:"from_end_user_id,omitempty"`
	FromAccountID           *uuid.UUID             `json:"from_account_id,omitempty"`
	CreatedBy               *uuid.UUID             `json:"created_by,omitempty"`
	AgentBased              bool                   `json:"agent_based"`
	WorkflowRunID           *uuid.UUID             `json:"workflow_run_id,omitempty"`
	WebAppID                *string                `json:"web_app_id,omitempty"` // Web application ID
}

// CreateWorkflowMessageRequest represents the request to create a workflow message
type CreateWorkflowMessageRequest struct {
	AgentID         uuid.UUID              `json:"agent_id"`
	ConversationID  uuid.UUID              `json:"conversation_id"`
	Query           string                 `json:"query"`
	Answer          string                 `json:"answer"`
	Status          string                 `json:"status,omitempty"`
	WorkflowRunID   uuid.UUID              `json:"workflow_run_id"`
	FromSource      string                 `json:"from_source"`
	FromUserID      uuid.UUID              `json:"from_user_id"`
	InvokeFrom      string                 `json:"invoke_from"`
	CreatedBy       *uuid.UUID             `json:"created_by,omitempty"`
	ParentMessageID *uuid.UUID             `json:"parent_message_id,omitempty"`
	WebAppID        *string                `json:"web_app_id,omitempty"`
	Inputs          map[string]interface{} `json:"inputs,omitempty"`
}

// agentMessageService implements AgentMessageService
type agentMessageService struct {
	messageRepo      AgentMessageRepository
	conversationRepo AgentConversationRepository
}

// NewAgentMessageService creates a new AgentMessageService
func NewAgentMessageService(
	messageRepo AgentMessageRepository,
	conversationRepo AgentConversationRepository,
) AgentMessageService {
	return &agentMessageService{
		messageRepo:      messageRepo,
		conversationRepo: conversationRepo,
	}
}

// CreateMessage creates a new message
func (s *agentMessageService) CreateMessage(ctx context.Context, req *CreateMessageRequest) (*AgentMessage, error) {
	logger.DebugContext(ctx, "creating agent message",
		"agent_id", req.AgentID.String(),
		"conversation_id", req.ConversationID.String(),
		"query_length", len(req.Query),
		"answer_length", len(req.Answer),
		"message_count", len(req.Message),
	)

	// Convert inputs map to JSON string
	inputsJSON, err := json.Marshal(req.Inputs)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal inputs: %w", err)
	}

	// Convert message array to JSON string
	messageJSON, err := json.Marshal(req.Message)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal message: %w", err)
	}

	// Convert metadata map to JSON string
	var metadataJSON *string
	if req.MessageMetadata != nil {
		metadataBytes, err := json.Marshal(req.MessageMetadata)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal message metadata: %w", err)
		}
		metadataStr := string(metadataBytes)
		metadataJSON = &metadataStr
	}

	message := &AgentMessage{
		ID:                      uuid.New(),
		AgentID:                 req.AgentID,
		ModelProvider:           req.ModelProvider,
		ModelVersionID:          req.ModelVersionID,
		OverrideModelConfigs:    req.OverrideModelConfigs,
		ConversationID:          req.ConversationID,
		Inputs:                  string(inputsJSON),
		Query:                   req.Query,
		Message:                 string(messageJSON),
		MessageTokens:           req.MessageTokens,
		MessageUnitPrice:        req.MessageUnitPrice,
		MessagePriceUnit:        req.MessagePriceUnit,
		Answer:                  req.Answer,
		AnswerTokens:            req.AnswerTokens,
		AnswerUnitPrice:         req.AnswerUnitPrice,
		AnswerPriceUnit:         req.AnswerPriceUnit,
		ParentMessageID:         req.ParentMessageID,
		ProviderResponseLatency: req.ProviderResponseLatency,
		Currency:                req.Currency,
		Status:                  messageStatus(req.Status),
		Error:                   req.Error,
		MessageMetadata:         metadataJSON,
		InvokeFrom:              req.InvokeFrom,
		FromSource:              req.FromSource,
		FromEndUserID:           req.FromEndUserID,
		FromAccountID:           req.FromAccountID,
		CreatedBy:               req.CreatedBy,
		AgentBased:              req.AgentBased,
		WorkflowRunID:           req.WorkflowRunID,
		WebAppID:                req.WebAppID,
	}

	// Calculate total price
	message.CalculateTotalPrice()

	err = s.messageRepo.Create(ctx, message)
	if err != nil {
		return nil, fmt.Errorf("failed to create message: %w", err)
	}

	// Increment dialogue count in conversation
	err = s.conversationRepo.IncrementDialogueCount(ctx, req.ConversationID)
	if err != nil {
		// Log error but don't fail the message creation
		logger.WarnContext(ctx, "failed to increment dialogue count", "conversation_id", req.ConversationID.String(), err)
	}

	return message, nil
}

// CreateWorkflowMessage creates a message specifically for workflow execution
func (s *agentMessageService) CreateWorkflowMessage(ctx context.Context, req *CreateWorkflowMessageRequest) (*AgentMessage, error) {
	logger.DebugContext(ctx, "creating workflow message",
		"agent_id", req.AgentID.String(),
		"conversation_id", req.ConversationID.String(),
		"workflow_run_id", req.WorkflowRunID.String(),
		"query_length", len(req.Query),
		"answer_length", len(req.Answer),
	)

	// Use provided inputs if available, otherwise create basic inputs
	var inputs map[string]interface{}
	if req.Inputs != nil && len(req.Inputs) > 0 {
		inputs = req.Inputs
		// Ensure query is set in inputs
		if _, hasQuery := inputs["query"]; !hasQuery {
			inputs["query"] = req.Query
		}
	} else {
		// Create basic inputs for workflow message
		inputs = map[string]interface{}{
			"query": req.Query,
			"files": []interface{}{},
		}
	}

	messages := []interface{}{
		map[string]interface{}{
			"role":    "user",
			"content": req.Query,
		},
		map[string]interface{}{
			"role":    "assistant",
			"content": req.Answer,
		},
	}

	// Determine user fields based on source
	var fromEndUserID, fromAccountID *uuid.UUID
	if req.FromSource == "end_user" {
		fromEndUserID = &req.FromUserID
	} else if req.FromSource == "account" {
		fromAccountID = &req.FromUserID
	}

	// Set default price values (required by database NOT NULL constraint)
	defaultMessageUnitPrice := 0.0
	defaultAnswerUnitPrice := 0.0
	defaultPriceUnit := 0.001

	createReq := &CreateMessageRequest{
		AgentID:          req.AgentID,
		ConversationID:   req.ConversationID,
		Inputs:           inputs,
		Query:            req.Query,
		Message:          messages,
		Answer:           req.Answer,
		MessageUnitPrice: &defaultMessageUnitPrice,
		MessagePriceUnit: &defaultPriceUnit,
		AnswerUnitPrice:  &defaultAnswerUnitPrice,
		AnswerPriceUnit:  &defaultPriceUnit,
		Currency:         "USD",
		Status:           workflowMessageStatus(req.Status),
		InvokeFrom:       stringPtr(req.InvokeFrom),
		FromSource:       req.FromSource,
		FromEndUserID:    fromEndUserID,
		FromAccountID:    fromAccountID,
		CreatedBy:        req.CreatedBy,
		AgentBased:       true,
		WorkflowRunID:    &req.WorkflowRunID,
		ParentMessageID:  req.ParentMessageID,
		WebAppID:         req.WebAppID,
	}

	msg, err := s.CreateMessage(ctx, createReq)
	if err != nil {
		logger.ErrorContext(ctx, "failed to create workflow message", "workflow_run_id", req.WorkflowRunID.String(), err)
		return nil, fmt.Errorf("failed to create message: %w", err)
	}
	logger.DebugContext(ctx, "workflow message created", "message_id", msg.ID.String(), "workflow_run_id", req.WorkflowRunID.String())

	return msg, nil
}

func messageStatus(status string) string {
	if status != "" {
		return status
	}
	return AgentMessageStatusNormal
}

func workflowMessageStatus(status string) string {
	if status != "" {
		return status
	}
	return AgentMessageStatusCompleted
}

// GetMessage retrieves a message by ID
func (s *agentMessageService) GetMessage(ctx context.Context, id uuid.UUID) (*AgentMessage, error) {
	return s.messageRepo.GetByID(ctx, id)
}

// GetMessagesByConversation retrieves messages by conversation ID with pagination
func (s *agentMessageService) GetMessagesByConversation(ctx context.Context, conversationID uuid.UUID, limit, offset int) ([]*AgentMessage, int64, error) {
	return s.messageRepo.GetByConversationID(ctx, conversationID, limit, offset)
}

// UpdateMessage updates a message
func (s *agentMessageService) UpdateMessage(ctx context.Context, message *AgentMessage) error {
	return s.messageRepo.Update(ctx, message)
}

// DeleteMessage soft deletes a message
func (s *agentMessageService) DeleteMessage(ctx context.Context, id uuid.UUID, deletedBy uuid.UUID) error {
	return s.messageRepo.Delete(ctx, id, deletedBy)
}

// GetMessagesByUser retrieves messages by user with pagination
func (s *agentMessageService) GetMessagesByUser(ctx context.Context, agentID uuid.UUID, fromSource string, userID uuid.UUID, limit, offset int) ([]*AgentMessage, int64, error) {
	return s.messageRepo.GetByAgentAndUser(ctx, agentID, fromSource, userID, limit, offset)
}

// GetLatestMessageByConversation retrieves the latest message in a conversation
func (s *agentMessageService) GetLatestMessageByConversation(ctx context.Context, conversationID uuid.UUID) (*AgentMessage, error) {
	return s.messageRepo.GetLatestByConversation(ctx, conversationID)
}

// GetMessagesByWorkflowRun retrieves messages by workflow run ID
func (s *agentMessageService) GetMessagesByWorkflowRun(ctx context.Context, workflowRunID uuid.UUID) ([]*AgentMessage, error) {
	return s.messageRepo.GetByWorkflowRunID(ctx, workflowRunID)
}

// GetFirstMessagesByWorkflowRunIDs retrieves the earliest message for each workflow run ID.
func (s *agentMessageService) GetFirstMessagesByWorkflowRunIDs(ctx context.Context, workflowRunIDs []string) (map[string]*AgentMessage, error) {
	return s.messageRepo.GetFirstByWorkflowRunIDs(ctx, workflowRunIDs)
}

// UpdateMessageStatus updates the status and error of a message
func (s *agentMessageService) UpdateMessageStatus(ctx context.Context, id uuid.UUID, status string, error *string) error {
	return s.messageRepo.UpdateStatus(ctx, id, status, error)
}

// GetConversationMessages retrieves all messages for a conversation (ordered by creation time)
func (s *agentMessageService) GetConversationMessages(ctx context.Context, conversationID uuid.UUID) ([]*AgentMessage, error) {
	messages, _, err := s.messageRepo.GetByConversationID(ctx, conversationID, 10000, 0)
	return messages, err
}
