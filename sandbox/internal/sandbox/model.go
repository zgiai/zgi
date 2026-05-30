package sandbox

import "time"

type RuntimeProfile string

const (
	RuntimeLite        RuntimeProfile = "lite"
	RuntimeSession     RuntimeProfile = "session"
	RuntimeInteractive RuntimeProfile = "interactive"
)

type Status string

const (
	StatusActive  Status = "active"
	StatusExpired Status = "expired"
	StatusDeleted Status = "deleted"
)

type Sandbox struct {
	ID                string            `json:"id"`
	RuntimeProfile    RuntimeProfile    `json:"runtime_profile"`
	Status            Status            `json:"status"`
	CreatedAt         time.Time         `json:"created_at"`
	UpdatedAt         time.Time         `json:"updated_at"`
	ExpiresAt         time.Time         `json:"expires_at"`
	RootPath          string            `json:"root_path"`
	Metadata          map[string]string `json:"metadata,omitempty"`
	TenantID          string            `json:"tenant_id,omitempty"`
	WorkspaceID       string            `json:"workspace_id,omitempty"`
	AppID             string            `json:"app_id,omitempty"`
	WorkflowRunID     string            `json:"workflow_run_id,omitempty"`
	UserID            string            `json:"user_id,omitempty"`
	NetworkEnabled    bool              `json:"network_enabled"`
	NetworkPolicy     string            `json:"network_policy"`
	DependencyProfile string            `json:"dependency_profile"`
	WorkspaceBinding  string            `json:"workspace_binding,omitempty"`
	TTLSeconds        int               `json:"ttl_seconds"`
	WorkerID          string            `json:"worker_id,omitempty"`
	WorkerAddr        string            `json:"worker_addr,omitempty"`
	EffectiveLimits   *ResourceLimits   `json:"effective_limits,omitempty"`
}

type ResourceLimits struct {
	RuntimeBackend              string `json:"runtime_backend"`
	NetworkPolicyEnforced       bool   `json:"network_policy_enforced"`
	MaxWorkers                  int    `json:"max_workers"`
	MaxActiveSandboxes          int    `json:"max_active_sandboxes"`
	MaxActiveSandboxesPerTenant int    `json:"max_active_sandboxes_per_tenant"`
	QueueTimeoutMS              int    `json:"queue_timeout_ms"`
	DefaultTimeoutSeconds       int    `json:"default_timeout"`
	DefaultExecutionTimeoutMS   int64  `json:"default_execution_timeout_ms"`
	OutputLimitKB               int    `json:"output_limit_kb"`
	MaxCommandTimeoutMS         int64  `json:"max_command_timeout_ms"`
	MaxCommandTimeoutSeconds    int    `json:"max_command_timeout_secs"`
	OutputLimitBytes            int    `json:"output_limit_bytes"`
	MaxFileSizeKB               int    `json:"max_file_size_kb"`
	MaxFileSizeBytes            int64  `json:"max_file_size_bytes"`
	MaxArchiveFiles             int    `json:"max_archive_files"`
	MaxArchiveTotalBytes        int64  `json:"max_archive_total_bytes"`
	SessionTTLSecs              int    `json:"session_ttl_secs"`
	SessionTTLSeconds           int    `json:"session_ttl_seconds"`
	InteractiveTTLSecs          int    `json:"interactive_ttl_secs"`
	InteractiveTTLSeconds       int    `json:"interactive_ttl_seconds"`
	MaxCompatTTLSecs            int    `json:"max_compat_ttl_secs"`
	MaxCompatTTLSeconds         int    `json:"max_compat_ttl_seconds"`
	DependencyUpdatesLocked     bool   `json:"dependency_updates_locked"`
	WorkspaceByteLimitEnforced  bool   `json:"workspace_byte_limit_enforced"`
}

type Endpoint struct {
	SandboxID  string    `json:"sandbox_id"`
	Port       string    `json:"port"`
	URL        string    `json:"url"`
	Status     string    `json:"status"`
	TargetHost string    `json:"target_host,omitempty"`
	TargetPort int       `json:"target_port,omitempty"`
	Scheme     string    `json:"scheme,omitempty"`
	CreatedAt  time.Time `json:"created_at,omitempty"`
	UpdatedAt  time.Time `json:"updated_at,omitempty"`
}
