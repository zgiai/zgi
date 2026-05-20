package model

import (
	"time"

	"github.com/google/uuid"
	llmmodel "github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/model"
	providermodel "github.com/zgiai/zgi/api/internal/modules/llm/provider/model"
	"gorm.io/gorm"
)

// Deprecated: TenantModel is deprecated and will be removed in a future version.
// Use llmmodel.ModelConfig instead, which uses UUID model_id foreign key.
// This table (llm_tenant_models) has been superseded by llm_tenant_model_configs.
//
// TenantModel represents the relationship between a tenant and specific LLM models
// Tenant administrators can select which models from enabled providers are available
type TenantModel struct {
	ID        uuid.UUID      `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"id"`
	TenantID  uuid.UUID      `gorm:"type:uuid;not null;index" json:"tenant_id"`            // Foreign key to tenants
	Provider  string         `gorm:"type:varchar(100);not null;index" json:"provider"`     // Foreign key to llm_providers.provider
	Model     string         `gorm:"type:varchar(100);not null;index" json:"model"`        // Foreign key to llm_models.name
	IsEnabled bool           `gorm:"default:true;index" json:"is_enabled"`                 // Whether this model is enabled
	CreatedAt time.Time      `gorm:"not null;default:CURRENT_TIMESTAMP" json:"created_at"` // Record creation time
	UpdatedAt time.Time      `gorm:"not null;default:CURRENT_TIMESTAMP" json:"updated_at"` // Record update time
	DeletedAt gorm.DeletedAt `gorm:"index" json:"deleted_at,omitempty"`                    // Soft delete timestamp

	// Relations - using V2 models
	ProviderInfo providermodel.LLMProvider `gorm:"foreignKey:Provider;references:Name;constraint:OnDelete:CASCADE" json:"provider_info,omitempty"`
	ModelInfo    llmmodel.LLMModel         `gorm:"foreignKey:Provider,Model;references:Provider,Name;constraint:OnDelete:CASCADE" json:"model_info,omitempty"`
}

func (TenantModel) TableName() string {
	return "llm_tenant_models"
}

// BeforeCreate hook to generate UUID if not set
func (tm *TenantModel) BeforeCreate(tx *gorm.DB) error {
	if tm.ID == uuid.Nil {
		tm.ID = uuid.New()
	}
	return nil
}
