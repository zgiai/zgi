package contentparse

import (
	"context"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	contentparsecap "github.com/zgiai/zgi/api/internal/capabilities/contentparse"
	hyperparsesdk "github.com/zgiai/zgi/api/internal/capabilities/contentparse/adapters/hyperparse_sdk"
	systemvlm "github.com/zgiai/zgi/api/internal/capabilities/contentparse/adapters/system_vlm"
	"github.com/zgiai/zgi/api/internal/capabilities/contentparse/routing"
	"github.com/zgiai/zgi/api/internal/contracts"
	"github.com/zgiai/zgi/api/internal/modules/contentparse/handler"
	"github.com/zgiai/zgi/api/internal/modules/contentparse/repository"
	"github.com/zgiai/zgi/api/internal/modules/contentparse/service"
	llmclient "github.com/zgiai/zgi/api/internal/modules/llm/client"
	llmdefaultservice "github.com/zgiai/zgi/api/internal/modules/llm/defaultmodel/service"
	llmcrypto "github.com/zgiai/zgi/api/internal/modules/llm/shared/crypto"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	"gorm.io/gorm"
)

type Module struct {
	ProviderConfigRepo repository.ProviderConfigRepository
	RoutePolicyRepo    repository.RoutePolicyRepository
	ProviderHealthRepo repository.ProviderHealthRepository
	ArtifactRepo       repository.ArtifactRepository
	ChunkArtifactRepo  repository.ChunkArtifactSetRepository
	ParseRunRepo       repository.ParseRunRepository
	ChunkingRunRepo    repository.ChunkingRunRepository
	PlaygroundRunRepo  repository.PlaygroundRunRepository

	ProviderAdminService service.ProviderAdminService
	PolicyAdminService   service.PolicyAdminService
	HealthService        service.HealthService
	ArtifactService      service.ArtifactService
	ChunkArtifactService service.ChunkArtifactSetService
	RunQueryService      service.RunQueryService
	PlaygroundRunService service.PlaygroundRunService
	ProviderCatalogs     service.ProviderCatalogResolver
	ProviderSettings     service.ProviderSettingsService

	ContentParseService contracts.ContentParseService
	Orchestrator        *contentparsecap.Orchestrator
	Planner             routing.Planner
	Catalog             *contracts.ParseProviderCatalog

	ProviderHandler   *handler.ProviderHandler
	PolicyHandler     *handler.PolicyHandler
	HealthHandler     *handler.HealthHandler
	ArtifactHandler   *handler.ArtifactHandler
	RunHandler        *handler.RunHandler
	PlaygroundHandler *handler.PlaygroundHandler
	SettingsHandler   *handler.ProviderSettingsHandler
}

type ModuleOption func(*moduleOptions)

type moduleOptions struct {
	llmClient          llmclient.LLMClient
	defaultModelSvc    llmdefaultservice.DefaultModelService
	enableSystemVLM    bool
	systemVLMAvailable bool
	organization       interfaces.OrganizationService
	account            interfaces.AccountService
}

func WithSystemVisionModel(llmClient llmclient.LLMClient, defaultModelSvc llmdefaultservice.DefaultModelService) ModuleOption {
	return func(opts *moduleOptions) {
		opts.llmClient = llmClient
		opts.defaultModelSvc = defaultModelSvc
		opts.enableSystemVLM = true
		opts.systemVLMAvailable = llmClient != nil && defaultModelSvc != nil
	}
}

func WithOrganizationService(service interfaces.OrganizationService) ModuleOption {
	return func(opts *moduleOptions) {
		opts.organization = service
	}
}

func WithAccountService(service interfaces.AccountService) ModuleOption {
	return func(opts *moduleOptions) {
		opts.account = service
	}
}

