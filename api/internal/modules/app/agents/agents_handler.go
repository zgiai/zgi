package agents

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	runtimedto "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/dto"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	runtimeservice "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/service"
	"github.com/zgiai/zgi/api/internal/dto"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	workspace_model "github.com/zgiai/zgi/api/internal/modules/workspace/model"
	"github.com/zgiai/zgi/api/internal/util"
	"github.com/zgiai/zgi/api/pkg/logger"
	"github.com/zgiai/zgi/api/pkg/response"
	"gorm.io/gorm"
)

type AgentsHandler struct {
	appService          AgentsService
	tenantService       interfaces.WorkspaceManagementService
	accountService      interfaces.AccountService
	organizationService interfaces.OrganizationService
	db                  *gorm.DB
	chatRuntimeService  runtimeservice.Service
}

func NewAgentsHandler(appService AgentsService, tenantService interfaces.WorkspaceManagementService, accountService interfaces.AccountService, organizationService interfaces.OrganizationService, db *gorm.DB, chatRuntimeServices ...runtimeservice.Service) *AgentsHandler {
	var chatRuntimeService runtimeservice.Service
	if len(chatRuntimeServices) > 0 {
		chatRuntimeService = chatRuntimeServices[0]
	}
	return &AgentsHandler{
		appService:          appService,
		tenantService:       tenantService,
		accountService:      accountService,
		organizationService: organizationService,
		db:                  db,
		chatRuntimeService:  chatRuntimeService,
	}
}

