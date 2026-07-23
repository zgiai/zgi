package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	llmcache "github.com/zgiai/zgi/api/internal/modules/llm/cache"
	"github.com/zgiai/zgi/api/internal/modules/llm/gateway/types"
	llmmodel "github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/model"
	"github.com/zgiai/zgi/api/internal/modules/llm/modelmeta"
	providermodel "github.com/zgiai/zgi/api/internal/modules/llm/provider/model"
	"github.com/zgiai/zgi/api/pkg/logger"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

const modelCacheVersion = 1

// Compile-time interface check
var _ types.ConfigCache = (*ConfigCache)(nil)

// ConfigCache provides caching for read-only configuration data
// It only caches static configuration that rarely changes
// Critical billing data (API key quota, credit balance) is NOT cached
type ConfigCache struct {
	redis  *redis.Client
	db     *gorm.DB
	prefix string

	// TTL settings
	modelTTL        time.Duration
	providerTTL     time.Duration
	shadowTenantTTL time.Duration

	// Stats
	hits   int64
	misses int64
}

// ConfigCacheConfig contains configuration for ConfigCache
type ConfigCacheConfig struct {
	ModelTTL        time.Duration
	ProviderTTL     time.Duration
	ShadowTenantTTL time.Duration
}

// DefaultConfigCacheConfig returns default configuration
func DefaultConfigCacheConfig() *ConfigCacheConfig {
	return &ConfigCacheConfig{
		ModelTTL:        5 * time.Minute,
		ProviderTTL:     5 * time.Minute,
		ShadowTenantTTL: 10 * time.Minute,
	}
}

// NewConfigCache creates a new configuration cache
func NewConfigCache(redis *redis.Client, db *gorm.DB, config *ConfigCacheConfig) *ConfigCache {
	if config == nil {
		config = DefaultConfigCacheConfig()
	}
	cache := &ConfigCache{
		redis:           redis,
		db:              db,
		prefix:          "llm:config:",
		modelTTL:        config.ModelTTL,
		providerTTL:     config.ProviderTTL,
		shadowTenantTTL: config.ShadowTenantTTL,
	}
	modelmeta.SetModelCacheInvalidator(cache)
	return cache
}

// ===== LLMModel Cache =====

// GetModelByName retrieves model by name, using cache first
// Falls back to database if cache miss or error
func (c *ConfigCache) GetModelByName(ctx context.Context, name string) (*llmmodel.LLMModel, error) {
	key := c.prefix + "model:name:" + name

	// Try cache first
	data, err := c.redis.Get(ctx, key).Bytes()
	if err == nil {
		var m llmmodel.LLMModel
		if unmarshalCachedModel(data, &m) == nil && cachedModelIsActive(&m) {
			// Verify provider is still active before returning cached model
			provider, provErr := c.GetProviderByName(ctx, m.Provider)
			if provErr == nil && provider != nil && provider.IsActive {
				c.hits++
				return &m, nil
			}
			// Provider not active, fall through to database query
		}
	}

	// Cache miss or error, query database
	// Join with llm_providers to ensure provider is active
	c.misses++
	var m llmmodel.LLMModel
	if err := c.db.WithContext(ctx).
		Model(&llmmodel.LLMModel{}).
		Joins("JOIN llm_providers ON llm_models.provider = llm_providers.provider").
		Where("llm_models.name = ? AND llm_models.is_active = ? AND llm_models.status = ? AND llm_models.deleted_at IS NULL", name, true, llmmodel.ModelStatusActive).
		Where("llm_providers.is_active = ? AND llm_providers.deleted_at IS NULL", true).
		First(&m).Error; err != nil {
		return nil, err
	}

	// Cache the result (async, don't block on failure)
	go c.cacheModel(context.Background(), key, &m)

	return &m, nil
}

// GetModelByID retrieves model by ID, using cache first
func (c *ConfigCache) GetModelByID(ctx context.Context, id uuid.UUID) (*llmmodel.LLMModel, error) {
	key := c.prefix + "model:id:" + id.String()

	// Try cache first
	data, err := c.redis.Get(ctx, key).Bytes()
	if err == nil {
		var m llmmodel.LLMModel
		if unmarshalCachedModel(data, &m) == nil && cachedModelIsActive(&m) {
			c.hits++
			return &m, nil
		}
	}

	// Cache miss, query database
	c.misses++
	var m llmmodel.LLMModel
	if err := c.db.WithContext(ctx).
		Where("id = ? AND is_active = ? AND status = ? AND deleted_at IS NULL", id, true, llmmodel.ModelStatusActive).
		First(&m).Error; err != nil {
		return nil, err
	}

	// Cache the result
	go c.cacheModel(context.Background(), key, &m)

	return &m, nil
}

