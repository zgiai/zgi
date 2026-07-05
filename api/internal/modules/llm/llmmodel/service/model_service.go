package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	channelmodel "github.com/zgiai/zgi/api/internal/modules/llm/channel/model"
	"github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/dto"
	"github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/model"
	"github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/repository"
	officialmodel "github.com/zgiai/zgi/api/internal/modules/llm/officialmodel"
	providerrepo "github.com/zgiai/zgi/api/internal/modules/llm/provider/repository"
	"gorm.io/gorm"
)

var (
	ErrModelNotFound = errors.New("model not found")
	ErrModelExists   = errors.New("model already exists")
)

func parseOptionalModelPrice(value string, field string) (decimal.Decimal, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return decimal.Zero, nil
	}
	price, err := decimal.NewFromString(trimmed)
	if err != nil {
		return decimal.Zero, fmt.Errorf("invalid %s: %w", field, err)
	}
	if price.IsNegative() {
		return decimal.Zero, fmt.Errorf("invalid %s: must be greater than or equal to zero", field)
	}
	return price, nil
}

func modelPriceConfigured(value string) bool {
	return strings.TrimSpace(value) != ""
}

func parseOptionalModelPriceOverride(value string, field string) (*decimal.Decimal, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil, nil
	}
	price, err := decimal.NewFromString(trimmed)
	if err != nil {
		return nil, fmt.Errorf("invalid %s: %w", field, err)
	}
	if price.IsNegative() {
		return nil, fmt.Errorf("invalid %s: must be greater than or equal to zero", field)
	}
	return &price, nil
}

type modelService struct {
	globalRepo         repository.ModelRepository
	configRepo         repository.ModelConfigRepository
	customRepo         repository.CustomModelRepository
	customProviderRepo providerrepo.CustomProviderRepository
	globalProviderRepo providerrepo.ProviderRepository
	providerConfigRepo providerrepo.ProviderConfigRepository
	db                 *gorm.DB
	availableModels    AvailableModelsService
	availabilitySvc    ModelAvailabilityService
}

// NewModelService creates a new model service
func NewModelService(
	db *gorm.DB,
	globalRepo repository.ModelRepository,
	configRepo repository.ModelConfigRepository,
	customRepo repository.CustomModelRepository,
	availabilitySvc ModelAvailabilityService,
	customProviderRepo providerrepo.CustomProviderRepository,
) ModelService {
	return NewModelServiceWithProviderRepos(
		db,
		globalRepo,
		configRepo,
		customRepo,
		availabilitySvc,
		customProviderRepo,
		nil,
		nil,
	)
}

func NewModelServiceWithProviderRepos(
	db *gorm.DB,
	globalRepo repository.ModelRepository,
	configRepo repository.ModelConfigRepository,
	customRepo repository.CustomModelRepository,
	availabilitySvc ModelAvailabilityService,
	customProviderRepo providerrepo.CustomProviderRepository,
	globalProviderRepo providerrepo.ProviderRepository,
	providerConfigRepo providerrepo.ProviderConfigRepository,
) ModelService {
	return &modelService{
		globalRepo:         globalRepo,
		configRepo:         configRepo,
		customRepo:         customRepo,
		customProviderRepo: customProviderRepo,
		globalProviderRepo: globalProviderRepo,
		providerConfigRepo: providerConfigRepo,
		db:                 db,
		availabilitySvc:    availabilitySvc,
	}
}

// SetAvailableModelsService sets the available models service for cache invalidation
func (s *modelService) SetAvailableModelsService(svc AvailableModelsService) {
	s.availableModels = svc
}

// ============================================================================
// Global model operations
// ============================================================================

