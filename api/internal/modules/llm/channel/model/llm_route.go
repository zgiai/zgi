package model

import (
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/zgiai/zgi/api/internal/modules/llm/catalogvendor"
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
	PlatformAPIKey              string              `gorm:"-" json:"-"`
	UpstreamGeneration          int64               `gorm:"-" json:"-"`
	UpstreamWouldGuard          bool                `gorm:"-" json:"-"`
	UpstreamHalfOpen            bool                `gorm:"-" json:"-"`
	UpstreamProbe               bool                `gorm:"-" json:"-"`
	UpstreamProbeRequiresBackup bool                `gorm:"-" json:"-"`
	OfficialProviderModels      []ProviderModel     `gorm:"-" json:"-"`
	OfficialModelProtocols      map[string][]string `gorm:"-" json:"-"`
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
	OpenAIChatCompletions NativeProtocolEndpoint `json:"openai_chat_completions,omitempty"`
	OpenAIResponses       NativeProtocolEndpoint `json:"openai_responses,omitempty"`
	AnthropicMessages     NativeProtocolEndpoint `json:"anthropic_messages,omitempty"`
	GeminiGenerateContent NativeProtocolEndpoint `json:"gemini_generate_content,omitempty"`
	ModelProtocols        map[string][]string    `json:"model_protocols,omitempty"`
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

// SupportsModelForProvider keeps model discovery scoped to the logical model.
// Supplier identity must not make the same platform model appear or disappear.
//
// This answers "does this route serve this logical model at all". To ask
// whether a route can serve one exact platform model, use SupportsPlatformModel
// instead: that is the boundary invocation enforces.
func (r *LLMRoute) SupportsModelForProvider(provider, modelName string) bool {
	if !r.IsOfficial && r.Type != shared.RouteTypeZGICloud {
		return r.SupportsModel(modelName)
	}
	if strings.TrimSpace(provider) == "" {
		return false
	}
	return r.supportsOfficialLogicalModel(modelName)
}

// supportsOfficialLogicalModel reports whether an official route serves the
// logical model under any supplier. The flat Models list can lag behind a
// catalog sync, so the advertised provider/model pairs are consulted as well;
// consulting only the flat list lets a stale snapshot hide a model the route
// demonstrably serves.
func (r *LLMRoute) supportsOfficialLogicalModel(modelName string) bool {
	if r.SupportsModel(modelName) {
		return true
	}
	target := canonicalRouteModel(modelName)
	if target == "" {
		return false
	}
	for _, pair := range r.OfficialProviderModels {
		switch canonicalRouteModel(pair.Model) {
		case target, "*":
			return true
		}
	}
	return false
}

// SupportsPlatformModel reports whether the route can serve the exact platform
// model identified by provider and name. Official routes are validated against
// the supplier/model pairs Console advertises; private routes keep their
// model-list behavior.
//
// This is the boundary the router enforces at invocation time, so catalog and
// availability layers must use it too. Using the looser
// SupportsModelForProvider there makes them advertise routes the router will
// then reject, which surfaces as "the catalog says supported but the call has
// no candidate".
func (r *LLMRoute) SupportsPlatformModel(provider, modelName string) bool {
	if !r.IsOfficial && r.Type != shared.RouteTypeZGICloud {
		return r.SupportsModel(modelName)
	}
	if r.SupportsOfficialProviderModel(provider, modelName) {
		return true
	}
	// Before giving up, the router re-resolves the model to another supplier of
	// the same logical vendor that this route does advertise. Availability has
	// to grant the same tolerance, otherwise it hides a model that is in fact
	// callable through a sibling supplier.
	return r.servesModelUnderSameVendor(provider, modelName)
}

// servesModelUnderSameVendor mirrors the router's same-vendor fallback: the
// route serves this model name through a different supplier that maps to the
// same logical vendor as the requested one.
func (r *LLMRoute) servesModelUnderSameVendor(provider, modelName string) bool {
	target := canonicalRouteModel(modelName)
	requestedVendor := logicalModelVendor(provider, modelName)
	if target == "" || requestedVendor == "" {
		return false
	}
	for _, pair := range r.OfficialProviderModels {
		pairProvider := strings.TrimSpace(pair.Provider)
		if pairProvider == "" || canonicalRouteModel(pair.Model) != target {
			continue
		}
		if logicalModelVendor(pairProvider, pair.Model) == requestedVendor {
			return true
		}
	}
	return false
}

func logicalModelVendor(provider, modelName string) string {
	if vendor := strings.TrimSpace(catalogvendor.Lookup(provider, modelName)); vendor != "" {
		return strings.ToLower(vendor)
	}
	return strings.ToLower(strings.TrimSpace(provider))
}

// SupportsOfficialProviderModel validates the exact supplier/model capability
// advertised by Console. Invocation routing uses this stricter boundary after
// model-only discovery has completed.
func (r *LLMRoute) SupportsOfficialProviderModel(provider, modelName string) bool {
	provider = strings.TrimSpace(provider)
	modelName = strings.TrimSpace(modelName)
	if provider == "" || modelName == "" {
		return false
	}
	if len(r.OfficialProviderModels) > 0 {
		for _, pair := range r.OfficialProviderModels {
			pairProvider := strings.TrimSpace(pair.Provider)
			pairModel := canonicalRouteModel(pair.Model)
			if (pairProvider == provider || pairProvider == "*") && (pairModel == modelName || pairModel == "*") {
				return true
			}
		}
		return false
	}

	return r.SupportsModel(modelName)
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