func (h *AgentsHandler) GetAgentsList(c *gin.Context) {
	// 1. Authenticate user - Requirement 11.1: Handle authentication errors (401)
	accountID := c.GetString("account_id")
	if accountID == "" {
		// Requirement 11.1: Return 401 for missing account_id
		logger.Error("GetAgentsList: authentication failed - account_id not found in context", nil)
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	// Validate account_id format
	if _, err := uuid.Parse(accountID); err != nil {
		// Requirement 11.1: Return 401 for invalid account_id format
		// Requirement 11.5: Add structured logging with context
		logger.Error(fmt.Sprintf("GetAgentsList: authentication failed - invalid account_id format: account_id=%s", accountID), err)
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	// 2. Parse query parameters - Requirement 11.4: Handle invalid parameter errors (400)
	var req dto.GetAgentsListRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		// Log parameter binding errors but continue with defaults
		logger.Warn("GetAgentsList: failed to bind query parameters, using defaults", map[string]interface{}{
			"account_id": accountID,
			"error":      err.Error(),
		})
		// Continue with default values
	}

	// 3. Parse internal parameter correctly (handle "true"/"false" strings)
	if internalStr := c.Query("internal"); internalStr != "" {
		switch internalStr {
		case "true":
			internalVal := true
			req.Internal = &internalVal
		case "false":
			internalVal := false
			req.Internal = &internalVal
		default:
			// Requirement 11.4: Handle invalid parameter errors (400)
			logger.Warn("GetAgentsList: invalid internal parameter value", map[string]interface{}{
				"account_id": accountID,
				"value":      internalStr,
			})
			response.Fail(c, response.ErrInvalidParam)
			return
		}
	}

	// 4. Validate and normalize pagination parameters
	if req.Page <= 0 {
		req.Page = 1
	}
	if req.Page > 99999 {
		req.Page = 99999
	}

	pageSize := 20
	if req.PageSize > 0 {
		pageSize = req.PageSize
	} else if req.Limit > 0 {
		pageSize = req.Limit
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	req.Limit = pageSize

	// 5. Call new service method with permission-based filtering
	// Requirement 11.5: Add structured logging with context (account_id, org_id, dept_ids)
	logger.Info("GetAgentsList: calling GetAgentsListWithPermissions", map[string]interface{}{
		"account_id": accountID,
		"page":       req.Page,
		"limit":      req.Limit,
		"name":       req.Name,
		"keyword":    req.Keyword,
		"agent_type": req.AgentType,
		"internal":   req.Internal,
	})

	// Requirement 11.2, 11.3, 11.4: Handle service errors and map to HTTP responses
	result, err := h.appService.GetAgentsListWithPermissions(c.Request.Context(), accountID, req)
	if err != nil {
		// Requirement 11.5: Add structured logging with context
		logger.Error(fmt.Sprintf("GetAgentsList: service error for account_id=%s", accountID), err)

		// Requirement 11.2: Handle tenant not found errors (404)
		if err.Error() == "tenant not found" {
			response.Fail(c, response.ErrWorkspaceNotFound)
			return
		}

		// Requirement 11.3: Handle database errors (500) with logging
		if err.Error() == "failed to determine user permissions" ||
			err.Error() == "failed to get current organization" ||
			err.Error() == "failed to get organization departments" ||
			err.Error() == "failed to get user department memberships" ||
			err.Error() == "failed to retrieve agents" {
			response.Fail(c, response.ErrSystemError)
			return
		}

		// Default error response
		response.SpecialFail(c, gin.H{
			"code":    "399001",
			"message": err.Error(),
		})
		return
	}

	// 6. Return successful response with agent list and pagination info
	// Requirement 11.5: Add structured logging with context
	logger.Info("GetAgentsList: successfully retrieved agents", map[string]interface{}{
		"account_id":  accountID,
		"agent_count": len(result.Data),
		"total":       result.Total,
		"page":        result.Page,
		"limit":       result.Limit,
	})
	response.Success(c, result)
}

func (h *AgentsHandler) GetRunnableWebApps(c *gin.Context) {
	accountID := c.GetString("account_id")
	if accountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	if _, err := uuid.Parse(accountID); err != nil {
		logger.Error(fmt.Sprintf("GetRunnableWebApps: invalid account_id format: account_id=%s", accountID), err)
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	var req dto.GetRunnableWebAppsRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	if req.WorkspaceID != "" {
		if _, err := uuid.Parse(req.WorkspaceID); err != nil {
			response.Fail(c, response.ErrInvalidParam)
			return
		}
	}

	result, err := h.appService.GetRunnableWebApps(c.Request.Context(), accountID, req)
	if err != nil {
		switch {
		case errors.Is(err, errCurrentOrganizationNotFound):
			response.Fail(c, response.ErrOrganizationNotFound)
		default:
			logger.Error(fmt.Sprintf("GetRunnableWebApps: service error for account_id=%s", accountID), err)
			response.Fail(c, response.ErrSystemError)
		}
		return
	}

	response.Success(c, result)
}

func (h *AgentsHandler) CreateAgent(c *gin.Context) {
	accountID := c.GetString("account_id")
	if accountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	var req dto.CreateAgentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	callerOrganizationID := util.GetOrganizationID(c)
	if callerOrganizationID == "" {
		response.Fail(c, response.ErrOrganizationNotFound)
		return
	}

	targetWorkspaceID := req.WorkspaceID
	if targetWorkspaceID == "" {
		response.Fail(c, response.ErrWorkspaceNotFound)
		return
	}

	if h.organizationService != nil {
		hasPermission, err := h.organizationService.CheckWorkspacePermission(
			c.Request.Context(),
			callerOrganizationID,
			targetWorkspaceID,
			accountID,
			workspace_model.WorkspacePermissionAgentManage,
		)
		if err != nil {
			logger.Error("Failed to check create agent workspace permission", err)
			response.Fail(c, response.ErrSystemError)
			return
		}
		if !hasPermission {
			logger.Warn("CreateAgent: workspace permission denied", map[string]interface{}{
				"account_id":   accountID,
				"workspace_id": targetWorkspaceID,
			})
			response.Fail(c, response.ErrPermissionDenied)
			return
		}
	}

	result, err := h.appService.CreateAgent(c.Request.Context(), targetWorkspaceID, req, accountID)
	if err != nil {
		// Basic error mapping
		response.SpecialFail(c, gin.H{
			"code":    "399001",
			"message": err.Error(),
		})
		return
	}

	response.Success(c, result)
}

func (h *AgentsHandler) GetAgent(c *gin.Context) {
	accountID := c.GetString("account_id")
	if accountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	agentID := c.Param("agent_id")
	if agentID == "" {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	// Pass account_id via context for is_editor calculation
	ctx := c.Request.Context()
	ctx = context.WithValue(ctx, "account_id", accountID)
	if callerOrganizationID := util.GetOrganizationID(c); callerOrganizationID != "" {
		ctx = context.WithValue(ctx, "tenant_id", callerOrganizationID)
	}

	result, err := h.appService.GetAgent(ctx, agentID)
	if err != nil {
		switch err.Error() {
		case "permission denied":
			response.SpecialFail(c, gin.H{"code": "403001", "message": "Permission denied"})
		default:
			response.SpecialFail(c, gin.H{
				"code":    "399001",
				"message": err.Error(),
			})
		}
		return
	}

	response.Success(c, result)
}

func (h *AgentsHandler) GetAgentConfig(c *gin.Context) {
	accountID := c.GetString("account_id")
	if accountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}
	result, err := h.appService.GetAgentConfig(c.Request.Context(), c.Param("agent_id"), accountID)
	if err != nil {
		response.SpecialFail(c, gin.H{"code": "399001", "message": err.Error()})
		return
	}
	response.Success(c, result)
}

func (h *AgentsHandler) UpdateAgentConfig(c *gin.Context) {
	accountID := c.GetString("account_id")
	if accountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}
	var req dto.AgentConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}
	if err := h.validateAgentRuntimeSkills(c, req); err != nil {
		response.SpecialFail(c, gin.H{"code": "399001", "message": err.Error()})
		return
	}
	result, err := h.appService.UpdateAgentConfig(c.Request.Context(), c.Param("agent_id"), accountID, req)
	if err != nil {
		response.SpecialFail(c, gin.H{"code": "399001", "message": err.Error()})
		return
	}
	response.Success(c, result)
}

func (h *AgentsHandler) PublishAgent(c *gin.Context) {
	accountID := c.GetString("account_id")
	if accountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}
	var req dto.PublishAgentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		req = dto.PublishAgentRequest{}
	}
	cfg, err := h.appService.GetAgentConfig(c.Request.Context(), c.Param("agent_id"), accountID)
	if err != nil {
		response.SpecialFail(c, gin.H{"code": "399001", "message": err.Error()})
		return
	}
	if err := h.validateAgentRuntimeSkills(c, dto.AgentConfigRequest{
		EnabledSkillIDs:     cfg.EnabledSkillIDs,
		KnowledgeDatasetIDs: cfg.KnowledgeDatasetIDs,
	}); err != nil {
		response.SpecialFail(c, gin.H{"code": "399001", "message": err.Error()})
		return
	}
	result, err := h.appService.PublishAgent(c.Request.Context(), c.Param("agent_id"), accountID, req)
	if err != nil {
		response.SpecialFail(c, gin.H{"code": "399001", "message": err.Error()})
		return
	}
	response.Success(c, result)
}

