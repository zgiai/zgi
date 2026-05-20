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
	NetworkEnabled    bool              `json:"network_enabled"`
	NetworkPolicy     string            `json:"network_policy"`
	DependencyProfile string            `json:"dependency_profile"`
	WorkspaceBinding  string            `json:"workspace_binding,omitempty"`
	TTLSeconds        int               `json:"ttl_seconds"`
	WorkerID          string            `json:"worker_id,omitempty"`
	WorkerAddr        string            `json:"worker_addr,omitempty"`
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
