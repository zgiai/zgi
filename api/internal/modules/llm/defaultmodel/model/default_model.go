package model

import (
	"time"

	"github.com/google/uuid"
	llmsharedtypes "github.com/zgiai/zgi/api/internal/modules/llm/shared/types"
	"gorm.io/gorm"
)

type DefaultModel struct {
	ID             uuid.UUID                 `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"id"`
	OrganizationID uuid.UUID                 `gorm:"type:uuid;not null;index;column:organization_id" json:"organization_id"`
	UseCase        string                    `gorm:"type:varchar(50);not null;index;column:use_case" json:"use_case"`
	Provider       string                    `gorm:"type:varchar(100);not null" json:"provider"`
	Model          string                    `gorm:"type:varchar(100);not null;column:model" json:"model"`
	Params         llmsharedtypes.JSONObject `gorm:"type:jsonb;not null;default:'{}'" json:"params"`
	CreatedBy      *uuid.UUID                `gorm:"type:uuid;column:created_by" json:"created_by,omitempty"`
	UpdatedBy      *uuid.UUID                `gorm:"type:uuid;column:updated_by" json:"updated_by,omitempty"`
	CreatedAt      time.Time                 `gorm:"not null;default:CURRENT_TIMESTAMP" json:"created_at"`
	UpdatedAt      time.Time                 `gorm:"not null;default:CURRENT_TIMESTAMP" json:"updated_at"`
	DeletedAt      gorm.DeletedAt            `gorm:"index" json:"deleted_at,omitempty"`
}

func (DefaultModel) TableName() string {
	return "llm_default_models"
}

func (m *DefaultModel) BeforeCreate(tx *gorm.DB) error {
	if m.ID == uuid.Nil {
		m.ID = uuid.New()
	}
	if m.Params == nil {
		m.Params = llmsharedtypes.JSONObject{}
	}
	return nil
}