func (h *AgentsHandler) GenerateAgentSuggestedQuestions(c *gin.Context) {
	accountID := c.GetString("account_id")
	var req dto.GenerateAgentSuggestedQuestionsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}
	result, err := h.appService.GenerateAgentSuggestedQuestions(c.Request.Context(), c.Param("agent_id"), accountID, &req)
	if err != nil {
		if isAgentSuggestedQuestionsConfigurationError(err) {
			response.FailWithMessage(c, response.ErrConfigError, "Please configure a default LLM model before generating suggested questions.")
			return
		}
		if isAgentSuggestedQuestionsModelOutputError(err) {
			response.FailWithMessage(c, response.ErrServiceUnavailable, "The model did not return usable suggested questions. Please try again.")
			return
		}
		response.FailWithMessage(c, response.ErrServiceUnavailable, "Failed to generate suggested questions. Please try again.")
		return
	}
	response.Success(c, result)
}

func (h *AgentsHandler) ChatAgent(c *gin.Context) {
	if h.chatRuntimeService == nil {
		response.Fail(c, response.ErrSystemError)
		return
	}
	accountID, err := uuid.Parse(strings.TrimSpace(c.GetString("account_id")))
	if err != nil {
		response.Fail(c, response.ErrUnauthorized)
		return
	}
	organizationID, err := uuid.Parse(strings.TrimSpace(util.GetOrganizationID(c)))
	if err != nil {
		response.Fail(c, response.ErrUnauthorized)
		return
	}
	agentID, err := uuid.Parse(strings.TrimSpace(c.Param("agent_id")))
	if err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}
	var workspaceID *uuid.UUID
	if raw := strings.TrimSpace(util.GetWorkspaceID(c)); raw != "" {
		if parsed, err := uuid.Parse(raw); err == nil {
			workspaceID = &parsed
		}
	}
	var req runtimedto.ChatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}
	cfg, err := h.appService.GetAgentConfig(c.Request.Context(), agentID.String(), accountID.String())
	if err != nil {
		response.SpecialFail(c, gin.H{"code": "399001", "message": err.Error()})
		return
	}
	runConfig := runtimeservice.RunConfig{
		SystemPrompt:             cfg.SystemPrompt,
		SystemPromptVersion:      "agent.draft",
		ModelProvider:            cfg.ModelProvider,
		Model:                    cfg.Model,
		ModelParameters:          cfg.ModelParameters,
		EnabledSkillIDs:          cfg.EnabledSkillIDs,
		KnowledgeDatasetIDs:      cfg.KnowledgeDatasetIDs,
		KnowledgeRetrievalConfig: cfg.KnowledgeRetrievalConfig,
		UseMemory:                cfg.UseMemory,
		BillingAppID:             agentID.String(),
		BillingAppType:           runtimemodel.ConversationCallerAgent,
	}
	prepared, err := h.chatRuntimeService.PrepareConfiguredChat(
		c.Request.Context(),
		runtimeservice.Scope{OrganizationID: organizationID, AccountID: accountID, WorkspaceID: workspaceID},
		runtimeservice.Caller{Type: runtimemodel.ConversationCallerAgent, ID: &agentID, Source: runtimemodel.ConversationSourceConsole},
		runConfig,
		req,
	)
	if err != nil {
		response.SpecialFail(c, gin.H{"code": "399001", "message": err.Error()})
		return
	}
	setupAgentSSE(c)
	_ = writeAgentSSE(c, "message_start", gin.H{
		"conversation_id": prepared.Conversation.ID.String(),
		"message_id":      prepared.Message.ID.String(),
		"parent_id":       uuidPtrToString(prepared.Message.ParentID),
		"title":           prepared.Conversation.Title,
		"model":           prepared.Message.ModelName,
		"created_at":      prepared.Message.CreatedAt.Unix(),
	})
	result, err := h.chatRuntimeService.RunPreparedStream(c.Request.Context(), prepared, func(chunk string) error {
		return writeAgentSSE(c, "message", gin.H{
			"conversation_id": prepared.Conversation.ID.String(),
			"message_id":      prepared.Message.ID.String(),
			"answer":          chunk,
		})
	}, func(event runtimeservice.StreamEvent) error {
		return writeAgentSSEEvent(c, event.ID, event.EventType, event.Payload)
	})
	if err != nil {
		status := runtimemodel.MessageStatusError
		if errors.Is(err, runtimeservice.ErrMessageStopped) {
			status = runtimemodel.MessageStatusStopped
		}
		_ = writeAgentSSE(c, "message_end", gin.H{
			"conversation_id": prepared.Conversation.ID.String(),
			"message_id":      prepared.Message.ID.String(),
			"status":          status,
			"metadata":        gin.H{},
		})
		return
	}
	_ = writeAgentSSE(c, "message_end", gin.H{
		"conversation_id": prepared.Conversation.ID.String(),
		"message_id":      prepared.Message.ID.String(),
		"status":          runtimemodel.MessageStatusCompleted,
		"metadata": gin.H{
			"usage": result.Metadata["usage"],
		},
	})
}

