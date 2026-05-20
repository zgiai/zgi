package model

import "time"

// ============================================
// Common Response Types
// ============================================

// ErrorResponse represents an error response from plugin runner
type ErrorResponse struct {
	Error string `json:"error"`
}

// StatusResponse represents a simple status response
type StatusResponse struct {
	Status string `json:"status"`
}

// ============================================
// Plugin Management Types
// ============================================

// PluginRunner represents the runner configuration for a plugin
type PluginRunner struct {
	Language   string `json:"language"`
	Entrypoint string `json:"entrypoint"`
}

// PluginManifest represents the manifest for a plugin
type PluginManifest struct {
	Name                 string       `json:"name"`
	Version              string       `json:"version"`
	Description          string       `json:"description,omitempty"`
	Author               string       `json:"author,omitempty"`
	Tags                 []string     `json:"tags,omitempty"`
	Runner               PluginRunner `json:"runner"`
	MarketplacePluginID  string       `json:"marketplace_plugin_id,omitempty"`
	MarketplaceVersionID string       `json:"marketplace_version_id,omitempty"`
}

// Plugin represents a registered plugin
type Plugin struct {
	ID      string       `json:"id"`
	Name    string       `json:"name"`
	Version string       `json:"version"`
	Runner  PluginRunner `json:"runner"`
}

// RegisterPluginRequest represents the request to register a plugin
type RegisterPluginRequest struct {
	Manifest PluginManifest `json:"manifest"`
}

// InstallPluginRequest represents the request to install a plugin (JSON mode)
type InstallPluginRequest struct {
	PackageBase64 string `json:"package_b64"`
	Force         bool   `json:"force"`
}

// Installation represents an installed plugin
type Installation struct {
	Manifest        PluginManifest `json:"manifest"`
	Path            string         `json:"path"`
	InstalledAt     time.Time      `json:"installed_at"`
	PackageChecksum string         `json:"package_checksum,omitempty"`
}

// ============================================
// Session Management Types
// ============================================

// SessionStatus represents the status of a session
type SessionStatus string

const (
	SessionStatusLaunching SessionStatus = "launching"
	SessionStatusRunning   SessionStatus = "running"
	SessionStatusExited    SessionStatus = "exited"
	SessionStatusFailed    SessionStatus = "failed"
)

// SessionLogEntry represents a log entry from a session
type SessionLogEntry struct {
	Timestamp string `json:"timestamp"`
	Stream    string `json:"stream"` // stdout or stderr
	Line      string `json:"line"`
}

// StartSessionRequest represents the request to start a session
type StartSessionRequest struct {
	Name       string            `json:"name"`
	Version    string            `json:"version"`
	Language   string            `json:"language,omitempty"`
	Entrypoint string            `json:"entrypoint"`
	WorkingDir string            `json:"working_dir,omitempty"`
	Args       []string          `json:"args,omitempty"`
	Env        map[string]string `json:"env,omitempty"`
	TenantID   uint              `json:"tenant_id,omitempty"`
}

// SessionMetadata carries optional lifecycle metadata echoed by runner.
type SessionMetadata struct {
	WorkflowRunID             string `json:"workflow_run_id,omitempty"`
	SessionPolicy             string `json:"session_policy,omitempty"`
	SessionIdleTTLSeconds     int    `json:"session_idle_ttl_seconds,omitempty"`
	SessionMaxLifetimeSeconds int    `json:"session_max_lifetime_seconds,omitempty"`
}

// Session represents a plugin session
type Session struct {
	ID             string            `json:"id"`
	Manifest       PluginManifest    `json:"manifest"`
	Status         SessionStatus     `json:"status"`
	StartedAt      time.Time         `json:"started_at"`
	FinishedAt     *time.Time        `json:"finished_at,omitempty"`
	PID            int               `json:"pid,omitempty"`
	Logs           []SessionLogEntry `json:"logs,omitempty"`
	Metadata       *SessionMetadata  `json:"metadata,omitempty"`
	LastActivityAt *time.Time        `json:"last_activity_at,omitempty"`
}

// ============================================
// Invoke API Types
// ============================================

// InvokeRequest represents a generic invoke request
type InvokeRequest struct {
	SessionID  string                 `json:"session_id"`
	Action     string                 `json:"action"`
	Provider   string                 `json:"provider,omitempty"`
	Name       string                 `json:"name,omitempty"`
	Parameters map[string]interface{} `json:"parameters,omitempty"`
	Timeout    int                    `json:"timeout,omitempty"`
}

// InvokeResponse represents a generic invoke response
type InvokeResponse struct {
	RequestID string                 `json:"request_id"`
	Success   bool                   `json:"success"`
	Data      map[string]interface{} `json:"data,omitempty"`
	Error     string                 `json:"error,omitempty"`
}

// ToolInvokeRequest represents a tool invoke request
type ToolInvokeRequest struct {
	SessionID  string                 `json:"session_id"`
	Provider   string                 `json:"provider"`
	Tool       string                 `json:"tool"`
	Parameters map[string]interface{} `json:"parameters,omitempty"`
	Timeout    int                    `json:"timeout,omitempty"`
	WaitMode   string                 `json:"wait_mode,omitempty"`
	StreamMode string                 `json:"stream_mode,omitempty"`
}

// SessionReadyResponse represents the session ready check response
type SessionReadyResponse struct {
	SessionID string        `json:"session_id"`
	Ready     bool          `json:"ready"`
	Status    SessionStatus `json:"status"`
}

// ============================================
// Multi-Tenant Types
// ============================================

// Deprecated: tenant bindings are legacy and will be removed.
// CreateTenantRequest represents the request to create a tenant.
type CreateTenantRequest struct {
	Name string `json:"name"`
}

// Deprecated: tenant bindings are legacy and will be removed.
// Tenant represents a tenant.
type Tenant struct {
	ID        uint      `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

// Deprecated: tenant bindings are legacy and will be removed.
// PluginTenantBinding represents a plugin-tenant binding.
type PluginTenantBinding struct {
	TenantID   uint      `json:"tenant_id"`
	PluginID   uint      `json:"plugin_id"`
	Enabled    bool      `json:"enabled"`
	ConfigJSON string    `json:"config_json,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}

// Deprecated: tenant bindings are legacy and will be removed.
// TenantConfig represents tenant-specific configuration.
type TenantConfig struct {
	APIKey   string `json:"api_key,omitempty"`
	Endpoint string `json:"endpoint,omitempty"`
}

// ============================================
// Health Check Types
// ============================================

// HealthResponse represents the health check response
type HealthResponse struct {
	Status string `json:"status"`
}
