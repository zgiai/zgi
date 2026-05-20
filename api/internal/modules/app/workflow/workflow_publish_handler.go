package workflow

import (
	"fmt"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/zgiai/ginext/internal/dto"
	workspace_model "github.com/zgiai/ginext/internal/modules/workspace/model"
	"github.com/zgiai/ginext/internal/util"
	"github.com/zgiai/ginext/pkg/logger"
	"github.com/zgiai/ginext/pkg/response"
)

func (h *WorkflowHandler) PublishWorkflow(c *gin.Context) {
	logger.Info("PublishWorkflow handler started")

	// Extract parameters from URL
	agentID := c.Param("agent_id")
	logger.Info("Extracted agent ID", "agent_id", agentID)
	if agentID == "" {
		logger.WarnContext(c.Request.Context(), "agent id is empty", fmt.Errorf("agent_id parameter is missing"))
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	// Get account ID from context (set by middleware)
	logger.Info("Getting account ID from context")
	accountID := c.GetString("account_id")
	if accountID == "" {
		logger.WarnContext(c.Request.Context(), "account id not found in context", fmt.Errorf("account_id missing from gin context"))
		response.Fail(c, response.ErrUnauthorized)
		return
	}
	logger.Info("Got account ID", "account_id", accountID)

	workspaceID, ok := h.resolveAgentWorkspaceID(c, agentID)
	if !ok {
		return
	}
	logger.Info("Got agent workspace ID", "workspace_id", workspaceID)

	if h.enterpriseService != nil {
		hasPermission, err := h.enterpriseService.CheckWorkspacePermission(
			c.Request.Context(),
			util.GetOrganizationID(c),
			workspaceID,
			accountID,
			workspace_model.WorkspacePermissionAgentManage,
		)
		if err != nil {
			response.Fail(c, response.ErrSystemError)
			return
		}
		if !hasPermission {
			response.Fail(c, response.ErrPermissionDenied)
			return
		}
	}

	// Parse request body (optional parameters for publishing)
	logger.Info("Parsing request body")
	var req dto.PublishWorkflowRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		// If no body provided, use empty request (optional parameters)
		logger.Info("No request body provided, using empty request")
		req = dto.PublishWorkflowRequest{}
	}
	logger.Info("Request parsed successfully", "req", req)

	// Call service to publish workflow
	logger.Info("Calling workflow service to publish workflow")
	result, err := h.workflowService.PublishWorkflow(
		c.Request.Context(),
		workspaceID,
		agentID,
		req,
		accountID,
	)
	if err != nil {
		logger.CriticalContext(c.Request.Context(), "failed to publish workflow", "agent_id", agentID, err)
		response.Fail(c, response.ErrSystemError)
		return
	}
	logger.Info("Workflow published successfully", "result", result)

	response.Success(c, result)
}

// GetLatestWorkflowVersion handles GET /agents/:agent_id/workflows/latest-version
// @Summary Get latest workflow version
// @Description Get the web_app_id for an agent's workflow
// @Tags Workflow
// @Accept json
// @Produce json
// @Param agent_id path string true "Agent ID"
// @Success 200 {object} map[string]interface{}
// @Failure 500 {object} response.ErrorResponse
// @Router /agents/{agent_id}/workflows/latest-version [get]
func (h *WorkflowHandler) GetLatestWorkflowVersion(c *gin.Context) {
	agentID := c.Param("agent_id")
	workspaceID, ok := h.requireAgentWorkspacePermission(c, agentID, workspace_model.WorkspacePermissionAgentView)
	if !ok {
		return
	}

	logger.Info("Getting workflow info for agent", "agentID", agentID)

	result := map[string]interface{}{}

	// Get agent to retrieve web_app_id
	ws, ok := h.workflowService.(*WorkflowService)
	if !ok || ws.agentsRepo == nil {
		logger.CriticalContext(c.Request.Context(), "workflow service or agents repository not available", "agent_id", agentID)
		response.Fail(c, response.ErrSystemError)
		return
	}

	agent, err := ws.agentsRepo.GetByID(c.Request.Context(), agentID)
	if err != nil {
		logger.Warn("Agent not found", "agentID", agentID, "error", err)
		// Return empty result instead of 404
		response.Success(c, result)
		return
	}

	// Add web_app_id from agent
	if agent.WebAppID != uuid.Nil {
		result["web_app_id"] = agent.WebAppID.String()
	}
	// Get latest published workflow to get workflow_id
	workflowData, err := h.workflowService.GetLatestPublishedWorkflow(c.Request.Context(), workspaceID, agentID, true)
	if err != nil {
		logger.Warn("No published workflow found", "agentID", agentID, "error", err)
		// Return result with only web_app_id if available, no 404
		response.Success(c, result)
		return
	}

	// Parse workflow data
	workflowMap, ok := workflowData.(map[string]interface{})
	if !ok {
		logger.Warn("Invalid workflow data format", "agentID", agentID)
		// Return result with only web_app_id if available, no error
		response.Success(c, result)
		return
	}

	// Add workflow_id
	if workflowID, exists := workflowMap["id"]; exists {
		result["workflow_id"] = workflowID
	}
	if versionUUID, exists := workflowMap["version_uuid"]; exists {
		if versionUUIDString, ok := versionUUID.(string); ok && strings.TrimSpace(versionUUIDString) != "" {
			result["version_uuid"] = versionUUIDString
		}
	}
	if _, exists := result["version_uuid"]; !exists {
		workflowID, exists := result["workflow_id"]
		if !exists {
			response.Success(c, result)
			return
		}
		result["version_uuid"] = workflowID
	}

	response.Success(c, result)
}
