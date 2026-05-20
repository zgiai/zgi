package workflow

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/zgiai/ginext/internal/modules/app/conversation"
	"github.com/zgiai/ginext/pkg/database"
	"github.com/zgiai/ginext/pkg/logger"
	"go.uber.org/zap"
)

// AdvancedChatWorkflowHandler handles advanced chat workflow operations with new data models
type AdvancedChatWorkflowHandler struct {
	conversationService conversation.AgentConversationService
	messageService      conversation.AgentMessageService
	variableService     conversation.WorkflowConversationVariableService
}

// NewAdvancedChatWorkflowHandler creates a new AdvancedChatWorkflowHandler
func NewAdvancedChatWorkflowHandler() *AdvancedChatWorkflowHandler {
	db := database.GetDB()

	// Create repositories
	conversationRepo := conversation.NewAgentConversationRepository(db)
	messageRepo := conversation.NewAgentMessageRepository(db)
	variableRepo := conversation.NewWorkflowConversationVariableRepository(db)

	// Create services
	conversationService := conversation.NewAgentConversationService(conversationRepo, messageRepo)
	messageService := conversation.NewAgentMessageService(messageRepo, conversationRepo)
	variableService := conversation.NewWorkflowConversationVariableService(variableRepo)

	return &AdvancedChatWorkflowHandler{
		conversationService: conversationService,
		messageService:      messageService,
		variableService:     variableService,
	}
}

// CreateConversationRecord creates a new conversation record using the new data models
func (h *AdvancedChatWorkflowHandler) CreateConversationRecord(tenantID, agentID, accountID string) (string, error) {
	ctx := logger.WithFields(context.Background(),
		zap.String("tenant_id", tenantID),
		zap.String("agent_id", agentID),
		zap.String("account_id", accountID),
	)
	logger.DebugContext(ctx, "creating conversation record")

	// Parse UUIDs
	agentUUID, err := uuid.Parse(agentID)
	if err != nil {
		logger.WarnContext(ctx, "invalid agent id", zap.Error(err))
		return "", fmt.Errorf("invalid agent ID: %w", err)
	}

	accountUUID, err := uuid.Parse(accountID)
	if err != nil {
		logger.WarnContext(ctx, "invalid account id", zap.Error(err))
		return "", fmt.Errorf("invalid account ID: %w", err)
	}

	// Create conversation using the service
	conv, err := h.conversationService.GetOrCreateConversation(
		ctx,
		agentUUID,
		nil, // No specific conversation ID
		accountUUID,
		"account",    // From source is account for console API
		&accountUUID, // Created by account user
	)

	if err != nil {
		logger.ErrorContext(ctx, "failed to create conversation", zap.Error(err))
		return "", fmt.Errorf("failed to create conversation record: %w", err)
	}

	logger.DebugContext(ctx, "conversation record created", zap.String("conversation_id", conv.ID.String()))
	return conv.ID.String(), nil
}

