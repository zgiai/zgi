package v1

import (
	"github.com/gin-gonic/gin"
	"github.com/zgiai/zgi/api/internal/container"
	"github.com/zgiai/zgi/api/internal/modules/memory"
)

func RegisterMemoryRoutes(router *gin.RouterGroup, serviceContainer *container.ServiceContainer) {
	module := memory.NewModule(serviceContainer.GetDB())
	module.Service = serviceContainer.GetMemoryService()
	module.Handler = memory.NewHandler(module.Service)
	module.RegisterRoutes(router, serviceContainer.GetAccountServiceAdapter())
}
