package service

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/shopspring/decimal"
	appconfig "github.com/zgiai/zgi/api/config"
	consoleintf "github.com/zgiai/zgi/api/internal/infra/platform/console"
	"github.com/zgiai/zgi/api/internal/modules/llm/channel/dto"
	"github.com/zgiai/zgi/api/internal/modules/llm/channel/model"
	"github.com/zgiai/zgi/api/internal/modules/llm/channel/repository"
	"github.com/zgiai/zgi/api/internal/modules/llm/channelprovider"
	credentialdto "github.com/zgiai/zgi/api/internal/modules/llm/credential/dto"
	credentialsvc "github.com/zgiai/zgi/api/internal/modules/llm/credential/service"
	llmmodelmodel "github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/model"
	llmmodelrepo "github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/repository"
	llmmodelsvc "github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/service"
	officialmodel "github.com/zgiai/zgi/api/internal/modules/llm/officialmodel"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	providerrepo "github.com/zgiai/zgi/api/internal/modules/llm/provider/repository"
	"github.com/zgiai/zgi/api/internal/modules/llm/shared"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	"gorm.io/gorm"
)

var (
	ErrChannelNotFound            = errors.New("channel not found")
	ErrRouteNotFound              = errors.New("route not found")
	ErrCredentialNotFound         = errors.New("credential not found")
	ErrInvalidRouteType           = errors.New("invalid route type")
	ErrNoAvailableOfficialChannel = errors.New("no available official channels for this organization")
)

// ============================================================================
// Channel Service - manages system channels and tenant routes
// ============================================================================

// ChannelService defines the interface for channel operations
// Note: System channel management has been moved to console-api
type ChannelService interface {
	// Tenant route operations
	CreateRoute(ctx context.Context, organizationID uuid.UUID, req *dto.CreateRouteRequest) (*dto.ChannelView, error)
	GetRoute(ctx context.Context, organizationID, id uuid.UUID) (*model.LLMRoute, error)
	ListRoutes(ctx context.Context, organizationID uuid.UUID, req *dto.ListRouteRequest) ([]*dto.ChannelView, int64, error)
	ListRoutesAggregated(ctx context.Context, organizationID uuid.UUID, req *dto.ListRoutesAggregatedRequest) (*dto.ChannelListResponse, error)
	UpdateRoute(ctx context.Context, organizationID, id uuid.UUID, req *dto.UpdateRouteRequest) (*dto.ChannelView, error)
	DeleteRoute(ctx context.Context, organizationID, id uuid.UUID) error
	TestRoute(ctx context.Context, organizationID, id uuid.UUID, model string) (*dto.TestChannelResult, error)

	// Route selection for API calls
	SelectRoute(ctx context.Context, organizationID uuid.UUID, modelName string) (*model.RouteQueryResult, error)
	GetRoutesForModel(ctx context.Context, organizationID uuid.UUID, modelName string) ([]*model.RouteQueryResult, error)

	// Platform channel (ZGI Cloud official, Cloud mode only)
	GetPlatformChannel(ctx context.Context, organizationID uuid.UUID) (*dto.PlatformChannelAggregatedView, error)
	UpdatePlatformChannel(ctx context.Context, channelID string, req *dto.UpdatePlatformChannelRequest) error
	UpdatePlatformChannelSettings(ctx context.Context, organizationID uuid.UUID, req *dto.UpdatePlatformChannelRequest) error

	// Tenant initialization
	InitOfficialChannel(ctx context.Context, organizationID uuid.UUID) error

	// Official channel settings (per-org priority/weight)
	UpdateOfficialChannelSettings(ctx context.Context, organizationID uuid.UUID, req *dto.UpdateOfficialChannelSettingsRequest) (int64, error)

	// Channel operations
	UpdateChannelBalance(ctx context.Context, id uuid.UUID, organizationID uuid.UUID) (*dto.UpdateChannelBalanceResponse, error)
	AdjustChannelWallet(ctx context.Context, organizationID, channelID uuid.UUID, req *dto.AdjustChannelWalletRequest) (*dto.AdjustChannelWalletResponse, error)

	// Advanced testing
	DiscoverDraftChannelModels(ctx context.Context, req *dto.DiscoverDraftChannelModelsRequest) (*dto.DiscoverDraftChannelModelsResponse, error)
	DiscoverOllamaModels(ctx context.Context, req *dto.DiscoverOllamaModelsRequest) (*dto.DiscoverOllamaModelsResponse, error)
	TestDraftChannelModel(ctx context.Context, organizationID uuid.UUID, req *dto.DraftTestChannelModelRequest) (*dto.ChannelModelTestResult, error)
	TestChannelModel(ctx context.Context, channelID uuid.UUID, organizationID uuid.UUID, model string, testMethod string) (*dto.ChannelModelTestResult, error)
	BatchTestChannelModels(ctx context.Context, channelID uuid.UUID, organizationID uuid.UUID, models []string, testMethod string, resultChan chan<- *dto.BatchTestChannelModelsStreamResponse)

	// Batch operations
	BatchToggleRoutes(ctx context.Context, organizationID uuid.UUID, req *dto.BatchToggleRoutesRequest) (*dto.BatchOperationResult, error)
	BatchDeleteRoutes(ctx context.Context, organizationID uuid.UUID, req *dto.BatchDeleteRoutesRequest) (*dto.BatchOperationResult, error)

	// Utility operations
	GetAvailableProviders(ctx context.Context, organizationID uuid.UUID) ([]string, error)
}

type ChannelValidator interface {
	ValidateModelsForCreation(ctx context.Context, organizationID uuid.UUID, channelProvider, apiKey, apiBaseURL string, models []string) (*channelprovider.ValidationResult, error)
	ValidateModels(ctx context.Context, organizationID uuid.UUID, channelProvider, apiKey, apiBaseURL string, models []string) (*channelprovider.ValidationResult, error)
	TestModel(ctx context.Context, organizationID uuid.UUID, channelProvider, apiKey, apiBaseURL, modelName, testMethod string) (*channelprovider.TestResult, error)
}

type channelService struct {
	tenantRouteRepo    repository.TenantRouteRepository
	tenantCredService  credentialsvc.TenantCredentialService
	validator          ChannelValidator
	modelRepo          llmmodelrepo.ModelRepository
	modelConfigRepo    llmmodelrepo.ModelConfigRepository
	customProviderRepo providerrepo.CustomProviderRepository
	customModelRepo    llmmodelrepo.CustomModelRepository
	privateModels      llmmodelsvc.PrivateModelLookupService
	availableModels    llmmodelsvc.AvailableModelsService
	db                 *gorm.DB
	crypto             shared.CryptoService
	consoleProvider    consoleintf.ConsoleProvider // Cloud mode: calls console-api HTTP for platform channels
	ollamaModelLister  func(ctx context.Context, apiBaseURL, apiKey string) ([]adapter.Model, error)
}

