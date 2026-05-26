package v1

import (
	"github.com/gin-gonic/gin"
	"github.com/zgiai/zgi/api/internal/modules/memory"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
)

// MemoryRouteDeps contains dependencies required by memory routes.
type MemoryRouteDeps struct {
	MemoryService  *memory.Service
	AccountService interfaces.AccountService
}

func RegisterMemoryRoutes(router *gin.RouterGroup, deps MemoryRouteDeps) {
	if deps.MemoryService == nil {
		panic("memory routes require memory service")
	}
	if deps.AccountService == nil {
		panic("memory routes require account service")
	}

	module := &memory.Module{
		Service: deps.MemoryService,
		Handler: memory.NewHandler(deps.MemoryService),
	}
	module.RegisterRoutes(router, deps.AccountService)
}
