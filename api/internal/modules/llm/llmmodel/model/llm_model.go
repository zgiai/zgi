package model

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// ============================================================================
// Global Model - managed by platform administrators
// Consolidated from model.LLMModel with full field support
// ============================================================================

// Model represents an AI model with its capabilities, pricing, and specifications
// Based on ModelMeta API structure - this is the canonical model definition
type LLMModel struct {
	ID uuid.UUID `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"id"`

	// Provider reference
	Provider string `gorm:"type:varchar(100);not null;index:idx_model_provider" json:"provider"` // References LLMProvider.Provider

	// Basic info (ModelMeta aligned)
	Object              string     `gorm:"-" json:"object"` // Fixed value "model" (not stored in DB)
	Model               string     `gorm:"type:varchar(100);not null;index:idx_model_name;column:name" json:"model"`
	ModelName           string     `gorm:"type:varchar(200);not null;column:display_name" json:"model_name"`
	Family              string     `gorm:"type:varchar(100);index" json:"family,omitempty"` // Model family (GPT-4, Claude)
	FamilyName          string     `gorm:"type:varchar(200)" json:"family_name,omitempty"`  // Family display name (e.g., "GPT-4o")
	ParentID            *uuid.UUID `gorm:"type:uuid;index" json:"parent_id,omitempty"`      // Parent model ID for version relationships
	FamilyDefault       bool       `gorm:"default:false" json:"family_default"`             // Whether this is the default model in its family
	Status              string     `gorm:"type:varchar(20);default:'active'" json:"status"` // active, deprecated
	ReplacementProvider string     `gorm:"type:varchar(100);column:replacement_provider" json:"replacement_provider,omitempty"`
	ReplacementModel    string     `gorm:"type:varchar(100);column:replacement_model" json:"replacement_model,omitempty"`
	DeprecationReason   string     `gorm:"type:text;column:deprecation_reason" json:"deprecation_reason,omitempty"`
	Tagline             string     `gorm:"type:text" json:"tagline,omitempty"` // Short description
	Description         string     `gorm:"type:text" json:"description,omitempty"`

	// Flags (ModelHub-aligned)
	IsFlagship bool   `gorm:"default:false" json:"is_flagship"`                     // Featured model
	IsFeatured bool   `gorm:"default:false" json:"is_featured"`                     // Highlighted in UI
	IsNew      bool   `gorm:"default:false" json:"is_new"`                          // Recently released
	AccessType string `gorm:"type:varchar(20);default:'closed'" json:"access_type"` // open, closed
	Currency   string `gorm:"type:varchar(10);default:'USD'" json:"currency"`       // Currency for pricing

	// Model capabilities
	UseCases                 StringArray `gorm:"type:text[];default:'{}'" json:"use_cases"`
	SupportsReasoning        bool        `gorm:"column:reasoning;default:false;index" json:"-"`
	SupportsToolCall         bool        `gorm:"column:function_calling;default:false;index" json:"-"`
	SupportsStructuredOutput bool        `gorm:"column:structured_output;default:false" json:"-"`

	// Parameter Supports (Standardized Core)
	SupportsTemperature      bool `gorm:"column:temperature;default:true" json:"-"`
	SupportsTopP             bool `gorm:"column:top_p" json:"-"`
	SupportsPresencePenalty  bool `gorm:"column:presence_penalty" json:"-"`
	SupportsFrequencyPenalty bool `gorm:"column:frequency_penalty" json:"-"`
	SupportsLogitBias        bool `gorm:"column:logit_bias" json:"-"`
	SupportsSeed             bool `gorm:"column:seed;default:false" json:"-"`
	SupportsStop             bool `gorm:"column:stop;default:true" json:"-"`
	MaxStopSequences         int  `gorm:"column:max_stop_sequences;default:4" json:"-"`

	// Legacy / Compatibility fields (Required by service layer)
	SupportsVision       bool `gorm:"column:vision;default:false" json:"-"`
	SupportsAudio        bool `gorm:"column:audio;default:false" json:"-"`
	SupportsFunctionCall bool `gorm:"-" json:"-"` // Deprecated - use SupportsToolCall instead
	SupportsJsonMode     bool `gorm:"column:json_mode;default:false" json:"-"`
	SupportsStreaming    bool `gorm:"column:streaming;default:true" json:"-"`

	// ModelHub Aligned Capabilities (New flat columns from m0068/m0069)
	ChatCompletions   bool `gorm:"column:chat_completions;default:true" json:"-"`
	Embeddings        bool `gorm:"column:embeddings;default:false" json:"-"`
	ImageGeneration   bool `gorm:"column:image_generation;default:false" json:"-"`
	SpeechGeneration  bool `gorm:"column:speech_generation;default:false" json:"-"`
	Transcription     bool `gorm:"column:transcription;default:false" json:"-"`
	Translation       bool `gorm:"column:translation;default:false" json:"-"` // Translation capability (ModelMeta aligned)
	Moderation        bool `gorm:"column:moderation;default:false" json:"-"`
	Videos            bool `gorm:"column:videos;default:false" json:"-"`     // Video processing capability (ModelMeta aligned)
	ImageEdit         bool `gorm:"column:image_edit;default:false" json:"-"` // Image editing capability (ModelMeta aligned)
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
	KnowledgeCutoff     string               `gorm:"type:varchar(20)" json:"knowledge_cutoff,omitempty"`                           // Knowledge cutoff date (e.g., "2024-07")
	OpenWeights         bool                 `gorm:"default:false" json:"open_weights"`                                            // Open-source weights availability
	ContextWindow       int                  `gorm:"column:context_window" json:"context_window,omitempty"`                        // Maximum context window size in tokens (OpenAI naming)
	MaxOutputTokens     int                  `gorm:"column:max_output_tokens" json:"max_output_tokens,omitempty"`                  // Maximum output tokens (OpenAI naming)
	MaxInputTokens      int                  `gorm:"column:max_input_tokens" json:"max_input_tokens,omitempty"`                    // Maximum input tokens (OpenAI naming)
	SupportedParameters ParameterDefinitions `gorm:"column:supported_parameters;type:jsonb" json:"supported_parameters,omitempty"` // Supported parameters
	ConfigParameters    ConfigParameters     `gorm:"column:config_parameters;type:jsonb" json:"-"`
	DefaultParameters   JSONObject           `gorm:"column:default_parameters;type:jsonb" json:"default_parameters,omitempty"`    // Default parameters
	IsModerated         bool                 `gorm:"default:false" json:"is_moderated"`                                           // Whether model has moderation
	IsFinetuned         bool                 `gorm:"default:false" json:"is_finetuned"`                                           // Whether model is a fine-tuned version (kept for business value)
	Metadata            JSONObject           `gorm:"-" json:"metadata,omitempty"`                                                 // Additional metadata (not in DB)
	CostRate            JSONObject           `gorm:"type:jsonb;default:'{\"input\":1, \"output\":1}'" json:"cost_rate,omitempty"` // Cost multipliers (kept for billing core)

	// Pricing (per million tokens in USD) - OpenAI naming (ModelMeta aligned)
	InputPrice          decimal.Decimal `gorm:"column:input_price;type:decimal(10,4)" json:"input_price,omitempty"`               // Price per million input tokens
	OutputPrice         decimal.Decimal `gorm:"column:output_price;type:decimal(10,4)" json:"output_price,omitempty"`             // Price per million output tokens
	CachedInputPrice    decimal.Decimal `gorm:"column:cached_input_price;type:decimal(10,4)" json:"cached_input_price,omitempty"` // Price per million cached input tokens (ModelMeta aligned)
	CostCacheRead       decimal.Decimal `gorm:"type:decimal(10,4)" json:"cost_cache_read,omitempty"`                              // Cost per million cached tokens read
	CostCacheWrite      decimal.Decimal `gorm:"type:decimal(10,4)" json:"cost_cache_write,omitempty"`                             // Cost per million cached tokens write
	CostContextOver200k JSONObject      `gorm:"column:cost_context_over_200k;type:jsonb" json:"cost_context_over_200k,omitempty"` // Special pricing for large contexts

	// Image Pricing Rules
	ImagePrices datatypes.JSON `gorm:"column:image_prices;type:jsonb;default:'[]'" json:"image_prices,omitempty"` // Structured pricing rules for image generation

	// Status and ordering
	IsActive     bool `gorm:"default:true;index" json:"is_active"`      // Whether model is available
	IsConfigured bool `gorm:"default:false;index" json:"is_configured"` // Whether model has valid channel configuration (credentials, routes, etc.)
	SortOrder    int  `gorm:"default:0" json:"sort_order"`              // Display order

	// Model tier and recommendation
	ModelTier     string `gorm:"type:varchar(20);index" json:"model_tier,omitempty"`  // Model tier: flagship/premium/standard/basic
	IsRecommended bool   `gorm:"default:false;index" json:"is_recommended,omitempty"` // Quick filter for recommended models

	// Timestamps
	CreatedAt time.Time      `gorm:"not null;default:CURRENT_TIMESTAMP" json:"created_at"`
	UpdatedAt time.Time      `gorm:"not null;default:CURRENT_TIMESTAMP" json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"deleted_at,omitempty"`

	// ModelMeta nested capability objects (not stored in DB, populated via hooks)
	Endpoints  *ModelEndpoints  `gorm:"-" json:"endpoints,omitempty"`
	Features   *ModelFeatures   `gorm:"-" json:"features,omitempty"`
	Tools      *ModelTools      `gorm:"-" json:"tools,omitempty"`
	Parameters *ModelParameters `gorm:"-" json:"parameters,omitempty"`
}

