// Package runtimeworkspace contains the server-side model and authentication
// boundary for Runtime Workspace external exposure groups.
package runtimeworkspace

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
)

const (
	StatusActive              = "active"
	StatusDisabled            = "disabled"
	ModeAttached              = "attached"
	ModeIsolated              = "isolated"
	CredentialActive          = "active"
	CredentialRevoked         = "revoked"
	CredentialOperationActive = "processing"
	CredentialOperationDone   = "succeeded"
	AgentBindingActive        = "active"
	AgentBindingDisabled      = "disabled"
	SubjectIssuerType         = "runtime_workspace_api"
)

type RuntimeWorkspace struct {
	// RuntimeID is the stable logical identity of the exposure group. In the
	// current attached mode it is initialized to WorkspaceID so existing rows
	// and child resources remain addressable without a data rewrite. Isolated
	// mode may use a different RuntimeID while WorkspaceID points at a hidden
	// resource workspace.
	RuntimeID          uuid.UUID `gorm:"column:runtime_id;type:uuid;not null;uniqueIndex" json:"runtime_id"`
	WorkspaceID        uuid.UUID `gorm:"type:uuid;primaryKey" json:"workspace_id"`
	OrganizationID     uuid.UUID `gorm:"type:uuid;not null;index" json:"organization_id"`
	ManagedByAccountID uuid.UUID `gorm:"type:uuid;not null;index" json:"managed_by_account_id"`
	Status             string    `gorm:"type:varchar(24);not null;default:'active';index" json:"status"`
	Mode               string    `gorm:"type:varchar(24);not null;default:'attached';index" json:"mode"`
	PolicyJSON         string    `gorm:"type:jsonb;not null;default:'{}'" json:"policy_json"`
	BindingsRevision   int64     `gorm:"not null;default:0" json:"bindings_revision"`
	InFlightRequests   int64     `gorm:"column:in_flight_requests;not null;default:0" json:"-"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

func (RuntimeWorkspace) TableName() string { return "runtime_workspaces" }

// LogicalID returns the stable Runtime Workspace identity. The fallback keeps
// callers safe while reading rows created before the identity migration.
func (r RuntimeWorkspace) LogicalID() uuid.UUID {
	if r.RuntimeID != uuid.Nil {
		return r.RuntimeID
	}
	return r.WorkspaceID
}

// ResourceWorkspaceID returns the underlying ZGI Workspace used by existing
// resource and permission services. It is intentionally separate from the
// logical Runtime Workspace identity so isolated mode can swap this resource
// without changing external Subjects or Runtime resource ownership.
func (r RuntimeWorkspace) ResourceWorkspaceID() uuid.UUID {
	return r.WorkspaceID
}

type Credential struct {
	ID                 uuid.UUID  `gorm:"type:uuid;primaryKey" json:"id"`
	RuntimeWorkspaceID uuid.UUID  `gorm:"type:uuid;not null;index" json:"runtime_workspace_id"`
	KeyPrefix          string     `gorm:"type:varchar(20);not null" json:"key_prefix"`
	KeyHash            string     `gorm:"type:varchar(64);not null;uniqueIndex" json:"-"`
	Name               string     `gorm:"type:varchar(255);not null;default:''" json:"name"`
	Status             string     `gorm:"type:varchar(20);not null;default:'active';index" json:"status"`
	ExpiresAt          *time.Time `json:"expires_at,omitempty"`
	UsageCount         int64      `gorm:"not null;default:0" json:"usage_count"`
	LastUsedAt         *time.Time `json:"last_used_at,omitempty"`
	RevokedAt          *time.Time `json:"revoked_at,omitempty"`
	CreatedByAccountID uuid.UUID  `gorm:"type:uuid;not null" json:"created_by_account_id"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
}

func (Credential) TableName() string { return "runtime_workspace_credentials" }

