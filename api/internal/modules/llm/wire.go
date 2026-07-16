package llm

import (
	"context"

	"github.com/zgiai/zgi/api/config"
	"github.com/zgiai/zgi/api/internal/infra/platform"
	pconsole "github.com/zgiai/zgi/api/internal/infra/platform/console"
	"github.com/zgiai/zgi/api/internal/modules/llm/apikey"
	apikeyhandler "github.com/zgiai/zgi/api/internal/modules/llm/apikey/handler"
	apikeyrepo "github.com/zgiai/zgi/api/internal/modules/llm/apikey/repository"
	apikeysvc "github.com/zgiai/zgi/api/internal/modules/llm/apikey/service"
	"github.com/zgiai/zgi/api/internal/modules/llm/availability"
	availhandler "github.com/zgiai/zgi/api/internal/modules/llm/availability/handler"
	"github.com/zgiai/zgi/api/internal/modules/llm/catalogsync"
	"github.com/zgiai/zgi/api/internal/modules/llm/channel"
	channelhandler "github.com/zgiai/zgi/api/internal/modules/llm/channel/handler"
	channelrepo "github.com/zgiai/zgi/api/internal/modules/llm/channel/repository"
	channelsvc "github.com/zgiai/zgi/api/internal/modules/llm/channel/service"
	"github.com/zgiai/zgi/api/internal/modules/llm/credential"
	credentialhandler "github.com/zgiai/zgi/api/internal/modules/llm/credential/handler"
	credentialrepo "github.com/zgiai/zgi/api/internal/modules/llm/credential/repository"
	credentialsvc "github.com/zgiai/zgi/api/internal/modules/llm/credential/service"
	"github.com/zgiai/zgi/api/internal/modules/llm/credential/upstreamstate"
	"github.com/zgiai/zgi/api/internal/modules/llm/defaultmodel"
	defaultmodelhandler "github.com/zgiai/zgi/api/internal/modules/llm/defaultmodel/handler"
	defaultmodelsvc "github.com/zgiai/zgi/api/internal/modules/llm/defaultmodel/service"
	"github.com/zgiai/zgi/api/internal/modules/llm/gateway"
	"github.com/zgiai/zgi/api/internal/modules/llm/llmmodel"
	llmmodelhandler "github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/handler"
	llmmodelrepo "github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/repository"
	llmmodelsvc "github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/service"
	"github.com/zgiai/zgi/api/internal/modules/llm/modelmeta"
	officialmodel "github.com/zgiai/zgi/api/internal/modules/llm/officialmodel"
	"github.com/zgiai/zgi/api/internal/modules/llm/provider"
	providerhandler "github.com/zgiai/zgi/api/internal/modules/llm/provider/handler"
	providerrepo "github.com/zgiai/zgi/api/internal/modules/llm/provider/repository"
	providersvc "github.com/zgiai/zgi/api/internal/modules/llm/provider/service"
	"github.com/zgiai/zgi/api/internal/modules/llm/shared"
	"github.com/zgiai/zgi/api/internal/modules/llm/statistics"
	statisticshandler "github.com/zgiai/zgi/api/internal/modules/llm/statistics/handler"
	"github.com/zgiai/zgi/api/internal/modules/llm/workspacequota"
	workspacequotahandler "github.com/zgiai/zgi/api/internal/modules/llm/workspacequota/handler"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	"gorm.io/gorm"
)

var ensureOfficialModelSyncStarted = func(ctx context.Context, db *gorm.DB, edition string, grpcAddr string) {
	officialmodel.EnsureSynchronizerStarted(ctx, db, edition, grpcAddr)
}

var ensureCatalogSyncStarted = func(ctx context.Context, db *gorm.DB, edition string, grpcAddr string) {
	catalogsync.EnsureSynchronizerStarted(ctx, db, edition, grpcAddr)
}

var ensureLocalModelMetaSyncStarted = func(ctx context.Context, db *gorm.DB, edition string) {
	modelmeta.EnsureLocalBootstrapStarted(ctx, db, edition)
}

