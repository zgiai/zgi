package gateway

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"math/rand"
	"sort"
	"strings"

	"github.com/google/uuid"
	appconfig "github.com/zgiai/zgi/api/config"
	platformchannel "github.com/zgiai/zgi/api/internal/infra/platform/channel"
	channelmodel "github.com/zgiai/zgi/api/internal/modules/llm/channel/model"
	channelrepo "github.com/zgiai/zgi/api/internal/modules/llm/channel/repository"
	"github.com/zgiai/zgi/api/internal/modules/llm/channelprovider"
	credentialmodel "github.com/zgiai/zgi/api/internal/modules/llm/credential/model"
	credentialrepo "github.com/zgiai/zgi/api/internal/modules/llm/credential/repository"
	"github.com/zgiai/zgi/api/internal/modules/llm/credential/upstreamstate"
	llmerrors "github.com/zgiai/zgi/api/internal/modules/llm/errors"
	llmmodel "github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/model"
	llmmodelsvc "github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/service"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	providermodel "github.com/zgiai/zgi/api/internal/modules/llm/provider/model"
	"github.com/zgiai/zgi/api/internal/modules/llm/shared"
	"github.com/zgiai/zgi/api/pkg/logger"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// ChannelRouter handles provider selection using the V2 channel architecture
// This replaces the legacy ProviderRouter that used llm_organizationID_channels table
type ChannelRouter struct {
	db                      *gorm.DB
	organizationIDRouteRepo channelrepo.TenantRouteRepository
	organizationIDCredRepo  credentialrepo.TenantCredentialRepository
	cryptoService           shared.CryptoService
	configCache             *ConfigCache
	strategyFactory         *StrategyFactory
	channelProvider         platformchannel.ChannelProvider // Platform official channels
	privateModels           llmmodelsvc.PrivateModelLookupService
	upstreamState           *upstreamstate.Service
}

type channelModelSource string

const (
	channelModelSourceGlobal      channelModelSource = "global"
	channelModelSourceCustom      channelModelSource = "custom"
	channelModelSourcePassthrough channelModelSource = "passthrough"
)

// ProviderSelection represents the selected provider configuration for API calls
// This is used by the billing and service layers
type ProviderSelection struct {
	OrganizationID    uuid.UUID
	Provider          providermodel.LLMProvider
	Model             llmmodel.LLMModel
	ModelSource       PricingModelSource
	BillingLane       UsageBillingLane
	UseSystemProvider bool

	// Channel configuration from the selected route
	RouteID                     uuid.UUID
	ChannelProvider             string
	APIKey                      string // Decrypted API key
	APIBaseURL                  string
	NativeProtocols             channelmodel.NativeProtocolConfig
	ModelMaps                   map[string]interface{}
	ParamOverride               map[string]interface{}
	HeaderOverride              map[string]interface{}
	Priority                    int
	Weight                      int
	AutoBan                     bool
	CredentialID                uuid.UUID
	CredentialGeneration        int64
	UpstreamWouldGuard          bool
	UpstreamHalfOpen            bool
	UpstreamProbe               bool
	UpstreamProbeRequiresBackup bool
	UpstreamProbeHasBackup      bool
}

// HasRoute returns true if this selection has a valid route
func (ps *ProviderSelection) HasRoute() bool {
	return ps.RouteID != uuid.Nil
}

// ChannelSelection represents the selected channel configuration for API calls
type ChannelSelection struct {
	OrganizationID              uuid.UUID
	RouteID                     uuid.UUID
	ModelName                   string
	ChannelProvider             string
	APIBaseURL                  string
	NativeProtocols             channelmodel.NativeProtocolConfig
	APIKey                      string // Decrypted API key
	ModelMaps                   map[string]interface{}
	ParamOverride               map[string]interface{}
	HeaderOverride              map[string]interface{}
	Model                       *llmmodel.LLMModel
	Priority                    int
	Weight                      int
	BillingLane                 UsageBillingLane
	UseSystemProvider           bool
	IsOfficial                  bool // True for official aggregated routes
	ModelSource                 channelModelSource
	ModelProviderID             uuid.UUID
	CredentialID                uuid.UUID
	CredentialGeneration        int64
	UpstreamWouldGuard          bool
	UpstreamHalfOpen            bool
	UpstreamProbe               bool
	UpstreamProbeRequiresBackup bool
}

// NewChannelRouter creates a new channel router using V2 architecture
func NewChannelRouter(db *gorm.DB, cryptoService shared.CryptoService, privateModels llmmodelsvc.PrivateModelLookupService) *ChannelRouter {
	return &ChannelRouter{
		db:                      db,
		organizationIDRouteRepo: channelrepo.NewTenantRouteRepository(db),
		organizationIDCredRepo:  credentialrepo.NewTenantCredentialRepository(db),
		cryptoService:           cryptoService,
		strategyFactory:         NewStrategyFactory(),
		privateModels:           privateModels,
		upstreamState:           upstreamstate.NewService(db, cryptoService),
	}
}

// SetConfigCache sets the optional config cache for performance optimization
func (r *ChannelRouter) SetConfigCache(cache *ConfigCache) {
	r.configCache = cache
}

// SetChannelProvider sets the platform channel provider for official channels
func (r *ChannelRouter) SetChannelProvider(provider platformchannel.ChannelProvider) {
	r.channelProvider = provider
}

// SelectChannel selects the best channel for a model request
func (r *ChannelRouter) SelectChannel(
	ctx context.Context,
	organizationID uuid.UUID,
	modelName string,
) (*ChannelSelection, error) {
	selections, err := r.SelectChannels(ctx, organizationID, modelName, 1)
	if err != nil {
		return nil, err
	}
	if len(selections) == 0 {
		return nil, ErrNoProviderAvailable
	}
	return selections[0], nil
}

