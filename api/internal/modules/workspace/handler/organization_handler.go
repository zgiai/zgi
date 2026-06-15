package handler

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/zgiai/zgi/api/config"
	"github.com/zgiai/zgi/api/internal/dto"
	shared_dto "github.com/zgiai/zgi/api/internal/dto"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	"github.com/zgiai/zgi/api/internal/modules/workspace/model"
	workspace_service "github.com/zgiai/zgi/api/internal/modules/workspace/service"
	helper "github.com/zgiai/zgi/api/internal/util"
	"github.com/zgiai/zgi/api/middleware"
	"github.com/zgiai/zgi/api/pkg/response"
	"gorm.io/gorm"
)

type OrganizationHandler struct {
	organizationService        interfaces.OrganizationService
	workspaceManagementService interfaces.WorkspaceManagementService
	accountService             interfaces.AccountService
	workspaceService           workspace_service.WorkspaceService
	departmentService          workspace_service.DepartmentService
	consoleWebURL              string
}

func NewOrganizationHandler(
	organizationService interfaces.OrganizationService,
	workspaceManagementService interfaces.WorkspaceManagementService,
	accountService interfaces.AccountService,
	workspaceService workspace_service.WorkspaceService,
	departmentService workspace_service.DepartmentService,
	consoleWebURL string,
) *OrganizationHandler {
	return &OrganizationHandler{
		organizationService:        organizationService,
		workspaceManagementService: workspaceManagementService,
		accountService:             accountService,
		workspaceService:           workspaceService,
		departmentService:          departmentService,
		consoleWebURL:              consoleWebURL,
	}
}

type DepartmentMemberWithRoleResponse struct {
	*workspace_service.DepartmentMemberDetail
	OrganizationRole model.OrganizationRole `json:"organization_role"`
	DepartmentName   string                 `json:"department_name"`
}

type TenantMemberWithDepartmentResponse struct {
	ID             string  `json:"id"`
	Name           string  `json:"name"`
	AccountName    string  `json:"account_name"`
	MemberName     *string `json:"member_name"`
	Avatar         string  `json:"avatar"`
	AvatarURL      string  `json:"avatar_url"`
	Email          string  `json:"email"`
	LastLoginAt    *int64  `json:"last_login_at"`
	LastActiveAt   *int64  `json:"last_active_at"`
	CreatedAt      int64   `json:"created_at"`
	Role           string  `json:"role"`
	RoleID         *string `json:"role_id,omitempty"`
	RoleName       string  `json:"role_name"`
	Status         string  `json:"status"`
	HasMobile      bool    `json:"has_mobile"`
	DepartmentID   *string `json:"department_id,omitempty"`
	DepartmentName *string `json:"department_name,omitempty"`
}

func getWorkspaceRoleDisplayName(role string) string {
	switch role {
	case string(model.WorkspaceRoleOwner):
		return "Owner"
	case string(model.WorkspaceRoleAdmin):
		return "Admin"
	case string(model.WorkspaceRoleEditor),
		string(model.WorkspaceRoleNormal),
		string(model.WorkspaceRoleMember):
		return "Member"
	case string(model.WorkspaceRoleViewer):
		return "Viewer"
	default:
		return role
	}
}

// getOrganizationID gets organization_id from URL param or context (for current routes)
func (h *OrganizationHandler) getOrganizationID(c *gin.Context) string {
	organizationID := c.Param("organization_id")
	if organizationID == "" || organizationID == "current" {
		organizationID = helper.GetOrganizationID(c)
	}
	return organizationID
}

func handleOrganizationWorkspaceDetailError(c *gin.Context, err error) bool {
	if err == nil {
		return false
	}

	errMsg := err.Error()
	switch {
	case strings.Contains(errMsg, "organization not found"):
		response.Fail(c, response.ErrOrganizationNotFound)
	case strings.Contains(errMsg, "user not found"):
		response.Fail(c, response.ErrAccountNotFound)
	case strings.Contains(errMsg, "workspace not found"):
		response.Fail(c, response.ErrWorkspaceNotFound)
	case strings.Contains(errMsg, "workspace not in organization"):
		response.Fail(c, response.ErrWorkspaceNotInOrganization)
	case strings.Contains(errMsg, "permission denied"):
		response.Fail(c, response.ErrPermissionDenied)
	default:
		response.Fail(c, response.ErrSystemError)
	}

	return true
}

// ListWorkspacePermissions lists permission definitions for a organization
func (h *OrganizationHandler) ListWorkspacePermissions(c *gin.Context) {
	organizationID := h.getOrganizationID(c)
	if organizationID == "" {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	accountID := c.GetString("account_id")
	if accountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	result, err := h.organizationService.ListWorkspacePermissionDefinitions(c.Request.Context(), organizationID, accountID)
	if err != nil {
		response.Fail(c, response.ErrSystemError)
		return
	}

	response.Success(c, result)
}

// ListWorkspaceRoles lists roles (builtin + custom) for a organization
func (h *OrganizationHandler) ListWorkspaceRoles(c *gin.Context) {
	organizationID := h.getOrganizationID(c)
	if organizationID == "" {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	accountID := c.GetString("account_id")
	if accountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	includeOwner := false
	if v := c.Query("include_owner"); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			includeOwner = b
		}
	}

	result, err := h.organizationService.ListWorkspaceRoles(c.Request.Context(), organizationID, accountID, includeOwner)
	if err != nil {
		response.Fail(c, response.ErrSystemError)
		return
	}

	response.Success(c, result)
}