// NewChannelService creates a new channel service
func NewChannelService(
	tenantRouteRepo repository.TenantRouteRepository,
	tenantCredService credentialsvc.TenantCredentialService,
	validator ChannelValidator,
	modelRepo llmmodelrepo.ModelRepository,
	modelConfigRepo llmmodelrepo.ModelConfigRepository,
	customProviderRepo providerrepo.CustomProviderRepository,
	customModelRepo llmmodelrepo.CustomModelRepository,
	privateModels llmmodelsvc.PrivateModelLookupService,
	availableModels llmmodelsvc.AvailableModelsService,
	db *gorm.DB,
	crypto shared.CryptoService,
	cp consoleintf.ConsoleProvider, // nil in self-hosted mode
) ChannelService {
	if validator == nil {
		validator = channelprovider.NewValidator(modelRepo, privateModels)
	}
	return &channelService{
		tenantRouteRepo:    tenantRouteRepo,
		tenantCredService:  tenantCredService,
		validator:          validator,
		modelRepo:          modelRepo,
		modelConfigRepo:    modelConfigRepo,
		customProviderRepo: customProviderRepo,
		customModelRepo:    customModelRepo,
		privateModels:      privateModels,
		availableModels:    availableModels,
		db:                 db,
		crypto:             crypto,
		consoleProvider:    cp,
		ollamaModelLister:  listOllamaModels,
	}
}

func NewOfficialRouteBootstrapper(db *gorm.DB, cp consoleintf.ConsoleProvider) interfaces.OfficialRouteBootstrapper {
	return &channelService{
		tenantRouteRepo: repository.NewTenantRouteRepository(db),
		db:              db,
		consoleProvider: cp,
	}
}

// ============================================================================
// System channel operations (read-only for sync purposes)
// Note: System channel creation/update/delete has been moved to console-api
// ============================================================================

// ============================================================================
// Tenant route operations
// ============================================================================

func (s *channelService) CreateRoute(ctx context.Context, organizationID uuid.UUID, req *dto.CreateRouteRequest) (*dto.ChannelView, error) {
	if req.InitialFunds < 0 {
		return nil, fmt.Errorf("initial funds must be greater than or equal to 0")
	}

	spec, err := channelprovider.ValidateConnectionFields(req.ChannelProvider, req.APIBaseURL)
	if err != nil {
		return nil, err
	}
	if err := channelprovider.ValidateAPIKey(spec, req.APIKey); err != nil {
		return nil, err
	}
	channelProvider := spec.Name
	if err := validateNativeProtocolBaseURLs(spec, req.NativeProtocols); err != nil {
		return nil, err
	}

	if err := validateRouteNativeProtocols(channelProvider, req.NativeProtocols); err != nil {
		return nil, err
	}
	if err := s.ensureOllamaCustomModels(ctx, organizationID, channelProvider, req.APIBaseURL, req.APIKey, req.Models); err != nil {
		return nil, err
	}
	if err := s.validateRouteModelNames(ctx, organizationID, req.Models); err != nil {
		return nil, err
	}

	validationResult, err := s.validator.ValidateModelsForCreation(ctx, organizationID, channelProvider, req.APIKey, req.APIBaseURL, req.Models)
	if err != nil {
		return nil, err
	}
	normalizedModels := validationResult.NormalizedModels

	route := &model.LLMRoute{
		OrganizationID:   organizationID,
		Type:             shared.RouteTypePrivate,
		Name:             req.Name,
		ChannelProvider:  channelProvider,
		Models:           normalizedModels,
		APIBaseURL:       req.APIBaseURL,
		NativeProtocols:  req.NativeProtocols,
		ModelMaps:        req.ModelMaps,
		ParamOverride:    req.ParamOverride,
		HeaderOverride:   req.HeaderOverride,
		ValidationReport: validationResult.Report,
		Tags:             req.Tags,
		Description:      req.Description,
		Priority:         req.Priority,
		Weight:           req.Weight,
		IsEnabled:        true,
		Balance:          decimal.NewFromInt(req.InitialFunds),
	}

	credReq := &credentialdto.CreateTenantCredentialRequest{
		Name:            req.Name + " Credential",
		ChannelProvider: channelProvider,
		APIKey:          req.APIKey,
		APIBaseURL:      req.APIBaseURL,
	}
	cred, createdCredential, err := s.tenantCredService.GetOrCreateByAPIKey(ctx, organizationID, credReq)
	if err != nil {
		return nil, fmt.Errorf("failed to create credential: %w", err)
	}
	route.CredentialID = &cred.ID
	if route.APIBaseURL == "" {
		route.APIBaseURL = cred.APIBaseURL
	}

	if route.Priority == 0 {
		route.Priority = 100
	}
	if route.Weight == 0 {
		route.Weight = 100
	}

	if err := s.createRouteWithInitialFunds(ctx, route, req.InitialFunds); err != nil {
		if createdCredential {
			_ = s.tenantCredService.Delete(context.Background(), organizationID, cred.ID)
		}
		return nil, fmt.Errorf("failed to create route: %w", err)
	}
	if err := s.autoEnableModelsForRoute(ctx, organizationID, normalizedModels); err != nil {
		return nil, fmt.Errorf("auto-enable route models: %w", err)
	}

	// Reload with credential for building the view
	created, err := s.tenantRouteRepo.GetByID(ctx, organizationID, route.ID)
	if err != nil {
		view, viewErr := s.buildChannelViewWithWalletBalance(ctx, organizationID, route)
		if viewErr != nil {
			return nil, fmt.Errorf("load channel wallet balance: %w", viewErr)
		}
		return view, nil
	}
	view, err := s.buildChannelViewWithWalletBalance(ctx, organizationID, created)
	if err != nil {
		return nil, fmt.Errorf("load channel wallet balance: %w", err)
	}
	return view, nil
}

func (s *channelService) createRouteWithInitialFunds(
	ctx context.Context,
	route *model.LLMRoute,
	initialFunds int64,
) error {
	if s.db == nil {
		if initialFunds > 0 {
			return fmt.Errorf("initial funds require database support")
		}
		return s.tenantRouteRepo.Create(ctx, route)
	}

	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(route).Error; err != nil {
			return err
		}
		if initialFunds <= 0 {
			return nil
		}

		now := time.Now()
		wallet := &channelWalletRecord{
			ChannelID:      route.ID,
			OrganizationID: route.OrganizationID,
			Balance:        initialFunds,
			Status:         channelWalletStatusActive,
			CreatedAt:      now,
			UpdatedAt:      now,
		}
		if err := tx.Create(wallet).Error; err != nil {
			return fmt.Errorf("create channel wallet: %w", err)
		}
		return nil
	})
}

