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
// execution. An empty value preserves the legacy Agent console admission;
// malformed values are returned so callers can fail closed.
func EntryAdmissionFromEnv() (execution.EntryAdmission, error) {
	return execution.ParseEntryAdmission(os.Getenv(EntryAdmissionEnv))
}
