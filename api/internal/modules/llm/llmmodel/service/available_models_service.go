package service

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	channelmodel "github.com/zgiai/zgi/api/internal/modules/llm/channel/model"
	channelrepo "github.com/zgiai/zgi/api/internal/modules/llm/channel/repository"
	"github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/model"
	"github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/repository"
	providerrepo "github.com/zgiai/zgi/api/internal/modules/llm/provider/repository"
	"github.com/zgiai/zgi/api/internal/modules/llm/shared/types"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
)

// ModelScene represents different business scenarios for model selection
type ModelScene string

const (
	SceneChat      ModelScene = "chat"      // For workflow LLM nodes, agents
	SceneEmbedding ModelScene = "embedding" // For knowledge base embedding
	SceneRerank    ModelScene = "rerank"    // For reranking
	SceneTTS       ModelScene = "tts"       // For text-to-speech
	SceneSTT       ModelScene = "stt"       // For speech-to-text
	SceneImage     ModelScene = "image"     // For image generation
)

// AvailableModel represents a simplified model for business use
// Aligned with ModelHub structure for consistency
type AvailableModel struct {
	// Basic info
	ID          uuid.UUID `json:"id"`
	Name        string    `json:"model"`
	DisplayName string    `json:"model_name"`
	Provider    string    `json:"provider"`

	// Context
	ContextWindow   int `json:"context_window,omitempty"`
	MaxOutputTokens int `json:"max_output_tokens,omitempty"`

	// ModelHub-aligned capabilities (nested structures)
	Endpoints  model.ModelEndpoints  `json:"endpoints"`
	Features   model.ModelFeatures   `json:"features"`
	Tools      model.ModelTools      `json:"tools"`
	Parameters model.ModelParameters `json:"parameters"`

	// Use cases array
	UseCases []string `json:"use_cases,omitempty"`

	// Passthrough support - custom models from channels
	IsCustom      bool   `json:"is_custom,omitempty"`
	SourceChannel string `json:"source_channel,omitempty"`
}

// AvailableModelsService provides fast access to available models for business use
type AvailableModelsService interface {
	// ListAvailable returns available models and optionally filters by provider and use case.
	// NOTE: type-based filtering is deprecated; use use_case for capability filtering.
	ListAvailable(ctx context.Context, organizationID uuid.UUID, provider string, useCase string) ([]*AvailableModel, error)
	// RefreshCache forces a cache refresh for a tenant
	RefreshCache(ctx context.Context, organizationID uuid.UUID) error
	// InvalidateTenantCache invalidates cache for a specific tenant
	InvalidateTenantCache(organizationID uuid.UUID)
	// InvalidateGlobalCache is kept for callers that still notify global model changes.
	InvalidateGlobalCache()
	// SetOfficialRouteBootstrapper is kept for compatibility; listing available models is read-only.
	SetOfficialRouteBootstrapper(bootstrapper interfaces.OfficialRouteBootstrapper)
}

// availableModelsService implements AvailableModelsService with in-memory caching
type availableModelsService struct {
	globalRepo         repository.ModelRepository
	configRepo         repository.ModelConfigRepository
	customRepo         repository.CustomModelRepository
	routeRepo          channelrepo.TenantRouteRepository
	globalProviderRepo providerrepo.ProviderRepository
	providerConfigRepo providerrepo.ProviderConfigRepository
	customProviderRepo providerrepo.CustomProviderRepository

	// Tenant config cache (per-tenant)
	tenantCache   map[uuid.UUID]*tenantCacheEntry
	tenantCacheMu sync.RWMutex

	availableCache   map[availableModelsCacheKey]*availableModelsCacheEntry
	availableCacheMu sync.RWMutex

	availableResponseCache   map[availableModelsCacheKey]*availableModelsResponseCacheEntry
	availableResponseCacheMu sync.RWMutex

	// Cache configuration
	tenantCacheTTL    time.Duration
	availableCacheTTL time.Duration
}

