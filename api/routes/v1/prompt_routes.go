package v1

import (
	"github.com/gin-gonic/gin"
	"github.com/zgiai/ginext/internal/container"
	"github.com/zgiai/ginext/internal/modules/prompts"
)

func RegisterPromptRoutes(router *gin.RouterGroup, serviceContainer *container.ServiceContainer) {
	module := prompts.NewModule(
		serviceContainer.GetDB(),
		serviceContainer.GetAccountService(),
		serviceContainer.GetOrganizationService(),
		serviceContainer.GetLLMClient(),
		serviceContainer.GetDefaultModelService(),
	)
	module.PromptHandler.RegisterRoutes(router)
}
