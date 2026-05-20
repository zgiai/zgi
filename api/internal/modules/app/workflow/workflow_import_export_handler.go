package workflow

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
	workspace_model "github.com/zgiai/ginext/internal/modules/workspace/model"
	"github.com/zgiai/ginext/internal/util"
	"github.com/zgiai/ginext/pkg/logger"
	"github.com/zgiai/ginext/pkg/response"
)

// ExportWorkflow handles GET /agents/{agent_id}/workflows/export
func (h *WorkflowHandler) ExportWorkflow(c *gin.Context) {
	agentID := c.Param("agent_id")
	accountID := c.GetString("account_id")
	organizationID := util.GetOrganizationID(c)
	version := c.DefaultQuery("version", "draft")

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

	svc, ok := h.workflowService.(*WorkflowService)
	if !ok {
		response.Fail(c, response.ErrSystemError)
		return
	}

	data, filename, err := svc.ExportWorkflow(c.Request.Context(), agentID, version)
	if err != nil {
		logger.CriticalContext(c.Request.Context(), "failed to export workflow", "agent_id", agentID, "version", version, err)
		response.Fail(c, response.ErrSystemError)
		return
	}

	// Content-Disposition: use both filename (fallback) and filename* (RFC 5987 UTF-8) for compatibility
	// filename* takes precedence in modern browsers; filename provides fallback for older clients
	// Also URL-encode filename to support varied frontend extraction methods
	encodedFilename := url.PathEscape(filename)
	disposition := fmt.Sprintf(`attachment; filename="%s"; filename*=utf-8''%s`, url.QueryEscape(filename), encodedFilename)
	c.Header("Content-Disposition", disposition)
	c.Data(http.StatusOK, "application/x-yaml", data)
}

// ImportWorkflow handles POST /agents/workflows/import. Import only creates new agents.
func (h *WorkflowHandler) ImportWorkflow(c *gin.Context) {
	accountID := c.GetString("account_id")
	workspaceID := util.GetWorkspaceID(c)

	// Form workspace_id takes precedence over context workspace_id.
	if requestedWorkspaceID := c.PostForm("workspace_id"); requestedWorkspaceID != "" {
		workspaceID = requestedWorkspaceID
	}

	if accountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}
	if workspaceID == "" {
		response.Fail(c, response.ErrWorkspaceNotFound)
		return
	}

	organizationID := util.GetOrganizationID(c)
	if organizationID == "" {
		response.Fail(c, response.ErrOrganizationNotFound)
		return
	}
	if h.enterpriseService != nil {
		hasPermission, err := h.enterpriseService.CheckWorkspacePermission(
			c.Request.Context(),
			organizationID,
			workspaceID,
			accountID,
			workspace_model.WorkspacePermissionAgentManage,
		)
		if err != nil {
			logger.CriticalContext(c.Request.Context(), "failed to check import workflow workspace permission", "workspace_id", workspaceID, "account_id", accountID, err)
			response.Fail(c, response.ErrSystemError)
			return
		}
		if !hasPermission {
			response.Fail(c, response.ErrPermissionDenied)
			return
		}
	}

	// Import creates new agent. Verify user has editor permission in current workspace.
	isEditor, err := h.accountService.IsEditor(c.Request.Context(), accountID)
	if err != nil {
		response.Fail(c, response.ErrSystemError)
		return
	}
	if !isEditor {
		response.Fail(c, response.ErrPermissionDenied)
		return
	}

	file, err := c.FormFile("file")
	if err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, "file is required")
		return
	}

	if file.Size > 10*1024*1024 {
		response.FailWithMessage(c, response.ErrInvalidParam, "file size exceeds 10MB limit")
		return
	}

	ext := strings.ToLower(filepath.Ext(file.Filename))
	if ext != ".yml" && ext != ".yaml" {
		response.FailWithMessage(c, response.ErrInvalidParam, "unsupported file format, expected .yml or .yaml")
		return
	}

	f, err := file.Open()
	if err != nil {
		response.Fail(c, response.ErrSystemError)
		return
	}
	defer f.Close()

	fileData, err := io.ReadAll(f)
	if err != nil {
		response.Fail(c, response.ErrSystemError)
		return
	}

	svc, ok := h.workflowService.(*WorkflowService)
	if !ok {
		response.Fail(c, response.ErrSystemError)
		return
	}

	result, err := svc.ImportWorkflow(c.Request.Context(), workspaceID, accountID, fileData)
	if err != nil {
		logger.ErrorContext(c.Request.Context(), "failed to import workflow", "workspace_id", workspaceID, "account_id", accountID, err)
		response.FailWithMessage(c, response.ErrInvalidParam, err.Error())
		return
	}

	response.Success(c, result)
}
