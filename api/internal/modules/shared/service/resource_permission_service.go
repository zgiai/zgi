package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	workspace_model "github.com/zgiai/zgi/api/internal/modules/workspace/model"
)

// resourcePermissionServiceImpl implements ResourcePermissionService
type resourcePermissionServiceImpl struct {
	authorizationService interfaces.AuthorizationService
}

// NewResourcePermissionService creates a new resource permission service instance
func NewResourcePermissionService(authorizationService interfaces.AuthorizationService) interfaces.ResourcePermissionService {
	return &resourcePermissionServiceImpl{
		authorizationService: authorizationService,
	}
}

// CheckSingleResourceEditPermission checks if a user can edit a single resource
func (s *resourcePermissionServiceImpl) CheckSingleResourceEditPermission(ctx context.Context, params interfaces.SingleResourcePermissionParams) (bool, error) {
	// Step 1: Quick check - Is user the creator?
	if params.AccountID == params.CreatedBy {
		return true, nil
	}

	return s.checkWorkspacePermissions(ctx, params.AccountID, params.OrganizationID, params.TenantID, params.GroupID, params.PermissionCodes)
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

	for _, resource := range remainingResources {
		canEdit, err := s.checkWorkspacePermissions(
			ctx,
			params.AccountID,
			resource.OrganizationID,
			resource.WorkspaceID,
			resource.GroupID,
			resource.PermissionCodes,
		)
		if err != nil {
			return nil, err
		}
		result[resource.ResourceID] = canEdit
	}

	return result, nil
}

func (s *resourcePermissionServiceImpl) checkWorkspacePermissions(
	ctx context.Context,
	accountID string,
	organizationID string,
	workspaceID string,
	groupID *string,
	permissionCodes []workspace_model.WorkspacePermissionCode,
) (bool, error) {
	organizationID = resourceOrganizationID(ctx, organizationID, groupID)
	workspaceID = strings.TrimSpace(workspaceID)
	accountID = strings.TrimSpace(accountID)
	if accountID == "" || organizationID == "" || workspaceID == "" || len(permissionCodes) == 0 {
		return false, nil
	}
	if s.authorizationService == nil {
		return false, fmt.Errorf("authorization service is not initialized")
	}

	_, err := s.authorizationService.RequireWorkspacePermission(ctx, interfaces.WorkspaceScopeRequest{
		OrganizationID:  organizationID,
		WorkspaceID:     workspaceID,
		AccountID:       accountID,
		PermissionCodes: permissionCodes,
	})
	if errors.Is(err, ErrAuthorizationDenied) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("failed to check workspace permission: %w", err)
	}
	return true, nil
}

func resourceOrganizationID(ctx context.Context, organizationID string, groupID *string) string {
	if id := strings.TrimSpace(organizationID); id != "" {
		return id
	}
	if groupID != nil {
		if id := strings.TrimSpace(*groupID); id != "" {
			return id
		}
	}
	if v := ctx.Value("tenant_id"); v != nil {
		if id, ok := v.(string); ok {
			return strings.TrimSpace(id)
		}
	}
	return ""
}
