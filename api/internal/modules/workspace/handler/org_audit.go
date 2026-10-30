package handler

import (
	"errors"

	"github.com/gin-gonic/gin"
	"github.com/zgiai/zgi/api/internal/dto"
	usererrors "github.com/zgiai/zgi/api/internal/errors"
	"github.com/zgiai/zgi/api/internal/modules/orgaudit"
	"github.com/zgiai/zgi/api/internal/modules/workspace/model"
)

// SetAuditRecorder makes the handler record organization and permission
// changes. Without it the routes are served unaudited.
func (h *OrganizationHandler) SetAuditRecorder(recorder *orgaudit.Recorder) { h.audit = recorder }

// SetAuditRecorder makes the handler record member and permission changes.
func (h *MembersHandler) SetAuditRecorder(recorder *orgaudit.Recorder) { h.audit = recorder }

// SetAuditRecorder makes the handler record department changes.
func (h *DepartmentHandler) SetAuditRecorder(recorder *orgaudit.Recorder) { h.audit = recorder }

func memberAuditTarget(accountID string) orgaudit.Target {
	return orgaudit.Target{AccountID: accountID, Type: orgaudit.TargetMember, ID: accountID}
}

func roleAuditFields(role *dto.OrganizationRoleDetailResponse) map[string]any {
	if role == nil {
		return nil
	}
	fields := map[string]any{"name": role.Name, "permissions": role.Permissions}
	if role.Description != nil {
		fields["description"] = *role.Description
	}
	return fields
}

func workspaceMemberAuditFields(member *dto.WorkspaceMemberPermissionsResponse) map[string]any {
	if member == nil {
		return nil
	}
	fields := map[string]any{
		"workspace_role":      member.WorkspaceRole,
		"workspace_role_name": member.WorkspaceRoleName,
		"permissions":         member.Permissions,
		"permission_source":   member.PermissionSource,
	}
	if member.WorkspaceRoleID != nil {
		fields["workspace_role_id"] = *member.WorkspaceRoleID
	}
	return fields
}

// auditRoleBefore records a role's state before it changes. It only reads
// when auditing is on, and a failed read leaves before empty.
func (h *OrganizationHandler) auditRoleBefore(c *gin.Context, organizationID, roleID, accountID string) {
	if h.audit == nil {
		return
	}
	role, err := h.organizationService.GetWorkspaceRoleDetail(c.Request.Context(), organizationID, roleID, accountID)
	if err == nil {
		orgaudit.SetBefore(c, roleAuditFields(role))
	}
}

// auditWorkspaceMemberBefore records a workspace member's role and
// permissions before they change.
func (h *OrganizationHandler) auditWorkspaceMemberBefore(c *gin.Context, organizationID, workspaceID, accountID, targetAccountID string) {
	if h.audit == nil {
		return
	}
	member, err := h.organizationService.GetWorkspaceMemberPermissions(c.Request.Context(), organizationID, workspaceID, accountID, targetAccountID)
	if err == nil {
		orgaudit.SetBefore(c, workspaceMemberAuditFields(member))
	}
}

// auditOrganizationRoleBefore records a member's organization role before
// it changes.
func (h *OrganizationHandler) auditOrganizationRoleBefore(c *gin.Context, organizationID, targetAccountID string) {
	if h.audit == nil {
		return
	}
	role, err := h.organizationService.GetUserOrganizationRole(c.Request.Context(), organizationID, targetAccountID)
	if err == nil && role != "" {
		orgaudit.SetBefore(c, map[string]any{"organization_role": role})
	}
}

func joinRequestAuditFields(request *model.OrganizationJoinRequest) map[string]any {
	if request == nil {
		return nil
	}
	return map[string]any{
		"join_request_id":           request.ID,
		"invite_link_id":            request.InviteLinkID,
		"status":                    request.Status,
		"department_id":             request.DepartmentID,
		"workspace_id":              request.WorkspaceID,
		"default_organization_role": request.DefaultOrganizationRole,
		"default_workspace_role":    request.DefaultWorkspaceRole,
	}
}

func departmentAuditFields(department *model.Department) map[string]any {
	if department == nil {
		return nil
	}
	return map[string]any{"name": department.Name, "parent_id": department.ParentID, "status": department.Status}
}

// auditMemberStatusBefore records a member's organization status before it
// changes.
func (h *OrganizationHandler) auditMemberStatusBefore(c *gin.Context, organizationID, targetAccountID string) {
	if h.audit == nil {
		return
	}
	member, err := h.organizationService.GetOrganizationMemberByAccountID(c.Request.Context(), organizationID, targetAccountID)
	if err == nil && member != nil {
		orgaudit.SetBefore(c, map[string]any{"status": member.Status})
	}
}

// auditInvite adds one audit target for an invitation into workspaceID, with
// that invitation's own result, so a batch that partly fails records each
// member separately. An account already in the workspace changed nothing and
// adds no target. A member invited by email is resolved to its account when
// it has one.
func (h *MembersHandler) auditInvite(c *gin.Context, workspaceID, accountID, email string, err error) {
	if h.audit == nil || errors.Is(err, usererrors.ErrAccountAlreadyInWorkspace) {
		return
	}
	if accountID == "" && email != "" {
		if account, lookupErr := h.accountService.GetUserThroughEmail(c.Request.Context(), email); lookupErr == nil && account != nil {
			accountID = account.ID
		}
	}
	target := orgaudit.Target{AccountID: accountID, Type: orgaudit.TargetMember, ID: accountID, WorkspaceID: workspaceID, Result: orgaudit.ResultSuccess}
	if target.ID == "" {
		target.ID = email
	}
	// An invitation whose email failed to send still created the member.
	if err != nil && !errors.Is(err, usererrors.ErrInviteEmailDeliveryFailed) {
		target.Result = orgaudit.ResultFailed
	}
	orgaudit.AddTarget(c, target)
}

// auditWorkspaceOrganization names the organization that owns workspace, so
// a workspace route is recorded in that organization rather than the
// caller's current one.
func auditWorkspaceOrganization(c *gin.Context, workspace *model.Workspace) {
	if workspace != nil && workspace.OrganizationID != nil {
		orgaudit.SetOrganization(c, *workspace.OrganizationID)
	}
}
