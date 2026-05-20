package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	llmmodelmodel "github.com/zgiai/ginext/internal/modules/llm/llmmodel/model"
	llmmodelrepo "github.com/zgiai/ginext/internal/modules/llm/llmmodel/repository"
	llmmodelservice "github.com/zgiai/ginext/internal/modules/llm/llmmodel/service"
	"github.com/zgiai/ginext/internal/modules/llm/provider/dto"
	"github.com/zgiai/ginext/internal/modules/llm/provider/model"
	"github.com/zgiai/ginext/internal/modules/llm/provider/repository"
	"gorm.io/gorm"
)

var (
	ErrProviderNotFound = errors.New("provider not found")
	ErrProviderExists   = errors.New("provider already exists")
)

type providerService struct {
	globalRepo      repository.ProviderRepository
	configRepo      repository.ProviderConfigRepository
	customRepo      repository.CustomProviderRepository
	modelRepo       llmmodelrepo.ModelRepository
	modelConfigRepo llmmodelrepo.ModelConfigRepository
	availableModels llmmodelservice.AvailableModelsService
	db              *gorm.DB
}

// NewProviderService creates a new provider service
func NewProviderService(
	db *gorm.DB,
	globalRepo repository.ProviderRepository,
	configRepo repository.ProviderConfigRepository,
	customRepo repository.CustomProviderRepository,
	modelRepo llmmodelrepo.ModelRepository,
	modelConfigRepo llmmodelrepo.ModelConfigRepository,
	availableModels llmmodelservice.AvailableModelsService,
) ProviderService {
	return &providerService{
		globalRepo:      globalRepo,
		configRepo:      configRepo,
		customRepo:      customRepo,
		modelRepo:       modelRepo,
		modelConfigRepo: modelConfigRepo,
		availableModels: availableModels,
		db:              db,
	}
}

func (s *providerService) SetAvailableModelsService(svc llmmodelservice.AvailableModelsService) {
	s.availableModels = svc
}

func (s *providerService) invalidateAvailableModelsCache(organizationID uuid.UUID) {
	if s.availableModels != nil {
		s.availableModels.InvalidateTenantCache(organizationID)
	}
}

// ============================================================================
// Global provider operations
// ============================================================================

func (s *providerService) CreateGlobal(ctx context.Context, req *dto.CreateProviderRequest) (*model.LLMProvider, error) {
	exists, err := s.globalRepo.ExistsByName(ctx, req.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to check provider existence: %w", err)
	}
	if exists {
		return nil, ErrProviderExists
	}

	provider := &model.LLMProvider{
		Provider:     req.Name,
		ProviderName: req.ProviderName,
		APIBaseURL:   req.APIBaseURL,
		LogoURL:      req.LogoURL,
		APIDocsURL:   req.APIDocsURL,
		Description:  req.Description,
		ProviderType: req.ProviderType,
		Website:      req.Website,
		PricingURL:   req.PricingURL,
		Tagline:      req.Tagline,
		CountryCode:  req.CountryCode,
		FoundedYear:  req.FoundedYear,
		IsActive:     true,
	}

	if provider.ProviderType == "" {
		provider.ProviderType = "vendor"
	}

	if err := s.globalRepo.Create(ctx, provider); err != nil {
		return nil, fmt.Errorf("failed to create provider: %w", err)
	}

	return provider, nil
}

func (s *providerService) GetGlobal(ctx context.Context, id uuid.UUID) (*model.LLMProvider, error) {
	provider, err := s.globalRepo.GetByID(ctx, id)
	if err != nil {
		return nil, ErrProviderNotFound
	}
	return provider, nil
}

func (s *providerService) ListGlobal(ctx context.Context, req *dto.ListProviderRequest) ([]*model.LLMProvider, int64, error) {
	offset := (req.Page - 1) * req.PageSize
	return s.globalRepo.List(ctx, req.IsActive, offset, req.PageSize)
}

