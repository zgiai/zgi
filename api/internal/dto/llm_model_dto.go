package dto

import "time"

// LLMModelCreateRequest represents the request to create a new LLM model
type LLMModelCreateRequest struct {
	Provider                 string                 `json:"provider" binding:"required"`
	Name                     string                 `json:"name" binding:"required"`
	DisplayName              string                 `json:"display_name" binding:"required"`
	SupportsAttachment       bool                   `json:"supports_attachment"`
	SupportsReasoning        bool                   `json:"supports_reasoning"`
	SupportsToolCall         bool                   `json:"supports_tool_call"`
	SupportsStructuredOutput bool                   `json:"supports_structured_output"`
	SupportsTemperature      bool                   `json:"supports_temperature"`
	KnowledgeCutoff          string                 `json:"knowledge_cutoff"`
	ReleaseDate              *time.Time             `json:"release_date"`
	LastUpdated              *time.Time             `json:"last_updated"`
	InputModalities          []string               `json:"input_modalities"`
	OutputModalities         []string               `json:"output_modalities"`
	OpenWeights              bool                   `json:"open_weights"`
	InputPrice                float64                `json:"input_price"`
	OutputPrice               float64                `json:"output_price"`
	CostCacheRead            float64                `json:"cost_cache_read"`
	CostCacheWrite           float64                `json:"cost_cache_write"`
	CostContextOver200k      map[string]interface{} `json:"cost_context_over_200k"`
	ContextWindow             int                    `json:"context_window"`
	MaxOutputTokens              int                    `json:"max_output_tokens"`
	IsActive                 *bool                  `json:"is_active"`
	Description              string                 `json:"description"`
}

// LLMModelUpdateRequest represents the request to update an existing LLM model
type LLMModelUpdateRequest struct {
	DisplayName              *string                `json:"display_name"`
	Description              *string                `json:"description"`
	SupportsAttachment       *bool                  `json:"supports_attachment"`
	SupportsReasoning        *bool                  `json:"supports_reasoning"`
	SupportsToolCall         *bool                  `json:"supports_tool_call"`
	SupportsStructuredOutput *bool                  `json:"supports_structured_output"`
	SupportsTemperature      *bool                  `json:"supports_temperature"`
	KnowledgeCutoff          *string                `json:"knowledge_cutoff"`
	ReleaseDate              *time.Time             `json:"release_date"`
	LastUpdated              *time.Time             `json:"last_updated"`
	InputModalities          []string               `json:"input_modalities"`
	OutputModalities         []string               `json:"output_modalities"`
	OpenWeights              *bool                  `json:"open_weights"`
	InputPrice                *float64               `json:"input_price"`
	OutputPrice               *float64               `json:"output_price"`
	CostCacheRead            *float64               `json:"cost_cache_read"`
	CostCacheWrite           *float64               `json:"cost_cache_write"`
	CostContextOver200k      map[string]interface{} `json:"cost_context_over_200k"`
	ContextWindow             *int                   `json:"context_window"`
	MaxOutputTokens              *int                   `json:"max_output_tokens"`
	IsActive                 *bool                  `json:"is_active"`
}

// LLMModelResponse represents the response for a single LLM model
type LLMModelResponse struct {
	ID                       string     `json:"id"` // UUID as string
	Provider                 string     `json:"provider"`
	Name                     string     `json:"name"`
	DisplayName              string     `json:"display_name"`
	SupportsAttachment       bool       `json:"supports_attachment"`
	SupportsReasoning        bool       `json:"supports_reasoning"`
	SupportsToolCall         bool       `json:"supports_tool_call"`
	SupportsStructuredOutput bool       `json:"supports_structured_output"`
	SupportsTemperature      bool       `json:"supports_temperature"`
	KnowledgeCutoff          string     `json:"knowledge_cutoff"`
	ReleaseDate              *time.Time `json:"release_date"`
	LastUpdated              *time.Time `json:"last_updated"`
	InputModalities          []string   `json:"input_modalities"`
	OutputModalities         []string   `json:"output_modalities"`
	OpenWeights              bool       `json:"open_weights"`
	InputPrice                float64    `json:"input_price"`
	OutputPrice               float64    `json:"output_price"`
	CostCacheRead            float64    `json:"cost_cache_read"`
	CostCacheWrite           float64    `json:"cost_cache_write"`
	ContextWindow             int        `json:"context_window"`
	MaxOutputTokens              int        `json:"max_output_tokens"`
	IsActive                 bool       `json:"is_active"`
	CreatedAt                time.Time  `json:"created_at"`
	UpdatedAt                time.Time  `json:"updated_at"`
}

// LLMModelListRequest represents the request to list LLM models
type LLMModelListRequest struct {
	Page     int    `form:"page" binding:"omitempty,min=1"`
	Limit    int    `form:"limit" binding:"omitempty,max=100"`
	IsActive *bool  `form:"is_active"`
	Search   string `form:"search"`

	// Exact match filters
	Provider string `form:"provider"`
	Name     string `form:"name"`

	// Boolean capability filters
	SupportsAttachment       *bool `form:"supports_attachment"`
	SupportsReasoning        *bool `form:"supports_reasoning"`
	SupportsToolCall         *bool `form:"supports_tool_call"`
	SupportsStructuredOutput *bool `form:"supports_structured_output"`
	SupportsTemperature      *bool `form:"supports_temperature"`

	// Array filters (comma-separated values)
	InputModalities  string `form:"input_modalities"`  // e.g., "text,image"
	OutputModalities string `form:"output_modalities"` // e.g., "text,audio"

	// Cost range filters (per million tokens in USD)
	InputPriceMin      *float64 `form:"input_price_min"`
	InputPriceMax      *float64 `form:"input_price_max"`
	OutputPriceMin     *float64 `form:"output_price_min"`
	OutputPriceMax     *float64 `form:"output_price_max"`
	CostCacheReadMin  *float64 `form:"cost_cache_read_min"`
	CostCacheReadMax  *float64 `form:"cost_cache_read_max"`
	CostCacheWriteMin *float64 `form:"cost_cache_write_min"`
	CostCacheWriteMax *float64 `form:"cost_cache_write_max"`
}

// LLMModelListResponse represents the response for listing LLM models
type LLMModelListResponse struct {
	Items   []LLMModelResponse `json:"items"`
	Total   int64              `json:"total"`
	Page    int                `json:"page"`
	Limit   int                `json:"limit"`
	HasMore bool               `json:"has_more"`
}
