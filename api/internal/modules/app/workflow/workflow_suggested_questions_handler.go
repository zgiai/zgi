package workflow

import (
	"github.com/gin-gonic/gin"
	"github.com/zgiai/zgi/api/internal/dto"
	workspace_model "github.com/zgiai/zgi/api/internal/modules/workspace/model"
	"github.com/zgiai/zgi/api/pkg/logger"
	"github.com/zgiai/zgi/api/pkg/response"
)

// GenerateDraftWorkflowSuggestedQuestions handles
// POST /agents/{agent_id}/workflows/draft/suggested-questions/generate.
func (h *WorkflowHandler) GenerateDraftWorkflowSuggestedQuestions(c *gin.Context) {
	agentID := c.Param("agent_id")
	accountID := c.GetString("account_id")

	workspaceID, ok := h.requireAgentWorkspacePermission(c, agentID, workspace_model.WorkspacePermissionAgentManage)
	if !ok {
		return
	}

	var req dto.GenerateSuggestedQuestionsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		logger.WarnContext(c.Request.Context(), "invalid suggested questions request body", "agent_id", agentID, err)
		response.Fail(c, response.ErrInvalidParam)
		return
	}
	if err := h.validator.Struct(&req); err != nil {
		logger.WarnContext(c.Request.Context(), "suggested questions request validation failed", "agent_id", agentID, err)
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	result, err := h.workflowService.GenerateDraftWorkflowSuggestedQuestions(c.Request.Context(), workspaceID, agentID, &req, accountID)
	if err != nil {
		logger.ErrorContext(c.Request.Context(), "failed to generate workflow suggested questions", "agent_id", agentID, err)
		if isSuggestedQuestionsConfigurationError(err) {
			response.FailWithMessage(c, response.ErrConfigError, "Please configure a default LLM model before generating suggested questions.")
			return
		}
		if isSuggestedQuestionsModelOutputError(err) {
			response.FailWithMessage(c, response.ErrServiceUnavailable, "The model did not return usable suggested questions. Please try again.")
			return
		}
		response.FailWithMessage(c, response.ErrServiceUnavailable, "Failed to generate suggested questions. Please try again.")
		return
	}

	response.Success(c, result)
}
