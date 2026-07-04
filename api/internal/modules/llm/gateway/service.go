package gateway

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	appconfig "github.com/zgiai/zgi/api/config"
	"github.com/zgiai/zgi/api/internal/infra/platform"
	pchannel "github.com/zgiai/zgi/api/internal/infra/platform/channel"
	pconsole "github.com/zgiai/zgi/api/internal/infra/platform/console"
	apikeymodel "github.com/zgiai/zgi/api/internal/modules/llm/apikey/model"
	apikeyrepo "github.com/zgiai/zgi/api/internal/modules/llm/apikey/repository"
	"github.com/zgiai/zgi/api/internal/modules/llm/channelprovider"
	llmerrors "github.com/zgiai/zgi/api/internal/modules/llm/errors"
	llmmodel "github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/model"
	llmmodelrepo "github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/repository"
	llmmodelsvc "github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/service"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/modules/llm/shared"
	paymentRepo "github.com/zgiai/zgi/api/internal/modules/payment/repository"
	paymentservice "github.com/zgiai/zgi/api/internal/modules/payment/service"
	"github.com/zgiai/zgi/api/pkg/logger"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// AppContext contains app-specific context for tracking usage by apps (agents/datasets)
type AppContext struct {
	AppID              *uuid.UUID // App ID (agent or dataset)
	AppType            *string    // App type: "agent" or "dataset"
	AccountID          *uuid.UUID // User account ID who is using the app
	WorkspaceID        *string    // Optional workspace subject for workspace-level quota billing
	BillingSubjectType *string    // Optional billing subject override for app-scoped calls
	SessionID          string
	ConversationID     string
	WorkflowID         string
	WorkflowRunID      string
	NodeID             string
	NodeType           string
}

// LLMGatewayService defines the interface for LLM gateway operations
type LLMGatewayService interface {
	// ChatCompletion handles chat completion requests
	ChatCompletion(ctx context.Context, apiKey *apikeymodel.TenantAPIKey, req *adapter.ChatRequest) (*adapter.ChatResponse, error)

	// ChatCompletionStream handles streaming chat completion requests
	ChatCompletionStream(ctx context.Context, apiKey *apikeymodel.TenantAPIKey, req *adapter.ChatRequest) (<-chan adapter.StreamResponse, error)

	// CreateResponse handles response creation requests
	CreateResponse(ctx context.Context, apiKey *apikeymodel.TenantAPIKey, req *adapter.CreateResponseRequest) (*adapter.CreateResponseResponse, error)

	// CreateResponseRaw handles native OpenAI Responses requests
	CreateResponseRaw(ctx context.Context, apiKey *apikeymodel.TenantAPIKey, req *adapter.RawResponseRequest) (*adapter.RawResponse, error)

	// CreateResponseStream handles native OpenAI Responses stream requests
	CreateResponseStream(ctx context.Context, apiKey *apikeymodel.TenantAPIKey, req *adapter.RawResponseRequest) (<-chan adapter.RawStreamEvent, error)

	// CreateAnthropicMessage handles native Anthropic Messages requests
	CreateAnthropicMessage(ctx context.Context, apiKey *apikeymodel.TenantAPIKey, req *adapter.AnthropicMessageRequest) (*adapter.RawResponse, error)

	// CreateAnthropicMessageStream handles native Anthropic Messages stream requests
	CreateAnthropicMessageStream(ctx context.Context, apiKey *apikeymodel.TenantAPIKey, req *adapter.AnthropicMessageRequest) (<-chan adapter.RawStreamEvent, error)

	// CreateEmbeddings handles embeddings creation requests
	CreateEmbeddings(ctx context.Context, apiKey *apikeymodel.TenantAPIKey, req *adapter.EmbeddingsRequest) (*adapter.EmbeddingsResponse, error)

	// CreateImage handles image generation requests
	CreateImage(ctx context.Context, apiKey *apikeymodel.TenantAPIKey, req *adapter.ImageRequest) (*adapter.ImageResponse, error)

	// Rerank handles rerank requests
	Rerank(ctx context.Context, apiKey *apikeymodel.TenantAPIKey, req *adapter.RerankRequest) (*adapter.RerankResponse, error)

	// ListAvailableModels lists available models for the API key
	ListAvailableModels(ctx context.Context, apiKey *apikeymodel.TenantAPIKey) ([]adapter.Model, error)

	// ChatCompletionWithAppContext handles chat completion requests with app context for usage tracking
	ChatCompletionWithAppContext(ctx context.Context, apiKey *apikeymodel.TenantAPIKey, appCtx *AppContext, req *adapter.ChatRequest) (*adapter.ChatResponse, error)

	// ChatCompletionStreamWithAppContext handles streaming chat completion requests with app context
	ChatCompletionStreamWithAppContext(ctx context.Context, apiKey *apikeymodel.TenantAPIKey, appCtx *AppContext, req *adapter.ChatRequest) (<-chan adapter.StreamResponse, error)

	// CreateResponseWithAppContext handles response creation requests with app context
	CreateResponseWithAppContext(ctx context.Context, apiKey *apikeymodel.TenantAPIKey, appCtx *AppContext, req *adapter.CreateResponseRequest) (*adapter.CreateResponseResponse, error)

	// CreateEmbeddingsWithAppContext handles embeddings creation requests with app context
	CreateEmbeddingsWithAppContext(ctx context.Context, apiKey *apikeymodel.TenantAPIKey, appCtx *AppContext, req *adapter.EmbeddingsRequest) (*adapter.EmbeddingsResponse, error)

	// CreateImageWithAppContext handles image generation requests with app context
	CreateImageWithAppContext(ctx context.Context, apiKey *apikeymodel.TenantAPIKey, appCtx *AppContext, req *adapter.ImageRequest) (*adapter.ImageResponse, error)

	// RerankWithAppContext handles rerank requests with app context
	RerankWithAppContext(ctx context.Context, apiKey *apikeymodel.TenantAPIKey, appCtx *AppContext, req *adapter.RerankRequest) (*adapter.RerankResponse, error)

	// SetConfigCache sets the optional configuration cache for performance optimization
	// This does not affect billing accuracy - only caches read-only configuration data
	SetConfigCache(cache *ConfigCache)

	// SetChannelProvider sets the platform channel provider for official channels
	SetChannelProvider(provider interface{})
}