func cachedModelIsActive(m *llmmodel.LLMModel) bool {
	return m != nil && m.IsActive && m.Status == llmmodel.ModelStatusActive
}

func (c *ConfigCache) cacheModel(ctx context.Context, key string, m *llmmodel.LLMModel) {
	data, err := marshalCachedModel(m)
	if err != nil {
		return
	}
	if err := c.redis.Set(ctx, key, data, c.modelTTL).Err(); err != nil {
		logger.WarnContext(ctx, "failed to cache LLM model",
			err,
			zap.String("cache_key", key),
		)
	}
}

type llmModelCachePayload struct {
	llmmodel.LLMModel

	CacheVersion             int                       `json:"_cache_version"`
	SupportsReasoning        bool                      `json:"reasoning"`
	SupportsToolCall         bool                      `json:"function_calling"`
	SupportsStructuredOutput bool                      `json:"structured_output"`
	SupportsTemperature      bool                      `json:"temperature"`
	SupportsTopP             bool                      `json:"top_p"`
	SupportsPresencePenalty  bool                      `json:"presence_penalty"`
	SupportsFrequencyPenalty bool                      `json:"frequency_penalty"`
	SupportsLogitBias        bool                      `json:"logit_bias"`
	SupportsSeed             bool                      `json:"seed"`
	SupportsStop             bool                      `json:"stop"`
	MaxStopSequences         int                       `json:"max_stop_sequences"`
	SupportsVision           bool                      `json:"vision"`
	SupportsAudio            bool                      `json:"audio"`
	SupportsFunctionCall     bool                      `json:"supports_function_call"`
	SupportsJsonMode         bool                      `json:"json_mode"`
	SupportsStreaming        bool                      `json:"streaming"`
	ChatCompletions          bool                      `json:"chat_completions"`
	Embeddings               bool                      `json:"embeddings"`
	ImageGeneration          bool                      `json:"image_generation"`
	SpeechGeneration         bool                      `json:"speech_generation"`
	Transcription            bool                      `json:"transcription"`
	Translation              bool                      `json:"translation"`
	Moderation               bool                      `json:"moderation"`
	Videos                   bool                      `json:"videos"`
	ImageEdit                bool                      `json:"image_edit"`
	Realtime                 bool                      `json:"realtime"`
	Batch                    bool                      `json:"batch"`
	FineTuning               bool                      `json:"fine_tuning"`
	Assistants               bool                      `json:"assistants"`
	Responses                bool                      `json:"responses"`
	Distillation             bool                      `json:"distillation"`
	SystemPrompt             bool                      `json:"system_prompt"`
	Logprobs                 bool                      `json:"logprobs"`
	WebSearch                bool                      `json:"web_search"`
	FileSearch               bool                      `json:"file_search"`
	CodeInterpreter          bool                      `json:"code_interpreter"`
	ComputerUse              bool                      `json:"computer_use"`
	Mcp                      bool                      `json:"mcp"`
	ParallelToolCalls        bool                      `json:"parallel_tool_calls"`
	ReasoningEffort          bool                      `json:"reasoning_effort"`
	InputModalities          llmmodel.JSONArray        `json:"input_modalities"`
	OutputModalities         llmmodel.JSONArray        `json:"output_modalities"`
	ConfigParameters         llmmodel.ConfigParameters `json:"config_parameters"`
}

