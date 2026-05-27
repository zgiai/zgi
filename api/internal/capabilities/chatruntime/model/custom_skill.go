package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	CustomSkillStatusActive  = "active"
	CustomSkillStatusInvalid = "invalid"
)

type CustomSkill struct {
	ID              uuid.UUID              `gorm:"type:uuid;primaryKey" json:"id"`
	OrganizationID  uuid.UUID              `gorm:"type:uuid;not null;index:idx_chat_runtime_custom_skills_org_skill,unique" json:"organization_id"`
	SkillID         string                 `gorm:"type:varchar(128);not null;index:idx_chat_runtime_custom_skills_org_skill,unique" json:"skill_id"`
	Name            string                 `gorm:"type:varchar(128);not null" json:"name"`
	Description     string                 `gorm:"type:text;not null" json:"description"`
	WhenToUse       string                 `gorm:"type:text;not null" json:"when_to_use"`
	RuntimeType     string                 `gorm:"type:varchar(32);not null" json:"runtime_type"`
	Display         map[string]interface{} `gorm:"type:jsonb;serializer:json;not null;default:'{}'" json:"display"`
	StoragePath     string                 `gorm:"type:text;not null" json:"storage_path"`
	Manifest        map[string]interface{} `gorm:"type:jsonb;serializer:json;not null;default:'{}'" json:"manifest"`
	Status          string                 `gorm:"type:varchar(32);not null" json:"status"`
	ValidationError string                 `gorm:"type:text" json:"validation_error,omitempty"`
	CreatedBy       uuid.UUID              `gorm:"type:uuid;not null" json:"created_by"`
	CreatedAt       time.Time              `json:"created_at"`
	UpdatedAt       time.Time              `json:"updated_at"`
	DeletedAt       gorm.DeletedAt         `gorm:"index" json:"-"`
}

func (CustomSkill) TableName() string {
	return "chat_runtime_custom_skills"
}
