package digitalhuman

import (
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/contracts/chatfile"
	"gorm.io/datatypes"
)

// Status describes whether a DigitalHuman is still a draft, runnable, or disabled.
type Status string

const (
	// StatusDraft identifies a DigitalHuman that has never been published.
	StatusDraft Status = "draft"
	// StatusActive identifies a published and runnable DigitalHuman.
	StatusActive Status = "active"
	// StatusDisabled identifies a published DigitalHuman disabled by an operator.
	StatusDisabled Status = "disabled"
)

// AutonomyLevel controls how independently a DigitalHuman may act.
type AutonomyLevel string

const (
	// AutonomyAssistive requires an operator to drive each meaningful step.
	AutonomyAssistive AutonomyLevel = "assistive"
	// AutonomySupervised allows planning while retaining approval boundaries.
	AutonomySupervised AutonomyLevel = "supervised"
	// AutonomyAutonomous allows policy-governed independent execution.
	AutonomyAutonomous AutonomyLevel = "autonomous"
)

// Visibility describes who may discover a DigitalHuman.
type Visibility string

const (
	// VisibilityPrivate limits discovery to its owner and administrators.
	VisibilityPrivate Visibility = "private"
	// VisibilityWorkspace makes the DigitalHuman discoverable in its workspace.
	VisibilityWorkspace Visibility = "workspace"
)

// VersionState separates the mutable draft from immutable published versions.
type VersionState string

const (
	// VersionStateDraft is the single mutable configuration version.
	VersionStateDraft VersionState = "draft"
	// VersionStatePublished is an immutable execution configuration version.
	VersionStatePublished VersionState = "published"
)

// Definition is the independently owned DigitalHuman aggregate root.
type Definition struct {
	ID             uuid.UUID `gorm:"type:uuid;primaryKey" json:"id"`
	WebAppID       uuid.UUID `gorm:"type:uuid;not null;uniqueIndex;column:web_app_id" json:"web_app_id"`
	OrganizationID uuid.UUID `gorm:"type:uuid;not null;column:organization_id" json:"organization_id"`
	WorkspaceID    uuid.UUID `gorm:"type:uuid;not null;column:workspace_id" json:"workspace_id"`
	// IdentityWorkspaceID is the workspace end-user identities derive from:
	// where the DigitalHuman was created, kept across workspace moves.
	IdentityWorkspaceID *uuid.UUID    `gorm:"type:uuid;column:identity_workspace_id" json:"-"`
	Name                string        `gorm:"type:varchar(80);not null" json:"name"`
	Avatar              string        `gorm:"type:text;not null;default:''" json:"avatar"`
	IconType            string        `gorm:"type:varchar(16);not null;default:'text';column:icon_type" json:"icon_type"`
	Icon                string        `gorm:"type:text;not null;default:''" json:"icon"`
	IconURL             string        `gorm:"-" json:"icon_url,omitempty"`
	Description         string        `gorm:"type:text;not null;default:''" json:"description"`
	Status              Status        `gorm:"type:varchar(16);not null;default:'draft'" json:"status"`
	AutonomyLevel       AutonomyLevel `gorm:"type:varchar(16);not null;default:'supervised'" json:"autonomy_level"`
	Visibility          Visibility    `gorm:"type:varchar(16);not null;default:'workspace'" json:"visibility"`
	DraftVersionID      *uuid.UUID    `gorm:"type:uuid;column:draft_version_id" json:"draft_version_id,omitempty"`
	PublishedVersionID  *uuid.UUID    `gorm:"type:uuid;column:published_version_id" json:"published_version_id,omitempty"`
	// SetupRequirements are the required items of the built-in template it was
	// created from (ADR 0023); publishing waits until the draft meets them.
	SetupRequirements datatypes.JSON `gorm:"type:jsonb;not null;default:'{}';column:setup_requirements" json:"setup_requirements"`
	Revision          int64          `gorm:"not null;default:1" json:"revision"`
	CreatedBy         *uuid.UUID     `gorm:"type:uuid;column:created_by" json:"created_by,omitempty"`
	UpdatedBy         *uuid.UUID     `gorm:"type:uuid;column:updated_by" json:"updated_by,omitempty"`
	CreatedAt         time.Time      `gorm:"not null" json:"created_at"`
	UpdatedAt         time.Time      `gorm:"not null" json:"updated_at"`
	DeletedAt         *time.Time     `gorm:"column:deleted_at" json:"-"`
}

