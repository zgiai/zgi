package runtime

import (
	"context"
	"time"

	"plugin_runner/internal/plugin"
)

// Runtime describes the behaviour of a plugin runtime implementation.
type Runtime interface {
	Start(ctx context.Context, req StartRequest) (*Session, error)
}

// StartRequest contains everything the runtime needs to boot a plugin process.
type StartRequest struct {
	Manifest   plugin.Manifest
	WorkingDir string
	Env        map[string]string
	Args       []string
}

// SessionStatus enumerates the lifecycle states of a plugin process.
type SessionStatus string

const (
	SessionStatusLaunching SessionStatus = "launching"
	SessionStatusRunning   SessionStatus = "running"
	SessionStatusExited    SessionStatus = "exited"
	SessionStatusFailed    SessionStatus = "failed"
)

// SessionPolicy controls how a caller expects the session to be managed.
type SessionPolicy string

const (
	SessionPolicyNoReuse        SessionPolicy = "no_reuse"
	SessionPolicyReuseWithinRun SessionPolicy = "reuse_within_run"
)

// LogLine represents one line of STDOUT/STDERR captured from the plugin.
type LogLine struct {
	Timestamp time.Time `json:"timestamp"`
	Stream    string    `json:"stream"`
	Line      string    `json:"line"`
}

// SessionMetadata carries optional caller-provided lifecycle hints.
type SessionMetadata struct {
	WorkflowRunID             string `json:"workflow_run_id,omitempty"`
	SessionPolicy             string `json:"session_policy,omitempty"`
	SessionIdleTTLSeconds     int    `json:"session_idle_ttl_seconds,omitempty"`
	SessionMaxLifetimeSeconds int    `json:"session_max_lifetime_seconds,omitempty"`
}

// Snapshot is a read-only view of a session.
type Snapshot struct {
	ID             string           `json:"id"`
	Manifest       plugin.Manifest  `json:"manifest"`
	WorkingDir     string           `json:"working_dir"`
	Status         SessionStatus    `json:"status"`
	StartedAt      time.Time        `json:"started_at"`
	FinishedAt     *time.Time       `json:"finished_at,omitempty"`
	PID            int              `json:"pid"`
	Error          string           `json:"error,omitempty"`
	Logs           []LogLine        `json:"logs"`
	Metadata       *SessionMetadata `json:"metadata,omitempty"`
	LastActivityAt *time.Time       `json:"last_activity_at,omitempty"`
}