func (h *OrganizationHandler) GetWorkspaceRole(c *gin.Context) {
	organizationID := h.getOrganizationID(c)
	if organizationID == "" {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	roleID := c.Param("role_id")
	if roleID == "" {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	accountID := c.GetString("account_id")
	if accountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	result, err := h.organizationService.GetWorkspaceRoleDetail(c.Request.Context(), organizationID, roleID, accountID)
	if err != nil {
		response.Fail(c, response.ErrSystemError)
		return
	}

	response.Success(c, result)
}

func (h *OrganizationHandler) ListWorkspaceRoleMembers(c *gin.Context) {
	organizationID := h.getOrganizationID(c)
	if organizationID == "" {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	roleID := c.Param("role_id")
	if roleID == "" {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	accountID := c.GetString("account_id")
	if accountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	keyword := c.Query("keyword")

	page := 1
	if pageStr := c.Query("page"); pageStr != "" {
		if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
			page = p
		}
	}

	limit := 20
	if limitStr := c.Query("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 100 {
			limit = l
		}
	}

	result, err := h.organizationService.ListWorkspaceRoleMembers(c.Request.Context(), organizationID, roleID, accountID, keyword, page, limit)
	if err != nil {
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}

	response.Success(c, result)
}

// CreateWorkspaceRole creates a custom role for a organization
func (h *OrganizationHandler) CreateWorkspaceRole(c *gin.Context) {
	organizationID := h.getOrganizationID(c)
	if organizationID == "" {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	accountID := c.GetString("account_id")
	if accountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	var req dto.CreateWorkspaceRoleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, err.Error())
		return
	}
	req.OrganizationID = organizationID
	req.CreatedBy = accountID

	result, err := h.organizationService.CreateCustomWorkspaceRole(c.Request.Context(), &req)
	if err != nil {
		if errors.Is(err, workspace_service.ErrRoleNameExists) {
			response.Fail(c, response.ErrorCode{
				Code:        response.ErrInvalidParam.Code,
				Message:     "Role name already exists",
				UserVisible: true,
			})
			return
		}
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}

	response.Success(c, result)
}

// UpdateWorkspaceRole updates role basic info
func (h *OrganizationHandler) UpdateWorkspaceRole(c *gin.Context) {
	organizationID := h.getOrganizationID(c)
	if organizationID == "" {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	roleID := c.Param("role_id")
	if roleID == "" {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	accountID := c.GetString("account_id")
	if accountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	var req dto.UpdateWorkspaceRoleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	req.OrganizationID = organizationID
	req.RoleID = roleID

	result, err := h.organizationService.UpdateCustomWorkspaceRole(c.Request.Context(), &req)
	if err != nil {
		response.Fail(c, response.ErrSystemError)
		return
	}

	response.Success(c, result)
}

// UpdateWorkspaceRolePermissions updates role permissions (idempotent)
func (h *OrganizationHandler) UpdateWorkspaceRolePermissions(c *gin.Context) {
	organizationID := h.getOrganizationID(c)
	if organizationID == "" {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	roleID := c.Param("role_id")
	if roleID == "" {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	accountID := c.GetString("account_id")
	if accountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	var body struct {
		Permissions []string `json:"permissions"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	req := dto.UpdateWorkspaceRolePermissionsRequest{
		OrganizationID: organizationID,
		RoleID:         roleID,
		Permissions:    body.Permissions,
		OperatorID:     accountID,
	}

	if err := h.organizationService.UpdateWorkspaceRolePermissions(c.Request.Context(), &req); err != nil {
		if errors.Is(err, workspace_service.ErrCannotUpdateBuiltinRole) {
			response.Fail(c, response.ErrorCode{
				Code:        response.ErrInvalidParam.Code,
				Message:     "Cannot update built-in role",
				UserVisible: true,
			})
			return
		}
		response.Fail(c, response.ErrSystemError)
		return
	}

	response.Success(c, gin.H{"message": "success"})
}

// DeleteWorkspaceRole deletes a custom role (soft delete)
func (h *OrganizationHandler) DeleteWorkspaceRole(c *gin.Context) {
	organizationID := h.getOrganizationID(c)
	if organizationID == "" {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	roleID := c.Param("role_id")
	if roleID == "" {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	accountID := c.GetString("account_id")
	if accountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	if err := h.organizationService.DeleteCustomWorkspaceRole(c.Request.Context(), organizationID, roleID, accountID); err != nil {
		response.Fail(c, response.ErrSystemError)
		return
	}

	response.Success(c, gin.H{"message": "success"})
}

// GetMemberPermissions returns effective permissions for a member
func (h *OrganizationHandler) GetMemberPermissions(c *gin.Context) {
	organizationID := h.getOrganizationID(c)
	if organizationID == "" {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	targetAccountID := c.Param("account_id")
	if targetAccountID == "" {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	accountID := c.GetString("account_id")
	if accountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	result, err := h.organizationService.GetMemberEffectivePermissions(c.Request.Context(), organizationID, accountID, targetAccountID)
	if err != nil {
		response.Fail(c, response.ErrSystemError)
		return
	}

	response.Success(c, result)
}

func (h *OrganizationHandler) GetWorkspaceMemberPermissions(c *gin.Context) {
	organizationID := h.getOrganizationID(c)
	workspaceID := c.Param("workspace_id")
	if organizationID == "" || workspaceID == "" {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	accountID := c.GetString("account_id")
	if accountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	targetAccountID := h.resolveTargetAccountID(c, accountID)

	result, err := h.organizationService.GetWorkspaceMemberPermissions(c.Request.Context(), organizationID, workspaceID, accountID, targetAccountID)
	if err != nil {
		errMsg := err.Error()
		switch {
		case strings.Contains(errMsg, "organization not found"):
			response.Fail(c, response.ErrOrganizationNotFound)
		case strings.Contains(errMsg, "workspace not in organization"):
			response.Fail(c, response.ErrWorkspaceNotInOrganization)
		case strings.Contains(errMsg, "member not in tenant"), strings.Contains(errMsg, "tenant member not found"):
			response.Fail(c, response.ErrMemberNotInWorkspace)
		default:
			response.Fail(c, response.ErrSystemError)
		}
		return
	}

	response.Success(c, result)
}

func (h *OrganizationHandler) resolveTargetAccountID(c *gin.Context, accountID string) string {
	targetAccountID := c.Param("account_id")
	if targetAccountID == "" || targetAccountID == "current" {
		return accountID
	}
	return targetAccountID
}

// CreateOrganization
func (h *OrganizationHandler) CreateOrganization(c *gin.Context) {
	var req struct {
		Name      string  `json:"name" binding:"required"`
		ShortName *string `json:"short_name,omitempty"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	accountID := c.GetString("account_id")
	if accountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	// Check if organization name already exists
	exists, err := h.organizationService.CheckOrganizationNameExists(c.Request.Context(), req.Name)
	if err != nil {
		response.Fail(c, response.ErrSystemError)
		return
	}
	if exists {
		response.Fail(c, response.ErrOrganizationExists)
		return
	}

	// Create organization (includes complete business logic)
	_, err = h.organizationService.CreateOrganizationWithWorkspace(c.Request.Context(), &shared_dto.CreateOrganizationWithWorkspaceRequest{
		Name:      req.Name,
		ShortName: req.ShortName,
		CreatedBy: accountID,
	})
	if err != nil {
		response.Fail(c, response.ErrSystemError)
		return
	}

	response.Success(c, gin.H{"message": "success"})
}

func (h *OrganizationHandler) GetOrganizationDetails(c *gin.Context) {
	organizationID := h.getOrganizationID(c)
	if organizationID == "" {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	accountID := c.GetString("account_id")
	if accountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	// Get organization information
	organization, err := h.organizationService.GetOrganizationByID(c.Request.Context(), organizationID)
	if err != nil {
		response.Fail(c, response.ErrOrganizationNotFound)
		return
	}

	// Get user's role in organization
	role, err := h.organizationService.GetUserOrganizationRole(c.Request.Context(), organizationID, accountID)
	if err != nil {
		response.Fail(c, response.ErrSystemError)
		return
	}

	type OrganizationWithRole struct {
		ID               string                   `json:"id"`
		Name             string                   `json:"name"`
		ShortName        *string                  `json:"short_name"`
		Status           model.OrganizationStatus `json:"status"`
		CreatedAt        int64                    `json:"created_at"` // Timestamp format
		OrganizationRole model.OrganizationRole   `json:"organization_role"`
	}

	organizationWithRole := OrganizationWithRole{
		ID:               organization.ID,
		Name:             organization.Name,
		ShortName:        organization.ShortName,
		Status:           organization.Status,
		CreatedAt:        organization.CreatedAt.Unix(), // Convert to timestamp
		OrganizationRole: role,
	}

	response.Success(c, organizationWithRole)
}

func (h *OrganizationHandler) CreateOrganizationWorkspace(c *gin.Context) {
	organizationID := h.getOrganizationID(c)
	if organizationID == "" {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	var req struct {
		Name         string  `json:"name" binding:"required"`
		DepartmentID *string `json:"department_id,omitempty"`
		LeaderID     *string `json:"leader_id,omitempty"`
		APIKeyID     *string `json:"api_key_id,omitempty"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	accountID := c.GetString("account_id")
	if accountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	// 1. Check whether the organization exists
	organization, err := h.organizationService.GetOrganizationByID(c.Request.Context(), organizationID)
	if err != nil {
		response.Fail(c, response.ErrOrganizationNotFound)
		return
	}
	if organization == nil {
		response.Fail(c, response.ErrOrganizationNotFound)
		return
	}

	role, err := h.organizationService.GetUserOrganizationRole(c.Request.Context(), organizationID, accountID)
	if err != nil {
		response.Fail(c, response.ErrSystemError)
		return
	}
	if role != model.OrganizationRoleOwner && role != model.OrganizationRoleAdmin {
		response.Fail(c, response.ErrSuperAdminRequired)
		return
	}

	if req.LeaderID != nil && *req.LeaderID != "" {
		isMember, err := h.organizationService.IsOrganizationMember(c.Request.Context(), organizationID, *req.LeaderID)
		if err != nil {
			response.Fail(c, response.ErrSystemError)
			return
		}
		if !isMember {
			response.FailWithMessage(c, response.ErrNotFound, "leader is not a member of the organization")
			return
		}
	}

	// 3.Check if there is already a tenant with the same name in the organization.
	exists, err := h.organizationService.CheckWorkspaceNameExistsInOrganization(c.Request.Context(), organizationID, req.Name)
	if err != nil {
		response.Fail(c, response.ErrSystemError)
		return
	}
	if exists {
		response.Fail(c, response.ErrWorkspaceExists)
		return
	}

	// 4.Create workspace
	workspace, err := h.workspaceManagementService.CreateWorkspace(c.Request.Context(), req.Name, true)
	if err != nil {
		response.Fail(c, response.ErrSystemError)
		return
	}

	// 5.Add the tenant to the organization
	err = h.organizationService.AddWorkspace(c.Request.Context(), &shared_dto.AddWorkspaceToOrganizationRequest{
		OrganizationID: organizationID,
		WorkspaceID:    workspace.ID,
		DepartmentID:   req.DepartmentID,
		APIKeyID:       req.APIKeyID,
	})
	if err != nil {
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}

	var warning string
	if req.LeaderID != nil && *req.LeaderID != "" {
		if err := h.workspaceManagementService.CreateWorkspaceMember(c.Request.Context(), workspace.ID, *req.LeaderID, string(model.WorkspaceRoleOwner)); err != nil {
			warning = err.Error()
		}
	}

	if warning != "" {
		response.Success(c, gin.H{
			"workspace": workspace,
			"warning":   warning,
		})
		return
	}

	response.Success(c, gin.H{
		"workspace": workspace,
	})
}

func (h *OrganizationHandler) UpdateOrganizationWorkspace(c *gin.Context) {
	organizationID := h.getOrganizationID(c)
	workspaceID := c.Param("workspace_id")
	if organizationID == "" || workspaceID == "" {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	var req struct {
		model.WorkspaceUpdateRequest
		DepartmentID *string `json:"department_id,omitempty"`
		LeaderID     *string `json:"leader_id,omitempty"`
		APIKeyID     *string `json:"api_key_id,omitempty"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	accountID := c.GetString("account_id")
	if accountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	organization, err := h.organizationService.GetOrganizationByID(c.Request.Context(), organizationID)
	if err != nil || organization == nil {
		response.Fail(c, response.ErrOrganizationNotFound)
		return
	}

	workspaceOrganization, err := h.organizationService.GetOrganizationByWorkspaceID(c.Request.Context(), workspaceID)
	if err != nil {
		response.Fail(c, response.ErrSystemError)
		return
	}
	if workspaceOrganization == nil || workspaceOrganization.ID != organizationID {
		response.Fail(c, response.ErrWorkspaceNotInOrganization)
		return
	}

	hasAdminPermission := false

	role, err := h.organizationService.GetUserOrganizationRole(c.Request.Context(), organizationID, accountID)
	if err != nil {
		response.Fail(c, response.ErrSystemError)
		return
	}
	if role == model.OrganizationRoleOwner || role == model.OrganizationRoleAdmin {
		hasAdminPermission = true
	}

	var name string
	if req.Name != nil {
		name = *req.Name
	}

	ctx := c.Request.Context()

	result, err := h.workspaceService.UpdateWorkspace(ctx, workspaceID, name, req.Status, accountID, hasAdminPermission)
	if err != nil {
		if err.Error() == "workspace not found" {
			response.Fail(c, response.ErrNotFound)
			return
		}
		if err.Error() == "no permission to update workspace name" {
			response.Fail(c, response.ErrPermissionDenied)
			return
		}
		if strings.HasPrefix(err.Error(), "workspace name '") && strings.HasSuffix(err.Error(), "' already exists in the organization") {
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

	if (req.DepartmentID != nil || req.APIKeyID != nil) && h.organizationService != nil {
		if err := h.organizationService.UpdateWorkspaceJoinMeta(ctx, organizationID, workspaceID, req.DepartmentID, req.APIKeyID); err != nil {
			response.FailWithMessage(c, response.ErrSystemError, err.Error())
			return
		}
	}

	if req.LeaderID != nil && *req.LeaderID != "" {
		leaderID := *req.LeaderID

		isMember, err := h.organizationService.IsOrganizationMember(ctx, organizationID, leaderID)
		if err != nil {
			response.Fail(c, response.ErrSystemError)
			return
		}
		if !isMember {
			response.FailWithMessage(c, response.ErrNotFound, "leader is not a member of the organization")
			return
		}

		targetWorkspace, err := h.workspaceManagementService.GetWorkspaceByID(ctx, workspaceID)
		if err != nil || targetWorkspace == nil {
			response.Fail(c, response.ErrWorkspaceNotFound)
			return
		}

		newLeader, err := h.accountService.GetAccountByID(ctx, leaderID)
		if err != nil || newLeader == nil {
			response.Fail(c, response.ErrAccountNotFound)
			return
		}

		operator, err := h.accountService.GetAccountByID(ctx, accountID)
		if err != nil || operator == nil {
			response.Fail(c, response.ErrAccountNotFound)
			return
		}

		existingJoinRole, err := h.workspaceManagementService.GetUserRole(ctx, leaderID, workspaceID)
		if err != nil {
			response.Fail(c, response.ErrSystemError)
			return
		}
		if existingJoinRole == nil {
			if err := h.workspaceManagementService.CreateWorkspaceMember(ctx, workspaceID, leaderID, string(model.WorkspaceRoleNormal)); err != nil {
				response.FailWithMessage(c, response.ErrSystemError, err.Error())
				return
			}
		}

		if existingJoinRole == nil || *existingJoinRole != model.WorkspaceRoleOwner {
			err = h.workspaceManagementService.UpdateMemberRoleWithPermissionCheck(ctx, targetWorkspace, newLeader, string(model.WorkspaceRoleOwner), operator)
			if err != nil {
				switch {
				case isCannotOperateSelfError(err):
					response.Fail(c, response.ErrCannotOperateSelf)
					return
				case isNoPermissionError(err):
					response.Fail(c, response.ErrPermissionDenied)
					return
				case isMemberNotInWorkspaceError(err):
					response.Fail(c, response.ErrMemberNotInWorkspace)
					return
				case isRoleAlreadyAssignedError(err):
					response.Fail(c, response.ErrRoleAlreadyAssigned)
					return
				default:
					response.FailWithMessage(c, response.ErrSystemError, err.Error())
					return
				}
			}
		}
	}

	response.Success(c, result)
}

// TransferOrganizationOwnership transfers the ownership of the organization
func (h *OrganizationHandler) TransferOrganizationOwnership(c *gin.Context) {
	organizationID := h.getOrganizationID(c)
	if organizationID == "" {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	var req struct {
		MemberID string `json:"member_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	accountID := c.GetString("account_id")
	if accountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	// 1. Check if organization exists
	organization, err := h.organizationService.GetOrganizationByID(c.Request.Context(), organizationID)
	if err != nil {
		response.Fail(c, response.ErrOrganizationNotFound)
		return
	}
	if organization == nil {
		response.Fail(c, response.ErrOrganizationNotFound)
		return
	}

	// 2. Check permission (must be owner)
	role, err := h.organizationService.GetUserOrganizationRole(c.Request.Context(), organizationID, accountID)
	if err != nil {
		response.Fail(c, response.ErrSystemError)
		return
	}
	if role != model.OrganizationRoleOwner {
		response.Fail(c, response.ErrPermissionDenied)
		return
	}

	// 3. Transfer ownership
	if err := h.organizationService.TransferOwnership(c.Request.Context(), organizationID, accountID, req.MemberID); err != nil {
		// Handle specific errors if needed
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}

	response.Success(c, nil)
}

// DeleteOrganizationWorkspace deletes a tenant from an organization
func (h *OrganizationHandler) DeleteOrganizationWorkspace(c *gin.Context) {
	organizationID := h.getOrganizationID(c)
	workspaceID := c.Param("workspace_id")
	if organizationID == "" || workspaceID == "" {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	accountID := c.GetString("account_id")
	if accountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	// 1. Check whether the organization exists
	organization, err := h.organizationService.GetOrganizationByID(c.Request.Context(), organizationID)
	if err != nil {
		response.Fail(c, response.ErrOrganizationNotFound)
		return
	}
	if organization == nil {
		response.Fail(c, response.ErrOrganizationNotFound)
		return
	}

	role, err := h.organizationService.GetUserOrganizationRole(c.Request.Context(), organizationID, accountID)
	if err != nil {
		response.Fail(c, response.ErrSystemError)
		return
	}
	if role != model.OrganizationRoleOwner && role != model.OrganizationRoleAdmin {
		response.Fail(c, response.ErrSuperAdminRequired)
		return
	}

	// 3. Check if tenant exists
	tenant, err := h.workspaceManagementService.GetWorkspaceByID(c.Request.Context(), workspaceID)
	if err != nil {
		response.Fail(c, response.ErrWorkspaceNotFound)
		return
	}
	if tenant == nil {
		response.Fail(c, response.ErrWorkspaceNotFound)
		return
	}

	// 4. Check if tenant belongs to the organization
	workspaceInOrganization, err := h.organizationService.GetOrganizationByWorkspaceID(c.Request.Context(), workspaceID)
	if err != nil {
		response.Fail(c, response.ErrSystemError)
		return
	}
	if workspaceInOrganization == nil || workspaceInOrganization.ID != organizationID {
		response.Fail(c, response.ErrWorkspaceNotInOrganization)
		return
	}

	hasAssets, _, err := h.organizationService.CheckWorkspaceAssets(c.Request.Context(), workspaceID)
	if err != nil {
		response.Fail(c, response.ErrSystemError)
		return
	}
	if hasAssets {
		response.Fail(c, response.ErrCannotDeleteTenantWithAssets)
		return
	}

	// 6. Delete workspace and all its members
	// Note: We don't need to explicitly remove it from organization first because deleting the workspace
	// will automatically remove the association. Plus, DeleteWorkspaceWithMembers handles quota release.
	if err := h.workspaceManagementService.DeleteWorkspaceWithMembers(c.Request.Context(), workspaceID); err != nil {
		response.Fail(c, response.ErrSystemError)
		return
	}

	response.Success(c, gin.H{"result": "success"})
}

func (h *OrganizationHandler) GetOrganizationWorkspaceAssets(c *gin.Context) {
	organizationID := h.getOrganizationID(c)
	workspaceID := c.Param("workspace_id")
	if organizationID == "" || workspaceID == "" {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	accountID := c.GetString("account_id")
	if accountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	if h.organizationService != nil {
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
		if !hasPermission {
			response.Fail(c, response.ErrPermissionDenied)
			return
		}
	}

	workspaceInOrganization, err := h.organizationService.GetOrganizationByWorkspaceID(c.Request.Context(), workspaceID)
	if err != nil {
		response.Fail(c, response.ErrSystemError)
		return
	}
	if workspaceInOrganization == nil || workspaceInOrganization.ID != organizationID {
		response.Fail(c, response.ErrWorkspaceNotInOrganization)
		return
	}

	hasAssets, assetCounts, err := h.organizationService.CheckWorkspaceAssets(c.Request.Context(), workspaceID)
	if err != nil {
		response.Fail(c, response.ErrSystemError)
		return
	}

	response.Success(c, gin.H{
		"has_assets": hasAssets,
		"assets":     assetCounts,
	})
}

func (h *OrganizationHandler) GetUnjoinedWorkspaces(c *gin.Context) {
	organizationID := h.getOrganizationID(c)
	accountID := c.Param("account_id")
	if organizationID == "" || accountID == "" {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	// Parse pagination parameters
	page := 1
	limit := 20
	if pageStr := c.Query("page"); pageStr != "" {
		if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
			page = p
		}
	}
	if limitStr := c.Query("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}
	}

	currentAccountID := c.GetString("account_id")
	if currentAccountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	organization, err := h.organizationService.GetOrganizationByID(c.Request.Context(), organizationID)
	if err != nil {
		response.Fail(c, response.ErrOrganizationNotFound)
		return
	}
	if organization == nil {
		response.Fail(c, response.ErrOrganizationNotFound)
		return
	}

	account, err := h.accountService.GetAccountByID(c.Request.Context(), accountID)
	if err != nil {
		response.Fail(c, response.ErrAccountNotFound)
		return
	}
	if account == nil {
		response.Fail(c, response.ErrAccountNotFound)
		return
	}

	pagination, err := h.organizationService.GetUnjoinedWorkspacesForUser(c.Request.Context(), organizationID, accountID, page, limit)
	if err != nil {
		response.Fail(c, response.ErrSystemError)
		return
	}

	response.Success(c, gin.H{
		"page":     pagination.Page,
		"limit":    pagination.Limit,
		"total":    pagination.Total,
		"has_more": pagination.HasMore,
		"data":     pagination.Data,
	})
}

func (h *OrganizationHandler) GetJoinedWorkspaces(c *gin.Context) {
	organizationID := h.getOrganizationID(c)
	accountID := c.Param("account_id")
	if organizationID == "" || accountID == "" {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	page := 1
	limit := 20
	if pageStr := c.Query("page"); pageStr != "" {
		if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
			page = p
		}
	}
	if limitStr := c.Query("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}
	}

	currentAccountID := c.GetString("account_id")
	if currentAccountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	pagination, err := h.organizationService.GetUserWorkspacesInOrganization(c.Request.Context(), organizationID, accountID, page, limit)
	if err != nil {
		response.Fail(c, response.ErrSystemError)
		return
	}

	response.Success(c, gin.H{
		"page":     pagination.Page,
		"limit":    pagination.Limit,
		"total":    pagination.Total,
		"has_more": pagination.HasMore,
		"data":     pagination.Data,
	})
}

func (h *OrganizationHandler) GetJoinedWorkspacesRoles(c *gin.Context) {
	organizationID := h.getOrganizationID(c)
	accountID := c.Param("account_id")
	if organizationID == "" || accountID == "" {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	currentAccountID := c.GetString("account_id")
	if currentAccountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	workspaceRoles, err := h.organizationService.GetUserWorkspacesRolesInOrganization(c.Request.Context(), organizationID, accountID)
	if err != nil {
		response.Fail(c, response.ErrSystemError)
		return
	}

	response.Success(c, workspaceRoles)
}

func (h *OrganizationHandler) GetJoinedOrganizations(c *gin.Context) {

	page := 1
	limit := 20
	if pageStr := c.Query("page"); pageStr != "" {
		if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
			page = p
		}
	}
	if limitStr := c.Query("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}
	}

	accountID := c.Param("account_id")
	if accountID == "" {
		// If no account_id provided, use current user ID
		accountID = c.GetString("account_id")
	}
	if accountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	currentAccountID := c.GetString("account_id")
	if currentAccountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	// Check if has permission to view specified account's joined organizations
	if currentAccountID != accountID {
		response.Fail(c, response.ErrPermissionDenied)
		return
	}

	pagination, err := h.organizationService.GetUserOrganizationsByAccount(c.Request.Context(), accountID, page, limit)
	if err != nil {
		response.Fail(c, response.ErrSystemError)
		return
	}

	response.Success(c, gin.H{
		"page":     pagination.Page,
		"limit":    pagination.Limit,
		"total":    pagination.Total,
		"has_more": pagination.HasMore,
		"data":     pagination.Data,
	})
}

func (h *OrganizationHandler) CheckManagePermission(c *gin.Context) {
	organizationID := h.getOrganizationID(c)
	if organizationID == "" {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	accountID := c.GetString("account_id")
	if accountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	switchTenantStr := c.DefaultQuery("switch_tenant", "true")
	switchStr := c.DefaultQuery("switch", "true")
	if switchStr != "" {
		switchTenantStr = switchStr
	}
	switchTenant := true
	if switchTenantStr == "false" || switchTenantStr == "0" {
		switchTenant = false
	}

	hasPermission, err := h.organizationService.CheckAnyManagedWorkspacePermission(c.Request.Context(), organizationID, accountID)
	if err != nil {
		if strings.Contains(err.Error(), "organization not found") {
			response.Fail(c, response.ErrSystemError)
			return
		}
		response.Fail(c, response.ErrSystemError)
		return
	}

	if switchTenant {
		// Update account context to current organization
		_, err := h.accountService.UpdateAccountContext(c.Request.Context(), accountID, &organizationID, nil)
		if err != nil {
			response.Fail(c, response.ErrSystemError)
			return
		}
	}

	// Return results
	response.Success(c, gin.H{
		"has_permission": hasPermission,
	})
}

func (h *OrganizationHandler) GetCurrentOrganization(c *gin.Context) {
	// Permission validation
	accountID := c.GetString("account_id")
	if accountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	// Call service to get current joined organization
	currentOrganization, err := h.organizationService.GetCurrentOrganization(c.Request.Context(), accountID)
	if err != nil {
		if strings.Contains(err.Error(), "no current tenant found") {
			response.Fail(c, response.ErrNotFound)
			return
		}
		response.Fail(c, response.ErrSystemError)
		return
	}

	// Return joined organization info
	response.Success(c, currentOrganization)
}

func (h *OrganizationHandler) GetCurrentOrganizationDetail(c *gin.Context) {
	// Permission validation
	accountID := c.GetString("account_id")
	if accountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	// Call service to get current organization details.
	currentOrganizationDetail, err := h.organizationService.GetCurrentOrganizationDetail(c.Request.Context(), accountID)
	if err != nil {
		if strings.Contains(err.Error(), "no current tenant found") {
			response.Fail(c, response.ErrNotFound)
			return
		}
		response.Fail(c, response.ErrSystemError)
		return
	}

	// Return results
	response.Success(c, currentOrganizationDetail)
}

func (h *OrganizationHandler) GetManagedWorkspaces(c *gin.Context) {
	organizationID := h.getOrganizationID(c)
	if organizationID == "" {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	// Permission validation
	accountID := c.GetString("account_id")
	if accountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	page := 1
	limit := 20
	if pageStr := c.Query("page"); pageStr != "" {
		if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
			page = p
		}
	}
	if limitStr := c.Query("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}
	}

	// Call service method to get managed tenant list
	pagination, err := h.organizationService.GetManagedWorkspacesInOrganization(c.Request.Context(), organizationID, accountID, page, limit)
	if err != nil {
		response.Fail(c, response.ErrSystemError)
		return
	}

	// Return paginated results
	response.Success(c, gin.H{
		"page":     pagination.Page,
		"limit":    pagination.Limit,
		"total":    pagination.Total,
		"has_more": pagination.HasMore,
		"data":     pagination.Data,
	})
}

func (h *OrganizationHandler) GetManagedAppWorkspaces(c *gin.Context) {
	organizationID := h.getOrganizationID(c)
	if organizationID == "" {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	// Permission validation
	accountID := c.GetString("account_id")
	if accountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	page := 1
	limit := 20
	if pageStr := c.Query("page"); pageStr != "" {
		if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
			page = p
		}
	}
	if limitStr := c.Query("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}
	}

	// Call service method to get managed application tenant list
	pagination, err := h.organizationService.GetManagedAppWorkspacesInOrganization(c.Request.Context(), organizationID, accountID, page, limit)
	if err != nil {
		response.Fail(c, response.ErrSystemError)
		return
	}

	// Return paginated results
	response.Success(c, gin.H{
		"page":     pagination.Page,
		"limit":    pagination.Limit,
		"total":    pagination.Total,
		"has_more": pagination.HasMore,
		"data":     pagination.Data,
	})
}

func (h *OrganizationHandler) GetManagedDatasetWorkspaces(c *gin.Context) {
	organizationID := h.getOrganizationID(c)
	if organizationID == "" {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	// Permission validation
	accountID := c.GetString("account_id")
	if accountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	page := 1
	limit := 20
	if pageStr := c.Query("page"); pageStr != "" {
		if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
			page = p
		}
	}
	if limitStr := c.Query("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}
	}

	pagination, err := h.organizationService.GetManagedDatasetWorkspacesInOrganization(c.Request.Context(), organizationID, accountID, page, limit)
	if err != nil {
		response.Fail(c, response.ErrSystemError)
		return
	}

	// Return paginated results
	response.Success(c, gin.H{
		"page":     pagination.Page,
		"limit":    pagination.Limit,
		"total":    pagination.Total,
		"has_more": pagination.HasMore,
		"data":     pagination.Data,
	})
}

func (h *OrganizationHandler) GetOrganizationMembers(c *gin.Context) {
	organizationID := h.getOrganizationID(c)
	if organizationID == "" {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	accountID := c.GetString("account_id")
	if accountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	// Check if organization exists
	_, err := h.organizationService.GetByID(c.Request.Context(), organizationID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			response.Fail(c, response.ErrOrganizationNotFound)
			return
		}
		response.Fail(c, response.ErrSystemError)
		return
	}

	// If not system admin, check if has management permissions
	hasPermission, err := h.organizationService.CheckAnyManagedWorkspacePermission(c.Request.Context(), organizationID, accountID)
	if err != nil {
		response.Fail(c, response.ErrSystemError)
		return
	}
	if !hasPermission {
		response.Fail(c, response.ErrPermissionDenied)
		return
	}

	page := 1
	limit := 20
	if pageStr := c.Query("page"); pageStr != "" {
		if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
			page = p
		}
	}
	if limitStr := c.Query("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}
	}

	// Call service to get paginated data
	keyword := c.Query("keyword")
	pagination, err := h.organizationService.GetOrganizationMembersPaginated(c.Request.Context(), organizationID, page, limit, keyword)
	if err != nil {
		response.Fail(c, response.ErrSystemError)
		return
	}

	if pagination != nil && len(pagination.Data) > 0 {
		for _, member := range pagination.Data {
			dept, err := h.departmentService.GetMemberDepartment(c.Request.Context(), organizationID, member.ID)
			if err != nil {
				if err == workspace_service.ErrMemberNotInDept {
					continue
				}
				response.Fail(c, response.ErrSystemError)
				return
			}
			if dept != nil {
				id := dept.ID
				name := dept.Name
				member.DepartmentID = &id
				member.DepartmentName = &name
			}
		}
	}

	response.Success(c, pagination)
}

func (h *OrganizationHandler) DirectAddMember(c *gin.Context) {
	organizationID := h.getOrganizationID(c)
	if organizationID == "" {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	if !middleware.IsOrganizationAdminOrOwner(c) {
		response.Fail(c, response.ErrPermissionDenied)
		return
	}

	var req struct {
		Name         string  `json:"name" binding:"required"`
		Email        string  `json:"email" binding:"required,email"`
		WorkspaceID  string  `json:"workspace_id" binding:"required"`
		DepartmentID *string `json:"department_id,omitempty"`
		SendEmail    *bool   `json:"send_email,omitempty"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, err.Error())
		return
	}

	ctx := c.Request.Context()
	workspaceID := strings.TrimSpace(req.WorkspaceID)
	if workspaceID == "" {
		response.Fail(c, response.ErrInvalidParam)
		return
	}
	workspace, err := h.workspaceManagementService.GetWorkspaceByID(ctx, workspaceID)
	if err != nil || workspace == nil {
		response.Fail(c, response.ErrWorkspaceNotFound)
		return
	}
	if workspace.OrganizationID == nil || *workspace.OrganizationID != organizationID {
		response.Fail(c, response.ErrWorkspaceNotInOrganization)
		return
	}
	if workspace.Status != model.WorkspaceStatusNormal {
		response.Fail(c, response.ErrWorkspaceNotFound)
		return
	}

	var deptID string
	var respDeptID string
	var respDeptName string
	if req.DepartmentID != nil {
		deptID = strings.TrimSpace(*req.DepartmentID)
	}

	if deptID != "" {
		dept, err := h.departmentService.GetDepartment(ctx, deptID)
		if err != nil {
			if errors.Is(err, workspace_service.ErrDepartmentNotFound) {
				c.JSON(http.StatusNotFound, gin.H{"code": "DepartmentNotFound", "message": err.Error()})
				return
			}
			response.Fail(c, response.ErrSystemError)
			return
		}

		if dept.OrganizationID != organizationID {
			response.Fail(c, response.ErrInvalidParam)
			return
		}

		respDeptID = dept.ID
		respDeptName = dept.Name
	}

	// Check for duplicate name in organization
	exists, err := h.organizationService.ExistsMemberByName(ctx, organizationID, req.Name, "")
	if err != nil {
		response.Fail(c, response.ErrSystemError)
		return
	}
	if exists {
		response.FailWithMessage(c, response.ErrInvalidParam, "member name already exists")
		return
	}

	account, err := h.accountService.GetUserThroughEmail(ctx, req.Email)
	if err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			response.Fail(c, response.ErrSystemError)
			return
		}
		account = nil
	}

	if account == nil {
		password := helper.GenerateString(16)
		createReq := &dto.CreateAccountRequest{
			Name:     req.Name,
			Email:    req.Email,
			Password: password,
			Language: "",
			Timezone: "",
			IsSetup:  false,
		}

		account, err = h.accountService.CreateAccount(ctx, createReq)
		if err != nil {
			response.Fail(c, response.ErrSystemError)
			return
		}
	}

	isMember, err := h.organizationService.IsOrganizationMember(ctx, organizationID, account.ID)
	if err != nil {
		response.Fail(c, response.ErrSystemError)
		return
	}
	if !isMember {
		addReq := &shared_dto.AddOrganizationMemberRequest{
			OrganizationID: organizationID,
			AccountID:      account.ID,
			Role:           model.OrganizationRoleNormal,
			Name:           &req.Name,
		}
		if err := h.organizationService.AddMember(ctx, addReq); err != nil && !strings.Contains(err.Error(), "already exists in organization") {
			response.Fail(c, response.ErrSystemError)
			return
		}
	}

	if deptID != "" {
		_, err = h.departmentService.AddMemberToDepartment(ctx, organizationID, deptID, account.ID)
		if err != nil {
			if errors.Is(err, workspace_service.ErrDepartmentNotFound) {
				c.JSON(http.StatusNotFound, gin.H{"code": "DepartmentNotFound", "message": err.Error()})
				return
			}
			if errors.Is(err, workspace_service.ErrMemberAlreadyInDept) {
				currentDept, deptErr := h.departmentService.GetMemberDepartment(ctx, organizationID, account.ID)
				if deptErr != nil && !errors.Is(deptErr, workspace_service.ErrMemberNotInDept) {
					response.Fail(c, response.ErrSystemError)
					return
				}

				resp := gin.H{
					"code":    "MemberAlreadyInDepartment",
					"message": err.Error(),
				}
				if currentDept != nil {
					resp["current_department"] = gin.H{
						"id":   currentDept.ID,
						"name": currentDept.Name,
					}
				}

				c.JSON(http.StatusBadRequest, resp)
				return
			}
			response.Fail(c, response.ErrSystemError)
			return
		}
	}

	existingWorkspaceRole, err := h.workspaceManagementService.GetUserRole(ctx, account.ID, workspaceID)
	if err != nil {
		response.Fail(c, response.ErrSystemError)
		return
	}
	if existingWorkspaceRole == nil {
		if err := h.workspaceManagementService.AddMember(ctx, &interfaces.AddMemberRequest{
			WorkspaceID: workspaceID,
			AccountID:   account.ID,
			Role:        model.WorkspaceRoleNormal,
		}); err != nil && !strings.Contains(err.Error(), "already a member") {
			response.Fail(c, response.ErrSystemError)
			return
		}
	}

	if _, _, err := h.accountService.EnsureAccountContextForWorkspace(ctx, account.ID, organizationID, workspaceID); err != nil {
		response.Fail(c, response.ErrSystemError)
		return
	}

	sendEmail := false
	if req.SendEmail != nil {
		sendEmail = *req.SendEmail
	}

	if sendEmail {
		ip := c.ClientIP()
		limited, err := h.accountService.IsEmailSendIPLimit(ctx, ip)
		if err != nil || limited {
			response.Fail(c, response.ErrRateLimitExceeded)
			return
		}

		language := "zh-Hans"
		if account.InterfaceLanguage != nil && *account.InterfaceLanguage != "" {
			language = *account.InterfaceLanguage
		}

		organization, err := h.organizationService.GetByID(ctx, organizationID)
		if err != nil || organization == nil {
			response.Fail(c, response.ErrSystemError)
			return
		}

		if err := h.accountService.SendDirectAddMemberEmail(ctx, account, organizationID, organization.Name, respDeptName, language); err != nil {
			response.Fail(c, response.ErrEmailSendFailed)
			return
		}
	}

	response.Success(c, gin.H{
		"account_id": account.ID,
		"name":       account.Name,
		"email":      account.Email,
		"department": gin.H{
			"id":   respDeptID,
			"name": respDeptName,
		},
		"workspace": gin.H{
			"id":   workspace.ID,
			"name": workspace.Name,
		},
	})
}

func (h *OrganizationHandler) InviteCurrentOrganizationMember(c *gin.Context) {
	if !isSelfHostedEdition() {
		response.Fail(c, response.ErrSelfHostedOnly)
		return
	}

	accountID := middleware.GetAccountID(c)
	organizationID, ok := h.resolveCurrentOrganizationForMemberAdmin(c, accountID)
	if !ok {
		return
	}

	var req struct {
		Email        string  `json:"email" binding:"required,email"`
		Name         string  `json:"name" binding:"required"`
		Password     string  `json:"password"`
		WorkspaceID  string  `json:"workspace_id" binding:"required"`
		DepartmentID *string `json:"department_id,omitempty"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, err.Error())
		return
	}
	workspaceID := strings.TrimSpace(req.WorkspaceID)
	if workspaceID == "" {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	password, ok := resolveOrganizationInvitePassword(c, req.Password)
	if !ok {
		return
	}

	result, err := h.organizationService.InviteCurrentOrganizationMember(c.Request.Context(), &shared_dto.InviteCurrentOrganizationMemberRequest{
		OrganizationID:    organizationID,
		OperatorAccountID: accountID,
		WorkspaceID:       workspaceID,
		Email:             req.Email,
		Name:              strings.TrimSpace(req.Name),
		Password:          password,
		DepartmentID:      req.DepartmentID,
	})
	if err != nil {
		handleCurrentOrganizationMemberAdminError(c, err)
		return
	}

	response.Success(c, result)
}

func (h *OrganizationHandler) ResetCurrentOrganizationMemberPassword(c *gin.Context) {
	if !isSelfHostedEdition() {
		response.Fail(c, response.ErrSelfHostedOnly)
		return
	}

	accountID := middleware.GetAccountID(c)
	organizationID, ok := h.resolveCurrentOrganizationForMemberAdmin(c, accountID)
	if !ok {
		return
	}

	var req struct {
		Email    string `json:"email" binding:"required,email"`
		Password string `json:"password"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, err.Error())
		return
	}

	password, ok := resolveOrganizationInvitePassword(c, req.Password)
	if !ok {
		return
	}

	result, err := h.organizationService.ResetCurrentOrganizationMemberPassword(c.Request.Context(), &shared_dto.ResetCurrentOrganizationMemberPasswordRequest{
		OrganizationID:    organizationID,
		OperatorAccountID: accountID,
		Email:             req.Email,
		Password:          password,
	})
	if err != nil {
		handleCurrentOrganizationMemberAdminError(c, err)
		return
	}

	response.Success(c, result)
}

func (h *OrganizationHandler) resolveCurrentOrganizationForMemberAdmin(c *gin.Context, accountID string) (string, bool) {
	if accountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return "", false
	}

	organizationID, err := h.accountService.EnsureCurrentOrganizationID(c.Request.Context(), accountID)
	if err != nil || organizationID == "" {
		response.Fail(c, response.ErrOrganizationNotFound)
		return "", false
	}

	isAdmin, err := h.organizationService.IsOrganizationAdminOrOwner(c.Request.Context(), organizationID, accountID)
	if err != nil {
		response.Fail(c, response.ErrSystemError)
		return "", false
	}
	if !isAdmin {
		response.Fail(c, response.ErrPermissionDenied)
		return "", false
	}

	return organizationID, true
}

func isSelfHostedEdition() bool {
	cfg := config.Current()
	return cfg != nil && strings.EqualFold(strings.TrimSpace(cfg.Platform.Edition), "SELF_HOSTED")
}

func resolveOrganizationInvitePassword(c *gin.Context, provided string) (string, bool) {
	password := strings.TrimSpace(provided)
	if password != "" {
		return password, true
	}

	cfg := config.Current()
	if cfg == nil {
		response.Fail(c, response.ErrConfigError)
		return "", false
	}

	password = strings.TrimSpace(cfg.Platform.OrgInviteDefaultPassword)
	if password == "" {
		response.FailWithMessage(c, response.ErrConfigError, "organization invite default password is not configured")
		return "", false
	}

	return password, true
}

func handleCurrentOrganizationMemberAdminError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, workspace_service.ErrOrganizationInvitePermissionDenied),
		errors.Is(err, workspace_service.ErrOrganizationOwnerPasswordReset),
		errors.Is(err, workspace_service.ErrSuperAdminPasswordReset):
		response.Fail(c, response.ErrPermissionDenied)
	case errors.Is(err, workspace_service.ErrOrganizationMemberNotFound):
		response.Fail(c, response.ErrAccountNotFound)
	case errors.Is(err, workspace_service.ErrOrganizationInviteWorkspaceInvalid):
		response.Fail(c, response.ErrWorkspaceNotInOrganization)
	case errors.Is(err, workspace_service.ErrDepartmentNotFound):
		c.JSON(http.StatusNotFound, gin.H{"code": "DepartmentNotFound", "message": err.Error()})
	case errors.Is(err, workspace_service.ErrMemberAlreadyInDept):
		c.JSON(http.StatusBadRequest, gin.H{"code": "MemberAlreadyInDepartment", "message": err.Error()})
	default:
		response.Fail(c, response.ErrSystemError)
	}
}

func (h *OrganizationHandler) validateInviteDepartment(c *gin.Context, organizationID string, departmentID *string) (*model.Department, bool) {
	if departmentID == nil {
		return nil, true
	}

	deptID := strings.TrimSpace(*departmentID)
	if deptID == "" {
		return nil, true
	}

	dept, err := h.departmentService.GetDepartment(c.Request.Context(), deptID)
	if err != nil {
		if errors.Is(err, workspace_service.ErrDepartmentNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"code": "DepartmentNotFound", "message": err.Error()})
			return nil, false
		}
		response.Fail(c, response.ErrSystemError)
		return nil, false
	}
	if dept.OrganizationID != organizationID {
		response.Fail(c, response.ErrInvalidParam)
		return nil, false
	}

	return dept, true
}

func (h *OrganizationHandler) addCurrentOrganizationMemberToDepartment(c *gin.Context, organizationID, departmentID, accountID string) bool {
	_, err := h.departmentService.AddMemberToDepartment(c.Request.Context(), organizationID, departmentID, accountID)
	if err == nil {
		return true
	}

	if errors.Is(err, workspace_service.ErrDepartmentNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"code": "DepartmentNotFound", "message": err.Error()})
		return false
	}
	if errors.Is(err, workspace_service.ErrMemberAlreadyInDept) {
		currentDept, deptErr := h.departmentService.GetMemberDepartment(c.Request.Context(), organizationID, accountID)
		if deptErr != nil && !errors.Is(deptErr, workspace_service.ErrMemberNotInDept) {
			response.Fail(c, response.ErrSystemError)
			return false
		}

		resp := gin.H{
			"code":    "MemberAlreadyInDepartment",
			"message": err.Error(),
		}
		if currentDept != nil {
			resp["current_department"] = gin.H{
				"id":   currentDept.ID,
				"name": currentDept.Name,
			}
		}

		c.JSON(http.StatusBadRequest, resp)
		return false
	}

	response.Fail(c, response.ErrSystemError)
	return false
}