type llmGatewayServiceImpl struct {
	db             *gorm.DB
	apiKeyRepo     apikeyrepo.APIKeyRepository
	adapterFactory adapter.AdapterFactory
	tokenEstimator *TokenEstimator
	pricingEngine  PricingEngine
	channelRouter  *ChannelRouter // V2 router using llm_routes
	billing        BillingProvider
	// localBilling is always the local billing implementation. We use it for private
	// channels (self-paid / direct upstream) so they don't go through console billing.
	localBilling          BillingProvider
	healthTracker         *ChannelHealthTracker
	configCache           *ConfigCache // Optional: for caching Model/Provider/ShadowTenant
	cryptoService         shared.CryptoService
	consoleProvider       pconsole.ConsoleProvider // Console provider for official channels
	officialCreditChecker paymentservice.OfficialCreditChecker
	policyPrompt          llmPolicyPromptInjector
}

func (s *llmGatewayServiceImpl) isModelRoutable(ctx context.Context, organizationID uuid.UUID, modelName string) (bool, error) {
	if s.channelRouter == nil {
		return false, nil
	}

	routes, err := s.channelRouter.CandidateRoutesForModel(ctx, organizationID, modelName, 1)
	if err != nil {
		if errors.Is(err, llmerrors.DomainErrModelNotFound) {
			return false, nil
		}
		return false, err
	}

	return len(routes) > 0, nil
}

// NewLLMGatewayService creates a new LLM gateway service
func NewLLMGatewayService(
	db *gorm.DB,
	apiKeyRepo apikeyrepo.APIKeyRepository,
	adapterFactory adapter.AdapterFactory,
) (LLMGatewayService, error) {
	// Use default crypto service
	cryptoService, _ := shared.DefaultCryptoService()
	return NewLLMGatewayServiceWithCrypto(db, apiKeyRepo, adapterFactory, cryptoService)
}

