package workflow

import (
	"github.com/gin-gonic/gin"
	"github.com/zgiai/zgi/api/pkg/logger"
	"github.com/zgiai/zgi/api/pkg/response"
)

// BuiltInWorkflowHandler handles built-in workflow HTTP requests
// Requirements: 3.1, 3.2, 3.3, 3.4
type BuiltInWorkflowHandler struct {
	service BuiltInWorkflowService
}

// NewBuiltInWorkflowHandler creates a new BuiltInWorkflowHandler instance
func NewBuiltInWorkflowHandler(service BuiltInWorkflowService) *BuiltInWorkflowHandler {
	return &BuiltInWorkflowHandler{
		service: service,
	}
}

// GetBuiltInWorkflows returns all built-in workflows
// @Summary Get all built-in workflows
// @Description Retrieves all system-provided built-in workflows with their agent IDs, workflow IDs, and metadata
// @Tags Built-in Workflows
// @Produce json
// @Success 200 {object} response.Response{data=[]dto.BuiltInWorkflowDTO} "List of all built-in workflows"
// @Failure 500 {object} response.Response "Internal server error"
// @Router /api/v1/built-in-workflows [get]
// Requirements: 3.1, 3.4
func (h *BuiltInWorkflowHandler) GetBuiltInWorkflows(c *gin.Context) {
	logger.Info("API: Getting all built-in workflows")

	// Call service to get all built-in workflows
	workflows, err := h.service.GetAllBuiltInWorkflows(c.Request.Context())
	if err != nil {
		logger.Error("Failed to get built-in workflows", err)
		response.Fail(c, response.ErrSystemError)
		return
	}

	logger.Info("Successfully retrieved built-in workflows", "count", len(workflows))
	response.Success(c, workflows)
}

// GetBuiltInWorkflowByScenario returns a specific built-in workflow by scenario name
// @Summary Get built-in workflow by scenario
// @Description Retrieves a specific built-in workflow by its business scenario name (e.g., "global_chat", "bi_chat")
// @Tags Built-in Workflows
// @Param scenario path string true "Business scenario name (e.g., global_chat, bi_chat)"
// @Produce json
// @Success 200 {object} response.Response{data=dto.BuiltInWorkflowDTO} "Built-in workflow details"
// @Failure 400 {object} response.Response "Invalid scenario name format"
// @Failure 403 {object} response.Response "Workflow exists but is not enabled"
// @Failure 404 {object} response.Response "Built-in workflow not found for the specified scenario"
// @Failure 500 {object} response.Response "Internal server error"
// @Router /api/v1/built-in-workflows/{scenario} [get]
// Requirements: 3.1, 3.2, 3.3
func (h *BuiltInWorkflowHandler) GetBuiltInWorkflowByScenario(c *gin.Context) {
	scenario := c.Param("scenario")
	logger.Info("API: Getting built-in workflow by scenario", "scenario", scenario)

	// Validate scenario parameter
	if scenario == "" {
		logger.Warn("Empty scenario parameter provided")
		response.FailWithMessage(c, response.ErrInvalidParam, "scenario parameter is required")
		return
	}

	// Call service to get workflow by scenario
	workflow, err := h.service.GetBuiltInWorkflowByScenario(c.Request.Context(), scenario)
	if err != nil {
		logger.Error("Failed to get built-in workflow by scenario", err)

		// Requirement 3.2, 3.3: Proper error responses based on error type
		errMsg := err.Error()

		// Check for validation errors (invalid scenario name format)
		if contains(errMsg, "invalid scenario name") {
			response.FailWithMessage(c, response.ErrInvalidParam, errMsg)
			return
		}

		// Check for not found errors
		// Requirement 3.2: Return 404 with clear error message
		if contains(errMsg, "not found") {
			response.FailWithMessage(c, response.ErrNotFound, errMsg)
			return
		}

		// Check for disabled/not enabled workflows
		// Requirement 3.3: Return 403 for disabled workflows
		if contains(errMsg, "not enabled") || contains(errMsg, "disabled") {
			response.FailWithMessage(c, response.ErrorCode{Code: 403001, Message: "Built-in workflow is not enabled", UserVisible: true}, errMsg)
			return
		}

		// Default to internal server error
		response.Fail(c, response.ErrSystemError)
		return
	}

	logger.Info("Successfully retrieved built-in workflow", "scenario", scenario, "agentID", workflow.AgentID)
	response.Success(c, workflow)
}

// contains checks if a string contains a substring (case-insensitive helper)
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
