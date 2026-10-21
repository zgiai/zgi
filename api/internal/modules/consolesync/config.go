// Package consolesync implements the pre side of the Console control plane.
//
// It runs only in Cloud mode (ZGI_RUN_MODE=cloud) and only when enabled. It polls the
// console for commands and desired state over outbound HTTPS, applies them idempotently,
// acknowledges the result, reports the real local state back, and cuts the official
// channel when the console link goes stale.
//
// Nothing in this package touches the billing, pricing or gateway packages: state is
// written with raw SQL through its own repository so the plan stays merge-safe against
// codex/video-image-rollback-main.
package consolesync

import (
	"strings"
	"time"

	"github.com/zgiai/zgi/api/config"
)

// Default cadence values. The plan allows 30-60s normally and 2-10s while an
// immediate-priority command is outstanding.
const (
	DefaultIntervalSec = 45
	DefaultStaleSec    = 300
	DefaultHTTPTimeout = 15 * time.Second
	DefaultReconcile   = 10 * time.Minute

	// DefaultOrganizationReportInterval is how often the organization inventory is
	// pushed to console (plan P0). CONSOLE_SYNC_ORGANIZATION_REPORT_INTERVAL overrides
	// it; 0 disables the report entirely.
	DefaultOrganizationReportInterval = 5 * time.Minute

	// OrganizationReportTimeout bounds one inventory report so a slow console can never
	// hold the report goroutine (or shutdown) for long. The shared HTTP client timeout
	// is usually tighter; this context is the outer backstop.
	OrganizationReportTimeout = 30 * time.Second

	// syncStateSingletonID is the primary key of the single pre_sync_state row.
	syncStateSingletonID = 1
)

// Config is the resolved control-plane configuration for this process.
type Config struct {
	Enabled        bool
	Edition        string
	BaseURL        string
	InstanceID     string
	InstanceSecret string
	Interval       time.Duration
	StaleAfter     time.Duration
	HTTPTimeout    time.Duration
	ReconcileEvery time.Duration
	// OrganizationReportInterval is the organization inventory report cadence.
	// Values <= 0 disable the report.
	OrganizationReportInterval time.Duration
}

// ConfigFromApp resolves the sync configuration from the application config.
func ConfigFromApp(cfg *config.Config) Config {
	resolved := Config{
		Interval:                   DefaultIntervalSec * time.Second,
		StaleAfter:                 DefaultStaleSec * time.Second,
		HTTPTimeout:                DefaultHTTPTimeout,
		ReconcileEvery:             DefaultReconcile,
		OrganizationReportInterval: DefaultOrganizationReportInterval,
	}
	if cfg == nil {
		return resolved
	}

	resolved.Edition = strings.ToUpper(strings.TrimSpace(cfg.Platform.Edition))
	resolved.BaseURL = strings.TrimRight(strings.TrimSpace(cfg.Console.APIURL), "/")
	resolved.InstanceID = strings.TrimSpace(cfg.Console.InstanceID)
	resolved.InstanceSecret = strings.TrimSpace(cfg.Console.InstanceSecret)
	resolved.Enabled = cfg.Console.SyncEnabled
	// loadConsoleConfig supplies the 5m default, but 0 is an explicit "disabled", so the
	// value is copied verbatim instead of being defaulted again here.
	resolved.OrganizationReportInterval = cfg.Console.SyncOrganizationReportInterval

	if cfg.Console.SyncIntervalSec > 0 {
		resolved.Interval = time.Duration(cfg.Console.SyncIntervalSec) * time.Second
	}
	if cfg.Console.SyncStaleSec > 0 {
		resolved.StaleAfter = time.Duration(cfg.Console.SyncStaleSec) * time.Second
	}
	// A stale window shorter than the poll cadence would flap.
	if resolved.StaleAfter <= resolved.Interval {
		resolved.StaleAfter = resolved.Interval * 2
	}
	return resolved
}

// IsCloud reports whether the process runs in Cloud mode.
func (c Config) IsCloud() bool { return c.Edition == "CLOUD" }

// ShouldStart reports whether the sync loop must run.
//
// SELF_HOSTED mode is untouched: the loop never starts, so behaviour is identical to
// before this feature existed.
func (c Config) ShouldStart() bool {
	return c.IsCloud() && c.Enabled && c.BaseURL != "" && c.InstanceID != "" && c.InstanceSecret != ""
}

// Reason explains why the loop is not starting; empty when it will start.
func (c Config) Reason() string {
	switch {
	case !c.IsCloud():
		return "platform edition is not CLOUD"
	case !c.Enabled:
		return "CONSOLE_SYNC_ENABLED is false"
	case c.BaseURL == "":
		return "CONSOLE_API_URL is empty"
	case c.InstanceID == "":
		return "CONSOLE_INSTANCE_ID is empty"
	case c.InstanceSecret == "":
		return "CONSOLE_INSTANCE_SECRET is empty"
	default:
		return ""
	}
}
