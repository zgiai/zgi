package executionruntime

import (
	"os"

	execution "github.com/zgiai/zgi/api/internal/capabilities/agentexecution"
)

// EntryAdmissionEnv is the operator-owned admission setting shared by every
// execution entry. Keeping the environment lookup here prevents capability
// projections and runtime wiring from drifting apart.
const EntryAdmissionEnv = "ZGI_AGENT_RUNTIME_ENTRY_ALLOWLIST_JSON"

// EntryAdmissionFromEnv reads the same entry admission policy used by runtime
// execution. When ZGI_AGENT_RUNTIME_ORG_MODE admits every authenticated
// organization, every supported entry is admitted and the entry allowlist is
// not consulted, so one deployment setting opens the runtime to all
// organizations. Otherwise an empty value preserves the legacy Agent console
// admission and malformed values are returned so callers can fail closed.
func EntryAdmissionFromEnv() (execution.EntryAdmission, error) {
	if normalizeOrganizationAccessMode(os.Getenv(OrganizationAccessModeEnv)) == OrganizationAccessModeAllAuthenticated {
		return execution.AllOrganizationsEntryAdmission(), nil
	}
	return execution.ParseEntryAdmission(os.Getenv(EntryAdmissionEnv))
}
