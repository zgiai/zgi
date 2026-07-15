package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	channelmodel "github.com/zgiai/zgi/api/internal/modules/llm/channel/model"
	channelrepo "github.com/zgiai/zgi/api/internal/modules/llm/channel/repository"
	"github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/dto"
	"github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/model"
	"github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/repository"
	providerrepo "github.com/zgiai/zgi/api/internal/modules/llm/provider/repository"
	"gorm.io/gorm"
)

// ModelAvailabilityService provides methods to check if models are usable by a tenant
type ModelAvailabilityService interface {
	// CheckModelAvailable checks if a single model is available for a tenant
	CheckModelAvailable(ctx context.Context, organizationID uuid.UUID, modelID uuid.UUID) (*dto.ModelAvailabilityResponse, error)

	// BatchCheckAvailability checks availability for multiple models
	BatchCheckAvailability(ctx context.Context, organizationID uuid.UUID, modelNames []string) (*dto.BatchModelAvailabilityResponse, error)
}

type modelAvailabilityService struct {
	modelRepo          repository.ModelRepository
	configRepo         repository.ModelConfigRepository
	routeRepo          channelrepo.TenantRouteRepository
	globalProviderRepo providerrepo.ProviderRepository
	providerConfigRepo providerrepo.ProviderConfigRepository
}

// NewModelAvailabilityService creates a new model availability service
func NewModelAvailabilityService(
	modelRepo repository.ModelRepository,
	configRepo repository.ModelConfigRepository,
	routeRepo channelrepo.TenantRouteRepository,
) ModelAvailabilityService {
	return NewModelAvailabilityServiceWithProviderRepos(modelRepo, configRepo, routeRepo, nil, nil)
}

func NewModelAvailabilityServiceWithProviderRepos(
	modelRepo repository.ModelRepository,
	configRepo repository.ModelConfigRepository,
	routeRepo channelrepo.TenantRouteRepository,
	globalProviderRepo providerrepo.ProviderRepository,
	providerConfigRepo providerrepo.ProviderConfigRepository,
) ModelAvailabilityService {
	return &modelAvailabilityService{
		modelRepo:          modelRepo,
		configRepo:         configRepo,
		routeRepo:          routeRepo,
		globalProviderRepo: globalProviderRepo,
		providerConfigRepo: providerConfigRepo,
	}
}

// CheckModelAvailable checks if a single model is available for a tenant
func (s *modelAvailabilityService) CheckModelAvailable(ctx context.Context, organizationID uuid.UUID, modelID uuid.UUID) (*dto.ModelAvailabilityResponse, error) {
	m, err := s.modelRepo.GetByID(ctx, modelID)
	if err != nil {
		return &dto.ModelAvailabilityResponse{
			Available: false,
			Message:   "Model not found in system repository",
		}, nil
	}

	visibility, err := loadProviderVisibility(
		ctx,
		organizationID,
		s.globalProviderRepo,
		s.providerConfigRepo,
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to load provider visibility: %w", err)
	}

	routes, err := s.routeRepo.GetEnabledRoutes(ctx, organizationID)
	if err != nil {
		return nil, fmt.Errorf("failed to get tenant routes: %w", err)
	}

	return s.availabilityForModel(ctx, organizationID, m, routes, visibility)
}

// BatchCheckAvailability checks availability for multiple models
func (s *modelAvailabilityService) BatchCheckAvailability(ctx context.Context, organizationID uuid.UUID, modelNames []string) (*dto.BatchModelAvailabilityResponse, error) {
	// Get all enabled routes for the tenant once
	routes, err := s.routeRepo.GetEnabledRoutes(ctx, organizationID)
	if err != nil {
		return nil, fmt.Errorf("failed to get tenant routes: %w", err)
	}

	// Get all global models to map names to providers.
	// Using a large limit to get all relevant models
	allModels, _, err := s.modelRepo.List(ctx, nil, "", "", "", nil, 0, 1000)
	if err != nil {
		return nil, fmt.Errorf("failed to list models: %w", err)
	}

	modelMap := make(map[string][]*model.LLMModel)
	for _, m := range allModels {
		modelMap[m.Model] = append(modelMap[m.Model], m)
	}

	result := &dto.BatchModelAvailabilityResponse{
		Items: make(map[string]*dto.ModelAvailabilityResponse),
	}

	visibility, err := loadProviderVisibility(
		ctx,
		organizationID,
		s.globalProviderRepo,
		s.providerConfigRepo,
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to load provider visibility: %w", err)
	}

	for _, name := range modelNames {
		candidates, exists := modelMap[name]
		if !exists {
			result.Items[name] = &dto.ModelAvailabilityResponse{
				Available: false,
				Message:   "Model not found in system",
			}
			continue
		}

		providers := make(map[string]struct{}, len(candidates))
		for _, candidate := range candidates {
			providers[candidate.Provider] = struct{}{}
		}
		if len(providers) > 1 {
			result.Items[name] = &dto.ModelAvailabilityResponse{
				Available: false,
				Message:   fmt.Sprintf("Model %q is ambiguous across multiple catalog providers", name),
			}
			continue
		}

		var availability *dto.ModelAvailabilityResponse
		for _, candidate := range candidates {
			candidateAvailability, err := s.availabilityForModel(ctx, organizationID, candidate, routes, visibility)
			if err != nil {
				return nil, err
			}
			if availability == nil || candidateAvailability.Available {
				availability = candidateAvailability
			}
			if candidateAvailability.Available {
				break
			}
		}
		result.Items[name] = availability
	}

	return result, nil
}

func (s *modelAvailabilityService) availabilityForModel(
	ctx context.Context,
	organizationID uuid.UUID,
	m *model.LLMModel,
	routes []*channelmodel.LLMRoute,
	visibility *providerVisibility,
) (*dto.ModelAvailabilityResponse, error) {
	if m == nil {
		return &dto.ModelAvailabilityResponse{
			Available: false,
			Message:   "Model not found in system",
		}, nil
	}

	if !m.IsActive || m.Status != model.ModelStatusActive {
		return &dto.ModelAvailabilityResponse{
			Available: false,
			Message:   "Model is currently inactive in the system",
		}, nil
	}

	cfg, err := s.configRepo.GetByModelID(ctx, organizationID, m.ID)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("failed to get tenant model config: %w", err)
	}
	if cfg != nil && !cfg.IsEnabled {
		return &dto.ModelAvailabilityResponse{
			Available: false,
			Message:   "Model is explicitly disabled for your tenant",
		}, nil
	}

	if !visibility.Allows(m.Provider) {
		return &dto.ModelAvailabilityResponse{
			Available:    false,
			ChannelCount: 0,
			Message:      "Model provider is disabled for your tenant",
		}, nil
	}

	channelCount := countSupportingRoutes(routes, m.Provider, m.Model)
	return &dto.ModelAvailabilityResponse{
		Available:    channelCount > 0,
		ChannelCount: channelCount,
		Message:      getAvailabilityMessage(channelCount),
	}, nil
}

func countSupportingRoutes(routes []*channelmodel.LLMRoute, provider, modelName string) int {
	count := 0
	for _, route := range routes {
		if route.SupportsModelForProvider(provider, modelName) {
			count++
		}
	}
	return count
}

func getAvailabilityMessage(count int) string {
	if count > 0 {
		return fmt.Sprintf("Model is available via %d channel(s)", count)
	}
	return "No active channels support this model for your tenant"
}