// CreateConversationRecordWithParams creates a new conversation record with additional parameters
func (h *AdvancedChatWorkflowHandler) CreateConversationRecordWithParams(tenantID, agentID, accountID, fromSource, invokeFrom string, inputs map[string]interface{}, overrideModelConfigs *string, webAppID *string) (string, error) {
	ctx := logger.WithFields(context.Background(),
		zap.String("tenant_id", tenantID),
		zap.String("agent_id", agentID),
		zap.String("account_id", accountID),
		zap.String("from_source", fromSource),
		zap.String("invoke_from", invokeFrom),
		zap.Bool("has_web_app_id", webAppID != nil),
	)
	logger.DebugContext(ctx, "creating conversation record with params")

	// Parse UUIDs
	agentUUID, err := uuid.Parse(agentID)
	if err != nil {
		logger.WarnContext(ctx, "invalid agent id", zap.Error(err))
		return "", fmt.Errorf("invalid agent ID: %w", err)
	}

	accountUUID, err := uuid.Parse(accountID)
	if err != nil {
		logger.WarnContext(ctx, "invalid account id", zap.Error(err))
		return "", fmt.Errorf("invalid account ID: %w", err)
	}

	// Create conversation using the service
	name := h.conversationService.GenerateConversationName(ctx, "")

	// Set default inputs if not provided
	if inputs == nil {
		inputs = map[string]interface{}{
			"query": "",
			"files": []interface{}{},
		}
	}

	req := &conversation.CreateConversationRequest{
		AgentID:              agentUUID,
		Mode:                 "advanced-chat",
		Name:                 name,
		Inputs:               inputs,
		Status:               "normal",
		InvokeFrom:           &invokeFrom,
		WebAppID:             webAppID,
		FromSource:           fromSource,
		CreatedBy:            &accountUUID,
		OverrideModelConfigs: overrideModelConfigs,
	}

	// Set user ID based on source
	if fromSource == "end_user" {
		req.FromEndUserID = &accountUUID
	} else if fromSource == "account" {
		req.FromAccountID = &accountUUID
	}

	conv, err := h.conversationService.CreateConversation(ctx, req)
	if err != nil {
		logger.ErrorContext(ctx, "failed to create conversation", zap.Error(err))
		return "", fmt.Errorf("failed to create conversation record: %w", err)
	}

	logger.DebugContext(ctx, "conversation record created", zap.String("conversation_id", conv.ID.String()))
	return conv.ID.String(), nil
}

// UpdateConversationWebAppID updates the web_app_id for an existing conversation
func (h *AdvancedChatWorkflowHandler) UpdateConversationWebAppID(conversationID, webAppID string) error {
	ctx := logger.WithFields(context.Background(),
		zap.String("conversation_id", conversationID),
		zap.String("web_app_id", webAppID),
	)
	logger.DebugContext(ctx, "updating conversation web app id")

	// Parse conversation UUID
	convUUID, err := uuid.Parse(conversationID)
	if err != nil {
		logger.WarnContext(ctx, "invalid conversation id", zap.Error(err))
		return fmt.Errorf("invalid conversation ID: %w", err)
	}

	// Update the conversation's web_app_id
	err = h.conversationService.UpdateConversationWebAppID(ctx, convUUID, webAppID)
	if err != nil {
		logger.ErrorContext(ctx, "failed to update conversation web app id", zap.Error(err))
		return fmt.Errorf("failed to update conversation web_app_id: %w", err)
	}

	logger.DebugContext(ctx, "conversation web app id updated")
	return nil
}

// CreateWorkflowMessage creates a message for workflow execution
func (h *AdvancedChatWorkflowHandler) CreateWorkflowMessage(agentID, conversationID, workflowRunID uuid.UUID, query, answer, fromSource, invokeFrom string, fromUserID uuid.UUID, createdBy *uuid.UUID, webAppID *string) (*conversation.AgentMessage, error) {
	req := &conversation.CreateWorkflowMessageRequest{
		AgentID:        agentID,
		ConversationID: conversationID,
		Query:          query,
		Answer:         answer,
		WorkflowRunID:  workflowRunID,
		FromSource:     fromSource,
		FromUserID:     fromUserID,
		InvokeFrom:     invokeFrom,
		CreatedBy:      createdBy,
		WebAppID:       webAppID,
	}

	return h.messageService.CreateWorkflowMessage(context.Background(), req)
}

// CreateWorkflowMessageWithInputs creates a message for workflow execution with full inputs
func (h *AdvancedChatWorkflowHandler) CreateWorkflowMessageWithInputs(agentID, conversationID, workflowRunID uuid.UUID, query, answer, fromSource, invokeFrom string, fromUserID uuid.UUID, createdBy *uuid.UUID, webAppID *string, inputs map[string]interface{}) (*conversation.AgentMessage, error) {
	return h.CreateWorkflowMessageWithInputsAndStatus(
		agentID,
		conversationID,
		workflowRunID,
		query,
		answer,
		fromSource,
		invokeFrom,
		fromUserID,
		createdBy,
		webAppID,
		inputs,
		"",
	)
}