func (s *modelService) CreateGlobal(ctx context.Context, req *dto.CreateModelRequest) (*model.LLMModel, error) {
	existing, _ := s.globalRepo.GetByProviderAndName(ctx, req.Provider, req.Name)
	if existing != nil {
		return nil, ErrModelExists
	}

	configParameters, err := prepareConfigParameters(req.ConfigParameters)
	if err != nil {
		return nil, err
	}

	costInput, err := parseOptionalModelPrice(req.InputPrice, "input_price")
	if err != nil {
		return nil, err
	}
	costOutput, err := parseOptionalModelPrice(req.OutputPrice, "output_price")
	if err != nil {
		return nil, err
	}

	m := &model.LLMModel{
		Provider:              req.Provider,
		Model:                 req.Name,
		ModelName:             req.DisplayName,
		UseCases:              model.StringArray(model.EnsureUseCases(req.UseCases, nil)),
		ContextWindow:         req.ContextWindow,
		MaxOutputTokens:       req.MaxOutputTokens,
		InputPrice:            costInput,
		OutputPrice:           costOutput,
		InputPriceConfigured:  modelPriceConfigured(req.InputPrice),
		OutputPriceConfigured: modelPriceConfigured(req.OutputPrice),
		SupportsVision:        req.SupportsVision,
		SupportsToolCall:      req.SupportsToolCall,
		SupportsStreaming:     req.SupportsStreaming,
		SupportsReasoning:     req.SupportsReasoning,
		KnowledgeCutoff:       req.KnowledgeCutoff,
		Description:           req.Description,
		ConfigParameters:      configParameters,
		IsActive:              true,
	}

	if err := s.globalRepo.Create(ctx, m); err != nil {
		return nil, fmt.Errorf("failed to create model: %w", err)
	}

	return m, nil
}

func (s *modelService) GetGlobal(ctx context.Context, id uuid.UUID) (*model.LLMModel, error) {
	m, err := s.globalRepo.GetByID(ctx, id)
	if err != nil {
		return nil, ErrModelNotFound
	}
	return m, nil
}

func (s *modelService) ListGlobal(ctx context.Context, req *dto.ListModelRequest) ([]*model.LLMModel, int64, error) {
	offset := (req.Page - 1) * req.PageSize
	return s.globalRepo.List(ctx, req.ProviderID, req.Provider, req.UseCase, req.Status, req.IsActive, offset, req.PageSize)
}

func (s *modelService) UpdateGlobal(ctx context.Context, id uuid.UUID, req *dto.UpdateModelRequest) (*model.LLMModel, error) {
	m, err := s.globalRepo.GetByID(ctx, id)
	if err != nil {
		return nil, ErrModelNotFound
	}

	if req.DisplayName != nil {
		m.ModelName = *req.DisplayName
	}
	if req.ContextWindow != nil {
		m.ContextWindow = *req.ContextWindow
	}
	if req.MaxOutputTokens != nil {
		m.MaxOutputTokens = *req.MaxOutputTokens
	}
	if req.InputPrice != nil {
		price, err := parseOptionalModelPrice(*req.InputPrice, "input_price")
		if err != nil {
			return nil, err
		}
		m.InputPrice = price
		m.InputPriceConfigured = modelPriceConfigured(*req.InputPrice)
	}
	if req.OutputPrice != nil {
		price, err := parseOptionalModelPrice(*req.OutputPrice, "output_price")
		if err != nil {
			return nil, err
		}
		m.OutputPrice = price
		m.OutputPriceConfigured = modelPriceConfigured(*req.OutputPrice)
	}
	if req.SupportsVision != nil {
		m.SupportsVision = *req.SupportsVision
	}
	if req.SupportsToolCall != nil {
		m.SupportsToolCall = *req.SupportsToolCall
	}
	if req.SupportsStreaming != nil {
		m.SupportsStreaming = *req.SupportsStreaming
	}
	if req.SupportsReasoning != nil {
		m.SupportsReasoning = *req.SupportsReasoning
	}
	if req.KnowledgeCutoff != nil {
		m.KnowledgeCutoff = *req.KnowledgeCutoff
	}
	if req.Description != nil {
		m.Description = *req.Description
	}
	if req.IsActive != nil {
		m.IsActive = *req.IsActive
	}
	if req.IsActive != nil {
		m.IsActive = *req.IsActive
	}
	if req.SortOrder != nil {
		m.SortOrder = *req.SortOrder
	}
	if req.UseCases != nil {
		m.UseCases = req.UseCases
	}
	if req.ConfigParameters != nil {
		configParameters, err := prepareConfigParameters(*req.ConfigParameters)
		if err != nil {
			return nil, err
		}
		m.ConfigParameters = configParameters
	}

	if err := s.globalRepo.Update(ctx, m); err != nil {
		return nil, fmt.Errorf("failed to update model: %w", err)
	}

	return m, nil
}

func (s *modelService) DeleteGlobal(ctx context.Context, id uuid.UUID) error {
	return s.globalRepo.Delete(ctx, id)
}

// ============================================================================
// Model config operations
// ============================================================================