// NewLLMGatewayServiceWithCrypto creates a new LLM gateway service with custom crypto service
func NewLLMGatewayServiceWithCrypto(
	db *gorm.DB,
	apiKeyRepo apikeyrepo.APIKeyRepository,
	adapterFactory adapter.AdapterFactory,
	cryptoService shared.CryptoService,
) (LLMGatewayService, error) {
	healthTracker := NewChannelHealthTracker(db)
	// Start cleanup routine in background
	go healthTracker.StartCleanupRoutine(context.Background())

	// Initialize AI credit repositories
	creditAccountRepo := paymentRepo.NewGroupAICreditAccountRepository(db)
	creditTxRepo := paymentRepo.NewTransactionRepository(db)

	// Initialize V2 channel router
	var channelRouter *ChannelRouter
	if cryptoService != nil {
		logger.Info("llm gateway initializing channel router")
		privateModels := llmmodelsvc.NewPrivateModelLookupService(llmmodelrepo.NewCustomModelRepository(db))
		channelRouter = NewChannelRouter(db, cryptoService, privateModels)
	} else {
		logger.Warn("llm gateway channel router not initialized", "reason", "crypto_service_nil")
	}

	// Get Console provider from platform container
	// This will be Remote (Cloud) or Standalone (Self-Hosted) based on ZGI_EDITION
	cfg := appconfig.Current()
	cloudMode := cfg.Platform.Edition == "CLOUD"
	platformContainer, err := platform.NewContainer(db)
	if err != nil {
		if cloudMode {
			return nil, fmt.Errorf("failed to initialize platform container in CLOUD mode: %w", err)
		}
		logger.Warn("llm gateway platform container fallback to standalone", err)
		// Non-CLOUD fallback to standalone console.
		platformContainer = &platform.Container{
			Console: pconsole.NewStandalone(),
		}
	}
	logger.Info("llm gateway console provider initialized", "mode", platformContainer.Console.GetMode())

	// Create local billing service (always needed for CalculateCreditsFromTokens)
	localBilling := NewBillingService(db, apiKeyRepo, creditAccountRepo, creditTxRepo)
	localBilling.StartLocalPredeductRecoveryWorker(context.Background())

	// Select billing provider based on platform mode:
	// - CLOUD: PreDeduct/Settle go via gRPC to console-api (billing authority)
	// - SELF_HOSTED: all billing is local
	var billing BillingProvider = localBilling
	if platformContainer.Console.GetMode() == "CLOUD" {
		grpcAddr := cfg.Console.GRPCAddr
		if grpcAddr == "" {
			grpcAddr = "localhost:50051"
		}
		remoteBilling, err := NewRemoteBilling(grpcAddr, localBilling)
		if err != nil {
			return nil, fmt.Errorf("cloud mode requires remote billing at %s: %w", grpcAddr, err)
		}

		billing = remoteBilling
		logger.Info("llm gateway remote billing enabled", "grpc_addr", grpcAddr)
	}

	return &llmGatewayServiceImpl{
		db:                    db,
		apiKeyRepo:            apiKeyRepo,
		adapterFactory:        adapterFactory,
		tokenEstimator:        NewTokenEstimator(),
		pricingEngine:         NewPricingEngine(db),
		channelRouter:         channelRouter,
		billing:               billing,
		localBilling:          localBilling,
		healthTracker:         healthTracker,
		cryptoService:         cryptoService,
		consoleProvider:       platformContainer.Console,
		officialCreditChecker: paymentservice.NewConsoleOfficialCreditChecker(),
		policyPrompt:          newLLMPolicyPromptInjector(cfg.LLMPolicyPrompt),
	}, nil
}

// SetConfigCache sets the optional configuration cache
// This enables caching for Model/Provider/ShadowTenant lookups
// Can be called after service creation to enable caching
func (s *llmGatewayServiceImpl) SetConfigCache(cache *ConfigCache) {
	s.configCache = cache
	if s.channelRouter != nil {
		s.channelRouter.SetConfigCache(cache)
	}
}

// SetChannelProvider sets the platform channel provider
func (s *llmGatewayServiceImpl) SetChannelProvider(provider interface{}) {
	if s.channelRouter != nil {
		if p, ok := provider.(pchannel.ChannelProvider); ok {
			s.channelRouter.SetChannelProvider(p)
		}
	}
}

// getShadowTenantInfo retrieves shadow tenant info, using cache if available
func (s *llmGatewayServiceImpl) getShadowTenantInfo(ctx context.Context, organizationID uuid.UUID) (shadowOrganizationID uuid.UUID, ownerID uuid.UUID, err error) {
	if s.configCache != nil {
		info, err := s.configCache.GetShadowTenantInfo(ctx, organizationID)
		if err != nil {
			return uuid.Nil, uuid.Nil, err
		}
		return info.ShadowOrganizationID, info.OwnerID, nil
	}

	// No cache, use direct queries (original behavior)
	shadowOrganizationID, err = GetShadowOrganizationID(ctx, s.db, organizationID)
	if err != nil {
		return uuid.Nil, uuid.Nil, err
	}

	ownerID, err = GetShadowTenantOwnerID(ctx, s.db, shadowOrganizationID)
	if err != nil {
		// Owner not found is not fatal, account will be created on demand
		ownerID = uuid.Nil
	}

	return shadowOrganizationID, ownerID, nil
}

