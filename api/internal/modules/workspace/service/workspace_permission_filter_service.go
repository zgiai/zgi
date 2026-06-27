package service

import (
	"context"
	"errors"
	"fmt"

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

	return s.getPermittedWorkspacesByUser(ctx, accountID, organizationID, permissionCode)
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
	}

	for _, workspace := range workspaces {
		accountJoin := workspaceJoins[workspace.ID]
		if accountJoin == nil {
			continue
		}
		if !workspaceMemberAllowsPermission(
			accountJoin.Role,
			accountJoin.RoleID,
			accountJoin.Permissions,
			accountJoin.PermissionSource,
			permissionCode,
		) {
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
		return model.WorkspacePermissionAgentCreate, nil
	case "create_database":
		return model.WorkspacePermissionDatabaseCreate, nil
	case "create_knowledge":
		return model.WorkspacePermissionKnowledgeBaseCreate, nil
	default:
		return "", fmt.Errorf("invalid permission type: %s", permissionType)
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
