package handler

import (
	"context"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	interfaces "github.com/zgiai/ginext/internal/modules/shared/interface"
	"github.com/zgiai/ginext/internal/modules/workspace/model"
	workspace_service "github.com/zgiai/ginext/internal/modules/workspace/service"
	"github.com/zgiai/ginext/internal/util"
	"github.com/zgiai/ginext/middleware"
	"github.com/zgiai/ginext/pkg/response"
)

// WorkspaceHandler handles workspace-related HTTP requests
type WorkspaceHandler struct {
	workspaceService    workspace_service.WorkspaceService
	accountService      interfaces.AccountService
	organizationService interfaces.OrganizationService
}

var errWorkspaceOrganizationServiceUnavailable = errors.New("workspace organization service is unavailable")
var errWorkspaceOrganizationNotFound = errors.New("workspace organization not found")

// NewWorkspaceHandler creates a new workspace handler
func NewWorkspaceHandler(
	workspaceService workspace_service.WorkspaceService,
	accountService interfaces.AccountService,
	organizationService interfaces.OrganizationService,
) *WorkspaceHandler {
	return &WorkspaceHandler{
		workspaceService:    workspaceService,
		accountService:      accountService,
		organizationService: organizationService,
	}
}

func (h *WorkspaceHandler) resolveWorkspaceOrganizationID(ctx context.Context, workspaceID string) (string, error) {
	if h.organizationService == nil {
		return "", errWorkspaceOrganizationServiceUnavailable
	}

	organization, err := h.organizationService.GetOrganizationByWorkspaceID(ctx, workspaceID)
	if err != nil {
		return "", err
	}
	if organization == nil {
		return "", errWorkspaceOrganizationNotFound
	}

	return organization.ID, nil
}

// UploadWebappLogo handles POST /workspaces/custom-config/webapp-logo/upload - upload webapp logo
func (h *WorkspaceHandler) UploadWebappLogo(c *gin.Context) {
	// Check admin/owner permission
	if !middleware.IsAdminOrOwner(c) {
		response.Fail(c, response.ErrPermissionDenied)
		return
	}

	accountID := util.GetAccountID(c)
	organizationID := util.GetOrganizationID(c)
	userRole := ""

	// Parse multipart form
	file, err := c.FormFile("file")
	if err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	// Check file size (5MB limit)
	if file.Size > 5*1024*1024 {
		response.Fail(c, response.ErrFileTooLarge)
		return
	}

	// Check file type
	allowedTypes := []string{
		"image/jpeg",
		"image/jpg",
		"image/png",
		"image/webp",
		"image/svg+xml",
	}

	contentType := file.Header.Get("Content-Type")
	isAllowed := false
	for _, allowedType := range allowedTypes {
		if contentType == allowedType {
			isAllowed = true
			break
		}
	}

	if !isAllowed {
		response.Fail(c, response.ErrUnsupportedFileType)
		return
	}

	// Read file content
	fileContent, err := file.Open()
	if err != nil {
		response.Fail(c, response.ErrSystemError)
		return
	}
	defer fileContent.Close()

	content := make([]byte, file.Size)
	_, err = fileContent.Read(content)
	if err != nil {
		response.Fail(c, response.ErrSystemError)
		return
	}

	// Temporary response until file service is re-implemented
	_ = accountID                             // TODO: remove when file service is re-implemented
	_ = organizationID                        // TODO: remove when file service is re-implemented
	_ = userRole                              // TODO: remove when file service is re-implemented
	response.Fail(c, response.ErrSystemError) // TODO: change to proper error when implemented
}

// CreateWorkspace handles POST /workspaces/create - create new workspace (system admin only)
func (h *WorkspaceHandler) CreateWorkspace(c *gin.Context) {
	if !middleware.IsAdminOrOwner(c) && !middleware.IsOrganizationAdminOrOwner(c) {
		response.Fail(c, response.ErrPermissionDenied)
		return
	}

	accountID := middleware.GetAccountID(c)

	var req model.WorkspaceCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	err := h.workspaceService.CreateWorkspace(c.Request.Context(), req.Name, accountID)
	if err != nil {
		response.Fail(c, response.ErrSystemError)
		return
	}

	c.JSON(http.StatusCreated, gin.H{"message": "workspace created."})
}