func (h *AgentsHandler) ListAgentPublishedVersions(c *gin.Context) {
	page := 1
	limit := 20
	if parsed, err := strconv.Atoi(c.DefaultQuery("page", "1")); err == nil {
		page = parsed
	}
	if parsed, err := strconv.Atoi(c.DefaultQuery("limit", "20")); err == nil {
		limit = parsed
	}
	result, err := h.appService.ListAgentPublishedVersions(c.Request.Context(), c.Param("agent_id"), page, limit)
	if err != nil {
		response.SpecialFail(c, gin.H{"code": "399001", "message": err.Error()})
		return
	}
	response.Success(c, result)
}

func (h *AgentsHandler) RollbackAgentPublishedVersion(c *gin.Context) {
	accountID := c.GetString("account_id")
	if accountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}
	var req dto.RollbackAgentPublishedVersionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}
	result, err := h.appService.RollbackAgentPublishedVersion(c.Request.Context(), c.Param("agent_id"), accountID, req)
	if err != nil {
		response.SpecialFail(c, gin.H{"code": "399001", "message": err.Error()})
		return
	}
	response.Success(c, result)
}

func (h *AgentsHandler) GetWebAppRuntimeConfig(c *gin.Context) {
	result, err := h.appService.GetPublishedAgentWebAppConfig(c.Request.Context(), c.Param("web_app_id"))
	if err != nil {
		response.SpecialFail(c, gin.H{"code": "399001", "message": err.Error()})
		return
	}
	response.Success(c, gin.H{
		"variables": []interface{}{},
		"features": gin.H{
			"agent_type":          result.AgentType,
			"runtime":             "chat",
			"suggested_questions": result.Config.SuggestedQuestions,
		},
		"config": gin.H{
			"agent_id":   result.AgentID,
			"type":       result.AgentType,
			"icon":       result.Icon,
			"icon_type":  result.IconType,
			"icon_url":   result.IconURL,
			"title":      result.Name,
			"web_app_id": result.WebAppID,
		},
		"agent_config": result.Config,
		"version": gin.H{
			"version":      result.Version,
			"version_uuid": result.VersionUUID,
		},
	})
}