// selectProvidersWithChannelRouter uses the V2 ChannelRouter only.
func (s *llmGatewayServiceImpl) selectProvidersWithChannelRouter(
	ctx context.Context,
	organizationID uuid.UUID,
	providerHint string,
	modelName string,
	maxSelections int,
) ([]*ProviderSelection, error) {
	logCtx := logger.WithFields(ctx,
		zap.String("organization_id", organizationID.String()),
		zap.String("provider_hint", providerHint),
		zap.String("model", modelName),
		zap.Int("max_selections", maxSelections),
	)
	logger.DebugContext(logCtx, "selecting LLM providers with channel router")

	if s.channelRouter == nil {
		return nil, fmt.Errorf("no channel selections available for model '%s' (tenant: %s)", modelName, organizationID)
	}

	channelSelections, err := s.channelRouter.SelectChannelsForProvider(ctx, organizationID, providerHint, modelName, maxSelections)
	if err != nil {
		return nil, err
	}

	if len(channelSelections) == 0 {
		return nil, fmt.Errorf("no channel selections available for model '%s' (tenant: %s)", modelName, organizationID)
	}

	selections := make([]*ProviderSelection, 0, len(channelSelections))
	conversionErrors := make([]string, 0)

	for _, cs := range channelSelections {
		ps, err := cs.ConvertToProviderSelectionWithCache(ctx, s.db, s.configCache)
		if err != nil {
			conversionErrors = append(conversionErrors,
				fmt.Sprintf("routeID=%s channel_provider=%s model=%s err=%v", cs.RouteID, cs.ChannelProvider, modelName, err))

			logger.WarnContext(logCtx, "failed to convert LLM channel selection to provider selection",
				err,
				zap.String("route_id", cs.RouteID.String()),
				zap.String("channel_provider", cs.ChannelProvider),
			)

			continue
		}
		selections = append(selections, ps)
	}

	if len(selections) > 0 {
		return selections, nil
	}
	if len(conversionErrors) > 0 {
		return nil, fmt.Errorf("failed to convert channel selections: %v", conversionErrors)
	}

	return nil, fmt.Errorf("no channel selections available for model '%s' (tenant: %s)", modelName, organizationID)
}

// CreateResponse handles response creation requests
func (s *llmGatewayServiceImpl) CreateResponse(
	ctx context.Context,
	apiKey *apikeymodel.TenantAPIKey,
	req *adapter.CreateResponseRequest,
) (*adapter.CreateResponseResponse, error) {
	return s.createResponseInternal(ctx, apiKey, nil, req)
}

// CreateEmbeddings handles embeddings creation requests
func (s *llmGatewayServiceImpl) CreateEmbeddings(
	ctx context.Context,
	apiKey *apikeymodel.TenantAPIKey,
	req *adapter.EmbeddingsRequest,
) (*adapter.EmbeddingsResponse, error) {
	return s.createEmbeddingsInternal(ctx, apiKey, nil, req)
}

// CreateImage handles image generation requests
func (s *llmGatewayServiceImpl) CreateImage(
	ctx context.Context,
	apiKey *apikeymodel.TenantAPIKey,
	req *adapter.ImageRequest,
) (*adapter.ImageResponse, error) {
	return s.createImageInternal(ctx, apiKey, nil, req)
}

// Rerank handles rerank requests
func (s *llmGatewayServiceImpl) Rerank(
	ctx context.Context,
	apiKey *apikeymodel.TenantAPIKey,
	req *adapter.RerankRequest,
) (*adapter.RerankResponse, error) {
	return s.rerankInternal(ctx, apiKey, nil, req)
}

