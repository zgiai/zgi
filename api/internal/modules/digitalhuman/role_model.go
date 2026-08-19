package digitalhuman

import (
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/resourcebindings"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// RoleTemplate is a reusable workspace-scoped job definition.
type RoleTemplate struct {
	ID                 uuid.UUID      `gorm:"type:uuid;primaryKey" json:"id"`
	OrganizationID     uuid.UUID      `gorm:"type:uuid;not null" json:"organization_id"`
	WorkspaceID        uuid.UUID      `gorm:"type:uuid;not null" json:"workspace_id"`
	Name               string         `gorm:"type:varchar(80);not null" json:"name"`
	Description        string         `gorm:"type:text;not null" json:"description"`
	DraftVersionID     *uuid.UUID     `gorm:"type:uuid" json:"draft_version_id,omitempty"`
	PublishedVersionID *uuid.UUID     `gorm:"type:uuid" json:"published_version_id,omitempty"`
	Revision           int64          `gorm:"not null" json:"revision"`
	CreatedBy          *uuid.UUID     `gorm:"type:uuid" json:"created_by,omitempty"`
	UpdatedBy          *uuid.UUID     `gorm:"type:uuid" json:"updated_by,omitempty"`
	CreatedAt          time.Time      `gorm:"not null" json:"created_at"`
	UpdatedAt          time.Time      `gorm:"not null" json:"updated_at"`
	DeletedAt          gorm.DeletedAt `gorm:"index" json:"-"`
}

func (RoleTemplate) TableName() string { return "digital_human_role_templates" }

// RoleTemplateVersion freezes goals, responsibilities, capabilities, and SOP references.
type RoleTemplateVersion struct {
	ID               uuid.UUID      `gorm:"type:uuid;primaryKey" json:"id"`
	RoleTemplateID   uuid.UUID      `gorm:"type:uuid;not null" json:"role_template_id"`
	OrganizationID   uuid.UUID      `gorm:"type:uuid;not null" json:"organization_id"`
	WorkspaceID      uuid.UUID      `gorm:"type:uuid;not null" json:"workspace_id"`
	State            VersionState   `gorm:"type:varchar(16);not null" json:"state"`
	VersionNumber    int            `gorm:"not null" json:"version_number"`
	Goals            datatypes.JSON `gorm:"type:jsonb;not null" json:"goals"`
	Responsibilities datatypes.JSON `gorm:"type:jsonb;not null" json:"responsibilities"`
	Capabilities     datatypes.JSON `gorm:"type:jsonb;not null" json:"capabilities"`
	SOPRefs          datatypes.JSON `gorm:"type:jsonb;not null" json:"sop_refs"`
	KnowledgeRefs    datatypes.JSON `gorm:"type:jsonb;not null" json:"knowledge_refs"`
	Policy           datatypes.JSON `gorm:"type:jsonb;not null" json:"policy"`
	EvaluationRefs   datatypes.JSON `gorm:"type:jsonb;not null" json:"evaluation_refs"`
	Revision         string         `gorm:"type:varchar(71);not null" json:"revision"`
	LockVersion      int64          `gorm:"not null" json:"lock_version"`
	PublishedBy      *uuid.UUID     `gorm:"type:uuid" json:"published_by,omitempty"`
	PublishedAt      *time.Time     `json:"published_at,omitempty"`
	CreatedBy        *uuid.UUID     `gorm:"type:uuid" json:"created_by,omitempty"`
	UpdatedBy        *uuid.UUID     `gorm:"type:uuid" json:"updated_by,omitempty"`
	CreatedAt        time.Time      `gorm:"not null" json:"created_at"`
	UpdatedAt        time.Time      `gorm:"not null" json:"updated_at"`
}

func (RoleTemplateVersion) TableName() string { return "digital_human_role_template_versions" }

// RoleAssignment pins one published reusable role to one DigitalHuman version.
type RoleAssignment struct {
	ID                    uuid.UUID      `gorm:"type:uuid;primaryKey" json:"id"`
	OrganizationID        uuid.UUID      `gorm:"type:uuid;not null" json:"organization_id"`
	WorkspaceID           uuid.UUID      `gorm:"type:uuid;not null" json:"workspace_id"`
	DigitalHumanID        uuid.UUID      `gorm:"type:uuid;not null" json:"digital_human_id"`
	DigitalHumanVersionID uuid.UUID      `gorm:"type:uuid;not null" json:"digital_human_version_id"`
	RoleTemplateID        uuid.UUID      `gorm:"type:uuid;not null" json:"role_template_id"`
	RoleTemplateVersionID uuid.UUID      `gorm:"type:uuid;not null" json:"role_template_version_id"`
	Parameters            datatypes.JSON `gorm:"type:jsonb;not null" json:"parameters"`
	OverridePolicy        datatypes.JSON `gorm:"type:jsonb;not null" json:"override_policy"`
	EffectiveFrom         *time.Time     `json:"effective_from,omitempty"`
	EffectiveTo           *time.Time     `json:"effective_to,omitempty"`
	Revision              string         `gorm:"type:varchar(71);not null" json:"revision"`
	CreatedBy             *uuid.UUID     `gorm:"type:uuid" json:"created_by,omitempty"`
	UpdatedBy             *uuid.UUID     `gorm:"type:uuid" json:"updated_by,omitempty"`
	CreatedAt             time.Time      `gorm:"not null" json:"created_at"`
	UpdatedAt             time.Time      `gorm:"not null" json:"updated_at"`
}

func (RoleAssignment) TableName() string { return "digital_human_role_assignments" }

// ResourceBinding pins a credential-safe resource authorization to one DigitalHuman version.
type ResourceBinding struct {
	ID                    uuid.UUID                    `gorm:"type:uuid;primaryKey" json:"id"`
	OrganizationID        uuid.UUID                    `gorm:"type:uuid;not null" json:"organization_id"`
	WorkspaceID           uuid.UUID                    `gorm:"type:uuid;not null" json:"workspace_id"`
	DigitalHumanID        uuid.UUID                    `gorm:"type:uuid;not null" json:"digital_human_id"`
	DigitalHumanVersionID uuid.UUID                    `gorm:"type:uuid;not null" json:"digital_human_version_id"`
	BindingType           resourcebindings.BindingType `gorm:"type:varchar(32);not null" json:"binding_type"`
	ResourceID            string                       `gorm:"type:varchar(255);not null" json:"resource_id"`
	ParentResourceID      string                       `gorm:"type:varchar(255);not null" json:"parent_resource_id"`
	DisplayName           string                       `gorm:"type:varchar(255);not null" json:"display_name"`
	AccessMode            resourcebindings.AccessMode  `gorm:"type:varchar(16);not null" json:"access_mode"`
	ResourceRevision      string                       `gorm:"type:varchar(255);not null" json:"resource_revision"`
	BindingRevision       string                       `gorm:"type:varchar(71);not null" json:"binding_revision"`
	CredentialRef         string                       `gorm:"type:varchar(512);not null" json:"credential_ref,omitempty"`
	Metadata              datatypes.JSON               `gorm:"type:jsonb;not null" json:"metadata"`
	AuthorizedBy          *uuid.UUID                   `gorm:"type:uuid" json:"authorized_by,omitempty"`
	AuthorizedAt          *time.Time                   `json:"authorized_at,omitempty"`
	CreatedAt             time.Time                    `gorm:"not null" json:"created_at"`
	UpdatedAt             time.Time                    `gorm:"not null" json:"updated_at"`
}

func (ResourceBinding) TableName() string { return "digital_human_resource_bindings" }
