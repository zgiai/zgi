package workflow

import (
	"context"
	"fmt"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/zgiai/zgi/api/internal/dto"
	"github.com/zgiai/zgi/api/internal/modules/app/agents"
	"github.com/zgiai/zgi/api/internal/modules/workspace/repository"
	"github.com/zgiai/zgi/api/internal/util"
	"github.com/zgiai/zgi/api/pkg/logger"
	"github.com/zgiai/zgi/api/pkg/response"
	"go.uber.org/zap"
)

// PublishWorkflow handles POST /agents/{agent_id}/workflows/publish
// @Summary Publish workflow
// @Description Publish the current draft workflow as a new version
// @Tags Workflow
// @Accept json
// @Produce json
// @Param agent_id path string true "Agent ID"
// @Param request body PublishWorkflowRequest true "Publish request"
// @Success 200 {object} PublishWorkflowResponse
// RunWorkflowByVersionUUID handles POST /workflows/:version_uuid/run
// @Summary Run workflow by version UUID
// @Description Run a specific version of workflow using version UUID
// @Tags Workflow
// RunWorkflowByVersionUUID handles POST /workflows/:version_uuid/run (kept for backward compatibility)
// @Summary Run workflow by version UUID
// @Description Run a specific version of workflow using version UUID
// @Tags Workflow
// @Accept json
// @Produce json
// @Param version_uuid path string true "Workflow Version UUID"
// @Param request body dto.DraftWorkflowRunRequest true "Run request"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} response.ErrorResponse
// @Failure 404 {object} response.ErrorResponse
// @Failure 500 {object} response.ErrorResponse
// @Router /workflows/{version_uuid}/run [post]
func (h *WorkflowHandler) RunWorkflowByVersionUUID(c *gin.Context) {
	versionUUID := c.Param("version_uuid")
	accountID := c.GetString("account_id")
	workspaceID := util.GetWorkspaceID(c)

	logger.Info("Running workflow by web_app_id (legacy)", "webAppID", versionUUID, "accountID", accountID)

	// Get workflow by version UUID first to determine workflow type
	workflowInterface, err := h.workflowService.GetWorkflowByVersionUUID(c.Request.Context(), versionUUID)
	if err != nil {
		logger.ErrorContext(c.Request.Context(), "failed to get workflow by version uuid", "version_uuid", versionUUID, err)
		if strings.Contains(err.Error(), "not found") {
			response.Fail(c, response.ErrAppNotFound)
		} else {
			response.Fail(c, response.ErrSystemError)
		}
		return
	}

	// Type assertion to get workflow details
	workflow, ok := workflowInterface.(*Workflow)
	if !ok {
		logger.CriticalContext(c.Request.Context(), "failed to cast workflow", "version_uuid", versionUUID, fmt.Errorf("invalid type"))
		response.Fail(c, response.ErrSystemError)
		return
	}
	logger.Info("Using workspace scope for legacy workflow run", "workspaceID", workspaceID)

	// If workspace_id is empty (e.g., when using account-uuid auth without workspace association),
	// resolve it from the account's current workspace or workflow's workspace.
	if workspaceID == "" {
		createAccountID := workflow.CreatedBy

		// Try to get the current workspace for this account.
		if ws, ok := h.workflowService.(*WorkflowService); ok {
			if wr, ok := ws.repo.(*workflowRepository); ok && wr.db != nil {
				// Create workspace membership repository.
				workspaceMemberRepo := repository.NewWorkspaceMemberRepository(wr.db)

				// Get current workspace for the account.
				currentTenant, err := workspaceMemberRepo.GetCurrentWorkspace(c.Request.Context(), createAccountID)
				if err == nil && currentTenant != nil {
					workspaceID = currentTenant.WorkspaceID
					logger.Info("Using current workspace_id from account", "accountID", createAccountID, "workspaceID", workspaceID)
				} else {
					logger.Warn("Failed to get current workspace for account, falling back to workflow workspace scope", "accountID", createAccountID, "error", err)
				}
			}
		}

		// Restore both canonical workspace_id and legacy tenant_id for downstream compatibility.
		util.SetWorkspaceScopeCompat(c, workspaceID)
	}

	agentID := workflow.AgentID
	workflowType := string(workflow.Type)

	// Use different request structure based on workflow type
	// For conversation workflow, use AdvancedChatDraftWorkflowRunRequest
	var chatReq dto.AdvancedChatDraftWorkflowRunRequest
	if err := c.ShouldBindJSON(&chatReq); err != nil {
		logger.WarnContext(c.Request.Context(), "failed to bind chat request", "version_uuid", versionUUID, err)
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	// Convert to DraftWorkflowRunRequest for unified processing
	req := dto.DraftWorkflowRunRequest{
		Inputs:       chatReq.Inputs,
		ResponseMode: chatReq.ResponseMode,
		UserID:       chatReq.UserID,
		Files:        chatReq.Files,
	}

	// Add chat-specific fields to inputs
	if req.Inputs == nil {
		req.Inputs = make(map[string]interface{})
	}
	req.Inputs["query"] = chatReq.Query
	req.Inputs["conversation_id"] = chatReq.ConversationID

	h.runWorkflowByVersionUUIDInternal(c, versionUUID, agentID, workspaceID, accountID, workflowType, &req)
}

// runWorkflowByVersionUUIDInternal handles the actual workflow execution (kept for backward compatibility)
func (h *WorkflowHandler) runWorkflowByVersionUUIDInternal(c *gin.Context, versionUUID, agentID, tenantID, accountID, workflowType string, req *dto.DraftWorkflowRunRequest) {

	// Set context values for workflow execution
	ctx := context.WithValue(c.Request.Context(), "invoke_from", string(InvokeFromWebApp))
	ctx = context.WithValue(ctx, "created_from", "web-app")
	ctx = context.WithValue(ctx, "created_by_role", "account")
	ctx = context.WithValue(ctx, "version_uuid", versionUUID)

	// Update context in gin.Context
	c.Request = c.Request.WithContext(ctx)

	// Validate workflow inputs before execution
	workflow, err := h.workflowService.GetLatestPublishedWorkflow(ctx, tenantID, agentID, true)
	if err != nil {
		logger.CriticalContext(ctx, "failed to get workflow for validation", "agent_id", agentID, "version_uuid", versionUUID, err)
		response.Fail(c, response.ErrAppNotFound)
		return
	}

	if err := h.validateWorkflowInputs(ctx, workflow, req.Inputs); err != nil {
		logger.WarnContext(ctx, "workflow input validation failed", "agent_id", agentID, "version_uuid", versionUUID, err)
		response.FailWithMessage(c, response.ErrInvalidParam, err.Error())
		return
	}

	// Check response mode - if streaming, use streaming handler
	if req.ResponseMode == "streaming" {
		// Add version_uuid to inputs for tracking
		if req.Inputs == nil {
			req.Inputs = make(map[string]interface{})
		}
		req.Inputs["sys.version_uuid"] = versionUUID

		// Determine workflow run type based on workflow type
		runType := "WORKFLOW"
		if workflowType == "chat" {
			runType = "CONVERSATION_WORKFLOW"
			req.Inputs["sys.workflow_type"] = "chat"

			query, _ := req.Inputs["query"].(string)
			conversationID, _ := req.Inputs["conversation_id"].(string)

			if conversationID != "" {
				logger.DebugContext(c.Request.Context(), "workflow version run continuing conversation", zap.String("conversation_id", conversationID))

				latestMessageID, err := h.getLatestMessageID(conversationID)
				if err == nil && latestMessageID != "" {
					req.Inputs["sys.parent_message_id"] = latestMessageID
					logger.DebugContext(c.Request.Context(), "workflow version run parent message set", zap.Bool("has_parent_message_id", true))
				}

				req.Inputs["sys.conversation_id"] = conversationID
				req.Inputs["sys.dialogue_count"] = h.getDialogueCount(conversationID)
			} else {
				req.Inputs["sys.conversation_id"] = ""
				req.Inputs["sys.parent_message_id"] = ""
				req.Inputs["sys.dialogue_count"] = 1
				logger.DebugContext(c.Request.Context(), "workflow version run starting new conversation")
			}

			if query != "" {
				req.Inputs["sys.query"] = query
			}

			if req.Inputs["conversation_params"] == nil {
				req.Inputs["conversation_params"] = map[string]interface{}{
					"from_source": "account",
					"invoke_from": string(InvokeFromWebApp),
				}
			}
		}

		// Run workflow with streaming (isDraft=false for published version)
		h.runWorkflowStream(c, tenantID, agentID, req, accountID, false, runType, "web-app")
		return
	}

	// Non-streaming mode: use service layer
	result, err := h.workflowService.RunWorkflowByVersionUUID(ctx, versionUUID, req, accountID)
	if err != nil {
		logger.CriticalContext(ctx, "failed to run workflow by version uuid", "agent_id", agentID, "version_uuid", versionUUID, err)
		if strings.Contains(err.Error(), "not found") {
			response.Fail(c, response.ErrAppNotFound)
		} else {
			response.Fail(c, response.ErrSystemError)
		}
		return
	}

	response.Success(c, result)
}

// @Accept json
// @Produce json
// @Param web_app_id path string true "Web App ID"
// @Param request body dto.DraftWorkflowRunRequest true "Run request"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} response.ErrorResponse
// @Failure 404 {object} response.ErrorResponse
// @Failure 500 {object} response.ErrorResponse
// @Router /workflows/{web_app_id}/run [post]
func (h *WorkflowHandler) RunWorkflowByWebAppID(c *gin.Context) {
	webAppID := c.Param("web_app_id")

	accountID := c.GetString("account_id")
	requestedWorkspaceID := util.GetWorkspaceID(c)

	var err error

	logger.Info("Running workflow by web_app_id", "webAppID", webAppID, "accountID", accountID)

	// Get agent by web_app_id
	var agent *agents.Agent
	if ws, ok := h.workflowService.(*WorkflowService); ok && ws.agentsRepo != nil {
		agent, err = ws.agentsRepo.GetByWebAppID(c.Request.Context(), webAppID)
		if err != nil {
			logger.ErrorContext(c.Request.Context(), "failed to get agent by web app id", "web_app_id", webAppID, err)
			response.Fail(c, response.ErrAppNotFound)
			return
		}
	} else {
		logger.CriticalContext(c.Request.Context(), "workflow service or agents repository not available", fmt.Errorf("service not properly initialized"))
		response.Fail(c, response.ErrSystemError)
		return
	}
	if rejectInactiveWebApp(c, agent, webAppID) {
		return
	}

	fallbackWorkspaceID := ""
	if !isUnsetWorkflowWorkspaceID(agent.TenantID.String()) {
		fallbackWorkspaceID = agent.TenantID.String()
	}

	shadowWorkspaceService, _ := h.enterpriseService.(shadowWorkspaceEnsurer)
	runScope, err := resolveWebAppRunScope(
		c.Request.Context(),
		h.accountService,
		h.enterpriseService,
		shadowWorkspaceService,
		accountID,
		agent,
		fallbackWorkspaceID,
	)
	if err != nil {
		logger.CriticalContext(c.Request.Context(), "failed to resolve scope for web app workflow run", "web_app_id", webAppID, "account_id", accountID, err)
		response.Fail(c, response.ErrSystemError)
		return
	}

	workspaceID := runScope.WorkspaceID
	if workspaceID == "" {
		logger.WarnContext(c.Request.Context(), "caller has no available workspace for web app workflow run", "web_app_id", webAppID, "account_id", accountID, "agent_id", agent.ID.String())
		response.Fail(c, response.ErrWorkspaceNotFound)
		return
	}

	logger.Info("Resolved workspace for web app workflow run", "requestedWorkspaceID", requestedWorkspaceID, "workspaceID", workspaceID, "accountID", accountID, "agentID", agent.ID.String())
	util.SetWorkspaceScopeCompat(c, workspaceID)

	agentID := agent.ID.String()

	// Get latest published workflow for this agent
	workflowInterface, err := h.workflowService.GetLatestPublishedWorkflow(c.Request.Context(), workspaceID, agentID, true)
	if err != nil {
		logger.ErrorContext(c.Request.Context(), "failed to get latest published workflow", "web_app_id", webAppID, "agent_id", agentID, "workspace_id", workspaceID, err)
		response.FailWithMessage(c, response.ErrAppNotFound, "workflow not found")
		return
	}

	// Extract workflow type from the response map
	workflowMap, ok := workflowInterface.(map[string]interface{})
	if !ok {
		logger.CriticalContext(c.Request.Context(), "failed to cast workflow to map", "web_app_id", webAppID, "agent_id", agentID, fmt.Errorf("invalid type"))
		response.Fail(c, response.ErrSystemError)
		return
	}

	workflowType, ok := workflowMap["type"].(string)
	if !ok {
		logger.CriticalContext(c.Request.Context(), "failed to extract workflow type", "web_app_id", webAppID, "agent_id", agentID, fmt.Errorf("type field missing or invalid"))
		response.Fail(c, response.ErrSystemError)
		return
	}

	// Use different request structure based on workflow type
	// For conversation workflow, use AdvancedChatDraftWorkflowRunRequest
	var chatReq dto.AdvancedChatDraftWorkflowRunRequest
	if err := c.ShouldBindJSON(&chatReq); err != nil {
		logger.WarnContext(c.Request.Context(), "failed to bind chat request", "web_app_id", webAppID, "agent_id", agentID, err)
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	// Convert to DraftWorkflowRunRequest for unified processing
	req := dto.DraftWorkflowRunRequest{
		Inputs:       chatReq.Inputs,
		ResponseMode: chatReq.ResponseMode,
		UserID:       chatReq.UserID,
		Files:        chatReq.Files,
	}

	// Add chat-specific fields to inputs
	if req.Inputs == nil {
		req.Inputs = make(map[string]interface{})
	}
	req.Inputs["query"] = chatReq.Query
	req.Inputs["conversation_id"] = chatReq.ConversationID
	if runScope.OrganizationID != "" {
		req.Inputs["sys.organization_id"] = runScope.OrganizationID
	}
	if runScope.BillingSubjectType != "" {
		req.Inputs["sys.billing_subject_type"] = runScope.BillingSubjectType
	}

	h.runWorkflowByWebAppIDInternal(c, webAppID, agentID, workspaceID, accountID, workflowType, &req)

}

// runWorkflowByWebAppIDInternal handles the actual workflow execution
func (h *WorkflowHandler) runWorkflowByWebAppIDInternal(c *gin.Context, webAppID, agentID, tenantID, accountID, workflowType string, req *dto.DraftWorkflowRunRequest) {

	// Set context values for workflow execution
	ctx := context.WithValue(c.Request.Context(), "invoke_from", string(InvokeFromWebApp))
	ctx = context.WithValue(ctx, "created_from", "web-app")
	ctx = context.WithValue(ctx, "created_by_role", "account")
	ctx = context.WithValue(ctx, "web_app_id", webAppID)

	// Update context in gin.Context
	c.Request = c.Request.WithContext(ctx)

	// Validate workflow inputs before execution
	workflow, err := h.workflowService.GetLatestPublishedWorkflow(ctx, tenantID, agentID, true)
	if err != nil {
		logger.CriticalContext(ctx, "failed to get workflow for validation", "web_app_id", webAppID, "agent_id", agentID, err)
		response.Fail(c, response.ErrAppNotFound)
		return
	}

	if err := h.validateWorkflowInputs(ctx, workflow, req.Inputs); err != nil {
		logger.WarnContext(ctx, "workflow input validation failed", "web_app_id", webAppID, "agent_id", agentID, err)
		response.FailWithMessage(c, response.ErrInvalidParam, err.Error())
		return
	}

	// Add web_app_id to inputs for tracking
	if req.Inputs == nil {
		req.Inputs = make(map[string]interface{})
	}
	req.Inputs["sys.web_app_id"] = webAppID

	// Determine workflow run type based on workflow type
	runType := "WORKFLOW"
	if workflowType == "chat" {
		runType = "CONVERSATION_WORKFLOW"
		req.Inputs["sys.workflow_type"] = "chat"

		query, _ := req.Inputs["query"].(string)
		conversationID, _ := req.Inputs["conversation_id"].(string)

		if conversationID != "" {
			logger.DebugContext(c.Request.Context(), "web app workflow run continuing conversation", zap.String("conversation_id", conversationID))

			latestMessageID, err := h.getLatestMessageID(conversationID)
			if err == nil && latestMessageID != "" {
				req.Inputs["sys.parent_message_id"] = latestMessageID
				logger.DebugContext(c.Request.Context(), "web app workflow run parent message set", zap.Bool("has_parent_message_id", true))
			}

			req.Inputs["sys.conversation_id"] = conversationID
			req.Inputs["sys.dialogue_count"] = h.getDialogueCount(conversationID)
		} else {
			req.Inputs["sys.conversation_id"] = ""
			req.Inputs["sys.parent_message_id"] = ""
			req.Inputs["sys.dialogue_count"] = 1
			logger.DebugContext(c.Request.Context(), "web app workflow run starting new conversation")
		}

		if query != "" {
			req.Inputs["sys.query"] = query
		}

		if req.Inputs["conversation_params"] == nil {
			req.Inputs["conversation_params"] = map[string]interface{}{
				"from_source": "account",
				"invoke_from": string(InvokeFromWebApp),
			}
		}
	}

	// Run workflow with streaming (isDraft=false for published version)
	h.runWorkflowStream(c, tenantID, agentID, req, accountID, false, runType, "web-app")
	return

}