func (h *AgentsHandler) ChatWebAppAgent(c *gin.Context) {
	if h.chatRuntimeService == nil {
		response.Fail(c, response.ErrSystemError)
		return
	}
	webAppID := strings.TrimSpace(c.Param("web_app_id"))
	published, err := h.appService.GetPublishedAgentWebAppConfig(c.Request.Context(), webAppID)
	if err != nil {
		response.SpecialFail(c, gin.H{"code": "399001", "message": err.Error()})
		return
	}
	accountID, err := uuid.Parse(strings.TrimSpace(c.GetString("account_id")))
	if err != nil {
		response.Fail(c, response.ErrUnauthorized)
		return
	}
	agentID, err := uuid.Parse(published.AgentID)
	if err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}
	workspaceID, err := uuid.Parse(published.WorkspaceID)
	if err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}
	organizationID, err := uuid.Parse(published.OrganizationID)
	if err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}
	sourceWebAppID, err := uuid.Parse(published.WebAppID)
	if err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}
	var req runtimedto.ChatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}
	runConfig := runtimeservice.RunConfig{
		SystemPrompt:             published.Config.SystemPrompt,
		SystemPromptVersion:      "agent.published." + published.Version,
		ModelProvider:            published.Config.ModelProvider,
		Model:                    published.Config.Model,
		ModelParameters:          published.Config.ModelParameters,
		EnabledSkillIDs:          published.Config.EnabledSkillIDs,
		KnowledgeDatasetIDs:      published.Config.KnowledgeDatasetIDs,
		KnowledgeRetrievalConfig: published.Config.KnowledgeRetrievalConfig,
		UseMemory:                published.Config.UseMemory,
		BillingAppID:             published.AgentID,
		BillingAppType:           runtimemodel.ConversationCallerAgent,
	}
	prepared, err := h.chatRuntimeService.PrepareConfiguredChat(
		c.Request.Context(),
		runtimeservice.Scope{OrganizationID: organizationID, AccountID: accountID, WorkspaceID: &workspaceID, SkipAccessCheck: true},
		runtimeservice.Caller{
			Type:           runtimemodel.ConversationCallerAgent,
			ID:             &agentID,
			Source:         runtimemodel.ConversationSourceWebApp,
			SourceWebAppID: &sourceWebAppID,
		},
		runConfig,
		req,
	)
	if err != nil {
		response.SpecialFail(c, gin.H{"code": "399001", "message": err.Error()})
		return
	}
	setupAgentSSE(c)
	_ = writeAgentSSE(c, "message_start", gin.H{
		"conversation_id": prepared.Conversation.ID.String(),
		"message_id":      prepared.Message.ID.String(),
		"parent_id":       uuidPtrToString(prepared.Message.ParentID),
		"title":           prepared.Conversation.Title,
		"model":           prepared.Message.ModelName,
		"created_at":      prepared.Message.CreatedAt.Unix(),
	})
	result, err := h.chatRuntimeService.RunPreparedStream(c.Request.Context(), prepared, func(chunk string) error {
		return writeAgentSSE(c, "message", gin.H{
			"conversation_id": prepared.Conversation.ID.String(),
			"message_id":      prepared.Message.ID.String(),
			"answer":          chunk,
		})
	}, func(event runtimeservice.StreamEvent) error {
		return writeAgentSSEEvent(c, event.ID, event.EventType, event.Payload)
	})
	if err != nil {
		status := runtimemodel.MessageStatusError
		if errors.Is(err, runtimeservice.ErrMessageStopped) {
			status = runtimemodel.MessageStatusStopped
		}
		_ = writeAgentSSE(c, "message_end", gin.H{
			"conversation_id": prepared.Conversation.ID.String(),
			"message_id":      prepared.Message.ID.String(),
			"status":          status,
			"metadata":        gin.H{},
		})
		return
	}
	_ = writeAgentSSE(c, "message_end", gin.H{
		"conversation_id": prepared.Conversation.ID.String(),
		"message_id":      prepared.Message.ID.String(),
		"status":          runtimemodel.MessageStatusCompleted,
		"metadata": gin.H{
			"usage": result.Metadata["usage"],
		},
	})
}