// LLMModule holds all LLM module dependencies
type LLMModule struct {
	// Database
	DB *gorm.DB

	// IsCloudMode indicates whether the system is running in Cloud mode (ZGI_RUN_MODE=cloud)
	IsCloudMode bool

	// Repositories
	TenantCredentialRepo credentialrepo.TenantCredentialRepository
	ProviderRepo         providerrepo.ProviderRepository
	ProviderCfgRepo      providerrepo.ProviderConfigRepository
	CustomProvRepo       providerrepo.CustomProviderRepository
	ModelRepo            llmmodelrepo.ModelRepository
	ModelCfgRepo         llmmodelrepo.ModelConfigRepository
	CustomModelRepo      llmmodelrepo.CustomModelRepository
	// No system-channel repository in the provider-first channel design.
	TenantRouteRepo channelrepo.TenantRouteRepository
	APIKeyRepo      apikeyrepo.APIKeyRepository

	// Services
	TenantCredentialSvc credentialsvc.TenantCredentialService
	ProviderSvc         providersvc.ProviderService
	ModelSvc            llmmodelsvc.ModelService
	DefaultModelSvc     defaultmodelsvc.DefaultModelService
	ChannelSvc          channelsvc.ChannelService
	APIKeySvc           apikeysvc.APIKeyService
	UpstreamStateSvc    *upstreamstate.Service

	// Handlers
	TenantCredentialHandler *credentialhandler.TenantCredentialHandler
	ProviderHandler         *providerhandler.ProviderHandler
	ModelHandler            *llmmodelhandler.ModelHandler
	DefaultModelHandler     *defaultmodelhandler.Handler
	ChannelHandler          *channelhandler.ChannelHandler
	APIKeyHandler           *apikeyhandler.APIKeyHandler

	// Gateway
	GatewayRouter          *gateway.ChannelRouter
	PricingFallbackHandler *gateway.PricingFallbackHandler

	// Modules (for convenience)
	ProviderModule       *provider.Module
	LLMModelModule       *llmmodel.Module
	DefaultModelModule   *defaultmodel.Module
	CredentialModule     *credential.Module
	APIKeyModule         *apikey.Module
	StatisticsModule     *statistics.Module
	WorkspaceQuotaModule *workspacequota.Module
	AvailabilityModule   *availability.Module

	// Handlers for new modules
	StatisticsHandler     *statisticshandler.StatisticsHandler
	WorkspaceQuotaHandler *workspacequotahandler.WorkspaceQuotaHandler
	AvailabilityHandler   *availhandler.AvailabilityHandler
	ModelMetaHandler      *modelmeta.Handler
}

