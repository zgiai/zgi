package agents

import (
	"context"
	"fmt"

	"github.com/zgiai/zgi/api/pkg/logger"
)

// buildPermissionContext builds a permission context for the given user
// This determines which agents the user can access based on their organization
// and department memberships
// Requirements: 1.1, 1.2, 1.4, 2.1, 3.1, 3.2, 3.3, 4.1, 4.2, 4.3, 11.2, 11.3, 11.5
func (s *agentsService) buildPermissionContext(ctx context.Context, accountID string) (*PermissionContext, error) {
	permCtx := &PermissionContext{
		AccountID: accountID,
	}

	// Step 1: Get current organization (tenant with current=true)
	// Requirement 1.1: Query members WHERE account_id matches
	// Requirement 11.3: Handle database errors (500) with logging
	currentOrganization, err := s.tenantService.GetCurrentOrganization(ctx, accountID)
	if err != nil {
		// Requirement 11.5: Add structured logging with context
		logger.Error(fmt.Sprintf("buildPermissionContext: Failed to get current tenant for account_id=%s", accountID), err)
		return nil, fmt.Errorf("failed to get current organization: %w", err)
	}

	// Requirement 1.3, 11.2: Return error if no current tenant found (404)
	if currentOrganization == nil {
		// Requirement 11.5: Add structured logging with context
		logger.Error(fmt.Sprintf("buildPermissionContext: No current tenant found for account_id=%s", accountID), nil)
		return nil, fmt.Errorf("no current organization found for user")
	}

	// Requirement 1.2: Extract organization ID and role
	permCtx.OrganizationID = currentOrganization.OrganizationID
	permCtx.OrganizationRole = string(currentOrganization.Role)

	// Requirement 1.4: Get all departments belonging to the organization
	// Requirement 11.3: Handle database errors (500) with logging
	orgDeptIDs, err := s.tenantService.GetWorkspaceIDsByOrganizationID(ctx, permCtx.OrganizationID)
	if err != nil {
		// Requirement 11.5: Add structured logging with context
		logger.Error(fmt.Sprintf("buildPermissionContext: Failed to get organization departments for account_id=%s, org_id=%s",
			accountID, permCtx.OrganizationID), err)
		return nil, fmt.Errorf("failed to get organization departments: %w", err)
	}
	permCtx.OrganizationDeptIDs = orgDeptIDs

	// Step 2: Check if user is organization admin/owner
	// Requirement 2.1: If role is owner or admin, return early with all departments
	if permCtx.OrganizationRole == "owner" || permCtx.OrganizationRole == "admin" {
		// Requirement 11.5: Add structured logging with context
		logger.Info("buildPermissionContext: User is org admin/owner", map[string]interface{}{
			"account_id":      accountID,
			"organization_id": permCtx.OrganizationID,
			"role":            permCtx.OrganizationRole,
			"dept_count":      len(orgDeptIDs),
		})
		// Org admins have access to all departments, no need to calculate intersections
		return permCtx, nil
	}

	// Step 3: For normal users, get department memberships and calculate intersection
	// Requirement 3.1: Query user's department memberships (current=false)
	// Requirement 11.3: Handle database errors (500) with logging
	userDepts, err := s.tenantService.GetUserWorkspaceMemberships(ctx, accountID)
	if err != nil {
		// Requirement 11.5: Add structured logging with context
		logger.Error(fmt.Sprintf("buildPermissionContext: Failed to get user departments for account_id=%s, org_id=%s",
			accountID, permCtx.OrganizationID), err)
		return nil, fmt.Errorf("failed to get user department memberships: %w", err)
	}

	// Convert interface type to local type
	permCtx.UserDepartments = make([]DepartmentMembership, len(userDepts))
	for i, dept := range userDepts {
		permCtx.UserDepartments[i] = DepartmentMembership{
			TenantID: dept.WorkspaceID,
			Role:     string(dept.Role),
		}
	}

	// Requirement 3.2, 3.3: Calculate intersection of organization departments and user departments
	orgDeptSet := make(map[string]bool)
	for _, deptID := range orgDeptIDs {
		orgDeptSet[deptID] = true
	}

	// Build valid department IDs (intersection) and separate by role
	// Requirement 4.3: Check department roles for each valid department
	for _, userDept := range permCtx.UserDepartments {
		// Check if user's department is in the organization
		if orgDeptSet[userDept.TenantID] {
			permCtx.ValidDepartmentIDs = append(permCtx.ValidDepartmentIDs, userDept.TenantID)

			// Requirement 4.1, 4.2: Separate departments by role
			if userDept.Role == "owner" || userDept.Role == "admin" {
				permCtx.AdminDepartmentIDs = append(permCtx.AdminDepartmentIDs, userDept.TenantID)
			} else {
				permCtx.NormalDepartmentIDs = append(permCtx.NormalDepartmentIDs, userDept.TenantID)
			}
		}
	}

	// Requirement 3.4: If user has no valid departments, they will see an empty list
	// (This is handled by the repository layer, not an error)
	// Requirement 11.5: Add structured logging with context
	logger.Info("buildPermissionContext: Permission context built for normal user", map[string]interface{}{
		"account_id":        accountID,
		"organization_id":   permCtx.OrganizationID,
		"valid_dept_count":  len(permCtx.ValidDepartmentIDs),
		"admin_dept_count":  len(permCtx.AdminDepartmentIDs),
		"normal_dept_count": len(permCtx.NormalDepartmentIDs),
		"valid_dept_ids":    permCtx.ValidDepartmentIDs,
		"admin_dept_ids":    permCtx.AdminDepartmentIDs,
		"normal_dept_ids":   permCtx.NormalDepartmentIDs,
	})

	return permCtx, nil
}