func (s *modelService) ConfigureModel(ctx context.Context, organizationID uuid.UUID, req *dto.ConfigureModelRequest) (*model.ModelConfig, error) {
	// Verify global model exists
	_, err := s.globalRepo.GetByID(ctx, req.ModelID)
	if err != nil {
		return nil, ErrModelNotFound
	}

	config, err := s.modelConfigForUpdate(ctx, organizationID, req.ModelID)
	if err != nil {
		return nil, err
	}

	if req.IsEnabled != nil {
		config.IsEnabled = *req.IsEnabled
	}
	if req.CustomDisplayName != "" {
		config.CustomDisplayName = req.CustomDisplayName
	}
	if req.AccessScope != "" {
		config.AccessScope = model.AccessScope(req.AccessScope)
	}
	if req.VisibleGroups != nil {
		config.VisibleGroups = req.VisibleGroups
	}
	if req.VisibleUsers != nil {
		config.VisibleUsers = req.VisibleUsers
	}
	if req.InputPriceOverride != nil {
		cost, err := parseOptionalModelPriceOverride(*req.InputPriceOverride, "input_price_override")
		if err != nil {
			return nil, err
		}
		config.InputPriceOverride = cost
	}
	if req.OutputPriceOverride != nil {
		cost, err := parseOptionalModelPriceOverride(*req.OutputPriceOverride, "output_price_override")
		if err != nil {
			return nil, err
		}
		config.OutputPriceOverride = cost
	}
	if req.SortOrder != nil {
		config.SortOrder = *req.SortOrder
	}
	if config.AccessScope == "" {
		config.AccessScope = model.AccessScopeAll
	}

	if err := s.configRepo.Upsert(ctx, config); err != nil {
		return nil, fmt.Errorf("failed to configure model: %w", err)
	}

	// Invalidate cache for immediate effect
	if s.availableModels != nil {
		s.availableModels.InvalidateTenantCache(organizationID)
	}

	return config, nil
}