func (h *OrganizationHandler) UpdateMemberInfo(c *gin.Context) {
	organizationID := h.getOrganizationID(c)
	if organizationID == "" {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	targetAccountID := c.Param("account_id")
	if targetAccountID == "" {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	currentAccountID := c.GetString("account_id")
	if currentAccountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	// Check permissions: either updating self or is admin/owner
	if targetAccountID != currentAccountID {
		if !middleware.IsOrganizationAdminOrOwner(c) {
			response.Fail(c, response.ErrPermissionDenied)
			return
		}
	}

	var req shared_dto.UpdateOrganizationMemberRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}
	if req.Role != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, "use organization-role endpoint to update organization member role")
		return
	}

	// Force override IDs from path/context
	req.OrganizationID = organizationID
	req.AccountID = targetAccountID

	if err := h.organizationService.UpdateMemberInfo(c.Request.Context(), &req); err != nil {
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}

	response.Success(c, nil)
}

func (h *OrganizationHandler) applyDepartmentJoinApprovedEffects(c *gin.Context, req *model.OrganizationJoinRequest) error {
	isMember, _ := h.organizationService.IsOrganizationMember(c.Request.Context(), req.OrganizationID, req.AccountID)
	if !isMember {
		role := model.OrganizationRoleNormal
		if req.DefaultOrganizationRole != "" {
			role = model.OrganizationRole(req.DefaultOrganizationRole)
		}
		addReq := &dto.AddOrganizationMemberRequest{
			OrganizationID: req.OrganizationID,
			AccountID:      req.AccountID,
			Role:           role,
			Name:           req.Name,
		}
		if err := h.organizationService.AddMember(c.Request.Context(), addReq); err != nil {
			return err
		}
	}

	if req.DepartmentID != nil {
		_, err := h.departmentService.AddMemberToDepartment(c.Request.Context(), req.OrganizationID, *req.DepartmentID, req.AccountID)
		if err != nil && err != workspace_service.ErrMemberAlreadyInDept {
			return err
		}
	}

	return nil
}