func (LLMModel) TableName() string {
	return "llm_models"
}

func (m *LLMModel) BeforeCreate(tx *gorm.DB) error {
	if m.ID == uuid.Nil {
		m.ID = uuid.New()
	}
	// Auto-set Object field to "model" (ModelMeta standard)
	m.Object = "model"
	return nil
}

// AfterFind hook - populate nested capability objects from flat database fields
func (m *LLMModel) AfterFind(tx *gorm.DB) error {
	// Populate Endpoints
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
		Videos:           m.Videos,
		ImageEdit:        m.ImageEdit,
	}

	// Populate Features
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

	// Populate Tools
	m.Tools = &ModelTools{
		WebSearch:         m.WebSearch,
		FileSearch:        m.FileSearch,
		ImageGeneration:   m.ImageGeneration,
		CodeInterpreter:   m.CodeInterpreter,
		ComputerUse:       m.ComputerUse,
		Mcp:               m.Mcp,
		ParallelToolCalls: m.ParallelToolCalls,
	}

	// Populate Parameters
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

	// Auto-set Object field
	m.Object = "model"

	return nil
}

// BeforeSave hook - update flat database fields from nested capability objects
func (m *LLMModel) BeforeSave(tx *gorm.DB) error {
	// Update from Endpoints if provided
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
		m.Videos = m.Endpoints.Videos
		m.ImageEdit = m.Endpoints.ImageEdit
	}

	// Update from Features if provided
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

	// Update from Tools if provided
	if m.Tools != nil {
		m.ParallelToolCalls = m.Tools.ParallelToolCalls
		// Note: Other tool fields are already updated from Features
	}

	// Update from Parameters if provided
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

