package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/dto"
	"github.com/zgiai/zgi/api/internal/modules/app/agents"
	"github.com/zgiai/zgi/api/internal/modules/app/conversation"
	fileProcessRepo "github.com/zgiai/zgi/api/internal/modules/file_process/repository"
	file_service "github.com/zgiai/zgi/api/internal/modules/file_process/service"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	"github.com/zgiai/zgi/api/pkg/database"
	"github.com/zgiai/zgi/api/pkg/logger"
	"github.com/zgiai/zgi/api/pkg/response"
	"github.com/zgiai/zgi/api/pkg/storage"
)

// ConversationQueryHandler handles conversation query operations
type ConversationQueryHandler struct {
	conversationService conversation.AgentConversationService
	messageService      conversation.AgentMessageService
	workflowRepo        WorkflowRepository
	agentsRepo          agents.AgentsRepository
	fileService         interfaces.FileService
}

// NewConversationQueryHandler creates a new ConversationQueryHandler
func NewConversationQueryHandler(workflowRepo WorkflowRepository, agentsRepo agents.AgentsRepository) *ConversationQueryHandler {
	db := database.GetDB()

	conversationRepo := conversation.NewAgentConversationRepository(db)
	messageRepo := conversation.NewAgentMessageRepository(db)

	conversationService := conversation.NewAgentConversationService(conversationRepo, messageRepo)
	messageService := conversation.NewAgentMessageService(messageRepo, conversationRepo)

	fileRepo := fileProcessRepo.NewFileRepository(db)
	storageClient := storage.GetStorage()
	fileService := file_service.NewFileService(fileRepo, storageClient, db, nil, nil)

	return &ConversationQueryHandler{
		conversationService: conversationService,
		messageService:      messageService,
		workflowRepo:        workflowRepo,
		agentsRepo:          agentsRepo,
		fileService:         fileService,
	}
}

func (h *ConversationQueryHandler) requireActiveWebAppAgent(c *gin.Context, webAppID string) (*agents.Agent, bool) {
	if webAppID == "" {
		logger.Error("web_app_id is required", nil)
		response.Fail(c, response.ErrInvalidParam)
		return nil, false
	}

	agent, err := h.agentsRepo.GetByWebAppID(c.Request.Context(), webAppID)
	if err != nil {
		logger.Error("Failed to get agent by web_app_id", err)
		response.Fail(c, response.ErrAppNotFound)
		return nil, false
	}
	if rejectInactiveWebApp(c, agent, webAppID) {
		return nil, false
	}
	return agent, true
}