type tenantCacheEntry struct {
	configs   map[uuid.UUID]*model.ModelConfig
	customs   []*model.CustomModel
	updatedAt time.Time
}

type availableModelsCacheKey struct {
	organizationID uuid.UUID
	provider       string
	useCase        string
}

type availableModelsCacheEntry struct {
	models    []*AvailableModel
	updatedAt time.Time
}

type availableModelsResponseCacheEntry struct {
	response  []byte
	updatedAt time.Time
}

type availableModelsSuccessResponse struct {
	Code    string                      `json:"code"`
	Message string                      `json:"message"`
	Data    availableModelsListResponse `json:"data"`
}

type availableModelsListResponse struct {
	Items []*AvailableModel `json:"items"`
	Total int               `json:"total"`
}

// NewAvailableModelsService creates a new available models service with caching
func NewAvailableModelsService(
	globalRepo repository.ModelRepository,
	configRepo repository.ModelConfigRepository,
	customRepo repository.CustomModelRepository,
	routeRepo channelrepo.TenantRouteRepository,
) AvailableModelsService {
	return NewAvailableModelsServiceWithProviderRepos(
		globalRepo,
		configRepo,
		customRepo,
		routeRepo,
		nil,
		nil,
		nil,
	)
}

func NewAvailableModelsServiceWithProviderRepos(
	globalRepo repository.ModelRepository,
	configRepo repository.ModelConfigRepository,
	customRepo repository.CustomModelRepository,
	routeRepo channelrepo.TenantRouteRepository,
	globalProviderRepo providerrepo.ProviderRepository,
	providerConfigRepo providerrepo.ProviderConfigRepository,
	customProviderRepo providerrepo.CustomProviderRepository,
) AvailableModelsService {
	svc := &availableModelsService{
		globalRepo:             globalRepo,
		configRepo:             configRepo,
		customRepo:             customRepo,
		routeRepo:              routeRepo,
		globalProviderRepo:     globalProviderRepo,
		providerConfigRepo:     providerConfigRepo,
		customProviderRepo:     customProviderRepo,
		tenantCache:            make(map[uuid.UUID]*tenantCacheEntry),
		availableCache:         make(map[availableModelsCacheKey]*availableModelsCacheEntry),
		availableResponseCache: make(map[availableModelsCacheKey]*availableModelsResponseCacheEntry),
		tenantCacheTTL:         2 * time.Minute,  // Tenant configs may change more often
		availableCacheTTL:      30 * time.Second, // Full response cache absorbs hot polling with short staleness.
	}

	return svc
}

// ListAvailableJSON returns the final API response body for the available-models endpoint.
// It shares the same cache key and invalidation lifecycle as ListAvailable, avoiding
// repeated clone and JSON encoding work on hot selector requests.
func (s *availableModelsService) ListAvailableJSON(ctx context.Context, organizationID uuid.UUID, provider string, useCase string) ([]byte, error) {
	provider = strings.TrimSpace(provider)
	useCase = strings.TrimSpace(useCase)
	cacheKey := availableModelsCacheKey{
		organizationID: organizationID,
		provider:       provider,
		useCase:        useCase,
	}
	if cached, ok := s.getAvailableResponseCache(cacheKey); ok {
		return cached, nil
	}

	models, sourceUpdatedAt, err := s.getAvailableModelsForCache(ctx, cacheKey, organizationID, provider, useCase)
	if err != nil {
		return nil, err
	}

	body, err := json.Marshal(availableModelsSuccessResponse{
		Code:    "0",
		Message: "success",
		Data: availableModelsListResponse{
			Items: models,
			Total: len(models),
		},
	})
	if err != nil {
		return nil, err
	}

	s.setAvailableResponseCache(cacheKey, body, sourceUpdatedAt)
	return cloneBytes(body), nil
}

