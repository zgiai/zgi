package workflow

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
	"github.com/zgiai/zgi/api/internal/dto"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/diagnosis"
	workflow_interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	workspace_model "github.com/zgiai/zgi/api/internal/modules/workspace/model"
	"github.com/zgiai/zgi/api/pkg/logger"
	"github.com/zgiai/zgi/api/pkg/response"
)

// WorkflowHandler handles workflow-related HTTP requests
type WorkflowHandler struct {
	workflowService            workflow_interfaces.WorkflowService
	agentWorkspaceResolver     workflowAgentWorkspaceResolver
	workspacePermissionChecker workflowWorkspacePermissionChecker
	accountService             workflow_interfaces.AccountService
	fileService                workflow_interfaces.FileService
	enterpriseService          workflow_interfaces.OrganizationService
	userMigrationService       UserMigrationService
	webAppMigrationAuthorizer  WebAppMigrationAuthorizer
	diagnoser                  *diagnosis.Diagnoser
	validator                  *validator.Validate
	advancedChatHandler        *AdvancedChatWorkflowHandler
}

// NewWorkflowHandler creates a new workflow handler
func NewWorkflowHandler(
	workflowService workflow_interfaces.WorkflowService,
	accountService workflow_interfaces.AccountService,
	fileService workflow_interfaces.FileService,
	userMigrationService UserMigrationService,
	enterpriseService workflow_interfaces.OrganizationService,
) *WorkflowHandler {
	return &WorkflowHandler{
		workflowService:            workflowService,
		agentWorkspaceResolver:     workflowService,
		workspacePermissionChecker: enterpriseService,
		accountService:             accountService,
		fileService:                fileService,
		enterpriseService:          enterpriseService,
		userMigrationService:       userMigrationService,
		validator:                  validator.New(),
		advancedChatHandler:        NewAdvancedChatWorkflowHandler(),
	}
}

// SetDiagnoser sets the diagnoser for the handler
func (h *WorkflowHandler) SetDiagnoser(diagnoser *diagnosis.Diagnoser) {
	h.diagnoser = diagnoser
}

// ==========================================
// Draft Workflow APIs
// ==========================================

// GetDraftWorkflow handles GET /agents/{agent_id}/workflows/draft
// @Summary Get draft workflow
// @Description Get the draft workflow for the specified app
// @Tags Workflow
// @Accept json
// @Produce json
// @Param agent_id path string true "App ID"
// @Success 200 {object} dto.WorkflowDetail
// @Failure 400 {object} response.ErrorResponse
// @Failure 404 {object} response.ErrorResponse
// @Failure 500 {object} response.ErrorResponse
// @Router /agents/{agent_id}/workflows/draft [get]
func (h *WorkflowHandler) GetDraftWorkflow(c *gin.Context) {
	appID := c.Param("agent_id")
	accountID := c.GetString("account_id")

	if _, ok := h.requireAgentWorkspacePermission(c, appID, workspace_model.WorkspacePermissionAgentView); !ok {
		return
	}

	logger.Info("Getting draft workflow", appID, accountID)

	workflow, err := h.workflowService.GetDraftWorkflow(c.Request.Context(), appID, true)
	if err != nil {
		logger.CriticalContext(c.Request.Context(), "failed to get draft workflow", "agent_id", appID, err)
		if err.Error() == "draft workflow not found" {
			response.Fail(c, response.ErrAppNotFound)
		} else {
			response.Fail(c, response.ErrSystemError)
		}
		return
	}

	response.Success(c, workflow)
}

// SyncDraftWorkflow handles POST /agents/{agent_id}/workflows/draft
// @Summary Sync draft workflow
// @Description Sync (create or update) the draft workflow
// @Tags Workflow
// @Accept json
// @Produce json
// @Param agent_id path string true "App ID"
// @Param request body dto.SyncDraftWorkflowRequest true "Sync request"
// @Success 200 {object} dto.SyncDraftWorkflowResponse
// @Failure 400 {object} response.ErrorResponse
// @Failure 500 {object} response.ErrorResponse
// @Router /agents/{agent_id}/workflows/draft [post]
func (h *WorkflowHandler) SyncDraftWorkflow(c *gin.Context) {
	appID := c.Param("agent_id")
	accountID := c.GetString("account_id")

	appWorkspaceID, ok := h.requireAgentWorkspacePermission(c, appID, workspace_model.WorkspacePermissionAgentManage)
	if !ok {
		return
	}

	var req dto.SyncDraftWorkflowRequest
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

	logger.Info("Syncing draft workflow", appID, accountID)

	result, err := h.workflowService.SyncDraftWorkflow(c.Request.Context(), appWorkspaceID, appID, &req, accountID)
	if err != nil {
		logger.CriticalContext(c.Request.Context(), "failed to sync draft workflow", "agent_id", appID, err)
		if err.Error() == "workflow hash mismatch, please refresh and try again" {
			response.Fail(c, response.ErrInvalidParam)
		} else {
			response.Fail(c, response.ErrSystemError)
		}
		return
	}

	response.Success(c, result)
}

