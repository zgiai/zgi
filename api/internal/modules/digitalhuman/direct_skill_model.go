package digitalhuman

import (
	"time"

	"github.com/google/uuid"
)

// DirectSkillCredential stores a DigitalHuman-owned experimental capability
// credential. Secret material is encrypted and is never serialized.
type DirectSkillCredential struct {
	ID                   uuid.UUID `gorm:"type:uuid;primaryKey" json:"-"`
	OrganizationID       uuid.UUID `gorm:"type:uuid;not null;column:organization_id" json:"-"`
	WorkspaceID          uuid.UUID `gorm:"type:uuid;not null;column:workspace_id" json:"-"`
	DigitalHumanID       uuid.UUID `gorm:"type:uuid;not null;column:digital_human_id" json:"-"`
	SkillID              string    `gorm:"type:varchar(128);not null;column:skill_id" json:"-"`
	EncryptedCredentials string    `gorm:"type:text;not null;column:encrypted_credentials" json:"-"`
	CredentialVersion    int       `gorm:"not null;column:credential_version" json:"-"`
	Enabled              bool      `gorm:"not null;default:true" json:"-"`
	Revision             int64     `gorm:"not null;default:1" json:"-"`
	CreatedBy            uuid.UUID `gorm:"type:uuid;not null;column:created_by" json:"-"`
	UpdatedBy            uuid.UUID `gorm:"type:uuid;not null;column:updated_by" json:"-"`
	CreatedAt            time.Time `gorm:"not null" json:"-"`
	UpdatedAt            time.Time `gorm:"not null" json:"-"`
}

func (DirectSkillCredential) TableName() string {
	return "digital_human_direct_skill_credentials"
}
