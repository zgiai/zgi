package interfaces

import (
	"context"
	"time"

	auth_model "github.com/zgiai/zgi/api/internal/modules/user/auth/model"
	"github.com/zgiai/zgi/api/internal/modules/workspace/model"
	"gorm.io/gorm"
)

type WorkspaceManagementService interface {
	GetWorkspaceByID(ctx context.Context, id string) (*model.Workspace, error)
	CreateWorkspace(ctx context.Context, name string, isFromDashboard bool) (*model.Workspace, error)
	UpdateWorkspace(ctx context.Context, id string, req *UpdateWorkspaceRequest) error
	DeleteWorkspace(ctx context.Context, id string) error
	DeleteWorkspaceWithMembers(ctx context.Context, id string) error

	AddMember(ctx context.Context, req *AddMemberRequest) error
	RemoveMember(ctx context.Context, workspaceID, accountID string) error
	RemoveMemberFromWorkspace(ctx context.Context, tenant *model.Workspace, member *auth_model.Account, operator *auth_model.Account) error
	UpdateMemberRole(ctx context.Context, req *UpdateMemberRoleRequest) error
	UpdateMemberRoleWithPermissionCheck(ctx context.Context, tenant *model.Workspace, member *auth_model.Account, newRole string, operator *auth_model.Account) error
	UpdateMemberRoleAndRoleIDWithPermissionCheck(ctx context.Context, tenant *model.Workspace, member *auth_model.Account, newRole string, roleID *string, operator *auth_model.Account) error
	UpdateMemberCustomRoleWithPermissionCheck(ctx context.Context, tenant *model.Workspace, member *auth_model.Account, roleID string, operator *auth_model.Account) error
	UpdateMemberDirectPermissions(ctx context.Context, workspaceID, accountID string, permissions []string) error
	GetWorkspaceMembers(ctx context.Context, workspaceID string) ([]*AccountWithRole, error)
	GetWorkspaceMembersPaginated(ctx context.Context, workspaceID string, page, limit int, keyword, roleFilter string) ([]*AccountWithRole, int64, error)
	GetWorkspaceMembersWithExtensions(ctx context.Context, workspaceID string) ([]*WorkspaceMemberWithExtensionResponse, error)
	GetWorkspaceMemberWithExtensionsById(ctx context.Context, workspaceID, memberID string) (*WorkspaceMemberWithExtensionResponse, error)
	GetDatasetOperatorMembers(ctx context.Context, workspaceID string) ([]*AccountWithRole, error)
	CreateWorkspaceMember(ctx context.Context, workspaceID string, accountID string, role string) error

	GetByWorkspaceAndMember(ctx context.Context, workspaceID, accountID string) (*model.WorkspaceMember, error)

	LeaveWorkspace(ctx context.Context, workspaceID, accountID string) error
	TransferOwner(ctx context.Context, workspaceID, currentOwnerID, newOwnerID string) error

	CheckPermission(ctx context.Context, workspaceID, accountID string) bool
	CheckMemberPermission(ctx context.Context, tenant *model.Workspace, operator *auth_model.Account, member *auth_model.Account, action string) error

	ChangeWorkspaceWithJoin(ctx context.Context, member *auth_model.Account, fromWorkspaceID, toWorkspaceID string, operator *auth_model.Account) error
	UpdateMemberRoleExtensions(ctx context.Context, tenant *model.Workspace, member *auth_model.Account, newRole, newPosition *string, newPermissions []string, operator *auth_model.Account) error

	CreateMemberExtension(ctx context.Context, req *CreateMemberExtensionRequest) error
	UpdateMemberExtension(ctx context.Context, req *UpdateMemberExtensionRequest) error

	GetAccountWorkspaces(ctx context.Context, accountID string) ([]*model.Workspace, error)
	SwitchWorkspace(ctx context.Context, accountID, workspaceID string) error
	GetCurrentWorkspace(ctx context.Context, accountID string) (*model.WorkspaceMember, error)
	GetCurrentOrganization(ctx context.Context, accountID string) (*model.OrganizationMember, error)
	GetUserRole(ctx context.Context, accountID, workspaceID string) (*model.WorkspaceMemberRole, error)
	GetJoinWorkspaces(ctx context.Context, account *auth_model.Account) ([]*model.Workspace, error)

	WithTx(tx *gorm.DB) WorkspaceManagementService

	GetAccountWorkspaceJoins(ctx context.Context, accountID string) ([]*model.WorkspaceMember, error)
	GetWorkspaceAccountJoins(ctx context.Context, workspaceID string) ([]*model.WorkspaceMember, error)
	GetWorkspacesByIDs(ctx context.Context, workspaceIDs []string) ([]*model.Workspace, error)
	GetAccessibleWorkspaceIDs(ctx context.Context, accountID string) ([]string, error)
	GetWorkspaceIDsByOrganizationID(ctx context.Context, organizationID string) ([]string, error)
	GetUserWorkspaceMemberships(ctx context.Context, accountID string) ([]WorkspaceMembership, error)
}

