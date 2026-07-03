package model

import (
	"time"

	"github.com/google/uuid"
)

// AccountSkillPreference stores per-account chat runtime skill preferences.
type AccountSkillPreference struct {
	OrganizationID  uuid.UUID `gorm:"type:uuid;primaryKey" json:"organization_id"`
	AccountID       uuid.UUID `gorm:"type:uuid;primaryKey" json:"account_id"`
	CallerType      string    `gorm:"type:varchar(32);primaryKey" json:"caller_type"`
	EnabledSkillIDs []string  `gorm:"type:jsonb;serializer:json;not null;default:'[]'" json:"enabled_skill_ids"`
	CreatedAt       time.Time `gorm:"not null;default:CURRENT_TIMESTAMP" json:"created_at"`
	UpdatedAt       time.Time `gorm:"not null;default:CURRENT_TIMESTAMP" json:"updated_at"`
}

func (AccountSkillPreference) TableName() string {
	return "chat_runtime_account_skill_preferences"
}
