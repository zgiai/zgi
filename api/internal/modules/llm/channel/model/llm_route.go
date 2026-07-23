package model

import (
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	credentialmodel "github.com/zgiai/zgi/api/internal/modules/llm/credential/model"
	"github.com/zgiai/zgi/api/internal/modules/llm/shared"
	"gorm.io/gorm"
)

// ============================================================================
// LLMRoute - tenant's routing configuration
// ============================================================================

// LLMRoute represents a tenant's routing configuration for load balancing
type LLMRoute struct {
	ID             uuid.UUID `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"id"`
	OrganizationID uuid.UUID `gorm:"type:uuid;not null;index" json:"organization_id"`

	// Route type: ZGI_CLOUD or PRIVATE
	Type shared.RouteType `gorm:"column:type;type:varchar(20);not null;index" json:"type"`

	// For PRIVATE: tenant's own configuration
	CredentialID *uuid.UUID `gorm:"column:user_credential_id;type:uuid;index" json:"credential_id,omitempty"`
	Name         string     `gorm:"type:varchar(255)" json:"name"`

	// Core: Supported models (primary field)
	Models []string `gorm:"type:jsonb;serializer:json;default:'[]'" json:"models,omitempty"`

	// API configuration
	ChannelProvider  string                 `gorm:"column:provider;type:varchar(100)" json:"channel_provider,omitempty"`
	APIBaseURL       string                 `gorm:"type:varchar(500)" json:"api_base_url,omitempty"`
	NativeProtocols  NativeProtocolConfig   `gorm:"type:jsonb;serializer:json;default:'{}'" json:"native_protocols,omitempty"`
	ModelMaps        map[string]interface{} `gorm:"type:jsonb;serializer:json;default:'{}'" json:"model_maps,omitempty"`
	ParamOverride    map[string]interface{} `gorm:"type:jsonb;serializer:json;default:'{}'" json:"param_override,omitempty"`
	HeaderOverride   map[string]interface{} `gorm:"type:jsonb;serializer:json;default:'{}'" json:"header_override,omitempty"`
	ValidationReport map[string]interface{} `gorm:"type:jsonb;serializer:json;default:'{}'" json:"validation_report,omitempty"`

	// Additional fields
	Tags        []string `gorm:"type:jsonb;serializer:json;default:'[]'" json:"tags,omitempty"`
	Description string   `gorm:"type:text" json:"description,omitempty"`

	// Routing parameters
	Priority   int  `gorm:"not null;default:0" json:"priority"`
	Weight     int  `gorm:"not null;default:1" json:"weight"`
	IsEnabled  bool `gorm:"not null;default:true;index" json:"is_enabled"`
	IsOfficial bool `gorm:"not null;default:false;index" json:"is_official"` // For mixed load balancing: true = official Console channel
	AutoBan    bool `gorm:"default:false" json:"auto_ban"`

	// Sync mode: 'snapshot' (default) or 'realtime'
	SyncMode     string     `gorm:"type:varchar(20);default:'snapshot'" json:"sync_mode"`
	LastSyncedAt *time.Time `gorm:"type:timestamp" json:"last_synced_at,omitempty"`

	// Balance tracking (for user channels)
	Balance  decimal.Decimal `gorm:"type:decimal(15,4);default:0" json:"balance"`
	Currency string          `gorm:"type:varchar(10);default:'USD'" json:"currency"`

	// Timestamps
	CreatedAt time.Time      `gorm:"not null;default:CURRENT_TIMESTAMP" json:"created_at"`
	UpdatedAt time.Time      `gorm:"not null;default:CURRENT_TIMESTAMP" json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"deleted_at,omitempty"`

	// Relations
	TenantCredential *credentialmodel.TenantCredential `gorm:"foreignKey:CredentialID;references:ID" json:"tenant_credential,omitempty"`

	// Transient fields (not persisted)
	PlatformAPIKey              string          `gorm:"-" json:"-"`
	UpstreamGeneration          int64           `gorm:"-" json:"-"`
	UpstreamWouldGuard          bool            `gorm:"-" json:"-"`
	UpstreamHalfOpen            bool            `gorm:"-" json:"-"`
	UpstreamProbe               bool            `gorm:"-" json:"-"`
	UpstreamProbeRequiresBackup bool            `gorm:"-" json:"-"`
	OfficialProviderModels      []ProviderModel `gorm:"-" json:"-"`
}

// ProviderModel identifies one exact provider/model pair advertised by an official channel.
type ProviderModel struct {
	Provider string `json:"provider"`
	Model    string `json:"model"`
}

func (LLMRoute) TableName() string {
	return "llm_routes"
}

type NativeProtocolConfig struct {
	OpenAIResponses   NativeProtocolEndpoint `json:"openai_responses,omitempty"`
	AnthropicMessages NativeProtocolEndpoint `json:"anthropic_messages,omitempty"`
}

type NativeProtocolEndpoint struct {
	Enabled bool   `json:"enabled,omitempty"`
	BaseURL string `json:"base_url,omitempty"`
}

func (r *LLMRoute) BeforeCreate(tx *gorm.DB) error {
	if r.ID == uuid.Nil {
		r.ID = uuid.New()
	}
	return nil
}

// IsUserChannel returns true if this is a user's own channel
func (r *LLMRoute) IsUserChannel() bool {
	return r.Type == shared.RouteTypePrivate
}

// GetEffectiveModels returns the effective model list for this route
// This is the single source of truth for model list resolution
func (r *LLMRoute) GetEffectiveModels() []string {
	if len(r.Models) == 0 {
		return []string{}
	}
	return normalizeRouteModels(r.Models)
}

// SupportsModel returns whether this route explicitly supports the model name.
func (r *LLMRoute) SupportsModel(modelName string) bool {
	targetModel := strings.TrimSpace(modelName)
	if targetModel == "" {
		return false
	}

	for _, modelName := range r.GetEffectiveModels() {
		if modelName == targetModel || modelName == "*" {
			return true
		}
	}

	return false
}

// SupportsModelForProvider requires exact provider provenance for official routes.
// Private routes retain their existing model-list behavior.
func (r *LLMRoute) SupportsModelForProvider(provider, modelName string) bool {
	if !r.IsOfficial && r.Type != shared.RouteTypeZGICloud {
		return r.SupportsModel(modelName)
	}

	provider = strings.TrimSpace(provider)
	modelName = strings.TrimSpace(modelName)
	if provider == "" || modelName == "" {
		return false
	}

	// Snapshots created before provider provenance was introduced can retain
	// their effective model names while the new provider-model list is empty.
	// Preserve the legacy model-name behavior until a successful sync supplies
	// exact pairs; once any pair exists, the strict provider check below applies.
	if len(r.OfficialProviderModels) == 0 {
		return r.SupportsModel(modelName)
	}
	for _, pair := range r.OfficialProviderModels {
		if pair.Provider == provider && pair.Model == modelName {
			return true
		}
	}
	return false
}

func normalizeRouteModels(models []string) []string {
	normalized := make([]string, 0, len(models))
	seen := make(map[string]struct{}, len(models))

	for _, modelName := range models {
		canonical := canonicalRouteModel(modelName)
		if canonical == "" {
			continue
		}
		if _, ok := seen[canonical]; ok {
			continue
		}
		seen[canonical] = struct{}{}
		normalized = append(normalized, canonical)
	}

	return normalized
}

func canonicalRouteModel(modelName string) string {
	trimmed := strings.TrimSpace(modelName)
	if trimmed == "" {
		return ""
	}
	if strings.HasPrefix(trimmed, "*:") {
		return "*"
	}
	return trimmed
}
