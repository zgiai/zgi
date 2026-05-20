package model

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ============================================================================
// Global Provider - managed by platform administrators
// ============================================================================

// LLMProvider represents a global LLM provider (e.g., OpenAI, Anthropic)
// Schema aligned with ModelMeta API: https://api.modelmeta.dev/v1/providers
// Database column names and JSON tags are aligned with ModelMeta standard
type LLMProvider struct {
	// ModelMeta standard fields
	ID           uuid.UUID              `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"id"`
	Object       string                 `gorm:"type:varchar(20);default:'provider'" json:"object"`                        // Type identifier, always "provider"
	Provider     string                 `gorm:"type:varchar(50);uniqueIndex;not null;column:provider" json:"provider"`    // Provider identifier (e.g., "openai", "anthropic")
	ProviderName string                 `gorm:"type:varchar(100);not null;column:provider_name" json:"provider_name"`     // Display name (e.g., "OpenAI", "Anthropic")
	LogoURL      string                 `gorm:"type:varchar(255);column:logo_url" json:"logo_url,omitempty"`              // Provider logo URL
	Website      string                 `gorm:"type:varchar(255)" json:"website,omitempty"`                               // Official website
	APIDocsURL   string                 `gorm:"type:varchar(255);column:documentation_url" json:"api_docs_url,omitempty"` // API documentation URL
	PricingURL   string                 `gorm:"type:varchar(255);column:pricing_url" json:"pricing_url,omitempty"`        // Pricing page URL
	CountryCode  string                 `gorm:"type:varchar(10);column:country_code" json:"country_code,omitempty"`       // Country code (ISO 3166-1 alpha-2)
	Tagline      string                 `gorm:"type:varchar(500)" json:"tagline,omitempty"`                               // Provider tagline/slogan
	Description  string                 `gorm:"type:text" json:"description,omitempty"`                                   // Provider description
	ModelCount   int                    `gorm:"-" json:"model_count"`                                                     // Number of models (computed, not stored)
	Metadata     map[string]interface{} `gorm:"type:jsonb;serializer:json;default:'{}'" json:"metadata,omitempty"`        // Extended metadata (i18n, social, etc.)
	CreatedAt    time.Time              `gorm:"not null;default:CURRENT_TIMESTAMP" json:"created_at"`                     // Creation timestamp
	UpdatedAt    time.Time              `gorm:"not null;default:CURRENT_TIMESTAMP" json:"updated_at"`                     // Last update timestamp

	FoundedYear int `gorm:"default:0;column:founded_year" json:"founded_year,omitempty"` // Founded year (ModelMeta standard)

	// ZGI internal fields (not exposed in public API)
	APIBaseURL   string         `gorm:"type:varchar(255);column:api_base_url" json:"-"`                  // Internal API base URL
	ProviderType string         `gorm:"type:varchar(20);default:'vendor';column:provider_type" json:"-"` // Business type: vendor/aggregator
	IsActive     bool           `gorm:"default:true;index" json:"-"`                                     // Whether provider is active
	SortOrder    int            `gorm:"default:0" json:"-"`                                              // Display sort order
	DeletedAt    gorm.DeletedAt `gorm:"index" json:"-"`                                                  // Soft delete timestamp
}

func (LLMProvider) TableName() string {
	return "llm_providers"
}

func (p *LLMProvider) BeforeCreate(tx *gorm.DB) error {
	if p.ID == uuid.Nil {
		p.ID = uuid.New()
	}
	return nil
}

// MarshalJSON customizes JSON serialization to align with ModelMeta API standard
// Converts time.Time to Unix timestamp (integer)
func (p LLMProvider) MarshalJSON() ([]byte, error) {
	type Alias LLMProvider
	return json.Marshal(&struct {
		*Alias
		CreatedAt int64 `json:"created_at"`
		UpdatedAt int64 `json:"updated_at"`
	}{
		Alias:     (*Alias)(&p),
		CreatedAt: p.CreatedAt.Unix(),
		UpdatedAt: p.UpdatedAt.Unix(),
	})
}
