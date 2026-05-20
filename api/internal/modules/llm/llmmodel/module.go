package llmmodel

import (
	channelrepo "github.com/zgiai/ginext/internal/modules/llm/channel/repository"
	"github.com/zgiai/ginext/internal/modules/llm/llmmodel/handler"
	"github.com/zgiai/ginext/internal/modules/llm/llmmodel/repository"
	"github.com/zgiai/ginext/internal/modules/llm/llmmodel/service"
	providerrepo "github.com/zgiai/ginext/internal/modules/llm/provider/repository"
	"gorm.io/gorm"
)

// Module provides model management functionality
type Module struct {
	GlobalRepo repository.ModelRepository
	ConfigRepo repository.ModelConfigRepository
	CustomRepo repository.CustomModelRepository
	Service    service.ModelService
	Handler    *handler.ModelHandler

	// Available models service and handler (optimized for business use)
	AvailableModelsSvc     service.AvailableModelsService
	AvailableModelsHandler *handler.AvailableModelsHandler
	PrivateModelLookupSvc  service.PrivateModelLookupService
}

// NewModule creates a new model module with all dependencies wired
func NewModule(db *gorm.DB) *Module {
	globalRepo := repository.NewModelRepository(db)
	configRepo := repository.NewModelConfigRepository(db)
	customRepo := repository.NewCustomModelRepository(db)
	globalProviderRepo := providerrepo.NewProviderRepository(db)
	providerConfigRepo := providerrepo.NewProviderConfigRepository(db)
	customProvRepo := providerrepo.NewCustomProviderRepository(db)
	routeRepo := channelrepo.NewTenantRouteRepository(db)
	availabilitySvc := service.NewModelAvailabilityServiceWithProviderRepos(globalRepo, configRepo, routeRepo, globalProviderRepo, providerConfigRepo)
	svc := service.NewModelServiceWithProviderRepos(db, globalRepo, configRepo, customRepo, availabilitySvc, customProvRepo, globalProviderRepo, providerConfigRepo)
	h := handler.NewModelHandler(svc)

	// Initialize available models service with caching and tenant route filtering
	availableSvc := service.NewAvailableModelsServiceWithProviderRepos(globalRepo, configRepo, customRepo, routeRepo, globalProviderRepo, providerConfigRepo, customProvRepo)
	availableHandler := handler.NewAvailableModelsHandler(availableSvc)
	privateLookupSvc := service.NewPrivateModelLookupService(customRepo)

	// Wire cache invalidation: model service -> available models service
	svc.SetAvailableModelsService(availableSvc)

	return &Module{
		GlobalRepo:             globalRepo,
		ConfigRepo:             configRepo,
		CustomRepo:             customRepo,
		Service:                svc,
		Handler:                h,
		AvailableModelsSvc:     availableSvc,
		AvailableModelsHandler: availableHandler,
		PrivateModelLookupSvc:  privateLookupSvc,
	}
}
