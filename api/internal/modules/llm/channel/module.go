package channel

import (
	"github.com/zgiai/ginext/internal/infra/platform/console"
	"github.com/zgiai/ginext/internal/modules/llm/channel/handler"
	"github.com/zgiai/ginext/internal/modules/llm/channel/repository"
	"github.com/zgiai/ginext/internal/modules/llm/channel/service"
	credentialsvc "github.com/zgiai/ginext/internal/modules/llm/credential/service"
	llmmodelrepo "github.com/zgiai/ginext/internal/modules/llm/llmmodel/repository"
	llmmodelsvc "github.com/zgiai/ginext/internal/modules/llm/llmmodel/service"
	providerrepo "github.com/zgiai/ginext/internal/modules/llm/provider/repository"
	"github.com/zgiai/ginext/internal/modules/llm/shared"
	"gorm.io/gorm"
)

// Module provides channel and route management functionality
type Module struct {
	TenantRouteRepo repository.TenantRouteRepository
	Service         service.ChannelService
	Handler         *handler.ChannelHandler
}

// NewModule creates a new channel module with all dependencies wired
func NewModule(
	db *gorm.DB,
	tenantCredService credentialsvc.TenantCredentialService,
	validator service.ChannelValidator,
	modelRepo llmmodelrepo.ModelRepository,
	modelConfigRepo llmmodelrepo.ModelConfigRepository,
	customProviderRepo providerrepo.CustomProviderRepository,
	customModelRepo llmmodelrepo.CustomModelRepository,
	privateModels llmmodelsvc.PrivateModelLookupService,
	availableModels llmmodelsvc.AvailableModelsService,
	crypto shared.CryptoService,
	cp console.ConsoleProvider,
) *Module {
	tenantRouteRepo := repository.NewTenantRouteRepository(db)

	svc := service.NewChannelService(tenantRouteRepo, tenantCredService, validator, modelRepo, modelConfigRepo, customProviderRepo, customModelRepo, privateModels, availableModels, db, crypto, cp)

	h := handler.NewChannelHandler(svc, cp)

	return &Module{
		TenantRouteRepo: tenantRouteRepo,
		Service:         svc,
		Handler:         h,
	}
}
