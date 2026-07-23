package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/llm/availability/dto"
	channelmodel "github.com/zgiai/zgi/api/internal/modules/llm/channel/model"
	channelrepo "github.com/zgiai/zgi/api/internal/modules/llm/channel/repository"
	llmmodel "github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/model"
	llmrepo "github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/repository"
	providerrepo "github.com/zgiai/zgi/api/internal/modules/llm/provider/repository"
	"github.com/zgiai/zgi/api/pkg/logger"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type availabilityService struct {
	modelRepo          llmrepo.ModelRepository
	configRepo         llmrepo.ModelConfigRepository
	routeRepo          channelrepo.TenantRouteRepository
	globalProviderRepo providerrepo.ProviderRepository
	providerConfigRepo providerrepo.ProviderConfigRepository
}

// NewAvailabilityService creates a new availability service
func NewAvailabilityService(
	modelRepo llmrepo.ModelRepository,
	routeRepo channelrepo.TenantRouteRepository,
) AvailabilityService {
	return NewAvailabilityServiceWithProviderRepos(modelRepo, nil, routeRepo, nil, nil)
}

func NewAvailabilityServiceWithProviderRepos(
	modelRepo llmrepo.ModelRepository,
	configRepo llmrepo.ModelConfigRepository,
	routeRepo channelrepo.TenantRouteRepository,
	globalProviderRepo providerrepo.ProviderRepository,
	providerConfigRepo providerrepo.ProviderConfigRepository,
) AvailabilityService {
	return &availabilityService{
		modelRepo:          modelRepo,
		configRepo:         configRepo,
		routeRepo:          routeRepo,
		globalProviderRepo: globalProviderRepo,
		providerConfigRepo: providerConfigRepo,
	}
}

// CheckModelAvailability checks if a model is available for the tenant
func (s *availabilityService) CheckModelAvailability(
	ctx context.Context,
	organizationID, modelID uuid.UUID,
) (*dto.ModelAvailability, error) {
	// 1. Get model info
	modelRecord, err := s.modelRepo.GetByID(ctx, modelID)
	if err != nil {
		return nil, fmt.Errorf("failed to get model: %w", err)
	}
	if !modelRecordIsAvailable(modelRecord) {
		return s.unavailableModel(modelRecord, []string{"Model is currently inactive in the system"}), nil
	}

	tenantEnabled, err := s.isTenantModelEnabled(ctx, organizationID, modelRecord.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to load model config: %w", err)
	}
	if !tenantEnabled {
		return s.unavailableModel(modelRecord, []string{"Model is explicitly disabled for your tenant"}), nil
	}

	providerEnabled, err := s.isProviderEnabled(ctx, organizationID, modelRecord.Provider)
	if err != nil {
		return nil, fmt.Errorf("failed to load provider visibility: %w", err)
	}
	if !providerEnabled {
		return s.unavailableModel(modelRecord, []string{"Model provider is disabled for your tenant"}), nil
	}

	// 2. Get all enabled routes for this tenant
	routes, err := s.routeRepo.GetEnabledRoutes(ctx, organizationID)
	if err != nil {
		return nil, fmt.Errorf("failed to get routes: %w", err)
	}

	// 3. Filter routes that support this model
	supportedRoutes := s.filterRoutesForModel(routes, modelRecord.Provider, modelRecord.Model)

	// 4. Analyze channel status
	channelInfo := s.analyzeChannelStatus(supportedRoutes)

	// 5. Determine overall status
	status := s.determineStatus(channelInfo)

	return &dto.ModelAvailability{
		ModelID:     modelRecord.ID,
		ModelName:   modelRecord.Model,
		Provider:    modelRecord.Provider,
		Status:      status,
		ChannelInfo: channelInfo,
		UpdatedAt:   time.Now(),
	}, nil
}

// BatchCheckAvailability checks multiple models at once
func (s *availabilityService) BatchCheckAvailability(
	ctx context.Context,
	organizationID uuid.UUID,
	modelIDs []uuid.UUID,
) ([]*dto.ModelAvailability, error) {
	results := make([]*dto.ModelAvailability, 0, len(modelIDs))

	for _, modelID := range modelIDs {
		availability, err := s.CheckModelAvailability(ctx, organizationID, modelID)
		if err != nil {
			logger.WarnContext(ctx, "Model availability check failed",
				zap.Stringer("organization_id", organizationID),
				zap.Stringer("model_id", modelID),
				zap.Error(err),
			)
			return nil, err
		}
		results = append(results, availability)
	}

	return results, nil
}

// filterRoutesForModel filters routes that support the given model
func (s *availabilityService) filterRoutesForModel(
	routes []*channelmodel.LLMRoute,
	provider string,
	modelName string,
) []*channelmodel.LLMRoute {
	var validRoutes []*channelmodel.LLMRoute

	for _, route := range routes {
		if route.SupportsModelForProvider(provider, modelName) {
			validRoutes = append(validRoutes, route)
		}
	}

	return validRoutes
}

