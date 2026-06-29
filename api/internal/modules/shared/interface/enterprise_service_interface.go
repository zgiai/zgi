package interfaces

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/dto"

	"github.com/zgiai/zgi/api/internal/modules/workspace/model"
	"gorm.io/gorm"
)

type OfficialRouteBootstrapper interface {
	InitOfficialChannel(ctx context.Context, organizationID uuid.UUID) error
}

type OrganizationService interface {
	CreateOrganization(ctx context.Context, req *dto.CreateOrganizationRequest) (*model.Organization, error)
	CreateOrganizationWithWorkspace(ctx context.Context, req *dto.CreateOrganizationWithWorkspaceRequest) (*model.Organization, error)
	CheckOrganizationNameExists(ctx context.Context, name string) (bool, error)
	ListUserOrganizations(ctx context.Context, page, limit int, status string, accountID string) (*dto.OrganizationPaginationResponse, error)
	GetOrganizationByID(ctx context.Context, id string) (*model.Organization, error)
	UpdateOrganization(ctx context.Context, id, accountID string, req *dto.UpdateOrganizationRequest) (*model.Organization, error)
	DeleteOrganization(ctx context.Context, id string, accountID string) error

	AddWorkspace(ctx context.Context, req *dto.AddWorkspaceToOrganizationRequest) error
	RemoveWorkspace(ctx context.Context, organizationID, workspaceID string) error
	UpdateWorkspaceJoinMeta(ctx context.Context, organizationID, workspaceID string, apiKeyID *string) error
	GetOrganizationWorkspaces(ctx context.Context, organizationID string, page, limit int, accountID string) (*dto.WorkspacePaginationResponse, error)
	GetOrganizationWorkspacesWithDetails(ctx context.Context, organizationID string, page, limit int, accountID string, status string, keyword string) (*dto.OrganizationWorkspacePaginationResponse, error)
	GetOrganizationWorkspaceDetail(ctx context.Context, organizationID, workspaceID, accountID string) (*dto.OrganizationWorkspaceResponse, error)
	GetOrganizationWorkspacesList(ctx context.Context, organizationID string) ([]*model.Workspace, error)
	GetOrganizationByWorkspaceID(ctx context.Context, workspaceID string) (*model.Organization, error)

	AddMember(ctx context.Context, req *dto.AddOrganizationMemberRequest) error
	DirectAddOrganizationMember(ctx context.Context, req *dto.DirectAddOrganizationMemberRequest) (*dto.DirectAddOrganizationMemberResponse, error)
	InviteCurrentOrganizationMember(ctx context.Context, req *dto.InviteCurrentOrganizationMemberRequest) (*dto.InviteCurrentOrganizationMemberResponse, error)
	ResetCurrentOrganizationMemberPassword(ctx context.Context, req *dto.ResetCurrentOrganizationMemberPasswordRequest) (*dto.ResetCurrentOrganizationMemberPasswordResponse, error)
	RemoveMember(ctx context.Context, organizationID, accountID string) error
	UpdateMemberRole(ctx context.Context, req *dto.UpdateOrganizationMemberRoleRequest) error
	UpdateCurrentOrganizationMemberRole(ctx context.Context, operatorID, memberID string, role model.OrganizationRole) error
	UpdateMemberStatus(ctx context.Context, req *dto.UpdateOrganizationMemberStatusRequest) error
	UpdateMemberInfo(ctx context.Context, req *dto.UpdateOrganizationMemberRequest) error
	TransferOwnership(ctx context.Context, organizationID, currentOwnerID, newOwnerID string) error
	GetOrganizationMembers(ctx context.Context, organizationID string) ([]*dto.OrganizationMemberResponse, error)
	GetOrganizationMembersPaginated(ctx context.Context, organizationID string, page, limit int, keyword string) (*dto.OrganizationMemberPaginationResponse, error)
	GetOrganizationMemberByAccountID(ctx context.Context, organizationID, accountID string) (*dto.OrganizationMemberWithExtensionResponse, error)
	ExistsMemberByName(ctx context.Context, organizationID string, name string, excludeAccountID string) (bool, error)

	IsOrganizationMember(ctx context.Context, organizationID, accountID string) (bool, error)
	GetUserOrganizationRole(ctx context.Context, organizationID, accountID string) (model.OrganizationRole, error)
	CheckOrganizationOwner(ctx context.Context, organizationID, accountID string) (bool, error)
	IsOrganizationAdminOrOwner(ctx context.Context, organizationID, accountID string) (bool, error)
	CheckAnyManagedWorkspacePermission(ctx context.Context, organizationID, accountID string) (bool, error)
	CheckAnyWorkspaceCreateAppPermission(ctx context.Context, organizationID, accountID string) (bool, error)
	CheckAnyWorkspaceCreateDatasetPermission(ctx context.Context, organizationID, accountID string) (bool, error)
	CheckWorkspacePermission(ctx context.Context, organizationID, workspaceID, accountID string, permissionCode model.WorkspacePermissionCode) (bool, error)
	CheckWorkspaceOrganizationAnyPermission(ctx context.Context, organizationID, workspaceID, accountID string, permissionCodes ...model.WorkspacePermissionCode) (bool, error)
	ListWorkspaceIDsByPermission(ctx context.Context, organizationID, accountID string, permissionCode model.WorkspacePermissionCode) ([]string, error)

	CheckWorkspaceNameExistsInOrganization(ctx context.Context, organizationID, workspaceName string) (bool, error)
	GetUnjoinedWorkspacesForUser(ctx context.Context, organizationID, userID string, page, limit int) (*dto.WorkspacePaginationResponse, error)
	GetUserOrganizationsByAccount(ctx context.Context, accountID string, page, limit int) (*dto.OrganizationPaginationResponse, error)
	GetUserWorkspacesInOrganization(ctx context.Context, organizationID, accountID string, page, limit int) (*dto.WorkspacePaginationResponse, error)
	GetUserWorkspacesRolesInOrganization(ctx context.Context, organizationID, accountID string) ([]*dto.WorkspaceRoleResponse, error)
	GetUserAllWorkspacesInOrganization(ctx context.Context, organizationID, accountID string) ([]*model.Workspace, error)
	GetVisibleOrganizationMembersPaginated(ctx context.Context, organizationID, accountID string, page, limit int, keyword string) (*dto.OrganizationMemberPaginationResponse, error)
	CheckWorkspaceAssets(ctx context.Context, workspaceID string) (bool, map[string]int64, error)
	GetManagedWorkspacesInOrganization(ctx context.Context, organizationID, accountID string, page, limit int) (*dto.WorkspacePaginationResponse, error)
	GetManagedAppWorkspacesInOrganization(ctx context.Context, organizationID, accountID string, page, limit int) (*dto.WorkspacePaginationResponse, error)
	GetManagedDatasetWorkspacesInOrganization(ctx context.Context, organizationID, accountID string, page, limit int) (*dto.WorkspacePaginationResponse, error)

	GetByID(ctx context.Context, organizationID string) (*model.Organization, error)
	GetFirstOwnedOrganization(ctx context.Context, accountID string) (*model.Organization, error)
	GetFirstJoinedOrganization(ctx context.Context, accountID string) (*model.Organization, error)
	GetCurrentOrganization(ctx context.Context, accountID string) (*dto.CurrentOrganizationResponse, error)
	GetCurrentOrganizationDetail(ctx context.Context, accountID string) (*dto.CurrentOrganizationDetailResponse, error)

	GetOrganizationDatasetsPaginated(ctx context.Context, req *dto.GetOrganizationDatasetsPaginatedRequest) (*dto.DatasetPaginationResponse, error)

	GetDepartmentInviteLink(ctx context.Context, organizationID, departmentID, accountID string) (*model.OrganizationInviteLink, error)
	CreateOrResetDepartmentInviteLink(ctx context.Context, organizationID, departmentID, accountID string, requireApproval bool, expiresAt *time.Time) (*model.OrganizationInviteLink, error)
	UpdateDepartmentInviteLinkStatus(ctx context.Context, organizationID, departmentID, accountID, status string) (*model.OrganizationInviteLink, error)

	GetInviteLinkByToken(ctx context.Context, token string) (*model.OrganizationInviteLink, error)
	GetPendingJoinRequest(ctx context.Context, organizationID, accountID string) (*model.OrganizationJoinRequest, error)
	AcceptInviteByToken(ctx context.Context, token, accountID string, name *string) (*model.OrganizationJoinRequest, error)

	ListDepartmentJoinRequests(ctx context.Context, organizationID, departmentID, accountID string, status *model.OrganizationJoinRequestStatus) ([]*model.OrganizationJoinRequest, error)
	ListOrganizationJoinRequests(ctx context.Context, organizationID, accountID string, departmentID *string, status *model.OrganizationJoinRequestStatus, page, limit int) (*dto.OrganizationJoinRequestPaginationResponse, error)
	ApproveDepartmentJoinRequest(ctx context.Context, organizationID, joinRequestID, reviewerAccountID string) (*model.OrganizationJoinRequest, error)
	RejectDepartmentJoinRequest(ctx context.Context, organizationID, joinRequestID, reviewerAccountID string, reason *string) error

	// roles & permissions
	ListWorkspacePermissionDefinitions(ctx context.Context, organizationID, accountID string) ([]string, error)
	ListWorkspaceRoles(ctx context.Context, organizationID, accountID string, includeOwner bool) (*dto.WorkspaceRoleListResponse, error)
	GetWorkspaceRoleDetail(ctx context.Context, organizationID, roleID, accountID string) (*dto.OrganizationRoleDetailResponse, error)
	ListWorkspaceRoleMembers(ctx context.Context, organizationID, roleID, accountID, keyword string, page, limit int) (*dto.OrganizationRoleMembersResponse, error)
	CreateCustomWorkspaceRole(ctx context.Context, req *dto.CreateWorkspaceRoleRequest) (*dto.OrganizationRoleDetailResponse, error)
	UpdateCustomWorkspaceRole(ctx context.Context, req *dto.UpdateWorkspaceRoleRequest) (*dto.OrganizationRoleDetailResponse, error)
	UpdateWorkspaceRolePermissions(ctx context.Context, req *dto.UpdateWorkspaceRolePermissionsRequest) error
	ApplyWorkspaceRoleTemplate(ctx context.Context, req *dto.ApplyWorkspaceRoleTemplateRequest) (*dto.ApplyWorkspaceRoleTemplateResponse, error)
	DeleteCustomWorkspaceRole(ctx context.Context, organizationID, roleID, accountID string) error
	GetMemberEffectivePermissions(ctx context.Context, organizationID, accountID, targetAccountID string) (*dto.MemberPermissionsResponse, error)
	GetWorkspaceMemberPermissions(ctx context.Context, organizationID, workspaceID, accountID, targetAccountID string) (*dto.WorkspaceMemberPermissionsResponse, error)
	IsValidCustomWorkspaceRole(ctx context.Context, organizationID, roleID, accountID string) (bool, error)
}

type OrganizationManagementService interface {
	CreateOrganization(ctx context.Context, name string) (*model.Organization, error)
	UpsertOrganizationRole(ctx context.Context, organizationID string, accountID string, role model.OrganizationRole) error
	AddWorkspace(ctx context.Context, organizationID string, workspaceID string) error
	WithTx(tx *gorm.DB) OrganizationManagementService
}