func (s *channelService) GetRoute(ctx context.Context, organizationID, id uuid.UUID) (*model.LLMRoute, error) {
	route, err := s.tenantRouteRepo.GetByID(ctx, organizationID, id)
	if err != nil {
		return nil, ErrRouteNotFound
	}
	return route, nil
}

func (s *channelService) ListRoutes(ctx context.Context, organizationID uuid.UUID, req *dto.ListRouteRequest) ([]*dto.ChannelView, int64, error) {
	offset := (req.Page - 1) * req.PageSize
	routes, total, err := s.tenantRouteRepo.List(ctx, organizationID, req.IsEnabled, offset, req.PageSize)
	if err != nil {
		return nil, 0, err
	}
	walletBalances, err := s.loadChannelWalletBalances(ctx, organizationID, routes)
	if err != nil {
		return nil, 0, fmt.Errorf("load channel wallet balances: %w", err)
	}

	views := make([]*dto.ChannelView, 0, len(routes))
	for _, route := range routes {
		view := s.buildChannelView(route, walletBalances)
		views = append(views, view)
	}
	return views, total, nil
}

// buildChannelView converts a LLMRoute model to a clean ChannelView DTO
func (s *channelService) buildChannelView(route *model.LLMRoute, walletBalances map[uuid.UUID]int64) *dto.ChannelView {
	view := &dto.ChannelView{
		ID:               route.ID,
		Name:             route.Name,
		Type:             string(route.Type),
		ChannelProvider:  route.ChannelProvider,
		Models:           route.GetEffectiveModels(),
		RemainingFunds:   walletBalances[route.ID],
		APIBaseURL:       route.APIBaseURL,
		NativeProtocols:  route.NativeProtocols,
		ValidationReport: route.ValidationReport,
		Warnings:         channelprovider.WarningMessages(route.ValidationReport),
		Priority:         route.Priority,
		Weight:           route.Weight,
		IsEnabled:        route.IsEnabled,
		AutoBan:          route.AutoBan,
		Tags:             route.Tags,
		Description:      route.Description,
		CreatedAt:        route.CreatedAt.Unix(),
		UpdatedAt:        route.UpdatedAt.Unix(),
	}

	// Extract api_base_url and api_key_masked from credential
	if route.TenantCredential != nil {
		if view.APIBaseURL == "" && route.TenantCredential.APIBaseURL != "" {
			view.APIBaseURL = route.TenantCredential.APIBaseURL
		}
		if route.TenantCredential.APIKeyCiphertext != "" && s.crypto != nil {
			decrypted, err := s.crypto.Decrypt(route.TenantCredential.APIKeyCiphertext)
			if err == nil && decrypted != "" {
				view.APIKeyMasked = maskAPIKey(decrypted)
			}
		}
	}

	return view
}

func (s *channelService) buildChannelViewWithWalletBalance(
	ctx context.Context,
	organizationID uuid.UUID,
	route *model.LLMRoute,
) (*dto.ChannelView, error) {
	walletBalances, err := s.loadChannelWalletBalances(ctx, organizationID, []*model.LLMRoute{route})
	if err != nil {
		return nil, err
	}
	return s.buildChannelView(route, walletBalances), nil
}

// ListRoutesAggregated returns a paginated list of private channels (excludes ZGI_CLOUD).
func (s *channelService) ListRoutesAggregated(ctx context.Context, organizationID uuid.UUID, req *dto.ListRoutesAggregatedRequest) (*dto.ChannelListResponse, error) {
	// Get all routes for tenant
	routes, _, err := s.tenantRouteRepo.List(ctx, organizationID, nil, 0, 1000)
	if err != nil {
		return nil, fmt.Errorf("failed to list routes: %w", err)
	}
	walletBalances, err := s.loadChannelWalletBalances(ctx, organizationID, routes)
	if err != nil {
		return nil, fmt.Errorf("failed to load channel wallet balances: %w", err)
	}

	// Build views, skip official channels
	allChannels := make([]*dto.ChannelView, 0, len(routes))

	for _, route := range routes {
		// Skip ZGI_CLOUD channels — they have a dedicated endpoint: GET /channels/platform
		if route.IsOfficial || route.Type == shared.RouteTypeZGICloud {
			continue
		}

		view := s.buildChannelView(route, walletBalances)

		// Apply filters
		if req.ChannelProvider != "" && !strings.EqualFold(view.ChannelProvider, req.ChannelProvider) {
			continue
		}
		if req.Search != "" {
			searchLower := strings.ToLower(req.Search)
			if !strings.Contains(strings.ToLower(view.Name), searchLower) &&
				!strings.Contains(strings.ToLower(view.ChannelProvider), searchLower) {
				continue
			}
		}

		allChannels = append(allChannels, view)
	}

	// Apply pagination
	total := len(allChannels)
	page := req.Page
	pageSize := req.PageSize
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	start := (page - 1) * pageSize
	if start > total {
		start = total
	}
	end := start + pageSize
	if end > total {
		end = total
	}

	return &dto.ChannelListResponse{
		Channels: allChannels[start:end],
		Total:    total,
		Page:     page,
		PageSize: pageSize,
	}, nil
}

func (s *channelService) loadChannelWalletBalances(
	ctx context.Context,
	organizationID uuid.UUID,
	routes []*model.LLMRoute,
) (map[uuid.UUID]int64, error) {
	walletBalances := make(map[uuid.UUID]int64)
	if s.db == nil || len(routes) == 0 {
		return walletBalances, nil
	}

	channelIDs := make([]uuid.UUID, 0, len(routes))
	for _, route := range routes {
		if route == nil || route.IsOfficial || route.Type != shared.RouteTypePrivate {
			continue
		}
		channelIDs = append(channelIDs, route.ID)
	}
	if len(channelIDs) == 0 {
		return walletBalances, nil
	}

	var wallets []channelWalletRecord
	if err := s.db.WithContext(ctx).
		Select("channel_id", "balance").
		Where("organization_id = ? AND channel_id IN ?", organizationID, channelIDs).
		Find(&wallets).Error; err != nil {
		return nil, err
	}
	for _, wallet := range wallets {
		walletBalances[wallet.ChannelID] = wallet.Balance
	}
	return walletBalances, nil
}

