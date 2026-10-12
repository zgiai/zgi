package config

import "strings"

const ConnectorRetirementModeEnv = "ZGI_CONNECTOR_RETIREMENT_MODE"

// ConnectorRetirementDeferred keeps the native Connector runtime available
// while the destructive legacy-table retirement is postponed. It is opt-in;
// the normal mode still requires schema2 readiness.
func ConnectorRetirementDeferred() bool {
	return strings.EqualFold(strings.TrimSpace(GetString(ConnectorRetirementModeEnv, "")), "deferred")
}