func (s *modelService) modelConfigForUpdate(ctx context.Context, organizationID, modelID uuid.UUID) (*model.ModelConfig, error) {
	config, err := s.configRepo.GetByModelID(ctx, organizationID, modelID)
	if err == nil {
		return config, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	return &model.ModelConfig{
		OrganizationID: organizationID,
		ModelID:        modelID,
		IsEnabled:      true,
		AccessScope:    model.AccessScopeAll,
	}, nil
}

func (s *modelService) GetModelConfig(ctx context.Context, organizationID, modelID uuid.UUID) (*model.ModelConfig, error) {
	config, err := s.configRepo.GetByModelID(ctx, organizationID, modelID)
	if err != nil {
		return nil, ErrModelNotFound
	}
	return config, nil
}

func (s *modelService) ListModelConfigs(ctx context.Context, organizationID uuid.UUID, req *dto.ListModelRequest) ([]*model.ModelConfig, int64, error) {
	offset := (req.Page - 1) * req.PageSize
	isEnabled := req.IsActive
	return s.configRepo.List(ctx, organizationID, isEnabled, offset, req.PageSize)
}

// ============================================================================
// Custom model operations
// ============================================================================

func (s *modelService) CreateCustom(ctx context.Context, organizationID uuid.UUID, req *dto.CreateCustomModelRequest) (*model.CustomModel, error) {
	// Resolve provider slug to ProviderID
	providerID, err := s.resolveProviderID(ctx, organizationID, req.Provider, req.ProviderID)
	if err != nil {
		return nil, err
	}

	existing, _ := s.customRepo.GetByProviderAndName(ctx, organizationID, providerID, req.Name)
	if existing != nil {
		return nil, ErrModelExists
	}

	configParameters, err := prepareConfigParameters(req.ConfigParameters)
	if err != nil {
		return nil, err
	}

	costInput, err := parseOptionalModelPrice(req.InputPrice, "input_price")
	if err != nil {
		return nil, err
	}
	costOutput, err := parseOptionalModelPrice(req.OutputPrice, "output_price")
	if err != nil {
		return nil, err
	}

	useCases := model.EnsureUseCases(req.UseCases, req.Endpoints)

	// Auto-infer capabilities from use_cases if not explicitly provided
	endpoints := req.Endpoints
	if endpoints == nil {
		inferred := model.DefaultEndpointsForUseCases(useCases)
		endpoints = &inferred
	}
	features := req.Features
	if features == nil {
		inferred := model.DefaultFeaturesForLLM()
		features = &inferred
	}
	tools := req.Tools
	if tools == nil {
		inferred := model.DefaultTools()
		tools = &inferred
	}
	parameters := req.Parameters
	if parameters == nil {
		inferred := model.DefaultParameters()
		parameters = &inferred
	}

	m := &model.CustomModel{
		OrganizationID:        organizationID,
		ProviderID:            providerID,
		Provider:              req.Provider,
		Name:                  req.Name,
		DisplayName:           req.DisplayName,
		UseCases:              model.StringArray(useCases),
		ContextWindow:         req.ContextWindow,
		MaxOutputTokens:       req.MaxOutputTokens,
		InputPrice:            costInput,
		OutputPrice:           costOutput,
		InputPriceConfigured:  modelPriceConfigured(req.InputPrice),
		OutputPriceConfigured: modelPriceConfigured(req.OutputPrice),
		KnowledgeCutoff:       req.KnowledgeCutoff,
		Description:           req.Description,
		IsActive:              true,
		Endpoints:             endpoints,
		Features:              features,
		Tools:                 tools,
		Parameters:            parameters,
		ConfigParameters:      configParameters,
	}

	if err := s.customRepo.Create(ctx, m); err != nil {
		return nil, fmt.Errorf("failed to create custom model: %w", err)
	}
	if s.availableModels != nil {
		s.availableModels.InvalidateTenantCache(organizationID)
	}

	return m, nil
}

// resolveProviderID resolves a provider slug to a ProviderID UUID.
// Falls back to the deprecated ProviderID field if slug resolution fails.
func (s *modelService) resolveProviderID(ctx context.Context, orgID uuid.UUID, slug string, legacyID *uuid.UUID) (uuid.UUID, error) {
	if slug != "" && s.customProviderRepo != nil {
		cp, err := s.customProviderRepo.GetByProvider(ctx, orgID, slug)
		if err == nil {
			return cp.ID, nil
		}
	}
	if legacyID != nil {
		return *legacyID, nil
	}
	return uuid.Nil, fmt.Errorf("provider not found: %s", slug)
}

func (s *modelService) GetCustom(ctx context.Context, organizationID, id uuid.UUID) (*model.CustomModel, error) {
	m, err := s.customRepo.GetByID(ctx, organizationID, id)
	if err != nil {
		return nil, ErrModelNotFound
	}
	return m, nil
}

func (s *modelService) ListCustom(ctx context.Context, organizationID uuid.UUID, req *dto.ListModelRequest) ([]*model.CustomModel, int64, error) {
	offset := (req.Page - 1) * req.PageSize
	return s.customRepo.List(ctx, organizationID, req.ProviderID, req.Provider, req.UseCase, req.IsActive, offset, req.PageSize)
}

func (s *modelService) UpdateCustom(ctx context.Context, organizationID, id uuid.UUID, req *dto.UpdateCustomModelRequest) (*model.CustomModel, error) {
	m, err := s.customRepo.GetByID(ctx, organizationID, id)
	if err != nil {
		return nil, ErrModelNotFound
	}

	if req.DisplayName != nil {
		m.DisplayName = *req.DisplayName
	}
	if req.ContextWindow != nil {
		m.ContextWindow = *req.ContextWindow
	}
	if req.MaxOutputTokens != nil {
		m.MaxOutputTokens = *req.MaxOutputTokens
	}
	if req.InputPrice != nil {
		price, err := parseOptionalModelPrice(*req.InputPrice, "input_price")
		if err != nil {
			return nil, err
		}
		m.InputPrice = price
		m.InputPriceConfigured = modelPriceConfigured(*req.InputPrice)
	}
	if req.OutputPrice != nil {
		price, err := parseOptionalModelPrice(*req.OutputPrice, "output_price")
		if err != nil {
			return nil, err
		}
		m.OutputPrice = price
		m.OutputPriceConfigured = modelPriceConfigured(*req.OutputPrice)
	}
	if req.KnowledgeCutoff != nil {
		m.KnowledgeCutoff = *req.KnowledgeCutoff
	}
	if req.Description != nil {
		m.Description = *req.Description
	}
	if req.IsActive != nil {
		m.IsActive = *req.IsActive
	}
	if req.SortOrder != nil {
		m.SortOrder = *req.SortOrder
	}
	if req.UseCases != nil {
		m.UseCases = req.UseCases
	}
	if req.Endpoints != nil {
		m.Endpoints = req.Endpoints
	}
	if req.Features != nil {
		m.Features = req.Features
	}
	if req.Tools != nil {
		m.Tools = req.Tools
	}
	if req.Parameters != nil {
		m.Parameters = req.Parameters
	}
	if req.ConfigParameters != nil {
		configParameters, err := prepareConfigParameters(*req.ConfigParameters)
		if err != nil {
			return nil, err
		}
		m.ConfigParameters = configParameters
	}

	if err := s.customRepo.Update(ctx, m); err != nil {
		return nil, fmt.Errorf("failed to update custom model: %w", err)
	}
	if s.availableModels != nil {
		s.availableModels.InvalidateTenantCache(organizationID)
	}

	return m, nil
}

func (s *modelService) DeleteCustom(ctx context.Context, organizationID, id uuid.UUID) error {
	if err := s.customRepo.Delete(ctx, organizationID, id); err != nil {
		return err
	}
	if s.availableModels != nil {
		s.availableModels.InvalidateTenantCache(organizationID)
	}
	return nil
}

func (s *modelService) GetModelParameters(ctx context.Context, organizationID uuid.UUID, provider, modelName string) (model.ConfigParameters, error) {
	customModel, err := s.customRepo.GetByProviderAndModel(ctx, organizationID, provider, modelName)
	if err == nil {
		return model.NormalizeConfigParameters(customModel.ConfigParameters), nil
	}
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("failed to load custom model parameters: %w", err)
	}

	globalModel, err := s.globalRepo.GetByProviderAndName(ctx, provider, modelName)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return model.ConfigParameters{}, ErrModelNotFound
		}
		return nil, fmt.Errorf("failed to load global model parameters: %w", err)
	}

	return model.NormalizeConfigParameters(globalModel.ConfigParameters), nil
}