// SelectChannels selects multiple channels for failover support
func (r *ChannelRouter) SelectChannels(
	ctx context.Context,
	organizationID uuid.UUID,
	modelName string,
	maxSelections int,
) ([]*ChannelSelection, error) {
	return r.SelectChannelsForProvider(ctx, organizationID, "", modelName, maxSelections)
}

func (r *ChannelRouter) SelectChannelsForProvider(
	ctx context.Context,
	organizationID uuid.UUID,
	providerHint string,
	modelName string,
	maxSelections int,
) ([]*ChannelSelection, error) {
	modelName = normalizeRequestedModelName(modelName)
	providerHint = strings.TrimSpace(providerHint)
	modelCategory, _ := ctx.Value(shared.ContextKeyModelCategory).(string)
	logCtx := logger.WithFields(ctx,
		zap.String("organization_id", organizationID.String()),
		zap.String("provider_hint", providerHint),
		zap.String("model", modelName),
		zap.Int("max_selections", maxSelections),
		zap.String("model_category", modelCategory),
	)
	logger.DebugContext(logCtx, "selecting LLM channels")

	llmModel, privateModel, err := r.resolveSelectionModel(ctx, organizationID, providerHint, modelName)
	isPrivateCustomModel := privateModel != nil
	isPassthroughMode := false
	modelProvider := ""
	if err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			logger.ErrorContext(logCtx, "failed to resolve LLM model", err)
			return nil, fmt.Errorf("failed to resolve LLM model %q: %w", modelName, err)
		}
		knownGlobalModel, knownErr := r.globalModelExists(ctx, providerHint, modelName)
		if knownErr != nil {
			logger.ErrorContext(logCtx, "failed to check LLM model lifecycle", knownErr)
			return nil, fmt.Errorf("failed to check LLM model %q: %w", modelName, knownErr)
		}
		if knownGlobalModel {
			logger.DebugContext(logCtx, "LLM model exists in global registry but is not active")
			return nil, llmerrors.NewModelNotFoundErrorWithName(modelName)
		}
		// Model not in local registries - enable passthrough mode
		logger.DebugContext(logCtx, "LLM model not found in local registries, using passthrough mode")
		isPassthroughMode = true
	} else {
		modelProvider = llmModel.Provider
	}

	logCtx = logger.WithFields(logCtx,
		zap.String("provider", modelProvider),
		zap.Bool("passthrough", isPassthroughMode),
	)
	logger.DebugContext(logCtx, "loading enabled LLM routes")
	routes, err := r.organizationIDRouteRepo.GetEnabledRoutes(ctx, organizationID)
	if err != nil {
		logger.ErrorContext(logCtx, "failed to load enabled LLM routes", err)
		return nil, fmt.Errorf("failed to get enabled routes: %w", err)
	}
	logger.DebugContext(logCtx, "enabled LLM routes loaded",
		zap.Int("route_count", len(routes)),
	)
	// Note: Official channels are now persisted in llm_routes (is_official=true)
	// via InitOfficialChannel, so they are included in GetEnabledRoutes above.
	// The previous cloud.go virtual route injection has been removed.

	if len(routes) == 0 {
		return nil, fmt.Errorf("no enabled routes found for organizationID %s. Please configure at least one active channel in your workspace", organizationID)
	}

	validRoutes, err := r.prepareCandidateRoutes(ctx, organizationID, routes, modelName, modelProvider, isPrivateCustomModel, llmModel, true, maxSelections > 1)
	if err != nil {
		return nil, err
	}

	logger.DebugContext(logCtx, "valid LLM routes filtered",
		zap.Int("valid_route_count", len(validRoutes)),
	)
	selectedRoutes := r.selectRoutesByPriorityAndWeight(validRoutes, maxSelections)

	for i, route := range selectedRoutes {
		logger.DebugContext(logCtx, "selected LLM route",
			zap.Int("selection_index", i),
			zap.String("route_id", route.ID.String()),
			zap.String("route_type", string(route.Type)),
			zap.Int("priority", route.Priority),
			zap.Int("weight", route.Weight),
		)
	}

	var selections []*ChannelSelection
	var buildErrors []string
	for _, route := range selectedRoutes {
		selection, err := r.buildChannelSelection(ctx, route, llmModel, privateModel, modelName, isPassthroughMode, modelCategory)
		if err != nil {
			errMsg := fmt.Sprintf("route %s (%s): %v", route.ID, route.Name, err)
			buildErrors = append(buildErrors, errMsg)
			logger.WarnContext(logCtx, "failed to build LLM channel selection",
				err,
				zap.String("route_id", route.ID.String()),
			)
			continue
		}
		selections = append(selections, selection)
	}

	logger.DebugContext(logCtx, "LLM channel selections built",
		zap.Int("selection_count", len(selections)),
	)

	if len(selections) == 0 {
		if len(buildErrors) > 0 {
			return nil, fmt.Errorf("failed to build channel selections for model '%s'. Errors: %v", modelName, buildErrors)
		}
		return nil, fmt.Errorf("no valid channel selections could be built for model '%s' (provider: %s)", modelName, modelProvider)
	}

	return selections, nil
}

