package digitalhuman

import (
	"time"

	"github.com/google/uuid"
)

// ChannelInstallationCredential stores credential material owned by one
// channel installation. The encrypted envelope is never serialized or exposed
// by the channel API.
type ChannelInstallationCredential struct {
	InstallationID       uuid.UUID `gorm:"type:uuid;primaryKey" json:"-"`
	OrganizationID       uuid.UUID `gorm:"type:uuid;not null" json:"-"`
	WorkspaceID          uuid.UUID `gorm:"type:uuid;not null" json:"-"`
	Provider             string    `gorm:"type:varchar(32);not null" json:"-"`
	EncryptedCredentials string    `gorm:"type:text;not null" json:"-"`
	CredentialVersion    int       `gorm:"not null" json:"-"`
	CreatedBy            uuid.UUID `gorm:"type:uuid;not null" json:"-"`
	UpdatedBy            uuid.UUID `gorm:"type:uuid;not null" json:"-"`
	CreatedAt            time.Time `gorm:"not null" json:"-"`
	UpdatedAt            time.Time `gorm:"not null" json:"-"`
}

func (ChannelInstallationCredential) TableName() string {
	return "channel_installation_credentials"
}