func NewModule(db *gorm.DB, options ...ModuleOption) *Module {
	opts := moduleOptions{}
	for _, option := range options {
		if option != nil {
			option(&opts)
		}
	}

	providerConfigRepo := repository.NewProviderConfigRepository(db)
	routePolicyRepo := repository.NewRoutePolicyRepository(db)
	providerHealthRepo := repository.NewProviderHealthRepository(db)
	artifactRepo := repository.NewArtifactRepository(db)
	chunkArtifactRepo := repository.NewChunkArtifactSetRepository(db)
	parseRunRepo := repository.NewParseRunRepository(db)
	chunkingRunRepo := repository.NewChunkingRunRepository(db)
	playgroundRunRepo := repository.NewPlaygroundRunRepository(db)

	providerAdminService := service.NewProviderAdminService(providerConfigRepo)
	policyAdminService := service.NewPolicyAdminService(routePolicyRepo)
	healthService := service.NewHealthService(providerHealthRepo)
	artifactService := service.NewArtifactService(artifactRepo)
	chunkArtifactService := service.NewChunkArtifactSetService(chunkArtifactRepo)
	runQueryService := service.NewRunQueryService(parseRunRepo, chunkingRunRepo)
	playgroundRunService := service.NewPlaygroundRunService(playgroundRunRepo)
	capabilityOptions := make([]contentparsecap.ModuleOption, 0, 2)
	if opts.enableSystemVLM {
		if opts.systemVLMAvailable {
			capabilityOptions = append(capabilityOptions, contentparsecap.WithAdapters(systemvlm.NewAdapter(opts.llmClient, opts.defaultModelSvc)))
			capabilityOptions = append(capabilityOptions, contentparsecap.WithFigureSummaryEnhancer(
				hyperparsesdk.NewDefaultChatFigureSummaryLocalizer(opts.llmClient, opts.defaultModelSvc),
			))
		}
		capabilityOptions = append(capabilityOptions, contentparsecap.WithProviderOverrides(contentparsecap.SystemVLMProviderConfig(opts.systemVLMAvailable)))
	}
	capabilityModule := contentparsecap.NewModule(capabilityOptions...)
	cryptoService, _ := llmcrypto.DefaultCryptoService()
	providerCatalogs := service.NewProviderCatalogResolver(providerConfigRepo, capabilityModule.Catalog, cryptoService)
	if capabilityModule.RoutedService != nil {
		capabilityModule.RoutedService.SetProviderCatalogResolver(
			func(ctx context.Context, req contracts.ParseRequest) (*contracts.ParseProviderCatalog, string, error) {
				return providerCatalogs.Resolve(
					ctx,
					parseRequestMetadataUUID(req.Metadata, "organization_id", "tenant_id"),
					parseRequestMetadataUUID(req.Metadata, "workspace_id", "team_tenant_id"),
				)
			},
		)
	}
	providerSettings := service.NewProviderSettingsService(providerConfigRepo, cryptoService)
	playgroundHandler := handler.NewPlaygroundHandler(capabilityModule, playgroundRunService)
	if playgroundHandler != nil {
		playgroundHandler.SetProviderCatalogResolver(providerCatalogs)
		playgroundHandler.SetOrganizationService(opts.organization)
		playgroundHandler.SetAccountService(opts.account)
	}

	return &Module{
		ProviderConfigRepo: providerConfigRepo,
		RoutePolicyRepo:    routePolicyRepo,
		ProviderHealthRepo: providerHealthRepo,
		ArtifactRepo:       artifactRepo,
		ChunkArtifactRepo:  chunkArtifactRepo,
		ParseRunRepo:       parseRunRepo,
		ChunkingRunRepo:    chunkingRunRepo,
		PlaygroundRunRepo:  playgroundRunRepo,

		ProviderAdminService: providerAdminService,
		PolicyAdminService:   policyAdminService,
		HealthService:        healthService,
		ArtifactService:      artifactService,
		ChunkArtifactService: chunkArtifactService,
		RunQueryService:      runQueryService,
		PlaygroundRunService: playgroundRunService,
		ProviderCatalogs:     providerCatalogs,
		ProviderSettings:     providerSettings,

		ContentParseService: capabilityModule.Service,
		Orchestrator:        capabilityModule.Orchestrator,
		Planner:             capabilityModule.Planner,
		Catalog:             capabilityModule.Catalog,

		ProviderHandler:   handler.NewProviderHandler(providerAdminService),
		PolicyHandler:     handler.NewPolicyHandler(policyAdminService),
		HealthHandler:     handler.NewHealthHandler(healthService),
		ArtifactHandler:   handler.NewArtifactHandler(artifactService),
		RunHandler:        handler.NewRunHandler(runQueryService, artifactService),
		PlaygroundHandler: playgroundHandler,
		SettingsHandler:   handler.NewProviderSettingsHandler(providerSettings),
	}
}

func parseRequestMetadataUUID(metadata map[string]any, keys ...string) *uuid.UUID {
	for _, key := range keys {
		value, ok := metadata[key]
		if !ok || value == nil {
			continue
		}
		text, ok := value.(string)
		if !ok {
			continue
		}
		parsed, err := uuid.Parse(text)
		if err == nil && parsed != uuid.Nil {
			return &parsed
		}
	}
	return nil
}

func (m *Module) RegisterInternalRoutes(rg *gin.RouterGroup) {
	if m == nil {
		return
	}
	handler.RegisterInternalRoutes(
		rg,
		m.ProviderHandler,
		m.PolicyHandler,
		m.HealthHandler,
		m.ArtifactHandler,
		m.RunHandler,
		m.PlaygroundHandler,
	)
}

func (m *Module) RegisterPlaygroundRoutes(rg *gin.RouterGroup) {
	if m == nil {
		return
	}
	group := rg.Group("/content-parse")
	if m.PlaygroundHandler != nil {
		m.PlaygroundHandler.RegisterRoutes(group)
	}
	if m.SettingsHandler != nil {
		m.SettingsHandler.RegisterRoutes(group)
	}
}