func filterRoutesForNativeProtocol(routes []*channelmodel.LLMRoute, llmModel *llmmodel.LLMModel, modelCategory string) []*channelmodel.LLMRoute {
	switch modelCategory {
	case modelCategoryResponses:
		return filterRoutesByCapability(routes, func(route *channelmodel.LLMRoute) bool {
			if llmModel != nil && !llmModel.Responses {
				return false
			}
			return routeSupportsOpenAIResponses(route)
		})
	case modelCategoryAnthropicMessages:
		return filterRoutesByCapability(routes, func(route *channelmodel.LLMRoute) bool {
			return routeSupportsAnthropicMessages(route)
		})
	default:
		return routes
	}
}

func filterRoutesForNativeProtocolOrError(routes []*channelmodel.LLMRoute, llmModel *llmmodel.LLMModel, modelCategory string) ([]*channelmodel.LLMRoute, error) {
	filtered := filterRoutesForNativeProtocol(routes, llmModel, modelCategory)
	if modelCategory != "" && len(routes) > 0 && len(filtered) == 0 {
		modelName := ""
		if llmModel != nil {
			modelName = llmModel.Model
		}
		return nil, fmt.Errorf("%w: model %q does not support %s on any enabled route", adapter.ErrCapabilityUnsupported, modelName, modelCategory)
	}
	return filtered, nil
}

func filterRoutesByCapability(routes []*channelmodel.LLMRoute, supports func(*channelmodel.LLMRoute) bool) []*channelmodel.LLMRoute {
	filtered := make([]*channelmodel.LLMRoute, 0, len(routes))
	for _, route := range routes {
		if supports(route) {
			filtered = append(filtered, route)
		}
	}
	return filtered
}

func routeSupportsOpenAIResponses(route *channelmodel.LLMRoute) bool {
	if route == nil {
		return false
	}
	capability := channelprovider.OpenAIResponsesCapability(route.ChannelProvider)
	if !capability.Supported {
		return false
	}
	if capability.RequiresExplicitConfig {
		return route.NativeProtocols.OpenAIResponses.Enabled
	}
	return true
}

func routeSupportsAnthropicMessages(route *channelmodel.LLMRoute) bool {
	if route == nil {
		return false
	}
	capability := channelprovider.AnthropicMessagesCapability(route.ChannelProvider)
	if !capability.Supported {
		return false
	}
	if capability.RequiresExplicitConfig {
		return route.NativeProtocols.AnthropicMessages.Enabled
	}
	return true
}

func (r *ChannelRouter) resolveSelectionModel(ctx context.Context, organizationID uuid.UUID, providerHint string, modelName string) (*llmmodel.LLMModel, *llmmodel.CustomModel, error) {
	if providerHint != "" {
		if privateModel, err := r.getPrivateModelForProvider(ctx, organizationID, providerHint, modelName); err != nil {
			return nil, nil, err
		} else if privateModel != nil {
			return llmModelFromPrivateModel(privateModel), privateModel, nil
		}

		llmModel, err := r.getModelForProvider(ctx, providerHint, modelName)
		if err != nil {
			return nil, nil, err
		}
		return llmModel, nil, nil
	}

	if privateModel, err := r.getPrivateModel(ctx, organizationID, modelName); err != nil {
		return nil, nil, err
	} else if privateModel != nil {
		return llmModelFromPrivateModel(privateModel), privateModel, nil
	}

	llmModel, err := r.getModel(ctx, modelName)
	if err != nil {
		return nil, nil, err
	}

	return llmModel, nil, nil
}

func (r *ChannelRouter) getPrivateModel(ctx context.Context, organizationID uuid.UUID, modelName string) (*llmmodel.CustomModel, error) {
	if r == nil || r.privateModels == nil || organizationID == uuid.Nil {
		return nil, nil
	}

	return r.privateModels.ResolveActiveModel(ctx, organizationID, modelName)
}

func (r *ChannelRouter) getPrivateModelForProvider(ctx context.Context, organizationID uuid.UUID, provider string, modelName string) (*llmmodel.CustomModel, error) {
	if r == nil || r.privateModels == nil || organizationID == uuid.Nil {
		return nil, nil
	}

	return r.privateModels.ResolveActiveModelForProvider(ctx, organizationID, provider, modelName)
}

func llmModelFromPrivateModel(customModel *llmmodel.CustomModel) *llmmodel.LLMModel {
	if customModel == nil {
		return nil
	}

	return &llmmodel.LLMModel{
		ID:                customModel.ID,
		Provider:          customModel.Provider,
		Model:             customModel.Name,
		ModelName:         customModel.DisplayName,
		UseCases:          customModel.UseCases,
		SupportsReasoning: customModel.SupportsReasoning,
		SupportsToolCall:  customModel.SupportsToolCall,
		SupportsVision:    customModel.SupportsVision,
		SupportsStreaming: customModel.SupportsStreaming,
		ChatCompletions:   customModel.ChatCompletions,
		Embeddings:        customModel.Embeddings,
		ImageGeneration:   customModel.ImageGeneration,
		Responses:         customModel.Responses,
		ContextWindow:     customModel.ContextWindow,
		MaxOutputTokens:   customModel.MaxOutputTokens,
		InputPrice:        customModel.InputPrice,
		OutputPrice:       customModel.OutputPrice,
		IsActive:          customModel.IsActive,
	}
}

// getModel retrieves model by name, using cache if available
func (r *ChannelRouter) getModel(ctx context.Context, modelName string) (*llmmodel.LLMModel, error) {
	if r.configCache != nil {
		return r.configCache.GetModelByName(ctx, modelName)
	}

	return r.queryActiveModel(ctx, "", modelName)
}

func (r *ChannelRouter) getModelForProvider(ctx context.Context, provider, modelName string) (*llmmodel.LLMModel, error) {
	return r.queryActiveModel(ctx, provider, modelName)
}