// UpdateWorkspace handles PUT /workspaces/{workspace_id}/update - update workspace info
func (h *WorkspaceHandler) UpdateWorkspace(c *gin.Context) {
	workspaceID := c.Param("workspace_id")
	accountID := middleware.GetAccountID(c)
	if accountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	var req model.WorkspaceUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	var name string
	if req.Name != nil {
		name = *req.Name
	}

	organizationID, err := h.resolveWorkspaceOrganizationID(c.Request.Context(), workspaceID)
	if err != nil {
		if errors.Is(err, errWorkspaceOrganizationNotFound) {
			response.Fail(c, response.ErrWorkspaceNotFound)
			return
		}
		response.Fail(c, response.ErrSystemError)
		return
	}
	hasPermission, err := h.organizationService.CheckWorkspacePermission(
		c.Request.Context(),
		organizationID,
		workspaceID,
		accountID,
		model.WorkspacePermissionWorkspaceManage,
	)
	if err != nil {
		response.Fail(c, response.ErrSystemError)
		return
	}
	isOrganizationAdmin, err := h.accountService.IsOrganizationAdminOrOwner(c.Request.Context(), organizationID, accountID)
	if err != nil {
		response.Fail(c, response.ErrSystemError)
		return
	}
	if !hasPermission && !isOrganizationAdmin {
		response.Fail(c, response.ErrPermissionDenied)
		return
	}

	isAdmin := hasPermission || isOrganizationAdmin
	result, err := h.workspaceService.UpdateWorkspace(c.Request.Context(), workspaceID, name, req.Status, accountID, isAdmin)
	if err != nil {
		if err.Error() == "workspace not found" {
			response.Fail(c, response.ErrNotFound)
			return
		}
		if err.Error() == "no permission to update workspace name" {
			response.Fail(c, response.ErrPermissionDenied)
			return
		}
		if err.Error() == "workspace name '"+name+"' already exists in the organization" {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if err.Error() == "invalid workspace status" {
			response.Fail(c, response.ErrInvalidParam)
			return
		}
		response.Fail(c, response.ErrSystemError)
		return
	}

	response.Success(c, result)
}

// GetWorkspaceStatistics handles GET /workspaces/{workspace_id}/statistics - get workspace statistics
func (h *WorkspaceHandler) GetWorkspaceStatistics(c *gin.Context) {
	workspaceID := c.Param("workspace_id")
	accountID := middleware.GetAccountID(c)
	if accountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	organizationID, err := h.resolveWorkspaceOrganizationID(c.Request.Context(), workspaceID)
	if err != nil {
		if errors.Is(err, errWorkspaceOrganizationNotFound) {
			response.Fail(c, response.ErrWorkspaceNotFound)
			return
		}
		response.Fail(c, response.ErrSystemError)
		return
	}
	hasPermission, err := h.organizationService.CheckWorkspacePermission(
		c.Request.Context(),
		organizationID,
		workspaceID,
		accountID,
		model.WorkspacePermissionWorkspaceView,
	)
	if err != nil {
		response.Fail(c, response.ErrSystemError)
		return
	}
	if !hasPermission {
		response.Fail(c, response.ErrPermissionDenied)
		return
	}

	statistics, err := h.workspaceService.GetWorkspaceStatistics(c.Request.Context(), workspaceID)
	if err != nil {
		if err.Error() == "workspace not found" {
			response.Fail(c, response.ErrNotFound)
			return
		}
		response.Fail(c, response.ErrSystemError)
		return
	}

	response.Success(c, statistics)
}

func (h *WorkspaceHandler) RegisterRoutes(router *gin.RouterGroup) {
	workspaces := router.Group("/workspaces", middleware.JWTWithOrganizationAndService(h.accountService))
	{
		tenantRoutes := workspaces.Group("/:workspace_id")
		{
			tenantRoutes.PUT("/update", h.UpdateWorkspace)
			tenantRoutes.GET("/statistics", h.GetWorkspaceStatistics)
		}
	}
}