// ListAvailable returns available models and optionally filters by provider and use case.
func (s *availableModelsService) ListAvailable(ctx context.Context, organizationID uuid.UUID, provider string, useCase string) ([]*AvailableModel, error) {
	provider = strings.TrimSpace(provider)
	useCase = strings.TrimSpace(useCase)
	cacheKey := availableModelsCacheKey{
		organizationID: organizationID,
		provider:       provider,
		useCase:        useCase,
	}
	models, _, err := s.getAvailableModelsForCache(ctx, cacheKey, organizationID, provider, useCase)
	if err != nil {
		return nil, err
	}
	return models, nil
}

func (s *availableModelsService) getAvailableModelsForCache(ctx context.Context, cacheKey availableModelsCacheKey, organizationID uuid.UUID, provider string, useCase string) ([]*AvailableModel, time.Time, error) {
	if cached, updatedAt, ok := s.getAvailableCache(cacheKey); ok {
		return cached, updatedAt, nil
	}

	result, err := s.listAvailableUncached(ctx, organizationID, provider, useCase)
	if err != nil {
		return nil, time.Time{}, err
	}
	updatedAt := s.setAvailableCache(cacheKey, result)
	return cloneAvailableModels(result), updatedAt, nil
}

func (s *availableModelsService) listAvailableUncached(ctx context.Context, organizationID uuid.UUID, provider string, useCase string) ([]*AvailableModel, error) {
	visibility, err := loadProviderVisibility(
		ctx,
		organizationID,
		s.globalProviderRepo,
		s.providerConfigRepo,
		s.customProviderRepo,
	)
	if err != nil {
		return nil, err
	}

	// Get tenant enabled routes to filter models
	enabledRoutes, err := s.loadEnabledRoutes(ctx, organizationID)
	if err != nil {
		return nil, err
	}

	// Build set of available models from tenant routes
	availableModelNames := make(map[string]bool)
	for _, route := range enabledRoutes {
		// Use unified GetEffectiveModels method
		models := route.GetEffectiveModels()
		for _, modelName := range models {
			availableModelNames[modelName] = true
		}
	}

	// Strict: a model is "available" only if it is backed by at least one enabled route.
	// If we can't find any enabled route models, return an empty list.
	if len(availableModelNames) == 0 {
		return []*AvailableModel{}, nil
	}

	globalModels, err := s.listAvailableGlobalModels(ctx, availableModelNames, provider, useCase)
	if err != nil {
		return nil, err
	}

	// Get tenant config from cache
	tenantEntry, err := s.getTenantCache(ctx, organizationID)
	if err != nil {
		return nil, err
	}

	// Build result
	result := make([]*AvailableModel, 0, len(availableModelNames))

	// Filter and transform global models
	for _, m := range globalModels {
		// Filter by system-level enabled status (business control)
		if !m.IsActive {
			continue
		}
		if !visibility.Allows(m.Provider) {
			continue
		}
		// Contract: every model must have provider + use_cases
		if m.Provider == "" || len(m.UseCases) == 0 {
			continue
		}

		// Filter by tenant routes - only include models available in tenant channels
		if !availableModelNames[m.Model] && !availableModelNames["*"] {
			continue
		}

		// Filter by provider if specified
		if provider != "" && m.Provider != provider {
			continue
		}

		// Check if model is enabled for this tenant
		if cfg, ok := tenantEntry.configs[m.ID]; ok {
			if !cfg.IsEnabled {
				continue
			}
		}

		// Filter by use_case if specified
		if useCase != "" {
			// Strict: use_case must be explicitly present in use_cases.
			// (type is a legacy field and must not be used as a fallback for capability filtering)
			if !containsUseCase(m.UseCases, useCase) {
				continue
			}
		}

		// Transform to AvailableModel with new nested structure
		am := &AvailableModel{
			ID:              m.ID,
			Name:            m.Model,
			DisplayName:     m.ModelName,
			Provider:        m.Provider,
			ContextWindow:   m.ContextWindow,
			MaxOutputTokens: m.MaxOutputTokens,

			// ModelHub-aligned nested structures
			Endpoints: model.ModelEndpoints{
				ChatCompletions:  m.ChatCompletions,
				Responses:        m.Responses,
				Realtime:         m.Realtime,
				Assistants:       m.Assistants,
				Batch:            m.Batch,
				Embeddings:       m.Embeddings,
				Vision:           m.SupportsVision,
				ImageGeneration:  m.ImageGeneration,
				SpeechGeneration: m.SpeechGeneration,
				Transcription:    m.Transcription,
				Moderation:       m.Moderation,
			},
			Features: model.ModelFeatures{
				Streaming:        m.SupportsStreaming,
				FunctionCalling:  m.SupportsToolCall,
				StructuredOutput: m.SupportsStructuredOutput,
				JsonMode:         m.SupportsJsonMode,
				Reasoning:        m.SupportsReasoning,
				SystemPrompt:     m.SystemPrompt,
				Logprobs:         m.Logprobs,
				WebSearch:        m.WebSearch,
				FileSearch:       m.FileSearch,
				CodeInterpreter:  m.CodeInterpreter,
				ComputerUse:      m.ComputerUse,
				Mcp:              m.Mcp,
				ReasoningEffort:  m.ReasoningEffort,
			},
			Tools: model.ModelTools{
				ParallelToolCalls: m.ParallelToolCalls,
			},
			Parameters: model.ModelParameters{
				SupportsTemperature:      m.SupportsTemperature,
				SupportsTopP:             m.SupportsTopP,
				SupportsPresencePenalty:  m.SupportsPresencePenalty,
				SupportsFrequencyPenalty: m.SupportsFrequencyPenalty,
				SupportsLogitBias:        m.SupportsLogitBias,
				SupportsSeed:             m.SupportsSeed,
				SupportsStop:             m.SupportsStop,
				MaxStopSequences:         m.MaxStopSequences,
			},

			// Use cases
			UseCases: []string(m.UseCases),
		}

		// Apply tenant custom display name if set
		if cfg, ok := tenantEntry.configs[m.ID]; ok && cfg.CustomDisplayName != "" {
			am.DisplayName = cfg.CustomDisplayName
		}

		result = append(result, am)
	}

	// Add tenant custom models
	for _, m := range tenantEntry.customs {
		if !visibility.Allows(m.Provider) {
			continue
		}
		// Filter by tenant routes - only include models available in tenant channels
		if !availableModelNames[m.Name] && !availableModelNames["*"] {
			continue
		}
		// Contract: every model must have provider + use_cases
		if m.Provider == "" || len(m.UseCases) == 0 {
			continue
		}

		// Filter by provider if specified
		if provider != "" && m.Provider != provider {
			continue
		}

		if !m.IsActive {
			continue
		}

		// Filter by use_case if specified
		if useCase != "" {
			// Strict: use_case must be explicitly present in use_cases.
			if !containsUseCase(m.UseCases, useCase) {
				continue
			}
		}

		am := &AvailableModel{
			ID:              m.ID,
			Name:            m.Name,
			DisplayName:     m.DisplayName,
			Provider:        m.Provider,
			ContextWindow:   m.ContextWindow,
			MaxOutputTokens: m.MaxOutputTokens,

			// ModelHub-aligned nested structures (aligned with global models)
			Endpoints: model.ModelEndpoints{
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
			},
			Features: model.ModelFeatures{
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
			},
			Tools: model.ModelTools{
				WebSearch:         m.WebSearch,
				FileSearch:        m.FileSearch,
				ImageGeneration:   m.ImageGeneration,
				CodeInterpreter:   m.CodeInterpreter,
				ComputerUse:       m.ComputerUse,
				Mcp:               m.Mcp,
				ParallelToolCalls: m.ParallelToolCalls,
			},
			Parameters: model.ModelParameters{
				SupportsTemperature:      m.SupportsTemperature,
				SupportsTopP:             m.SupportsTopP,
				SupportsPresencePenalty:  m.SupportsPresencePenalty,
				SupportsFrequencyPenalty: m.SupportsFrequencyPenalty,
				SupportsLogitBias:        m.SupportsLogitBias,
				SupportsSeed:             m.SupportsSeed,
				SupportsStop:             m.SupportsStop,
				MaxStopSequences:         m.MaxStopSequences,
			},

			// Use cases
			UseCases: m.UseCases,
		}
		result = append(result, am)
	}

	return result, nil
}

