package workflow

import (
	"context"

	"github.com/gin-gonic/gin"
	"github.com/zgiai/zgi/api/internal/dto"
	workspace_model "github.com/zgiai/zgi/api/internal/modules/workspace/model"
	"github.com/zgiai/zgi/api/internal/util"
	"github.com/zgiai/zgi/api/pkg/logger"
	"github.com/zgiai/zgi/api/pkg/response"
	"go.uber.org/zap"
)

// RunAdvancedChatDraftWorkflow handles POST /agents/{agent_id}/advanced-chat/workflows/draft/run
// @Summary Run advanced chat draft workflow
// @Description Execute the advanced chat draft workflow
// @Tags Workflow
// @Accept json
// @Produce json,text/event-stream
// @Param agent_id path string true "App ID"
// @Param request body dto.AdvancedChatDraftWorkflowRunRequest true "Run request"
// @Success 200 {object} dto.WorkflowRunResponse
// @Success 200 {string} string "Server-Sent Events stream"
// @Failure 400 {object} response.ErrorResponse
// @Failure 500 {object} response.ErrorResponse
// @Router /agents/{agent_id}/advanced-chat/workflows/draft/run [post]
func (h *WorkflowHandler) RunAdvancedChatDraftWorkflow(c *gin.Context) {
	appID := c.Param("agent_id")
	accountID := c.GetString("account_id")
	requestedWorkspaceID := util.GetWorkspaceID(c)

	if _, ok := h.requireAgentWorkspacePermission(c, appID, workspace_model.WorkspacePermissionWorkflowRunDraft); !ok {
		return
	}

	var req dto.AdvancedChatDraftWorkflowRunRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		logger.WarnContext(c.Request.Context(), "invalid request body", "agent_id", appID, err)
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	if err := h.validator.Struct(&req); err != nil {
		logger.WarnContext(c.Request.Context(), "request validation failed", "agent_id", appID, err)
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	logger.Info("Running advanced chat draft workflow", appID, accountID)

	// Validate workflow inputs before execution
	workflow, err := h.workflowService.GetDraftWorkflow(c.Request.Context(), appID, true)
	if err != nil {
		logger.CriticalContext(c.Request.Context(), "failed to get draft workflow for validation", "agent_id", appID, err)
		response.Fail(c, response.ErrAppNotFound)
		return
	}

	if err := h.validateWorkflowInputs(c.Request.Context(), workflow, req.Inputs); err != nil {
		logger.WarnContext(c.Request.Context(), "workflow input validation failed", "agent_id", appID, err)
		response.FailWithMessage(c, response.ErrInvalidParam, err.Error())
		return
	}

	if req.ConversationID != "" {
		if err := validateWebAppConversationAccess(c.Request.Context(), h.advancedChatHandler, req.ConversationID, appID, accountID); err != nil {
			logger.WarnContext(c.Request.Context(), "advanced chat draft conversation access denied", "conversation_id", req.ConversationID, "agent_id", appID, err)
			failWebAppConversationAccess(c, err)
			return
		}
	}

	if err := h.updateAgentWorkflowConfig(c.Request.Context(), appID, req.ConversationID, req.Inputs); err != nil {
		logger.ErrorContext(c.Request.Context(), "failed to update agent workflow config", "agent_id", appID, err)
	}

	// Convert to DraftWorkflowRunRequest for streaming
	draftReq := &dto.DraftWorkflowRunRequest{
		Inputs:       req.Inputs,
		UserID:       req.UserID,
		ResponseMode: req.ResponseMode,
		Files:        req.Files,
	}
	// Add query to inputs if present
	if req.Query != "" {
		if draftReq.Inputs == nil {
			draftReq.Inputs = make(map[string]interface{})
		}
		draftReq.Inputs["query"] = req.Query
	}

	if req.ConversationID != "" {
		logger.DebugContext(c.Request.Context(), "advanced chat draft continuing conversation",
			zap.String("conversation_id", req.ConversationID),
		)

		latestMessageID, err := h.getLatestMessageIDForCaller(c.Request.Context(), req.ConversationID, appID, accountID)
		if err == nil && latestMessageID != "" {
			draftReq.Inputs["sys.parent_message_id"] = latestMessageID
			logger.DebugContext(c.Request.Context(), "advanced chat draft set parent message",
				zap.String("parent_message_id", latestMessageID),
			)
		}

	} else {
		draftReq.Inputs["sys.parent_message_id"] = ""
		logger.DebugContext(c.Request.Context(), "advanced chat draft starting new conversation")
	}

	if req.ConversationID != "" {
		draftReq.Inputs["sys.conversation_id"] = req.ConversationID
		draftReq.Inputs["sys.dialogue_count"] = h.getDialogueCountForCaller(c.Request.Context(), req.ConversationID, appID, accountID)
		logger.DebugContext(c.Request.Context(), "advanced chat draft added existing conversation id",
			zap.String("conversation_id", req.ConversationID),
		)
	} else {
		draftReq.Inputs["sys.dialogue_count"] = 1
		logger.DebugContext(c.Request.Context(), "advanced chat draft starting new conversation")
	}
	if req.Query != "" {
		draftReq.Inputs["sys.query"] = req.Query
	}
	draftReq.Inputs["sys.workflow_type"] = "chat"

	if draftReq.Inputs == nil {
		draftReq.Inputs = make(map[string]interface{})
	}

	fromSource := "account"
	invokeFrom := string(InvokeFromDebugger)

	draftReq.Inputs["conversation_params"] = map[string]interface{}{
		"from_source": fromSource,
		"invoke_from": invokeFrom,
	}

	h.runWorkflowStream(c, requestedWorkspaceID, appID, draftReq, accountID, true, "CONVERSATION_WORKFLOW", "debugging")
}

// RunAdvancedChatWorkflow handles POST /agents/{agent_id}/advanced-chat/workflows/run
// @Summary Run advanced chat published workflow
// @Description Execute the advanced chat published workflow
// @Tags Workflow
// @Accept json
// @Produce json,text/event-stream
// @Param agent_id path string true "App ID"
// @Param request body dto.AdvancedChatDraftWorkflowRunRequest true "Run request"
// @Success 200 {object} dto.WorkflowRunResponse
// @Success 200 {string} string "Server-Sent Events stream"
// @Failure 400 {object} response.ErrorResponse
// @Failure 500 {object} response.ErrorResponse
// @Router /agents/{agent_id}/advanced-chat/workflows/run [post]
func (h *WorkflowHandler) RunAdvancedChatWorkflow(c *gin.Context) {
	appID := c.Param("agent_id")
	accountID := c.GetString("account_id")
	requestedWorkspaceID := util.GetWorkspaceID(c)
	organizationID := util.GetOrganizationID(c)

	if _, ok := h.requirePublishedRuntimeAgentAccess(c, appID, workspace_model.WorkspacePermissionWorkflowView); !ok {
		return
	}

	// Get invoke_from and created_from from context (set by external API middleware)
	invokeFrom := c.GetString("invoke_from")
	if invokeFrom == "" {
		invokeFrom = string(InvokeFromWebApp) // Default for internal calls
	}
	createdFrom := c.GetString("created_from")
	if createdFrom == "" {
		createdFrom = "web-app" // Default for internal calls
	}
	createdByRole := c.GetString("created_by_role")
	if createdByRole == "" {
		createdByRole = "account" // Default for internal calls
	}

	var req dto.AdvancedChatDraftWorkflowRunRequest

	// Check if this is an API Key call (external API format)
	isAPIKeyCall := invokeFrom == string(InvokeFromExternalAPI)

	if isAPIKeyCall {
		// Handle API Key call format
		var apiReq struct {
			Query             string                 `json:"query" binding:"required"`
			Inputs            map[string]interface{} `json:"inputs,omitempty"`
			ResponseMode      string                 `json:"response_mode" binding:"required"`
			User              string                 `json:"user,omitempty"`
			ConversationID    string                 `json:"conversation_id,omitempty"`
			HistoryWindowSize *int                   `json:"history_window_size,omitempty"`
			Files             []interface{}          `json:"files,omitempty"`
		}

		if err := c.ShouldBindJSON(&apiReq); err != nil {
			logger.WarnContext(c.Request.Context(), "invalid api request body", "agent_id", appID, err)
			response.Fail(c, response.ErrInvalidParam)
			return
		}

		// Convert files from []interface{} to []dto.FileInfo
		var files []dto.FileInfo
		for _, f := range apiReq.Files {
			if fileMap, ok := f.(map[string]interface{}); ok {
				file := dto.FileInfo{}
				if t, ok := fileMap["type"].(string); ok {
					file.Type = t
				}
				if tm, ok := fileMap["transfer_method"].(string); ok {
					file.TransferMethod = tm
				}
				if url, ok := fileMap["url"].(string); ok {
					file.URL = url
				}
				files = append(files, file)
			}
		}

		// Convert API request to internal format
		req = dto.AdvancedChatDraftWorkflowRunRequest{
			Query:          apiReq.Query,
			Inputs:         apiReq.Inputs,
			ResponseMode:   "streaming", // Force streaming for API calls
			UserID:         apiReq.User,
			ConversationID: apiReq.ConversationID,
			Files:          files,
		}

		logger.Info("API Key call converted to internal format", "originalResponseMode", apiReq.ResponseMode, "forcedResponseMode", req.ResponseMode)
	} else {
		// Handle internal call with standard format
		if err := c.ShouldBindJSON(&req); err != nil {
			logger.WarnContext(c.Request.Context(), "invalid request body", "agent_id", appID, err)
			response.Fail(c, response.ErrInvalidParam)
			return
		}

		// Force streaming mode for all calls
		req.ResponseMode = "streaming"
	}

	if err := h.validator.Struct(&req); err != nil {
		logger.WarnContext(c.Request.Context(), "request validation failed", "agent_id", appID, err)
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	logger.Info("Running advanced chat published workflow", "appID", appID, "accountID", accountID, "invokeFrom", invokeFrom, "createdFrom", createdFrom, "isAPIKeyCall", isAPIKeyCall)

	// Store context parameters in request context for service layer
	ctx := context.WithValue(c.Request.Context(), "invoke_from", invokeFrom)
	ctx = context.WithValue(ctx, "created_from", createdFrom)
	ctx = context.WithValue(ctx, "created_by_role", createdByRole)

	// Validate workflow inputs before execution
	workflow, err := h.workflowService.GetLatestPublishedWorkflow(ctx, organizationID, appID, true)
	if err != nil {
		logger.CriticalContext(ctx, "failed to get published workflow for validation", "agent_id", appID, err)
		response.Fail(c, response.ErrAppNotFound)
		return
	}

	if err := h.validateWorkflowInputs(ctx, workflow, req.Inputs); err != nil {
		logger.WarnContext(ctx, "workflow input validation failed", "agent_id", appID, err)
		response.FailWithMessage(c, response.ErrInvalidParam, err.Error())
		return
	}

	if req.ConversationID != "" {
		if err := validateWebAppConversationAccess(ctx, h.advancedChatHandler, req.ConversationID, appID, accountID); err != nil {
			logger.WarnContext(ctx, "advanced chat conversation access denied", "conversation_id", req.ConversationID, "agent_id", appID, err)
			failWebAppConversationAccess(c, err)
			return
		}
	}

	// Always use streaming mode (no need to check req.ResponseMode)
	{
		// Convert to DraftWorkflowRunRequest for streaming
		draftReq := &dto.DraftWorkflowRunRequest{
			Inputs:       req.Inputs,
			UserID:       req.UserID,
			ResponseMode: req.ResponseMode,
			Files:        req.Files,
		}
		// Add query to inputs if present
		if req.Query != "" {
			if draftReq.Inputs == nil {
				draftReq.Inputs = make(map[string]interface{})
			}
			draftReq.Inputs["query"] = req.Query
		}

		if req.ConversationID != "" {
			logger.DebugContext(ctx, "advanced chat continuing conversation",
				zap.String("conversation_id", req.ConversationID),
			)

			latestMessageID, err := h.getLatestMessageIDForCaller(ctx, req.ConversationID, appID, accountID)
			if err == nil && latestMessageID != "" {
				draftReq.Inputs["sys.parent_message_id"] = latestMessageID
				logger.DebugContext(ctx, "advanced chat set parent message",
					zap.String("parent_message_id", latestMessageID),
				)
			}

		} else {
			draftReq.Inputs["sys.parent_message_id"] = ""
			logger.DebugContext(ctx, "advanced chat starting new conversation")
		}

		if req.ConversationID != "" {
			draftReq.Inputs["sys.conversation_id"] = req.ConversationID
			draftReq.Inputs["sys.dialogue_count"] = h.getDialogueCountForCaller(ctx, req.ConversationID, appID, accountID)
			logger.DebugContext(ctx, "advanced chat added existing conversation id",
				zap.String("conversation_id", req.ConversationID),
			)
		} else {
			draftReq.Inputs["sys.dialogue_count"] = 1
			logger.DebugContext(ctx, "advanced chat will create conversation in stream")
		}
		if req.Query != "" {
			draftReq.Inputs["sys.query"] = req.Query
		}
		draftReq.Inputs["sys.workflow_type"] = "chat"

		if draftReq.Inputs == nil {
			draftReq.Inputs = make(map[string]interface{})
		}

		draftReq.Inputs["conversation_params"] = map[string]interface{}{
			"from_source": createdByRole,
			"invoke_from": invokeFrom,
		}

		// Update context in gin.Context
		c.Request = c.Request.WithContext(ctx)

		// Determine triggeredFrom for published conversation workflow
		triggeredFrom := invokeFrom // Use invokeFrom from context (e.g., "external-api", "web-app")
		if triggeredFrom == "" {
			triggeredFrom = "app-run" // Default for published workflows
		}

		// Run published workflow (isDraft=false)
		h.runWorkflowStream(c, requestedWorkspaceID, appID, draftReq, accountID, false, "CONVERSATION_WORKFLOW", triggeredFrom)
	}
}
