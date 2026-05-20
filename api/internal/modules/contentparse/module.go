package contentparse

import (
	"github.com/gin-gonic/gin"
	contentparsecap "github.com/zgiai/ginext/internal/capabilities/contentparse"
	systemvlm "github.com/zgiai/ginext/internal/capabilities/contentparse/adapters/system_vlm"
	"github.com/zgiai/ginext/internal/modules/contentparse/handler"
	"github.com/zgiai/ginext/internal/modules/contentparse/repository"
	"github.com/zgiai/ginext/internal/modules/contentparse/service"
	llmclient "github.com/zgiai/ginext/internal/modules/llm/client"
	llmdefaultservice "github.com/zgiai/ginext/internal/modules/llm/defaultmodel/service"
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

	ProviderHandler   *handler.ProviderHandler
	PolicyHandler     *handler.PolicyHandler
	HealthHandler     *handler.HealthHandler
	ArtifactHandler   *handler.ArtifactHandler
	RunHandler        *handler.RunHandler
	PlaygroundHandler *handler.PlaygroundHandler
}

type ModuleOption func(*moduleOptions)

type moduleOptions struct {
	llmClient          llmclient.LLMClient
	defaultModelSvc    llmdefaultservice.DefaultModelService
	enableSystemVLM    bool
	systemVLMAvailable bool
}

func WithSystemVisionModel(llmClient llmclient.LLMClient, defaultModelSvc llmdefaultservice.DefaultModelService) ModuleOption {
	return func(opts *moduleOptions) {
		opts.llmClient = llmClient
		opts.defaultModelSvc = defaultModelSvc
		opts.enableSystemVLM = true
		opts.systemVLMAvailable = llmClient != nil && defaultModelSvc != nil
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
		}
		capabilityOptions = append(capabilityOptions, contentparsecap.WithProviderOverrides(contentparsecap.SystemVLMProviderConfig(opts.systemVLMAvailable)))
	}
	capabilityModule := contentparsecap.NewModule(capabilityOptions...)
	providerCatalogs := service.NewProviderCatalogResolver(providerConfigRepo, capabilityModule.Catalog)
	playgroundHandler := handler.NewPlaygroundHandler(capabilityModule, playgroundRunService)
	if playgroundHandler != nil {
		playgroundHandler.SetProviderCatalogResolver(providerCatalogs)
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

		ProviderHandler:   handler.NewProviderHandler(providerAdminService),
		PolicyHandler:     handler.NewPolicyHandler(policyAdminService),
		HealthHandler:     handler.NewHealthHandler(healthService),
		ArtifactHandler:   handler.NewArtifactHandler(artifactService),
		RunHandler:        handler.NewRunHandler(runQueryService, artifactService),
		PlaygroundHandler: playgroundHandler,
	}
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
	if m == nil || m.PlaygroundHandler == nil {
		return
	}
	group := rg.Group("/content-parse")
	m.PlaygroundHandler.RegisterRoutes(group)
}
