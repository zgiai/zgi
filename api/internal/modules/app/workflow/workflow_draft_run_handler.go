package workflow

import (
	"github.com/gin-gonic/gin"
	"github.com/zgiai/zgi/api/internal/dto"
	workspace_model "github.com/zgiai/zgi/api/internal/modules/workspace/model"
	"github.com/zgiai/zgi/api/pkg/logger"
	"github.com/zgiai/zgi/api/pkg/response"
	"go.uber.org/zap"
)

// RunDraftWorkflow handles POST /agents/{agent_id}/workflows/draft/run
// @Summary Run draft workflow
// @Description Execute the draft workflow
// @Tags Workflow
// @Accept json
// @Produce json,text/event-stream
// @Param agent_id path string true "App ID"
// @Param request body dto.DraftWorkflowRunRequest true "Run request"
// @Success 200 {object} dto.WorkflowRunResponse
// @Success 200 {string} string "Server-Sent Events stream"
// @Failure 400 {object} response.ErrorResponse
// @Failure 500 {object} response.ErrorResponse
// @Router /agents/{agent_id}/workflows/draft/run [post]
func (h *WorkflowHandler) RunDraftWorkflow(c *gin.Context) {
	appID := c.Param("agent_id")
	accountID := c.GetString("account_id")

	appWorkspaceID, ok := h.requireAgentWorkspacePermission(c, appID, workspace_model.WorkspacePermissionAgentManage)
	if !ok {
		return
	}

	var req dto.DraftWorkflowRunRequest
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

	logger.Info("Running draft workflow", appID, accountID)

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

	if req.ResponseMode == "blocking" {
		result, err := h.workflowService.RunDraftWorkflow(c.Request.Context(), appWorkspaceID, appID, &req, accountID)
		if err != nil {
			logger.CriticalContext(c.Request.Context(), "failed to run blocking draft workflow", "agent_id", appID, err)
			response.Fail(c, response.ErrSystemError)
			return
		}
		response.Success(c, result)
		return
	}

	runType := "WORKFLOW"
	if agentsService, ok := h.workflowService.(*WorkflowService); ok && agentsService.agentsRepo != nil {
		agent, err := agentsService.agentsRepo.GetByID(c.Request.Context(), appID)
		if err == nil && agent != nil {
			agentType := agent.AgentsType
			if agentType == "chat" || agentType == "advanced-chat" || agentType == "CONVERSATIONAL_WORKFLOW" {
				runType = "CONVERSATION_WORKFLOW"
				logger.DebugContext(c.Request.Context(), "draft workflow run type resolved",
					zap.String("agent_type", agentType),
					zap.String("run_type", runType),
				)
			} else {
				logger.DebugContext(c.Request.Context(), "draft workflow run type resolved",
					zap.String("agent_type", agentType),
					zap.String("run_type", runType),
				)
			}
		} else {
			logger.WarnContext(c.Request.Context(), "failed to get agent, defaulting to workflow",
				zap.String("agent_id", appID),
				zap.Error(err),
			)
		}
	}

	if runType == "CONVERSATION_WORKFLOW" {
		if err := validateWorkflowInputConversationAccess(c.Request.Context(), h.advancedChatHandler, req.Inputs, appID, accountID); err != nil {
			logger.WarnContext(c.Request.Context(), "draft workflow run conversation access denied", "agent_id", appID, err)
			failWebAppConversationAccess(c, err)
			return
		}
		promoteWorkflowInputConversationIDToSystemInput(req.Inputs)
	}

	h.runWorkflowStream(c, appWorkspaceID, appID, &req, accountID, true, runType, "debugging")
}
