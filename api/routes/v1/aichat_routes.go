package v1

import (
	"github.com/gin-gonic/gin"
	"github.com/zgiai/zgi/api/internal/container"
	"github.com/zgiai/zgi/api/internal/modules/aichat"
	"github.com/zgiai/zgi/api/internal/modules/skills"
	"github.com/zgiai/zgi/api/middleware"
	"github.com/zgiai/zgi/api/pkg/logger"
)

func RegisterAIChatRoutes(router *gin.RouterGroup, serviceContainer *container.ServiceContainer) {
	module := aichat.NewModuleWithDependencies(
		serviceContainer.GetDB(),
		serviceContainer.GetLLMClient(),
		serviceContainer.GetDefaultModelService(),
		serviceContainer.GetFileService(),
		serviceContainer.GetContentExtractor(),
		serviceContainer.GetOrganizationService(),
		serviceContainer.GetMemoryService(),
		skills.NewRuntime(serviceContainer.GetToolEngine(), serviceContainer.GetToolManager()),
	)
	group := router.Group("")
	group.Use(middleware.SetupRequired())
	group.Use(middleware.JWTWithOrganizationAndService(serviceContainer.GetAccountServiceAdapter()))
	module.RegisterRoutes(group)
	logger.Info("AIChat routes registered", "path", "/console/api/aichat/*")
}