func (s *channelService) UpdateRoute(ctx context.Context, organizationID, id uuid.UUID, req *dto.UpdateRouteRequest) (*dto.ChannelView, error) {
	route, err := s.tenantRouteRepo.GetByID(ctx, organizationID, id)
	if err != nil {
		return nil, ErrRouteNotFound
	}

	newChannelProvider := route.ChannelProvider
	var normalizedChannelProvider *string
	if req.ChannelProvider != nil {
		spec, err := channelprovider.Resolve(*req.ChannelProvider)
		if err != nil {
			return nil, err
		}
		newChannelProvider = spec.Name
		normalizedChannelProvider = &spec.Name
	}

	newModels := route.Models
	if req.Models != nil {
		newModels = req.Models
	}

	newAPIBaseURL := route.APIBaseURL
	if req.APIBaseURL != nil {
		newAPIBaseURL = *req.APIBaseURL
	}

	spec, err := channelprovider.ValidateConnectionFields(newChannelProvider, newAPIBaseURL)
	if err != nil {
		return nil, err
	}
	effectiveNativeProtocols := route.NativeProtocols
	if req.NativeProtocols != nil {
		effectiveNativeProtocols = *req.NativeProtocols
	}
	if err := validateNativeProtocolBaseURLs(spec, effectiveNativeProtocols); err != nil {
		return nil, err
	}
	if req.APIKey != nil {
		if err := channelprovider.ValidateAPIKey(spec, *req.APIKey); err != nil {
			return nil, err
		}
	} else if route.CredentialID == nil {
		if err := channelprovider.ValidateAPIKey(spec, ""); err != nil {
			return nil, err
		}
	}

	newNativeProtocols := route.NativeProtocols
	if req.NativeProtocols != nil {
		newNativeProtocols = *req.NativeProtocols
	}
	if err := validateRouteNativeProtocols(newChannelProvider, newNativeProtocols); err != nil {
		return nil, err
	}

	coreChanged := req.ChannelProvider != nil || req.Models != nil || req.APIBaseURL != nil || req.APIKey != nil
	if coreChanged && !route.IsOfficial {
		apiKey := ""
		if req.APIKey != nil {
			apiKey = *req.APIKey
		} else if route.CredentialID != nil {
			apiKey, err = s.tenantCredService.GetDecryptedAPIKey(ctx, organizationID, *route.CredentialID)
			if err != nil {
				return nil, fmt.Errorf("failed to load credential api key: %w", err)
			}
		}

		if err := s.ensureOllamaCustomModels(ctx, organizationID, newChannelProvider, newAPIBaseURL, apiKey, newModels); err != nil {
			return nil, err
		}
		if err := s.validateRouteModelNames(ctx, organizationID, newModels); err != nil {
			return nil, err
		}
		if route.CredentialID == nil && !spec.AllowsEmptyKey && (req.APIKey == nil || *req.APIKey == "") {
			return nil, fmt.Errorf("route has no credential configured")
		}

		validationResult, err := s.validator.ValidateModelsForCreation(ctx, organizationID, newChannelProvider, apiKey, newAPIBaseURL, newModels)
		if err != nil {
			return nil, err
		}
		route.ValidationReport = validationResult.Report
		newModels = validationResult.NormalizedModels
	}

	if req.Name != nil {
		route.Name = *req.Name
	}
	if normalizedChannelProvider != nil {
		route.ChannelProvider = *normalizedChannelProvider
	}
	if req.Models != nil {
		route.Models = newModels
	}
	if req.APIBaseURL != nil {
		route.APIBaseURL = newAPIBaseURL
	}
	if req.NativeProtocols != nil {
		route.NativeProtocols = *req.NativeProtocols
	}
	if req.ModelMaps != nil {
		route.ModelMaps = req.ModelMaps
	}
	if req.ParamOverride != nil {
		route.ParamOverride = req.ParamOverride
	}
	if req.HeaderOverride != nil {
		route.HeaderOverride = req.HeaderOverride
	}
	if req.Tags != nil {
		route.Tags = req.Tags
	}
	if req.Description != nil {
		route.Description = *req.Description
	}
	if req.Priority != nil {
		route.Priority = *req.Priority
	}
	if req.Weight != nil {
		route.Weight = *req.Weight
	}
	if req.IsEnabled != nil {
		route.IsEnabled = *req.IsEnabled
	}

	if route.CredentialID != nil && (normalizedChannelProvider != nil || req.APIBaseURL != nil || req.APIKey != nil) {
		credUpdateReq := &credentialdto.UpdateTenantCredentialRequest{
			ChannelProvider: normalizedChannelProvider,
			APIBaseURL:      req.APIBaseURL,
		}
		if req.APIKey != nil {
			credUpdateReq.APIKey = req.APIKey
		}
		if _, err := s.tenantCredService.Update(ctx, organizationID, *route.CredentialID, credUpdateReq); err != nil {
			return nil, fmt.Errorf("failed to update credential: %w", err)
		}
	}

	if err := s.tenantRouteRepo.Update(ctx, route); err != nil {
		return nil, fmt.Errorf("failed to update route: %w", err)
	}
	if req.Models != nil {
		if err := s.autoEnableModelsForRoute(ctx, organizationID, route.Models); err != nil {
			return nil, fmt.Errorf("auto-enable route models: %w", err)
		}
	}

	view, err := s.buildChannelViewWithWalletBalance(ctx, organizationID, route)
	if err != nil {
		return nil, fmt.Errorf("load channel wallet balance: %w", err)
	}
	return view, nil
}

func (s *channelService) validateRouteModelNames(ctx context.Context, organizationID uuid.UUID, models []string) error {
	if s == nil || len(models) == 0 {
		return nil
	}

	exactNames, legacyShortNames, err := s.loadActiveModelNameIndexes(ctx, organizationID)
	if err != nil {
		return fmt.Errorf("failed to load model library: %w", err)
	}

	for _, modelName := range models {
		normalizedModelName := strings.TrimSpace(modelName)
		if normalizedModelName == "" || normalizedModelName == "*" {
			continue
		}
		if slices.Contains(exactNames, normalizedModelName) {
			continue
		}
		if fullModelName, ok := legacyShortNames[normalizedModelName]; ok {
			return fmt.Errorf("model %q must use the full model name %q", normalizedModelName, fullModelName)
		}
	}

	return nil
}

func validateRouteNativeProtocols(channelProvider string, protocols model.NativeProtocolConfig) error {
	if protocols.OpenAIResponses.Enabled {
		capability := channelprovider.OpenAIResponsesCapability(channelProvider)
		if !capability.Supported {
			return fmt.Errorf("native_protocols.openai_responses is not supported by channel_provider %q", channelProvider)
		}
	}
	if protocols.AnthropicMessages.Enabled {
		capability := channelprovider.AnthropicMessagesCapability(channelProvider)
		if !capability.Supported {
			return fmt.Errorf("native_protocols.anthropic_messages is not supported by channel_provider %q", channelProvider)
		}
	}
	return nil
}

