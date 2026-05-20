package service

import (
	"context"
	"fmt"

	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	"github.com/zgiai/zgi/api/internal/modules/shared/repository"
)

// resourcePermissionServiceImpl implements ResourcePermissionService
type resourcePermissionServiceImpl struct {
	permissionRepo repository.PermissionRepository
}

// NewResourcePermissionService creates a new resource permission service instance
func NewResourcePermissionService(permissionRepo repository.PermissionRepository) interfaces.ResourcePermissionService {
	return &resourcePermissionServiceImpl{
		permissionRepo: permissionRepo,
	}
}

// CheckSingleResourceEditPermission checks if a user can edit a single resource
func (s *resourcePermissionServiceImpl) CheckSingleResourceEditPermission(ctx context.Context, params interfaces.SingleResourcePermissionParams) (bool, error) {
	// Step 1: Quick check - Is user the creator?
	if params.AccountID == params.CreatedBy {
		return true, nil
	}

	// Step 2: Check organization-level permission (if organization compatibility scope exists)
	if params.GroupID != nil && *params.GroupID != "" {
		orgRole, err := s.permissionRepo.GetUserOrganizationRole(ctx, params.AccountID, *params.GroupID)
		if err != nil {
			return false, fmt.Errorf("failed to check organization role: %w", err)
		}

		if isAdminRole(orgRole) {
			return true, nil
		}
	}

	// Step 3: Check workspace-level permission using the compatibility alias
	workspaceRoles, err := s.permissionRepo.GetUserRolesForTenants(ctx, params.AccountID, []string{params.TenantID})
	if err != nil {
		return false, fmt.Errorf("failed to check workspace role: %w", err)
	}

	workspaceRole, exists := workspaceRoles[params.TenantID]
	if exists && isAdminRole(workspaceRole) {
		return true, nil
	}

	return false, nil
}

// CheckBatchResourceEditPermission checks edit permissions for multiple resources
func (s *resourcePermissionServiceImpl) CheckBatchResourceEditPermission(ctx context.Context, params interfaces.BatchResourcePermissionParams) (map[string]bool, error) {
	result := make(map[string]bool, len(params.Resources))

	// Quick pass: Mark resources created by the user as editable
	remainingResources := make([]interfaces.ResourcePermissionInfo, 0)
	for _, resource := range params.Resources {
		if params.AccountID == resource.CreatedBy {
			result[resource.ResourceID] = true
		} else {
			remainingResources = append(remainingResources, resource)
		}
	}

	// If all resources are created by the user, we're done
	if len(remainingResources) == 0 {
		return result, nil
	}

	// Collect unique workspace IDs and organization IDs
	uniqueWorkspaceIDs := make(map[string]bool)
	uniqueOrganizationIDs := make(map[string]bool)
	for _, resource := range remainingResources {
		workspaceID := resource.WorkspaceID
		uniqueWorkspaceIDs[workspaceID] = true
		if resource.GroupID != nil && *resource.GroupID != "" {
			uniqueOrganizationIDs[*resource.GroupID] = true
		}
	}

	// Convert maps to slices
	workspaceIDList := make([]string, 0, len(uniqueWorkspaceIDs))
	for workspaceID := range uniqueWorkspaceIDs {
		workspaceIDList = append(workspaceIDList, workspaceID)
	}

	organizationIDList := make([]string, 0, len(uniqueOrganizationIDs))
	for organizationID := range uniqueOrganizationIDs {
		organizationIDList = append(organizationIDList, organizationID)
	}

	// Batch query: Fetch user roles for all unique workspace IDs
	workspaceRoles, err := s.permissionRepo.GetUserRolesForTenants(ctx, params.AccountID, workspaceIDList)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch workspace roles: %w", err)
	}

	// Batch query: Fetch user roles for all unique organization IDs
	var orgRoles map[string]string
	if len(organizationIDList) > 0 {
		orgRoles, err = s.permissionRepo.GetUserOrganizationRoles(ctx, params.AccountID, organizationIDList)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch organization roles: %w", err)
		}
	} else {
		orgRoles = make(map[string]string)
	}

	// Check permissions for remaining resources using cached roles
	for _, resource := range remainingResources {
		canEdit := false

		// Check organization-level permission first
		if resource.GroupID != nil && *resource.GroupID != "" {
			if orgRole, exists := orgRoles[*resource.GroupID]; exists && isAdminRole(orgRole) {
				canEdit = true
			}
		}

		// If not org admin, check workspace-level permission
		if !canEdit {
			workspaceID := resource.WorkspaceID
			if workspaceRole, exists := workspaceRoles[workspaceID]; exists && isAdminRole(workspaceRole) {
				canEdit = true
			}
		}

		result[resource.ResourceID] = canEdit
	}

	return result, nil
}

// isAdminRole checks if a role is an admin role (owner or admin)
func isAdminRole(role string) bool {
	return role == "owner" || role == "admin"
}