type CredentialOperation struct {
	ID                 uuid.UUID  `gorm:"type:uuid;primaryKey" json:"id"`
	RuntimeWorkspaceID uuid.UUID  `gorm:"type:uuid;not null;uniqueIndex:idx_runtime_credential_operation_key,priority:1" json:"runtime_workspace_id"`
	CreatedByAccountID uuid.UUID  `gorm:"type:uuid;not null;uniqueIndex:idx_runtime_credential_operation_key,priority:2" json:"created_by_account_id"`
	IdempotencyKeyHash string     `gorm:"type:varchar(64);not null;uniqueIndex:idx_runtime_credential_operation_key,priority:3" json:"-"`
	RequestFingerprint string     `gorm:"type:varchar(64);not null" json:"-"`
	State              string     `gorm:"type:varchar(20);not null;index" json:"state"`
	CredentialID       *uuid.UUID `gorm:"type:uuid;index" json:"credential_id,omitempty"`
	SecretEnvelope     *string    `gorm:"type:text" json:"-"`
	SecretExpiresAt    *time.Time `json:"secret_expires_at,omitempty"`
	ExpiresAt          time.Time  `gorm:"not null;index" json:"expires_at"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
}

func (CredentialOperation) TableName() string {
	return "runtime_workspace_credential_operations"
}

type AgentBinding struct {
	RuntimeWorkspaceID uuid.UUID `gorm:"type:uuid;primaryKey;uniqueIndex:idx_runtime_workspace_app_key,priority:1" json:"runtime_workspace_id"`
	AgentID            uuid.UUID `gorm:"type:uuid;primaryKey" json:"agent_id"`
	PublicAppKey       string    `gorm:"type:varchar(128);not null;uniqueIndex:idx_runtime_workspace_app_key,priority:2" json:"app_key"`
	Status             string    `gorm:"type:varchar(20);not null;default:'active';index" json:"status"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

func (AgentBinding) TableName() string { return "runtime_workspace_agents" }

type ExternalFile struct {
	FileID             string    `gorm:"type:uuid;primaryKey" json:"file_id"`
	RuntimeWorkspaceID uuid.UUID `gorm:"type:uuid;not null;index" json:"runtime_workspace_id"`
	WorkspaceID        uuid.UUID `gorm:"type:uuid;not null;index" json:"workspace_id"`
	OrganizationID     uuid.UUID `gorm:"type:uuid;not null;index" json:"organization_id"`
	SubjectID          uuid.UUID `gorm:"type:uuid;not null;index" json:"subject_id"`
	SubjectKind        string    `gorm:"type:varchar(32);not null" json:"subject_kind"`
	Surface            string    `gorm:"type:varchar(32);not null;default:'runtime_workspace_api'" json:"surface"`
	CreatedAt          time.Time `json:"created_at"`
}

func (ExternalFile) TableName() string { return "runtime_external_files" }

// AccessEvent is the persisted audit envelope for an authenticated Runtime
// Workspace request. It intentionally stores only stable internal IDs and
// transport metadata; raw API keys, external user IDs, request bodies, and
// file or memory content must never be written to this table.
type AccessEvent struct {
	ID                 uuid.UUID      `gorm:"type:uuid;primaryKey" json:"id"`
	RuntimeWorkspaceID uuid.UUID      `gorm:"type:uuid;not null;index" json:"runtime_workspace_id"`
	OrganizationID     uuid.UUID      `gorm:"type:uuid;not null;index" json:"organization_id"`
	WorkspaceID        uuid.UUID      `gorm:"type:uuid;not null;index" json:"workspace_id"`
	CredentialID       *uuid.UUID     `gorm:"type:uuid;index" json:"credential_id,omitempty"`
	SubjectID          *uuid.UUID     `gorm:"type:uuid;index" json:"subject_id,omitempty"`
	AgentID            *uuid.UUID     `gorm:"type:uuid;index" json:"agent_id,omitempty"`
	Method             string         `gorm:"type:varchar(16);not null" json:"method"`
	Route              string         `gorm:"type:varchar(255);not null" json:"route"`
	StatusCode         int            `gorm:"not null" json:"status_code"`
	Result             string         `gorm:"type:varchar(16);not null" json:"result"`
	Details            datatypes.JSON `gorm:"type:jsonb;not null;default:'{}'" json:"details"`
	CreatedAt          time.Time      `json:"created_at"`
}

func (AccessEvent) TableName() string { return "runtime_workspace_access_events" }

type Principal struct {
	RuntimeWorkspace *RuntimeWorkspace
	Credential       *Credential
	SubjectID        uuid.UUID
	ExternalUserID   string
	Agent            *AgentBinding
}

// RuntimeContext is the transport-neutral identity assembled by the Runtime
// Workspace boundary. Handlers may continue to consume the existing Agent
// runtime context keys, while future capabilities can use this typed scope
// without re-parsing credentials or external headers.
type RuntimeContext struct {
	RuntimeWorkspaceID uuid.UUID `json:"runtime_workspace_id"`
	WorkspaceID        uuid.UUID `json:"workspace_id"`
	OrganizationID     uuid.UUID `json:"organization_id"`
	CredentialID       uuid.UUID `json:"credential_id"`
	SubjectID          uuid.UUID `json:"subject_id"`
	ExternalUserIDHash string    `json:"external_user_id_hash,omitempty"`
	AgentID            uuid.UUID `json:"agent_id,omitempty"`
	Scopes             []string  `json:"scopes,omitempty"`
}

type App struct {
	AppKey               string   `json:"app_key"`
	Name                 string   `json:"name"`
	Description          string   `json:"description,omitempty"`
	Icon                 string   `json:"icon,omitempty"`
	IconType             string   `json:"icon_type,omitempty"`
	IconURL              string   `json:"icon_url,omitempty"`
	Version              string   `json:"version"`
	VersionUUID          string   `json:"version_uuid"`
	HomeTitle            string   `json:"home_title,omitempty"`
	OpeningStatement     string   `json:"opening_statement,omitempty"`
	InputPlaceholder     string   `json:"input_placeholder,omitempty"`
	ThemeColor           string   `json:"theme_color,omitempty"`
	SuggestedQuestions   []string `json:"suggested_questions"`
	Status               string   `json:"status"`
	SupportsStream       bool     `json:"supports_stream"`
	FileUploadEnabled    bool     `json:"file_upload_enabled"`
	SupportsVision       bool     `json:"supports_vision"`
	AgentMemoryEnabled   bool     `json:"agent_memory_enabled"`
	MemoryRequiresUser   bool     `json:"memory_requires_user"`
	ExternalUserRequired bool     `json:"external_user_required"`
	Capabilities         []string `json:"capabilities"`
}

type ManagementApp struct {
	AgentID            string       `json:"agent_id"`
	AppKey             string       `json:"app_key"`
	WorkspaceID        string       `json:"workspace_id"`
	WorkspaceName      string       `json:"workspace_name"`
	Name               string       `json:"name"`
	Description        string       `json:"description,omitempty"`
	Icon               string       `json:"icon,omitempty"`
	IconType           string       `json:"icon_type,omitempty"`
	IconURL            string       `json:"icon_url,omitempty"`
	Version            string       `json:"version"`
	VersionUUID        string       `json:"version_uuid"`
	PublishedAt        time.Time    `json:"published_at"`
	Status             string       `json:"status"`
	SupportsStream     bool         `json:"supports_stream"`
	FileUploadEnabled  bool         `json:"file_upload_enabled"`
	SupportsVision     bool         `json:"supports_vision"`
	AgentMemoryEnabled bool         `json:"agent_memory_enabled"`
	Capabilities       []string     `json:"capabilities"`
	Tags               []CatalogTag `json:"tags"`
	Callable           bool         `json:"callable"`
	UnavailableReason  string       `json:"unavailable_reason,omitempty"`
}

type CatalogTag struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type ManagementAppSet struct {
	Revision int64           `json:"revision"`
	Items    []ManagementApp `json:"items"`
}

type SourceWorkspace struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	IsCurrent bool   `json:"current"`
}