func (s *channelService) loadActiveModelNameIndexes(ctx context.Context, organizationID uuid.UUID) ([]string, map[string]string, error) {
	exactNames := make([]string, 0)
	legacyShortNames := make(map[string]string)

	if s != nil && s.modelRepo != nil {
		activeOnly := true
		models, _, err := s.modelRepo.List(ctx, nil, "", "", "active", &activeOnly, 0, 10000)
		if err != nil {
			return nil, nil, err
		}
		exactNames, legacyShortNames = appendModelNameIndexes(exactNames, legacyShortNames, models)
	}

	if s != nil && s.privateModels != nil && organizationID != uuid.Nil {
		privateExactNames, privateLegacyShortNames, err := s.privateModels.LoadActiveModelNameIndexes(ctx, organizationID)
		if err != nil {
			return nil, nil, err
		}
		exactNames = append(exactNames, privateExactNames...)
		for shortName, fullModelName := range privateLegacyShortNames {
			if _, exists := legacyShortNames[shortName]; !exists {
				legacyShortNames[shortName] = fullModelName
			}
		}
	}

	return exactNames, legacyShortNames, nil
}

func appendModelNameIndexes(exactNames []string, legacyShortNames map[string]string, models []*llmmodelmodel.LLMModel) ([]string, map[string]string) {
	for _, record := range models {
		if record == nil {
			continue
		}

		modelName := strings.TrimSpace(record.Model)
		if modelName == "" {
			continue
		}

		exactNames = append(exactNames, modelName)
		if strings.Count(modelName, "/") != 1 {
			continue
		}

		parts := strings.SplitN(modelName, "/", 2)
		shortModelName := strings.TrimSpace(parts[1])
		if shortModelName == "" {
			continue
		}
		if _, exists := legacyShortNames[shortModelName]; !exists {
			legacyShortNames[shortModelName] = modelName
		}
	}

	return exactNames, legacyShortNames
}

func (s *channelService) DeleteRoute(ctx context.Context, organizationID, id uuid.UUID) error {
	// Get route before deletion to check credential
	route, err := s.tenantRouteRepo.GetByID(ctx, organizationID, id)
	if err != nil {
		return ErrRouteNotFound
	}

	// Store credential ID for cleanup check
	var credentialID *uuid.UUID
	if route.CredentialID != nil {
		credentialID = route.CredentialID
	}

	// Delete the route
	if err := s.tenantRouteRepo.Delete(ctx, organizationID, id); err != nil {
		return err
	}

	// Asynchronously cleanup unused credential
	if credentialID != nil {
		go s.cleanupUnusedCredential(context.Background(), organizationID, *credentialID)
	}

	return nil
}

// cleanupUnusedCredential checks if a credential is still referenced by any routes
// and deletes it if no longer in use
func (s *channelService) cleanupUnusedCredential(ctx context.Context, organizationID, credentialID uuid.UUID) {
	// Check if credential is still referenced by any routes
	count, err := s.tenantRouteRepo.CountByCredentialID(ctx, organizationID, credentialID)
	if err != nil {
		// Log error but don't fail - cleanup is best effort
		return
	}

	// If no routes reference this credential, delete it
	if count == 0 {
		_ = s.tenantCredService.Delete(ctx, organizationID, credentialID)
	}
}

// UpdateOfficialChannelSettings updates settings for all routes in an official channel group
// Returns the number of routes updated
func (s *channelService) UpdateOfficialChannelSettings(ctx context.Context, organizationID uuid.UUID, req *dto.UpdateOfficialChannelSettingsRequest) (int64, error) {
	routes, _, err := s.tenantRouteRepo.List(ctx, organizationID, nil, 0, 1000)
	if err != nil {
		return 0, fmt.Errorf("failed to list routes: %w", err)
	}

	var routesToUpdate []*model.LLMRoute
	for _, route := range routes {
		if route.IsOfficial && route.ChannelProvider == req.GroupID {
			routesToUpdate = append(routesToUpdate, route)
		}
	}

	if len(routesToUpdate) == 0 {
		return 0, fmt.Errorf("no routes found for channel group: %s", req.GroupID)
	}

	var updated int64
	for _, route := range routesToUpdate {
		if req.Priority != nil {
			route.Priority = *req.Priority
		}
		if req.Weight != nil {
			route.Weight = *req.Weight
		}
		if req.IsEnabled != nil {
			route.IsEnabled = *req.IsEnabled
		}
		if err := s.tenantRouteRepo.Update(ctx, route); err != nil {
			return updated, fmt.Errorf("failed to update route %s: %w", route.ID, err)
		}
		updated++
	}

	return updated, nil
}

// ============================================================================
// Route selection
// ============================================================================

func (s *channelService) SelectRoute(ctx context.Context, organizationID uuid.UUID, modelName string) (*model.RouteQueryResult, error) {
	routes, err := s.GetRoutesForModel(ctx, organizationID, modelName)
	if err != nil {
		return nil, err
	}

	if len(routes) == 0 {
		return nil, ErrRouteNotFound
	}

	// Select route using priority and weighted random selection
	return s.selectRouteByPriorityAndWeight(routes), nil
}

// selectRouteByPriorityAndWeight selects a route based on priority and weight
// Routes with higher priority are preferred, and among same priority routes,
// selection is done using weighted random selection
func (s *channelService) selectRouteByPriorityAndWeight(routes []*model.RouteQueryResult) *model.RouteQueryResult {
	if len(routes) == 0 {
		return nil
	}
	if len(routes) == 1 {
		return routes[0]
	}

	// Find the highest priority
	maxPriority := routes[0].Priority
	for _, r := range routes {
		if r.Priority > maxPriority {
			maxPriority = r.Priority
		}
	}

	// Filter routes with the highest priority
	var topPriorityRoutes []*model.RouteQueryResult
	for _, r := range routes {
		if r.Priority == maxPriority {
			topPriorityRoutes = append(topPriorityRoutes, r)
		}
	}

	// If only one route with highest priority, return it
	if len(topPriorityRoutes) == 1 {
		return topPriorityRoutes[0]
	}

	// Weighted random selection among routes with same priority
	return s.weightedRandomSelect(topPriorityRoutes)
}

// weightedRandomSelect performs weighted random selection
func (s *channelService) weightedRandomSelect(routes []*model.RouteQueryResult) *model.RouteQueryResult {
	totalWeight := 0
	for _, r := range routes {
		totalWeight += r.Weight
	}

	if totalWeight == 0 {
		// If all weights are 0, return random route
		return routes[rand.Intn(len(routes))]
	}

	n := rand.Intn(totalWeight)
	for _, r := range routes {
		n -= r.Weight
		if n < 0 {
			return r
		}
	}

	return routes[0]
}