// analyzeChannelStatus analyzes the status of channels
func (s *availabilityService) analyzeChannelStatus(
	routes []*channelmodel.LLMRoute,
) dto.ChannelAvailabilityInfo {
	info := dto.ChannelAvailabilityInfo{
		TotalCount: len(routes),
		Channels:   make([]dto.ChannelBriefInfo, 0, len(routes)),
		Warnings:   []string{},
	}

	for _, route := range routes {
		channelBrief := s.buildChannelBrief(route)
		info.Channels = append(info.Channels, channelBrief)

		// Count by status
		switch channelBrief.Status {
		case dto.ChannelReady:
			info.ReadyCount++
		case dto.ChannelNeedsConfig:
			info.NeedsConfigCount++
		}
	}

	// Add warnings
	if info.ReadyCount == 0 && info.TotalCount > 0 {
		info.Warnings = append(info.Warnings, "All channels need configuration")
	} else if info.ReadyCount == 1 {
		info.Warnings = append(info.Warnings, "Only one ready channel, consider adding backup channels")
	}

	return info
}

// buildChannelBrief builds brief info for a channel
func (s *availabilityService) buildChannelBrief(
	route *channelmodel.LLMRoute,
) dto.ChannelBriefInfo {
	brief := dto.ChannelBriefInfo{
		ID:       route.ID,
		Priority: route.Priority,
		Weight:   route.Weight,
	}

	// Determine status based on route type and config
	// Use route's own fields
	brief.Name = route.Name
	brief.Provider = route.ChannelProvider

	// Check if route has credential configured
	if route.IsOfficial {
		// Official channels are always ready (proxied to Console)
		brief.Status = dto.ChannelReady
	} else if route.CredentialID != nil && *route.CredentialID != uuid.Nil {
		// Private channel with credential
		brief.Status = dto.ChannelReady
	} else {
		// No credential configured
		brief.Status = dto.ChannelNeedsConfig
	}

	if !route.IsEnabled {
		brief.Status = dto.ChannelInactive
	}

	return brief
}

// determineStatus determines overall availability status
func (s *availabilityService) determineStatus(
	info dto.ChannelAvailabilityInfo,
) dto.ModelAvailabilityStatus {
	if info.ReadyCount > 0 {
		return dto.ModelAvailable
	}
	if info.NeedsConfigCount > 0 {
		return dto.ModelPartial
	}
	return dto.ModelUnavailable
}

func (s *availabilityService) isProviderEnabled(ctx context.Context, organizationID uuid.UUID, provider string) (bool, error) {
	if s.globalProviderRepo == nil || s.providerConfigRepo == nil {
		return true, nil
	}

	providerName := strings.ToLower(strings.TrimSpace(provider))
	if providerName == "" {
		return false, nil
	}

	providers, _, err := s.globalProviderRepo.List(ctx, boolPtr(true), 0, 1000)
	if err != nil {
		return false, err
	}

	providerNamesByID := make(map[uuid.UUID]string, len(providers))
	enabledByName := make(map[string]bool, len(providers))
	for _, provider := range providers {
		name := strings.ToLower(strings.TrimSpace(provider.Provider))
		if name == "" {
			continue
		}
		providerNamesByID[provider.ID] = name
		enabledByName[name] = true
	}

	configs, _, err := s.providerConfigRepo.List(ctx, organizationID, nil, 0, 1000)
	if err != nil {
		return false, err
	}
	for _, cfg := range configs {
		name, ok := providerNamesByID[cfg.ProviderID]
		if !ok {
			continue
		}
		enabledByName[name] = cfg.IsEnabled
	}

	enabled, ok := enabledByName[providerName]
	if !ok {
		return false, nil
	}
	return enabled, nil
}

func (s *availabilityService) isTenantModelEnabled(ctx context.Context, organizationID uuid.UUID, modelID uuid.UUID) (bool, error) {
	if s.configRepo == nil {
		return true, nil
	}

	config, err := s.configRepo.GetByModelID(ctx, organizationID, modelID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return true, nil
		}
		return false, err
	}
	return config == nil || config.IsEnabled, nil
}

func (s *availabilityService) unavailableModel(modelRecord *llmmodel.LLMModel, warnings []string) *dto.ModelAvailability {
	result := &dto.ModelAvailability{
		Status: dto.ModelUnavailable,
		ChannelInfo: dto.ChannelAvailabilityInfo{
			Warnings: warnings,
		},
		UpdatedAt: time.Now(),
	}
	if modelRecord != nil {
		result.ModelID = modelRecord.ID
		result.ModelName = modelRecord.Model
		result.Provider = modelRecord.Provider
	}
	return result
}

func modelRecordIsAvailable(modelRecord *llmmodel.LLMModel) bool {
	return modelRecord != nil && modelRecord.IsActive && modelRecord.Status == llmmodel.ModelStatusActive
}

func boolPtr(b bool) *bool {
	return &b
}
