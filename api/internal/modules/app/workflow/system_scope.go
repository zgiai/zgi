package workflow

import "strings"

const builtInWorkflowTenantID = "00000000-0000-0000-0000-000000000000"

func isSystemWorkflowTenantID(tenantID string) bool {
	tenantID = strings.TrimSpace(tenantID)
	return tenantID == "" || tenantID == builtInWorkflowTenantID
}