func (s *channelService) GetRoutesForModel(ctx context.Context, organizationID uuid.UUID, modelName string) ([]*model.RouteQueryResult, error) {
	routes, err := s.tenantRouteRepo.GetEnabledRoutes(ctx, organizationID)
	if err != nil {
		return nil, fmt.Errorf("failed to get enabled routes: %w", err)
	}

	// No auto-initialization - tenant must create routes manually

	var result []*model.RouteQueryResult
	for _, r := range routes {
		// Check if route supports the model
		if !s.routeSupportsModel(r, modelName) {
			continue
		}

		qr := &model.RouteQueryResult{
			RouteID:        r.ID,
			OrganizationID: r.OrganizationID,
			Type:           r.Type,
			Source:         model.RouteSourceExplicit,
			Name:           r.Name,
			Priority:       r.Priority,
			Weight:         r.Weight,
			ModelMaps:      r.ModelMaps,
			ParamOverride:  r.ParamOverride,
			HeaderOverride: r.HeaderOverride,
		}

		// Fill in details based on route type
		if r.IsOfficial {
			qr.ChannelProvider = r.ChannelProvider
			qr.Models = r.GetEffectiveModels()
			qr.APIBaseURL = r.APIBaseURL
		} else if r.IsUserChannel() {
			qr.ChannelProvider = r.ChannelProvider
			qr.Models = r.GetEffectiveModels()
			qr.APIBaseURL = r.APIBaseURL
			if r.TenantCredential != nil {
				qr.APIKeyCiphertext = r.TenantCredential.APIKeyCiphertext
			}
		}

		result = append(result, qr)
	}

	return result, nil
}

func (s *channelService) routeSupportsModel(route *model.LLMRoute, modelName string) bool {
	return route.SupportsModel(modelName)
}

// ============================================================================
// Test Channel/Route Implementation
// ============================================================================

// TestRoute tests a tenant route by sending a chat completion request
func (s *channelService) TestRoute(ctx context.Context, organizationID, id uuid.UUID, requestedModel string) (*dto.TestChannelResult, error) {
	route, err := s.tenantRouteRepo.GetByID(ctx, organizationID, id)
	if err != nil {
		return nil, ErrRouteNotFound
	}

	// For official routes, cannot test directly (proxied to Console)
	if route.IsOfficial {
		return &dto.TestChannelResult{
			Success: false,
			Message: "Official channels are proxied to Console and cannot be tested directly from ZGI-API",
		}, nil
	}

	// For PRIVATE routes, test the tenant's credential
	if route.CredentialID == nil {
		return &dto.TestChannelResult{
			Success: false,
			Message: "route has no credential configured",
		}, nil
	}

	if route.IsUserChannel() {
		if len(route.Models) == 0 {
			return &dto.TestChannelResult{
				Success: false,
				Message: "route has no models configured",
			}, nil
		}

		// Use the requested model if provided, otherwise fallback to the first model
		testModel := requestedModel
		if testModel == "" {
			testModel = route.Models[0]
		}

		apiKey, err := s.tenantCredService.GetDecryptedAPIKey(ctx, organizationID, *route.CredentialID)
		if err != nil {
			return &dto.TestChannelResult{
				Success: false,
				Message: fmt.Sprintf("failed to load credential api key: %v", err),
			}, nil
		}
		result, err := s.validator.TestModel(ctx, organizationID, route.ChannelProvider, apiKey, route.APIBaseURL, testModel, "")
		if err != nil {
			return &dto.TestChannelResult{
				Success: false,
				Message: fmt.Sprintf("failed to test credential: %v", err),
			}, nil
		}

		return &dto.TestChannelResult{
			Success:      result.Success,
			Message:      result.Message,
			ResponseTime: result.ResponseTimeMs,
			Models:       route.Models,
		}, nil
	}

	return &dto.TestChannelResult{
		Success: false,
		Message: "route has no associated credential",
	}, nil
}

// UpdateChannelBalance updates the balance for a channel
func (s *channelService) UpdateChannelBalance(ctx context.Context, id uuid.UUID, organizationID uuid.UUID) (*dto.UpdateChannelBalanceResponse, error) {
	// For now, return a placeholder response
	// In a real implementation, this would query the provider API for balance
	return &dto.UpdateChannelBalanceResponse{
		ChannelID:   id,
		OldBalance:  "0",
		NewBalance:  "0",
		Currency:    "USD",
		UpdatedAt:   time.Now().Format(time.RFC3339),
		IsUnlimited: true,
	}, nil
}

// GetPlatformChannel returns the aggregated official channel view for a tenant.
// Reads from llm_routes WHERE is_official=true for the given organization.
func (s *channelService) GetPlatformChannel(ctx context.Context, organizationID uuid.UUID) (*dto.PlatformChannelAggregatedView, error) {
	route, err := s.ensureOfficialRoute(ctx, organizationID)
	if err != nil {
		return nil, fmt.Errorf("failed to get official route: %w", err)
	}

	if route == nil {
		// No official channels are available for this organization.
		return nil, nil
	}

	models, err := officialmodel.GetEffectiveModels(ctx, s.db)
	if err != nil {
		return nil, fmt.Errorf("failed to load official model snapshot: %w", err)
	}

	return &dto.PlatformChannelAggregatedView{
		Name:       route.Name,
		ModelCount: len(models),
		Priority:   route.Priority,
		Weight:     route.Weight,
		IsEnabled:  route.IsEnabled,
	}, nil
}

// UpdatePlatformChannel updates routing-related fields of a platform channel via console-api.
func (s *channelService) UpdatePlatformChannel(ctx context.Context, channelID string, req *dto.UpdatePlatformChannelRequest) error {
	if s.consoleProvider == nil || !s.consoleProvider.IsAvailable() {
		return fmt.Errorf("platform channel updates are not available in self-hosted mode")
	}

	consoleReq := &consoleintf.UpdatePlatformChannelRequest{
		Priority: req.Priority,
		Weight:   req.Weight,
		IsActive: req.IsEnabled,
	}

	return s.consoleProvider.UpdatePlatformChannel(ctx, channelID, consoleReq)
}

