package dto

import (
	"time"

	"github.com/google/uuid"
)

// LLMProviderCreateRequest represents the request to create a new LLM provider
type LLMProviderCreateRequest struct {
	Name             string  `json:"name" binding:"required"` // Unique identifier (e.g., "openai", "deepseek")
	DisplayName      string  `json:"display_name" binding:"required"`
	Description      string  `json:"description"`
	APIBaseURL       string  `json:"api_base_url"`
	LogoURL          string  `json:"logo_url"`
	APIKey           string  `json:"api_key"`
	Balance          float64 `json:"balance"`
	Currency         string  `json:"currency"`
	IsActive         *bool   `json:"is_active"`
	OpenaiCompatible *bool   `json:"openai_compatible"`
	ProviderType     *string `json:"provider_type"` // Provider type: vendor, aggregator, cloud
}

// LLMProviderUpdateRequest represents the request to update an existing LLM provider
type LLMProviderUpdateRequest struct {
	Name             string  `json:"name" binding:"required"` // Provider name to identify which provider to update
	DisplayName      *string `json:"display_name"`
	Description      *string `json:"description"`
	APIBaseURL       *string `json:"api_base_url"`
	LogoURL          *string `json:"logo_url"`
	APIKey           *string `json:"api_key"`
	IsActive         *bool   `json:"is_active"`
	OpenaiCompatible *bool   `json:"openai_compatible"`
	ProviderType     *string `json:"provider_type"` // Provider type: vendor, aggregator, cloud
}

// LLMProviderModelInfo represents model information associated with a provider
type LLMProviderModelInfo struct {
	ID           uuid.UUID `json:"id"`
	Name         string    `json:"name"`          // Unique identifier (e.g., "gpt-4o", "deepseek-chat")
	DisplayName  string    `json:"display_name"`  // Display name
	Provider     string    `json:"provider"`      // Provider name
	IsActive     bool      `json:"is_active"`     // Whether model is available
	InputPrice    float64   `json:"input_price"`    // Cost per million input tokens
	OutputPrice   float64   `json:"output_price"`   // Cost per million output tokens
	ContextWindow int       `json:"context_window"` // Maximum context window size
	MaxOutputTokens  int       `json:"max_output_tokens"`  // Maximum output tokens
}

// LLMProviderResponse represents the response for a single LLM provider
type LLMProviderResponse struct {
	ID               uuid.UUID `json:"id"`
	Name             string    `json:"name"` // Unique identifier (e.g., "openai", "deepseek")
	DisplayName      string    `json:"display_name"`
	Description      string    `json:"description"`
	APIBaseURL       string    `json:"api_base_url"`
	LogoURL          string    `json:"logo_url,omitempty"`
	APIKey           string    `json:"api_key,omitempty"` // Only for admin/testing
	Balance          float64   `json:"balance"`
	Currency         string    `json:"currency"`
	IsActive         bool      `json:"is_active"`
	OpenaiCompatible bool      `json:"openai_compatible"`
	ProviderType     string    `json:"provider_type"` // Provider type: vendor, aggregator, cloud
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

// LLMProviderListRequest represents the request to list LLM providers
type LLMProviderListRequest struct {
	Page     int    `form:"page" binding:"omitempty,min=1"`
	Limit    int    `form:"limit" binding:"omitempty,min=1,max=100"`
	IsActive *bool  `form:"is_active"`
	Search   string `form:"search"`
}

// LLMProviderListResponse represents the response for listing LLM providers
type LLMProviderListResponse struct {
	Items   []LLMProviderResponse `json:"items"`
	Total   int64                 `json:"total"`
	Page    int                   `json:"page"`
	Limit   int                   `json:"limit"`
	HasMore bool                  `json:"has_more"`
}

// LLMProviderSyncResponse represents the response for syncing models
type LLMProviderSyncResponse struct {
	ModelsCreated int `json:"models_created"`
	ModelsUpdated int `json:"models_updated"`
	TotalModels   int `json:"total_models"`
}

// LLMPatchResponse represents the response for patching models
type LLMPatchResponse struct {
	ModelsPatched int      `json:"models_patched"`
	TotalModels   int      `json:"total_models"`
	Errors        []string `json:"errors,omitempty"`
}

// LLMProviderUpstreamModelRequest represents the request to fetch upstream models
type LLMProviderUpstreamModelRequest struct {
	Provider string `json:"provider" binding:"required"`
}

// LLMProviderUpstreamModel represents a model from upstream provider API
type LLMProviderUpstreamModel struct {
	ID                  string                   `json:"id"`
	Name                string                   `json:"name"`
	Type                string                   `json:"type,omitempty"`
	Description         string                   `json:"description,omitempty"`
	ContextLength       int                      `json:"context_length"`
	Capabilities        []string                 `json:"capabilities,omitempty"`
	Pricing             *LLMUpstreamPricing      `json:"pricing,omitempty"`
	Architecture        *LLMUpstreamArchitecture `json:"architecture,omitempty"`
	SupportedParameters []string                 `json:"supported_parameters,omitempty"`
	DefaultParameters   map[string]interface{}   `json:"default_parameters,omitempty"`
	IsModerated         bool                     `json:"is_moderated,omitempty"`
	Endpoints           []string                 `json:"endpoints,omitempty"`
	IsFinetuned         bool                     `json:"is_finetuned,omitempty"`
}

// LLMUpstreamPricing represents pricing information from upstream
type LLMUpstreamPricing struct {
	Prompt          float64 `json:"prompt"`
	Completion      float64 `json:"completion"`
	Image           float64 `json:"image,omitempty"`
	Audio           float64 `json:"audio,omitempty"`
	InputCacheRead  float64 `json:"input_cache_read,omitempty"`
	InputCacheWrite float64 `json:"input_cache_write,omitempty"`
}

// LLMUpstreamArchitecture represents architecture information from upstream
type LLMUpstreamArchitecture struct {
	InputModalities  []string `json:"input_modalities,omitempty"`
	OutputModalities []string `json:"output_modalities,omitempty"`
	Tokenizer        string   `json:"tokenizer,omitempty"`
	InstructType     string   `json:"instruct_type,omitempty"`
}

// LLMProviderUpstreamModelsResponse represents the response for fetching upstream models
type LLMProviderUpstreamModelsResponse struct {
	Provider string                     `json:"provider"`
	Models   []LLMProviderUpstreamModel `json:"models"`
	Total    int                        `json:"total"`
}
