package model

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

// ============================================================================
// Model Config - organization's configuration for global models
// ============================================================================

// ModelConfig represents organization-specific configuration for a global model
type ModelConfig struct {
	ID                  uuid.UUID              `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"id"`
	OrganizationID      uuid.UUID              `gorm:"type:uuid;not null;index;column:organization_id" json:"organization_id"`
	ModelID             uuid.UUID              `gorm:"type:uuid;not null" json:"model_id"`
	IsEnabled           bool                   `gorm:"default:true;index" json:"is_enabled"`
	CustomDisplayName   string                 `gorm:"type:varchar(200)" json:"custom_display_name,omitempty"`
	InputPriceOverride  *decimal.Decimal       `gorm:"type:decimal(10,4)" json:"input_price_override,omitempty"`
	OutputPriceOverride *decimal.Decimal       `gorm:"type:decimal(10,4)" json:"output_price_override,omitempty"`
	AccessScope         AccessScope            `gorm:"type:varchar(20);default:'all'" json:"access_scope"`
	VisibleGroups       []string               `gorm:"type:jsonb;serializer:json;default:'[]'" json:"visible_groups"`
	VisibleUsers        []string               `gorm:"type:jsonb;serializer:json;default:'[]'" json:"visible_users"`
	SortOrder           int                    `gorm:"default:0" json:"sort_order"`
	Metadata            map[string]interface{} `gorm:"type:jsonb;serializer:json;default:'{}'" json:"metadata,omitempty"`
	CreatedAt           time.Time              `gorm:"not null;default:CURRENT_TIMESTAMP" json:"created_at"`
	UpdatedAt           time.Time              `gorm:"not null;default:CURRENT_TIMESTAMP" json:"updated_at"`
	DeletedAt           gorm.DeletedAt         `gorm:"index" json:"deleted_at,omitempty"`

	// Relations
	Model *LLMModel `gorm:"foreignKey:ModelID" json:"model,omitempty"`
}

func (ModelConfig) TableName() string {
	return "llm_model_configs"
}

func (c *ModelConfig) BeforeCreate(tx *gorm.DB) error {
	if c.ID == uuid.Nil {
		c.ID = uuid.New()
	}
	return nil
}

// GetEffectiveDisplayName returns the custom display name or falls back to model's display name
func (c *ModelConfig) GetEffectiveDisplayName() string {
	if c.CustomDisplayName != "" {
		return c.CustomDisplayName
	}
	if c.Model != nil {
		return c.Model.ModelName
	}
	return ""
}

// GetEffectiveInputPrice returns the overridden price or falls back to model's price
func (c *ModelConfig) GetEffectiveInputPrice() decimal.Decimal {
	if c.InputPriceOverride != nil {
		return *c.InputPriceOverride
	}
	if c.Model != nil {
		return c.Model.InputPrice
	}
	return decimal.Zero
}

// GetEffectiveOutputPrice returns the overridden price or falls back to model's price
func (c *ModelConfig) GetEffectiveOutputPrice() decimal.Decimal {
	if c.OutputPriceOverride != nil {
		return *c.OutputPriceOverride
	}
	if c.Model != nil {
		return c.Model.OutputPrice
	}
	return decimal.Zero
}

// ============================================================================
// Custom Model - organization's own custom models
// ============================================================================