// UpdatePlatformChannelSettings updates the tenant's official route priority/weight/is_enabled.
func (s *channelService) UpdatePlatformChannelSettings(ctx context.Context, organizationID uuid.UUID, req *dto.UpdatePlatformChannelRequest) error {
	route, err := s.ensureOfficialRoute(ctx, organizationID)
	if err != nil {
		return fmt.Errorf("failed to find official route: %w", err)
	}
	if route == nil {
		return ErrNoAvailableOfficialChannel
	}

	if req.Priority != nil {
		route.Priority = *req.Priority
	}
	if req.Weight != nil {
		route.Weight = *req.Weight
	}
	if req.IsEnabled != nil {
		route.IsEnabled = *req.IsEnabled
	}

	return s.tenantRouteRepo.Update(ctx, route)
}

// InitOfficialChannel ensures the tenant has an official route in cloud mode.
// Official models are still sourced from llm_official_model_snapshots at read time, so the
// route may exist before the snapshot is populated.
func (s *channelService) InitOfficialChannel(ctx context.Context, organizationID uuid.UUID) error {
	if s.consoleProvider == nil || !s.consoleProvider.IsAvailable() {
		return nil // Self-hosted mode: no official channels
	}

	// Check if an official route already exists for this org
	existing, err := s.findOfficialRoute(ctx, organizationID)
	if err != nil {
		return fmt.Errorf("failed to check existing official route: %w", err)
	}

	if existing != nil {
		existing.APIBaseURL = ""
		now := time.Now().UTC()
		existing.LastSyncedAt = &now
		return s.tenantRouteRepo.Update(ctx, existing)
	}

	// Create new official route
	route := &model.LLMRoute{
		OrganizationID:  organizationID,
		Type:            shared.RouteTypeZGICloud,
		Name:            "ZGI Cloud",
		ChannelProvider: "zgi-cloud",
		APIBaseURL:      "",
		Priority:        200,
		Weight:          100,
		IsEnabled:       true,
		IsOfficial:      true,
		SyncMode:        "snapshot",
	}
	if err := s.tenantRouteRepo.Create(ctx, route); err != nil {
		// Concurrent initialization can hit unique index conflict. Re-read and treat as success.
		if isUniqueConstraintViolation(err) {
			existing, findErr := s.findOfficialRoute(ctx, organizationID)
			if findErr == nil && existing != nil {
				return nil
			}
		}
		return err
	}

	return nil
}

// ensureOfficialRoute returns an existing official route, or attempts lazy initialization once.
func (s *channelService) ensureOfficialRoute(ctx context.Context, organizationID uuid.UUID) (*model.LLMRoute, error) {
	route, err := s.findOfficialRoute(ctx, organizationID)
	if err != nil {
		return nil, err
	}
	if route != nil {
		return route, nil
	}

	if err := s.InitOfficialChannel(ctx, organizationID); err != nil && !isUniqueConstraintViolation(err) {
		return nil, fmt.Errorf("failed to initialize official channel: %w", err)
	}

	return s.findOfficialRoute(ctx, organizationID)
}

// findOfficialRoute finds the existing official route for an organization
func (s *channelService) findOfficialRoute(ctx context.Context, organizationID uuid.UUID) (*model.LLMRoute, error) {
	var route model.LLMRoute
	err := s.db.WithContext(ctx).
		Where("organization_id = ? AND is_official = true AND deleted_at IS NULL", organizationID).
		First(&route).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	if err := officialmodel.HydrateRoute(ctx, s.db, &route); err != nil {
		return nil, err
	}
	return &route, nil
}

func isUniqueConstraintViolation(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23505"
	}

	return strings.Contains(strings.ToLower(err.Error()), "duplicate key")
}

func validateNativeProtocolBaseURLs(spec channelprovider.Spec, protocols model.NativeProtocolConfig) error {
	if err := channelprovider.ValidateBaseURLForSpec(spec, "native_protocols.openai_responses.base_url", protocols.OpenAIResponses.BaseURL); err != nil {
		return err
	}
	if err := channelprovider.ValidateBaseURLForSpec(spec, "native_protocols.anthropic_messages.base_url", protocols.AnthropicMessages.BaseURL); err != nil {
		return err
	}
	return nil
}

// TestChannelModel tests a specific model on a channel
func (s *channelService) TestDraftChannelModel(ctx context.Context, organizationID uuid.UUID, req *dto.DraftTestChannelModelRequest) (*dto.ChannelModelTestResult, error) {
	spec, err := channelprovider.ValidateConnectionFields(req.ChannelProvider, req.APIBaseURL)
	if err != nil {
		return &dto.ChannelModelTestResult{
			Success: false,
			Message: err.Error(),
			Model:   strings.TrimSpace(req.Model),
		}, nil
	}
	if err := channelprovider.ValidateAPIKey(spec, req.APIKey); err != nil {
		return &dto.ChannelModelTestResult{
			Success: false,
			Message: err.Error(),
			Model:   strings.TrimSpace(req.Model),
		}, nil
	}
	if err := s.ensureOllamaCustomModels(ctx, organizationID, spec.Name, req.APIBaseURL, req.APIKey, []string{req.Model}); err != nil {
		return &dto.ChannelModelTestResult{
			Success: false,
			Message: err.Error(),
			Model:   strings.TrimSpace(req.Model),
		}, nil
	}

	result, err := s.validator.TestModel(ctx, organizationID, spec.Name, req.APIKey, req.APIBaseURL, req.Model, req.TestMethod)
	if err != nil {
		return &dto.ChannelModelTestResult{
			Success: false,
			Message: err.Error(),
			Model:   strings.TrimSpace(req.Model),
		}, nil
	}

	return buildChannelModelTestResult(result), nil
}