func setupAgentSSE(c *gin.Context) {
	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("Transfer-Encoding", "chunked")
	c.Status(http.StatusOK)
	c.Writer.Flush()
}

func writeAgentSSE(c *gin.Context, event string, data interface{}) error {
	return writeAgentSSEEvent(c, "", event, data)
}

func writeAgentSSEEvent(c *gin.Context, id string, event string, data interface{}) error {
	payload := gin.H{"event": event, "data": data}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	if id != "" {
		_, _ = fmt.Fprintf(c.Writer, "id: %s\n", id)
	}
	_, _ = fmt.Fprintf(c.Writer, "event: %s\n", event)
	_, _ = fmt.Fprintf(c.Writer, "data: %s\n\n", string(encoded))
	c.Writer.Flush()
	return nil
}

func (h *AgentsHandler) UpdateAgent(c *gin.Context) {
	// Authenticate
	accountID := c.GetString("account_id")
	if accountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	agentID := c.Param("agent_id")
	if agentID == "" {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	callerOrganizationID := util.GetOrganizationID(c)
	if callerOrganizationID == "" {
		response.Fail(c, response.ErrOrganizationNotFound)
		return
	}

	// Bind update request (partial update)
	var req map[string]interface{}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	// Optional: cross-tenant update permission check when tenant_id provided
	if v, ok := req["tenant_id"].(string); ok && v != "" {
		hasPermission, err := h.organizationService.CheckWorkspacePermission(
			c.Request.Context(),
			callerOrganizationID,
			v,
			accountID,
			workspace_model.WorkspacePermissionAgentManage,
		)
		if err != nil {
			logger.Error("Failed to check workspace permission for update", err)
			response.Fail(c, response.ErrSystemError)
			return
		}
		if !hasPermission {
			response.Fail(c, response.ErrPermissionDenied)
			return
		}
	}

	// Pass account_id and caller organization context for permission verification in service
	ctx := context.WithValue(c.Request.Context(), "account_id", accountID)
	ctx = context.WithValue(ctx, "tenant_id", callerOrganizationID)
	result, err := h.appService.UpdateAgent(ctx, agentID, req)
	if err != nil {
		// Map specific errors to appropriate responses
		switch err.Error() {
		case "agent not found":
			response.SpecialFail(c, gin.H{"code": "404001", "message": "Agent not found"})
		case "permission denied":
			response.SpecialFail(c, gin.H{"code": "403001", "message": "Permission denied"})
		case "agent with the same name already exists":
			response.SpecialFail(c, gin.H{"code": "409001", "message": "Duplicated agent name in tenant"})
		default:
			response.SpecialFail(c, gin.H{"code": "399001", "message": err.Error()})
		}
		return
	}

	response.Success(c, result)
}

func (h *AgentsHandler) DeleteAgent(c *gin.Context) {
	// Get account ID from context for authentication
	accountID := c.GetString("account_id")
	if accountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	// Get agent ID from URL parameter
	agentID := c.Param("agent_id")
	if agentID == "" {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	callerOrganizationID := util.GetOrganizationID(c)
	if callerOrganizationID == "" {
		response.Fail(c, response.ErrOrganizationNotFound)
		return
	}

	// Pass account_id and caller organization context for permission validation
	ctx := c.Request.Context()
	ctx = context.WithValue(ctx, "account_id", accountID)
	ctx = context.WithValue(ctx, "tenant_id", callerOrganizationID)

	// Call service to delete agent with permission validation
	err := h.appService.DeleteAgent(ctx, agentID)
	if err != nil {
		// Map specific errors to appropriate responses
		switch err.Error() {
		case "agent not found":
			response.SpecialFail(c, gin.H{
				"code":    "404001",
				"message": "Agent not found",
			})
		case "permission denied":
			response.SpecialFail(c, gin.H{
				"code":    "403001",
				"message": "Permission denied",
			})
		default:
			response.SpecialFail(c, gin.H{
				"code":    "399001",
				"message": err.Error(),
			})
		}
		return
	}

	response.Success(c, gin.H{"message": "Agent deleted successfully"})
}
