package model

import (
	"time"

	"github.com/google/uuid"
)

// OrganizationSkillConfig stores the AIChat skill enablement for one organization.
type OrganizationSkillConfig struct {
	OrganizationID uuid.UUID `gorm:"type:uuid;primaryKey" json:"organization_id"`
	SkillID        string    `gorm:"type:varchar(128);primaryKey" json:"skill_id"`
	Enabled        bool      `gorm:"not null;default:true" json:"enabled"`
	CreatedAt      time.Time `gorm:"not null;default:CURRENT_TIMESTAMP" json:"created_at"`
	UpdatedAt      time.Time `gorm:"not null;default:CURRENT_TIMESTAMP" json:"updated_at"`
}

func (OrganizationSkillConfig) TableName() string {
	return "aichat_organization_skill_configs"
}