// ============================================================================
// Aggregated operations
// ============================================================================

// ListTenantModels returns all models available to a tenant (global + custom).
// Visibility is derived from provider/model activity plus tenant configuration.
func (s *modelService) ListTenantModels(ctx context.Context, organizationID uuid.UUID, useCase string, provider string, status string) ([]*model.ModelView, error) {
	var result []*model.ModelView
	provider = strings.TrimSpace(provider)
	status = strings.TrimSpace(status)
	if status == "" {
		status = "active"
	}

	visibility, err := loadProviderVisibility(
		ctx,
		organizationID,
		s.globalProviderRepo,
		s.providerConfigRepo,
		s.customProviderRepo,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to load provider visibility: %w", err)
	}

	var globalModels []*model.LLMModel
	if visibility.ShouldQueryGlobal(provider) {
		// Get all active global models
		globalModels, _, err = s.globalRepo.List(ctx, nil, provider, useCase, status, boolPtr(true), 0, 1000)
		if err != nil {
			return nil, fmt.Errorf("failed to list global models: %w", err)
		}
	}

	// Filter to only include system-enabled models
	var enabledModels []*model.LLMModel
	for _, m := range globalModels {
		if m.IsActive {
			enabledModels = append(enabledModels, m)
		}
	}
	globalModels = enabledModels

	// Get tenant's model configs
	configs, _, err := s.configRepo.List(ctx, organizationID, nil, 0, 10000)
	if err != nil {
		return nil, fmt.Errorf("failed to list model configs: %w", err)
	}

	// Build config map for quick lookup
	configMap := make(map[uuid.UUID]*model.ModelConfig)
	for _, cfg := range configs {
		configMap[cfg.ModelID] = cfg
	}

	// Get available models from tenant routes (models that have enabled channels)
	availableModels := s.getAvailableModelsFromRoutes(ctx, organizationID)

	// Get system-wide available models (all active system channels)
	systemAvailableModels := s.getSystemAvailableModels(ctx)

	// Add global models with tenant config applied
	for _, m := range globalModels {
		if !matchesProviderFilter(m.Provider, provider) {
			continue
		}
		if !visibility.Allows(m.Provider) {
			continue
		}

		// Get modalities with defaults
		inputModalities := []string(m.InputModalities)
		if len(inputModalities) == 0 {
			inputModalities = []string{"text"}
		}

		outputModalities := []string(m.OutputModalities)
		if len(outputModalities) == 0 {
			outputModalities = []string{"text"}
		}

		supportedParams := make([]string, len(m.SupportedParameters))
		for i, p := range m.SupportedParameters {
			supportedParams[i] = p.Name
		}

		// Convert decimal to float64 for JSON response
		costInput, _ := m.InputPrice.Float64()
		costOutput, _ := m.OutputPrice.Float64()
		cachedInputPrice, _ := m.CachedInputPrice.Float64()

		// Check if model is available (has enabled channels for this tenant)
		isAvailable := availableModels[m.Model] || availableModels["*"]
		// Check if model is available system-wide (in any active system channel)
		isSystemAvailable := systemAvailableModels[m.Model] || systemAvailableModels["*"]

		view := &model.ModelView{
			// Basic info
			ID:                  m.ID,
			Provider:            m.Provider,
			Model:               m.Model,
			ModelName:           m.ModelName,
			Family:              m.Family,
			Status:              getStatusOrDefault(m.Status),
			ReplacementProvider: m.ReplacementProvider,
			ReplacementModel:    m.ReplacementModel,
			DeprecationReason:   m.DeprecationReason,
			Tagline:             m.Tagline,

			// Flags (read from database)
			IsFlagship:    m.IsFlagship,
			IsRecommended: m.IsRecommended,
			IsFeatured:    m.IsFeatured,
			IsNew:         m.IsNew,
			AccessType:    getAccessTypeOrDefault(m.AccessType, m.OpenWeights),
			OpenWeights:   m.OpenWeights,

			// Pricing (per million tokens)
			Currency:              getCurrencyOrDefault(m.Currency),
			InputPrice:            costInput,
			OutputPrice:           costOutput,
			InputPriceConfigured:  m.InputPriceConfigured,
			OutputPriceConfigured: m.OutputPriceConfigured,
			CachedInputPrice:      cachedInputPrice,

			// Context
			ContextWindow:   m.ContextWindow,
			MaxOutputTokens: m.MaxOutputTokens,
			MaxInputTokens:  m.MaxInputTokens,

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

			// Arrays
			UseCases:         []string(m.UseCases),
			InputModalities:  inputModalities,
			OutputModalities: outputModalities,

			// ZGI-specific
			Tier:                 m.ModelTier,
			IsEnabled:            false,
			IsAvailable:          isAvailable,
			ZgiOfficialAvailable: isSystemAvailable,
			Callable:             false,
			CreatedAt:            m.CreatedAt.Unix(),
			UpdatedAt:            m.UpdatedAt.Unix(),
			SupportedParameters:  supportedParams,
			ParametersMetadata:   m.SupportedParameters,
		}

		// Apply tenant config if exists
		if cfg, ok := configMap[m.ID]; ok {
			view.IsEnabled = cfg.IsEnabled
			if cfg.CustomDisplayName != "" {
				view.ModelName = cfg.CustomDisplayName
			}
			if cfg.InputPriceOverride != nil {
				overrideInputPrice, _ := cfg.InputPriceOverride.Float64()
				view.InputPrice = overrideInputPrice
				view.InputPriceConfigured = true
			}
			if cfg.OutputPriceOverride != nil {
				overrideOutputPrice, _ := cfg.OutputPriceOverride.Float64()
				view.OutputPrice = overrideOutputPrice
				view.OutputPriceConfigured = true
			}
		}
		view.Callable = view.IsAvailable && view.IsEnabled

		result = append(result, view)
	}

	var customModels []*model.CustomModel
	if status == "active" && visibility.ShouldQueryCustom(provider) {
		// Get tenant's custom models
		customModels, _, err = s.customRepo.List(ctx, organizationID, nil, provider, useCase, boolPtr(true), 0, 10000)
		if err != nil {
			return nil, fmt.Errorf("failed to list custom models: %w", err)
		}
	}

	// Add custom models
	for _, m := range customModels {
		if !matchesProviderFilter(m.Provider, provider) {
			continue
		}
		if !visibility.Allows(m.Provider) {
			continue
		}

		// Custom models have limited fields, use defaults for new fields
		customInputModalities := []string{"text"}
		if m.SupportsVision {
			customInputModalities = []string{"text", "image"}
		}

		// Convert decimal to float64 for custom models
		customInputPrice, _ := m.InputPrice.Float64()
		customOutputPrice, _ := m.OutputPrice.Float64()

		// Custom models are available if they have routes configured
		customIsAvailable := availableModels[m.Name] || availableModels["*"]

		view := &model.ModelView{
			// Basic info
			ID:        m.ID,
			Provider:  m.Provider,
			Model:     m.Name,
			ModelName: m.DisplayName,
			Status:    "active",

			// Flags
			IsRecommended: false,
			AccessType:    "closed",
			OpenWeights:   false,

			// Pricing (per million tokens)
			Currency:              "USD",
			InputPrice:            customInputPrice,
			OutputPrice:           customOutputPrice,
			InputPriceConfigured:  m.InputPriceConfigured,
			OutputPriceConfigured: m.OutputPriceConfigured,

			// Context
			ContextWindow:   m.ContextWindow,
			MaxOutputTokens: m.MaxOutputTokens,

			// Capabilities
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

			// Arrays
			UseCases:         m.UseCases,
			InputModalities:  customInputModalities,
			OutputModalities: []string{"text"},

			// ZGI-specific
			IsEnabled:            m.IsActive,
			IsAvailable:          customIsAvailable,
			ZgiOfficialAvailable: false,                           // Custom models are not in ZGI official channels
			Callable:             customIsAvailable && m.IsActive, // is_available && is_enabled
			CreatedAt:            m.CreatedAt.Unix(),
			UpdatedAt:            m.UpdatedAt.Unix(),
		}
		result = append(result, view)
	}

	return result, nil
}