// handleStreamBilling handles billing for streaming responses
func (s *llmGatewayServiceImpl) handleStreamBilling(
	ctx context.Context,
	inputChan <-chan adapter.StreamResponse,
	outputChan chan<- adapter.StreamResponse,
	billingCtx *BillingContext,
	req *adapter.ChatRequest,
	llmModel *llmmodel.LLMModel,
	startTime time.Time,
	channelID *uuid.UUID,
) {
	defer close(outputChan)

	// Use estimated tokens from billingCtx as initial values
	// These were calculated during pre-deduct phase
	totalPromptTokens := billingCtx.PromptTokens
	totalCompletionTokens := billingCtx.CompletionTokens
	sawUsage := false
	var settlement *adapter.SettlementResult
	var lastError error
	var doneResponse *adapter.StreamResponse

	// Collect all chunks for OpenTelemetry tracing.
	// Use strings.Builder for memory efficiency with large responses
	var collectedChunks strings.Builder

	decision, laneErr := s.resolveBillingDecision(nil, billingCtx)
	if laneErr != nil {
		wrappedLaneErr := wrapBillingLaneMismatchError(laneErr)
		outputChan <- adapter.StreamResponse{Error: wrappedLaneErr}
		return
	}
	useSystemProvider := decision.UseSystemProvider
	routeID := decision.RouteID
	if routeID == "" {
		routeID = routeIDString(channelID)
	}

	for response := range inputChan {
		// Extract token usage from response (usually in the last chunk with Done=true)
		if response.Usage != nil {
			if hasBillableTokenUsage(response.Usage) {
				sawUsage = true
				totalPromptTokens = response.Usage.PromptTokens
				totalCompletionTokens = response.Usage.CompletionTokens
			}
		}
		if response.Settlement != nil {
			settlement = response.Settlement
		}

		// Collect text chunks for tracing (non-blocking, best effort)
		if response.Choices != nil && len(response.Choices) > 0 {
			if content, ok := response.Choices[0].Delta.Content.(string); ok && content != "" {
				collectedChunks.WriteString(content)
			}
		}

		// Check for errors
		if response.Error != nil {
			lastError = response.Error
			outputChan <- response
			break
		}

		// Track completion (last message should have Done = true)
		if response.Done {
			resp := response
			doneResponse = &resp
			break
		}

		// Forward non-terminal response to output
		outputChan <- response
	}

	responseTime := time.Since(startTime).Milliseconds()

	// Settle billing
	if lastError != nil {
		billingCtx.Status = billingContextStatusError
		billingCtx.ErrorMessage = lastError.Error()
		billingCtx.PromptTokens = 0
		billingCtx.CompletionTokens = 0
		billingCtx.TotalTokens = 0
		billingCtx.ActualCredits = 0
		billingCtx.TotalCost = decimal.Zero

		// Record failure for health tracking
		if channelID != nil {
			autoBan := billingCtx.ChannelID != nil
			s.healthTracker.RecordFailure(ctx, *channelID, autoBan)
		}
	} else {
		if !sawUsage && settlement == nil {
			modelName := ""
			if llmModel != nil {
				modelName = llmModel.Model
			}
			missingUsageErr := missingTokenUsageError("", modelName)
			billingCtx.Status = billingContextStatusError
			if !useSystemProvider {
				billingCtx.Status = billingContextStatusPartial
			}
			billingCtx.ErrorMessage = missingUsageErr.Error()
			billingCtx.PromptTokens = 0
			billingCtx.CompletionTokens = 0
			billingCtx.TotalTokens = 0
			billingCtx.ActualCredits = 0
			billingCtx.InputUSD = decimal.Zero
			billingCtx.OutputUSD = decimal.Zero
			billingCtx.TotalUSD = decimal.Zero
			billingCtx.InputCost = decimal.Zero
			billingCtx.OutputCost = decimal.Zero
			billingCtx.TotalCost = decimal.Zero
			billingCtx.ResponseTime = responseTime

			if channelID != nil {
				autoBan := billingCtx.ChannelID != nil
				s.healthTracker.RecordFailure(ctx, *channelID, autoBan)
			}

			if err := s.billingProviderForDecision(decision).Settle(ctx, billingCtx); err != nil {
				wrappedErr := wrapBillingSettleError(err, billingCtx, useSystemProvider, routeID)
				logBillingEvent(
					billingCode("BILLING_SETTLE_FAILED", useSystemProvider),
					billingCtx,
					routeID,
					useSystemProvider,
					"settle",
					"error",
					err,
				)
				outputChan <- adapter.StreamResponse{Error: wrappedErr}
				return
			}
			logBillingEvent(
				billingCode("BILLING_SETTLE_OK", useSystemProvider),
				billingCtx,
				routeID,
				useSystemProvider,
				"settle",
				"ok",
				nil,
			)

			endTime := time.Now()
			s.traceStreamingChatCompletion(ctx, req, collectedChunks.String(), startTime, endTime, billingCtx, 0, 0, missingUsageErr)
			outputChan <- adapter.StreamResponse{Error: missingUsageErr}
			return
		}

		// Use estimated tokens (already set from billingCtx)
		billingCtx.PromptTokens = totalPromptTokens
		billingCtx.CompletionTokens = totalCompletionTokens
		billingCtx.TotalTokens = totalPromptTokens + totalCompletionTokens

		billingCtx.Status = billingContextStatusSuccess

		if !useSystemProvider {
			quote, err := s.quoteTokenPricingForSettlement(ctx, billingCtx, pricingModelRefFromBillingContext(billingCtx), billingCtx.PromptTokens, billingCtx.CompletionTokens)
			if err != nil {
				outputChan <- adapter.StreamResponse{Error: fmt.Errorf("failed to calculate credits: %w", err)}
				return
			}

			billingCtx.ActualCredits = quote.TotalCredits
			applyPricingQuoteToBillingContext(billingCtx, quote)
		}

		// Record success for health tracking
		if channelID != nil {
			s.healthTracker.RecordSuccess(*channelID)
		}
	}

	billingCtx.ResponseTime = responseTime

	var settleErr error
	if useSystemProvider {
		settleErr = s.finalizePlatformProxySettlement(ctx, billingCtx, settlement, decision)
	} else {
		settleErr = s.billingProviderForDecision(decision).Settle(ctx, billingCtx)
	}
	if settleErr != nil {
		wrappedErr := wrapBillingSettleError(settleErr, billingCtx, useSystemProvider, routeID)
		logBillingEvent(
			billingCode("BILLING_SETTLE_FAILED", useSystemProvider),
			billingCtx,
			routeID,
			useSystemProvider,
			"settle",
			"error",
			settleErr,
		)
		if lastError == nil {
			outputChan <- adapter.StreamResponse{Error: wrappedErr}
		}
		return
	}
	logBillingEvent(
		billingCode("BILLING_SETTLE_OK", useSystemProvider),
		billingCtx,
		routeID,
		useSystemProvider,
		"settle",
		"ok",
		nil,
	)

	// Trace to OpenTelemetry after billing has final usage and cost details.
	endTime := time.Now()
	s.traceStreamingChatCompletion(ctx, req, collectedChunks.String(), startTime, endTime, billingCtx, totalPromptTokens, totalCompletionTokens, lastError)

	if lastError == nil && doneResponse != nil {
		outputChan <- *doneResponse
	}
}