func (s *availableModelsService) getAvailableCache(key availableModelsCacheKey) ([]*AvailableModel, time.Time, bool) {
	s.availableCacheMu.RLock()
	entry, ok := s.availableCache[key]
	if ok && time.Since(entry.updatedAt) < s.availableCacheTTL {
		models := cloneAvailableModels(entry.models)
		updatedAt := entry.updatedAt
		s.availableCacheMu.RUnlock()
		return models, updatedAt, true
	}
	s.availableCacheMu.RUnlock()
	return nil, time.Time{}, false
}

func (s *availableModelsService) setAvailableCache(key availableModelsCacheKey, models []*AvailableModel) time.Time {
	updatedAt := time.Now()
	s.availableCacheMu.Lock()
	s.availableCache[key] = &availableModelsCacheEntry{
		models:    cloneAvailableModels(models),
		updatedAt: updatedAt,
	}
	s.availableCacheMu.Unlock()
	return updatedAt
}

func (s *availableModelsService) getAvailableResponseCache(key availableModelsCacheKey) ([]byte, bool) {
	s.availableResponseCacheMu.RLock()
	entry, ok := s.availableResponseCache[key]
	if ok && time.Since(entry.updatedAt) < s.availableCacheTTL {
		response := cloneBytes(entry.response)
		s.availableResponseCacheMu.RUnlock()
		return response, true
	}
	s.availableResponseCacheMu.RUnlock()
	return nil, false
}