type AgentCandidateSet struct {
	Scope                 string            `json:"scope"`
	CrossWorkspaceEnabled bool              `json:"cross_workspace_enabled"`
	SourceWorkspaces      []SourceWorkspace `json:"source_workspaces"`
	Items                 []AgentCandidate  `json:"items"`
}

type AgentCandidate struct {
	AgentID            string       `json:"agent_id"`
	WorkspaceID        string       `json:"workspace_id"`
	WorkspaceName      string       `json:"workspace_name"`
	Name               string       `json:"name"`
	Description        string       `json:"description,omitempty"`
	Icon               string       `json:"icon,omitempty"`
	IconType           string       `json:"icon_type,omitempty"`
	IconURL            string       `json:"icon_url,omitempty"`
	Version            string       `json:"version"`
	VersionUUID        string       `json:"version_uuid"`
	PublishedAt        time.Time    `json:"published_at"`
	FileUploadEnabled  bool         `json:"file_upload_enabled"`
	SupportsVision     bool         `json:"supports_vision"`
	AgentMemoryEnabled bool         `json:"agent_memory_enabled"`
	Capabilities       []string     `json:"capabilities"`
	Tags               []CatalogTag `json:"tags"`
}

type FileView struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Size      int64     `json:"size"`
	MimeType  string    `json:"mime_type"`
	Extension string    `json:"extension"`
	CreatedAt time.Time `json:"created_at"`
}
