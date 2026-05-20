package handler

import (
	"github.com/gin-gonic/gin"
	"github.com/zgiai/ginext/internal/modules/workspace/service"
	"github.com/zgiai/ginext/middleware"
	"github.com/zgiai/ginext/pkg/response"
)

// WorkspacePermissionFilterHandler handles workspace permission filtering HTTP requests
type WorkspacePermissionFilterHandler struct {
	workspacePermissionFilterService service.WorkspacePermissionFilterService
}

// NewWorkspacePermissionFilterHandler creates a new instance of WorkspacePermissionFilterHandler
func NewWorkspacePermissionFilterHandler(
	workspacePermissionFilterService service.WorkspacePermissionFilterService,
) *WorkspacePermissionFilterHandler {
	return &WorkspacePermissionFilterHandler{
		workspacePermissionFilterService: workspacePermissionFilterService,
	}
}

// GetAccessibleWorkspacesRequest represents the request payload
type GetAccessibleWorkspacesRequest struct {
	Type string `form:"type" binding:"required,oneof=create_agent create_database create_knowledge"`
}

// GetAccessibleWorkspacesResponse represents the response payload
type GetAccessibleWorkspacesResponse struct {
	Data  []*service.WorkspacePermissionResponse `json:"data"`
	Total int                                    `json:"total"`
}

// GetAccessibleWorkspaces handles HTTP request for accessible workspaces.
// GET /api/v1/workspace/organizations/:organization_id/accessible-workspaces?type=create_agent
func (h *WorkspacePermissionFilterHandler) GetAccessibleWorkspaces(c *gin.Context) {
	// Extract account ID from context (set by auth middleware)
	accountID := c.GetString("account_id")
	if accountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	// Extract organization ID from URL path.
	organizationID := c.Param("organization_id")
	if organizationID == "" {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	// Parse query parameters
	var req GetAccessibleWorkspacesRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	// Validate permission type
	if req.Type == "" {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	// Call service to get accessible workspaces
	workspaces, err := h.workspacePermissionFilterService.GetAccessibleWorkspacesByPermission(
		c.Request.Context(),
		accountID,
		organizationID,
		req.Type,
	)

	// Handle errors
	if err != nil {
		errMsg := err.Error()

		// Check for invalid permission type error
		if len(errMsg) > 24 && errMsg[:24] == "invalid permission type:" {
			response.Fail(c, response.ErrInvalidParam)
			return
		}

		if errMsg == "organization not found" || errMsg == "enterprise group not found" {
			response.Fail(c, response.ErrOrganizationNotFound)
			return
		}

		// Check for database errors
		if len(errMsg) > 6 && errMsg[:6] == "failed" {
			response.Fail(c, response.ErrSystemError)
			return
		}

		// Default to system error
		response.Fail(c, response.ErrSystemError)
		return
	}

	// Format response
	if workspaces == nil {
		workspaces = []*service.WorkspacePermissionResponse{}
	}

	resp := GetAccessibleWorkspacesResponse{
		Data:  workspaces,
		Total: len(workspaces),
	}

	response.Success(c, resp)
}

func (h *WorkspacePermissionFilterHandler) RegisterRoutes(router *gin.RouterGroup) {
	organizations := router.Group("/workspace/organizations", middleware.JWT())
	{
		organizations.GET("/:organization_id/accessible-workspaces", h.GetAccessibleWorkspaces)
	}
	groups := router.Group("/workspace/groups", middleware.JWT())
	{
		groups.GET("/:organization_id/accessible-tenants", h.GetAccessibleWorkspaces)
	}
}