func (s *providerService) UpdateGlobal(ctx context.Context, id uuid.UUID, req *dto.UpdateProviderRequest) (*model.LLMProvider, error) {
	provider, err := s.globalRepo.GetByID(ctx, id)
	if err != nil {
		return nil, ErrProviderNotFound
	}

	if req.ProviderName != nil {
		provider.ProviderName = *req.ProviderName
	}
	if req.APIBaseURL != nil {
		provider.APIBaseURL = *req.APIBaseURL
	}
	if req.LogoURL != nil {
		provider.LogoURL = *req.LogoURL
	}
	if req.APIDocsURL != nil {
		provider.APIDocsURL = *req.APIDocsURL
	}
	if req.Description != nil {
		provider.Description = *req.Description
	}
	if req.IsActive != nil {
		provider.IsActive = *req.IsActive
	}
	if req.SortOrder != nil {
		provider.SortOrder = *req.SortOrder
	}

	// ModelMeta fields
	if req.Website != nil {
		provider.Website = *req.Website
	}
	if req.PricingURL != nil {
		provider.PricingURL = *req.PricingURL
	}
	if req.Tagline != nil {
		provider.Tagline = *req.Tagline
	}
	if req.CountryCode != nil {
		provider.CountryCode = *req.CountryCode
	}
	if req.FoundedYear != nil {
		provider.FoundedYear = *req.FoundedYear
	}

	if err := s.globalRepo.Update(ctx, provider); err != nil {
		return nil, fmt.Errorf("failed to update provider: %w", err)
	}

	return provider, nil
}

func (s *providerService) DeleteGlobal(ctx context.Context, id uuid.UUID) error {
	return s.globalRepo.Delete(ctx, id)
}

// ============================================================================
// Provider config operations
// ============================================================================

func (s *providerService) ConfigureProvider(ctx context.Context, organizationID uuid.UUID, req *dto.ConfigureProviderRequest) (*model.ProviderConfig, error) {
	// Verify global provider exists
	_, err := s.globalRepo.GetByID(ctx, req.ProviderID)
	if err != nil {
		return nil, ErrProviderNotFound
	}

	config := &model.ProviderConfig{
		OrganizationID:    organizationID,
		ProviderID:        req.ProviderID,
		IsEnabled:         true,
		CustomDisplayName: req.CustomDisplayName,
		CustomAPIBaseURL:  req.CustomAPIBaseURL,
		CustomLogoURL:     req.CustomLogoURL,
	}

	if req.IsEnabled != nil {
		config.IsEnabled = *req.IsEnabled
	}
	if req.SortOrder != nil {
		config.SortOrder = *req.SortOrder
	}

	if err := s.configRepo.Upsert(ctx, config); err != nil {
		return nil, fmt.Errorf("failed to configure provider: %w", err)
	}

	s.invalidateAvailableModelsCache(organizationID)

	return config, nil
}

func (s *providerService) GetProviderConfig(ctx context.Context, organizationID, providerID uuid.UUID) (*model.ProviderConfig, error) {
	config, err := s.configRepo.GetByProviderID(ctx, organizationID, providerID)
	if err != nil {
		return nil, ErrProviderNotFound
	}
	return config, nil
}

func (s *providerService) ListProviderConfigs(ctx context.Context, organizationID uuid.UUID, req *dto.ListProviderRequest) ([]*model.ProviderConfig, int64, error) {
	offset := (req.Page - 1) * req.PageSize
	isEnabled := req.IsActive // Reuse IsActive as IsEnabled filter
	return s.configRepo.List(ctx, organizationID, isEnabled, offset, req.PageSize)
}

// ============================================================================
// Custom provider operations
// ============================================================================

func (s *providerService) CreateCustom(ctx context.Context, organizationID uuid.UUID, req *dto.CreateCustomProviderRequest) (*model.CustomProvider, error) {
	exists, err := s.customRepo.ExistsByProvider(ctx, organizationID, req.Provider)
	if err != nil {
		return nil, fmt.Errorf("failed to check provider existence: %w", err)
	}
	if exists {
		return nil, ErrProviderExists
	}

	provider := &model.CustomProvider{
		OrganizationID: organizationID,
		Provider:       req.Provider,
		ProviderName:   req.ProviderName,
		APIBaseURL:     req.APIBaseURL,
		LogoURL:        req.LogoURL,
		APIDocsURL:     req.APIDocsURL,
		Description:    req.Description,
		IsActive:       true,
	}

	if err := s.customRepo.Create(ctx, provider); err != nil {
		return nil, fmt.Errorf("failed to create custom provider: %w", err)
	}

	return provider, nil
}