func matchesProviderFilter(modelProvider string, provider string) bool {
	if strings.TrimSpace(provider) == "" {
		return true
	}
	return normalizeProviderKey(modelProvider) == normalizeProviderKey(provider)
}

func prepareConfigParameters(params model.ConfigParameters) (model.ConfigParameters, error) {
	normalized := model.NormalizeConfigParameters(params)
	if err := model.ValidateConfigParameters(normalized); err != nil {
		return nil, fmt.Errorf("invalid config_parameters: %w", err)
	}
	return normalized, nil
}

// ============================================================================
// Batch operations (Legacy support)
// ============================================================================

// ToggleProviderModels enables or disables all models for a provider
func (s *modelService) ToggleProviderModels(ctx context.Context, organizationID uuid.UUID, provider string, isEnabled bool) error {
	// Legacy toggle APIs still address providers by slug, so we load active models
	// and filter in memory instead of adding another provider lookup dependency here.

	req := &dto.ListModelRequest{
		IsActive: boolPtr(true),
		Page:     1,
		PageSize: 1000, // Reasonable limit
	}
	models, _, err := s.ListGlobal(ctx, req)
	if err != nil {
		return err
	}

	var targetModels []*model.LLMModel
	for _, m := range models {
		if m.Provider == provider {
			targetModels = append(targetModels, m)
		}
	}

	if len(targetModels) == 0 {
		return errors.New("no models found for provider")
	}

	for _, m := range targetModels {
		if err := s.setModelConfigEnabled(ctx, organizationID, m.ID, isEnabled); err != nil {
			return err
		}
	}

	if s.availableModels != nil {
		s.availableModels.InvalidateTenantCache(organizationID)
	}

	return nil
}