func (s *availableModelsService) setAvailableResponseCache(key availableModelsCacheKey, response []byte, updatedAt time.Time) {
	s.availableResponseCacheMu.Lock()
	s.availableResponseCache[key] = &availableModelsResponseCacheEntry{
		response:  cloneBytes(response),
		updatedAt: updatedAt,
	}
	s.availableResponseCacheMu.Unlock()
}

func cloneBytes(src []byte) []byte {
	if len(src) == 0 {
		return []byte{}
	}
	return append([]byte(nil), src...)
}

func cloneAvailableModels(models []*AvailableModel) []*AvailableModel {
	if len(models) == 0 {
		return []*AvailableModel{}
	}
	cloned := make([]*AvailableModel, 0, len(models))
	for _, item := range models {
		if item == nil {
			cloned = append(cloned, nil)
			continue
		}
		modelCopy := *item
		modelCopy.UseCases = append([]string(nil), item.UseCases...)
		cloned = append(cloned, &modelCopy)
	}
	return cloned
}

func (s *availableModelsService) SetOfficialRouteBootstrapper(_ interfaces.OfficialRouteBootstrapper) {
}

// RefreshCache forces a cache refresh for a tenant
func (s *availableModelsService) RefreshCache(ctx context.Context, organizationID uuid.UUID) error {
	s.invalidateAvailableCacheForTenant(organizationID)
	return s.refreshTenantCache(ctx, organizationID, true)
}

// InvalidateTenantCache invalidates cache for a specific tenant
func (s *availableModelsService) InvalidateTenantCache(organizationID uuid.UUID) {
	s.tenantCacheMu.Lock()
	delete(s.tenantCache, organizationID)
	s.tenantCacheMu.Unlock()
	s.invalidateAvailableCacheForTenant(organizationID)
}

// InvalidateGlobalCache clears the response cache because global model/provider changes can affect every tenant.
func (s *availableModelsService) InvalidateGlobalCache() {
	s.availableCacheMu.Lock()
	s.availableCache = make(map[availableModelsCacheKey]*availableModelsCacheEntry)
	s.availableCacheMu.Unlock()
	s.availableResponseCacheMu.Lock()
	s.availableResponseCache = make(map[availableModelsCacheKey]*availableModelsResponseCacheEntry)
	s.availableResponseCacheMu.Unlock()
}

