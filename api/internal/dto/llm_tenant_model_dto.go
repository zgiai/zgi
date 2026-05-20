package dto

// LLMTenantModelListRequest represents the request to list tenant LLM models
type LLMTenantModelListRequest struct {
	Page   int    `form:"page" binding:"omitempty,min=1"`
	Limit  int    `form:"limit" binding:"omitempty,min=1,max=100"`
	Search string `form:"search"`

	// Filter fields
	Provider           string `form:"provider"`
	UseCase            string `form:"use_case"`
	SupportsAttachment *bool  `form:"supports_attachment"`
	SupportsReasoning  *bool  `form:"supports_reasoning"`
	SupportsToolCall   *bool  `form:"supports_tool_call"`
	InputModalities    string `form:"input_modalities"`
	OutputModalities   string `form:"output_modalities"`
	IsEnabled          *bool  `form:"is_enabled"`
}

// LLMTenantModelResponse represents the response for a tenant LLM model
type LLMTenantModelResponse struct {
	ID                       string   `json:"id"`
	Provider                 string   `json:"provider"`
	Name                     string   `json:"name"`
	DisplayName              string   `json:"display_name"`
	UseCases                 []string `json:"use_cases"`
	SupportsAttachment       bool     `json:"supports_attachment"`
	SupportsReasoning        bool     `json:"supports_reasoning"`
	SupportsToolCall         bool     `json:"supports_tool_call"`
	SupportsStructuredOutput bool     `json:"supports_structured_output"`
	SupportsTemperature      bool     `json:"supports_temperature"`
	SupportedParameters      []string `json:"supported_parameters"`
	IsEnabled                bool     `json:"is_enabled"`
	InputModalities          []string `json:"input_modalities"`
	OutputModalities         []string `json:"output_modalities"`
	OpenWeights              bool     `json:"open_weights"`
	InputPrice                float64  `json:"input_price"`
	OutputPrice               float64  `json:"output_price"`
	CostCacheRead            float64  `json:"cost_cache_read"`
	CostCacheWrite           float64  `json:"cost_cache_write"`
	ContextWindow             int      `json:"context_window"`
	MaxOutputTokens              int      `json:"max_output_tokens"`
}

// LLMTenantModelListResponse represents the response for listing tenant LLM models
type LLMTenantModelListResponse struct {
	Items   []LLMTenantModelResponse `json:"items"`
	Total   int64                    `json:"total"`
	Page    int                      `json:"page"`
	Limit   int                      `json:"limit"`
	HasMore bool                     `json:"has_more"`
}

// LocalizedString represents a localized string
type LocalizedString struct {
	ZhHans string `json:"zh_Hans"`
	EnUS   string `json:"en_US"`
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

// BatchToggleModelsRequest represents the request to batch toggle specific models
type BatchToggleModelsRequest struct {
	Provider  string   `json:"provider" binding:"required"`
	Models    []string `json:"models" binding:"required,min=1"`
	IsEnabled bool     `json:"is_enabled"`
}