// BatchToggleModels enables or disables specific models by IDs
func (s *modelService) BatchToggleModels(ctx context.Context, organizationID uuid.UUID, modelIDs []uuid.UUID, isEnabled bool) error {
	for _, modelID := range modelIDs {
		if err := s.setModelConfigEnabled(ctx, organizationID, modelID, isEnabled); err != nil {
			return err
		}
	}

	if s.availableModels != nil {
		s.availableModels.InvalidateTenantCache(organizationID)
	}

	return nil
}

func (s *modelService) setModelConfigEnabled(ctx context.Context, organizationID, modelID uuid.UUID, isEnabled bool) error {
	config, err := s.modelConfigForUpdate(ctx, organizationID, modelID)
	if err != nil {
		return err
	}
	config.IsEnabled = isEnabled
	if config.AccessScope == "" {
		config.AccessScope = model.AccessScopeAll
	}
	return s.configRepo.Upsert(ctx, config)
}

// ============================================================================
// Official models (provided by system channels)
// ============================================================================

// ListOfficialModels returns models that are actually provided by active official routes
func (s *modelService) ListOfficialModels(ctx context.Context) ([]*model.LLMModel, error) {
	// Get all enabled official routes (formerly system channels, now in llm_routes)
	var routes []channelmodel.LLMRoute
	err := s.db.WithContext(ctx).
		Where("is_official = true AND is_enabled = true").
		Find(&routes).Error

	if err != nil {
		return nil, fmt.Errorf("failed to query official routes: %w", err)
	}
	if err := officialmodel.HydrateRouteValues(ctx, s.db, routes); err != nil {
		return nil, fmt.Errorf("failed to hydrate official route models: %w", err)
	}

	// Collect all unique model names
	modelNameSet := make(map[string]bool)
	for _, route := range routes {
		for _, modelName := range route.GetEffectiveModels() {
			if modelName == "*" {
				continue
			}
			modelNameSet[modelName] = true
		}
	}

	if len(modelNameSet) == 0 {
		return []*model.LLMModel{}, nil
	}

	// Convert map keys to slice
	modelNames := make([]string, 0, len(modelNameSet))
	for name := range modelNameSet {
		modelNames = append(modelNames, name)
	}

	// Get model details for these model names
	var models []*model.LLMModel
	err = s.db.WithContext(ctx).
		Where("name IN ? AND is_active = ?", modelNames, true).
		Order("provider ASC, sort_order ASC, name ASC").
		Find(&models).Error

	if err != nil {
		return nil, fmt.Errorf("failed to get model details: %w", err)
	}

	return models, nil
}

