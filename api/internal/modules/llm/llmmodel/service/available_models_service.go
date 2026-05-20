package service

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
	channelmodel "github.com/zgiai/ginext/internal/modules/llm/channel/model"
	channelrepo "github.com/zgiai/ginext/internal/modules/llm/channel/repository"
	"github.com/zgiai/ginext/internal/modules/llm/llmmodel/model"
	"github.com/zgiai/ginext/internal/modules/llm/llmmodel/repository"
	providerrepo "github.com/zgiai/ginext/internal/modules/llm/provider/repository"
	"github.com/zgiai/ginext/internal/modules/llm/shared/types"
	interfaces "github.com/zgiai/ginext/internal/modules/shared/interface"
	"github.com/zgiai/ginext/pkg/logger"
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
	// SetOfficialRouteBootstrapper injects the cloud-only bootstrapper used for defensive fallback.
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
	officialBootstrap  interfaces.OfficialRouteBootstrapper

	// Tenant config cache (per-tenant)
	tenantCache   map[uuid.UUID]*tenantCacheEntry
	tenantCacheMu sync.RWMutex

	// Cache configuration
	tenantCacheTTL time.Duration
}

type tenantCacheEntry struct {
	configs   map[uuid.UUID]*model.ModelConfig
	customs   []*model.CustomModel
	updatedAt time.Time
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
		globalRepo:         globalRepo,
		configRepo:         configRepo,
		customRepo:         customRepo,
		routeRepo:          routeRepo,
		globalProviderRepo: globalProviderRepo,
		providerConfigRepo: providerConfigRepo,
		customProviderRepo: customProviderRepo,
		tenantCache:        make(map[uuid.UUID]*tenantCacheEntry),
		tenantCacheTTL:     2 * time.Minute, // Tenant configs may change more often
	}

	return svc
}

// ListAvailable returns available models and optionally filters by provider and use case.
func (s *availableModelsService) ListAvailable(ctx context.Context, organizationID uuid.UUID, provider string, useCase string) ([]*AvailableModel, error) {
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
	enabledRoutes := s.loadEnabledRoutes(ctx, organizationID)
	if !hasOfficialRoute(enabledRoutes) && s.officialBootstrap != nil {
		if err := s.officialBootstrap.InitOfficialChannel(ctx, organizationID); err != nil {
			logger.Warn("Failed to bootstrap official route for available models: %v", err)
		} else {
			enabledRoutes = s.loadEnabledRoutes(ctx, organizationID)
		}
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
		// Log error but continue with empty config
		logger.Warn("Failed to get tenant cache, using defaults: %v", err)
		tenantEntry = &tenantCacheEntry{
			configs: make(map[uuid.UUID]*model.ModelConfig),
		}
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

func (s *availableModelsService) SetOfficialRouteBootstrapper(bootstrapper interfaces.OfficialRouteBootstrapper) {
	s.officialBootstrap = bootstrapper
}

// RefreshCache forces a cache refresh for a tenant
func (s *availableModelsService) RefreshCache(ctx context.Context, organizationID uuid.UUID) error {
	return s.refreshTenantCache(ctx, organizationID)
}

// InvalidateTenantCache invalidates cache for a specific tenant
func (s *availableModelsService) InvalidateTenantCache(organizationID uuid.UUID) {
	s.tenantCacheMu.Lock()
	delete(s.tenantCache, organizationID)
	s.tenantCacheMu.Unlock()
}

// InvalidateGlobalCache is kept for compatibility. Available models do not use a global model cache.
func (s *availableModelsService) InvalidateGlobalCache() {
}

func (s *availableModelsService) loadEnabledRoutes(ctx context.Context, organizationID uuid.UUID) []*channelmodel.LLMRoute {
	enabledRoutes, err := s.routeRepo.GetEnabledRoutes(ctx, organizationID)
	if err != nil {
		logger.Warn("Failed to get enabled routes: %v", err)
		return nil
	}
	return enabledRoutes
}

func hasOfficialRoute(routes []*channelmodel.LLMRoute) bool {
	for _, route := range routes {
		if route != nil && route.IsOfficial {
			return true
		}
	}
	return false
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
	return s.refreshTenantCacheAndReturn(ctx, organizationID)
}

// refreshTenantCacheAndReturn refreshes tenant cache and returns the result
func (s *availableModelsService) refreshTenantCacheAndReturn(ctx context.Context, organizationID uuid.UUID) (*tenantCacheEntry, error) {
	s.tenantCacheMu.Lock()
	defer s.tenantCacheMu.Unlock()

	// Double-check after acquiring write lock
	if entry, ok := s.tenantCache[organizationID]; ok && time.Since(entry.updatedAt) < s.tenantCacheTTL {
		return entry, nil
	}

	configs, err := s.configRepo.ListAvailableConfigs(ctx, organizationID)
	if err != nil {
		// If we have stale cache, return it
		if entry, ok := s.tenantCache[organizationID]; ok {
			logger.Warn("Failed to refresh tenant config cache, using stale data: %v", err)
			return entry, nil
		}
		return nil, err
	}

	// Fetch custom models from database
	customs, _, err := s.customRepo.List(ctx, organizationID, nil, "", "", boolPtr(true), 0, 10000)
	if err != nil {
		logger.Warn("Failed to fetch custom models: %v", err)
		customs = nil // Continue without custom models
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
func (s *availableModelsService) refreshTenantCache(ctx context.Context, organizationID uuid.UUID) error {
	_, err := s.refreshTenantCacheAndReturn(ctx, organizationID)
	return err
}

// NOTE: type-based filtering is deprecated and intentionally not supported for /models/available.