func (h *OrganizationHandler) GetDepartmentInviteLink(c *gin.Context) {
	organizationID := h.getOrganizationID(c)
	if organizationID == "" {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	deptID := c.Query("department_id")

	if !middleware.IsOrganizationAdminOrOwner(c) {
		response.Fail(c, response.ErrPermissionDenied)
		return
	}

	accountID := c.GetString("account_id")
	link, err := h.organizationService.GetDepartmentInviteLink(c.Request.Context(), organizationID, deptID, accountID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			autoCreate := c.DefaultQuery("auto_create", "true")
			if autoCreate == "false" || autoCreate == "0" {
				response.Success(c, nil)
				return
			}

			requireApproval := true
			if requireApprovalStr := c.Query("require_approval"); requireApprovalStr != "" {
				if val, err := strconv.ParseBool(requireApprovalStr); err == nil {
					requireApproval = val
				}
			}

			link, err = h.organizationService.CreateOrResetDepartmentInviteLink(
				c.Request.Context(),
				organizationID,
				deptID,
				accountID,
				requireApproval,
				nil,
			)
			if err != nil {
				response.Fail(c, response.ErrSystemError)
				return
			}
		} else {
			response.Fail(c, response.ErrSystemError)
			return
		}
	}

	if link == nil {
		response.Success(c, nil)
		return
	}

	inviteURL := h.consoleWebURL + "/invite/" + link.Token

	response.Success(c, gin.H{
		"id":               link.ID,
		"token":            link.Token,
		"url":              inviteURL,
		"status":           link.Status,
		"require_approval": link.RequireApproval,
		"expires_at":       link.ExpiresAt,
		"created_at":       link.CreatedAt,
	})
}

