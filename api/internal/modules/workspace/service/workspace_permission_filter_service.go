package service

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/zgiai/zgi/api/internal/modules/workspace/model"
	"github.com/zgiai/zgi/api/internal/modules/workspace/repository"
	"github.com/zgiai/zgi/api/pkg/logger"
	"gorm.io/gorm"
)

// WorkspacePermissionFilterService defines the contract for filtering workspaces by permission.
type WorkspacePermissionFilterService interface {
	// GetAccessibleWorkspacesByPermission returns workspaces where user has specific permission.
	GetAccessibleWorkspacesByPermission(
		ctx context.Context,
		accountID string,
		organizationID string,
		permissionType string,
	) ([]*WorkspacePermissionResponse, error)
}

// WorkspacePermissionResponse represents a workspace in the response.
type WorkspacePermissionResponse struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// WorkspacePermissionFilterServiceImpl implements WorkspacePermissionFilterService.
type WorkspacePermissionFilterServiceImpl struct {
	organizationRepo    repository.OrganizationRepository
	workspaceRepo       repository.WorkspaceRepository
	workspaceMemberRepo repository.WorkspaceMemberRepository
}

// NewWorkspacePermissionFilterService creates a new instance of WorkspacePermissionFilterService.
func NewWorkspacePermissionFilterService(
	organizationRepo repository.OrganizationRepository,
	workspaceRepo repository.WorkspaceRepository,
	workspaceMemberRepo repository.WorkspaceMemberRepository,
) WorkspacePermissionFilterService {
	return &WorkspacePermissionFilterServiceImpl{
		organizationRepo:    organizationRepo,
		workspaceRepo:       workspaceRepo,
		workspaceMemberRepo: workspaceMemberRepo,
	}
}

// GetAccessibleWorkspacesByPermission returns workspaces where user has specific permission.
func (s *WorkspacePermissionFilterServiceImpl) GetAccessibleWorkspacesByPermission(
	ctx context.Context,
	accountID string,
	organizationID string,
	permissionType string,
) ([]*WorkspacePermissionResponse, error) {
	permissionCode, err := workspacePermissionCodeForFilterType(permissionType)
	if err != nil {
		return nil, err
	}

	organization, err := s.organizationRepo.GetByID(ctx, organizationID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("organization not found")
		}
		logger.Error("Failed to get organization", err)
		return nil, fmt.Errorf("failed to get organization: %w", err)
	}
	if organization == nil {
		return nil, fmt.Errorf("organization not found")
	}

	organizationRole, err := s.organizationRepo.GetAccountJoin(ctx, organizationID, accountID)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		logger.Error("Failed to get account join", err)
		return nil, fmt.Errorf("failed to check user role: %w", err)
	}

	if err == nil && organizationRole != nil && isOwnerOrAdmin(organizationRole.Role) {
		return s.getAllWorkspacesByOrganization(ctx, organizationID)
	}

	return s.getPermittedWorkspacesByUser(ctx, accountID, organizationID, permissionCode)
}

// getAllWorkspacesByOrganization returns all active workspaces in an organization.
func (s *WorkspacePermissionFilterServiceImpl) getAllWorkspacesByOrganization(ctx context.Context, organizationID string) ([]*WorkspacePermissionResponse, error) {
	workspaces, err := s.organizationRepo.GetWorkspacesByOrganizationID(ctx, organizationID)
	if err != nil {
		logger.Error("Failed to get workspaces for organization", err)
		return nil, fmt.Errorf("failed to get workspaces for organization: %w", err)
	}

	if len(workspaces) == 0 {
		return []*WorkspacePermissionResponse{}, nil
	}

	// Extract workspace IDs
	workspaceIDs := make([]string, len(workspaces))
	for i, workspace := range workspaces {
		workspaceIDs[i] = workspace.ID
	}

	// Get workspace details
	workspacesDetails, err := s.workspaceRepo.GetByIDs(ctx, workspaceIDs)
	if err != nil {
		logger.Error("Failed to get workspace details", err)
		return nil, fmt.Errorf("failed to get workspace details: %w", err)
	}

	// Convert to response format and sort by creation date
	responses := make([]*WorkspacePermissionResponse, 0, len(workspacesDetails))
	for _, workspace := range workspacesDetails {
		if workspace.Status != model.WorkspaceStatusNormal {
			continue
		}

		responses = append(responses, &WorkspacePermissionResponse{
			ID:   workspace.ID,
			Name: workspace.Name,
		})
	}

	// Sort by creation date (ascending)
	sortWorkspacesByCreatedAt(responses, workspacesDetails)

	return responses, nil
}

