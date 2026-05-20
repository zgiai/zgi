package dto

import (
	"github.com/google/uuid"
	"github.com/zgiai/ginext/internal/modules/llm/llmmodel/model"
)

// CreateModelRequest is the request for creating a global model
type CreateModelRequest struct {
	ProviderID           uuid.UUID              `json:"provider_id" binding:"required"`
	Provider             string                 `json:"provider" binding:"required"`
	Name                 string                 `json:"name" binding:"required"`
	DisplayName          string                 `json:"display_name" binding:"required"`
	UseCases             []string               `json:"use_cases"` // Usage scenarios
	ContextWindow        int                    `json:"context_window"`
	MaxOutputTokens      int                    `json:"max_output_tokens"`
	InputPrice           string                 `json:"input_price"`
	OutputPrice          string                 `json:"output_price"`
	SupportsVision       bool                   `json:"supports_vision"`
	SupportsAudio        bool                   `json:"supports_audio"`
	SupportsToolCall     bool                   `json:"supports_tool_call"`
	SupportsFunctionCall bool                   `json:"supports_function_call"`
	SupportsJsonMode     bool                   `json:"supports_json_mode"`
	SupportsStreaming    bool                   `json:"supports_streaming"`
	SupportsReasoning    bool                   `json:"supports_reasoning"`
	TemperatureMin       *float64               `json:"temperature_min"`
	TemperatureMax       *float64               `json:"temperature_max"`
	TemperatureDefault   *float64               `json:"temperature_default"`
	ConfigParameters     model.ConfigParameters `json:"config_parameters"`
	KnowledgeCutoff      string                 `json:"knowledge_cutoff"`
	Description          string                 `json:"description"`
}

// UpdateModelRequest is the request for updating a global model
type UpdateModelRequest struct {
	DisplayName          *string                 `json:"display_name"`
	ContextWindow        *int                    `json:"context_window"`
	MaxOutputTokens      *int                    `json:"max_output_tokens"`
	ModelTier            *string                 `json:"model_tier"`     // flagship/premium/standard/basic
	IsRecommended        *bool                   `json:"is_recommended"` // Recommended flag
	InputPrice           *string                 `json:"input_price"`
	OutputPrice          *string                 `json:"output_price"`
	SupportsVision       *bool                   `json:"supports_vision"`
	SupportsAudio        *bool                   `json:"supports_audio"`
	SupportsToolCall     *bool                   `json:"supports_tool_call"`
	SupportsFunctionCall *bool                   `json:"supports_function_call"`
	SupportsJsonMode     *bool                   `json:"supports_json_mode"`
	SupportsStreaming    *bool                   `json:"supports_streaming"`
	SupportsReasoning    *bool                   `json:"supports_reasoning"`
	TemperatureMin       *float64                `json:"temperature_min"`
	TemperatureMax       *float64                `json:"temperature_max"`
	TemperatureDefault   *float64                `json:"temperature_default"`
	ConfigParameters     *model.ConfigParameters `json:"config_parameters"`
	KnowledgeCutoff      *string                 `json:"knowledge_cutoff"`
	Description          *string                 `json:"description"`
	IsActive             *bool                   `json:"is_active"`
	SortOrder            *int                    `json:"sort_order"`
	UseCases             []string                `json:"use_cases"` // Usage scenarios
}

// ListModelRequest is the request for listing models
type ListModelRequest struct {
	ProviderID *uuid.UUID `form:"provider_id"`
	Provider   string     `form:"provider"`  // Filter by provider name (e.g., "openai", "anthropic")
	UseCase    string     `form:"use_case"`  // Filter by use case (e.g., "text-chat", "embedding")
	UseCases   []string   `form:"use_cases"` // Filter by usage scenarios (comma-separated)
	IsActive   *bool      `form:"is_active"`
	Page       int        `form:"page,default=1"`
	PageSize   int        `form:"page_size,default=1000"`
}

// ConfigureModelRequest is the request for configuring a tenant's model
type ConfigureModelRequest struct {
	ModelID             uuid.UUID `json:"model_id" binding:"required"`
	IsEnabled           *bool     `json:"is_enabled"`
	CustomDisplayName   string    `json:"custom_display_name"`
	InputPriceOverride  *string   `json:"input_price_override"`
	OutputPriceOverride *string   `json:"output_price_override"`
	AccessScope         string    `json:"access_scope"`
	VisibleGroups       []string  `json:"visible_groups"`
	VisibleUsers        []string  `json:"visible_users"`
	SortOrder           *int      `json:"sort_order"`
}

