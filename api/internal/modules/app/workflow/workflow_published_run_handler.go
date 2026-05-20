package workflow

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/zgiai/ginext/internal/dto"
	"github.com/zgiai/ginext/internal/util"
	"github.com/zgiai/ginext/pkg/logger"
	"github.com/zgiai/ginext/pkg/response"
)

func (h *WorkflowHandler) RunPublishedWorkflow(c *gin.Context) {
	appID := c.Param("agent_id")
	accountID := c.GetString("account_id")
	requestedWorkspaceID := util.GetWorkspaceID(c)
	organizationID := util.GetOrganizationID(c)

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

	logger.Info("Running published workflow for app", "appID", appID, "invokeFrom", invokeFrom, "createdFrom", createdFrom)

	var req dto.DraftWorkflowRunRequest

	// Check if request body is empty
	if c.Request.Body == nil {
		logger.WarnContext(c.Request.Context(), "request body is nil", "agent_id", appID, fmt.Errorf("request body is nil"))
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	// Check Content-Type header
	contentType := c.GetHeader("Content-Type")
	if contentType != "" && !strings.Contains(contentType, "application/json") {
		logger.WarnContext(c.Request.Context(), "invalid content type", "agent_id", appID, "content_type", contentType)
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	// Check Content-Length header
	contentLength := c.GetHeader("Content-Length")
	logger.Info("Request details", "content_type", contentType, "content_length", contentLength)

	if err := c.ShouldBindJSON(&req); err != nil {
		logger.WarnContext(c.Request.Context(), "invalid request body", "agent_id", appID, "content_type", contentType, "content_length", contentLength, err)

		// Provide more specific error message
		if err == io.EOF {
			response.FailWithMessage(c, response.ErrInvalidParam, "Request body is empty")
		} else {
			response.FailWithMessage(c, response.ErrInvalidParam, "Invalid JSON format")
		}
		return
	}

	if err := h.validator.Struct(&req); err != nil {
		logger.WarnContext(c.Request.Context(), "request validation failed", "agent_id", appID, err)
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	logger.Info("Running published workflow", appID, accountID)

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

	// Always use streaming mode
	// Update context in gin.Context
	c.Request = c.Request.WithContext(ctx)
	// Determine runType and triggeredFrom
	runType := "WORKFLOW"       // For published workflows
	triggeredFrom := invokeFrom // Use invokeFrom from context (e.g., "external-api", "web-app")
	if triggeredFrom == "" {
		triggeredFrom = "app-run" // Default for published workflows
	}
	h.runWorkflowStream(c, requestedWorkspaceID, appID, &req, accountID, false, runType, triggeredFrom)
}