// ListAvailableModels lists available models for the API key
func (s *llmGatewayServiceImpl) ListAvailableModels(
	ctx context.Context,
	apiKey *apikeymodel.TenantAPIKey,
) ([]adapter.Model, error) {
	var models []llmmodel.LLMModel

	// Check if model limits are enabled
	if apiKey.ModelLimitsEnabled && apiKey.ModelLimits != nil && *apiKey.ModelLimits != "" {
		// Parse model limits from JSON
		var modelNames []string
		if err := json.Unmarshal([]byte(*apiKey.ModelLimits), &modelNames); err != nil {
			return nil, fmt.Errorf("failed to parse model limits: %w", err)
		}

		if len(modelNames) == 0 {
			return []adapter.Model{}, nil
		}

		// Get models from llm_models table by model names
		if err := s.db.WithContext(ctx).
			Where("name IN ? AND is_active = ? AND deleted_at IS NULL", modelNames, true).
			Find(&models).Error; err != nil {
			return nil, fmt.Errorf("failed to list limited models: %w", err)
		}
	} else {
		// No model limits, get models from tenant's enterprise group shadow tenant
		// First, check if tenant is a shadow tenant (enterprise group ID)
		var enterpriseGroup struct {
			ID string `gorm:"column:id"`
		}

		err := s.db.WithContext(ctx).Table("organizations").
			Where("id = ?", apiKey.OrganizationID).
			First(&enterpriseGroup).Error

		shadowOrganizationID := apiKey.OrganizationID

		if err != nil {
			// Not a shadow tenant, find the enterprise group for this tenant
			var workspace struct {
				OrganizationID *string `gorm:"column:organization_id"`
			}

			err = s.db.WithContext(ctx).Table("workspaces").
				Select("organization_id").
				Where("id = ?", apiKey.OrganizationID).
				First(&workspace).Error

			if err == nil && workspace.OrganizationID != nil && *workspace.OrganizationID != "" {
				shadowOrganizationID = *workspace.OrganizationID
			}
			// If no enterprise group found, use the original tenant ID
		}

		// Get enabled models from llm_tenant_model_configs for the shadow tenant
		// Uses the unified table with UUID model_id instead of string-based provider+model
		var modelConfigs []llmmodel.ModelConfig
		if err := s.db.WithContext(ctx).
			Preload("Model").
			Where("organization_id = ? AND is_enabled = ? AND deleted_at IS NULL", shadowOrganizationID, true).
			Find(&modelConfigs).Error; err != nil {
			return nil, fmt.Errorf("failed to list tenant model configs: %w", err)
		}

		if len(modelConfigs) == 0 {
			return []adapter.Model{}, nil
		}

		// Build models list from configs with preloaded Model relation
		models = make([]llmmodel.LLMModel, 0, len(modelConfigs))
		for _, cfg := range modelConfigs {
			if cfg.Model != nil && cfg.Model.IsActive && cfg.Model.DeletedAt.Time.IsZero() {
				models = append(models, *cfg.Model)
			}
		}
	}

	organizationID, err := uuid.Parse(apiKey.OrganizationID)
	if err != nil {
		return nil, fmt.Errorf("invalid organization id %q: %w", apiKey.OrganizationID, err)
	}

	// Convert to adapter models
	result := make([]adapter.Model, 0, len(models))
	for _, m := range models {
		routable, err := s.isModelRoutable(ctx, organizationID, m.Model)
		if err != nil {
			return nil, fmt.Errorf("failed to evaluate routable model %s: %w", m.Model, err)
		}
		if !routable {
			continue
		}

		result = append(result, adapter.Model{
			ID:            m.Model,
			Name:          m.ModelName,
			Description:   "",
			ContextLength: m.ContextWindow,
			Created:       m.CreatedAt.Unix(),
			OwnedBy:       m.Provider,
		})
	}

	return result, nil
}

