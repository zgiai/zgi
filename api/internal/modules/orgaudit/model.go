// Package orgaudit keeps a queryable record of organization and permission
// changes: invites and joins, removals, member status, organization and
// workspace roles, role templates and direct permission edits.
package orgaudit

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
)

// Result says whether a recorded change took effect.
type Result string

const (
	ResultSuccess Result = "success"
	ResultDenied  Result = "denied"
	ResultFailed  Result = "failed"
)

// Action names one kind of audited change. Values are stable: they are stored
// and used as filters and as UI copy keys.
type Action string

const (
	ActionMemberInvite           Action = "member.invite"
	ActionMemberAdd              Action = "member.add"
	ActionMemberJoin             Action = "member.join"
	ActionMemberJoinApprove      Action = "member.join_approve"
	ActionMemberJoinReject       Action = "member.join_reject"
	ActionMemberRemove           Action = "member.remove"
	ActionMemberLeave            Action = "member.leave"
	ActionMemberStatusUpdate     Action = "member.status_update"
	ActionMemberRoleUpdate       Action = "member.role_update"
	ActionMemberPasswordReset    Action = "member.password_reset"
	ActionOwnershipTransfer      Action = "organization.ownership_transfer"
	ActionInviteLinkUpdate       Action = "invite_link.update"
	ActionWorkspaceMemberAdd     Action = "workspace_member.add"
	ActionWorkspaceMemberRemove  Action = "workspace_member.remove"
	ActionWorkspaceMemberLeave   Action = "workspace_member.leave"
	ActionWorkspaceMemberRole    Action = "workspace_member.role_update"
	ActionWorkspaceMemberPerms   Action = "workspace_member.permissions_update"
	ActionWorkspaceOwnerTransfer Action = "workspace.ownership_transfer"
	ActionRoleCreate             Action = "role.create"
	ActionRoleUpdate             Action = "role.update"
	ActionRolePermissionsUpdate  Action = "role.permissions_update"
	ActionRoleTemplateApply      Action = "role.template_apply"
	ActionRoleDelete             Action = "role.delete"
	ActionRoleReplaceDelete      Action = "role.replace_delete"
	ActionDepartmentCreate       Action = "department.create"
	ActionDepartmentUpdate       Action = "department.update"
	ActionDepartmentDelete       Action = "department.delete"
	ActionDepartmentMemberAdd    Action = "department.member_add"
	ActionDepartmentMemberRemove Action = "department.member_remove"
	ActionDepartmentMemberChange Action = "department.member_change"
)

// Target types name what a record's target_id points at.
const (
	TargetMember      = "member"
	TargetRole        = "role"
	TargetDepartment  = "department"
	TargetWorkspace   = "workspace"
	TargetInviteLink  = "invite_link"
	TargetJoinRequest = "join_request"
)

// Log is one stored audit record.
type Log struct {
	ID              uuid.UUID      `gorm:"type:uuid;primaryKey" json:"id"`
	OrganizationID  uuid.UUID      `gorm:"type:uuid;not null" json:"organization_id"`
	WorkspaceID     *uuid.UUID     `gorm:"type:uuid" json:"workspace_id,omitempty"`
	ActorAccountID  *uuid.UUID     `gorm:"type:uuid" json:"actor_account_id,omitempty"`
	TargetAccountID *uuid.UUID     `gorm:"type:uuid" json:"target_account_id,omitempty"`
	TargetType      string         `gorm:"size:32;not null;default:''" json:"target_type"`
	TargetID        string         `gorm:"size:64;not null;default:''" json:"target_id"`
	Action          Action         `gorm:"size:64;not null" json:"action"`
	Result          Result         `gorm:"size:16;not null" json:"result"`
	Before          datatypes.JSON `gorm:"type:jsonb;not null" json:"before"`
	After           datatypes.JSON `gorm:"type:jsonb;not null" json:"after"`
	RequestID       string         `gorm:"size:128;not null;default:''" json:"request_id"`
	IP              string         `gorm:"column:ip;size:64;not null;default:''" json:"ip"`
	CreatedAt       time.Time      `gorm:"not null" json:"created_at"`
}

// TableName binds Log to its table.
func (Log) TableName() string { return "org_audit_logs" }

// Filter narrows a list of an organization's records.
type Filter struct {
	OrganizationID  uuid.UUID
	ActorAccountID  *uuid.UUID
	TargetAccountID *uuid.UUID
	Action          Action
	Result          Result
	From            *time.Time
	To              *time.Time
	Page            int
	Limit           int
}
