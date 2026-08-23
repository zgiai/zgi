package digitalhuman

import (
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/resourcebindings"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// PersonaTemplate is a workspace-scoped identity preset. Its name is the
// runtime identity name, not a second piece of template-only metadata.
type PersonaTemplate struct {
	ID             uuid.UUID      `gorm:"type:uuid;primaryKey" json:"id"`
	OrganizationID uuid.UUID      `gorm:"type:uuid;not null" json:"organization_id"`
	WorkspaceID    uuid.UUID      `gorm:"type:uuid;not null" json:"workspace_id"`
	Name           string         `gorm:"type:varchar(80);not null" json:"name"`
	Persona        string         `gorm:"type:text;not null" json:"persona"`
	Goals          datatypes.JSON `gorm:"type:jsonb;not null;default:'[]'" json:"goals"`
	Rules          datatypes.JSON `gorm:"type:jsonb;not null;default:'[]'" json:"rules"`
	Revision       int64          `gorm:"not null;default:1" json:"revision"`
	CreatedBy      *uuid.UUID     `gorm:"type:uuid" json:"created_by,omitempty"`
	UpdatedBy      *uuid.UUID     `gorm:"type:uuid" json:"updated_by,omitempty"`
	CreatedAt      time.Time      `gorm:"not null" json:"created_at"`
	UpdatedAt      time.Time      `gorm:"not null" json:"updated_at"`
	DeletedAt      gorm.DeletedAt `gorm:"index" json:"-"`
}

func (PersonaTemplate) TableName() string { return "digital_human_persona_templates" }

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