// CheckAvailability implements the explicit model availability check for a tenant
func (s *modelService) CheckAvailability(ctx context.Context, organizationID uuid.UUID, modelID uuid.UUID) (*dto.ModelAvailabilityResponse, error) {
	if s.availabilitySvc == nil {
		return nil, errors.New("availability service not initialized")
	}
	return s.availabilitySvc.CheckModelAvailable(ctx, organizationID, modelID)
}

// BatchCheckAvailability implements the batch model availability check for a tenant
func (s *modelService) BatchCheckAvailability(ctx context.Context, organizationID uuid.UUID, req *dto.BatchModelAvailabilityRequest) (*dto.BatchModelAvailabilityResponse, error) {
	if s.availabilitySvc == nil {
		return nil, errors.New("availability service not initialized")
	}
	return s.availabilitySvc.BatchCheckAvailability(ctx, organizationID, req.Models)
}

func boolPtr(b bool) *bool {
	return &b
}

// getAvailableModelsFromRoutes returns a set of model names that have available channels for the tenant
// Checks both system channels and user-owned routes
func (s *modelService) getAvailableModelsFromRoutes(ctx context.Context, organizationID uuid.UUID) map[string]bool {
	availableModels := make(map[string]bool)

	// Get all enabled routes for this tenant (both system and user-owned)
	var routes []channelmodel.LLMRoute
	err := s.db.WithContext(ctx).
		Where("organization_id = ? AND is_enabled = true", organizationID).
		Find(&routes).Error

	if err != nil {
		return availableModels
	}
	_ = officialmodel.HydrateRouteValues(ctx, s.db, routes)

	for _, route := range routes {
		for _, modelName := range route.GetEffectiveModels() {
			if modelName == "*" {
				availableModels["*"] = true
			} else {
				availableModels[modelName] = true
			}
		}
	}

	return availableModels
}

// getSystemAvailableModels returns a set of model names available in official (ZGI Cloud) routes
// This is independent of tenant configuration - shows what's available system-wide via official channels
func (s *modelService) getSystemAvailableModels(ctx context.Context) map[string]bool {
	systemModels := make(map[string]bool)

	// Get all enabled official routes (formerly system channels, now in llm_routes)
	var routes []channelmodel.LLMRoute
	err := s.db.WithContext(ctx).
		Where("is_official = true AND is_enabled = true").
		Find(&routes).Error

	if err != nil {
		return systemModels
	}
	_ = officialmodel.HydrateRouteValues(ctx, s.db, routes)

	for _, route := range routes {
		for _, modelName := range route.GetEffectiveModels() {
			if modelName == "*" {
				systemModels["*"] = true
			} else {
				systemModels[modelName] = true
			}
		}
	}

	return systemModels
}

// ============================================================================
// Helper functions for default value handling
// ============================================================================

// getStatusOrDefault returns status if set, otherwise returns "active"
func getStatusOrDefault(status string) string {
	if status != "" {
		return status
	}
	return "active"
}

// getAccessTypeOrDefault returns access_type if set, otherwise derives from open_weights
func getAccessTypeOrDefault(accessType string, openWeights bool) string {
	if accessType != "" {
		return accessType
	}
	if openWeights {
		return "open"
	}
	return "closed"
}

// getCurrencyOrDefault returns currency if set, otherwise returns "USD"
func getCurrencyOrDefault(currency string) string {
	if currency != "" {
		return currency
	}
	return "USD"
}