// NewLLMModule creates a new LLM module with all dependencies wired
func NewLLMModule(db *gorm.DB, crypto shared.CryptoService, tenantService interfaces.WorkspaceManagementService, accountService interfaces.AccountService, enterpriseService interfaces.OrganizationService, cp pconsole.ConsoleProvider) *LLMModule {
	m := &LLMModule{
		DB: db,
	}

	// Initialize Provider Module (new modular structure)
	m.ProviderModule = provider.NewModule(db)
	m.ProviderRepo = m.ProviderModule.GlobalRepo
	m.ProviderCfgRepo = m.ProviderModule.ConfigRepo
	m.CustomProvRepo = m.ProviderModule.CustomRepo
	m.ProviderSvc = m.ProviderModule.Service
	m.ProviderHandler = m.ProviderModule.Handler

	// Initialize LLMModel Module (new modular structure)
	m.LLMModelModule = llmmodel.NewModule(db)
	m.ModelRepo = m.LLMModelModule.GlobalRepo
	m.ModelCfgRepo = m.LLMModelModule.ConfigRepo
	m.CustomModelRepo = m.LLMModelModule.CustomRepo
	m.ModelSvc = m.LLMModelModule.Service
	m.ModelHandler = m.LLMModelModule.Handler
	m.ProviderSvc.SetAvailableModelsService(m.LLMModelModule.AvailableModelsSvc)
	m.DefaultModelModule = defaultmodel.NewModule(db, m.LLMModelModule.AvailableModelsSvc, m.LLMModelModule.GlobalRepo, m.LLMModelModule.CustomRepo)
	m.DefaultModelSvc = m.DefaultModelModule.Service
	m.DefaultModelHandler = m.DefaultModelModule.Handler

	// Initialize Credential Module (new modular structure)
	m.CredentialModule = credential.NewModule(db, crypto)
	m.TenantCredentialRepo = m.CredentialModule.TenantRepo
	m.TenantCredentialSvc = m.CredentialModule.TenantSvc
	m.TenantCredentialHandler = m.CredentialModule.TenantHandler

	// Initialize Channel Module (new modular structure)
	channelModule := channel.NewModule(
		db,
		m.CredentialModule.TenantSvc,
		nil,
		m.LLMModelModule.GlobalRepo,
		m.LLMModelModule.ConfigRepo,
		m.CustomProvRepo,
		m.CustomModelRepo,
		m.LLMModelModule.PrivateModelLookupSvc,
		m.LLMModelModule.AvailableModelsSvc,
		crypto,
		cp,
	)
	m.TenantRouteRepo = channelModule.TenantRouteRepo
	m.ChannelSvc = channelModule.Service
	m.ChannelHandler = channelModule.Handler
	m.UpstreamStateSvc = channelModule.UpstreamState

	// Initialize APIKey Module (requires TenantService, AccountService and EnterpriseService)
	if tenantService != nil && accountService != nil && enterpriseService != nil {
		m.APIKeyModule = apikey.NewModule(db, tenantService, accountService, enterpriseService)
		m.APIKeyRepo = m.APIKeyModule.Repository
		m.APIKeySvc = m.APIKeyModule.Service
		m.APIKeyHandler = m.APIKeyModule.Handler
	}

	// Initialize Statistics Module
	m.StatisticsModule = statistics.NewModule(db)
	m.StatisticsHandler = m.StatisticsModule.Handler

	// Initialize Workspace Quota Module
	m.WorkspaceQuotaModule = workspacequota.NewModule(db, enterpriseService)
	m.WorkspaceQuotaHandler = m.WorkspaceQuotaModule.Handler

	// Initialize Availability Module
	m.AvailabilityModule = availability.NewModule(m.ModelRepo, m.ModelCfgRepo, m.TenantRouteRepo, m.ProviderModule.GlobalRepo, m.ProviderModule.ConfigRepo)
	m.AvailabilityHandler = m.AvailabilityModule.Handler

	// Initialize ModelMeta Module (for syncing model metadata from modelmeta.dev)
	modelMetaSvc := modelmeta.NewService(db)
	m.ModelMetaHandler = modelmeta.NewHandler(modelMetaSvc)

	// Initialize Gateway
	m.GatewayRouter = gateway.NewChannelRouter(db, crypto, m.LLMModelModule.PrivateModelLookupSvc)
	m.PricingFallbackHandler = gateway.NewPricingFallbackHandler(db)

	// Inject PlatformContainer for official channels (Cloud mode)
	platformContainer, err := platform.NewContainer(db)
	if err == nil && platformContainer != nil {
		m.GatewayRouter.SetChannelProvider(platformContainer.Channel)
	}

	appCfg := config.Current()
	// Detect Cloud mode from centralized configuration.
	m.IsCloudMode = appCfg.Platform.Edition == "CLOUD"
	if m.ProviderHandler != nil {
		m.ProviderHandler.SetPlatformCatalogReadOnly(m.IsCloudMode)
	}
	if m.ModelHandler != nil {
		m.ModelHandler.SetPlatformCatalogReadOnly(m.IsCloudMode)
	}
	if m.ModelMetaHandler != nil {
		m.ModelMetaHandler.SetPlatformCatalogReadOnly(m.IsCloudMode)
	}
	if m.IsCloudMode {
		ensureOfficialModelSyncStarted(context.Background(), db, appCfg.Platform.Edition, appCfg.Console.GRPCAddr)
		ensureCatalogSyncStarted(context.Background(), db, appCfg.Platform.Edition, appCfg.Console.GRPCAddr)
	} else {
		ensureLocalModelMetaSyncStarted(context.Background(), db, appCfg.Platform.Edition)
	}

	return m
}