func (r *ChannelRouter) queryActiveModel(ctx context.Context, provider, modelName string) (*llmmodel.LLMModel, error) {
	var m llmmodel.LLMModel
	query := r.db.WithContext(ctx).
		Model(&llmmodel.LLMModel{}).
		Joins("JOIN llm_providers ON llm_models.provider = llm_providers.provider")
	if provider != "" {
		query = query.Where("llm_models.provider = ?", provider)
	}
	err := query.
		Where("llm_models.name = ? AND llm_models.is_active = ? AND llm_models.status = ? AND llm_models.deleted_at IS NULL", modelName, true, llmmodel.ModelStatusActive).
		Where("llm_providers.is_active = ? AND llm_providers.deleted_at IS NULL", true).
		First(&m).Error
	if err != nil {
		return nil, err
	}
	return &m, nil
}

func (r *ChannelRouter) globalModelExists(ctx context.Context, provider, modelName string) (bool, error) {
	if r == nil || r.db == nil {
		return false, nil
	}

	var count int64
	query := r.db.WithContext(ctx).Unscoped().Model(&llmmodel.LLMModel{})
	if provider != "" {
		query = query.Where("provider = ?", provider)
	}
	if err := query.Where("name = ?", modelName).Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

// filterRoutesForModel filters routes that support the given model
// Uses Strategy Pattern to delegate model matching logic to route-specific strategies
// This eliminates hard-coded type checking and follows Open-Closed Principle
func (r *ChannelRouter) filterRoutesForModel(routes []*channelmodel.LLMRoute, modelName, modelProvider string) []*channelmodel.LLMRoute {
	var validRoutes []*channelmodel.LLMRoute

	for _, route := range routes {
		// Delegate to appropriate strategy based on route type
		strategy := r.strategyFactory.GetStrategy(route)

		if strategy.SupportsModel(route, modelName, modelProvider) {
			logger.Debug("LLM route supports model",
				zap.String("route_id", route.ID.String()),
				zap.String("model", modelName),
				zap.String("provider", modelProvider),
				zap.String("strategy", strategy.GetStrategyName()),
			)
			validRoutes = append(validRoutes, route)
		} else {
			logger.Debug("LLM route does not support model",
				zap.String("route_id", route.ID.String()),
				zap.String("model", modelName),
				zap.String("provider", modelProvider),
				zap.String("strategy", strategy.GetStrategyName()),
			)
		}
	}

	return validRoutes
}

func (r *ChannelRouter) filterRoutesForSelection(routes []*channelmodel.LLMRoute, modelName, modelProvider string, isPrivateCustomModel bool) []*channelmodel.LLMRoute {
	if !isPrivateCustomModel {
		return r.filterRoutesForModel(routes, modelName, modelProvider)
	}

	validRoutes := make([]*channelmodel.LLMRoute, 0)
	for _, route := range routes {
		if isOfficialRoute(route) {
			continue
		}

		strategy := r.strategyFactory.GetStrategy(route)
		if strategy.SupportsModel(route, modelName, "") {
			validRoutes = append(validRoutes, route)
		}
	}

	return validRoutes
}

func (r *ChannelRouter) prepareCandidateRoutes(
	ctx context.Context,
	organizationID uuid.UUID,
	routes []*channelmodel.LLMRoute,
	modelName, modelProvider string,
	isPrivateCustomModel bool,
	llmModel *llmmodel.LLMModel,
	allowHalfOpen bool,
	allowAutomaticHalfOpen bool,
) ([]*channelmodel.LLMRoute, error) {
	validRoutes := r.filterRoutesForSelection(routes, modelName, modelProvider, isPrivateCustomModel)
	modelUseCase, _ := ctx.Value(shared.ContextKeyModelUseCase).(string)
	validRoutes = filterRoutesForModelScene(validRoutes, modelName, llmModel, modelUseCase, isPrivateCustomModel)
	modelCategory, _ := ctx.Value(shared.ContextKeyModelCategory).(string)
	validRoutes, err := filterRoutesForNativeProtocolOrError(validRoutes, llmModel, modelCategory)
	if err != nil {
		return nil, err
	}
	if len(validRoutes) == 0 {
		return nil, llmerrors.NewModelNotFoundErrorWithName(modelName)
	}

	eligibleRoutes, guardedCount := r.filterRoutesByUpstreamGuard(ctx, organizationID, validRoutes, allowHalfOpen, allowAutomaticHalfOpen)
	if len(eligibleRoutes) == 0 && guardedCount > 0 {
		upstreamstate.RecordNoCandidateMetric(ctx, "private_only")
		return nil, fmt.Errorf("%w", llmerrors.DomainErrPrivateChannelUpstreamUnavailable)
	}
	return eligibleRoutes, nil
}

func filterRoutesForModelScene(routes []*channelmodel.LLMRoute, modelName string, llmModel *llmmodel.LLMModel, useCase string, isPrivateCustomModel bool) []*channelmodel.LLMRoute {
	useCase = strings.TrimSpace(useCase)
	if useCase != string(llmmodel.UseCaseAgent) {
		return routes
	}
	trustCustomAgentLabel := isPrivateCustomModel && modelHasUseCase(llmModel, useCase)

	filtered := make([]*channelmodel.LLMRoute, 0, len(routes))
	for _, route := range routes {
		if route == nil {
			continue
		}
		if !trustCustomAgentLabel {
			channelProvider := route.ChannelProvider
			if isOfficialRoute(route) {
				channelProvider = "zgi-cloud"
			}
			if !channelprovider.SupportsAgentProtocol(channelProvider) {
				continue
			}
		}
		filtered = append(filtered, route)
	}
	return filtered
}

func modelHasUseCase(modelRecord *llmmodel.LLMModel, useCase string) bool {
	if modelRecord == nil {
		return false
	}
	for _, candidate := range modelRecord.UseCases {
		if candidate == useCase {
			return true
		}
	}
	return false
}

func (r *ChannelRouter) filterRoutesByUpstreamGuard(
	ctx context.Context,
	organizationID uuid.UUID,
	routes []*channelmodel.LLMRoute,
	allowHalfOpen bool,
	allowAutomaticHalfOpen bool,
) ([]*channelmodel.LLMRoute, int) {
	if r.upstreamState == nil || len(routes) == 0 {
		return routes, 0
	}
	credentialIDs := make([]uuid.UUID, 0, len(routes))
	seen := make(map[uuid.UUID]struct{}, len(routes))
	for _, route := range routes {
		if route == nil || isOfficialRoute(route) || route.CredentialID == nil {
			continue
		}
		if _, exists := seen[*route.CredentialID]; exists {
			continue
		}
		seen[*route.CredentialID] = struct{}{}
		credentialIDs = append(credentialIDs, *route.CredentialID)
	}
	states, err := r.upstreamState.GetMany(ctx, organizationID, credentialIDs)
	if err != nil {
		logger.WarnContext(ctx, "failed to load upstream credential guard state; allowing routes", err)
		return routes, 0
	}

	cfg := appconfig.Current().LLM
	mode := strings.ToLower(strings.TrimSpace(cfg.UpstreamGuardMode))
	enforce := mode == "enforce" && organizationInUpstreamGuardRollout(organizationID, cfg.UpstreamGuardPercentage)
	eligible := make([]*channelmodel.LLMRoute, 0, len(routes))
	guardedCount := 0
	for _, route := range routes {
		if route == nil {
			continue
		}
		route.UpstreamGeneration = 0
		route.UpstreamWouldGuard = false
		route.UpstreamHalfOpen = false
		route.UpstreamProbe = false
		route.UpstreamProbeRequiresBackup = false
		if isOfficialRoute(route) || route.CredentialID == nil {
			eligible = append(eligible, route)
			continue
		}
		state := states[*route.CredentialID]
		if state == nil {
			eligible = append(eligible, route)
			continue
		}
		// The state generation is read after routes are loaded. Reload the
		// credential later so the key cannot come from an older generation.
		route.TenantCredential = nil
		route.UpstreamGeneration = state.Generation
		if mode == "off" {
			route.UpstreamWouldGuard = state.BlockReason != ""
			eligible = append(eligible, route)
			continue
		}
		decision := r.upstreamState.EvaluateGuardReadOnly(state, enforce)
		if allowHalfOpen && decision.Block {
			hasPotentialBackup := allowAutomaticHalfOpen && r.hasEligibleBackupRoute(routes, states, route, enforce)
			probeEligible, requiresBackup := r.upstreamState.EvaluateProbeEligibility(state, enforce, hasPotentialBackup)
			if probeEligible {
				route.UpstreamProbe = true
				route.UpstreamProbeRequiresBackup = requiresBackup
				decision.Block = false
			}
		}
		route.UpstreamWouldGuard = decision.WouldGuard
		if decision.WouldGuard {
			upstreamstate.RecordWouldGuardMetric(ctx, route.ChannelProvider, decision.Reason)
		}
		if decision.Block {
			upstreamstate.RecordGuardMetric(ctx, route.ChannelProvider, decision.Reason)
			guardedCount++
			continue
		}
		eligible = append(eligible, route)
	}
	return eligible, guardedCount
}

func (r *ChannelRouter) hasEligibleBackupRoute(
	routes []*channelmodel.LLMRoute,
	states map[uuid.UUID]*upstreamstate.State,
	current *channelmodel.LLMRoute,
	enforce bool,
) bool {
	for _, candidate := range routes {
		if candidate == nil || candidate == current {
			continue
		}
		if isOfficialRoute(candidate) {
			return true
		}
		if candidate.CredentialID == nil {
			continue
		}
		if current != nil && current.CredentialID != nil && *candidate.CredentialID == *current.CredentialID {
			continue
		}
		state := states[*candidate.CredentialID]
		if state == nil || !r.upstreamState.EvaluateGuardReadOnly(state, enforce).Block {
			return true
		}
	}
	return false
}

func organizationInUpstreamGuardRollout(organizationID uuid.UUID, percentage int) bool {
	if percentage <= 0 {
		return false
	}
	if percentage >= 100 {
		return true
	}
	sum := sha256.Sum256(organizationID[:])
	bucket := int(binary.BigEndian.Uint16(sum[:2]) % 100)
	return bucket < percentage
}

// selectRoutesByPriorityAndWeight selects routes based on priority and weight
func (r *ChannelRouter) selectRoutesByPriorityAndWeight(routes []*channelmodel.LLMRoute, maxSelections int) []*channelmodel.LLMRoute {
	if len(routes) == 0 || maxSelections <= 0 {
		return nil
	}

	probeRoutes := make([]*channelmodel.LLMRoute, 0, len(routes))
	regularRoutes := make([]*channelmodel.LLMRoute, 0, len(routes))
	for _, route := range routes {
		if route.UpstreamProbe {
			probeRoutes = append(probeRoutes, route)
			continue
		}
		regularRoutes = append(regularRoutes, route)
	}
	if len(probeRoutes) > 0 {
		probe := r.selectRoutesByPriorityAndWeightWithoutProbe(probeRoutes, 1)
		fallbacks := r.selectRoutesByPriorityAndWeightWithoutProbe(regularRoutes, maxSelections-1)
		return append(probe, fallbacks...)
	}
	return r.selectRoutesByPriorityAndWeightWithoutProbe(regularRoutes, maxSelections)
}

func (r *ChannelRouter) selectRoutesByPriorityAndWeightWithoutProbe(routes []*channelmodel.LLMRoute, maxSelections int) []*channelmodel.LLMRoute {
	if len(routes) == 0 || maxSelections <= 0 {
		return nil
	}

	// Sort by priority descending
	sort.Slice(routes, func(i, j int) bool {
		return routes[i].Priority > routes[j].Priority
	})

	// Group by priority
	priorityGroups := make(map[int][]*channelmodel.LLMRoute)
	var priorities []int
	for _, route := range routes {
		if _, ok := priorityGroups[route.Priority]; !ok {
			priorities = append(priorities, route.Priority)
		}
		priorityGroups[route.Priority] = append(priorityGroups[route.Priority], route)
	}

	// Sort priorities descending
	sort.Sort(sort.Reverse(sort.IntSlice(priorities)))

	var selected []*channelmodel.LLMRoute
	for _, p := range priorities {
		if len(selected) >= maxSelections {
			break
		}

		samePriorityRoutes := priorityGroups[p]
		remaining := maxSelections - len(selected)
		selectedFromGroup := r.selectWeightedRoutes(samePriorityRoutes, remaining)
		selected = append(selected, selectedFromGroup...)
	}

	return selected
}

// selectWeightedRoutes selects routes using weighted random selection
// Even when selecting all routes, the order is randomized by weight
// so that the first route (primary) varies across requests for load balancing
func (r *ChannelRouter) selectWeightedRoutes(routes []*channelmodel.LLMRoute, count int) []*channelmodel.LLMRoute {
	if len(routes) <= 1 {
		return routes
	}

	selectCount := count
	if selectCount > len(routes) {
		selectCount = len(routes)
	}

	var selected []*channelmodel.LLMRoute
	remaining := make([]*channelmodel.LLMRoute, len(routes))
	copy(remaining, routes)

	for i := 0; i < selectCount && len(remaining) > 0; i++ {
		route := r.weightedRandomSelect(remaining)
		selected = append(selected, route)

		// Remove selected from remaining
		for j, rem := range remaining {
			if rem.ID == route.ID {
				remaining = append(remaining[:j], remaining[j+1:]...)
				break
			}
		}
	}

	return selected
}

// weightedRandomSelect performs weighted random selection
func (r *ChannelRouter) weightedRandomSelect(routes []*channelmodel.LLMRoute) *channelmodel.LLMRoute {
	totalWeight := 0
	for _, route := range routes {
		totalWeight += route.Weight
	}

	if totalWeight == 0 {
		return routes[0]
	}

	n := rand.Intn(totalWeight)
	for _, route := range routes {
		n -= route.Weight
		if n < 0 {
			return route
		}
	}

	return routes[0]
}

// buildChannelSelection builds a ChannelSelection from a TenantRoute
func (r *ChannelRouter) buildChannelSelection(
	ctx context.Context,
	route *channelmodel.LLMRoute,
	llmModel *llmmodel.LLMModel,
	privateModel *llmmodel.CustomModel,
	modelName string,
	isPassthroughMode bool,
	modelCategory string,
) (*ChannelSelection, error) {
	// Defensive check: verify route actually supports this model.
	// The route's model list is the capability source of truth.
	models := route.GetEffectiveModels()

	// Verify model is in supported list (unless wildcard)
	// In passthrough mode, use modelName directly
	targetModelName := modelName
	if !isPassthroughMode && llmModel != nil {
		targetModelName = llmModel.Model
	}

	isOfficial := route.IsOfficial || route.Type == shared.RouteTypeZGICloud
	modelProvider := ""
	if llmModel != nil {
		modelProvider = llmModel.Provider
	}
	if !route.SupportsModelForProvider(modelProvider, targetModelName) {
		return nil, fmt.Errorf("model %s not supported by route %s (supported: %v)", targetModelName, route.ID, models)
	}

	billingLane := UsageBillingLanePrivate
	if isOfficial {
		billingLane = UsageBillingLanePlatform
	}
	modelSource, modelProviderID := resolveChannelModelSource(privateModel, isPassthroughMode)
	selection := &ChannelSelection{
		OrganizationID:              route.OrganizationID,
		RouteID:                     route.ID,
		Model:                       llmModel,
		ModelName:                   targetModelName,
		Priority:                    route.Priority,
		Weight:                      route.Weight,
		BillingLane:                 billingLane,
		UseSystemProvider:           usageBillingLaneUsesSystemProvider(billingLane),
		IsOfficial:                  isOfficial,
		ModelSource:                 modelSource,
		ModelProviderID:             modelProviderID,
		CredentialGeneration:        route.UpstreamGeneration,
		UpstreamWouldGuard:          route.UpstreamWouldGuard,
		UpstreamHalfOpen:            route.UpstreamHalfOpen,
		UpstreamProbe:               route.UpstreamProbe,
		UpstreamProbeRequiresBackup: route.UpstreamProbeRequiresBackup,
	}
	if route.CredentialID != nil {
		selection.CredentialID = *route.CredentialID
	}

	selection.ChannelProvider = route.ChannelProvider
	selection.APIBaseURL = route.APIBaseURL
	selection.NativeProtocols = route.NativeProtocols
	selection.ModelMaps = route.ModelMaps
	selection.ParamOverride = route.ParamOverride
	selection.HeaderOverride = route.HeaderOverride
	if err := applyNativeProtocolBaseURL(selection, modelCategory); err != nil {
		return nil, err
	}

	if isOfficial {
		apiBaseURL, err := resolveOfficialRouteBaseURL()
		if err != nil {
			return nil, err
		}

		// Official channels go through standard adapter with APIBaseURL pointing to console-api.
		// console-api returns OpenAI-compatible responses (protocol conversion done internally).
		// Official channels use HMAC auth (injected via AuthHook), no Bearer API key
		selection.APIBaseURL = apiBaseURL
		selection.APIKey = ""
		return selection, nil
	}

	if route.IsUserChannel() {
		if err := r.populatePrivateRouteSelection(ctx, route, selection); err != nil {
			return nil, err
		}
		if err := applyNativeProtocolBaseURL(selection, modelCategory); err != nil {
			return nil, err
		}
	}

	return selection, nil
}

func resolveChannelModelSource(privateModel *llmmodel.CustomModel, isPassthroughMode bool) (channelModelSource, uuid.UUID) {
	if isPassthroughMode {
		return channelModelSourcePassthrough, uuid.Nil
	}
	if privateModel != nil {
		return channelModelSourceCustom, privateModel.ProviderID
	}
	return channelModelSourceGlobal, uuid.Nil
}

func (r *ChannelRouter) populatePrivateRouteSelection(ctx context.Context, route *channelmodel.LLMRoute, selection *ChannelSelection) error {
	cred, err := r.loadRouteCredential(ctx, route)
	if err != nil {
		return err
	}
	if cred == nil {
		logger.WarnContext(ctx, "private LLM route has no credential",
			zap.String("route_id", route.ID.String()),
		)
		return nil
	}

	if selection.CredentialGeneration > 0 {
		selection.ChannelProvider = cred.ChannelProvider
		selection.APIBaseURL = cred.APIBaseURL
	} else if selection.ChannelProvider == "" && cred.ChannelProvider != "" {
		selection.ChannelProvider = cred.ChannelProvider
	}

	apiKey, err := r.cryptoService.Decrypt(cred.APIKeyCiphertext)
	if err != nil {
		return fmt.Errorf("failed to decrypt organizationID credential: %w", err)
	}
	selection.APIKey = apiKey

	if selection.CredentialGeneration == 0 && selection.APIBaseURL == "" && cred.APIBaseURL != "" {
		selection.APIBaseURL = cred.APIBaseURL
	}

	logger.DebugContext(ctx, "private LLM route selection finalized",
		zap.String("route_id", route.ID.String()),
		zap.Bool("has_api_base_url", selection.APIBaseURL != ""),
	)
	return nil
}

func applyNativeProtocolBaseURL(selection *ChannelSelection, modelCategory string) error {
	if selection == nil || modelCategory == "" {
		return nil
	}

	var endpoint channelmodel.NativeProtocolEndpoint
	switch modelCategory {
	case modelCategoryResponses:
		endpoint = selection.NativeProtocols.OpenAIResponses
	case modelCategoryAnthropicMessages:
		endpoint = selection.NativeProtocols.AnthropicMessages
	default:
		return nil
	}

	if !endpoint.Enabled {
		return nil
	}
	baseURL := strings.TrimSpace(endpoint.BaseURL)
	if baseURL == "" {
		return nil
	}
	selection.APIBaseURL = strings.TrimRight(baseURL, "/")
	return nil
}

func (r *ChannelRouter) loadRouteCredential(ctx context.Context, route *channelmodel.LLMRoute) (*credentialmodel.TenantCredential, error) {
	if route.TenantCredential != nil {
		logger.DebugContext(ctx, "tenant credential already loaded for private LLM route",
			zap.String("route_id", route.ID.String()),
			zap.Bool("has_api_base_url", route.TenantCredential.APIBaseURL != ""),
		)
		return route.TenantCredential, nil
	}

	if route.CredentialID == nil {
		return nil, nil
	}

	logger.DebugContext(ctx, "loading tenant credential for private LLM route",
		zap.String("route_id", route.ID.String()),
		zap.String("credential_id", route.CredentialID.String()),
	)
	cred, err := r.organizationIDCredRepo.GetByID(ctx, route.OrganizationID, *route.CredentialID)
	if err != nil {
		return nil, fmt.Errorf("failed to get organizationID credential: %w", err)
	}
	logger.DebugContext(ctx, "tenant credential loaded for private LLM route",
		zap.String("route_id", route.ID.String()),
		zap.Bool("has_api_base_url", cred.APIBaseURL != ""),
	)
	return cred, nil
}

// ConvertToProviderSelection converts a ChannelSelection to legacy ProviderSelection for compatibility
func (cs *ChannelSelection) ConvertToProviderSelection(ctx context.Context, db *gorm.DB) (*ProviderSelection, error) {
	return cs.ConvertToProviderSelectionWithCache(ctx, db, nil)
}

// ConvertToProviderSelectionWithCache converts with optional cache support
func (cs *ChannelSelection) ConvertToProviderSelectionWithCache(ctx context.Context, db *gorm.DB, cache *ConfigCache) (*ProviderSelection, error) {
	if cs.ModelSource == channelModelSourceCustom {
		customProvider, err := loadCustomProvider(ctx, db, cs.OrganizationID, cs.ModelProviderID)
		if err != nil {
			return nil, err
		}
		return &ProviderSelection{
			OrganizationID:              cs.OrganizationID,
			Provider:                    *customProvider,
			Model:                       modelForSelection(cs),
			ModelSource:                 pricingModelSourceFromChannelModelSource(cs.ModelSource),
			BillingLane:                 cs.BillingLane,
			UseSystemProvider:           cs.UseSystemProvider,
			RouteID:                     cs.RouteID,
			ChannelProvider:             cs.ChannelProvider,
			APIKey:                      cs.APIKey,
			APIBaseURL:                  cs.APIBaseURL,
			NativeProtocols:             cs.NativeProtocols,
			ModelMaps:                   cs.ModelMaps,
			ParamOverride:               cs.ParamOverride,
			HeaderOverride:              cs.HeaderOverride,
			Priority:                    cs.Priority,
			Weight:                      cs.Weight,
			CredentialID:                cs.CredentialID,
			CredentialGeneration:        cs.CredentialGeneration,
			UpstreamWouldGuard:          cs.UpstreamWouldGuard,
			UpstreamHalfOpen:            cs.UpstreamHalfOpen,
			UpstreamProbe:               cs.UpstreamProbe,
			UpstreamProbeRequiresBackup: cs.UpstreamProbeRequiresBackup,
		}, nil
	}

	providerName := cs.ChannelProvider
	if cs.IsOfficial && cs.Model != nil && cs.Model.Provider != "" {
		providerName = cs.Model.Provider
	} else if lookupProvider, err := channelprovider.LookupProvider(cs.ChannelProvider); err == nil {
		providerName = lookupProvider
	} else if cs.Model != nil && cs.Model.Provider != "" {
		providerName = cs.Model.Provider
	}

	// Load the provider details
	var provider *providermodel.LLMProvider
	var err error

	if cache != nil {
		provider, err = cache.GetProviderByName(ctx, providerName)
	} else {
		var p providermodel.LLMProvider
		err = db.WithContext(ctx).
			Where("provider = ? AND is_active = ? AND deleted_at IS NULL", providerName, true).
			First(&p).Error
		if err == nil {
			provider = &p
		}
	}

	// Check if provider was found
	if err != nil && cs.IsOfficial {
		provider = buildPassthroughProvider(providerName)
		err = nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to load provider %s: %w", providerName, err)
	}
	if provider == nil && cs.IsOfficial {
		provider = buildPassthroughProvider(providerName)
	}
	if provider == nil {
		return nil, fmt.Errorf("provider %s not found or inactive", providerName)
	}

	return &ProviderSelection{
		OrganizationID:              cs.OrganizationID,
		Provider:                    *provider,
		Model:                       modelForSelection(cs),
		ModelSource:                 pricingModelSourceFromChannelModelSource(cs.ModelSource),
		BillingLane:                 cs.BillingLane,
		UseSystemProvider:           cs.UseSystemProvider,
		RouteID:                     cs.RouteID,
		ChannelProvider:             cs.ChannelProvider,
		APIKey:                      cs.APIKey,
		APIBaseURL:                  cs.APIBaseURL,
		NativeProtocols:             cs.NativeProtocols,
		ModelMaps:                   cs.ModelMaps,
		ParamOverride:               cs.ParamOverride,
		HeaderOverride:              cs.HeaderOverride,
		Priority:                    cs.Priority,
		Weight:                      cs.Weight,
		CredentialID:                cs.CredentialID,
		CredentialGeneration:        cs.CredentialGeneration,
		UpstreamWouldGuard:          cs.UpstreamWouldGuard,
		UpstreamHalfOpen:            cs.UpstreamHalfOpen,
		UpstreamProbe:               cs.UpstreamProbe,
		UpstreamProbeRequiresBackup: cs.UpstreamProbeRequiresBackup,
	}, nil
}

func pricingModelSourceFromChannelModelSource(source channelModelSource) PricingModelSource {
	switch source {
	case channelModelSourceCustom:
		return PricingModelSourceCustom
	case channelModelSourcePassthrough:
		return PricingModelSourcePassthrough
	default:
		return PricingModelSourceGlobal
	}
}

func modelForSelection(cs *ChannelSelection) llmmodel.LLMModel {
	if cs != nil && cs.Model != nil {
		return *cs.Model
	}
	if cs == nil {
		return llmmodel.LLMModel{}
	}
	return llmmodel.LLMModel{
		ID:        uuid.Nil,
		Model:     cs.ModelName,
		ModelName: cs.ModelName,
		Provider:  cs.ChannelProvider,
		IsActive:  true,
	}
}

func loadCustomProvider(ctx context.Context, db *gorm.DB, organizationID, providerID uuid.UUID) (*providermodel.LLMProvider, error) {
	if providerID == uuid.Nil {
		return nil, fmt.Errorf("custom model provider_id is required")
	}

	var customProvider providermodel.CustomProvider
	err := db.WithContext(ctx).
		Where("id = ? AND organization_id = ? AND is_active = ? AND deleted_at IS NULL", providerID, organizationID, true).
		First(&customProvider).Error
	if err != nil {
		return nil, fmt.Errorf("failed to load custom provider %s: %w", providerID, err)
	}

	return &providermodel.LLMProvider{
		ID:           customProvider.ID,
		Object:       "provider",
		Provider:     customProvider.Provider,
		ProviderName: customProvider.ProviderName,
		APIBaseURL:   customProvider.APIBaseURL,
		IsActive:     customProvider.IsActive,
	}, nil
}

func buildPassthroughProvider(providerName string) *providermodel.LLMProvider {
	return &providermodel.LLMProvider{
		ID:           uuid.Nil,
		Object:       "provider",
		Provider:     providerName,
		ProviderName: providerName,
		IsActive:     true,
	}
}