// getPermittedWorkspacesByUser returns workspaces where user has specific permission.
func (s *WorkspacePermissionFilterServiceImpl) getPermittedWorkspacesByUser(
	ctx context.Context,
	accountID string,
	organizationID string,
	permissionCode model.WorkspacePermissionCode,
) ([]*WorkspacePermissionResponse, error) {
	workspaces, err := s.organizationRepo.GetWorkspacesByOrganizationID(ctx, organizationID)
	if err != nil {
		logger.Error("Failed to get workspaces for organization", err)
		return nil, fmt.Errorf("failed to get workspaces for organization: %w", err)
	}

	if len(workspaces) == 0 {
		return []*WorkspacePermissionResponse{}, nil
	}

	// Check permissions for each workspace
	var permittedWorkspaces []*WorkspacePermissionResponse
	workspaceMap := make(map[string]*model.Workspace)
	customRoleIDs := make([]string, 0, len(workspaces))
	workspaceJoins := make(map[string]*model.WorkspaceMember, len(workspaces))

	for _, workspace := range workspaces {
		workspaceID := workspace.ID
		if workspace.Status != model.WorkspaceStatusNormal {
			continue
		}

		// Get workspace account join to check if user is member
		accountJoin, err := s.workspaceMemberRepo.GetByWorkspaceAndMember(ctx, workspaceID, accountID)
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			// Database error (not a "not found" error)
			logger.Error("Failed to get workspace account join", err)
			continue
		}
		if err != nil || accountJoin == nil {
			// User is not a member of this workspace
			continue
		}

		workspaceJoins[workspaceID] = accountJoin
		if accountJoin.RoleID != nil && *accountJoin.RoleID != "" && !model.IsBuiltinRole(*accountJoin.RoleID) {
			customRoleIDs = append(customRoleIDs, *accountJoin.RoleID)
		}
	}

	rolePermissions, err := s.listActiveRolePermissionsByID(ctx, organizationID, customRoleIDs)
	if err != nil {
		return nil, err
	}

	for _, workspace := range workspaces {
		accountJoin := workspaceJoins[workspace.ID]
		if accountJoin == nil {
			continue
		}
		if !workspaceRoleAllowsPermission(accountJoin.Role, accountJoin.RoleID, rolePermissions, permissionCode) {
			continue
		}

		workspaceMap[workspace.ID] = workspace
		permittedWorkspaces = append(permittedWorkspaces, &WorkspacePermissionResponse{
			ID:   workspace.ID,
			Name: workspace.Name,
		})
	}

	// Sort by creation date (ascending)
	sortWorkspacesByCreatedAtFromMap(permittedWorkspaces, workspaceMap)

	return permittedWorkspaces, nil
}

func workspacePermissionCodeForFilterType(permissionType string) (model.WorkspacePermissionCode, error) {
	switch permissionType {
	case "create_agent":
		return model.WorkspacePermissionAgentManage, nil
	case "create_database":
		return model.WorkspacePermissionDatabaseManage, nil
	case "create_knowledge":
		return model.WorkspacePermissionKnowledgeBaseManage, nil
	default:
		return "", fmt.Errorf("invalid permission type: %s", permissionType)
	}
}

func (s *WorkspacePermissionFilterServiceImpl) listActiveRolePermissionsByID(
	ctx context.Context,
	organizationID string,
	roleIDs []string,
) (map[string][]string, error) {
	permissionsByRoleID := make(map[string][]string)
	if len(roleIDs) == 0 {
		return permissionsByRoleID, nil
	}

	slices.Sort(roleIDs)
	roleIDs = slices.Compact(roleIDs)

	var roles []model.WorkspaceCustomRole
	if err := s.organizationRepo.GetDB().WithContext(ctx).
		Where("group_id = ? AND status = ? AND id IN ?", organizationID, model.WorkspaceCustomRoleStatusActive, roleIDs).
		Find(&roles).Error; err != nil {
		return nil, fmt.Errorf("failed to list workspace roles: %w", err)
	}

	for _, role := range roles {
		permissionsByRoleID[role.ID] = role.Permissions
	}

	return permissionsByRoleID, nil
}

// isOwnerOrAdmin checks if role is owner or admin
func isOwnerOrAdmin(role model.OrganizationRole) bool {
	return role == model.OrganizationRoleOwner || role == model.OrganizationRoleAdmin
}

// sortWorkspacesByCreatedAt sorts responses by workspace creation date
func sortWorkspacesByCreatedAt(responses []*WorkspacePermissionResponse, workspaces []*model.Workspace) {
	// Create a map for quick lookup
	workspaceMap := make(map[string]*model.Workspace)
	for _, workspace := range workspaces {
		workspaceMap[workspace.ID] = workspace
	}

	// Sort responses by workspace creation date
	for i := 0; i < len(responses); i++ {
		for j := i + 1; j < len(responses); j++ {
			workspaceI := workspaceMap[responses[i].ID]
			workspaceJ := workspaceMap[responses[j].ID]
			if workspaceI != nil && workspaceJ != nil && workspaceJ.CreatedAt.Before(workspaceI.CreatedAt) {
				responses[i], responses[j] = responses[j], responses[i]
			}
		}
	}
}

// sortWorkspacesByCreatedAtFromMap sorts responses by workspace creation date using a map
func sortWorkspacesByCreatedAtFromMap(responses []*WorkspacePermissionResponse, workspaceMap map[string]*model.Workspace) {
	// Sort responses by workspace creation date
	for i := 0; i < len(responses); i++ {
		for j := i + 1; j < len(responses); j++ {
			workspaceI := workspaceMap[responses[i].ID]
			workspaceJ := workspaceMap[responses[j].ID]
			if workspaceI != nil && workspaceJ != nil && workspaceJ.CreatedAt.Before(workspaceI.CreatedAt) {
				responses[i], responses[j] = responses[j], responses[i]
			}
		}
	}
}