// CreateWorkflowMessageWithInputsAndStatus creates a workflow message with full inputs and an explicit lifecycle status.
func (h *AdvancedChatWorkflowHandler) CreateWorkflowMessageWithInputsAndStatus(agentID, conversationID, workflowRunID uuid.UUID, query, answer, fromSource, invokeFrom string, fromUserID uuid.UUID, createdBy *uuid.UUID, webAppID *string, inputs map[string]interface{}, status string) (*conversation.AgentMessage, error) {
	req := &conversation.CreateWorkflowMessageRequest{
		AgentID:        agentID,
		ConversationID: conversationID,
		Query:          query,
		Answer:         answer,
		Status:         status,
		WorkflowRunID:  workflowRunID,
		FromSource:     fromSource,
		FromUserID:     fromUserID,
		InvokeFrom:     invokeFrom,
		CreatedBy:      createdBy,
		WebAppID:       webAppID,
		Inputs:         inputs,
	}

	return h.messageService.CreateWorkflowMessage(context.Background(), req)
}

// LoadConversationVariables loads variables for a conversation
func (h *AdvancedChatWorkflowHandler) LoadConversationVariables(conversationID uuid.UUID) (map[string]interface{}, error) {
	return h.variableService.LoadConversationVariables(context.Background(), conversationID)
}

// SaveConversationVariables saves variables for a conversation
func (h *AdvancedChatWorkflowHandler) SaveConversationVariables(conversationID, agentID uuid.UUID, variables map[string]interface{}) error {
	return h.variableService.SaveConversationVariables(context.Background(), conversationID, agentID, variables)
}

// GetConversation retrieves a conversation by ID
func (h *AdvancedChatWorkflowHandler) GetConversation(conversationID uuid.UUID) (*conversation.AgentConversation, error) {
	return h.conversationService.GetConversation(context.Background(), conversationID)
}

// IncrementDialogueCount increments the dialogue count for a conversation
func (h *AdvancedChatWorkflowHandler) IncrementDialogueCount(conversationID uuid.UUID) error {
	return h.conversationService.IncrementDialogueCount(context.Background(), conversationID)
}

// GetConversationMessages retrieves all messages for a conversation
func (h *AdvancedChatWorkflowHandler) GetConversationMessages(conversationID uuid.UUID) ([]*conversation.AgentMessage, error) {
	return h.messageService.GetConversationMessages(context.Background(), conversationID)
}

// GetFirstMessagesByWorkflowRunIDs retrieves the earliest message for each workflow run ID.
func (h *AdvancedChatWorkflowHandler) GetFirstMessagesByWorkflowRunIDs(ctx context.Context, workflowRunIDs []string) (map[string]*conversation.AgentMessage, error) {
	return h.messageService.GetFirstMessagesByWorkflowRunIDs(ctx, workflowRunIDs)
}

// CreateWorkflowMessageWithParent creates a workflow message with parent message ID
func (h *AdvancedChatWorkflowHandler) CreateWorkflowMessageWithParent(
	agentID, conversationID, workflowRunID uuid.UUID,
	query, answer, fromSource, invokeFrom string,
	fromUserID uuid.UUID,
	createdBy *uuid.UUID,
	parentMessageID string,
	webAppID *string,
) (*conversation.AgentMessage, error) {
	req := &conversation.CreateWorkflowMessageRequest{
		AgentID:        agentID,
		ConversationID: conversationID,
		Query:          query,
		Answer:         answer,
		WorkflowRunID:  workflowRunID,
		FromSource:     fromSource,
		FromUserID:     fromUserID,
		InvokeFrom:     invokeFrom,
		CreatedBy:      createdBy,
		WebAppID:       webAppID,
	}

	if parentMessageID != "" {
		parentUUID, err := uuid.Parse(parentMessageID)
		if err == nil {
			req.ParentMessageID = &parentUUID
		}
	}

	return h.messageService.CreateWorkflowMessage(context.Background(), req)
}
