package agents

// PermissionContext contains all permission-related information for a user
// Used to determine which agents a user can access based on their organization
// and department memberships
type PermissionContext struct {
	// AccountID is the user's account identifier
	AccountID string

	// OrganizationID is the user's current organization (tenant with current=true)
	OrganizationID string

	// OrganizationRole is the user's role in their current organization (owner, admin, normal)
	OrganizationRole string

	// OrganizationDeptIDs contains all department IDs that belong to the user's organization
	OrganizationDeptIDs []string

	// UserDepartments contains the user's department memberships with their roles
	// Only populated for normal users (not org admins)
	UserDepartments []DepartmentMembership

	// ValidDepartmentIDs contains the intersection of OrganizationDeptIDs and user's department memberships
	// These are the departments the user actually has access to within their organization
	ValidDepartmentIDs []string

	// AdminDepartmentIDs contains department IDs where the user has admin or owner role
	AdminDepartmentIDs []string

	// NormalDepartmentIDs contains department IDs where the user has normal role
	NormalDepartmentIDs []string
}

// DepartmentMembership represents a user's membership in a specific department
type DepartmentMembership struct {
	// TenantID is the department identifier
	TenantID string

	// Role is the user's role in this department (owner, admin, normal)
	Role string
}