func (s *providerService) GetCustom(ctx context.Context, organizationID, id uuid.UUID) (*model.CustomProvider, error) {
	provider, err := s.customRepo.GetByID(ctx, organizationID, id)
	if err != nil {
		return nil, ErrProviderNotFound
	}
	return provider, nil
}

func (s *providerService) ListCustom(ctx context.Context, organizationID uuid.UUID, req *dto.ListProviderRequest) ([]*model.CustomProvider, int64, error) {
	offset := (req.Page - 1) * req.PageSize
	return s.customRepo.List(ctx, organizationID, req.IsActive, offset, req.PageSize)
}

func (s *providerService) UpdateCustom(ctx context.Context, organizationID, id uuid.UUID, req *dto.UpdateCustomProviderRequest) (*model.CustomProvider, error) {
	provider, err := s.customRepo.GetByID(ctx, organizationID, id)
	if err != nil {
		return nil, ErrProviderNotFound
	}

	if req.ProviderName != nil {
		provider.ProviderName = *req.ProviderName
	}
	if req.APIBaseURL != nil {
		provider.APIBaseURL = *req.APIBaseURL
	}
	if req.LogoURL != nil {
		provider.LogoURL = *req.LogoURL
	}
	if req.APIDocsURL != nil {
		provider.APIDocsURL = *req.APIDocsURL
	}
	if req.Description != nil {
		provider.Description = *req.Description
	}
	if req.IsActive != nil {
		provider.IsActive = *req.IsActive
	}
	if req.SortOrder != nil {
		provider.SortOrder = *req.SortOrder
	}

	if err := s.customRepo.Update(ctx, provider); err != nil {
		return nil, fmt.Errorf("failed to update custom provider: %w", err)
	}

	return provider, nil
}

func (s *providerService) DeleteCustom(ctx context.Context, organizationID, id uuid.UUID) error {
	return s.customRepo.Delete(ctx, organizationID, id)
}

// ============================================================================
// Aggregated operations
// ============================================================================