// CreateCustomModelRequest is the request for creating a tenant's custom model.
// Only 4 fields are required; capabilities are auto-inferred from use_cases.
// Advanced users can override via endpoints/features/tools objects.
type CreateCustomModelRequest struct {
	// Required fields (4)
	Provider    string   `json:"provider" binding:"required"`   // Provider slug (e.g. "openai")
	Name        string   `json:"model" binding:"required"`      // Model identifier
	DisplayName string   `json:"model_name" binding:"required"` // Display name
	UseCases    []string `json:"use_cases" binding:"required"`  // Primary classification

	// Optional: backward compat
	ProviderID *uuid.UUID `json:"provider_id"` // Deprecated: use provider slug

	// Optional: specifications
	ContextWindow   int    `json:"context_window"`
	MaxOutputTokens int    `json:"max_output_tokens"`
	InputPrice      string `json:"input_price"`
	OutputPrice     string `json:"output_price"`
	KnowledgeCutoff string `json:"knowledge_cutoff"`
	Description     string `json:"description"`

	// Optional: capability overrides (nil = auto-infer from use_cases)
	Endpoints        *model.ModelEndpoints  `json:"endpoints"`
	Features         *model.ModelFeatures   `json:"features"`
	Tools            *model.ModelTools      `json:"tools"`
	Parameters       *model.ModelParameters `json:"parameters"`
	ConfigParameters model.ConfigParameters `json:"config_parameters"`
}

// UpdateCustomModelRequest is the request for updating a tenant's custom model
type UpdateCustomModelRequest struct {
	DisplayName     *string  `json:"model_name"`
	ContextWindow   *int     `json:"context_window"`
	MaxOutputTokens *int     `json:"max_output_tokens"`
	InputPrice      *string  `json:"input_price"`
	OutputPrice     *string  `json:"output_price"`
	KnowledgeCutoff *string  `json:"knowledge_cutoff"`
	Description     *string  `json:"description"`
	IsActive        *bool    `json:"is_active"`
	SortOrder       *int     `json:"sort_order"`
	UseCases        []string `json:"use_cases"`

	// Capability overrides (nil = no change)
	Endpoints        *model.ModelEndpoints   `json:"endpoints"`
	Features         *model.ModelFeatures    `json:"features"`
	Tools            *model.ModelTools       `json:"tools"`
	Parameters       *model.ModelParameters  `json:"parameters"`
	ConfigParameters *model.ConfigParameters `json:"config_parameters"`
}

// LocalizedString represents a localized string with BCP 47 compliant keys
type LocalizedString struct {
	EnUS   string `json:"en_US,omitempty"`
	ZhHans string `json:"zh_Hans,omitempty"`
}

// LLMModelParameter represents a model parameter definition
type LLMModelParameter struct {
	Name        string          `json:"name"`
	UseTemplate string          `json:"use_template,omitempty"`
	Label       LocalizedString `json:"label"`
	Type        string          `json:"type"`
	Help        LocalizedString `json:"help"`
	Required    bool            `json:"required"`
	Default     interface{}     `json:"default,omitempty"`
	Min         *float64        `json:"min,omitempty"`
	Max         *float64        `json:"max,omitempty"`
	Precision   *int            `json:"precision,omitempty"`
	Options     []string        `json:"options,omitempty"`
}

// LLMModelParametersRequest represents the request to get model parameters
type LLMModelParametersRequest struct {
	Provider string `form:"provider" binding:"required"`
	Model    string `form:"model" binding:"required"`
}

// ToggleProviderModelsRequest represents the request to toggle all models for a provider
type ToggleProviderModelsRequest struct {
	Provider  string `json:"provider" binding:"required"`
	IsEnabled bool   `json:"is_enabled"`
}

// BatchToggleModelsRequest represents the request to batch toggle models
type BatchToggleModelsRequest struct {
	Provider  string   `json:"provider" binding:"required"`
	Models    []string `json:"models" binding:"required"`
	IsEnabled bool     `json:"is_enabled"`
}

// TenantModelListResponse is the paginated response for tenant models
type TenantModelListResponse struct {
	Items      interface{} `json:"items"`
	Total      int64       `json:"total"`
	Page       int         `json:"page"`
	PageSize   int         `json:"page_size"`
	TotalPages int         `json:"total_pages"`
}

// ModelAvailabilityResponse represents the availability status of a model
type ModelAvailabilityResponse struct {
	Available    bool   `json:"available"`
	ChannelCount int    `json:"channel_count"`
	Message      string `json:"message,omitempty"`
}

// BatchModelAvailabilityRequest is the request for batch model availability check
type BatchModelAvailabilityRequest struct {
	Models []string `json:"models" binding:"required"`
}

// BatchModelAvailabilityResponse is the response for batch model availability check
type BatchModelAvailabilityResponse struct {
	Items map[string]*ModelAvailabilityResponse `json:"items"`
}