// TableName returns the DigitalHuman aggregate table.
func (Definition) TableName() string { return "digital_humans" }

// Version stores either the mutable draft or an immutable published snapshot.
type Version struct {
	FileUploadPolicy   *chatfile.Policy        `gorm:"serializer:json;type:jsonb;column:file_upload_policy" json:"file_upload_policy,omitempty"`
	ID                 uuid.UUID               `gorm:"type:uuid;primaryKey" json:"id"`
	DigitalHumanID     uuid.UUID               `gorm:"type:uuid;not null;column:digital_human_id" json:"digital_human_id"`
	OrganizationID     uuid.UUID               `gorm:"type:uuid;not null;column:organization_id" json:"organization_id"`
	WorkspaceID        uuid.UUID               `gorm:"type:uuid;not null;column:workspace_id" json:"workspace_id"`
	State              VersionState            `gorm:"type:varchar(16);not null" json:"state"`
	VersionNumber      int                     `gorm:"not null;default:0;column:version_number" json:"version_number"`
	Name               string                  `gorm:"type:varchar(80);not null;default:''" json:"name"`
	Description        string                  `gorm:"type:text;not null;default:''" json:"description"`
	IdentityName       string                  `gorm:"type:varchar(80);not null;default:'';column:identity_name" json:"identity_name"`
	Persona            string                  `gorm:"type:text;not null;default:''" json:"persona"`
	Goals              datatypes.JSON          `gorm:"type:jsonb;not null;default:'[]';column:goals" json:"goals"`
	Rules              datatypes.JSON          `gorm:"type:jsonb;not null;default:'[]';column:rules" json:"rules"`
	DefaultModel       string                  `gorm:"type:varchar(255);not null;default:'';column:default_model" json:"default_model"`
	MemoryPolicy       datatypes.JSON          `gorm:"type:jsonb;not null;default:'{}';column:memory_policy" json:"memory_policy"`
	TriggerPolicy      datatypes.JSON          `gorm:"type:jsonb;not null;default:'{}';column:trigger_policy" json:"trigger_policy"`
	PresentationPolicy datatypes.JSON          `gorm:"type:jsonb;not null;default:'{}';column:presentation_policy" json:"presentation_policy"`
	ExecutionRuntime   *ExecutionRuntimeConfig `gorm:"serializer:json;type:jsonb;column:execution_runtime" json:"execution_runtime,omitempty"`
	IdentitySnapshot   datatypes.JSON          `gorm:"type:jsonb;not null;default:'{}';column:identity_snapshot" json:"identity_snapshot"`
	Revision           string                  `gorm:"type:varchar(71);not null" json:"revision"`
	LockVersion        int64                   `gorm:"not null;default:1;column:lock_version" json:"lock_version"`
	PublishedBy        *uuid.UUID              `gorm:"type:uuid;column:published_by" json:"published_by,omitempty"`
	PublishedAt        *time.Time              `gorm:"column:published_at" json:"published_at,omitempty"`
	CreatedBy          *uuid.UUID              `gorm:"type:uuid;column:created_by" json:"created_by,omitempty"`
	UpdatedBy          *uuid.UUID              `gorm:"type:uuid;column:updated_by" json:"updated_by,omitempty"`
	CreatedAt          time.Time               `gorm:"not null" json:"created_at"`
	UpdatedAt          time.Time               `gorm:"not null" json:"updated_at"`
}

// TableName returns the DigitalHuman version table.
func (Version) TableName() string { return "digital_human_versions" }
