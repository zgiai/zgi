package repository

import (
	"context"
	"fmt"

	"gorm.io/gorm"
)

// PermissionRepository provides data access methods for permission-related queries
type PermissionRepository interface {
	// GetUserRolesForTenants fetches user roles for multiple tenants in a single query
	GetUserRolesForTenants(ctx context.Context, accountID string, tenantIDs []string) (map[string]string, error)

	// GetUserOrganizationRole fetches user's role in an organization
	GetUserOrganizationRole(ctx context.Context, accountID string, groupID string) (string, error)

	// GetUserOrganizationRoles fetches user roles for multiple organizations in a single query
	GetUserOrganizationRoles(ctx context.Context, accountID string, groupIDs []string) (map[string]string, error)
}

// permissionRepositoryImpl implements PermissionRepository
type permissionRepositoryImpl struct {
	db *gorm.DB
}

// NewPermissionRepository creates a new permission repository instance
func NewPermissionRepository(db *gorm.DB) PermissionRepository {
	return &permissionRepositoryImpl{
		db: db,
	}
}

// GetUserRolesForTenants fetches user roles for multiple tenants in a single query
func (r *permissionRepositoryImpl) GetUserRolesForTenants(ctx context.Context, accountID string, workspaceIDs []string) (map[string]string, error) {
	if accountID == "" {
		return nil, fmt.Errorf("account_id cannot be empty")
	}

	if len(workspaceIDs) == 0 {
		return make(map[string]string), nil
	}

	type Result struct {
		TenantID string
		Role     string
	}

	var results []Result
	err := r.db.WithContext(ctx).
		Table("workspace_members").
		Select("workspace_id, role").
		Where("account_id = ? AND workspace_id IN ?", accountID, workspaceIDs).
		Scan(&results).Error

	if err != nil {
		return nil, fmt.Errorf("failed to fetch user roles for tenants: %w", err)
	}

	roleMap := make(map[string]string, len(results))
	for _, result := range results {
		roleMap[result.TenantID] = result.Role
	}

	return roleMap, nil
}

// GetUserOrganizationRole fetches user's role in an organization
func (r *permissionRepositoryImpl) GetUserOrganizationRole(ctx context.Context, accountID string, organizationID string) (string, error) {
	if accountID == "" {
		return "", fmt.Errorf("organization_id cannot be empty")
	}

	if organizationID == "" {
		return "", fmt.Errorf("organization_id cannot be empty")
	}

	var role string
	err := r.db.WithContext(ctx).
		Table("members").
		Select("role").
		Where("account_id = ? AND organization_id = ?", accountID, organizationID).
		Scan(&role).Error

	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return "", nil // User is not a member of this organization
		}
		return "", fmt.Errorf("failed to fetch user organization role: %w", err)
	}

	return role, nil
}

// GetUserOrganizationRoles fetches user roles for multiple organizations in a single query
func (r *permissionRepositoryImpl) GetUserOrganizationRoles(ctx context.Context, accountID string, organizationIDs []string) (map[string]string, error) {
	if accountID == "" {
		return nil, fmt.Errorf("account_id cannot be empty")
	}

	if len(organizationIDs) == 0 {
		return make(map[string]string), nil
	}

	type Result struct {
		OrganizationID string `gorm:"column:organization_id"`
		Role           string
	}

	var results []Result
	err := r.db.WithContext(ctx).
		Table("members").
		Select("organization_id, role").
		Where("account_id = ? AND organization_id IN ?", accountID, organizationIDs).
		Scan(&results).Error

	if err != nil {
		return nil, fmt.Errorf("failed to fetch user roles for organizations: %w", err)
	}

	roleMap := make(map[string]string, len(results))
	for _, result := range results {
		roleMap[result.OrganizationID] = result.Role
	}

	return roleMap, nil
}
