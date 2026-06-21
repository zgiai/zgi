package handler

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/zgiai/zgi/api/middleware"
	accountMiddleware "github.com/zgiai/zgi/api/middleware"

	"errors"

	"github.com/gin-gonic/gin"
	usererrors "github.com/zgiai/zgi/api/internal/errors"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	auth_model "github.com/zgiai/zgi/api/internal/modules/user/auth/model"
	"github.com/zgiai/zgi/api/internal/modules/workspace/model"
	"github.com/zgiai/zgi/api/pkg/response"
)

type MembersHandler struct {
	workspaceManagementService interfaces.WorkspaceManagementService
	accountService             interfaces.AccountService
	//registerService *auth.RegisterService
	enterpriseService interfaces.OrganizationService
	consoleWebURL     string
}

func NewMembersHandler(workspaceManagementService interfaces.WorkspaceManagementService, accountService interfaces.AccountService, enterpriseService interfaces.OrganizationService, consoleWebURL string) *MembersHandler {
	return &MembersHandler{
		workspaceManagementService: workspaceManagementService,
		accountService:             accountService,
		//registerService: registerService,
		enterpriseService: enterpriseService,
		consoleWebURL:     consoleWebURL,
	}
}

func (h *MembersHandler) requireWorkspacePermission(
	c *gin.Context,
	workspaceID string,
	permissionCode model.WorkspacePermissionCode,
) bool {
	accountID := c.GetString("account_id")
	if accountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return false
	}
	if h.enterpriseService == nil {
		response.Fail(c, response.ErrSystemError)
		return false
	}

	organization, err := h.enterpriseService.GetOrganizationByWorkspaceID(c.Request.Context(), workspaceID)
	if err != nil || organization == nil {
		response.Fail(c, response.ErrWorkspaceNotFound)
		return false
	}

	hasPermission, err := h.enterpriseService.CheckWorkspacePermission(
		c.Request.Context(),
		organization.ID,
		workspaceID,
		accountID,
		permissionCode,
	)
	if err != nil {
		response.Fail(c, response.ErrSystemError)
		return false
	}
	if !hasPermission {
		response.Fail(c, response.ErrPermissionDenied)
		return false
	}

	return true
}

func (h *MembersHandler) GetCurrentOrganizationMemberDetail(c *gin.Context) {
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

	currentWorkspaceJoin, err := h.workspaceManagementService.GetCurrentWorkspace(c.Request.Context(), accountID)
	if err != nil {
		response.Fail(c, response.ErrWorkspaceNotFound)
		return
	}
	if currentWorkspaceJoin == nil {
		response.Fail(c, response.ErrWorkspaceNotFound)
		return
	}

	member, err := h.workspaceManagementService.GetWorkspaceMemberWithExtensionsById(c.Request.Context(), currentWorkspaceJoin.WorkspaceID, memberID)
	if err != nil {
		response.Fail(c, response.ErrSystemError)
		return
	}

	if member == nil {
		response.Fail(c, response.ErrMemberNotFound)
		return
	}

	organizationRole, err := h.accountService.GetOrganizationRoleByWorkspaceID(c.Request.Context(), memberID, currentWorkspaceJoin.WorkspaceID)
	if err != nil {
		organizationRole = "normal"
	}
	member.OrganizationRole = organizationRole

	response.Success(c, member)
}

func (h *MembersHandler) GetCurrentOrganizationMembers(c *gin.Context) {
	accountID := c.GetString("account_id")
	if accountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	currentWorkspaceJoin, err := h.workspaceManagementService.GetCurrentWorkspace(c.Request.Context(), accountID)
	if err != nil {
		response.Fail(c, response.ErrWorkspaceNotFound)
		return
	}
	if currentWorkspaceJoin == nil {
		response.Fail(c, response.ErrWorkspaceNotFound)
		return
	}

	if !h.requireWorkspacePermission(c, currentWorkspaceJoin.WorkspaceID, model.WorkspacePermissionWorkspaceView) {
		return
	}

	keyword := c.Query("keyword")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "1000"))

	members, total, err := h.workspaceManagementService.GetWorkspaceMembersPaginated(c.Request.Context(), currentWorkspaceJoin.WorkspaceID, page, limit, keyword, "")
	if err != nil {
		response.Fail(c, response.ErrSystemError)
		return
	}

	response.Success(c, gin.H{
		"result":   "success",
		"accounts": members,
		"total":    total,
		"page":     page,
		"limit":    limit,
	})
}

