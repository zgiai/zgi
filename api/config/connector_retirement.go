package config

import (
	"os"
	"strings"
)

const ConnectorRetirementModeEnv = "ZGI_CONNECTOR_RETIREMENT_MODE"

// ConnectorRetirementDeferred keeps the native Connector runtime available
// while the destructive legacy-table retirement is postponed. It is opt-in;
// the normal mode still requires schema2 readiness.
//
// Processes load the configuration before running migrations or serving, so
// GlobalConfig is the normal source. Without it (for example in package tests)
// the process environment is read directly instead of forcing a full Load,
// which would fail closed on unrelated required settings.
func ConnectorRetirementDeferred() bool {
	value := ""
	if GlobalConfig != nil {
		value = GetString(ConnectorRetirementModeEnv, "")
	} else {
		value = os.Getenv(ConnectorRetirementModeEnv)
	}
	return strings.EqualFold(strings.TrimSpace(value), "deferred")
}