// ListTenantProviders returns all providers available to an organization (global + custom)
// Only returns providers where is_active=true
func (s *providerService) ListTenantProviders(ctx context.Context, organizationID uuid.UUID) ([]*model.ProviderView, error) {
	var result []*model.ProviderView

	// Get all active global providers
	globalProviders, _, err := s.globalRepo.List(ctx, boolPtr(true), 0, 1000)
	if err != nil {
		return nil, fmt.Errorf("failed to list global providers: %w", err)
	}

	// Filter to only include active providers
	var enabledProviders []*model.LLMProvider
	for _, p := range globalProviders {
		if p.IsActive {
			enabledProviders = append(enabledProviders, p)
		}
	}
	globalProviders = enabledProviders

	// Get organization's provider configs
	configs, _, err := s.configRepo.List(ctx, organizationID, nil, 0, 1000)
	if err != nil {
		return nil, fmt.Errorf("failed to list provider configs: %w", err)
	}

	// Build config map for quick lookup
	configMap := make(map[uuid.UUID]*model.ProviderConfig)
	for _, cfg := range configs {
		configMap[cfg.ProviderID] = cfg
	}

	// Batch queries for availability, channel counts, and model counts
	availableProviders := s.getAvailableProviders(ctx, organizationID)
	channelCounts := s.getChannelCounts(ctx, organizationID)
	modelCounts := s.getModelCounts(ctx)

	// Add global providers with organization config applied
	for _, p := range globalProviders {
		view := &model.ProviderView{
			ID:           p.ID,
			Object:       "provider",
			Name:         p.Provider,
			DisplayName:  p.ProviderName,
			APIBaseURL:   p.APIBaseURL,
			LogoURL:      p.LogoURL,
			ProviderType: "global",
			IsEnabled:    true, // Default enabled
			IsAvailable:  availableProviders[p.Provider],
			SortOrder:    p.SortOrder,

			// Extended fields
			Description:  p.Description,
			Website:      p.Website,
			APIDocsURL:   p.APIDocsURL,
			PricingURL:   p.PricingURL,
			CountryCode:  p.CountryCode,
			FoundedYear:  p.FoundedYear,
			Tagline:      p.Tagline,
			ModelCount:   modelCounts[p.Provider],
			ChannelCount: channelCounts[p.Provider],
			Metadata:     p.Metadata,
			CreatedAt:    p.CreatedAt.Unix(),
			UpdatedAt:    p.UpdatedAt.Unix(),
		}

		// Apply organization config if exists
		if cfg, ok := configMap[p.ID]; ok {
			view.IsEnabled = cfg.IsEnabled
			if cfg.CustomDisplayName != "" {
				view.DisplayName = cfg.CustomDisplayName
			}
			if cfg.CustomAPIBaseURL != "" {
				view.APIBaseURL = cfg.CustomAPIBaseURL
			}
			if cfg.CustomLogoURL != "" {
				view.LogoURL = cfg.CustomLogoURL
			}
			view.SortOrder = cfg.SortOrder
		}

		result = append(result, view)
	}

	// Get organization's custom providers
	customProviders, _, err := s.customRepo.List(ctx, organizationID, boolPtr(true), 0, 1000)
	if err != nil {
		return nil, fmt.Errorf("failed to list custom providers: %w", err)
	}

	// Add custom providers
	for _, p := range customProviders {
		view := &model.ProviderView{
			ID:           p.ID,
			Object:       "provider",
			Name:         p.Provider,
			DisplayName:  p.ProviderName,
			APIBaseURL:   p.APIBaseURL,
			LogoURL:      p.LogoURL,
			ProviderType: "custom",
			IsEnabled:    p.IsActive,
			IsAvailable:  availableProviders[p.Provider],
			SortOrder:    p.SortOrder,

			// Extended fields
			Description:  p.Description,
			APIDocsURL:   p.APIDocsURL,
			ChannelCount: channelCounts[p.Provider],
			Metadata:     p.Metadata,
			CreatedAt:    p.CreatedAt.Unix(),
			UpdatedAt:    p.UpdatedAt.Unix(),
		}
		result = append(result, view)
	}

	return result, nil
}

// GetTenantProvider returns a single provider by name or ID for an organization
func (s *providerService) GetTenantProvider(ctx context.Context, organizationID uuid.UUID, providerIdentifier string) (*model.ProviderView, error) {
	// Try to parse as UUID first
	if id, err := uuid.Parse(providerIdentifier); err == nil {
		// Search by ID in global providers
		globalProvider, err := s.globalRepo.GetByID(ctx, id)
		if err == nil && globalProvider.IsActive {
			return s.buildProviderDetailView(ctx, organizationID, globalProvider, nil)
		}

		// Search by ID in custom providers
		customProvider, err := s.customRepo.GetByID(ctx, organizationID, id)
		if err == nil && customProvider.IsActive {
			return s.buildCustomProviderDetailView(ctx, organizationID, customProvider)
		}
	}

	// Search by name in global providers
	providers, _, err := s.globalRepo.List(ctx, boolPtr(true), 0, 1000)
	if err != nil {
		return nil, fmt.Errorf("failed to list global providers: %w", err)
	}

	for _, p := range providers {
		if p.Provider == providerIdentifier && p.IsActive {
			return s.buildProviderDetailView(ctx, organizationID, p, nil)
		}
	}

	// Search by name in custom providers
	customProviders, _, err := s.customRepo.List(ctx, organizationID, boolPtr(true), 0, 1000)
	if err != nil {
		return nil, fmt.Errorf("failed to list custom providers: %w", err)
	}

	for _, p := range customProviders {
		if p.Provider == providerIdentifier && p.IsActive {
			return s.buildCustomProviderDetailView(ctx, organizationID, p)
		}
	}

	return nil, ErrProviderNotFound
}

