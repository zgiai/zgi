package config

import (
	"os"
	"strings"
	"sync/atomic"
)

const ConnectorRetirementModeEnv = "ZGI_CONNECTOR_RETIREMENT_MODE"

// Connector retirement modes. auto (the default) retires legacy Connector
// storage when that loses nothing and otherwise keeps the native runtime
// running with retirement deferred; deferred always postpones it; strict
// refuses to migrate or start until an operator retires it.
const (
	ConnectorRetirementModeAuto     = "auto"
	ConnectorRetirementModeDeferred = "deferred"
	ConnectorRetirementModeStrict   = "strict"
)

// ConnectorRetirementMode returns the configured mode. An unrecognized value
// is treated as strict, so a typo stops the process instead of deferring.
//
// Processes load the configuration before running migrations or serving, so
// GlobalConfig is the normal source. Without it (for example in package tests)
// the process environment is read directly instead of forcing a full Load,
// which would fail closed on unrelated required settings.
func ConnectorRetirementMode() string {
	value := ""
	if GlobalConfig != nil {
		value = GetString(ConnectorRetirementModeEnv, "")
	} else {
		value = os.Getenv(ConnectorRetirementModeEnv)
	}
	switch mode := strings.ToLower(strings.TrimSpace(value)); mode {
	case "", ConnectorRetirementModeAuto:
		return ConnectorRetirementModeAuto
	case ConnectorRetirementModeDeferred, ConnectorRetirementModeStrict:
		return mode
	default:
		return ConnectorRetirementModeStrict
	}
}

// ConnectorRetirementDeferred reports an explicit deferred mode.
func ConnectorRetirementDeferred() bool {
	return ConnectorRetirementMode() == ConnectorRetirementModeDeferred
}

// ConnectorRetirementAdaptive reports the default auto mode.
func ConnectorRetirementAdaptive() bool {
	return ConnectorRetirementMode() == ConnectorRetirementModeAuto
}

var connectorRetirementDeferredDetected atomic.Bool

// MarkConnectorRetirementDeferred records that this process found its
// database running with retirement deferred in auto mode.
func MarkConnectorRetirementDeferred() {
	connectorRetirementDeferredDetected.Store(true)
}

// ConnectorRetirementDeferredActive reports whether retirement is deferred
// for this process, explicitly or as detected in auto mode.
func ConnectorRetirementDeferredActive() bool {
	return ConnectorRetirementDeferred() || connectorRetirementDeferredDetected.Load()
}
