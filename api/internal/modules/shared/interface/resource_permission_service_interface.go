package interfaces

import (
	"context"

	"github.com/zgiai/zgi/api/internal/modules/workspace/model"
)

// ResourcePermissionService provides permission checking logic for resources
type ResourcePermissionService interface {
	// CheckSingleResourceEditPermission checks if a user can edit a single resource
	CheckSingleResourceEditPermission(ctx context.Context, params SingleResourcePermissionParams) (bool, error)

	// CheckBatchResourceEditPermission checks edit permissions for multiple resources
	CheckBatchResourceEditPermission(ctx context.Context, params BatchResourcePermissionParams) (map[string]bool, error)
}

// SingleResourcePermissionParams parameters for single resource permission check
type SingleResourcePermissionParams struct {
	AccountID       string                          // Current user's account ID
	TenantID        string                          // Legacy compatibility alias for the resource workspace ID
	OrganizationID  string                          // Resource organization ID
	CreatedBy       string                          // Resource creator's account ID
	GroupID         *string                         // Optional legacy compatibility alias for the resource organization ID
	PermissionCodes []model.WorkspacePermissionCode // Workspace permissions that allow editing/using this resource
}

// BatchResourcePermissionParams parameters for batch resource permission check
type BatchResourcePermissionParams struct {
	AccountID string                   // Current user's account ID
	Resources []ResourcePermissionInfo // List of resources to check
}

// ResourcePermissionInfo information about a resource for permission checking
type ResourcePermissionInfo struct {
	ResourceID      string                          // Unique identifier for the resource
	WorkspaceID     string                          // Resource's workspace ID
	OrganizationID  string                          // Resource organization ID
	CreatedBy       string                          // Resource creator's account ID
	GroupID         *string                         // Optional legacy compatibility alias for the resource organization ID
	PermissionCodes []model.WorkspacePermissionCode // Workspace permissions that allow editing/using this resource
}
