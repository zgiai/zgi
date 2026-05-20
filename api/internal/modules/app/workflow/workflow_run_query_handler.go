package workflow

import (
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/zgiai/zgi/api/internal/dto"
	workspace_model "github.com/zgiai/zgi/api/internal/modules/workspace/model"
	"github.com/zgiai/zgi/api/internal/util"
	"github.com/zgiai/zgi/api/pkg/logger"
	"github.com/zgiai/zgi/api/pkg/response"
)

// GetWorkflowRuns handles GET /agents/{agent_id}/workflow-runs
// @Summary Get workflow runs
// @Description Get workflow runs for the specified agent
// @Tags Workflow
// @Accept json
// @Produce json
// @Param agent_id path string true "Agent ID"
// @Param page query int false "Page number" default(1)
// @Param limit query int false "Limit per page" default(20)
// @Param status query string false "Filter by status" Enums(succeeded,failed,stopped,running)
// @Param keyword query string false "Search keyword"
// @Success 200 {object} dto.WorkflowRunsResponse
// @Failure 400 {object} response.ErrorResponse
// @Failure 500 {object} response.ErrorResponse
// @Router /agents/{agent_id}/workflow-runs [get]
func (h *WorkflowHandler) GetWorkflowRuns(c *gin.Context) {
	agentID := c.Param("agent_id")
	organizationID := util.GetOrganizationID(c)
	accountID := c.GetString("account_id")

	// Verify permission first
	if accountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	appWorkspaceID, err := h.workflowService.GetAgentWorkspaceID(c.Request.Context(), agentID)
	if err != nil {
		logger.CriticalContext(c.Request.Context(), "failed to get agent workspace id", "agent_id", agentID, err)
		if err.Error() == "agent not found" {
			response.Fail(c, response.ErrAppNotFound)
		} else {
			response.Fail(c, response.ErrSystemError)
		}
		return
	}

	// System workflows use the built-in tenant ID; authenticated users can view only their own runs.
	if !isSystemWorkflowTenantID(appWorkspaceID) {
		if h.enterpriseService != nil {
			hasPermission, err := h.enterpriseService.CheckWorkspacePermission(
				c.Request.Context(),
				organizationID,
				appWorkspaceID,
				accountID,
				workspace_model.WorkspacePermissionAgentView,
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
	}

	// Parse query parameters
	var req dto.WorkflowRunsRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		logger.WarnContext(c.Request.Context(), "invalid query parameters", "agent_id", agentID, err)
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	// Set default values
	if req.Page <= 0 {
		req.Page = 1
	}
	if req.Limit <= 0 {
		req.Limit = 20
	}

	logger.Info("Getting workflow runs", "agentID", agentID, "workspaceID", appWorkspaceID, "page", req.Page, "limit", req.Limit)

	// Call service to get workflow runs
	result, err := h.workflowService.GetWorkflowRuns(c.Request.Context(), agentID, &req, appWorkspaceID, accountID)
	if err != nil {
		logger.CriticalContext(c.Request.Context(), "failed to get workflow runs", "agent_id", agentID, "workspace_id", appWorkspaceID, err)
		response.Fail(c, response.ErrSystemError)
		return
	}

	response.Success(c, result)
}

// GetWorkflowRunDetail handles GET /agents/{agent_id}/workflow-runs/{run_id}
// @Summary Get workflow run detail
// @Description Get detailed information about a specific workflow run
// @Tags Workflow
// @Accept json
// @Produce json
// @Param agent_id path string true "Agent ID"
// @Param run_id path string true "Workflow Run ID"
// @Success 200 {object} dto.WorkflowRunDetailResponse
// @Failure 400 {object} response.ErrorResponse
// @Failure 404 {object} response.ErrorResponse
// @Failure 500 {object} response.ErrorResponse
// @Router /agents/{agent_id}/workflow-runs/{run_id} [get]
func (h *WorkflowHandler) GetWorkflowRunDetail(c *gin.Context) {
	agentID := c.Param("agent_id")
	runID := c.Param("run_id")
	workspaceID := util.GetWorkspaceID(c)
	if workspaceID == "" {
		resolvedWorkspaceID, ok := h.resolveAgentWorkspaceID(c, agentID)
		if !ok {
			return
		}
		workspaceID = resolvedWorkspaceID
	}

	logger.Info("Getting workflow run detail", "agentID", agentID, "runID", runID, "workspaceID", workspaceID)

	// Call service to get workflow run detail
	result, err := h.workflowService.GetWorkflowRunDetail(c.Request.Context(), workspaceID, agentID, runID)
	if err != nil {
		logger.ErrorContext(c.Request.Context(), "failed to get workflow run detail", "agent_id", agentID, "run_id", runID, "workspace_id", workspaceID, err)
		if strings.Contains(err.Error(), "not found") {
			response.Fail(c, response.ErrNotFound)
		} else {
			response.Fail(c, response.ErrSystemError)
		}
		return
	}

	response.Success(c, result)
}

// GetWorkflowRunNodeExecutions handles GET /agents/{agent_id}/workflow-runs/{run_id}/node-executions
// @Summary Get node executions for a workflow run
// @Description List all node execution logs for a specific workflow run
// @Tags Workflow
// @Accept json
// @Produce json
// @Param agent_id path string true "Agent ID"
// @Param run_id path string true "Workflow Run ID"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} response.ErrorResponse
// @Failure 500 {object} response.ErrorResponse
// @Router /agents/{agent_id}/workflow-runs/{run_id}/node-executions [get]
func (h *WorkflowHandler) GetWorkflowRunNodeExecutions(c *gin.Context) {
	agentID := c.Param("agent_id")
	runID := c.Param("run_id")
	workspaceID := util.GetWorkspaceID(c)
	if workspaceID == "" {
		resolvedWorkspaceID, ok := h.resolveAgentWorkspaceID(c, agentID)
		if !ok {
			return
		}
		workspaceID = resolvedWorkspaceID
	}

	logger.Info("Getting workflow run node executions", "agentID", agentID, "runID", runID, "workspaceID", workspaceID)

	result, err := h.workflowService.GetWorkflowRunNodeExecutions(c.Request.Context(), workspaceID, agentID, runID)
	if err != nil {
		logger.CriticalContext(c.Request.Context(), "failed to get workflow run node executions", "agent_id", agentID, "run_id", runID, "workspace_id", workspaceID, err)
		response.Fail(c, response.ErrSystemError)
		return
	}

	response.Success(c, result)
}