func marshalCachedModel(m *llmmodel.LLMModel) ([]byte, error) {
	if m == nil {
		return nil, errors.New("model is nil")
	}
	payload := llmModelCachePayload{
		LLMModel:                 *m,
		CacheVersion:             modelCacheVersion,
		SupportsReasoning:        m.SupportsReasoning,
		SupportsToolCall:         m.SupportsToolCall,
		SupportsStructuredOutput: m.SupportsStructuredOutput,
		SupportsTemperature:      m.SupportsTemperature,
		SupportsTopP:             m.SupportsTopP,
		SupportsPresencePenalty:  m.SupportsPresencePenalty,
		SupportsFrequencyPenalty: m.SupportsFrequencyPenalty,
		SupportsLogitBias:        m.SupportsLogitBias,
		SupportsSeed:             m.SupportsSeed,
		SupportsStop:             m.SupportsStop,
		MaxStopSequences:         m.MaxStopSequences,
		SupportsVision:           m.SupportsVision,
		SupportsAudio:            m.SupportsAudio,
		SupportsFunctionCall:     m.SupportsFunctionCall,
		SupportsJsonMode:         m.SupportsJsonMode,
		SupportsStreaming:        m.SupportsStreaming,
		ChatCompletions:          m.ChatCompletions,
		Embeddings:               m.Embeddings,
		ImageGeneration:          m.ImageGeneration,
		SpeechGeneration:         m.SpeechGeneration,
		Transcription:            m.Transcription,
		Translation:              m.Translation,
		Moderation:               m.Moderation,
		Videos:                   m.Videos,
		ImageEdit:                m.ImageEdit,
		Realtime:                 m.Realtime,
		Batch:                    m.Batch,
		FineTuning:               m.FineTuning,
		Assistants:               m.Assistants,
		Responses:                m.Responses,
		Distillation:             m.Distillation,
		SystemPrompt:             m.SystemPrompt,
		Logprobs:                 m.Logprobs,
		WebSearch:                m.WebSearch,
		FileSearch:               m.FileSearch,
		CodeInterpreter:          m.CodeInterpreter,
		ComputerUse:              m.ComputerUse,
		Mcp:                      m.Mcp,
		ParallelToolCalls:        m.ParallelToolCalls,
		ReasoningEffort:          m.ReasoningEffort,
		InputModalities:          m.InputModalities,
		OutputModalities:         m.OutputModalities,
		ConfigParameters:         m.ConfigParameters,
	}
	return json.Marshal(payload)
}

func unmarshalCachedModel(data []byte, m *llmmodel.LLMModel) error {
	var payload llmModelCachePayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return err
	}
	if payload.CacheVersion != modelCacheVersion {
		return fmt.Errorf("unsupported model cache version %d", payload.CacheVersion)
	}
	*m = payload.LLMModel
	m.SupportsReasoning = payload.SupportsReasoning
	m.SupportsToolCall = payload.SupportsToolCall
	m.SupportsStructuredOutput = payload.SupportsStructuredOutput
	m.SupportsTemperature = payload.SupportsTemperature
	m.SupportsTopP = payload.SupportsTopP
	m.SupportsPresencePenalty = payload.SupportsPresencePenalty
	m.SupportsFrequencyPenalty = payload.SupportsFrequencyPenalty
	m.SupportsLogitBias = payload.SupportsLogitBias
	m.SupportsSeed = payload.SupportsSeed
	m.SupportsStop = payload.SupportsStop
	m.MaxStopSequences = payload.MaxStopSequences
	m.SupportsVision = payload.SupportsVision
	m.SupportsAudio = payload.SupportsAudio
	m.SupportsFunctionCall = payload.SupportsFunctionCall
	m.SupportsJsonMode = payload.SupportsJsonMode
	m.SupportsStreaming = payload.SupportsStreaming
	m.ChatCompletions = payload.ChatCompletions
	m.Embeddings = payload.Embeddings
	m.ImageGeneration = payload.ImageGeneration
	m.SpeechGeneration = payload.SpeechGeneration
	m.Transcription = payload.Transcription
	m.Translation = payload.Translation
	m.Moderation = payload.Moderation
	m.Videos = payload.Videos
	m.ImageEdit = payload.ImageEdit
	m.Realtime = payload.Realtime
	m.Batch = payload.Batch
	m.FineTuning = payload.FineTuning
	m.Assistants = payload.Assistants
	m.Responses = payload.Responses
	m.Distillation = payload.Distillation
	m.SystemPrompt = payload.SystemPrompt
	m.Logprobs = payload.Logprobs
	m.WebSearch = payload.WebSearch
	m.FileSearch = payload.FileSearch
	m.CodeInterpreter = payload.CodeInterpreter
	m.ComputerUse = payload.ComputerUse
	m.Mcp = payload.Mcp
	m.ParallelToolCalls = payload.ParallelToolCalls
	m.ReasoningEffort = payload.ReasoningEffort
	m.InputModalities = payload.InputModalities
	m.OutputModalities = payload.OutputModalities
	m.ConfigParameters = payload.ConfigParameters
	return nil
}

// InvalidateModel invalidates model cache
func (c *ConfigCache) InvalidateModel(ctx context.Context, id uuid.UUID, name string) {
	keys := []string{
		c.prefix + "model:id:" + id.String(),
		c.prefix + "model:name:" + name,
	}
	c.redis.Del(ctx, keys...)
}

