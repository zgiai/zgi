package v1

import (
	"github.com/gin-gonic/gin"
	"github.com/zgiai/zgi/api/internal/container"
	contentparsemodule "github.com/zgiai/zgi/api/internal/modules/contentparse"
	"github.com/zgiai/zgi/api/middleware"
)

func RegisterContentParseRoutes(v1 *gin.RouterGroup, serviceContainer *container.ServiceContainer) {
	group := v1.Group("")
	group.Use(middleware.SetupRequired())
	group.Use(middleware.JWTWithOrganizationAndService(serviceContainer.GetAccountServiceAdapter()))

	contentparsemodule.NewModule(
		serviceContainer.GetDB(),
		contentparsemodule.WithSystemVisionModel(serviceContainer.GetLLMClient(), serviceContainer.GetDefaultModelService()),
	).RegisterPlaygroundRoutes(group)
}
