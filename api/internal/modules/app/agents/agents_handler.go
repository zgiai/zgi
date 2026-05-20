package agents

import (
	"context"
	"errors"
	"fmt"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/zgiai/ginext/internal/dto"
	interfaces "github.com/zgiai/ginext/internal/modules/shared/interface"
	workspace_model "github.com/zgiai/ginext/internal/modules/workspace/model"
	"github.com/zgiai/ginext/internal/util"
	"github.com/zgiai/ginext/pkg/logger"
	"github.com/zgiai/ginext/pkg/response"
	"gorm.io/gorm"
)

type AgentsHandler struct {
	appService          AgentsService
	tenantService       interfaces.WorkspaceManagementService
	accountService      interfaces.AccountService
	organizationService interfaces.OrganizationService
	db                  *gorm.DB
}

func NewAgentsHandler(appService AgentsService, tenantService interfaces.WorkspaceManagementService, accountService interfaces.AccountService, organizationService interfaces.OrganizationService, db *gorm.DB) *AgentsHandler {
	return &AgentsHandler{
		appService:          appService,
		tenantService:       tenantService,
		accountService:      accountService,
		organizationService: organizationService,
		db:                  db,
	}
}

func (h *AgentsHandler) GetAgentsList(c *gin.Context) {
	// 1. Authenticate user - Requirement 11.1: Handle authentication errors (401)
	accountID := c.GetString("account_id")
	if accountID == "" {
		// Requirement 11.1: Return 401 for missing account_id
		logger.Error("GetAgentsList: authentication failed - account_id not found in context", nil)
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	// Validate account_id format
	if _, err := uuid.Parse(accountID); err != nil {
		// Requirement 11.1: Return 401 for invalid account_id format
		// Requirement 11.5: Add structured logging with context
		logger.Error(fmt.Sprintf("GetAgentsList: authentication failed - invalid account_id format: account_id=%s", accountID), err)
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	// 2. Parse query parameters - Requirement 11.4: Handle invalid parameter errors (400)
	var req dto.GetAgentsListRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		// Log parameter binding errors but continue with defaults
		logger.Warn("GetAgentsList: failed to bind query parameters, using defaults", map[string]interface{}{
			"account_id": accountID,
			"error":      err.Error(),
		})
		// Continue with default values
	}

	// 3. Parse internal parameter correctly (handle "true"/"false" strings)
	if internalStr := c.Query("internal"); internalStr != "" {
		switch internalStr {
		case "true":
			internalVal := true
			req.Internal = &internalVal
		case "false":
			internalVal := false
			req.Internal = &internalVal
		default:
			// Requirement 11.4: Handle invalid parameter errors (400)
			logger.Warn("GetAgentsList: invalid internal parameter value", map[string]interface{}{
				"account_id": accountID,
				"value":      internalStr,
			})
			response.Fail(c, response.ErrInvalidParam)
			return
		}
	}

	// 4. Validate and normalize pagination parameters
	if req.Page <= 0 {
		req.Page = 1
	}
	if req.Page > 99999 {
		req.Page = 99999
	}

	pageSize := 20
	if req.PageSize > 0 {
		pageSize = req.PageSize
	} else if req.Limit > 0 {
		pageSize = req.Limit
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	req.Limit = pageSize

	// 5. Call new service method with permission-based filtering
	// Requirement 11.5: Add structured logging with context (account_id, org_id, dept_ids)
	logger.Info("GetAgentsList: calling GetAgentsListWithPermissions", map[string]interface{}{
		"account_id": accountID,
		"page":       req.Page,
		"limit":      req.Limit,
		"name":       req.Name,
		"keyword":    req.Keyword,
		"agent_type": req.AgentType,
		"internal":   req.Internal,
	})

	// Requirement 11.2, 11.3, 11.4: Handle service errors and map to HTTP responses
	result, err := h.appService.GetAgentsListWithPermissions(c.Request.Context(), accountID, req)
	if err != nil {
		// Requirement 11.5: Add structured logging with context
		logger.Error(fmt.Sprintf("GetAgentsList: service error for account_id=%s", accountID), err)

		// Requirement 11.2: Handle tenant not found errors (404)
		if err.Error() == "tenant not found" {
			response.Fail(c, response.ErrWorkspaceNotFound)
			return
		}

		// Requirement 11.3: Handle database errors (500) with logging
		if err.Error() == "failed to determine user permissions" ||
			err.Error() == "failed to get current organization" ||
			err.Error() == "failed to get organization departments" ||
			err.Error() == "failed to get user department memberships" ||
			err.Error() == "failed to retrieve agents" {
			response.Fail(c, response.ErrSystemError)
			return
		}

		// Default error response
		response.SpecialFail(c, gin.H{
			"code":    "399001",
			"message": err.Error(),
		})
		return
	}

	// 6. Return successful response with agent list and pagination info
	// Requirement 11.5: Add structured logging with context
	logger.Info("GetAgentsList: successfully retrieved agents", map[string]interface{}{
		"account_id":  accountID,
		"agent_count": len(result.Data),
		"total":       result.Total,
		"page":        result.Page,
		"limit":       result.Limit,
	})
	response.Success(c, result)
}

func (h *AgentsHandler) GetRunnableWebApps(c *gin.Context) {
	accountID := c.GetString("account_id")
	if accountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	if _, err := uuid.Parse(accountID); err != nil {
		logger.Error(fmt.Sprintf("GetRunnableWebApps: invalid account_id format: account_id=%s", accountID), err)
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	var req dto.GetRunnableWebAppsRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	if req.WorkspaceID != "" {
		if _, err := uuid.Parse(req.WorkspaceID); err != nil {
			response.Fail(c, response.ErrInvalidParam)
			return
		}
	}

	result, err := h.appService.GetRunnableWebApps(c.Request.Context(), accountID, req)
	if err != nil {
		switch {
		case errors.Is(err, errCurrentOrganizationNotFound):
			response.Fail(c, response.ErrOrganizationNotFound)
		default:
			logger.Error(fmt.Sprintf("GetRunnableWebApps: service error for account_id=%s", accountID), err)
			response.Fail(c, response.ErrSystemError)
		}
		return
	}

	response.Success(c, result)
}

func (h *AgentsHandler) CreateAgent(c *gin.Context) {
	accountID := c.GetString("account_id")
	if accountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	var req dto.CreateAgentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	callerOrganizationID := util.GetOrganizationID(c)
	if callerOrganizationID == "" {
		response.Fail(c, response.ErrOrganizationNotFound)
		return
	}

	targetWorkspaceID := req.WorkspaceID
	if targetWorkspaceID == "" {
		response.Fail(c, response.ErrWorkspaceNotFound)
		return
	}

	if h.organizationService != nil {
		hasPermission, err := h.organizationService.CheckWorkspacePermission(
			c.Request.Context(),
			callerOrganizationID,
			targetWorkspaceID,
			accountID,
			workspace_model.WorkspacePermissionAgentManage,
		)
		if err != nil {
			logger.Error("Failed to check create agent workspace permission", err)
			response.Fail(c, response.ErrSystemError)
			return
		}
		if !hasPermission {
			logger.Warn("CreateAgent: workspace permission denied", map[string]interface{}{
				"account_id":   accountID,
				"workspace_id": targetWorkspaceID,
			})
			response.Fail(c, response.ErrPermissionDenied)
			return
		}
	}

	result, err := h.appService.CreateAgent(c.Request.Context(), targetWorkspaceID, req, accountID)
	if err != nil {
		// Basic error mapping
		response.SpecialFail(c, gin.H{
			"code":    "399001",
			"message": err.Error(),
		})
		return
	}

	response.Success(c, result)
}

func (h *AgentsHandler) GetAgent(c *gin.Context) {
	accountID := c.GetString("account_id")
	if accountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	agentID := c.Param("agent_id")
	if agentID == "" {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	// Pass account_id via context for is_editor calculation
	ctx := c.Request.Context()
	ctx = context.WithValue(ctx, "account_id", accountID)
	if callerOrganizationID := util.GetOrganizationID(c); callerOrganizationID != "" {
		ctx = context.WithValue(ctx, "tenant_id", callerOrganizationID)
	}

	result, err := h.appService.GetAgent(ctx, agentID)
	if err != nil {
		switch err.Error() {
		case "permission denied":
			response.SpecialFail(c, gin.H{"code": "403001", "message": "Permission denied"})
		default:
			response.SpecialFail(c, gin.H{
				"code":    "399001",
				"message": err.Error(),
			})
		}
		return
	}

	response.Success(c, result)
}
func (h *AgentsHandler) UpdateAgent(c *gin.Context) {
	// Authenticate
	accountID := c.GetString("account_id")
	if accountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	agentID := c.Param("agent_id")
	if agentID == "" {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	callerOrganizationID := util.GetOrganizationID(c)
	if callerOrganizationID == "" {
		response.Fail(c, response.ErrOrganizationNotFound)
		return
	}

	// Bind update request (partial update)
	var req map[string]interface{}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	// Optional: cross-tenant update permission check when tenant_id provided
	if v, ok := req["tenant_id"].(string); ok && v != "" {
		hasPermission, err := h.organizationService.CheckWorkspacePermission(
			c.Request.Context(),
			callerOrganizationID,
			v,
			accountID,
			workspace_model.WorkspacePermissionAgentManage,
		)
		if err != nil {
			logger.Error("Failed to check workspace permission for update", err)
			response.Fail(c, response.ErrSystemError)
			return
		}
		if !hasPermission {
			response.Fail(c, response.ErrPermissionDenied)
			return
		}
	}

	// Pass account_id and caller organization context for permission verification in service
	ctx := context.WithValue(c.Request.Context(), "account_id", accountID)
	ctx = context.WithValue(ctx, "tenant_id", callerOrganizationID)
	result, err := h.appService.UpdateAgent(ctx, agentID, req)
	if err != nil {
		// Map specific errors to appropriate responses
		switch err.Error() {
		case "agent not found":
			response.SpecialFail(c, gin.H{"code": "404001", "message": "Agent not found"})
		case "permission denied":
			response.SpecialFail(c, gin.H{"code": "403001", "message": "Permission denied"})
		case "agent with the same name already exists":
			response.SpecialFail(c, gin.H{"code": "409001", "message": "Duplicated agent name in tenant"})
		default:
			response.SpecialFail(c, gin.H{"code": "399001", "message": err.Error()})
		}
		return
	}

	response.Success(c, result)
}

func (h *AgentsHandler) DeleteAgent(c *gin.Context) {
	// Get account ID from context for authentication
	accountID := c.GetString("account_id")
	if accountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	// Get agent ID from URL parameter
	agentID := c.Param("agent_id")
	if agentID == "" {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	callerOrganizationID := util.GetOrganizationID(c)
	if callerOrganizationID == "" {
		response.Fail(c, response.ErrOrganizationNotFound)
		return
	}

	// Pass account_id and caller organization context for permission validation
	ctx := c.Request.Context()
	ctx = context.WithValue(ctx, "account_id", accountID)
	ctx = context.WithValue(ctx, "tenant_id", callerOrganizationID)

	// Call service to delete agent with permission validation
	err := h.appService.DeleteAgent(ctx, agentID)
	if err != nil {
		// Map specific errors to appropriate responses
		switch err.Error() {
		case "agent not found":
			response.SpecialFail(c, gin.H{
				"code":    "404001",
				"message": "Agent not found",
			})
		case "permission denied":
			response.SpecialFail(c, gin.H{
				"code":    "403001",
				"message": "Permission denied",
			})
		default:
			response.SpecialFail(c, gin.H{
				"code":    "399001",
				"message": err.Error(),
			})
		}
		return
	}

	response.Success(c, gin.H{"message": "Agent deleted successfully"})
}