func (c *ConfigCache) InvalidateModelCache(ctx context.Context) {
	if c == nil || c.redis == nil {
		return
	}
	llmcache.InvalidateGlobal(ctx)

	var cursor uint64
	match := c.prefix + "model:*"
	for {
		keys, nextCursor, err := c.redis.Scan(ctx, cursor, match, 100).Result()
		if err != nil {
			logger.WarnContext(ctx, "failed to scan LLM model cache keys", err)
			return
		}
		if len(keys) > 0 {
			if err := c.redis.Del(ctx, keys...).Err(); err != nil {
				logger.WarnContext(ctx, "failed to invalidate LLM model cache", err)
				return
			}
		}
		if nextCursor == 0 {
			return
		}
		cursor = nextCursor
	}
}

// ===== LLMProvider Cache =====

// GetProviderByName retrieves provider by name, using cache first
func (c *ConfigCache) GetProviderByName(ctx context.Context, name string) (*providermodel.LLMProvider, error) {
	key := c.prefix + "provider:name:" + name

	// Try cache first
	data, err := c.redis.Get(ctx, key).Bytes()
	if err == nil {
		var p providermodel.LLMProvider
		if json.Unmarshal(data, &p) == nil {
			c.hits++
			return &p, nil
		}
	}

	// Cache miss, query database
	c.misses++
	var p providermodel.LLMProvider
	if err := c.db.WithContext(ctx).
		Where("provider = ? AND is_active = ? AND deleted_at IS NULL", name, true).
		First(&p).Error; err != nil {
		return nil, err
	}

	// Cache the result
	go c.cacheProvider(context.Background(), key, &p)

	return &p, nil
}

func (c *ConfigCache) cacheProvider(ctx context.Context, key string, p *providermodel.LLMProvider) {
	data, err := json.Marshal(p)
	if err != nil {
		return
	}
	if err := c.redis.Set(ctx, key, data, c.providerTTL).Err(); err != nil {
		logger.WarnContext(ctx, "failed to cache LLM provider",
			err,
			zap.String("cache_key", key),
		)
	}
}

// InvalidateProvider invalidates provider cache
func (c *ConfigCache) InvalidateProvider(ctx context.Context, name string) {
	key := c.prefix + "provider:name:" + name
	c.redis.Del(ctx, key)
}

// ===== Shadow Tenant Cache =====

// GetShadowTenantInfo retrieves shadow tenant info, using cache first
// This caches the tenant -> group mapping, NOT the balance
func (c *ConfigCache) GetShadowTenantInfo(ctx context.Context, organizationID uuid.UUID) (*types.ShadowTenantInfo, error) {
	key := c.prefix + "shadow:" + organizationID.String()

	// Try cache first
	data, err := c.redis.Get(ctx, key).Bytes()
	if err == nil {
		var info types.ShadowTenantInfo
		if json.Unmarshal(data, &info) == nil {
			c.hits++
			return &info, nil
		}
	}

	// Cache miss, query database
	c.misses++

	// Get shadow tenant ID
	shadowOrganizationID, err := GetShadowOrganizationID(ctx, c.db, organizationID)
	if err != nil {
		return nil, fmt.Errorf("failed to get shadow tenant: %w", err)
	}

	// Get owner ID
	ownerID, err := GetShadowTenantOwnerID(ctx, c.db, shadowOrganizationID)
	if err != nil {
		// If no owner found, use empty UUID (account will be created on demand)
		ownerID = uuid.Nil
	}

	info := &types.ShadowTenantInfo{
		ShadowOrganizationID: shadowOrganizationID,
		OwnerID:              ownerID,
	}

	// Cache the result
	go c.cacheShadowTenant(context.Background(), key, info)

	return info, nil
}

func (c *ConfigCache) cacheShadowTenant(ctx context.Context, key string, info *types.ShadowTenantInfo) {
	data, err := json.Marshal(info)
	if err != nil {
		return
	}
	if err := c.redis.Set(ctx, key, data, c.shadowTenantTTL).Err(); err != nil {
		logger.WarnContext(ctx, "failed to cache LLM shadow tenant",
			err,
			zap.String("cache_key", key),
		)
	}
}

// InvalidateShadowTenant invalidates shadow tenant cache
func (c *ConfigCache) InvalidateShadowTenant(ctx context.Context, organizationID uuid.UUID) {
	key := c.prefix + "shadow:" + organizationID.String()
	c.redis.Del(ctx, key)
}

// ===== Stats =====

// Stats returns cache hit/miss statistics
func (c *ConfigCache) Stats() (hits, misses int64) {
	return c.hits, c.misses
}

// HitRate returns the cache hit rate as a percentage
func (c *ConfigCache) HitRate() float64 {
	total := c.hits + c.misses
	if total == 0 {
		return 0
	}
	return float64(c.hits) / float64(total) * 100
}