type UpdateWorkspaceRequest struct {
	Name string `json:"name"`
	Plan string `json:"plan"`
}

type AddMemberRequest struct {
	WorkspaceID string                    `json:"workspace_id" binding:"required"`
	AccountID   string                    `json:"account_id" binding:"required"`
	Role        model.WorkspaceMemberRole `json:"role" binding:"required"`
	RoleID      *string                   `json:"role_id,omitempty"`
	Permissions *[]string                 `json:"permissions,omitempty"`
}

type UpdateMemberRoleRequest struct {
	WorkspaceID string                    `json:"workspace_id" binding:"required"`
	AccountID   string                    `json:"account_id" binding:"required"`
	Role        model.WorkspaceMemberRole `json:"role" binding:"required"`
}

type AccountWithRole struct {
	ID                       string                                `json:"id"`
	Name                     string                                `json:"name"`         // Display name (member_name || account_name)
	AccountName              string                                `json:"account_name"` // Original account name
	MemberName               *string                               `json:"member_name"`  // Member nickname in organization
	Avatar                   string                                `json:"avatar"`
	AvatarURL                string                                `json:"avatar_url"`
	Email                    string                                `json:"email"`
	LastLoginAt              *int64                                `json:"last_login_at"`
	LastActiveAt             *int64                                `json:"last_active_at"`
	CreatedAt                int64                                 `json:"created_at"`
	Role                     string                                `json:"role"`
	RoleID                   *string                               `json:"role_id,omitempty"`
	Permissions              []string                              `json:"permissions"`
	PermissionSource         model.WorkspaceMemberPermissionSource `json:"permission_source"`
	PermissionTemplateRoleID *string                               `json:"permission_template_role_id,omitempty"`
	Status                   string                                `json:"status"`
	HasMobile                bool                                  `json:"has_mobile"`
	DepartmentID             *string                               `json:"department_id,omitempty"`
	DepartmentName           *string                               `json:"department_name,omitempty"`
}

type WorkspaceMemberWithExtensionResponse struct {
	Account                  *auth_model.Account                   `json:"account"`
	Role                     model.WorkspaceMemberRole             `json:"role"`
	RoleID                   *string                               `json:"role_id,omitempty"`
	JoinedAt                 time.Time                             `json:"joined_at"`
	Position                 string                                `json:"position"`
	Permissions              []string                              `json:"permissions"`
	PermissionSource         model.WorkspaceMemberPermissionSource `json:"permission_source"`
	PermissionTemplateRoleID *string                               `json:"permission_template_role_id,omitempty"`
	Extension                map[string]interface{}                `json:"extension"`
	Mobile                   string                                `json:"mobile"`
	OrganizationRole         string                                `json:"organization_role"`
}

type CreateMemberExtensionRequest struct {
	WorkspaceID string   `json:"workspace_id" binding:"required"`
	AccountID   string   `json:"account_id" binding:"required"`
	Position    string   `json:"position"`
	Permissions []string `json:"permissions"`
}

type UpdateMemberExtensionRequest struct {
	WorkspaceID string    `json:"workspace_id" binding:"required"`
	AccountID   string    `json:"account_id" binding:"required"`
	Position    *string   `json:"position"`
	Permissions *[]string `json:"permissions"`
}

// WorkspaceMembership represents a user's membership in a workspace (tenant with current=false)
type WorkspaceMembership struct {
	WorkspaceID string
	Role        model.WorkspaceMemberRole
}
