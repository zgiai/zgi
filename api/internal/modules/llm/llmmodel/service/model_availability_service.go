package service

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	channelrepo "github.com/zgiai/zgi/api/internal/modules/llm/channel/repository"
	"github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/dto"
	"github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/model"
	"github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/repository"
	providerrepo "github.com/zgiai/zgi/api/internal/modules/llm/provider/repository"
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
	// 1. Get model details
	m, err := s.modelRepo.GetByID(ctx, modelID)
	if err != nil {
		return &dto.ModelAvailabilityResponse{
			Available: false,
			Message:   "Model not found in system repository",
		}, nil
	}

	// 2. Check if model is active and system enabled
	if !m.IsActive {
		return &dto.ModelAvailabilityResponse{
			Available: false,
			Message:   "Model is currently inactive in the system",
		}, nil
	}

	if !m.IsActive {
		return &dto.ModelAvailabilityResponse{
			Available: false,
			Message:   "Model is disabled globally by system administrator",
		}, nil
	}

	// 3. Check if model is enabled for this tenant
	cfg, err := s.configRepo.GetByModelID(ctx, organizationID, modelID)
	if err == nil && cfg != nil && !cfg.IsEnabled {
		return &dto.ModelAvailabilityResponse{
			Available: false,
			Message:   "Model is explicitly disabled for your tenant",
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
	if !visibility.Allows(m.Provider) {
		return &dto.ModelAvailabilityResponse{
			Available:    false,
			ChannelCount: 0,
			Message:      "Model provider is disabled for your tenant",
		}, nil
	}

	// 4. Check available routes/channels
	routes, err := s.routeRepo.GetEnabledRoutes(ctx, organizationID)
	if err != nil {
		return nil, fmt.Errorf("failed to get tenant routes: %w", err)
	}

	channelCount := 0
	for _, route := range routes {
		if route.SupportsModel(m.Model) {
			channelCount++
		}
	}

	return &dto.ModelAvailabilityResponse{
		Available:    channelCount > 0,
		ChannelCount: channelCount,
		Message:      getAvailabilityMessage(channelCount),
	}, nil
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
	allModels, _, err := s.modelRepo.List(ctx, nil, "", "", nil, 0, 1000)
	if err != nil {
		return nil, fmt.Errorf("failed to list models: %w", err)
	}

	modelMap := make(map[string]*model.LLMModel)
	for _, m := range allModels {
		modelMap[m.Model] = m
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
		m, exists := modelMap[name]
		if !exists {
			result.Items[name] = &dto.ModelAvailabilityResponse{
				Available: false,
				Message:   "Model not found in system",
			}
			continue
		}

		if !visibility.Allows(m.Provider) {
			result.Items[name] = &dto.ModelAvailabilityResponse{
				Available:    false,
				ChannelCount: 0,
				Message:      "Model provider is disabled for your tenant",
			}
			continue
		}

		// Calculate availability based on routes
		channelCount := 0
		for _, route := range routes {
			if route.SupportsModel(m.Model) {
				channelCount++
			}
		}

		result.Items[name] = &dto.ModelAvailabilityResponse{
			Available:    channelCount > 0,
			ChannelCount: channelCount,
			Message:      getAvailabilityMessage(channelCount),
		}
	}

	return result, nil
}

func getAvailabilityMessage(count int) string {
	if count > 0 {
		return fmt.Sprintf("Model is available via %d channel(s)", count)
	}
	return "No active channels support this model for your tenant"
}