// ==========================================
// Workflow Configuration APIs
// ==========================================

// GetWorkflowConfig handles GET /agents/{agent_id}/workflows/draft/config
// @Summary Get workflow config
// @Description Get the workflow configuration
// @Tags Workflow
// @Accept json
// @Produce json
// @Param agent_id path string true "App ID"
// @Success 200 {object} dto.WorkflowConfigResponse
// @Failure 500 {object} response.ErrorResponse
// @Router /agents/{agent_id}/workflows/draft/config [get]
func (h *WorkflowHandler) GetWorkflowConfig(c *gin.Context) {
	appID := c.Param("agent_id")
	workspaceID, ok := h.requireAgentWorkspacePermission(c, appID, workspace_model.WorkspacePermissionAgentView)
	if !ok {
		return
	}

	logger.Info("Getting workflow config", "appID", appID)

	config, err := h.workflowService.GetWorkflowConfig(c.Request.Context(), workspaceID, appID)
	if err != nil {
		logger.CriticalContext(c.Request.Context(), "failed to get workflow config", "agent_id", appID, err)
		response.Fail(c, response.ErrSystemError)
		return
	}

	response.Success(c, config)
}

// ==========================================
// Workflow Execution APIs
// ==========================================

// ==========================================
// Node Execution APIs
// ==========================================

// GetPublishedWorkflowVersions handles GET /agents/:agent_id/workflows/published-versions.
func (h *WorkflowHandler) GetPublishedWorkflowVersions(c *gin.Context) {
	agentID := c.Param("agent_id")
	if _, ok := h.requireAgentWorkspacePermission(c, agentID, workspace_model.WorkspacePermissionAgentView); !ok {
		return
	}

	ws, ok := h.workflowService.(*WorkflowService)
	if !ok {
		logger.CriticalContext(c.Request.Context(), "workflow service is not available", "agent_id", agentID)
		response.Fail(c, response.ErrSystemError)
		return
	}

	versions, total, err := ws.GetPublishedVersions(c.Request.Context(), agentID, 100, 0)
	if err != nil {
		logger.CriticalContext(c.Request.Context(), "failed to get published workflow versions", "agent_id", agentID, err)
		response.Fail(c, response.ErrSystemError)
		return
	}

	items := make([]map[string]interface{}, 0, len(versions))
	for _, version := range versions {
		if version == nil {
			continue
		}
		items = append(items, map[string]interface{}{
			"workflow_id":  version.ID,
			"version_uuid": workflowVersionSelectorID(version),
			"version":      version.Version,
			"created_at":   version.CreatedAt.Format(time.RFC3339),
		})
	}

	response.Success(c, map[string]interface{}{
		"data":  items,
		"total": total,
	})
}

// ManualDiagnoseRequest represents the request body for manual diagnosis
type ManualDiagnoseRequest struct {
	Model string `json:"model" binding:"required"`
}

// ManualDiagnoseNode handles POST /agents/:agent_id/workflow-runs/:run_id/nodes/:node_log_id/diagnose
// @Summary Manually diagnose a failed workflow node
// @Description Triggers an AI diagnosis for a specific node failure using the provided model
// @Tags Workflow
// @Accept json
// @Produce json
// @Param node_log_id path string true "Node Log ID"
// @Param request body ManualDiagnoseRequest true "Diagnosis parameters"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} response.ErrorResponse
// @Failure 500 {object} response.ErrorResponse
// @Router /agents/{agent_id}/workflow-runs/{run_id}/nodes/{node_log_id}/diagnose [post]
func (h *WorkflowHandler) ManualDiagnoseNode(c *gin.Context) {
	agentID := c.Param("agent_id")
	runID := c.Param("run_id")
	nodeLogID := c.Param("node_log_id")
	if agentID == "" || runID == "" || nodeLogID == "" {
		response.Fail(c, response.ErrInvalidParam)
		return
	}
	if _, ok := h.requireAgentWorkspacePermission(c, agentID, workspace_model.WorkspacePermissionAgentManage); !ok {
		return
	}
	if err := h.workflowService.ValidateWorkflowRunNodeScope(c.Request.Context(), agentID, runID, nodeLogID); err != nil {
		logger.WarnContext(c.Request.Context(), "workflow node diagnosis scope rejected", "agent_id", agentID, "run_id", runID, "node_log_id", nodeLogID, err)
		response.Fail(c, response.ErrNotFound)
		return
	}

	var req ManualDiagnoseRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	// Determine language from header
	lang := c.GetHeader("Accept-Language")
	if lang == "" {
		lang = "zh" // Default to Chinese
	}

	result, err := h.workflowService.ManualDiagnoseNode(c.Request.Context(), nodeLogID, req.Model, lang)
	if err != nil {
		logger.Error("Manual diagnosis failed", err)
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}

	response.Success(c, result)
}