func (s *providerService) buildProviderDetailView(ctx context.Context, organizationID uuid.UUID, p *model.LLMProvider, config *model.ProviderConfig) (*model.ProviderView, error) {
	// Get organization config if not provided
	if config == nil {
		config, _ = s.configRepo.GetByProviderID(ctx, organizationID, p.ID)
	}

	// Batch queries for counts
	availableProviders := s.getAvailableProviders(ctx, organizationID)
	channelCounts := s.getChannelCounts(ctx, organizationID)
	modelCounts := s.getModelCounts(ctx)

	// Build view with all fields
	view := &model.ProviderView{
		ID:           p.ID,
		Object:       "provider",
		Name:         p.Provider,
		DisplayName:  p.ProviderName,
		APIBaseURL:   p.APIBaseURL,
		LogoURL:      p.LogoURL,
		ProviderType: "global",
		IsEnabled:    true,
		IsAvailable:  availableProviders[p.Provider],
		SortOrder:    p.SortOrder,

		// Extended fields
		Description:  p.Description,
		Website:      p.Website,
		APIDocsURL:   p.APIDocsURL,
		PricingURL:   p.PricingURL,
		CountryCode:  p.CountryCode,
		FoundedYear:  p.FoundedYear,
		Tagline:      p.Tagline,
		ModelCount:   modelCounts[p.Provider],
		ChannelCount: channelCounts[p.Provider],
		Metadata:     p.Metadata,
		CreatedAt:    p.CreatedAt.Unix(),
		UpdatedAt:    p.UpdatedAt.Unix(),
	}

	// Apply organization config if exists
	if config != nil {
		view.IsEnabled = config.IsEnabled
		if config.CustomDisplayName != "" {
			view.DisplayName = config.CustomDisplayName
		}
		if config.CustomAPIBaseURL != "" {
			view.APIBaseURL = config.CustomAPIBaseURL
		}
		if config.CustomLogoURL != "" {
			view.LogoURL = config.CustomLogoURL
		}
		view.SortOrder = config.SortOrder
	}

	return view, nil
}

func (s *providerService) buildCustomProviderDetailView(ctx context.Context, organizationID uuid.UUID, p *model.CustomProvider) (*model.ProviderView, error) {
	// Check channel availability and count for this provider
	availableProviders := s.getAvailableProviders(ctx, organizationID)
	channelCounts := s.getChannelCounts(ctx, organizationID)

	view := &model.ProviderView{
		ID:           p.ID,
		Object:       "provider",
		Name:         p.Provider,
		DisplayName:  p.ProviderName,
		APIBaseURL:   p.APIBaseURL,
		LogoURL:      p.LogoURL,
		ProviderType: "custom",
		IsEnabled:    p.IsActive,
		IsAvailable:  availableProviders[p.Provider],
		SortOrder:    p.SortOrder,

		// Extended fields
		Description:  p.Description,
		APIDocsURL:   p.APIDocsURL,
		ChannelCount: channelCounts[p.Provider],
		Metadata:     p.Metadata,
		CreatedAt:    p.CreatedAt.Unix(),
		UpdatedAt:    p.UpdatedAt.Unix(),
	}

	return view, nil
}

// getAvailableProviders returns a map of provider names that have at least one
// active channel (official or private) for the given organization.
func (s *providerService) getAvailableProviders(ctx context.Context, organizationID uuid.UUID) map[string]bool {
	result := make(map[string]bool)

	var providers []string
	s.db.WithContext(ctx).
		Table("llm_routes").
		Select("DISTINCT provider").
		Where("organization_id = ? AND is_enabled = true AND deleted_at IS NULL AND provider != ''", organizationID).
		Pluck("provider", &providers)

	for _, p := range providers {
		result[p] = true
	}
	return result
}

// getModelCounts returns a map of provider name -> active model count.
func (s *providerService) getModelCounts(ctx context.Context) map[string]int {
	result := make(map[string]int)

	type providerCount struct {
		Provider string
		Count    int
	}
	var counts []providerCount
	s.db.WithContext(ctx).
		Table("llm_models").
		Select("provider, COUNT(*) as count").
		Where("is_active = true AND deleted_at IS NULL AND provider != ''").
		Group("provider").
		Scan(&counts)

	for _, c := range counts {
		result[c.Provider] = c.Count
	}
	return result
}