// CustomModel represents a custom model created by an organization.
// Field layout is strictly aligned with LLMModel for API response parity.
type CustomModel struct {
	ID             uuid.UUID `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"id"`
	OrganizationID uuid.UUID `gorm:"type:uuid;not null;index;column:organization_id" json:"organization_id"`
	ProviderID     uuid.UUID `gorm:"type:uuid;not null;index" json:"provider_id"` // References llm_custom_providers
	Provider       string    `gorm:"type:varchar(100)" json:"provider"`           // Provider slug (e.g. "openai")

	// Basic info (aligned with LLMModel)
	Name        string      `gorm:"type:varchar(100);not null" json:"model"`
	DisplayName string      `gorm:"type:varchar(200);not null" json:"model_name"`
	Family      string      `gorm:"type:varchar(100)" json:"family,omitempty"`
	Status      string      `gorm:"type:varchar(20);default:'active'" json:"status"`
	Tagline     string      `gorm:"type:text" json:"tagline,omitempty"`
	Description string      `gorm:"type:text" json:"description,omitempty"`
	UseCases    StringArray `gorm:"type:text[];default:'{}'" json:"use_cases"`

	// Flags (aligned with LLMModel)
	IsFlagship bool   `gorm:"default:false" json:"is_flagship"`
	IsFeatured bool   `gorm:"default:false" json:"is_featured"`
	IsNew      bool   `gorm:"default:false" json:"is_new"`
	AccessType string `gorm:"type:varchar(20);default:'closed'" json:"access_type"`
	Currency   string `gorm:"type:varchar(10);default:'USD'" json:"currency"`

	// Model type and capabilities (aligned with LLMModel)
	SupportsReasoning        bool `gorm:"column:reasoning;default:false" json:"-"`
	SupportsToolCall         bool `gorm:"column:function_calling;default:false" json:"-"`
	SupportsStructuredOutput bool `gorm:"column:structured_output;default:false" json:"-"`

	// Parameter capabilities (aligned with LLMModel)
	SupportsTemperature      bool `gorm:"column:temperature;default:true" json:"-"`
	SupportsTopP             bool `gorm:"column:top_p;default:true" json:"-"`
	SupportsPresencePenalty  bool `gorm:"column:presence_penalty;default:false" json:"-"`
	SupportsFrequencyPenalty bool `gorm:"column:frequency_penalty;default:false" json:"-"`
	SupportsLogitBias        bool `gorm:"column:logit_bias;default:false" json:"-"`
	SupportsSeed             bool `gorm:"column:seed;default:false" json:"-"`
	SupportsStop             bool `gorm:"column:stop;default:true" json:"-"`
	MaxStopSequences         int  `gorm:"column:max_stop_sequences;default:4" json:"-"`

	// Legacy / Compatibility fields (aligned with LLMModel)
	SupportsVision    bool `gorm:"column:vision;default:false" json:"-"`
	SupportsAudio     bool `gorm:"column:audio;default:false" json:"-"`
	SupportsJsonMode  bool `gorm:"column:json_mode;default:false" json:"-"`
	SupportsStreaming bool `gorm:"column:streaming;default:true" json:"-"`

	// Endpoint capabilities (aligned with LLMModel)
	ChatCompletions   bool `gorm:"column:chat_completions;default:true" json:"-"`
	Embeddings        bool `gorm:"column:embeddings;default:false" json:"-"`
	ImageGeneration   bool `gorm:"column:image_generation;default:false" json:"-"`
	SpeechGeneration  bool `gorm:"column:speech_generation;default:false" json:"-"`
	Transcription     bool `gorm:"column:transcription;default:false" json:"-"`
	Translation       bool `gorm:"column:translation;default:false" json:"-"`
	Moderation        bool `gorm:"column:moderation;default:false" json:"-"`
	Realtime          bool `gorm:"column:realtime;default:false" json:"-"`
	Batch             bool `gorm:"column:batch;default:false" json:"-"`
	FineTuning        bool `gorm:"column:fine_tuning;default:false" json:"-"`
	Assistants        bool `gorm:"column:assistants;default:false" json:"-"`
	Responses         bool `gorm:"column:responses;default:false" json:"-"`
	Distillation      bool `gorm:"column:distillation;default:false" json:"-"`
	SystemPrompt      bool `gorm:"column:system_prompt;default:true" json:"-"`
	Logprobs          bool `gorm:"column:logprobs;default:false" json:"-"`
	WebSearch         bool `gorm:"column:web_search;default:false" json:"-"`
	FileSearch        bool `gorm:"column:file_search;default:false" json:"-"`
	CodeInterpreter   bool `gorm:"column:code_interpreter;default:false" json:"-"`
	ComputerUse       bool `gorm:"column:computer_use;default:false" json:"-"`
	Mcp               bool `gorm:"column:mcp;default:false" json:"-"`
	ParallelToolCalls bool `gorm:"column:parallel_tool_calls;default:false" json:"-"`
	ReasoningEffort   bool `gorm:"column:reasoning_effort;default:false" json:"-"`

	// Modalities
	InputModalities  JSONArray `gorm:"column:input_modalities;type:jsonb" json:"-"`
	OutputModalities JSONArray `gorm:"column:output_modalities;type:jsonb" json:"-"`

	// Model specifications
	KnowledgeCutoff     string               `gorm:"type:varchar(20)" json:"knowledge_cutoff,omitempty"`
	ContextWindow       int                  `gorm:"column:context_window" json:"context_window,omitempty"`
	MaxOutputTokens     int                  `gorm:"column:max_output_tokens" json:"max_output_tokens,omitempty"`
	MaxInputTokens      int                  `gorm:"column:max_input_tokens" json:"max_input_tokens,omitempty"`
	SupportedParameters ParameterDefinitions `gorm:"column:supported_parameters;type:jsonb" json:"supported_parameters,omitempty"`
	ConfigParameters    ConfigParameters     `gorm:"column:config_parameters;type:jsonb" json:"-"`
	DefaultParameters   JSONObject           `gorm:"column:default_parameters;type:jsonb" json:"default_parameters,omitempty"`

	// Pricing (per million tokens, aligned with LLMModel)
	InputPrice            decimal.Decimal `gorm:"column:input_price;type:decimal(10,4)" json:"input_price"`
	OutputPrice           decimal.Decimal `gorm:"column:output_price;type:decimal(10,4)" json:"output_price"`
	InputPriceConfigured  bool            `gorm:"column:input_price_configured;default:false" json:"input_price_configured"`
	OutputPriceConfigured bool            `gorm:"column:output_price_configured;default:false" json:"output_price_configured"`

	// Status and ordering
	IsActive  bool                   `gorm:"default:true;index" json:"is_active"`
	SortOrder int                    `gorm:"default:0" json:"sort_order"`
	Metadata  map[string]interface{} `gorm:"type:jsonb;serializer:json;default:'{}'" json:"metadata,omitempty"`
	CreatedAt time.Time              `gorm:"not null;default:CURRENT_TIMESTAMP" json:"created_at"`
	UpdatedAt time.Time              `gorm:"not null;default:CURRENT_TIMESTAMP" json:"updated_at"`
	DeletedAt gorm.DeletedAt         `gorm:"index" json:"deleted_at,omitempty"`

	// Nested capability objects (not stored in DB, populated via hooks)
	Endpoints  *ModelEndpoints  `gorm:"-" json:"endpoints,omitempty"`
	Features   *ModelFeatures   `gorm:"-" json:"features,omitempty"`
	Tools      *ModelTools      `gorm:"-" json:"tools,omitempty"`
	Parameters *ModelParameters `gorm:"-" json:"parameters,omitempty"`
}

