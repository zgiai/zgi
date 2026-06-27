package workflow

import (
	"github.com/gin-gonic/gin"
	"github.com/zgiai/zgi/api/internal/dto"
	workspace_model "github.com/zgiai/zgi/api/internal/modules/workspace/model"
	"github.com/zgiai/zgi/api/pkg/logger"
	"github.com/zgiai/zgi/api/pkg/response"
)

// StopWorkflowTask handles POST /agents/{agent_id}/workflow-runs/tasks/{task_id}/stop
// @Summary Stop workflow task
// @Description Stop a running workflow task
// @Tags Workflow
// @Accept json
// @Produce json
// @Param agent_id path string true "App ID"
// @Param task_id path string true "Task ID"
// @Success 200 {object} dto.WorkflowTaskStopResponse
// @Failure 500 {object} response.ErrorResponse
// @Router /agents/{agent_id}/workflow-runs/tasks/{task_id}/stop [post]
func (h *WorkflowHandler) StopWorkflowTask(c *gin.Context) {
	appID := c.Param("agent_id")
	taskID := c.Param("task_id")
	accountID := c.GetString("account_id")
	workspaceID, ok := h.requireAgentWorkspacePermission(c, appID, workspace_model.WorkspacePermissionWorkflowRunStop)
	if !ok {
		return
	}

	logger.Info("Stopping workflow task", appID, taskID, accountID)

	err := h.workflowService.StopWorkflowTask(c.Request.Context(), workspaceID, appID, taskID, accountID)
	if err != nil {
		logger.CriticalContext(c.Request.Context(), "failed to stop workflow task", "agent_id", appID, "task_id", taskID, err)
		response.Fail(c, response.ErrSystemError)
		return
	}

	response.Success(c, nil)
}

// RunDraftWorkflowNode handles POST /agents/{agent_id}/workflows/draft/nodes/{node_id}/run
// @Summary Run draft workflow node
// @Description Execute a specific node in the draft workflow
// @Tags Workflow
// @Accept json
// @Produce json
// @Param agent_id path string true "App ID"
// @Param node_id path string true "Node ID"
// @Param request body dto.DraftWorkflowNodeRunRequest true "Run request"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} response.ErrorResponse
// @Failure 500 {object} response.ErrorResponse
// @Router /agents/{agent_id}/workflows/draft/nodes/{node_id}/run [post]
func (h *WorkflowHandler) RunDraftWorkflowNode(c *gin.Context) {
	appID := c.Param("agent_id")
	nodeID := c.Param("node_id")
	accountID := c.GetString("account_id")

	appWorkspaceID, ok := h.requireAgentWorkspacePermission(c, appID, workspace_model.WorkspacePermissionWorkflowDebug)
	if !ok {
		return
	}

	var req dto.DraftWorkflowNodeRunRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		logger.WarnContext(c.Request.Context(), "invalid request body", "agent_id", appID, "node_id", nodeID, err)
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	if err := h.validator.Struct(&req); err != nil {
		logger.WarnContext(c.Request.Context(), "request validation failed", "agent_id", appID, "node_id", nodeID, err)
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	logger.Info("Running draft workflow node", appID, nodeID, accountID)

	result, err := h.workflowService.RunDraftWorkflowNode(c.Request.Context(), appWorkspaceID, appID, nodeID, &req, accountID)
	if err != nil {
		logger.CriticalContext(c.Request.Context(), "failed to run draft workflow node", "agent_id", appID, "node_id", nodeID, err)
		response.Fail(c, response.ErrSystemError)
		return
	}

	response.Success(c, result)
}