// GetConversationList handles GET /workflows/:web_app_id/conversations
// @Summary Get conversation list
// @Description Get list of conversations for a web application
// @Tags Conversation
// @Accept json
// @Produce json
// @Param web_app_id path string true "Web App ID"
// @Param page query int false "Page number" default(1)
// @Param limit query int false "Page size" default(20)
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} response.ErrorResponse
// @Failure 500 {object} response.ErrorResponse
// @Router /workflows/{web_app_id}/conversations [get]
func (h *ConversationQueryHandler) GetConversationList(c *gin.Context) {
	webAppID := c.Param("web_app_id")
	accountID := c.GetString("account_id")

	// Parse query parameters
	page := 1
	limit := 20

	if pageStr := c.Query("page"); pageStr != "" {
		fmt.Sscanf(pageStr, "%d", &page)
	}
	if limitStr := c.Query("limit"); limitStr != "" {
		fmt.Sscanf(limitStr, "%d", &limit)
	}

	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}

	logger.Info("Getting conversation list", "webAppID", webAppID, "accountID", accountID)

	agent, ok := h.requireActiveWebAppAgent(c, webAppID)
	if !ok {
		return
	}

	// Parse agent UUID
	agentUUID, err := uuid.Parse(agent.ID.String())
	if err != nil {
		logger.Error("Invalid agent ID", err)
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	// Parse account UUID
	accountUUID, err := uuid.Parse(accountID)
	if err != nil {
		logger.Error("Invalid account ID", err)
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	// Get conversations filtered by web_app_id
	conversations, total, err := h.conversationService.GetConversationsByAgent(
		c.Request.Context(),
		agentUUID,
		accountUUID,
		webAppID,
		page,
		limit,
	)
	if err != nil {
		logger.Error("Failed to get conversations", err)
		response.Fail(c, response.ErrSystemError)
		return
	}

	// Build response
	items := make([]map[string]interface{}, 0, len(conversations))
	for _, conv := range conversations {
		item := map[string]interface{}{
			"id":             conv.ID.String(),
			"name":           conv.Name,
			"status":         conv.Status,
			"dialogue_count": conv.DialogueCount,
			"created_at":     conv.CreatedAt.Unix(),
			"updated_at":     conv.UpdatedAt.Unix(),
		}

		if conv.Summary != nil {
			item["summary"] = *conv.Summary
		}
		if conv.InvokeFrom != nil {
			item["invoke_from"] = *conv.InvokeFrom
		}
		if conv.WebAppID != nil {
			item["web_app_id"] = *conv.WebAppID
		}

		items = append(items, item)
	}

	hasMore := int64(page*limit) < total

	response.Success(c, map[string]interface{}{
		"data":     items,
		"page":     page,
		"limit":    limit,
		"total":    total,
		"has_more": hasMore,
	})
}

// GetConversationDetail handles GET /workflows/:web_app_id/conversations/:conversation_id
// @Summary Get conversation detail
// @Description Get detailed information and messages for a conversation
// @Tags Conversation
// @Accept json
// @Produce json
// @Param web_app_id path string true "Web App ID"
// @Param conversation_id path string true "Conversation ID"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} response.ErrorResponse
// @Failure 404 {object} response.ErrorResponse
// @Failure 500 {object} response.ErrorResponse
// @Router /workflows/{web_app_id}/conversations/{conversation_id} [get]
func (h *ConversationQueryHandler) GetConversationDetail(c *gin.Context) {
	webAppID := c.Param("web_app_id")
	conversationID := c.Param("conversation_id")
	accountID := c.GetString("account_id")

	logger.Info("Getting conversation detail", "webAppID", webAppID, "conversationID", conversationID, "accountID", accountID)

	if _, ok := h.requireActiveWebAppAgent(c, webAppID); !ok {
		return
	}

	// Parse conversation UUID
	conversationUUID, err := uuid.Parse(conversationID)
	if err != nil {
		logger.Error("Invalid conversation ID", err)
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	// Parse account UUID
	accountUUID, err := uuid.Parse(accountID)
	if err != nil {
		logger.Error("Invalid account ID", err)
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	// Get conversation
	conv, err := h.conversationService.GetConversation(c.Request.Context(), conversationUUID)
	if err != nil {
		logger.Error("Failed to get conversation", err)
		response.Fail(c, response.ErrAppNotFound)
		return
	}

	// Note: We don't verify web_app_id here to allow cross-version access
	// Users can access any conversation under their account_id regardless of web_app_id

	if !conversationBelongsToAccount(conv, accountUUID) {
		logger.Error("Conversation does not belong to this user", fmt.Errorf("user mismatch"))
		response.Fail(c, response.ErrPermissionDenied)
		return
	}

	// Get messages for this conversation (ordered by created_at DESC)
	messages, err := h.messageService.GetConversationMessages(c.Request.Context(), conversationUUID)
	if err != nil {
		logger.Error("Failed to get conversation messages", err)
		response.Fail(c, response.ErrSystemError)
		return
	}

	// Build message list
	messageItems := make([]map[string]interface{}, 0, len(messages))
	for _, msg := range messages {
		item := map[string]interface{}{
			"id":         msg.ID.String(),
			"query":      msg.Query,
			"answer":     msg.Answer,
			"status":     msg.Status,
			"created_at": msg.CreatedAt.Unix(),
		}

		if msg.WorkflowRunID != nil {
			item["workflow_run_id"] = msg.WorkflowRunID.String()
		}
		if msg.WebAppID != nil {
			item["web_app_id"] = *msg.WebAppID
		}
		if msg.InvokeFrom != nil {
			item["invoke_from"] = *msg.InvokeFrom
		}
		if msg.Error != nil {
			item["error"] = *msg.Error
		}

		// Parse inputs if available
		if inputs, err := msg.GetInputsAsMap(); err == nil {
			item["inputs"] = inputs
		}
		if metadata := conversationMessageMetadataWithQuestionAnswer(c.Request.Context(), msg); len(metadata) > 0 {
			item["message_metadata"] = metadata
		}

		messageItems = append(messageItems, item)
	}

	// Build conversation detail response
	convDetail := map[string]interface{}{
		"id":             conv.ID.String(),
		"agent_id":       conv.AgentID.String(),
		"name":           conv.Name,
		"status":         conv.Status,
		"mode":           conv.Mode,
		"dialogue_count": conv.DialogueCount,
		"created_at":     conv.CreatedAt.Unix(),
		"updated_at":     conv.UpdatedAt.Unix(),
		"messages":       messageItems,
	}

	if conv.Summary != nil {
		convDetail["summary"] = *conv.Summary
	}
	if conv.InvokeFrom != nil {
		convDetail["invoke_from"] = *conv.InvokeFrom
	}
	if conv.WebAppID != nil {
		convDetail["web_app_id"] = *conv.WebAppID
	}

	// Parse inputs if available
	if inputs, err := conv.GetInputsAsMap(); err == nil {
		convDetail["inputs"] = inputs
	}

	response.Success(c, convDetail)
}

// GetWebAppConfig handles GET /workflows/:web_app_id/config
// @Summary Get workflow configuration
// @Description Get workflow configuration by web app ID including variables, features, and agent metadata
// @Tags Workflow
// @Accept json
// @Produce json
// @Param web_app_id path string true "Web App ID"
// @Success 200 {object} dto.WorkflowConfigResponse
// @Failure 400 {object} response.ErrorResponse
// @Failure 404 {object} response.ErrorResponse
// @Failure 500 {object} response.ErrorResponse
// @Router /workflows/{web_app_id}/config [get]
func (h *ConversationQueryHandler) GetWebAppConfig(c *gin.Context) {
	webAppID := c.Param("web_app_id")

	// Validate web_app_id is not empty
	if webAppID == "" {
		logger.Error("web_app_id is required", nil)
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	logger.Info("Getting workflow config", "webAppID", webAppID)

	agent, ok := h.requireActiveWebAppAgent(c, webAppID)
	if !ok {
		return
	}

	// Get latest published workflow for this agent
	workflow, err := h.workflowRepo.GetLatestPublishedWorkflow(
		c.Request.Context(),
		agent.ID.String(),
	)
	if err != nil {
		logger.Error(fmt.Sprintf("Failed to get workflow for agent_id=%s, web_app_id=%s", agent.ID.String(), webAppID), err)
		response.FailWithMessage(c, response.ErrAppNotFound, "workflow not found")
		return
	}

	// Parse graph JSON to extract start node variables
	variables, err := h.extractStartNodeVariables(workflow.Graph)
	if err != nil {
		logger.Error(fmt.Sprintf("Failed to parse workflow graph for web_app_id %s", webAppID), err)
		response.FailWithMessage(c, response.ErrSystemError, "failed to parse workflow graph")
		return
	}

	// Parse features JSON
	features, err := h.parseFeatures(workflow.Features)
	if err != nil {
		logger.Error(fmt.Sprintf("Failed to parse workflow features for web_app_id %s", webAppID), err)
		response.FailWithMessage(c, response.ErrSystemError, "failed to parse workflow features")
		return
	}

	// Build app config with workflow type mapping
	icon := agent.Icon
	iconType := agent.IconType
	iconUrl := ""

	if iconType != nil && *iconType == "base64" && icon != nil && strings.HasPrefix(*icon, "data:image") {
		icon = nil
		iconType = nil
	} else if iconType != nil && *iconType == "image" && icon != nil && *icon != "" {
		fileURL, err := h.fileService.GetFileURL(context.Background(), *icon)
		if err != nil {
			logger.Warn(fmt.Sprintf("failed to get file URL for icon %s: %v", *icon, err))
		} else {
			iconUrl = fileURL
		}
	}

	appConfig := &dto.AppConfig{
		AgentID:  agent.ID.String(),
		Type:     mapWorkflowTypeToAgentType(workflow.Type),
		Icon:     safeString(icon),
		IconType: safeString(iconType),
		IconURL:  iconUrl,
		Title:    agent.Name,
	}

	// Build and return response with app config
	configResponse := dto.WorkflowConfigResponse{
		Variables: variables,
		Features:  features,
		Config:    appConfig,
	}

	response.Success(c, configResponse)
}

// extractStartNodeVariables parses the graph JSON and extracts variables from the start node
func (h *ConversationQueryHandler) extractStartNodeVariables(graphJSON string) ([]map[string]interface{}, error) {
	// Handle empty graph
	if graphJSON == "" {
		return []map[string]interface{}{}, nil
	}

	// Parse graph JSON string to map
	var graph map[string]interface{}
	if err := json.Unmarshal([]byte(graphJSON), &graph); err != nil {
		return nil, fmt.Errorf("invalid graph JSON: %w", err)
	}

	// Extract nodes array from graph
	nodesInterface, ok := graph["nodes"]
	if !ok {
		// No nodes in graph, return empty variables
		logger.Warn("No nodes found in graph")
		return []map[string]interface{}{}, nil
	}

	nodes, ok := nodesInterface.([]interface{})
	if !ok {
		return nil, fmt.Errorf("nodes is not an array")
	}

	// Find start node in nodes array
	for _, nodeInterface := range nodes {
		node, ok := nodeInterface.(map[string]interface{})
		if !ok {
			continue
		}

		// Check if this is the start node
		dataInterface, ok := node["data"]
		if !ok {
			continue
		}

		data, ok := dataInterface.(map[string]interface{})
		if !ok {
			continue
		}

		nodeType, ok := data["type"].(string)
		if !ok || nodeType != "start" {
			continue
		}

		// Found start node, extract variables
		variablesInterface, ok := data["variables"]
		if !ok {
			// Start node found but no variables field
			logger.Warn("Start node found but no variables field")
			return []map[string]interface{}{}, nil
		}

		variables, ok := variablesInterface.([]interface{})
		if !ok {
			// Variables field is not an array
			logger.Warn("Variables field is not an array")
			return []map[string]interface{}{}, nil
		}

		// Convert variables to []map[string]interface{}
		result := make([]map[string]interface{}, 0, len(variables))
		for _, varInterface := range variables {
			varMap, ok := varInterface.(map[string]interface{})
			if ok {
				result = append(result, varMap)
			}
		}

		return result, nil
	}

	// Start node not found, return empty variables
	logger.Warn("Start node not found in graph")
	return []map[string]interface{}{}, nil
}

// parseFeatures parses the features JSON string
func (h *ConversationQueryHandler) parseFeatures(featuresJSON string) (map[string]interface{}, error) {
	// Handle empty features
	if featuresJSON == "" {
		return map[string]interface{}{}, nil
	}

	// Parse features JSON string to map
	var features map[string]interface{}
	if err := json.Unmarshal([]byte(featuresJSON), &features); err != nil {
		return nil, fmt.Errorf("invalid features JSON: %w", err)
	}

	return features, nil
}

// safeString safely converts a *string to string, returning empty string if nil
func safeString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// mapWorkflowTypeToAgentType maps workflow type to agent type for API response
func mapWorkflowTypeToAgentType(workflowType dto.WorkflowType) string {
	switch workflowType {
	case dto.WorkflowTypeChat:
		return "CONVERSATIONAL_WORKFLOW"
	case dto.WorkflowTypeWorkflow:
		return "WORKFLOW"
	default:
		return "WORKFLOW"
	}
}

// DeleteConversation handles DELETE /workflows/:web_app_id/conversations/:conversation_id
// @Summary Delete conversation
// @Description Delete a conversation by ID
// @Tags Conversation
// @Accept json
// @Produce json
// @Param web_app_id path string true "Web App ID"
// @Param conversation_id path string true "Conversation ID"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} response.ErrorResponse
// @Failure 404 {object} response.ErrorResponse
// @Failure 500 {object} response.ErrorResponse
// @Router /workflows/{web_app_id}/conversations/{conversation_id} [delete]
func (h *ConversationQueryHandler) DeleteConversation(c *gin.Context) {
	webAppID := c.Param("web_app_id")
	conversationID := c.Param("conversation_id")
	accountID := c.GetString("account_id")

	logger.Info("Deleting conversation", "webAppID", webAppID, "conversationID", conversationID, "accountID", accountID)

	if _, ok := h.requireActiveWebAppAgent(c, webAppID); !ok {
		return
	}

	// Parse conversation UUID
	conversationUUID, err := uuid.Parse(conversationID)
	if err != nil {
		logger.Error("Invalid conversation ID", err)
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	// Parse account UUID
	accountUUID, err := uuid.Parse(accountID)
	if err != nil {
		logger.Error("Invalid account ID", err)
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	// Get conversation to verify ownership
	conv, err := h.conversationService.GetConversation(c.Request.Context(), conversationUUID)
	if err != nil {
		logger.Error("Failed to get conversation", err)
		response.Fail(c, response.ErrAppNotFound)
		return
	}

	// Note: We don't verify web_app_id here to allow cross-version access
	// Users can access any conversation under their account_id regardless of web_app_id

	if !conversationBelongsToAccount(conv, accountUUID) {
		logger.Error("Conversation does not belong to this user", fmt.Errorf("user mismatch"))
		response.Fail(c, response.ErrPermissionDenied)
		return
	}

	// Delete conversation
	err = h.conversationService.DeleteConversation(c.Request.Context(), conversationUUID, accountUUID)
	if err != nil {
		logger.Error("Failed to delete conversation", err)
		response.Fail(c, response.ErrSystemError)
		return
	}

	response.Success(c, map[string]interface{}{
		"result": "success",
	})
}

func conversationBelongsToAccount(conv *conversation.AgentConversation, accountID uuid.UUID) bool {
	if conv == nil || accountID == uuid.Nil {
		return false
	}
	if conv.FromAccountID != nil && *conv.FromAccountID == accountID {
		return true
	}
	if conv.FromEndUserID != nil && *conv.FromEndUserID == accountID {
		return true
	}
	return false
}