func (CustomModel) TableName() string {
	return "llm_custom_models"
}

func (m *CustomModel) BeforeCreate(tx *gorm.DB) error {
	if m.ID == uuid.Nil {
		m.ID = uuid.New()
	}
	return nil
}

// AfterFind populates nested capability objects from flat database columns
func (m *CustomModel) AfterFind(tx *gorm.DB) error {
	m.Endpoints = &ModelEndpoints{
		ChatCompletions:  m.ChatCompletions,
		Responses:        m.Responses,
		Realtime:         m.Realtime,
		Assistants:       m.Assistants,
		Batch:            m.Batch,
		Embeddings:       m.Embeddings,
		FineTuning:       m.FineTuning,
		ImageGeneration:  m.ImageGeneration,
		Vision:           m.SupportsVision,
		SpeechGeneration: m.SpeechGeneration,
		Transcription:    m.Transcription,
		Translation:      m.Translation,
		Moderation:       m.Moderation,
	}
	m.Features = &ModelFeatures{
		Streaming:        m.SupportsStreaming,
		FunctionCalling:  m.SupportsToolCall,
		StructuredOutput: m.SupportsStructuredOutput,
		JsonMode:         m.SupportsJsonMode,
		Distillation:     m.Distillation,
		Reasoning:        m.SupportsReasoning,
		SystemPrompt:     m.SystemPrompt,
		Logprobs:         m.Logprobs,
		WebSearch:        m.WebSearch,
		FileSearch:       m.FileSearch,
		CodeInterpreter:  m.CodeInterpreter,
		ComputerUse:      m.ComputerUse,
		Mcp:              m.Mcp,
		ReasoningEffort:  m.ReasoningEffort,
	}
	m.Tools = &ModelTools{
		WebSearch:         m.WebSearch,
		FileSearch:        m.FileSearch,
		ImageGeneration:   m.ImageGeneration,
		CodeInterpreter:   m.CodeInterpreter,
		ComputerUse:       m.ComputerUse,
		Mcp:               m.Mcp,
		ParallelToolCalls: m.ParallelToolCalls,
	}
	m.Parameters = &ModelParameters{
		SupportsTemperature:      m.SupportsTemperature,
		SupportsTopP:             m.SupportsTopP,
		SupportsPresencePenalty:  m.SupportsPresencePenalty,
		SupportsFrequencyPenalty: m.SupportsFrequencyPenalty,
		SupportsLogitBias:        m.SupportsLogitBias,
		SupportsSeed:             m.SupportsSeed,
		SupportsStop:             m.SupportsStop,
		MaxStopSequences:         m.MaxStopSequences,
	}
	return nil
}