func (h *OrganizationHandler) CreateOrResetDepartmentInviteLink(c *gin.Context) {
	organizationID := h.getOrganizationID(c)
	if organizationID == "" {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	if !middleware.IsOrganizationAdminOrOwner(c) {
		response.Fail(c, response.ErrPermissionDenied)
		return
	}

	var req struct {
		DepartmentID    *string    `json:"department_id"`
		RequireApproval bool       `json:"require_approval"`
		ExpiresAt       *time.Time `json:"expires_at"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	var deptID string
	if req.DepartmentID != nil {
		deptID = strings.TrimSpace(*req.DepartmentID)
	}

	accountID := c.GetString("account_id")
	link, err := h.organizationService.CreateOrResetDepartmentInviteLink(
		c.Request.Context(),
		organizationID,
		deptID,
		accountID,
		req.RequireApproval,
		req.ExpiresAt,
	)
	if err != nil {
		response.Fail(c, response.ErrSystemError)
		return
	}

	inviteURL := h.consoleWebURL + "/invite/" + link.Token

	response.Success(c, gin.H{
		"id":               link.ID,
		"token":            link.Token,
		"url":              inviteURL,
		"status":           link.Status,
		"require_approval": link.RequireApproval,
		"expires_at":       link.ExpiresAt,
		"created_at":       link.CreatedAt,
	})
}

func (h *OrganizationHandler) UpdateDepartmentInviteLinkStatus(c *gin.Context) {
	organizationID := h.getOrganizationID(c)
	if organizationID == "" {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	if !middleware.IsOrganizationAdminOrOwner(c) {
		response.Fail(c, response.ErrPermissionDenied)
		return
	}

	var req struct {
		DepartmentID *string `json:"department_id"`
		Status       string  `json:"status" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Status == "" {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	var deptID string
	if req.DepartmentID != nil {
		deptID = strings.TrimSpace(*req.DepartmentID)
	}

	accountID := c.GetString("account_id")
	link, err := h.organizationService.UpdateDepartmentInviteLinkStatus(
		c.Request.Context(),
		organizationID,
		deptID,
		accountID,
		req.Status,
	)
	if err != nil {
		response.Fail(c, response.ErrSystemError)
		return
	}

	response.Success(c, gin.H{"status": link.Status})
}

func (h *OrganizationHandler) ApproveDepartmentJoinRequest(c *gin.Context) {
	organizationID := h.getOrganizationID(c)
	reqID := c.Param("id")
	if organizationID == "" || reqID == "" {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	if !middleware.IsOrganizationAdminOrOwner(c) {
		response.Fail(c, response.ErrPermissionDenied)
		return
	}

	accountID := c.GetString("account_id")

	req, err := h.organizationService.ApproveDepartmentJoinRequest(c.Request.Context(), organizationID, reqID, accountID)
	if err != nil {
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}

	if err := h.applyDepartmentJoinApprovedEffects(c, req); err != nil {
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}

	response.Success(c, gin.H{"status": "approved"})
}

func (h *OrganizationHandler) BatchApproveDepartmentJoinRequests(c *gin.Context) {
	organizationID := h.getOrganizationID(c)
	if organizationID == "" {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	if !middleware.IsOrganizationAdminOrOwner(c) {
		response.Fail(c, response.ErrPermissionDenied)
		return
	}

	var reqBody struct {
		RequestIDs []string `json:"request_ids" binding:"required"`
	}
	if err := c.ShouldBindJSON(&reqBody); err != nil || len(reqBody.RequestIDs) == 0 {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	accountID := c.GetString("account_id")

	type failedItem struct {
		ID    string `json:"id"`
		Error string `json:"error"`
	}

	var successIDs []string
	var failed []failedItem

	for _, id := range reqBody.RequestIDs {
		req, err := h.organizationService.ApproveDepartmentJoinRequest(c.Request.Context(), organizationID, id, accountID)
		if err != nil {
			failed = append(failed, failedItem{ID: id, Error: err.Error()})
			continue
		}

		if err := h.applyDepartmentJoinApprovedEffects(c, req); err != nil {
			failed = append(failed, failedItem{ID: id, Error: err.Error()})
			continue
		}

		successIDs = append(successIDs, id)
	}

	response.Success(c, gin.H{
		"success_ids": successIDs,
		"failed":      failed,
	})
}

func (h *OrganizationHandler) RejectDepartmentJoinRequest(c *gin.Context) {
	organizationID := h.getOrganizationID(c)
	reqID := c.Param("id")
	if organizationID == "" || reqID == "" {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	if !middleware.IsOrganizationAdminOrOwner(c) {
		response.Fail(c, response.ErrPermissionDenied)
		return
	}

	var req struct {
		Reason string `json:"reason"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	accountID := c.GetString("account_id")
	var reason *string
	if req.Reason != "" {
		reason = &req.Reason
	}

	err := h.organizationService.RejectDepartmentJoinRequest(c.Request.Context(), organizationID, reqID, accountID, reason)
	if err != nil {
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}

	response.Success(c, gin.H{"status": "rejected"})
}

func (h *OrganizationHandler) BatchRejectDepartmentJoinRequests(c *gin.Context) {
	organizationID := h.getOrganizationID(c)
	if organizationID == "" {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	if !middleware.IsOrganizationAdminOrOwner(c) {
		response.Fail(c, response.ErrPermissionDenied)
		return
	}

	var reqBody struct {
		RequestIDs []string `json:"request_ids" binding:"required"`
		Reason     string   `json:"reason"`
	}
	if err := c.ShouldBindJSON(&reqBody); err != nil || len(reqBody.RequestIDs) == 0 {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	accountID := c.GetString("account_id")
	var reason *string
	if reqBody.Reason != "" {
		reason = &reqBody.Reason
	}

	type failedItem struct {
		ID    string `json:"id"`
		Error string `json:"error"`
	}

	var successIDs []string
	var failed []failedItem

	for _, id := range reqBody.RequestIDs {
		err := h.organizationService.RejectDepartmentJoinRequest(c.Request.Context(), organizationID, id, accountID, reason)
		if err != nil {
			failed = append(failed, failedItem{ID: id, Error: err.Error()})
			continue
		}
		successIDs = append(successIDs, id)
	}

	response.Success(c, gin.H{
		"success_ids": successIDs,
		"failed":      failed,
	})
}

func (h *OrganizationHandler) ListOrganizationJoinRequests(c *gin.Context) {
	organizationID := h.getOrganizationID(c)
	if organizationID == "" {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	accountID := c.GetString("account_id")
	if accountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	if !middleware.IsOrganizationAdminOrOwner(c) {
		response.Fail(c, response.ErrPermissionDenied)
		return
	}

	page := 1
	limit := 20
	if pageStr := c.Query("page"); pageStr != "" {
		if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
			page = p
		}
	}
	if limitStr := c.Query("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 100 {
			limit = l
		}
	}

	statusStr := c.Query("status")
	var status *model.OrganizationJoinRequestStatus
	if statusStr != "" {
		s := model.OrganizationJoinRequestStatus(statusStr)
		status = &s
	}

	var departmentID *string
	if deptID := c.Query("department_id"); deptID != "" {
		departmentID = &deptID
	}

	pagination, err := h.organizationService.ListOrganizationJoinRequests(c.Request.Context(), organizationID, accountID, departmentID, status, page, limit)
	if err != nil {
		response.Fail(c, response.ErrSystemError)
		return
	}

	response.Success(c, pagination)
}

func (h *OrganizationHandler) GetOrganizationMemberDetailByID(c *gin.Context) {
	organizationID := h.getOrganizationID(c)
	memberID := c.Param("member_id")
	if organizationID == "" || memberID == "" {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	accountID := c.GetString("account_id")
	if accountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	_, err := h.organizationService.GetByID(c.Request.Context(), organizationID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			response.Fail(c, response.ErrOrganizationNotFound)
			return
		}
		response.Fail(c, response.ErrSystemError)
		return
	}

	isMember, err := h.organizationService.IsOrganizationMember(c.Request.Context(), organizationID, accountID)
	if err != nil {
		response.Fail(c, response.ErrSystemError)
		return
	}
	if !isMember {
		response.Fail(c, response.ErrPermissionDenied)
		return
	}

	member, err := h.organizationService.GetOrganizationMemberByAccountID(c.Request.Context(), organizationID, memberID)
	if err != nil {
		if strings.Contains(err.Error(), "member not found") {
			response.Fail(c, response.ErrMemberNotFound)
			return
		}
		response.Fail(c, response.ErrSystemError)
		return
	}

	response.Success(c, member)
}

// RemoveOrganizationMember removes a member from organization and all related tenants
func (h *OrganizationHandler) RemoveOrganizationMember(c *gin.Context) {
	organizationID := h.getOrganizationID(c)
	memberID := c.Param("member_id")

	if organizationID == "" || memberID == "" {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	// Get current operator
	currentAccountID := c.GetString("account_id")
	if currentAccountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	// Cannot remove yourself
	if memberID == currentAccountID {
		response.FailWithMessage(c, response.ErrInvalidParam, "cannot remove yourself")
		return
	}

	// Check if organization exists
	_, err := h.organizationService.GetByID(c.Request.Context(), organizationID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			response.Fail(c, response.ErrOrganizationNotFound)
			return
		}
		response.Fail(c, response.ErrSystemError)
		return
	}

	// Check operator permission (must be owner or admin)
	operatorRole, err := h.organizationService.GetUserOrganizationRole(c.Request.Context(), organizationID, currentAccountID)
	if err != nil {
		response.Fail(c, response.ErrSystemError)
		return
	}
	if operatorRole != model.OrganizationRoleOwner && operatorRole != model.OrganizationRoleAdmin {
		response.FailWithMessage(c, response.ErrPermissionDenied, "only owner or admin can remove members")
		return
	}

	// Check target member role to prevent admin from removing owner or other admins
	if operatorRole == model.OrganizationRoleAdmin {
		targetRole, err := h.organizationService.GetUserOrganizationRole(c.Request.Context(), organizationID, memberID)
		if err != nil {
			response.Fail(c, response.ErrSystemError)
			return
		}
		if targetRole == model.OrganizationRoleOwner || targetRole == model.OrganizationRoleAdmin {
			response.FailWithMessage(c, response.ErrPermissionDenied, "admin cannot remove owner or other admins")
			return
		}
	}

	// Remove member (cascading delete: organization role + all tenant memberships)
	if err := h.organizationService.RemoveMember(c.Request.Context(), organizationID, memberID); err != nil {
		if strings.Contains(err.Error(), "cannot remove") {
			response.FailWithMessage(c, response.ErrPermissionDenied, err.Error())
			return
		}
		if strings.Contains(err.Error(), "not exist") {
			response.FailWithMessage(c, response.ErrNotFound, err.Error())
			return
		}
		response.Fail(c, response.ErrSystemError)
		return
	}

	response.Success(c, gin.H{"message": "member removed successfully"})
}

func (h *OrganizationHandler) LeaveOrganization(c *gin.Context) {
	organizationID := h.getOrganizationID(c)
	if organizationID == "" {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	accountID := c.GetString("account_id")
	if accountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	userRole, err := h.organizationService.GetUserOrganizationRole(c.Request.Context(), organizationID, accountID)
	if err != nil {
		response.Fail(c, response.ErrSystemError)
		return
	}
	if userRole == model.OrganizationRoleOwner {
		response.FailWithMessage(c, response.ErrPermissionDenied, "cannot leave as organization owner")
		return
	}

	joinedOrganizations, err := h.organizationService.ListUserOrganizations(c.Request.Context(), 1, 1000, "", accountID)
	if err != nil {
		response.Fail(c, response.ErrSystemError)
		return
	}
	if joinedOrganizations != nil && joinedOrganizations.Total <= 1 {
		response.FailWithMessage(c, response.ErrPermissionDenied, "cannot leave the last joined organization")
		return
	}

	if err := h.organizationService.RemoveMember(c.Request.Context(), organizationID, accountID); err != nil {
		errMsg := err.Error()
		if strings.Contains(errMsg, "organization not found") {
			response.Fail(c, response.ErrOrganizationNotFound)
			return
		}
		if strings.Contains(errMsg, "cannot remove") {
			response.FailWithMessage(c, response.ErrPermissionDenied, errMsg)
			return
		}
		if strings.Contains(errMsg, "not exist") {
			response.FailWithMessage(c, response.ErrNotFound, errMsg)
			return
		}
		response.Fail(c, response.ErrSystemError)
		return
	}

	ctxModel, err := h.accountService.GetAccountContext(c.Request.Context(), accountID)
	if err != nil {
		response.Fail(c, response.ErrSystemError)
		return
	}

	if ctxModel != nil && ctxModel.CurrentOrganizationID != nil && *ctxModel.CurrentOrganizationID == organizationID {
		emptyOrganizationID := ""
		emptyWorkspaceID := ""
		if _, err := h.accountService.UpdateAccountContext(c.Request.Context(), accountID, &emptyOrganizationID, &emptyWorkspaceID); err != nil {
			response.Fail(c, response.ErrSystemError)
			return
		}
	}

	response.Success(c, gin.H{"message": "success"})
}

// UpdateOrganizationMember updates member information (e.g. nickname)
func (h *OrganizationHandler) UpdateOrganizationMember(c *gin.Context) {
	organizationID := h.getOrganizationID(c)
	memberID := c.Param("member_id")

	if organizationID == "" || memberID == "" {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	var req shared_dto.UpdateOrganizationMemberRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, err.Error())
		return
	}
	if req.Role != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, "use organization-role endpoint to update organization member role")
		return
	}

	req.OrganizationID = organizationID
	req.AccountID = memberID
	req.MemberID = memberID

	currentAccountID := c.GetString("account_id")
	if currentAccountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	// Check permission:
	// 1. Updating role requires Admin/Owner permission (even for self)
	// 2. Updating others' info requires Admin/Owner permission
	// 3. Updating self info (except role) is allowed
	isSelf := currentAccountID == memberID
	requiresPrivilege := !isSelf || req.Role != nil

	if requiresPrivilege {
		isPrivileged, err := h.organizationService.IsOrganizationAdminOrOwner(c.Request.Context(), organizationID, currentAccountID)
		if err != nil {
			response.Fail(c, response.ErrSystemError)
			return
		}
		if !isPrivileged {
			response.Fail(c, response.ErrPermissionDenied)
			return
		}
	}

	if err := h.organizationService.UpdateMemberInfo(c.Request.Context(), &req); err != nil {
		if strings.Contains(err.Error(), "not found") {
			response.Fail(c, response.ErrMemberNotFound)
			return
		}
		if errors.Is(err, workspace_service.ErrMemberNameExists) {
			response.FailWithMessage(c, response.ErrInvalidParam, "member name already exists")
			return
		}
		response.Fail(c, response.ErrSystemError)
		return
	}

	response.Success(c, gin.H{"result": "success"})
}

func (h *OrganizationHandler) UpdateCurrentOrganizationMemberRole(c *gin.Context) {
	memberID := c.Param("member_id")
	if memberID == "" {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	accountID := c.GetString("account_id")
	if accountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	var req shared_dto.UpdateCurrentOrganizationMemberRoleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	if err := h.organizationService.UpdateCurrentOrganizationMemberRole(c.Request.Context(), accountID, memberID, req.Role); err != nil {
		h.handleUpdateOrganizationMemberRoleError(c, err)
		return
	}

	response.Success(c, gin.H{"result": "success"})
}

func (h *OrganizationHandler) handleUpdateOrganizationMemberRoleError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, workspace_service.ErrInvalidOrganizationMemberRole),
		errors.Is(err, workspace_service.ErrOrganizationOwnerRoleImmutable),
		errors.Is(err, workspace_service.ErrOrganizationMemberNotActive),
		errors.Is(err, workspace_service.ErrOrganizationNotEditable):
		response.Fail(c, response.ErrInvalidParam)
	case errors.Is(err, workspace_service.ErrOrganizationMemberNotFound):
		response.Fail(c, response.ErrMemberNotFound)
	case errors.Is(err, workspace_service.ErrOrganizationPermissionDenied):
		response.Fail(c, response.ErrPermissionDenied)
	case errors.Is(err, workspace_service.ErrOrganizationNotFound):
		response.Fail(c, response.ErrOrganizationNotFound)
	default:
		response.Fail(c, response.ErrSystemError)
	}
}

// UpdateMemberStatus updates the status (active/inactive) of a member in organization
func (h *OrganizationHandler) UpdateMemberStatus(c *gin.Context) {
	organizationID := h.getOrganizationID(c)
	memberID := c.Param("member_id")

	if organizationID == "" || memberID == "" {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	// Parse request body
	var req struct {
		Status model.OrganizationMemberStatus `json:"status" binding:"required,oneof=active inactive"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, err.Error())
		return
	}

	// Get current operator
	currentAccountID := c.GetString("account_id")
	if currentAccountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	// Cannot disable yourself
	if memberID == currentAccountID && req.Status == model.OrganizationMemberStatusInactive {
		response.FailWithMessage(c, response.ErrInvalidParam, "cannot disable yourself")
		return
	}

	// Check operator permission (must be owner or admin)
	operatorRole, err := h.organizationService.GetUserOrganizationRole(c.Request.Context(), organizationID, currentAccountID)
	if err != nil {
		response.Fail(c, response.ErrSystemError)
		return
	}
	if operatorRole != model.OrganizationRoleOwner && operatorRole != model.OrganizationRoleAdmin {
		response.FailWithMessage(c, response.ErrPermissionDenied, "only owner or admin can update member status")
		return
	}

	// Update member status
	updateReq := &dto.UpdateOrganizationMemberStatusRequest{
		OrganizationID: organizationID,
		AccountID:      memberID,
		Status:         req.Status,
	}
	if err := h.organizationService.UpdateMemberStatus(c.Request.Context(), updateReq); err != nil {
		if strings.Contains(err.Error(), "cannot disable") {
			response.FailWithMessage(c, response.ErrPermissionDenied, err.Error())
			return
		}
		if strings.Contains(err.Error(), "not exist") || strings.Contains(err.Error(), "not found") {
			response.FailWithMessage(c, response.ErrNotFound, err.Error())
			return
		}
		response.Fail(c, response.ErrSystemError)
		return
	}

	response.Success(c, gin.H{"message": "member status updated successfully"})
}

// RemoveCurrentOrganizationMember removes a member from current organization
func (h *OrganizationHandler) RemoveCurrentOrganizationMember(c *gin.Context) {
	organizationID := helper.GetOrganizationID(c)
	memberID := c.Param("member_id")

	if organizationID == "" || memberID == "" {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	// Get current operator
	currentAccountID := c.GetString("account_id")
	if currentAccountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	// Cannot remove yourself
	if memberID == currentAccountID {
		response.FailWithMessage(c, response.ErrInvalidParam, "cannot remove yourself")
		return
	}

	// Check if organization exists
	_, err := h.organizationService.GetByID(c.Request.Context(), organizationID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			response.Fail(c, response.ErrOrganizationNotFound)
			return
		}
		response.Fail(c, response.ErrSystemError)
		return
	}

	// Check operator permission (must be owner or admin)
	operatorRole, err := h.organizationService.GetUserOrganizationRole(c.Request.Context(), organizationID, currentAccountID)
	if err != nil {
		response.Fail(c, response.ErrSystemError)
		return
	}
	if operatorRole != model.OrganizationRoleOwner && operatorRole != model.OrganizationRoleAdmin {
		response.FailWithMessage(c, response.ErrPermissionDenied, "only owner or admin can remove members")
		return
	}

	// Remove member (cascading delete: organization role + all tenant memberships)
	if err := h.organizationService.RemoveMember(c.Request.Context(), organizationID, memberID); err != nil {
		if strings.Contains(err.Error(), "cannot remove") {
			response.FailWithMessage(c, response.ErrPermissionDenied, err.Error())
			return
		}
		if strings.Contains(err.Error(), "not exist") {
			response.FailWithMessage(c, response.ErrNotFound, err.Error())
			return
		}
		response.Fail(c, response.ErrSystemError)
		return
	}

	response.Success(c, gin.H{"message": "member removed successfully"})
}

func (h *OrganizationHandler) GetCurrentOrganizationMembers(c *gin.Context) {
	organizationID := helper.GetOrganizationID(c)
	if organizationID == "" {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	accountID := c.GetString("account_id")
	if accountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	// Check if organization exists
	_, err := h.organizationService.GetByID(c.Request.Context(), organizationID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			response.Fail(c, response.ErrOrganizationNotFound)
			return
		}
		response.Fail(c, response.ErrSystemError)
		return
	}

	// If not system admin, check if has management permissions
	hasPermission, err := h.organizationService.CheckAnyManagedWorkspacePermission(c.Request.Context(), organizationID, accountID)
	if err != nil {
		response.Fail(c, response.ErrSystemError)
		return
	}
	if !hasPermission {
		response.Fail(c, response.ErrPermissionDenied)
		return
	}

	page := 1
	limit := 20
	if pageStr := c.Query("page"); pageStr != "" {
		if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
			page = p
		}
	}
	if limitStr := c.Query("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}
	}

	// Call service to get paginated data
	keyword := c.Query("keyword")
	if keyword == "" {
		keyword = c.Query("search")
	}
	pagination, err := h.organizationService.GetOrganizationMembersPaginated(c.Request.Context(), organizationID, page, limit, keyword)
	if err != nil {
		response.Fail(c, response.ErrSystemError)
		return
	}

	// Return paginated data
	response.Success(c, pagination)
}

func (h *OrganizationHandler) CheckAppPermission(c *gin.Context) {
	organizationID := h.getOrganizationID(c)
	if organizationID == "" {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	// Permission validation
	accountID := c.GetString("account_id")
	if accountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	// Call new service method to check app permissions
	hasPermission, err := h.organizationService.CheckAnyWorkspaceCreateAppPermission(c.Request.Context(), organizationID, accountID)
	if err != nil {
		response.Fail(c, response.ErrSystemError)
		return
	}

	response.Success(c, gin.H{
		"has_permission": hasPermission,
	})
}

func (h *OrganizationHandler) CheckDatasetPermission(c *gin.Context) {
	organizationID := h.getOrganizationID(c)
	if organizationID == "" {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	accountID := c.GetString("account_id")
	if accountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	// Add debug logging
	// log.Printf("CheckDatasetPermission called with organizationID: '%s', accountID: '%s'", organizationID, accountID) // Original code had this line commented out

	// Call organization service permission check method
	hasPermission, err := h.organizationService.CheckAnyWorkspaceCreateDatasetPermission(c.Request.Context(), organizationID, accountID)
	if err != nil {
		response.Fail(c, response.ErrSystemError)
		return
	}

	response.Success(c, gin.H{
		"has_permission": hasPermission,
	})
}

func (h *OrganizationHandler) GetOrganizations(c *gin.Context) {
	// Parse query parameters
	page := 1
	limit := 20
	status := c.Query("status")

	if pageStr := c.Query("page"); pageStr != "" {
		if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
			page = p
		}
	}
	if limitStr := c.Query("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 100 {
			limit = l
		}
	}

	accountID := c.GetString("account_id")
	if accountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	pagination, err := h.organizationService.ListUserOrganizations(c.Request.Context(), page, limit, status, accountID)
	if err != nil {
		response.Fail(c, response.ErrSystemError)
		return
	}

	response.Success(c, pagination)
}

func (h *OrganizationHandler) UpdateOrganization(c *gin.Context) {
	var req struct {
		ID        string  `json:"id" binding:"required"`
		Name      string  `json:"name" binding:"required"`
		ShortName *string `json:"short_name"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	accountID := c.GetString("account_id")
	if accountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	organization, err := h.organizationService.UpdateOrganization(c.Request.Context(), req.ID, accountID, &shared_dto.UpdateOrganizationRequest{
		Name:      req.Name,
		ShortName: req.ShortName,
	})
	if err != nil {
		h.handleUpdateOrganizationError(c, err)
		return
	}

	response.Success(c, organization)
}

func (h *OrganizationHandler) PatchOrganization(c *gin.Context) {
	organizationID := h.getOrganizationID(c)
	if organizationID == "" {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	var req struct {
		Name      string  `json:"name" binding:"required"`
		ShortName *string `json:"short_name"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	accountID := c.GetString("account_id")
	if accountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	organization, err := h.organizationService.UpdateOrganization(c.Request.Context(), organizationID, accountID, &shared_dto.UpdateOrganizationRequest{
		Name:      req.Name,
		ShortName: req.ShortName,
	})
	if err != nil {
		h.handleUpdateOrganizationError(c, err)
		return
	}

	response.Success(c, organization)
}

func (h *OrganizationHandler) handleUpdateOrganizationError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, workspace_service.ErrInvalidOrganizationName):
		response.Fail(c, response.ErrInvalidParam)
	case errors.Is(err, workspace_service.ErrOrganizationNotFound):
		response.Fail(c, response.ErrOrganizationNotFound)
	case errors.Is(err, workspace_service.ErrOrganizationNameExists):
		response.Fail(c, response.ErrOrganizationExists)
	case errors.Is(err, workspace_service.ErrOrganizationPermissionDenied):
		response.Fail(c, response.ErrPermissionDenied)
	case errors.Is(err, workspace_service.ErrOrganizationNotEditable):
		response.Fail(c, response.ErrInvalidParam)
	default:
		response.Fail(c, response.ErrSystemError)
	}
}

func (h *OrganizationHandler) DeleteOrganization(c *gin.Context) {
	var req struct {
		ID string `json:"id" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	accountID := c.GetString("account_id")
	if accountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	// Delete organization (permission check in service layer)
	err := h.organizationService.DeleteOrganization(c.Request.Context(), req.ID, accountID)
	if err != nil {
		// Matches API error handling: ValueError returns 400 error
		if strings.Contains(err.Error(), "insufficient permissions") {
			response.Fail(c, response.ErrPermissionDenied)
			return
		}
		if err.Error() == "organization not found" || strings.Contains(err.Error(), "not found") {
			// Matches API: ValueError returns 400 status code and InvalidRequest error code
			response.Fail(c, response.ErrOrganizationNotFound)
			return
		}
		response.Fail(c, response.ErrSystemError)
		return
	}

	// Return 204 status code and success result
	response.Success(c, gin.H{"result": "success"})
}

func (h *OrganizationHandler) GetOrganizationWorkspaces(c *gin.Context) {
	organizationID := h.getOrganizationID(c)
	if organizationID == "" {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	page := 1
	limit := 20
	if pageStr := c.Query("page"); pageStr != "" {
		if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
			page = p
		}
	}
	if limitStr := c.Query("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 100 {
			limit = l
		}
	}

	status := c.Query("status")
	if status != "" && status != string(model.WorkspaceStatusNormal) && status != string(model.WorkspaceStatusArchived) {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	keyword := c.Query("keyword")

	accountID := c.GetString("account_id")
	if accountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	// Get organization tenant list (pass complete parameters
	paginationResult, err := h.organizationService.GetOrganizationWorkspacesWithDetails(c.Request.Context(), organizationID, page, limit, accountID, status, keyword)
	if err != nil {
		// Matches API error handling format
		if strings.Contains(err.Error(), "organization not found") {
			response.Fail(c, response.ErrOrganizationNotFound)
			return
		}
		if strings.Contains(err.Error(), "user not found") {
			response.Fail(c, response.ErrAccountNotFound)
			return
		}
		response.Fail(c, response.ErrSystemError)
		return
	}

	// Return paginated results
	response.Success(c, gin.H{
		"page":     paginationResult.Page,
		"limit":    paginationResult.Limit,
		"total":    paginationResult.Total,
		"has_more": paginationResult.HasMore,
		"data":     paginationResult.Data,
	})
}

func (h *OrganizationHandler) GetOrganizationWorkspaceDetail(c *gin.Context) {
	organizationID := h.getOrganizationID(c)
	workspaceID := c.Param("workspace_id")
	if organizationID == "" || workspaceID == "" {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	accountID := c.GetString("account_id")
	if accountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	detail, err := h.organizationService.GetOrganizationWorkspaceDetail(c.Request.Context(), organizationID, workspaceID, accountID)
	if handleOrganizationWorkspaceDetailError(c, err) {
		return
	}

	response.Success(c, detail)
}

func (h *OrganizationHandler) GetOrganizationWorkspaceMembers(c *gin.Context) {
	organizationID := h.getOrganizationID(c)
	workspaceID := c.Param("workspace_id")
	if organizationID == "" || workspaceID == "" {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	accountID := c.GetString("account_id")
	if accountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	_, err := h.organizationService.GetOrganizationWorkspaceDetail(c.Request.Context(), organizationID, workspaceID, accountID)
	if handleOrganizationWorkspaceDetailError(c, err) {
		return
	}

	keyword := c.Query("keyword")
	roleFilter := c.Query("role")

	page := 1
	limit := 20
	if pageStr := c.Query("page"); pageStr != "" {
		if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
			page = p
		}
	}
	if limitStr := c.Query("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 100 {
			limit = l
		}
	}

	pagedMembers, total, err := h.workspaceManagementService.GetWorkspaceMembersPaginated(c.Request.Context(), workspaceID, page, limit, keyword, roleFilter)
	if err != nil {
		response.Fail(c, response.ErrSystemError)
		return
	}

	roleNameByID := make(map[string]string)
	builtinRoleIDByWorkspaceRole := map[string]string{
		string(model.WorkspaceRoleOwner):  model.WorkspaceBuiltinRoleOwnerID,
		string(model.WorkspaceRoleAdmin):  model.WorkspaceBuiltinRoleAdminID,
		string(model.WorkspaceRoleEditor): model.WorkspaceBuiltinRoleMemberID,
		string(model.WorkspaceRoleNormal): model.WorkspaceBuiltinRoleMemberID,
		string(model.WorkspaceRoleMember): model.WorkspaceBuiltinRoleMemberID,
		string(model.WorkspaceRoleViewer): model.WorkspaceBuiltinRoleViewerID,
	}
	hasCustomRole := false
	for _, m := range pagedMembers {
		if m.RoleID != nil {
			hasCustomRole = true
			break
		}
	}
	if hasCustomRole {
		roleList, err := h.organizationService.ListWorkspaceRoles(c.Request.Context(), organizationID, accountID, true)
		if err == nil && roleList != nil {
			for _, r := range roleList.Roles {
				roleNameByID[r.ID] = r.Name
			}
		}
	}

	results := make([]*TenantMemberWithDepartmentResponse, len(pagedMembers))
	for i, m := range pagedMembers {
		var deptID *string
		var deptName *string

		dept, err := h.departmentService.GetMemberDepartment(c.Request.Context(), organizationID, m.ID)
		if err != nil {
			if err != workspace_service.ErrMemberNotInDept {
				response.Fail(c, response.ErrSystemError)
				return
			}
		} else if dept != nil {
			deptID = &dept.ID
			deptName = &dept.Name
		}

		var roleID *string
		if m.RoleID != nil {
			roleID = m.RoleID
		} else {
			if id, ok := builtinRoleIDByWorkspaceRole[m.Role]; ok {
				idCopy := id
				roleID = &idCopy
			}
		}

		roleName := ""
		if roleName == "" && roleID != nil {
			if name, ok := roleNameByID[*roleID]; ok {
				roleName = name
			}
		}
		if roleName == "" {
			roleName = getWorkspaceRoleDisplayName(m.Role)
		}

		results[i] = &TenantMemberWithDepartmentResponse{
			ID:             m.ID,
			Name:           m.Name,
			AccountName:    m.AccountName,
			MemberName:     m.MemberName,
			Avatar:         m.Avatar,
			AvatarURL:      m.AvatarURL,
			Email:          m.Email,
			LastLoginAt:    m.LastLoginAt,
			LastActiveAt:   m.LastActiveAt,
			CreatedAt:      m.CreatedAt,
			Role:           m.Role,
			RoleID:         roleID,
			RoleName:       roleName,
			Status:         m.Status,
			HasMobile:      m.HasMobile,
			DepartmentID:   deptID,
			DepartmentName: deptName,
		}
	}

	hasMore := int64(page*limit) < total

	response.Success(c, gin.H{
		"data":     results,
		"total":    total,
		"page":     page,
		"limit":    limit,
		"has_more": hasMore,
	})
}

func (h *OrganizationHandler) GetOrganizationWorkspaceMemberDetailByID(c *gin.Context) {
	organizationID := h.getOrganizationID(c)
	workspaceID := c.Param("workspace_id")
	memberID := c.Param("member_id")
	if organizationID == "" || workspaceID == "" || memberID == "" {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	accountID := c.GetString("account_id")
	if accountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	_, err := h.organizationService.GetOrganizationWorkspaceDetail(c.Request.Context(), organizationID, workspaceID, accountID)
	if handleOrganizationWorkspaceDetailError(c, err) {
		return
	}

	members, err := h.workspaceManagementService.GetWorkspaceMembers(c.Request.Context(), workspaceID)
	if err != nil {
		response.Fail(c, response.ErrSystemError)
		return
	}

	var target *interfaces.AccountWithRole
	for _, m := range members {
		if m.ID == memberID {
			target = m
			break
		}
	}

	if target == nil {
		response.Fail(c, response.ErrMemberNotInWorkspace)
		return
	}

	var deptID *string
	var deptName *string

	dept, err := h.departmentService.GetMemberDepartment(c.Request.Context(), organizationID, target.ID)
	if err != nil {
		if err != workspace_service.ErrMemberNotInDept {
			response.Fail(c, response.ErrSystemError)
			return
		}
	} else if dept != nil {
		deptID = &dept.ID
		deptName = &dept.Name
	}

	result := &TenantMemberWithDepartmentResponse{
		ID:             target.ID,
		Name:           target.Name,
		AccountName:    target.AccountName,
		MemberName:     target.MemberName,
		Avatar:         target.Avatar,
		AvatarURL:      target.AvatarURL,
		Email:          target.Email,
		LastLoginAt:    target.LastLoginAt,
		LastActiveAt:   target.LastActiveAt,
		CreatedAt:      target.CreatedAt,
		Role:           target.Role,
		Status:         target.Status,
		HasMobile:      target.HasMobile,
		DepartmentID:   deptID,
		DepartmentName: deptName,
	}

	response.Success(c, result)
}

func (h *OrganizationHandler) GetOrganizationDatasets(c *gin.Context) {

	organizationID := h.getOrganizationID(c)
	if organizationID == "" {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	page := 1
	limit := 20
	var search *string

	// Parse page parameter (default 1)
	if pageStr := c.Query("page"); pageStr != "" {
		if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
			page = p
		}
	}

	// Parse limit parameter (default 20)
	if limitStr := c.Query("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 100 {
			limit = l
		}
	}

	// Parse search parameter (optional)
	if searchStr := c.Query("search"); searchStr != "" {
		search = &searchStr
	}

	accountID := c.GetString("account_id")
	if accountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	organization, err := h.organizationService.GetOrganizationByID(c.Request.Context(), organizationID)
	if err != nil || organization == nil {
		response.Fail(c, response.ErrOrganizationNotFound)
		return
	}

	pagination, err := h.organizationService.GetOrganizationDatasetsPaginated(c.Request.Context(), &shared_dto.GetOrganizationDatasetsPaginatedRequest{
		OrganizationID: organizationID,
		Page:           page,
		PerPage:        limit,
		Search:         search,
		UserID:         accountID,
	})
	if err != nil {

		response.Fail(c, response.ErrSystemError)
		return
	}

	response.Success(c, gin.H{
		"page":     pagination.Page,
		"limit":    pagination.PerPage,
		"total":    pagination.Total,
		"has_more": pagination.HasMore,
		"data":     pagination.Data,
	})
}

func (h *OrganizationHandler) RemoveOrganizationWorkspaceMember(c *gin.Context) {
	organizationID := h.getOrganizationID(c)
	workspaceID := c.Param("workspace_id")
	memberID := c.Param("member_id")
	if organizationID == "" || workspaceID == "" || memberID == "" {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	accountID := c.GetString("account_id")
	if accountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	if h.organizationService != nil {
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
		if !hasPermission {
			response.Fail(c, response.ErrPermissionDenied)
			return
		}
	}

	_, err := h.organizationService.GetOrganizationWorkspaceDetail(c.Request.Context(), organizationID, workspaceID, accountID)
	if handleOrganizationWorkspaceDetailError(c, err) {
		return
	}

	member, err := h.accountService.GetAccountByID(c.Request.Context(), memberID)
	if err != nil || member == nil {
		response.Fail(c, response.ErrMemberNotFound)
		return
	}

	currentUser, err := h.accountService.GetAccountByID(c.Request.Context(), accountID)
	if err != nil || currentUser == nil {
		response.Fail(c, response.ErrAccountNotFound)
		return
	}

	targetWorkspace, err := h.workspaceManagementService.GetWorkspaceByID(c.Request.Context(), workspaceID)
	if err != nil || targetWorkspace == nil {
		response.Fail(c, response.ErrWorkspaceNotFound)
		return
	}

	err = h.workspaceManagementService.RemoveMemberFromWorkspace(c.Request.Context(), targetWorkspace, member, currentUser)
	if err != nil {
		switch {
		case isCannotOperateSelfError(err):
			response.Fail(c, response.ErrCannotOperateSelf)
			return
		case isNoPermissionError(err):
			response.Fail(c, response.ErrPermissionDenied)
			return
		case isMemberNotInWorkspaceError(err):
			response.Fail(c, response.ErrMemberNotInWorkspace)
			return
		default:
			response.Fail(c, response.ErrSystemError)
			return
		}
	}

	response.Success(c, gin.H{
		"result": "success",
	})
}

func (h *OrganizationHandler) LeaveOrganizationWorkspace(c *gin.Context) {
	organizationID := h.getOrganizationID(c)
	workspaceID := c.Param("workspace_id")
	if organizationID == "" || workspaceID == "" {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	accountID := c.GetString("account_id")
	if accountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	_, err := h.organizationService.GetOrganizationWorkspaceDetail(c.Request.Context(), organizationID, workspaceID, accountID)
	if handleOrganizationWorkspaceDetailError(c, err) {
		return
	}

	err = h.workspaceManagementService.LeaveWorkspace(c.Request.Context(), workspaceID, accountID)
	if err != nil {
		errMsg := err.Error()
		if strings.Contains(errMsg, "member not found") {
			response.Fail(c, response.ErrMemberNotInWorkspace)
			return
		}
		response.Fail(c, response.ErrSystemError)
		return
	}

	emptyWorkspaceID := ""
	if _, err := h.accountService.UpdateAccountContext(c.Request.Context(), accountID, nil, &emptyWorkspaceID); err != nil {
		response.Fail(c, response.ErrSystemError)
		return
	}

	response.Success(c, gin.H{
		"result": "success",
	})
}

func (h *OrganizationHandler) UpdateOrganizationWorkspaceMemberRole(c *gin.Context) {
	organizationID := h.getOrganizationID(c)
	workspaceID := c.Param("workspace_id")
	memberID := c.Param("member_id")
	if organizationID == "" || workspaceID == "" || memberID == "" {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	var req struct {
		Role   *string `json:"role,omitempty"`
		RoleID *string `json:"role_id,omitempty"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	if (req.Role == nil || *req.Role == "") && (req.RoleID == nil || *req.RoleID == "") {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	accountID := c.GetString("account_id")
	if accountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	if h.organizationService != nil {
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
		if !hasPermission {
			response.Fail(c, response.ErrPermissionDenied)
			return
		}
	}

	_, err := h.organizationService.GetOrganizationWorkspaceDetail(c.Request.Context(), organizationID, workspaceID, accountID)
	if handleOrganizationWorkspaceDetailError(c, err) {
		return
	}

	member, err := h.accountService.GetAccountByID(c.Request.Context(), memberID)
	if err != nil || member == nil {
		response.Fail(c, response.ErrMemberNotFound)
		return
	}

	currentUser, err := h.accountService.GetAccountByID(c.Request.Context(), accountID)
	if err != nil || currentUser == nil {
		response.Fail(c, response.ErrAccountNotFound)
		return
	}

	targetWorkspace, err := h.workspaceManagementService.GetWorkspaceByID(c.Request.Context(), workspaceID)
	if err != nil || targetWorkspace == nil {
		response.Fail(c, response.ErrWorkspaceNotFound)
		return
	}

	if req.Role != nil && *req.Role != "" {
		newRole := *req.Role
		role := model.WorkspaceMemberRole(newRole)
		if !role.IsValidRole() {
			response.Fail(c, response.ErrInvalidRoleType)
			return
		}

		err = h.workspaceManagementService.UpdateMemberRoleWithPermissionCheck(c.Request.Context(), targetWorkspace, member, newRole, currentUser)
	} else {
		roleID := *req.RoleID

		builtinWorkspaceRole := ""
		switch roleID {
		case model.WorkspaceBuiltinRoleOwnerID:
			builtinWorkspaceRole = string(model.WorkspaceRoleOwner)
		case model.WorkspaceBuiltinRoleAdminID:
			builtinWorkspaceRole = string(model.WorkspaceRoleAdmin)
		case model.WorkspaceBuiltinRoleMemberID:
			builtinWorkspaceRole = string(model.WorkspaceRoleNormal)
		case model.WorkspaceBuiltinRoleViewerID:
			builtinWorkspaceRole = string(model.WorkspaceRoleViewer)
		}

		if builtinWorkspaceRole != "" {
			err = h.workspaceManagementService.UpdateMemberRoleAndRoleIDWithPermissionCheck(c.Request.Context(), targetWorkspace, member, builtinWorkspaceRole, &roleID, currentUser)
		} else {
			validRole, errCheck := h.organizationService.IsValidCustomWorkspaceRole(c.Request.Context(), organizationID, roleID, accountID)
			if errCheck != nil {
				response.Fail(c, response.ErrSystemError)
				return
			}
			if !validRole {
				response.Fail(c, response.ErrInvalidRoleType)
				return
			}

			err = h.workspaceManagementService.UpdateMemberCustomRoleWithPermissionCheck(c.Request.Context(), targetWorkspace, member, roleID, currentUser)
		}
	}

	if err != nil {
		switch {
		case isCannotOperateSelfError(err):
			response.Fail(c, response.ErrCannotOperateSelf)
			return
		case isNoPermissionError(err):
			response.Fail(c, response.ErrPermissionDenied)
			return
		case isMemberNotInWorkspaceError(err):
			response.Fail(c, response.ErrMemberNotInWorkspace)
			return
		case isRoleAlreadyAssignedError(err):
			response.Fail(c, response.ErrRoleAlreadyAssigned)
			return
		default:
			response.Fail(c, response.ErrSystemError)
			return
		}
	}

	response.Success(c, gin.H{"result": "success"})
}

func (h *OrganizationHandler) BatchAddOrganizationMembersToWorkspace(c *gin.Context) {
	organizationID := h.getOrganizationID(c)
	workspaceID := c.Param("workspace_id")
	if organizationID == "" || workspaceID == "" {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	var req struct {
		AccountIDs  []string `json:"account_ids" binding:"required"`
		Role        *string  `json:"role,omitempty"`
		RoleID      *string  `json:"role_id,omitempty"`
		Position    string   `json:"position,omitempty"`
		Permissions []string `json:"permissions,omitempty"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	if (req.Role == nil || *req.Role == "") && (req.RoleID == nil || *req.RoleID == "") {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	accountID := c.GetString("account_id")
	if accountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	if h.organizationService != nil {
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
		if !hasPermission {
			response.Fail(c, response.ErrPermissionDenied)
			return
		}
	}

	defaultRole := model.WorkspaceRoleNormal
	if req.Role != nil && *req.Role != "" {
		role := model.WorkspaceMemberRole(*req.Role)
		if !role.IsNonOwnerRole() {
			response.Fail(c, response.ErrInvalidRole)
			return
		}
		defaultRole = role
	}

	operatorAccount, err := h.accountService.GetAccountByID(c.Request.Context(), accountID)
	if err != nil || operatorAccount == nil {
		response.Fail(c, response.ErrAccountNotFound)
		return
	}

	var targetTenant *model.Workspace
	isOrganizationAdmin, err := h.accountService.CheckOrganizationpAdminByWorkspace(c.Request.Context(), accountID, workspaceID)
	if err != nil {
		response.Fail(c, response.ErrSystemError)
		return
	}
	if isOrganizationAdmin {
		targetTenant, err = h.workspaceManagementService.GetWorkspaceByID(c.Request.Context(), workspaceID)
		if err != nil {
			response.Fail(c, response.ErrWorkspaceNotFound)
			return
		}
	} else {
		isAdminOrOwner, err := middleware.CheckAdminOrOwnerRole(c.Request.Context(), h.workspaceManagementService, accountID, workspaceID)
		if err != nil {
			response.Fail(c, response.ErrSystemError)
			return
		}

		if !isAdminOrOwner {
			response.Fail(c, response.ErrPermissionDenied)
			return
		}

		targetTenant, err = h.workspaceManagementService.GetWorkspaceByID(c.Request.Context(), workspaceID)
		if err != nil {
			response.Fail(c, response.ErrWorkspaceNotFound)
			return
		}
	}

	if targetTenant == nil {
		response.Fail(c, response.ErrWorkspaceNotFound)
		return
	}

	var roleIDForAdd *string
	if (req.Role == nil || *req.Role == "") && req.RoleID != nil && *req.RoleID != "" {
		roleID := *req.RoleID

		builtinWorkspaceRole := model.WorkspaceMemberRole("")
		switch roleID {
		case model.WorkspaceBuiltinRoleOwnerID:
			response.Fail(c, response.ErrInvalidRole)
			return
		case model.WorkspaceBuiltinRoleAdminID:
			builtinWorkspaceRole = model.WorkspaceRoleAdmin
		case model.WorkspaceBuiltinRoleMemberID:
			builtinWorkspaceRole = model.WorkspaceRoleNormal
		case model.WorkspaceBuiltinRoleViewerID:
			builtinWorkspaceRole = model.WorkspaceRoleViewer
		}

		if builtinWorkspaceRole != "" {
			defaultRole = builtinWorkspaceRole
			roleIDForAdd = &roleID
		} else {
			validRole, err := h.organizationService.IsValidCustomWorkspaceRole(c.Request.Context(), organizationID, roleID, accountID)
			if err != nil {
				response.Fail(c, response.ErrSystemError)
				return
			}
			if !validRole {
				response.Fail(c, response.ErrInvalidRoleType)
				return
			}
			roleIDForAdd = &roleID
		}
	}

	organizationByOrganization, err := h.organizationService.GetOrganizationByWorkspaceID(c.Request.Context(), workspaceID)
	if err != nil {
		response.Fail(c, response.ErrSystemError)
		return
	}
	if organizationByOrganization == nil || organizationByOrganization.ID != organizationID {
		response.Fail(c, response.ErrWorkspaceNotInOrganization)
		return
	}

	results := make([]map[string]interface{}, 0, len(req.AccountIDs))
	addedCount := 0
	skippedCount := 0
	failedCount := 0

	for _, memberAccountID := range req.AccountIDs {
		result := make(map[string]interface{})
		result["account_id"] = memberAccountID

		targetAccount, err := h.accountService.GetAccountByID(c.Request.Context(), memberAccountID)
		if err != nil || targetAccount == nil {
			result["status"] = "failed"
			result["message"] = "Account not found"
			results = append(results, result)
			failedCount++
			continue
		}

		isOrganizationMember, err := h.organizationService.IsOrganizationMember(c.Request.Context(), organizationID, memberAccountID)
		if err != nil {
			result["status"] = "failed"
			result["message"] = "Failed to check organization membership"
			results = append(results, result)
			failedCount++
			continue
		}
		if !isOrganizationMember {
			result["status"] = "failed"
			result["message"] = "Account is not a member of the organization"
			results = append(results, result)
			failedCount++
			continue
		}

		existingJoin, _ := h.workspaceManagementService.GetUserRole(c.Request.Context(), memberAccountID, workspaceID)
		if existingJoin != nil {
			result["status"] = "skipped"
			result["reason"] = "already_workspace_member"
			result["message"] = "Already in workspace"
			results = append(results, result)
			skippedCount++
			continue
		}

		addReq := &interfaces.AddMemberRequest{
			WorkspaceID: workspaceID,
			AccountID:   memberAccountID,
			Role:        defaultRole,
			RoleID:      roleIDForAdd,
		}

		err = h.workspaceManagementService.AddMember(c.Request.Context(), addReq)
		if err != nil {
			result["status"] = "failed"
			result["message"] = err.Error()
			results = append(results, result)
			failedCount++
			continue
		}

		if req.Position != "" {
			extensionReq := &interfaces.CreateMemberExtensionRequest{
				WorkspaceID: workspaceID,
				AccountID:   memberAccountID,
				Position:    req.Position,
			}

			err = h.workspaceManagementService.CreateMemberExtension(c.Request.Context(), extensionReq)
			if err != nil {
				result["status"] = "success"
				result["extension_warning"] = err.Error()
			} else {
				result["status"] = "success"
			}
		} else {
			result["status"] = "success"
		}

		addedCount++
		results = append(results, result)
	}

	response.Success(c, gin.H{
		"success":            failedCount == 0,
		"result":             "success",
		"added_count":        addedCount,
		"skipped_count":      skippedCount,
		"failed_count":       failedCount,
		"invitation_results": results,
	})
}

// GetDepartmentInviteLink returns current invite link config for a department

// GetInviteLinkInfo returns info for a token (Public)
func (h *OrganizationHandler) GetInviteLinkInfo(c *gin.Context) {
	token := c.Param("token")
	if token == "" {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	link, err := h.organizationService.GetInviteLinkByToken(c.Request.Context(), token)
	if err != nil {
		response.Fail(c, response.ErrSystemError)
		return
	}
	if link == nil {
		response.Fail(c, response.ErrNotFound)
		return
	}

	if link.Status != "active" || (link.ExpiresAt != nil && link.ExpiresAt.Before(time.Now())) {
		response.FailWithMessage(c, response.ErrInvalidParam, "Invite link expired or invalid")
		return
	}

	organization, err := h.organizationService.GetOrganizationByID(c.Request.Context(), link.OrganizationID)
	if err != nil {
		response.Fail(c, response.ErrSystemError)
		return
	}

	var deptName string
	var deptID string
	if link.DepartmentID != nil {
		dept, err := h.departmentService.GetDepartment(c.Request.Context(), *link.DepartmentID)
		if err == nil && dept != nil {
			deptName = dept.Name
			deptID = dept.ID
		}
	}

	response.Success(c, gin.H{
		"valid":            true,
		"group":            gin.H{"id": organization.ID, "name": organization.Name},
		"department":       gin.H{"id": deptID, "name": deptName},
		"require_approval": link.RequireApproval,
		"expires_at":       link.ExpiresAt,
	})
}

// AcceptInviteLink accepts an invite (Authenticated)
func (h *OrganizationHandler) AcceptInviteLink(c *gin.Context) {
	token := c.Param("token")
	if token == "" {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	accountID := c.GetString("account_id")
	if accountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	var reqBody struct {
		Name *string `json:"name"`
	}
	_ = c.ShouldBindJSON(&reqBody)

	// Retrieve invite link info first to get organization ID.
	link, err := h.organizationService.GetInviteLinkByToken(c.Request.Context(), token)
	if err != nil {
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}
	if link == nil {
		response.FailWithMessage(c, response.ErrInvalidParam, "invalid invite token")
		return
	}

	// Check if already member
	isMember, err := h.organizationService.IsOrganizationMember(c.Request.Context(), link.OrganizationID, accountID)
	if err != nil {
		response.Fail(c, response.ErrSystemError)
		return
	}
	if isMember {
		response.Fail(c, response.ErrMemberAlreadyInOrganization)
		return
	}

	// Check for pending join request if approval required
	if link.RequireApproval {
		pendingReq, err := h.organizationService.GetPendingJoinRequest(c.Request.Context(), link.OrganizationID, accountID)
		if err != nil {
			response.Fail(c, response.ErrSystemError)
			return
		}
		if pendingReq != nil {
			response.Fail(c, response.ErrJoinRequestPending)
			return
		}
	}

	if reqBody.Name != nil && *reqBody.Name != "" {
		// Case 1: Name provided by user - Strict uniqueness check
		exists, err := h.organizationService.ExistsMemberByName(c.Request.Context(), link.OrganizationID, *reqBody.Name, accountID)
		if err != nil {
			response.Fail(c, response.ErrSystemError)
			return
		}
		if exists {
			response.FailWithMessage(c, response.ErrInvalidParam, "member name already exists")
			return
		}
	} else {
		// Case 2: No name provided - Use default account name and auto-resolve conflict
		account, err := h.accountService.GetAccountByID(c.Request.Context(), accountID)
		if err != nil || account == nil {
			response.Fail(c, response.ErrSystemError)
			return
		}

		baseName := account.Name
		finalName := baseName

		exists, err := h.organizationService.ExistsMemberByName(c.Request.Context(), link.OrganizationID, finalName, accountID)
		if err != nil {
			response.Fail(c, response.ErrSystemError)
			return
		}

		if exists {
			// Conflict detected: Append random suffix
			finalName = fmt.Sprintf("%s_%s", baseName, helper.GenerateRandomNumberString(4))

			// Double check with new name
			exists2, _ := h.organizationService.ExistsMemberByName(c.Request.Context(), link.OrganizationID, finalName, accountID)
			if exists2 {
				// Retry with longer suffix if still conflicts
				finalName = fmt.Sprintf("%s_%s", baseName, helper.GenerateRandomNumberString(6))
			}
		}
		reqBody.Name = &finalName
	}

	req, err := h.organizationService.AcceptInviteByToken(c.Request.Context(), token, accountID, reqBody.Name)
	if err != nil {
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}

	if req.Status == model.OrganizationJoinRequestStatusApproved {
		// Add to organization
		isMember, _ := h.organizationService.IsOrganizationMember(c.Request.Context(), req.OrganizationID, req.AccountID)
		if !isMember {
			role := model.OrganizationRoleNormal
			if req.DefaultOrganizationRole != "" {
				role = model.OrganizationRole(req.DefaultOrganizationRole)
			}
			addReq := &shared_dto.AddOrganizationMemberRequest{
				OrganizationID: req.OrganizationID,
				AccountID:      req.AccountID,
				Role:           role,
				Name:           reqBody.Name,
			}
			_ = h.organizationService.AddMember(c.Request.Context(), addReq)
		}

		// Add to department
		if req.DepartmentID != nil {
			_, _ = h.departmentService.AddMemberToDepartment(c.Request.Context(), req.OrganizationID, *req.DepartmentID, req.AccountID)
		}
	}

	response.Success(c, gin.H{"status": req.Status})
}

func (h *OrganizationHandler) TransferWorkspaceOwner(c *gin.Context) {
	organizationID := h.getOrganizationID(c)
	workspaceID := c.Param("workspace_id")
	if organizationID == "" || workspaceID == "" {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	accountID := middleware.GetAccountID(c)

	var req struct {
		NewOwnerID string `json:"new_owner_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	// Verify workspace belongs to organization (optional but recommended in this context).
	workspaceInOrganization, err := h.organizationService.GetOrganizationByWorkspaceID(c.Request.Context(), workspaceID)
	if err != nil {
		response.Fail(c, response.ErrSystemError)
		return
	}
	if workspaceInOrganization == nil || workspaceInOrganization.ID != organizationID {
		response.Fail(c, response.ErrWorkspaceNotInOrganization)
		return
	}

	if err := h.workspaceManagementService.TransferOwner(c.Request.Context(), workspaceID, accountID, req.NewOwnerID); err != nil {
		switch {
		case isNoPermissionError(err):
			response.Fail(c, response.ErrPermissionDenied)
			return
		case isMemberNotInWorkspaceError(err):
			response.Fail(c, response.ErrMemberNotInWorkspace)
			return
		default:
			response.Fail(c, response.ErrSystemError)
			return
		}
	}

	response.Success(c, gin.H{"result": "success"})
}

func (h *OrganizationHandler) CheckMemberNameExists(c *gin.Context) {
	organizationID := h.getOrganizationID(c)
	if organizationID == "" {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	name := c.Query("name")
	if name == "" {
		response.FailWithMessage(c, response.ErrInvalidParam, "name is required")
		return
	}

	accountID := middleware.GetAccountID(c)

	exists, err := h.organizationService.ExistsMemberByName(c.Request.Context(), organizationID, name, accountID)
	if err != nil {
		response.Fail(c, response.ErrSystemError)
		return
	}

	response.Success(c, gin.H{
		"exists": exists,
	})
}

func (h *OrganizationHandler) RegisterRoutes(router *gin.RouterGroup) {
	public := router.Group("/public")
	{
		public.GET("/invites/:token", h.GetInviteLinkInfo)
		public.POST("/invites/:token/accept", middleware.JWTWithOrganizationAndService(h.accountService), h.AcceptInviteLink)
	}

	organization := router.Group("/organizations", middleware.JWTWithOrganizationAndService(h.accountService))
	{
		organization.GET("/", h.GetOrganizations)
		organization.POST("/", h.CreateOrganization)
		organization.PUT("/", h.UpdateOrganization)
		organization.DELETE("/", h.DeleteOrganization)

		organization.GET("/current/members", h.GetCurrentOrganizationMembers)
		organization.POST("/current/members/invite", h.InviteCurrentOrganizationMember)
		organization.POST("/current/members/reset-password", h.ResetCurrentOrganizationMemberPassword)
		organization.PATCH("/current/members/:member_id/organization-role", h.UpdateCurrentOrganizationMemberRole)
		organization.GET("/info/:organization_id", h.GetOrganizationDetails)
		organization.PATCH("/info/:organization_id", h.PatchOrganization)
		organization.GET("/:organization_id/workspaces", h.GetOrganizationWorkspaces)
		organization.GET("/:organization_id/workspaces/:workspace_id", h.GetOrganizationWorkspaceDetail)
		organization.GET("/:organization_id/workspaces/:workspace_id/available-members", h.GetOrganizationWorkspaceAvailableMembers)
		organization.GET("/:organization_id/workspaces/:workspace_id/members", h.GetOrganizationWorkspaceMembers)
		organization.GET("/:organization_id/workspaces/:workspace_id/members/:member_id", h.GetOrganizationWorkspaceMemberDetailByID)
		organization.DELETE("/:organization_id/workspaces/:workspace_id/members/:member_id", h.RemoveOrganizationWorkspaceMember)
		organization.POST("/:organization_id/workspaces/:workspace_id/leave", h.LeaveOrganizationWorkspace)
		organization.PUT("/:organization_id/workspaces/:workspace_id/members/:member_id/update-role", h.UpdateOrganizationWorkspaceMemberRole)
		organization.POST("/:organization_id/workspaces/:workspace_id/members/batch-add", h.BatchAddOrganizationMembersToWorkspace)
		organization.GET("/:organization_id/workspaces/:workspace_id/accounts/:account_id/permissions", h.GetWorkspaceMemberPermissions)
		organization.POST("/:organization_id/workspaces", h.CreateOrganizationWorkspace)
		organization.PUT("/:organization_id/workspaces/:workspace_id", h.UpdateOrganizationWorkspace)
		organization.DELETE("/:organization_id/workspaces/:workspace_id", h.DeleteOrganizationWorkspace)
		organization.POST("/:organization_id/workspaces/:workspace_id/transfer-ownership", h.TransferWorkspaceOwner)

		organization.GET("/:organization_id/unjoined-workspaces/:account_id", h.GetUnjoinedWorkspaces)
		organization.GET("/:organization_id/joined-workspaces/:account_id", h.GetJoinedWorkspaces)
		organization.GET("/:organization_id/joined-workspaces-roles/:account_id", h.GetJoinedWorkspacesRoles)
		organization.GET("/joined-groups", h.GetJoinedOrganizations)
		organization.GET("/joined-groups/:account_id", h.GetJoinedOrganizations)
		organization.GET("/:organization_id/check-manage-permission", h.CheckManagePermission)
		organization.GET("/:organization_id/invite-link", h.GetDepartmentInviteLink)
		organization.POST("/:organization_id/invite-link", h.CreateOrResetDepartmentInviteLink)
		organization.PUT("/:organization_id/invite-link/status", h.UpdateDepartmentInviteLinkStatus)
		organization.GET("/:organization_id/join-requests", h.ListOrganizationJoinRequests)
		organization.POST("/:organization_id/join-requests/approve-batch", h.BatchApproveDepartmentJoinRequests)
		organization.POST("/:organization_id/join-requests/reject-batch", h.BatchRejectDepartmentJoinRequests)
		organization.POST("/:organization_id/join-requests/:id/approve", h.ApproveDepartmentJoinRequest)
		organization.POST("/:organization_id/join-requests/:id/reject", h.RejectDepartmentJoinRequest)
		organization.GET("/:organization_id/members", h.GetOrganizationMembers)
		organization.GET("/:organization_id/members/:member_id", h.GetOrganizationMemberDetailByID)
		organization.GET("/:organization_id/workspaces/:workspace_id/assets", h.GetOrganizationWorkspaceAssets)
		organization.POST("/:organization_id/members/direct-add", h.DirectAddMember)
		organization.POST("/:organization_id/transfer-ownership", h.TransferOrganizationOwnership)
		organization.GET("/:organization_id/check-member-name", h.CheckMemberNameExists)
		organization.PUT("/:organization_id/members/:member_id", h.UpdateOrganizationMember)
		organization.PUT("/:organization_id/members/:member_id/status", h.UpdateMemberStatus)
		organization.DELETE("/:organization_id/members/:member_id", h.RemoveOrganizationMember)
		organization.POST("/:organization_id/leave", h.LeaveOrganization)

		organization.GET("/current", h.GetCurrentOrganization)
		organization.GET("/current-detail", h.GetCurrentOrganizationDetail)

		organization.GET("/:organization_id/managed-workspaces", h.GetManagedWorkspaces)
		organization.GET("/:organization_id/managed-app-workspaces", h.GetManagedAppWorkspaces)
		organization.GET("/:organization_id/managed-dataset-workspaces", h.GetManagedDatasetWorkspaces)
		organization.GET("/:organization_id/datasets", h.GetOrganizationDatasets)

		organization.GET("/:organization_id/check-app-permission", h.CheckAppPermission)
		organization.GET("/:organization_id/check-dataset-permission", h.CheckDatasetPermission)

		// Group roles & permissions
		organization.GET("/:organization_id/permissions", h.ListWorkspacePermissions)
		organization.GET("/:organization_id/roles", h.ListWorkspaceRoles)
		organization.GET("/:organization_id/roles/:role_id", h.GetWorkspaceRole)
		organization.GET("/:organization_id/roles/:role_id/members", h.ListWorkspaceRoleMembers)
		organization.POST("/:organization_id/roles", h.CreateWorkspaceRole)
		organization.PATCH("/:organization_id/roles/:role_id", h.UpdateWorkspaceRole)
		organization.PUT("/:organization_id/roles/:role_id/permissions", h.UpdateWorkspaceRolePermissions)
		organization.DELETE("/:organization_id/roles/:role_id", h.DeleteWorkspaceRole)
		organization.GET("/:organization_id/accounts/:account_id/permissions", h.GetMemberPermissions)
	}

}
