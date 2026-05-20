package v1

import (
	"github.com/gin-gonic/gin"
	"github.com/zgiai/ginext/internal/container"
	"github.com/zgiai/ginext/internal/modules/aichat"
	"github.com/zgiai/ginext/internal/modules/skills"
	"github.com/zgiai/ginext/middleware"
	"github.com/zgiai/ginext/pkg/logger"
)

func RegisterAIChatRoutes(router *gin.RouterGroup, serviceContainer *container.ServiceContainer) {
	module := aichat.NewModuleWithDependencies(
		serviceContainer.GetDB(),
		serviceContainer.GetLLMClient(),
		serviceContainer.GetDefaultModelService(),
		serviceContainer.GetFileService(),
		serviceContainer.GetContentExtractor(),
		serviceContainer.GetOrganizationService(),
		skills.NewRuntime(serviceContainer.GetToolEngine(), serviceContainer.GetToolManager()),
	)
	group := router.Group("")
	group.Use(middleware.SetupRequired())
	group.Use(middleware.JWTWithOrganizationAndService(serviceContainer.GetAccountServiceAdapter()))
	module.RegisterRoutes(group)
	logger.Info("AIChat routes registered", "path", "/console/api/aichat/*")
}
