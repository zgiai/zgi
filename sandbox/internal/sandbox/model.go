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
	ID                       string            `json:"id"`
	RuntimeProfile           RuntimeProfile    `json:"runtime_profile"`
	Status                   Status            `json:"status"`
	CreatedAt                time.Time         `json:"created_at"`
	UpdatedAt                time.Time         `json:"updated_at"`
	ExpiresAt                time.Time         `json:"expires_at"`
	RootPath                 string            `json:"root_path"`
	Metadata                 map[string]string `json:"metadata,omitempty"`
	OrganizationID           string            `json:"organization_id,omitempty"`
	WorkspaceID              string            `json:"workspace_id,omitempty"`
	AppID                    string            `json:"app_id,omitempty"`
	WorkflowRunID            string            `json:"workflow_run_id,omitempty"`
	UserID                   string            `json:"user_id,omitempty"`
	NetworkEnabled           bool              `json:"network_enabled"`
	NetworkPolicy            string            `json:"network_policy"`
	DependencyProfile        string            `json:"dependency_profile"`
	DependencyProfileVersion string            `json:"dependency_profile_version"`
	WorkspaceBinding         string            `json:"workspace_binding,omitempty"`
	TTLSeconds               int               `json:"ttl_seconds"`
	WorkerID                 string            `json:"worker_id,omitempty"`
	WorkerAddr               string            `json:"worker_addr,omitempty"`
	EffectiveLimits          *ResourceLimits   `json:"effective_limits,omitempty"`
}

type ResourceLimits struct {
	RuntimeBackend                             string `json:"runtime_backend"`
	NetworkPolicyEnforced                      bool   `json:"network_policy_enforced"`
	MaxWorkers                                 int    `json:"max_workers"`
	MaxActiveSandboxes                         int    `json:"max_active_sandboxes"`
	MaxConcurrentExecutions                    int    `json:"max_concurrent_executions"`
	MaxConcurrentExecutionsPerProfile          int    `json:"max_concurrent_executions_per_profile"`
	MaxActiveSandboxesPerOrganization          int    `json:"max_active_sandboxes_per_organization"`
	MaxConcurrentExecutionsPerOrganization     int    `json:"max_concurrent_executions_per_organization"`
	MaxExecutionsPerMinutePerOrganization      int    `json:"max_executions_per_minute_per_organization"`
	MaxQueuedExecutionsPerOrganization         int    `json:"max_queued_executions_per_organization"`
	MaxWorkspaceFiles                          int    `json:"max_workspace_files"`
	MaxWorkspaceBytes                          int64  `json:"max_workspace_bytes"`
	MaxWorkspaceBytesPerOrganization           int64  `json:"max_workspace_bytes_per_organization"`
	QueueTimeoutMS                             int    `json:"queue_timeout_ms"`
	DefaultTimeoutSeconds                      int    `json:"default_timeout"`
	DefaultExecutionTimeoutMS                  int64  `json:"default_execution_timeout_ms"`
	OutputLimitKB                              int    `json:"output_limit_kb"`
	MaxCommandTimeoutMS                        int64  `json:"max_command_timeout_ms"`
	MaxCommandTimeoutSeconds                   int    `json:"max_command_timeout_secs"`
	OutputLimitBytes                           int    `json:"output_limit_bytes"`
	MaxFileSizeKB                              int    `json:"max_file_size_kb"`
	MaxFileSizeBytes                           int64  `json:"max_file_size_bytes"`
	MaxArchiveFiles                            int    `json:"max_archive_files"`
	MaxArchiveTotalBytes                       int64  `json:"max_archive_total_bytes"`
	MaxArtifactManifestFiles                   int    `json:"max_artifact_manifest_files"`
	MaxArtifactManifestTotalBytes              int64  `json:"max_artifact_manifest_total_bytes"`
	MaxArtifactManifestBytes                   int64  `json:"max_artifact_manifest_bytes"`
	MaxArtifactBytesPerOrganization            int64  `json:"max_artifact_bytes_per_organization"`
	MaxDependencyProfilesPerOrganization       int    `json:"max_dependency_profiles_per_organization"`
	SessionTTLSecs                             int    `json:"session_ttl_secs"`
	SessionTTLSeconds                          int    `json:"session_ttl_seconds"`
	InteractiveTTLSecs                         int    `json:"interactive_ttl_secs"`
	InteractiveTTLSeconds                      int    `json:"interactive_ttl_seconds"`
	MaxCompatTTLSecs                           int    `json:"max_compat_ttl_secs"`
	MaxCompatTTLSeconds                        int    `json:"max_compat_ttl_seconds"`
	DependencyUpdatesLocked                    bool   `json:"dependency_updates_locked"`
	WorkspaceFileLimitEnforced                 bool   `json:"workspace_file_limit_enforced"`
	WorkspaceByteLimitEnforced                 bool   `json:"workspace_byte_limit_enforced"`
	OrganizationWorkspaceByteLimitEnforced     bool   `json:"organization_workspace_byte_limit_enforced"`
	OrganizationArtifactByteLimitEnforced      bool   `json:"organization_artifact_byte_limit_enforced"`
	OrganizationDependencyProfileLimitEnforced bool   `json:"organization_dependency_profile_limit_enforced"`
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
