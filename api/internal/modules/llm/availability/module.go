package availability

import (
	"github.com/zgiai/zgi/api/internal/modules/llm/availability/handler"
	"github.com/zgiai/zgi/api/internal/modules/llm/availability/service"
	channelrepo "github.com/zgiai/zgi/api/internal/modules/llm/channel/repository"
	llmrepo "github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/repository"
	providerrepo "github.com/zgiai/zgi/api/internal/modules/llm/provider/repository"
)

// Module represents the availability module
type Module struct {
	Service service.AvailabilityService
	Handler *handler.AvailabilityHandler
}

// NewModule creates a new availability module
func NewModule(
	modelRepo llmrepo.ModelRepository,
	configRepo llmrepo.ModelConfigRepository,
	routeRepo channelrepo.TenantRouteRepository,
	globalProviderRepo providerrepo.ProviderRepository,
	providerConfigRepo providerrepo.ProviderConfigRepository,
) *Module {
	svc := service.NewAvailabilityServiceWithProviderRepos(modelRepo, configRepo, routeRepo, globalProviderRepo, providerConfigRepo)
	hdl := handler.NewAvailabilityHandler(svc)

	return &Module{
		Service: svc,
		Handler: hdl,
	}
}