// getChannelCounts returns a map of provider name -> active channel count for the given organization.
func (s *providerService) getChannelCounts(ctx context.Context, organizationID uuid.UUID) map[string]int {
	result := make(map[string]int)

	type providerCount struct {
		Provider string
		Count    int
	}
	var counts []providerCount
	s.db.WithContext(ctx).
		Table("llm_routes").
		Select("provider, COUNT(*) as count").
		Where("organization_id = ? AND is_enabled = true AND deleted_at IS NULL AND provider != ''", organizationID).
		Group("provider").
		Scan(&counts)

	for _, c := range counts {
		result[c.Provider] = c.Count
	}
	return result
}

func boolPtr(b bool) *bool {
	return &b
}

// ToggleProvider enables or disables a provider for an organization
func (s *providerService) ToggleProvider(ctx context.Context, organizationID uuid.UUID, provider string, isEnabled bool) error {
	// Find the global provider by name
	providers, _, err := s.globalRepo.List(ctx, nil, 0, 1000)
	if err != nil {
		return fmt.Errorf("failed to list providers: %w", err)
	}

	var targetProvider *model.LLMProvider
	for _, p := range providers {
		if p.Provider == provider {
			targetProvider = p
			break
		}
	}

	if targetProvider == nil {
		return fmt.Errorf("provider not found: %s", provider)
	}

	// Upsert the provider config
	config := &model.ProviderConfig{
		OrganizationID: organizationID,
		ProviderID:     targetProvider.ID,
		IsEnabled:      isEnabled,
	}

	if err := s.configRepo.Upsert(ctx, config); err != nil {
		return err
	}

	s.invalidateAvailableModelsCache(organizationID)
	return nil
}

// GetProviderDetail gets detailed provider information with models
func (s *providerService) GetProviderDetail(ctx context.Context, organizationID uuid.UUID, provider string) (*dto.ProviderDetailResponse, error) {
	// Find the global provider by name
	providers, _, err := s.globalRepo.List(ctx, nil, 0, 1000)
	if err != nil {
		return nil, fmt.Errorf("failed to list providers: %w", err)
	}

	var targetProvider *model.LLMProvider
	for _, p := range providers {
		if p.Provider == provider {
			targetProvider = p
			break
		}
	}

	if targetProvider == nil {
		return nil, fmt.Errorf("provider not found: %s", provider)
	}

	// Get config for this provider
	config, _ := s.configRepo.GetByProviderID(ctx, organizationID, targetProvider.ID)
	isEnabled := true
	if config != nil {
		isEnabled = config.IsEnabled
	}

	// Build response (models would need to be fetched from model service, but we return basic info here)
	return &dto.ProviderDetailResponse{
		Provider:    targetProvider.Provider,
		DisplayName: targetProvider.ProviderName,
		LogoURL:     targetProvider.LogoURL,
		IsEnabled:   isEnabled,
	}, nil
}

// ToggleModel enables or disables a model for an organization under a provider
func (s *providerService) ToggleModel(ctx context.Context, organizationID uuid.UUID, provider string, modelName string, isEnabled bool) error {
	// Find the model by provider name and model name
	models, _, err := s.modelRepo.List(ctx, nil, "", "", nil, 0, 10000)
	if err != nil {
		return fmt.Errorf("failed to list models: %w", err)
	}

	var targetModel *llmmodelmodel.LLMModel
	for _, m := range models {
		if m.Provider == provider && m.Model == modelName {
			targetModel = m
			break
		}
	}

	if targetModel == nil {
		return fmt.Errorf("model %s not found for provider %s", modelName, provider)
	}

	// Upsert model config
	config := &llmmodelmodel.ModelConfig{
		OrganizationID: organizationID,
		ModelID:        targetModel.ID,
		IsEnabled:      isEnabled,
		AccessScope:    "all",
	}

	if err := s.modelConfigRepo.Upsert(ctx, config); err != nil {
		return fmt.Errorf("failed to toggle model: %w", err)
	}

	s.invalidateAvailableModelsCache(organizationID)
	return nil
}
