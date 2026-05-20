package model

import "github.com/google/uuid"

// ProviderView represents a unified view of provider (global or custom)
// Aligned with ModelMeta API standard for consistency
type ProviderView struct {
	// Basic fields (used in list view)
	ID           uuid.UUID `json:"id"`
	Object       string    `json:"object"`        // Type identifier, always "provider"
	Name         string    `json:"provider"`      // Provider identifier (ModelMeta standard)
	DisplayName  string    `json:"provider_name"` // Display name (ModelMeta standard)
	APIBaseURL   string    `json:"api_base_url,omitempty"`
	LogoURL      string    `json:"logo_url,omitempty"`
	ProviderType string    `json:"provider_type"` // "global" or "custom" (source type)
	IsEnabled    bool      `json:"is_enabled"`
	IsAvailable  bool      `json:"is_available"` // Whether the provider has active channels (official or private)
	SortOrder    int       `json:"sort_order"`

	// Extended fields (used in detail view)
	Description  string                 `json:"description,omitempty"`
	Website      string                 `json:"website,omitempty"`
	APIDocsURL   string                 `json:"api_docs_url,omitempty"`
	PricingURL   string                 `json:"pricing_url,omitempty"`
	CountryCode  string                 `json:"country_code,omitempty"`
	FoundedYear  int                    `json:"founded_year,omitempty"`
	Tagline      string                 `json:"tagline,omitempty"`
	ModelCount   int                    `json:"model_count,omitempty"`
	ChannelCount int                    `json:"channel_count,omitempty"`
	Metadata     map[string]interface{} `json:"metadata,omitempty"`
	CreatedAt    int64                  `json:"created_at,omitempty"` // Unix timestamp (ModelMeta standard)
	UpdatedAt    int64                  `json:"updated_at,omitempty"` // Unix timestamp (ModelMeta standard)
}