func (s *channelService) DiscoverDraftChannelModels(ctx context.Context, req *dto.DiscoverDraftChannelModelsRequest) (*dto.DiscoverDraftChannelModelsResponse, error) {
	spec, err := channelprovider.ValidateConnectionFields(req.ChannelProvider, req.APIBaseURL)
	if err != nil {
		return nil, err
	}
	if err := channelprovider.ValidateAPIKey(spec, req.APIKey); err != nil {
		return nil, err
	}
	llmConfig := appconfig.Current().LLM

	adapterInstance, err := adapter.NewAdapter(&adapter.AdapterConfig{
		ProviderName:        spec.AdapterKey,
		APIKey:              req.APIKey,
		BaseURL:             req.APIBaseURL,
		Timeout:             30 * time.Second,
		MaxRetries:          1,
		GuardOutboundURL:    llmConfig.OutboundURLGuardEnabled(),
		GuardOutboundDNS:    llmConfig.GuardOutboundDNS,
		AllowPrivateBaseURL: channelprovider.AllowsPrivateBaseURL(spec.Name),
	})
	if err != nil {
		return nil, fmt.Errorf("create %s adapter: %w", spec.Name, err)
	}

	upstreamModels, err := adapterInstance.ListModels(ctx, req.APIKey)
	if err != nil {
		if adapter.IsCapabilityUnsupported(err) {
			return &dto.DiscoverDraftChannelModelsResponse{
				Models:           []dto.DiscoveredChannelModelView{},
				Total:            0,
				ListingSupported: false,
			}, nil
		}
		return nil, fmt.Errorf("discover %s models: %w", spec.Name, err)
	}

	views := make([]dto.DiscoveredChannelModelView, 0, len(upstreamModels))
	seen := make(map[string]struct{}, len(upstreamModels))
	for _, item := range upstreamModels {
		id := strings.TrimSpace(item.ID)
		if id == "" {
			id = strings.TrimSpace(item.Name)
		}
		if id == "" {
			continue
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		displayName := strings.TrimSpace(item.Name)
		if displayName == "" {
			displayName = id
		}
		views = append(views, dto.DiscoveredChannelModelView{
			ID:            id,
			Name:          id,
			DisplayName:   displayName,
			Provider:      spec.LookupProvider,
			OwnedBy:       item.OwnedBy,
			ContextLength: item.ContextLength,
			Capabilities:  append([]string(nil), item.Capabilities...),
			Created:       item.Created,
		})
	}

	return &dto.DiscoverDraftChannelModelsResponse{
		Models:           views,
		Total:            len(views),
		ListingSupported: true,
	}, nil
}

func (s *channelService) TestChannelModel(ctx context.Context, channelID uuid.UUID, organizationID uuid.UUID, modelName string, testMethod string) (*dto.ChannelModelTestResult, error) {
	// Get the route
	route, err := s.tenantRouteRepo.GetByID(ctx, organizationID, channelID)
	if err != nil {
		return &dto.ChannelModelTestResult{
			Success: false,
			Message: fmt.Sprintf("route not found: %v", err),
			Model:   strings.TrimSpace(modelName),
		}, nil
	}

	// Check if route has a credential
	if route.CredentialID == nil {
		return &dto.ChannelModelTestResult{
			Success: false,
			Message: "route has no associated credential",
			Model:   strings.TrimSpace(modelName),
		}, nil
	}

	apiKey, err := s.tenantCredService.GetDecryptedAPIKey(ctx, organizationID, *route.CredentialID)
	if err != nil {
		return &dto.ChannelModelTestResult{
			Success: false,
			Message: fmt.Sprintf("failed to load credential api key: %v", err),
			Model:   strings.TrimSpace(modelName),
		}, nil
	}
	result, err := s.validator.TestModel(ctx, organizationID, route.ChannelProvider, apiKey, route.APIBaseURL, modelName, testMethod)
	if err != nil {
		return &dto.ChannelModelTestResult{
			Success: false,
			Message: fmt.Sprintf("failed to test credential: %v", err),
			Model:   strings.TrimSpace(modelName),
		}, nil
	}

	return buildChannelModelTestResult(result), nil
}

// BatchTestChannelModels tests multiple models on a channel and streams results
func (s *channelService) BatchTestChannelModels(ctx context.Context, channelID uuid.UUID, organizationID uuid.UUID, models []string, testMethod string, resultChan chan<- *dto.BatchTestChannelModelsStreamResponse) {
	defer close(resultChan)

	for _, modelName := range models {
		result, err := s.TestChannelModel(ctx, channelID, organizationID, modelName, testMethod)

		response := &dto.BatchTestChannelModelsStreamResponse{
			Model:     modelName,
			Completed: false,
		}

		if err != nil {
			response.Success = false
			response.Message = err.Error()
		} else {
			response.Success = result.Success
			response.Message = result.Message
			response.ResponseTime = result.ResponseTimeMs
		}

		resultChan <- response
	}

	// Send completion message
	resultChan <- &dto.BatchTestChannelModelsStreamResponse{
		Completed: true,
	}
}

func buildChannelModelTestResult(result *channelprovider.TestResult) *dto.ChannelModelTestResult {
	if result == nil {
		return &dto.ChannelModelTestResult{
			Success: false,
			Message: "empty validation result",
		}
	}

	testMethod := strings.TrimSpace(result.TestMethod)
	if testMethod == "" {
		testMethod = strings.TrimSpace(result.UseCase)
	}

	return &dto.ChannelModelTestResult{
		Success:        result.Success,
		Message:        result.Message,
		Model:          result.Model,
		UseCase:        result.UseCase,
		TestMethod:     testMethod,
		ResponseTimeMs: result.ResponseTimeMs,
	}
}

// ============================================================================
// Batch operations
// ============================================================================

// BatchToggleRoutes toggles multiple routes at once
func (s *channelService) BatchToggleRoutes(ctx context.Context, organizationID uuid.UUID, req *dto.BatchToggleRoutesRequest) (*dto.BatchOperationResult, error) {
	result := &dto.BatchOperationResult{
		TotalCount: len(req.RouteIDs),
	}

	for _, routeID := range req.RouteIDs {
		route, err := s.tenantRouteRepo.GetByID(ctx, organizationID, routeID)
		if err != nil {
			result.FailedCount++
			result.FailedIDs = append(result.FailedIDs, routeID.String())
			continue
		}

		route.IsEnabled = req.IsEnabled
		if err := s.tenantRouteRepo.Update(ctx, route); err != nil {
			result.FailedCount++
			result.FailedIDs = append(result.FailedIDs, routeID.String())
			continue
		}

		result.SuccessCount++
	}

	return result, nil
}

// GetAvailableProviders returns all distinct channel_provider values actually used by tenant channels.
func (s *channelService) GetAvailableProviders(ctx context.Context, organizationID uuid.UUID) ([]string, error) {
	return s.tenantRouteRepo.GetDistinctProviders(ctx, organizationID)
}

// BatchDeleteRoutes deletes multiple routes at once
func (s *channelService) BatchDeleteRoutes(ctx context.Context, organizationID uuid.UUID, req *dto.BatchDeleteRoutesRequest) (*dto.BatchOperationResult, error) {
	result := &dto.BatchOperationResult{
		TotalCount: len(req.RouteIDs),
	}

	for _, routeID := range req.RouteIDs {
		if err := s.tenantRouteRepo.Delete(ctx, organizationID, routeID); err != nil {
			result.FailedCount++
			result.FailedIDs = append(result.FailedIDs, routeID.String())
			continue
		}

		result.SuccessCount++
	}

	return result, nil
}

// maskAPIKey masks an API key, showing first 4 and last 4 characters
// Example: "sk-1234567890abcdef" -> "sk-1************cdef"
func maskAPIKey(key string) string {
	if len(key) <= 8 {
		return strings.Repeat("*", len(key))
	}
	return key[:4] + strings.Repeat("*", len(key)-8) + key[len(key)-4:]
}