func (s *availableModelsService) invalidateAvailableCacheForTenant(organizationID uuid.UUID) {
	s.availableCacheMu.Lock()
	for key := range s.availableCache {
		if key.organizationID == organizationID {
			delete(s.availableCache, key)
		}
	}
	s.availableCacheMu.Unlock()
	s.availableResponseCacheMu.Lock()
	for key := range s.availableResponseCache {
		if key.organizationID == organizationID {
			delete(s.availableResponseCache, key)
		}
	}
	s.availableResponseCacheMu.Unlock()
}

func (s *availableModelsService) loadEnabledRoutes(ctx context.Context, organizationID uuid.UUID) ([]*channelmodel.LLMRoute, error) {
	enabledRoutes, err := s.routeRepo.GetEnabledRoutes(ctx, organizationID)
	if err != nil {
		return nil, err
	}
	return enabledRoutes, nil
}

func (s *availableModelsService) listAvailableGlobalModels(ctx context.Context, availableModelNames map[string]bool, provider string, useCase string) ([]*model.LLMModel, error) {
	if availableModelNames["*"] {
		return s.globalRepo.ListAvailableFiltered(ctx, provider, useCase)
	}

	names := make([]string, 0, len(availableModelNames))
	for name := range availableModelNames {
		names = append(names, name)
	}
	sort.Strings(names)
	return s.globalRepo.ListAvailableByNames(ctx, names, provider, useCase)
}

// containsUseCase checks if a use_case is present in the use_cases array
func containsUseCase(useCases types.StringArray, target string) bool {
	for _, uc := range useCases {
		if uc == target {
			return true
		}
	}
	return false
}

// getTenantCache returns tenant cache entry
func (s *availableModelsService) getTenantCache(ctx context.Context, organizationID uuid.UUID) (*tenantCacheEntry, error) {
	s.tenantCacheMu.RLock()
	entry, ok := s.tenantCache[organizationID]
	if ok && time.Since(entry.updatedAt) < s.tenantCacheTTL {
		s.tenantCacheMu.RUnlock()
		return entry, nil
	}
	s.tenantCacheMu.RUnlock()

	// Cache miss or expired, refresh
	return s.refreshTenantCacheAndReturn(ctx, organizationID, false)
}

// refreshTenantCacheAndReturn refreshes tenant cache and returns the result
func (s *availableModelsService) refreshTenantCacheAndReturn(ctx context.Context, organizationID uuid.UUID, force bool) (*tenantCacheEntry, error) {
	s.tenantCacheMu.Lock()
	defer s.tenantCacheMu.Unlock()

	// Double-check after acquiring write lock
	if !force {
		if entry, ok := s.tenantCache[organizationID]; ok && time.Since(entry.updatedAt) < s.tenantCacheTTL {
			return entry, nil
		}
	}

	configs, err := s.configRepo.ListAvailableConfigs(ctx, organizationID)
	if err != nil {
		return nil, err
	}

	// Fetch custom models from database
	customs, _, err := s.customRepo.List(ctx, organizationID, nil, "", "", boolPtr(true), 0, 10000)
	if err != nil {
		return nil, err
	}

	// Build config map
	configMap := make(map[uuid.UUID]*model.ModelConfig, len(configs))
	for _, cfg := range configs {
		configMap[cfg.ModelID] = cfg
	}

	entry := &tenantCacheEntry{
		configs:   configMap,
		customs:   customs,
		updatedAt: time.Now(),
	}
	s.tenantCache[organizationID] = entry

	return entry, nil
}

// refreshTenantCache refreshes tenant cache
func (s *availableModelsService) refreshTenantCache(ctx context.Context, organizationID uuid.UUID, force bool) error {
	_, err := s.refreshTenantCacheAndReturn(ctx, organizationID, force)
	return err
}

// NOTE: type-based filtering is deprecated and intentionally not supported for /models/available.