// validateRequest validates the chat request
func (s *llmGatewayServiceImpl) validateRequest(req *adapter.ChatRequest) error {
	req.Model = normalizeRequestedModelName(req.Model)
	if req.Model == "" {
		return ErrMissingModel
	}
	if req.Messages == nil {
		return ErrMissingMessages
	}
	if len(req.Messages) == 0 {
		return ErrEmptyMessages
	}
	return nil
}

// validateCreateResponseRequest validates the create response request
func (s *llmGatewayServiceImpl) validateCreateResponseRequest(req *adapter.CreateResponseRequest) error {
	req.Model = normalizeRequestedModelName(req.Model)
	if req.Model == "" {
		return ErrMissingModel
	}
	// Responses API might have optional messages or other inputs, but for now let's check model
	// If Messages are nil, it might be using another modality, so we might be less strict or check other fields
	// But for now, let's just ensure Model is present.
	return nil
}

// validateEmbeddingsRequest validates the embeddings request
func (s *llmGatewayServiceImpl) validateEmbeddingsRequest(req *adapter.EmbeddingsRequest) error {
	req.Model = normalizeRequestedModelName(req.Model)
	if req.Model == "" {
		return ErrMissingModel
	}
	if req.Input == nil {
		return fmt.Errorf("input is required")
	}
	return nil
}

// validateRerankRequest validates the rerank request
func (s *llmGatewayServiceImpl) validateRerankRequest(req *adapter.RerankRequest) error {
	req.Model = normalizeRequestedModelName(req.Model)
	if req.Model == "" {
		return ErrMissingModel
	}
	if req.Query == "" {
		return fmt.Errorf("query is required")
	}
	if req.Documents == nil {
		return fmt.Errorf("documents are required")
	}
	return nil
}

