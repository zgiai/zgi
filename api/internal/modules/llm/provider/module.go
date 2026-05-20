package provider

import (
	llmmodelrepo "github.com/zgiai/ginext/internal/modules/llm/llmmodel/repository"
	"github.com/zgiai/ginext/internal/modules/llm/provider/handler"
	"github.com/zgiai/ginext/internal/modules/llm/provider/repository"
	"github.com/zgiai/ginext/internal/modules/llm/provider/service"
	"gorm.io/gorm"
)

// Module provides provider management functionality
type Module struct {
	GlobalRepo repository.ProviderRepository
	ConfigRepo repository.ProviderConfigRepository
	CustomRepo repository.CustomProviderRepository
	Service    service.ProviderService
	Handler    *handler.ProviderHandler
}

// NewModule creates a new provider module with all dependencies wired
func NewModule(db *gorm.DB) *Module {
	globalRepo := repository.NewProviderRepository(db)
	configRepo := repository.NewProviderConfigRepository(db)
	customRepo := repository.NewCustomProviderRepository(db)
	modelRepo := llmmodelrepo.NewModelRepository(db)
	modelConfigRepo := llmmodelrepo.NewModelConfigRepository(db)
	svc := service.NewProviderService(db, globalRepo, configRepo, customRepo, modelRepo, modelConfigRepo, nil)
	h := handler.NewProviderHandler(svc)

	return &Module{
		GlobalRepo: globalRepo,
		ConfigRepo: configRepo,
		CustomRepo: customRepo,
		Service:    svc,
		Handler:    h,
	}
}