func (h *MembersHandler) InviteMemberByEmail(c *gin.Context) {
	var req struct {
		Emails   []string `json:"emails" binding:"required"`
		Role     string   `json:"role" binding:"required"`
		Language string   `json:"language,omitempty"`
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

	role := model.WorkspaceMemberRole(req.Role)
	if !role.IsNonOwnerRole() {
		response.Fail(c, response.ErrInvalidRoleType)
		return
	}

	currentWorkspaceJoin, err := h.workspaceManagementService.GetCurrentWorkspace(c.Request.Context(), accountID)
	if err != nil || currentWorkspaceJoin == nil {
		response.Fail(c, response.ErrWorkspaceNotFound)
		return
	}

	invitationResults := make([]map[string]interface{}, 0)

	for _, email := range req.Emails {
		token, err := h.accountService.InviteMember(
			c.Request.Context(),
			currentWorkspaceJoin.WorkspaceID,
			accountID,
			email,
			role,
			req.Language,
		)

		if err != nil {
			if err == usererrors.ErrAccountAlreadyInWorkspace {
				invitationResults = append(invitationResults, map[string]interface{}{
					"status": "success",
					"email":  email,
					"url":    h.consoleWebURL + "/login",
				})
				break
			} else {
				invitationResults = append(invitationResults, map[string]interface{}{
					"status":  "failed",
					"email":   email,
					"message": err.Error(),
				})
			}
		} else {
			// encodedEmail := url.QueryEscape(email)
			encodedEmail := email
			invitationResults = append(invitationResults, map[string]interface{}{
				"status": "success",
				"email":  email,
				"url":    h.consoleWebURL + "/activate?email=" + encodedEmail + "&token=" + token,
			})
		}
	}

	response.Success(c, gin.H{
		"result":             "success",
		"invitation_results": invitationResults,
	})
}

func (h *MembersHandler) CancelMemberInvite(c *gin.Context) {
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

	member, err := h.accountService.GetAccountByID(c.Request.Context(), memberID)
	if err != nil || member == nil {
		response.Fail(c, response.ErrAccountNotFound)
		return
	}

	operator, err := h.accountService.GetAccountByID(c.Request.Context(), accountID)
	if err != nil || operator == nil {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	currentWorkspaceJoin, err := h.workspaceManagementService.GetCurrentWorkspace(c.Request.Context(), accountID)
	if err != nil || currentWorkspaceJoin == nil {
		response.Fail(c, response.ErrWorkspaceNotFound)
		return
	}

	currentWorkspace, err := h.workspaceManagementService.GetWorkspaceByID(c.Request.Context(), currentWorkspaceJoin.WorkspaceID)
	if err != nil || currentWorkspace == nil {
		response.Fail(c, response.ErrWorkspaceNotFound)
		return
	}

	err = h.workspaceManagementService.RemoveMemberFromWorkspace(c.Request.Context(), currentWorkspace, member, operator)
	if err != nil {
		switch err.(type) {
		case *usererrors.CannotOperateSelfError:
			response.Fail(c, response.ErrCannotOperateSelf)
			return
		case *usererrors.NoPermissionError:
			response.Fail(c, response.ErrPermissionDenied)
			return
		case *usererrors.MemberNotInWorkspaceError:
			response.Fail(c, response.ErrMemberNotInWorkspace)
			return
		default:
			response.Fail(c, response.ErrSystemError)
			return
		}
	}

	response.Success(c, gin.H{"result": "success"})
}

func (h *MembersHandler) UpdateMemberRole(c *gin.Context) {
	memberID := c.Param("member_id")
	if memberID == "" {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	var req struct {
		Role string `json:"role" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	role := model.WorkspaceMemberRole(req.Role)
	if !role.IsValidRole() {
		response.Fail(c, response.ErrInvalidRoleType)
		return
	}

	accountID := c.GetString("account_id")
	if accountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	workspaces, err := h.workspaceManagementService.GetAccountWorkspaces(c.Request.Context(), accountID)
	if err != nil || len(workspaces) == 0 {
		response.Fail(c, response.ErrWorkspaceNotFound)
		return
	}

	currentWorkspace := workspaces[0]

	updateReq := &interfaces.UpdateMemberRoleRequest{
		WorkspaceID: currentWorkspace.ID,
		AccountID:   memberID,
		Role:        role,
	}
	err = h.workspaceManagementService.UpdateMemberRole(c.Request.Context(), updateReq)
	if err != nil {
		response.Fail(c, response.ErrSystemError)
		return
	}

	response.Success(c, gin.H{"result": "success"})
}

func (h *MembersHandler) GetDatasetOperatorMembers(c *gin.Context) {
	accountID := c.GetString("account_id")
	if accountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	currentWorkspaceJoin, err := h.workspaceManagementService.GetCurrentWorkspace(c.Request.Context(), accountID)
	if err != nil || currentWorkspaceJoin == nil {
		response.Fail(c, response.ErrWorkspaceNotFound)
		return
	}

	members, err := h.workspaceManagementService.GetDatasetOperatorMembers(c.Request.Context(), currentWorkspaceJoin.WorkspaceID)
	if err != nil {
		response.Fail(c, response.ErrSystemError)
		return
	}

	response.Success(c, gin.H{
		"result":   "success",
		"accounts": members,
	})
}

func (h *MembersHandler) GetWorkspaceMembers(c *gin.Context) {
	workspaceID := c.Param("workspace_id")
	if workspaceID == "" || workspaceID == "default" || workspaceID == ":workspace_id" {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	accountID := c.GetString("account_id")
	if accountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	accountUser, err := h.accountService.GetAccountByID(c.Request.Context(), accountID)
	if err != nil || accountUser == nil {
		response.Fail(c, response.ErrAccountNotFound)
		return
	}

	var targetWorkspace *model.Workspace

	isGroupAdmin, err := h.accountService.CheckOrganizationpAdminByWorkspace(c.Request.Context(), accountID, workspaceID)
	if err != nil {
		response.Fail(c, response.ErrSystemError)
		return
	}

	if isGroupAdmin {
		targetWorkspace, err = h.workspaceManagementService.GetWorkspaceByID(c.Request.Context(), workspaceID)
		if err != nil {
			response.Fail(c, response.ErrSystemError)
			return
		}
	} else {
		targetWorkspace, err = h.accountService.GetCurrentWorkspace(c.Request.Context(), accountID)
		if err != nil {
			response.Fail(c, response.ErrSystemError)
			return
		}
	}

	if targetWorkspace == nil {
		response.Fail(c, response.ErrWorkspaceNotFound)
		return
	}

	keyword := c.Query("keyword")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "1000"))

	members, total, err := h.workspaceManagementService.GetWorkspaceMembersPaginated(c.Request.Context(), targetWorkspace.ID, page, limit, keyword, "")
	if err != nil {
		response.Fail(c, response.ErrSystemError)
		return
	}

	response.Success(c, gin.H{
		"result":   "success",
		"accounts": members,
		"total":    total,
		"page":     page,
		"limit":    limit,
	})
}

type MemberExtensionFlat struct {
	ID               string      `json:"id"`
	Name             string      `json:"name"`
	Avatar           *string     `json:"avatar"`
	AvatarURL        *string     `json:"avatar_url"`
	Email            string      `json:"email"`
	Mobile           string      `json:"mobile"`
	LastLoginAt      *int64      `json:"last_login_at"`
	LastActiveAt     *int64      `json:"last_active_at"`
	CreatedAt        int64       `json:"created_at"`
	Role             string      `json:"role"`
	Status           string      `json:"status"`
	OrganizationRole string      `json:"organization_role"`
	AccountRole      interface{} `json:"account_role"`
	Extension        interface{} `json:"extension"`
	Position         *string     `json:"position"`
	Permissions      []string    `json:"permissions"`
}

func flattenMemberExtension(m *interfaces.WorkspaceMemberWithExtensionResponse) *MemberExtensionFlat {
	acc := m.Account
	var avatar, avatarURL *string
	if acc.Avatar != nil {
		avatar = acc.Avatar
		avatarURL = acc.Avatar
	}
	var lastLoginAt, lastActiveAt *int64
	if acc.LastLoginAt != nil {
		ts := acc.LastLoginAt.Unix()
		lastLoginAt = &ts
	}
	if acc.LastActiveAt != nil {
		ts := acc.LastActiveAt.Unix()
		lastActiveAt = &ts
	}
	var extension interface{}
	// Construct extension map from flattened fields
	extMap := map[string]interface{}{
		"position": m.Position,
	}
	extension = extMap

	var accountRole interface{} = nil
	var position *string
	if m.Position != "" {
		position = &m.Position
	} else {
		position = nil
	}
	permissions := m.Permissions
	if permissions == nil {
		permissions = []string{}
	}
	return &MemberExtensionFlat{
		ID:               acc.ID,
		Name:             acc.Name,
		Avatar:           avatar,
		AvatarURL:        avatarURL,
		Email:            acc.Email,
		LastLoginAt:      lastLoginAt,
		LastActiveAt:     lastActiveAt,
		CreatedAt:        acc.CreatedAt.Unix(),
		Role:             string(m.Role),
		Status:           string(acc.Status),
		OrganizationRole: m.OrganizationRole,
		AccountRole:      accountRole,
		Extension:        extension,
		Position:         position,
		Permissions:      permissions,
	}
}

func (h *MembersHandler) GetWorkspaceMembersExtension(c *gin.Context) {
	workspaceID := c.Param("workspace_id")
	if workspaceID == "" || workspaceID == "default" || workspaceID == ":workspace_id" {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	if !h.requireWorkspacePermission(c, workspaceID, model.WorkspacePermissionWorkspaceView) {
		return
	}

	members, err := h.workspaceManagementService.GetWorkspaceMembersWithExtensions(c.Request.Context(), workspaceID)
	if err != nil {
		response.Fail(c, response.ErrSystemError)
		return
	}

	mobiles := make([]string, len(members))
	for i, m := range members {
		accountExtension, err := h.accountService.GetAccountExtensionByID(c.Request.Context(), m.Account.ID)
		if err == nil {
			if val, ok := accountExtension["mobile"].(string); ok {
				mobiles[i] = val
			} else {
				mobiles[i] = ""
			}
		} else if err.Error() == "record not found" {
			mobiles[i] = ""
			continue
		} else {
			response.Fail(c, response.ErrSystemError)
			return
		}
	}

	var result []MemberExtensionFlat
	for _, m := range members {
		result = append(result, *flattenMemberExtension(m))
	}

	for i, _ := range result {
		result[i].Mobile = mobiles[i]
	}

	response.Success(c, result)
}

func (h *MembersHandler) GetWorkspaceMemberExtensionDetail(c *gin.Context) {
	workspaceID := c.Param("workspace_id")
	memberID := c.Param("member_id")
	if workspaceID == "" || workspaceID == "default" || workspaceID == ":workspace_id" || memberID == "" {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	accountID := c.GetString("account_id")
	if accountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	_, err := h.accountService.GetAccountByID(c.Request.Context(), accountID)
	if err != nil {
		response.Fail(c, response.ErrAccountNotFound)
		return
	}

	var targetWorkspace *model.Workspace

	isGroupAdmin, err := h.accountService.CheckOrganizationpAdminByWorkspace(c.Request.Context(), accountID, workspaceID)
	if err != nil {
		response.Fail(c, response.ErrSystemError)
		return
	}

	if isGroupAdmin {
		targetWorkspace, err = h.workspaceManagementService.GetWorkspaceByID(c.Request.Context(), workspaceID)
		if err != nil {
			response.Fail(c, response.ErrSystemError)
			return
		}
	} else {
		targetWorkspace, err = h.accountService.GetCurrentWorkspace(c.Request.Context(), accountID)
		if err != nil {
			response.Fail(c, response.ErrSystemError)
			return
		}
	}

	if targetWorkspace == nil {
		response.Fail(c, response.ErrWorkspaceNotFound)
		return
	}

	member, err := h.workspaceManagementService.GetWorkspaceMemberWithExtensionsById(c.Request.Context(), targetWorkspace.ID, memberID)
	if err != nil {
		response.Fail(c, response.ErrSystemError)
		return
	}

	if member == nil {
		response.Fail(c, response.ErrMemberNotFound)
		return
	}

	groupRole, err := h.accountService.GetOrganizationRoleByWorkspaceID(c.Request.Context(), memberID, targetWorkspace.ID)
	if err != nil {
		groupRole = "normal"
	}
	member.OrganizationRole = groupRole

	response.Success(c, member)
}

func (h *MembersHandler) UpdateWorkspaceMemberExtensionDetail(c *gin.Context) {
	workspaceID := c.Param("workspace_id")
	memberID := c.Param("member_id")
	if workspaceID == "" || workspaceID == "default" || workspaceID == ":workspace_id" || memberID == "" {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	var req struct {
		Name           *string `json:"name,omitempty"`
		Email          *string `json:"email,omitempty"`
		Status         *string `json:"status,omitempty"`
		Mobile         *string `json:"mobile,omitempty"`
		Gender         *string `json:"gender,omitempty"`
		NewWorkspaceID *string `json:"new_workspace_id,omitempty"`
		// Legacy compatibility alias for existing clients; prefer new_workspace_id.
		LegacyNewTenantID *string  `json:"new_tenant_id,omitempty"`
		Role              *string  `json:"role,omitempty"`
		Position          *string  `json:"position,omitempty"`
		Permissions       []string `json:"permissions,omitempty"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	// Get current user
	accountID := c.GetString("account_id")
	if accountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	accountUser, err := h.accountService.GetAccountByID(c.Request.Context(), accountID)
	if err != nil {
		response.Fail(c, response.ErrAccountNotFound)
		return
	}

	var targetWorkspace *model.Workspace
	isGroupAdmin, _ := h.accountService.CheckOrganizationpAdminByWorkspace(c.Request.Context(), accountUser.ID, workspaceID)

	if isGroupAdmin {
		targetWorkspace, err = h.workspaceManagementService.GetWorkspaceByID(c.Request.Context(), workspaceID)
		if err != nil {
			response.Fail(c, response.ErrWorkspaceNotFound)
			return
		}
	} else {
		accountUser.CurrentTenantID = workspaceID
		targetWorkspace, err = h.accountService.GetCurrentWorkspace(c.Request.Context(), accountUser.ID)
		if err != nil || targetWorkspace == nil {
			response.Fail(c, response.ErrWorkspaceNotFound)
			return
		}
	}

	member, err := h.workspaceManagementService.GetWorkspaceMemberWithExtensionsById(c.Request.Context(), targetWorkspace.ID, memberID)
	if err != nil {
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
	if member == nil {
		response.Fail(c, response.ErrMemberNotFound)
		return
	}

	if req.Permissions != nil {
		for _, perm := range req.Permissions {
			_ = perm
		}
	}

	if req.Name != nil || req.Email != nil || req.Status != nil {
		if req.Email != nil {
			existingUser, err := h.accountService.GetUserThroughEmail(c.Request.Context(), *req.Email)
			if err == nil && existingUser != nil && existingUser.ID != member.Account.ID {
				response.Fail(c, response.ErrEmailExists)
				return
			}
		}

		// Validate status value
		if req.Status != nil {
			validStatuses := []string{"active", "banned", "closed"}
			isValid := false
			for _, status := range validStatuses {
				if *req.Status == status {
					isValid = true
					break
				}
			}
			if !isValid {
				response.Fail(c, response.ErrInvalidStatus)
				return
			}
		}

		err = h.accountService.UpdateAccountBasicInfo(c.Request.Context(), member.Account, req.Name, req.Email, req.Status)
		if err != nil {
			response.Fail(c, response.ErrAccountUpdate)
			return
		}
	}

	if req.Mobile != nil || req.Gender != nil {
		err = h.accountService.UpdateAccountExtension(c.Request.Context(), member.Account, req.Mobile, req.Gender)
		if err != nil {
			response.Fail(c, response.ErrAccountUpdate)
			return
		}
	}

	newWorkspaceID := req.NewWorkspaceID
	if newWorkspaceID == nil {
		newWorkspaceID = req.LegacyNewTenantID
	}

	if newWorkspaceID != nil && *newWorkspaceID != workspaceID {
		newWorkspace, err := h.workspaceManagementService.GetWorkspaceByID(c.Request.Context(), *newWorkspaceID)
		if err != nil {
			response.Fail(c, response.ErrWorkspaceNotFound)
			return
		}
		if newWorkspace == nil {
			response.Fail(c, response.ErrWorkspaceNotFound)
			return
		}

		err = h.workspaceManagementService.ChangeWorkspaceWithJoin(c.Request.Context(), member.Account, workspaceID, newWorkspace.ID, accountUser)
		if err != nil {
			switch {
			case isNoPermissionError(err):
				response.Fail(c, response.ErrPermissionDenied)
				return
			case isMemberNotInWorkspaceError(err):
				response.Fail(c, response.ErrMemberNotInWorkspace)
				return
			case isCannotOperateSelfError(err):
				response.Fail(c, response.ErrCannotOperateSelf)
				return
			default:
				response.Fail(c, response.ErrSystemError)
				return
			}
		}
		targetWorkspace = newWorkspace
	}

	if req.Role != nil || req.Position != nil || len(req.Permissions) > 0 {
		err = h.workspaceManagementService.UpdateMemberRoleExtensions(
			c.Request.Context(),
			targetWorkspace,
			member.Account,
			req.Role,
			req.Position,
			req.Permissions,
			accountUser,
		)
		if err != nil {
			response.Fail(c, response.ErrSystemError)
			return
		}
	}

	updatedMember, err := h.workspaceManagementService.GetWorkspaceMemberWithExtensionsById(c.Request.Context(), targetWorkspace.ID, memberID)
	if err != nil {
		response.Fail(c, response.ErrSystemError)
		return
	}

	groupRole, err := h.accountService.GetOrganizationRoleByWorkspaceID(c.Request.Context(), updatedMember.Account.ID, targetWorkspace.ID)
	if err == nil {
		updatedMember.OrganizationRole = groupRole
	}

	response.Success(c, updatedMember)
}

func (h *MembersHandler) InviteWorkspaceMemberByEmail(c *gin.Context) {
	workspaceID := c.Param("workspace_id")
	if workspaceID == "" || workspaceID == "default" || workspaceID == ":workspace_id" {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	var req struct {
		Emails   []string `json:"emails" binding:"required"`
		Role     string   `json:"role" binding:"required"`
		Language string   `json:"language,omitempty"`
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

	inviteeRole := req.Role
	role := model.WorkspaceMemberRole(inviteeRole)
	if !role.IsNonOwnerRole() {
		response.Fail(c, response.ErrInvalidRole)
		return
	}

	inviter, err := h.accountService.GetAccountByID(c.Request.Context(), accountID)
	if err != nil || inviter == nil {
		response.Fail(c, response.ErrAccountNotFound)
		return
	}

	var targetWorkspace *model.Workspace

	isGroupAdmin, err := h.accountService.CheckOrganizationpAdminByWorkspace(c.Request.Context(), accountID, workspaceID)
	if err != nil {
		response.Fail(c, response.ErrSystemError)
		return
	}

	if isGroupAdmin {
		targetWorkspace, err = h.workspaceManagementService.GetWorkspaceByID(c.Request.Context(), workspaceID)
		if err != nil {
			response.Fail(c, response.ErrSystemError)
			return
		}
	} else {
		targetWorkspace, err = h.accountService.GetCurrentWorkspace(c.Request.Context(), accountID)
		if err != nil {
			response.Fail(c, response.ErrSystemError)
			return
		}
	}

	if targetWorkspace == nil {
		response.Fail(c, response.ErrWorkspaceNotFound)
		return
	}

	invitationResults := make([]map[string]interface{}, 0)

	for _, inviteeEmail := range req.Emails {
		token, err := h.accountService.InviteMember(
			c.Request.Context(),
			targetWorkspace.ID,
			accountID,
			inviteeEmail,
			role,
			req.Language,
		)

		if err != nil {
			if err == usererrors.ErrAccountAlreadyInWorkspace {
				invitationResults = append(invitationResults, map[string]interface{}{
					"status": "success",
					"email":  inviteeEmail,
					"url":    h.consoleWebURL + "/login",
				})
				break
			} else {
				invitationResults = append(invitationResults, map[string]interface{}{
					"status":  "failed",
					"email":   inviteeEmail,
					"message": err.Error(),
				})
			}
		} else {
			// encodedInviteeEmail := url.QueryEscape(inviteeEmail)
			encodedInviteeEmail := inviteeEmail
			activationURL := fmt.Sprintf("%s/activate?email=%s&token=%s", h.consoleWebURL, encodedInviteeEmail, token)

			invitationResults = append(invitationResults, map[string]interface{}{
				"status": "success",
				"email":  inviteeEmail,
				"url":    activationURL,
			})
		}
	}

	response.Success(c, gin.H{
		"result":             "success",
		"invitation_results": invitationResults,
	})
}

func (h *MembersHandler) InviteWorkspaceMemberByEmailEx(c *gin.Context) {
	workspaceID := c.Param("workspace_id")
	if workspaceID == "" || workspaceID == "default" || workspaceID == ":workspace_id" {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	var req struct {
		Email       string   `json:"email" binding:"required"`
		Role        string   `json:"role" binding:"required"`
		Language    string   `json:"language,omitempty"`
		Name        string   `json:"name,omitempty"`
		Mobile      string   `json:"mobile,omitempty"`
		Gender      string   `json:"gender,omitempty"`
		Position    string   `json:"position,omitempty"`
		Permissions []string `json:"permissions,omitempty"`
		SendEmail   *bool    `json:"send_email,omitempty"`
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

	permissions := req.Permissions
	if permissions == nil {
		permissions = []string{}
	}

	if req.Role == "" {
		req.Role = "admin"
	}

	var sendEmail = true
	if req.SendEmail != nil {
		sendEmail = *req.SendEmail
	}

	isOrganizationAdmin, err := h.accountService.CheckOrganizationpAdminByWorkspace(c.Request.Context(), accountID, workspaceID)
	if err != nil {
		response.Fail(c, response.ErrSystemError)
		return
	}

	if req.Gender != "" {
		if !h.isValidGender(req.Gender) {
			response.Fail(c, response.ErrInvalidGender)
			return
		}
	}

	inviter, err := h.accountService.GetAccountByID(c.Request.Context(), accountID)
	if err != nil || inviter == nil {
		response.Fail(c, response.ErrAccountNotFound)
		return
	}

	var targetWorkspace *model.Workspace
	if isOrganizationAdmin {
		targetWorkspace, err = h.workspaceManagementService.GetWorkspaceByID(c.Request.Context(), workspaceID)
		if err != nil {
			response.Fail(c, response.ErrWorkspaceNotFound)
			return
		}
	} else {
		// Check if user has admin or owner role in workspace
		isAdminOrOwner, err := middleware.CheckAdminOrOwnerRole(c.Request.Context(), h.workspaceManagementService, accountID, workspaceID)
		if err != nil {
			response.Fail(c, response.ErrSystemError)
			return
		}
		isEnterpriseAdminOrOwner := middleware.IsOrganizationAdminOrOwner(c)

		if !isAdminOrOwner && !isEnterpriseAdminOrOwner {
			response.Fail(c, response.ErrPermissionDenied)
			return
		}

		targetWorkspace, err = h.workspaceManagementService.GetWorkspaceByID(c.Request.Context(), workspaceID)
		if err != nil {
			response.Fail(c, response.ErrWorkspaceNotFound)
			return
		}
	}

	if targetWorkspace == nil {
		response.Fail(c, response.ErrWorkspaceNotFound)
		return
	}

	invitationResults := make([]map[string]interface{}, 0)
	// Use inviter's interface language if request language is not provided
	language := req.Language
	if language == "" && inviter.InterfaceLanguage != nil {
		language = *inviter.InterfaceLanguage
	}

	token, err := h.accountService.InviteMemberEx(
		c.Request.Context(),
		workspaceID,
		accountID,
		req.Email,
		model.WorkspaceMemberRole(req.Role),
		language,
		req.Name,
		req.Mobile,
		req.Gender,
		req.Position,
		sendEmail,
	)

	if err != nil {
		if err == usererrors.ErrAccountAlreadyInWorkspace {
			invitationResults = append(invitationResults, map[string]interface{}{
				"status": "success",
				"email":  req.Email,
				"url":    h.consoleWebURL + "/login",
			})
		} else {
			invitationResults = append(invitationResults, map[string]interface{}{
				"status":  "failed",
				"email":   req.Email,
				"message": err.Error(),
			})
		}
	} else {
		// encodedEmail := url.QueryEscape(req.Email)
		encodedEmail := req.Email
		invitationResults = append(invitationResults, map[string]interface{}{
			"status": "success",
			"email":  req.Email,
			"url":    h.consoleWebURL + "/activate?email=" + encodedEmail + "&token=" + token,
		})
	}

	response.Success(c, gin.H{
		"result":             "success",
		"invitation_results": invitationResults,
	})
}

func (h *MembersHandler) isValidGender(gender string) bool {
	validGenders := []string{"male", "female", "other"}
	for _, validGender := range validGenders {
		if gender == validGender {
			return true
		}
	}
	return false
}

func (h *MembersHandler) InviteWorkspaceMemberByAccountId(c *gin.Context) {
	workspaceID := c.Param("workspace_id")
	if workspaceID == "" || workspaceID == "default" || workspaceID == ":workspace_id" {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	var req struct {
		AccountID   string   `json:"account_id" binding:"required"`
		Role        string   `json:"role" binding:"required"`
		Position    string   `json:"position,omitempty"`
		Permissions []string `json:"permissions,omitempty"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	if req.Role == "" {
		req.Role = "admin"
	}

	role := model.WorkspaceMemberRole(req.Role)
	if !role.IsNonOwnerRole() {
		response.Fail(c, response.ErrInvalidRole)
		return
	}

	accountID, exists := c.Get("account_id")
	if !exists {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	inviterID := accountID.(string)

	_, err := h.accountService.GetAccountByID(c.Request.Context(), inviterID)
	if err != nil {
		response.Fail(c, response.ErrAccountNotFound)
		return
	}

	var targetWorkspace *model.Workspace
	isGroupAdmin, err := h.accountService.CheckOrganizationpAdminByWorkspace(c.Request.Context(), inviterID, workspaceID)
	if err != nil {
		response.Fail(c, response.ErrSystemError)
		return
	}
	if isGroupAdmin {
		targetWorkspace, err = h.workspaceManagementService.GetWorkspaceByID(c.Request.Context(), workspaceID)
		if err != nil {
			response.Fail(c, response.ErrWorkspaceNotFound)
			return
		}
	} else {
		// Check if user has admin or owner role in workspace
		isAdminOrOwner, err := middleware.CheckAdminOrOwnerRole(c.Request.Context(), h.workspaceManagementService, inviterID, workspaceID)
		if err != nil {
			response.Fail(c, response.ErrSystemError)
			return
		}

		if !isAdminOrOwner {
			response.Fail(c, response.ErrPermissionDenied)
			return
		}

		targetWorkspace, err = h.workspaceManagementService.GetWorkspaceByID(c.Request.Context(), workspaceID)
		if err != nil {
			response.Fail(c, response.ErrWorkspaceNotFound)
			return
		}
	}

	if targetWorkspace == nil {
		response.Fail(c, response.ErrWorkspaceNotFound)
		return
	}

	targetAccount, err := h.accountService.GetAccountByID(c.Request.Context(), req.AccountID)
	if err != nil {
		response.Fail(c, response.ErrAccountNotFound)
		return
	}

	if targetAccount == nil {
		response.Fail(c, response.ErrAccountNotFound)
		return
	}

	existingJoin, _ := h.workspaceManagementService.GetUserRole(c.Request.Context(), req.AccountID, workspaceID)
	if existingJoin != nil {
		response.Fail(c, response.ErrMemberAlreadyExists)
		return
	}

	addReq := &interfaces.AddMemberRequest{
		WorkspaceID: workspaceID,
		AccountID:   req.AccountID,
		Role:        role,
	}

	err = h.workspaceManagementService.AddMember(c.Request.Context(), addReq)
	if err != nil {
		if strings.Contains(err.Error(), "permission") {
			response.Fail(c, response.ErrPermissionDenied)
			return
		}

		response.Fail(c, response.ErrSystemError)
		return
	}

	response.Success(c, gin.H{
		"result":     "success",
		"message":    "Member added successfully",
		"account_id": req.AccountID,
	})
}

func (h *MembersHandler) BatchInviteWorkspaceMembers(c *gin.Context) {
	workspaceID := c.Param("workspace_id")
	if workspaceID == "" || workspaceID == "default" || workspaceID == ":workspace_id" {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	var req struct {
		AccountIDs  []string `json:"account_ids" binding:"required"`
		Role        string   `json:"role,omitempty"`
		Position    string   `json:"position,omitempty"`
		Permissions []string `json:"permissions,omitempty"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	if req.Role == "" {
		req.Role = string(model.WorkspaceRoleNormal)
	}

	role := model.WorkspaceMemberRole(req.Role)
	if !role.IsNonOwnerRole() {
		response.Fail(c, response.ErrInvalidRole)
		return
	}

	accountID, exists := c.Get("account_id")
	if !exists {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	inviterID := accountID.(string)

	_, err := h.accountService.GetAccountByID(c.Request.Context(), inviterID)
	if err != nil {
		response.Fail(c, response.ErrAccountNotFound)
		return
	}

	var targetWorkspace *model.Workspace
	isGroupAdmin, err := h.accountService.CheckOrganizationpAdminByWorkspace(c.Request.Context(), inviterID, workspaceID)
	if err != nil {
		response.Fail(c, response.ErrSystemError)
		return
	}
	if isGroupAdmin {
		targetWorkspace, err = h.workspaceManagementService.GetWorkspaceByID(c.Request.Context(), workspaceID)
		if err != nil {
			response.Fail(c, response.ErrWorkspaceNotFound)
			return
		}
	} else {
		// Check if user has admin or owner role in workspace
		isAdminOrOwner, err := middleware.CheckAdminOrOwnerRole(c.Request.Context(), h.workspaceManagementService, inviterID, workspaceID)
		if err != nil {
			response.Fail(c, response.ErrSystemError)
			return
		}

		if !isAdminOrOwner {
			response.Fail(c, response.ErrPermissionDenied)
			return
		}

		targetWorkspace, err = h.workspaceManagementService.GetWorkspaceByID(c.Request.Context(), workspaceID)
		if err != nil {
			response.Fail(c, response.ErrWorkspaceNotFound)
			return
		}
	}

	if targetWorkspace == nil {
		response.Fail(c, response.ErrWorkspaceNotFound)
		return
	}

	results := make([]map[string]interface{}, 0)

	for _, accountID := range req.AccountIDs {
		result := make(map[string]interface{})
		result["account_id"] = accountID
		if isGroupAdmin && inviterID == accountID {
			result["status"] = "failed"
			result["message"] = "\"organization admin\" cannot invite themselves to the workspace"
			results = append(results, result)
			continue
		}

		targetAccount, err := h.accountService.GetAccountByID(c.Request.Context(), accountID)
		if err != nil || targetAccount == nil {
			result["status"] = "failed"
			result["message"] = "Account not found"
			results = append(results, result)
			continue
		}

		existingJoin, _ := h.workspaceManagementService.GetUserRole(c.Request.Context(), accountID, workspaceID)
		if existingJoin != nil {
			result["status"] = "failed"
			result["message"] = "Already in workspace"
			results = append(results, result)
			continue
		}

		addReq := &interfaces.AddMemberRequest{
			WorkspaceID: workspaceID,
			AccountID:   accountID,
			Role:        role,
		}

		err = h.workspaceManagementService.AddMember(c.Request.Context(), addReq)
		if err != nil {
			result["status"] = "failed"
			result["message"] = err.Error()
			results = append(results, result)
			continue
		}

		if req.Position != "" {
			extensionReq := &interfaces.CreateMemberExtensionRequest{
				WorkspaceID: workspaceID,
				AccountID:   accountID,
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

		results = append(results, result)
	}

	response.Success(c, gin.H{
		"result":             "success",
		"invitation_results": results,
	})
}

func (h *MembersHandler) CancelWorkspaceMemberInvite(c *gin.Context) {
	workspaceID := c.Param("workspace_id")
	memberID := c.Param("member_id")
	if workspaceID == "" || workspaceID == "default" || workspaceID == ":workspace_id" || memberID == "" {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	accountID := c.GetString("account_id")
	if accountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	if !h.requireWorkspacePermission(c, workspaceID, model.WorkspacePermissionWorkspaceManage) {
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

	var targetWorkspace *model.Workspace

	isGroupAdmin := h.isGroupAdminByWorkspace(currentUser, workspaceID)

	if isGroupAdmin {
		targetWorkspace, err = h.workspaceManagementService.GetWorkspaceByID(c.Request.Context(), workspaceID)
		if err != nil || targetWorkspace == nil {
			response.Fail(c, response.ErrWorkspaceNotFound)
			return
		}
	} else {
		currentWorkspaceJoin, err := h.workspaceManagementService.GetCurrentWorkspace(c.Request.Context(), accountID)
		if err != nil || currentWorkspaceJoin == nil || currentWorkspaceJoin.WorkspaceID != workspaceID {
			response.Fail(c, response.ErrWorkspaceNotFound)
			return
		}

		targetWorkspace, err = h.workspaceManagementService.GetWorkspaceByID(c.Request.Context(), workspaceID)
		if err != nil || targetWorkspace == nil {
			response.Fail(c, response.ErrWorkspaceNotFound)
			return
		}
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

func (h *MembersHandler) isGroupAdminByWorkspace(user *auth_model.Account, workspaceID string) bool {
	accountID := user.ID
	isGroupAdmin, err := h.accountService.CheckOrganizationpAdminByWorkspace(context.Background(), accountID, workspaceID)
	if err != nil {
		return false
	}
	return isGroupAdmin
}

func isCannotOperateSelfError(err error) bool {
	_, ok := err.(*usererrors.CannotOperateSelfError)
	return ok
}

func isNoPermissionError(err error) bool {
	return errors.Is(err, usererrors.ErrNoPermission)
}

func isMemberNotInWorkspaceError(err error) bool {
	return errors.Is(err, usererrors.ErrMemberNotInWorkspace)
}

func (h *MembersHandler) UpdateWorkspaceMemberRole(c *gin.Context) {
	workspaceID := c.Param("workspace_id")
	memberID := c.Param("member_id")
	if workspaceID == "" || workspaceID == "default" || workspaceID == ":workspace_id" || memberID == "" {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	var req struct {
		Role string `json:"role" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	newRole := req.Role

	role := model.WorkspaceMemberRole(newRole)
	if !role.IsValidRole() {
		response.Fail(c, response.ErrInvalidRole)
		return
	}

	member, err := h.accountService.GetAccountByID(c.Request.Context(), memberID)
	if err != nil || member == nil {
		response.Fail(c, response.ErrMemberNotFound)
		return
	}

	accountID := c.GetString("account_id")
	if accountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	currentUser, err := h.accountService.GetAccountByID(c.Request.Context(), accountID)
	if err != nil || currentUser == nil {
		response.Fail(c, response.ErrAccountNotFound)
		return
	}

	var targetWorkspace *model.Workspace

	isGroupAdmin := h.isGroupAdminByWorkspace(currentUser, workspaceID)

	if isGroupAdmin {
		targetWorkspace, err = h.workspaceManagementService.GetWorkspaceByID(c.Request.Context(), workspaceID)
		if err != nil || targetWorkspace == nil {
			response.Fail(c, response.ErrWorkspaceNotFound)
			return
		}
	} else {
		currentWorkspaceJoin, err := h.workspaceManagementService.GetCurrentWorkspace(c.Request.Context(), accountID)
		if err != nil || currentWorkspaceJoin == nil || currentWorkspaceJoin.WorkspaceID != workspaceID {
			response.Fail(c, response.ErrWorkspaceNotFound)
			return
		}

		targetWorkspace, err = h.workspaceManagementService.GetWorkspaceByID(c.Request.Context(), workspaceID)
		if err != nil || targetWorkspace == nil {
			response.Fail(c, response.ErrWorkspaceNotFound)
			return
		}
	}

	err = h.workspaceManagementService.UpdateMemberRoleWithPermissionCheck(c.Request.Context(), targetWorkspace, member, newRole, currentUser)
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

	response.Success(c, gin.H{
		"result": "success",
	})
}

func isRoleAlreadyAssignedError(err error) bool {
	_, ok := err.(*usererrors.RoleAlreadyAssignedError)
	return ok
}

func (h *MembersHandler) UpdateWorkspaceMemberExtension(c *gin.Context) {
	workspaceID := c.Param("workspace_id")
	memberID := c.Param("member_id")
	if workspaceID == "" || workspaceID == "default" || workspaceID == ":workspace_id" || memberID == "" {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	var req struct {
		Role        *string  `json:"role,omitempty"`
		Position    *string  `json:"position,omitempty"`
		Permissions []string `json:"permissions,omitempty"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	var newPermissions []string
	if req.Permissions != nil {
		newPermissions = req.Permissions
	} else {
		newPermissions = []string{}
	}

	member, err := h.accountService.GetAccountByID(c.Request.Context(), memberID)
	if err != nil || member == nil {
		response.Fail(c, response.ErrMemberNotFound)
		return
	}

	accountID := c.GetString("account_id")
	if accountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	currentUser, err := h.accountService.GetAccountByID(c.Request.Context(), accountID)
	if err != nil || currentUser == nil {
		response.Fail(c, response.ErrAccountNotFound)
		return
	}

	var targetWorkspace *model.Workspace

	isGroupAdmin := h.isGroupAdminByWorkspace(currentUser, workspaceID)

	if isGroupAdmin {
		targetWorkspace, err = h.workspaceManagementService.GetWorkspaceByID(c.Request.Context(), workspaceID)
		if err != nil || targetWorkspace == nil {
			response.Fail(c, response.ErrWorkspaceNotFound)
			return
		}
	} else {
		currentWorkspaceJoin, err := h.workspaceManagementService.GetCurrentWorkspace(c.Request.Context(), accountID)
		if err != nil || currentWorkspaceJoin == nil || currentWorkspaceJoin.WorkspaceID != workspaceID {
			response.Fail(c, response.ErrWorkspaceNotFound)
			return
		}

		targetWorkspace, err = h.workspaceManagementService.GetWorkspaceByID(c.Request.Context(), workspaceID)
		if err != nil || targetWorkspace == nil {
			response.Fail(c, response.ErrWorkspaceNotFound)
			return
		}
	}

	err = h.workspaceManagementService.UpdateMemberRoleExtensions(
		c.Request.Context(),
		targetWorkspace,
		member,
		req.Role,       // new_role
		req.Position,   // new_position
		newPermissions, // new_permissions
		currentUser,    // account_user
	)
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

	response.Success(c, gin.H{
		"result": "success",
	})
}

func (h *MembersHandler) GetWorkspaceDatasetOperatorMembers(c *gin.Context) {
	workspaceID := c.Param("workspace_id")
	if workspaceID == "" || workspaceID == "default" || workspaceID == ":workspace_id" {
		response.Fail(c, response.ErrInvalidParam)
		return
	}
	if !h.requireWorkspacePermission(c, workspaceID, model.WorkspacePermissionWorkspaceView) {
		return
	}

	members, err := h.workspaceManagementService.GetWorkspaceMembers(c.Request.Context(), workspaceID)
	if err != nil {
		response.Fail(c, response.ErrSystemError)
		return
	}

	var datasetOperators []*interfaces.AccountWithRole
	for _, member := range members {
		if member.Role == string(model.WorkspaceRoleOwner) ||
			member.Role == string(model.WorkspaceRoleAdmin) ||
			member.Role == string(model.WorkspaceRoleEditor) {
			datasetOperators = append(datasetOperators, member)
		}
	}

	response.Success(c, gin.H{
		"result":   "success",
		"accounts": datasetOperators,
	})
}

func (h *MembersHandler) GetWorkspaceNonMembers(c *gin.Context) {
	workspaceID := c.Param("workspace_id")
	if workspaceID == "" || workspaceID == "default" || workspaceID == ":workspace_id" {
		response.Fail(c, response.ErrInvalidParam)
		return
	}
	if !h.requireWorkspacePermission(c, workspaceID, model.WorkspacePermissionWorkspaceManage) {
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

	pagination, err := h.accountService.GetAccountsNotInWorkspace(
		c.Request.Context(),
		workspaceID,
		search,
		page,
		limit,
	)

	if err != nil {
		if strings.Contains(err.Error(), "invalid workspace") {
			response.Fail(c, response.ErrWorkspaceNotFound)
		} else {
			response.Fail(c, response.ErrSystemError)
		}
		return
	}

	items, ok := pagination.Items.([]interface{})
	if !ok {
		response.Fail(c, response.ErrSystemError)
		return
	}

	accounts := make([]map[string]interface{}, len(items))
	for i, item := range items {
		account := item.(*auth_model.Account)
		accountData := map[string]interface{}{
			"id":                 account.ID,
			"name":               account.Name,
			"email":              account.Email,
			"avatar":             account.Avatar,
			"avatar_url":         "",
			"interface_language": account.InterfaceLanguage,
			"interface_theme":    account.InterfaceTheme,
			"timezone":           account.Timezone,
			"status":             string(account.Status),
			"organization_role":  account.GroupRole,
			"created_at":         account.CreatedAt.Unix(),
		}

		// Re-implemented extension access using Extensions JSONMap
		if account.Extensions != nil {
			accountData["extension"] = account.Extensions
		} else {
			accountData["extension"] = nil
		}

		accountData["account_role"] = nil

		accounts[i] = accountData
	}

	response.Success(c, map[string]interface{}{
		"page":     pagination.Page,
		"limit":    pagination.PerPage,
		"total":    pagination.Total,
		"has_more": pagination.Page < pagination.TotalPages,
		"data":     accounts,
	})
}

func (h *MembersHandler) ReInviteMemberById(c *gin.Context) {
	memberID := c.Param("member_id")
	if memberID == "" {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	var req struct {
		SendEmail *bool `json:"send_email,omitempty"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	sendEmail := false
	if req.SendEmail != nil {
		sendEmail = *req.SendEmail
	}

	accountID := c.GetString("account_id")
	if accountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}
	currentUser, err := h.accountService.GetAccountByID(c.Request.Context(), accountID)
	if err != nil {
		response.Fail(c, response.ErrAccountNotFound)
		return
	}

	member, err := h.accountService.GetAccountByID(c.Request.Context(), memberID)
	if err != nil || member == nil {
		response.Fail(c, response.ErrMemberNotFound)
		return
	}
	operatorWorkspaceJoin, err := h.workspaceManagementService.GetCurrentWorkspace(c.Request.Context(), accountID)
	if err != nil || operatorWorkspaceJoin == nil {
		response.Fail(c, response.ErrWorkspaceNotFound)
		return
	}
	workspaceJoins, err := h.getWorkspaceJoinsForPendingMember(c.Request.Context(), member.ID, operatorWorkspaceJoin)
	if err != nil {
		response.Fail(c, response.ErrSystemError)
		return
	}

	if len(workspaceJoins) == 0 {
		response.Fail(c, response.ErrMemberNotInWorkspace)
		return
	}

	results := make([]map[string]interface{}, 0)

	for _, join := range workspaceJoins {
		workspace, err := h.workspaceManagementService.GetWorkspaceByID(c.Request.Context(), join.WorkspaceID)
		if err != nil || workspace == nil {
			continue
		}

		interfaceLanguage := ""
		if member.InterfaceLanguage != nil {
			interfaceLanguage = *member.InterfaceLanguage
		}

		token, err := h.accountService.InviteMemberEx(
			c.Request.Context(),
			workspace.ID,
			currentUser.ID,
			member.Email,
			join.Role,
			interfaceLanguage,
			member.Name,
			"", // mobile
			"", // gender
			"", // position
			sendEmail,
		)

		// encodedEmail := url.QueryEscape(member.Email)
		encodedEmail := member.Email
		invitationURL := h.consoleWebURL + "/activate?email=" + encodedEmail + "&token=" + token

		results = append(results, map[string]interface{}{
			"status":       "success",
			"email":        member.Email,
			"workspace_id": workspace.ID,
			"url":          invitationURL,
		})
	}

	if len(results) == 0 {
		response.Fail(c, response.ErrPermissionDenied)
		return
	}

	response.Success(c, gin.H{
		"result":             "success",
		"invitation_results": results,
	})
}

func (h *MembersHandler) getWorkspaceJoinsForPendingMember(ctx context.Context, memberID string, operatorWorkspace *model.WorkspaceMember) ([]*model.WorkspaceMember, error) {
	member, err := h.accountService.GetAccountByID(ctx, memberID)
	if err != nil || member == nil {
		return nil, fmt.Errorf("member not found")
	}

	if member.Status != auth_model.AccountStatusPending {
		return []*model.WorkspaceMember{}, nil
	}

	validJoins := []*model.WorkspaceMember{}
	groupID := operatorWorkspace.WorkspaceID
	var limit int = 200
	pagination, err := h.enterpriseService.GetManagedWorkspacesInOrganization(ctx, groupID, operatorWorkspace.AccountID, 1, limit)
	if err != nil {
		return nil, err
	}

	var workspaceList []string
	for _, workspace := range pagination.Data {
		workspaceList = append(workspaceList, workspace.ID)
	}
	if pagination.Total > int64(limit) {
		totalPage := int(math.Ceil(float64(pagination.Total) / float64(limit)))
		for i := 2; i <= totalPage; i++ {
			pagination, err := h.enterpriseService.GetManagedWorkspacesInOrganization(ctx, groupID, operatorWorkspace.AccountID, i, limit)
			if err != nil {
				break
			}
			for _, workspace := range pagination.Data {
				workspaceList = append(workspaceList, workspace.ID)
			}
		}
	}

	managedWorkspaceMap := make(map[string]bool)
	for _, workspaceID := range workspaceList {
		managedWorkspaceMap[workspaceID] = true
	}

	workspaces, err := h.workspaceManagementService.GetAccountWorkspaces(ctx, memberID)
	if err != nil {
		return nil, err
	}

	for _, workspace := range workspaces {
		if workspace.Status == model.WorkspaceStatusNormal {
			if managedWorkspaceMap[workspace.ID] {
				join := &model.WorkspaceMember{
					WorkspaceID: workspace.ID,
					AccountID:   memberID,
					Role:        model.WorkspaceRoleNormal,
				}
				validJoins = append(validJoins, join)
			}
		}
	}

	return validJoins, nil
}

func (h *MembersHandler) InviteDefaultMemberByEmailEx(c *gin.Context) {
	var req struct {
		Email       string   `json:"email" binding:"required"`
		Role        string   `json:"role,omitempty"`
		Language    string   `json:"language,omitempty"`
		Name        string   `json:"name,omitempty"`
		Mobile      string   `json:"mobile,omitempty"`
		Gender      string   `json:"gender,omitempty"`
		Position    string   `json:"position,omitempty"`
		Permissions []string `json:"permissions,omitempty"`
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

	inviteeRole := req.Role
	if inviteeRole == "" {
		inviteeRole = "normal"
	}

	role := model.WorkspaceMemberRole(inviteeRole)
	if !role.IsNonOwnerRole() {
		response.Fail(c, response.ErrInvalidRole)
		return
	}

	currentWorkspaceJoin, err := h.workspaceManagementService.GetCurrentWorkspace(c.Request.Context(), accountID)
	if err != nil || currentWorkspaceJoin == nil {
		response.Fail(c, response.ErrWorkspaceNotFound)
		return
	}

	workspaceID := currentWorkspaceJoin.WorkspaceID
	workspace, err := h.workspaceManagementService.GetWorkspaceByID(c.Request.Context(), workspaceID)
	if err != nil || workspace == nil {
		response.Fail(c, response.ErrWorkspaceNotFound)
		return
	}

	invitationResults := make([]map[string]interface{}, 0)

	token, err := h.accountService.InviteMemberEx(
		c.Request.Context(),
		workspaceID,
		accountID,
		req.Email,
		role,
		req.Language,
		req.Name,
		req.Mobile,
		req.Gender,
		req.Position,
		true,
	)

	if err != nil {
		if err == usererrors.ErrAccountAlreadyInWorkspace {
			invitationResults = append(invitationResults, map[string]interface{}{
				"status": "success",
				"email":  req.Email,
				"url":    h.consoleWebURL + "/login",
			})
		} else {
			invitationResults = append(invitationResults, map[string]interface{}{
				"status":  "failed",
				"email":   req.Email,
				"message": err.Error(),
			})
		}
	} else {
		// encodedEmail := url.QueryEscape(req.Email)
		encodedEmail := req.Email
		invitationResults = append(invitationResults, map[string]interface{}{
			"status": "success",
			"email":  req.Email,
			"url":    h.consoleWebURL + "/activate?email=" + encodedEmail + "&token=" + token,
		})
	}

	response.Success(c, gin.H{
		"result":             "success",
		"invitation_results": invitationResults,
	})
}

func (h *MembersHandler) RegisterRoutes(router *gin.RouterGroup) {

	workspaces := router.Group("/workspaces", middleware.JWTWithOrganizationAndService(h.accountService))
	{
		current := workspaces.Group("/current")
		{
			current.GET("/members", h.GetCurrentOrganizationMembers)
			current.GET("/members/:member_id", h.GetCurrentOrganizationMemberDetail)
			current.POST("/members/invite-email",
				accountMiddleware.SetupRequired(),
				accountMiddleware.AccountInitializationRequired(),
				h.InviteMemberByEmail)
			current.DELETE("/members/:member_id", h.CancelMemberInvite)
			current.PUT("/members/:member_id/update-role", h.UpdateMemberRole)
			current.GET("/dataset-operators", h.GetDatasetOperatorMembers)
		}

		workspace := workspaces.Group("/:workspace_id")
		{
			//workspace.GET("/members", h.GetWorkspaceMembers)
			workspace.GET("/members", h.GetWorkspaceMembersExtension)
			workspace.GET("/members/:member_id", h.GetWorkspaceMemberExtensionDetail)
			workspace.PUT("/members/:member_id", h.UpdateWorkspaceMemberExtensionDetail)
			workspace.POST("/members/invite-email", h.InviteWorkspaceMemberByEmailEx)
			workspace.POST("/members/invite-email-ex", h.InviteWorkspaceMemberByEmailEx)
			workspace.POST("/members/invite-by-id", h.InviteWorkspaceMemberByAccountId)
			workspace.POST("/members/batch-invite-by-ids", h.BatchInviteWorkspaceMembers)
			workspace.DELETE("/members/:member_id", h.CancelWorkspaceMemberInvite)
			workspace.PUT("/members/:member_id/update-role", h.UpdateWorkspaceMemberRole)
			workspace.PUT("/members/:member_id/update-ex", h.UpdateWorkspaceMemberExtension)
			workspace.GET("/dataset-operators", h.GetWorkspaceDatasetOperatorMembers)
			workspace.GET("/non-members", h.GetWorkspaceNonMembers)
		}

		workspaces.POST("/members/:member_id/re-invite", h.ReInviteMemberById)

		workspaces.POST("/default/members/invite-email-ex", h.InviteDefaultMemberByEmailEx)
	}
}