// createAdapterConfig creates adapter configuration from provider selection
func (s *llmGatewayServiceImpl) createAdapterConfig(selection *ProviderSelection, organizationID ...uuid.UUID) *adapter.AdapterConfig {
	adapterProvider := selection.ChannelProvider
	if spec, err := channelprovider.Resolve(selection.ChannelProvider); err == nil {
		adapterProvider = spec.AdapterKey
	}
	llmConfig := appconfig.Current().LLM

	config := &adapter.AdapterConfig{
		ProviderName:        adapterProvider,
		ProviderID:          selection.Provider.ID.String(),
		BaseURL:             selection.Provider.APIBaseURL,
		Timeout:             500 * time.Second,
		MaxRetries:          3,
		GuardOutboundURL:    llmConfig.OutboundURLGuardEnabled(),
		GuardOutboundDNS:    llmConfig.GuardOutboundDNS,
		AllowPrivateBaseURL: selection.UseSystemProvider || channelprovider.AllowsPrivateBaseURL(selection.ChannelProvider),
	}

	// In V2 architecture, API key is already decrypted and passed through ProviderSelection
	if selection.APIKey != "" {
		config.APIKey = selection.APIKey
	}

	// Use channel's custom base URL if configured
	if selection.APIBaseURL != "" {
		config.BaseURL = selection.APIBaseURL
	}

	if selection.UseSystemProvider {
		// Official routes are a dedicated console transport, not a direct upstream provider adapter.
		config.ProviderName = "zgi-cloud"
		config.ProviderID = ""
	}

	// Official channels use HMAC signing instead of Bearer API key
	if selection.UseSystemProvider && len(organizationID) > 0 {
		config.AuthHook = s.buildConsoleAuthHook(organizationID[0])
	} else if selection.UseSystemProvider {
		config.AuthHook = s.buildConsoleAuthHook(uuid.Nil)
	}

	return config
}

// buildConsoleAuthHook returns an AuthHook that signs requests with HMAC-SHA256
// for console-api internal endpoints.
func (s *llmGatewayServiceImpl) buildConsoleAuthHook(organizationID uuid.UUID) func(req *http.Request) {
	internalAPIKey := appconfig.Current().Console.InternalAPIKey
	if internalAPIKey == "" {
		return nil
	}
	return func(req *http.Request) {
		signConsoleRequest(req, internalAPIKey)
		if organizationID != uuid.Nil {
			req.Header.Set("X-Organization-ID", organizationID.String())
		}
		applyPlatformProxyHeaders(req)
	}
}

// signConsoleRequest adds HMAC-SHA256 authentication headers to an outgoing request.
func signConsoleRequest(req *http.Request, internalAPIKey string) {
	ts := fmt.Sprintf("%d", time.Now().Unix())
	message := ts + "|" + req.URL.Path
	mac := hmac.New(sha256.New, []byte(internalAPIKey))
	mac.Write([]byte(message))
	sig := hex.EncodeToString(mac.Sum(nil))
	req.Header.Set("X-Internal-Timestamp", ts)
	req.Header.Set("X-Internal-Signature", sig)
}

// rollbackPreDeduction releases a pre-deducted reservation after a failed provider attempt.
func (s *llmGatewayServiceImpl) rollbackPreDeduction(ctx context.Context, billingCtx *BillingContext) error {
	if billingCtx == nil {
		return errors.New("billing context is nil")
	}
	billingCtx.TotalCost = decimal.Zero
	billingCtx.Status = "error"

	decision, laneErr := s.resolveBillingDecision(nil, billingCtx)
	if laneErr != nil {
		return wrapBillingLaneMismatchError(laneErr)
	}

	billingCtxForSettle, cancel := billingFinalizationContext(ctx)
	defer cancel()
	if err := s.billingProviderForDecision(decision).Settle(billingCtxForSettle, billingCtx); err != nil {
		return wrapBillingSettleError(
			err,
			billingCtx,
			decision.UseSystemProvider,
			decision.RouteID,
		)
	}
	return nil
}

// logProviderError logs structured error information for provider failures
func (s *llmGatewayServiceImpl) logProviderError(
	ctx context.Context,
	attemptIdx int,
	selection *ProviderSelection,
	err error,
	errorType string,
) {
	channelInfo := "system_provider"
	if selection.HasRoute() {
		channelInfo = fmt.Sprintf("channel_%s", selection.RouteID.String())
	}

	logger.ErrorContext(ctx, "llm provider attempt failed",
		"attempt", attemptIdx+1,
		"error_type", errorType,
		"provider", selection.Provider.Provider,
		"channel", channelInfo,
		"model", selection.Model.Model,
		err,
	)
}