// BeforeSave syncs nested capability objects back to flat database columns
func (m *CustomModel) BeforeSave(tx *gorm.DB) error {
	if m.Endpoints != nil {
		m.ChatCompletions = m.Endpoints.ChatCompletions
		m.Responses = m.Endpoints.Responses
		m.Realtime = m.Endpoints.Realtime
		m.Assistants = m.Endpoints.Assistants
		m.Batch = m.Endpoints.Batch
		m.Embeddings = m.Endpoints.Embeddings
		m.FineTuning = m.Endpoints.FineTuning
		m.ImageGeneration = m.Endpoints.ImageGeneration
		m.SupportsVision = m.Endpoints.Vision
		m.SpeechGeneration = m.Endpoints.SpeechGeneration
		m.Transcription = m.Endpoints.Transcription
		m.Translation = m.Endpoints.Translation
		m.Moderation = m.Endpoints.Moderation
	}
	if m.Features != nil {
		m.SupportsStreaming = m.Features.Streaming
		m.SupportsToolCall = m.Features.FunctionCalling
		m.SupportsStructuredOutput = m.Features.StructuredOutput
		m.SupportsJsonMode = m.Features.JsonMode
		m.Distillation = m.Features.Distillation
		m.SupportsReasoning = m.Features.Reasoning
		m.SystemPrompt = m.Features.SystemPrompt
		m.Logprobs = m.Features.Logprobs
		m.WebSearch = m.Features.WebSearch
		m.FileSearch = m.Features.FileSearch
		m.CodeInterpreter = m.Features.CodeInterpreter
		m.ComputerUse = m.Features.ComputerUse
		m.Mcp = m.Features.Mcp
		m.ReasoningEffort = m.Features.ReasoningEffort
	}
	if m.Tools != nil {
		m.ParallelToolCalls = m.Tools.ParallelToolCalls
	}
	if m.Parameters != nil {
		m.SupportsTemperature = m.Parameters.SupportsTemperature
		m.SupportsTopP = m.Parameters.SupportsTopP
		m.SupportsPresencePenalty = m.Parameters.SupportsPresencePenalty
		m.SupportsFrequencyPenalty = m.Parameters.SupportsFrequencyPenalty
		m.SupportsLogitBias = m.Parameters.SupportsLogitBias
		m.SupportsSeed = m.Parameters.SupportsSeed
		m.SupportsStop = m.Parameters.SupportsStop
		m.MaxStopSequences = m.Parameters.MaxStopSequences
	}
	return nil
}

// ============================================================================
// Unified Model View - for API responses
// ============================================================================

// ModelView represents a unified view of model (global or custom)
// Aligned with ModelHub API structure
type ModelView struct {
	// Basic info
	ID            uuid.UUID  `json:"id"`
	Provider      string     `json:"provider"`
	Model         string     `json:"model"`
	ModelName     string     `json:"model_name"`
	Family        string     `json:"family"`                // Model family (e.g., GPT-4, Claude)
	FamilyName    string     `json:"family_name,omitempty"` // Model family display name
	ParentID      *uuid.UUID `json:"-"`                     // Internal use only
	FamilyDefault bool       `json:"-"`                     // Internal use only
	Status        string     `json:"status"`                // active, deprecated
	Tagline       string     `json:"tagline"`               // Short description

	// Flags
	IsFlagship    bool   `json:"is_flagship"`
	IsRecommended bool   `json:"is_recommended"`
	IsFeatured    bool   `json:"is_featured"`
	IsNew         bool   `json:"is_new"`
	AccessType    string `json:"access_type"` // open, closed
	OpenWeights   bool   `json:"-"`           // Internal use only

	// Pricing (per million tokens)
	Currency              string  `json:"currency"`
	InputPrice            float64 `json:"input_price"`  // Price per million input tokens
	OutputPrice           float64 `json:"output_price"` // Price per million output tokens
	InputPriceConfigured  bool    `json:"input_price_configured"`
	OutputPriceConfigured bool    `json:"output_price_configured"`
	CachedInputPrice      float64 `json:"cached_input_price"`

	// Context
	ContextWindow   int `json:"context_window"`
	MaxOutputTokens int `json:"max_output_tokens"`
	MaxInputTokens  int `json:"-"` // Internal use only

	// Capabilities (ModelHub-aligned nested structures)
	Endpoints  ModelEndpoints  `json:"endpoints"`
	Features   ModelFeatures   `json:"features"`
	Tools      ModelTools      `json:"tools"`
	Parameters ModelParameters `json:"parameters"`

	// Arrays
	UseCases         []string `json:"use_cases"`
	InputModalities  []string `json:"input_modalities"`
	OutputModalities []string `json:"output_modalities"`

	// ZGI-specific
	Tier                 string               `json:"tier"`
	IsEnabled            bool                 `json:"is_enabled"`
	IsAvailable          bool                 `json:"is_available"` // true if model has available channels
	ZgiOfficialAvailable bool                 `json:"-"`            // Internal use only
	Callable             bool                 `json:"-"`            // Internal use only
	CreatedAt            int64                `json:"created_at"`   // Unix timestamp
	UpdatedAt            int64                `json:"updated_at"`   // Unix timestamp
	SupportedParameters  []string             `json:"supported_parameters,omitempty"`
	ParametersMetadata   ParameterDefinitions `json:"parameters_metadata,omitempty"`
}