// HasUseCase returns true if the model's UseCases array contains the given use case
func (m *LLMModel) HasUseCase(useCase string) bool {
	for _, uc := range m.UseCases {
		if uc == useCase {
			return true
		}
	}
	return false
}

// IsLLM returns true if this is a language model.
func (m *LLMModel) IsLLM() bool {
	if len(m.UseCases) > 0 {
		return m.HasUseCase(string(UseCaseTextChat))
	}
	return m.ChatCompletions || m.Responses || m.Realtime || m.Assistants
}

// IsEmbedding returns true if this is an embedding model.
func (m *LLMModel) IsEmbedding() bool {
	if len(m.UseCases) > 0 {
		return m.HasUseCase(string(UseCaseEmbedding))
	}
	return m.Embeddings
}

// IsRerank returns true if this is a rerank model.
func (m *LLMModel) IsRerank() bool {
	return m.HasUseCase(string(UseCaseRerank))
}

// IsImageGeneration returns true if this is an image generation model.
func (m *LLMModel) IsImageGeneration() bool {
	if len(m.UseCases) > 0 {
		return m.HasUseCase(string(UseCaseImageGen))
	}
	return m.ImageGeneration
}

// PricingRule represents a structured pricing rule for image generation
type PricingRule struct {
	ID         string                 `json:"id"`
	Priority   int                    `json:"priority"`
	Conditions map[string]interface{} `json:"conditions"` // size, quality
	Price      PricingDetail          `json:"price"`
}

// PricingDetail represents the cost details
type PricingDetail struct {
	Credits int64   `json:"credits"` // Credits consumed
	Amount  float64 `json:"amount"`  // Fiat amount (reserved field)
}
