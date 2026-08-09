package v1

import (
	"github.com/gin-gonic/gin"
	llmmodelsvc "github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/service"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	videomodule "github.com/zgiai/zgi/api/internal/modules/video"
	"github.com/zgiai/zgi/api/middleware"
	"github.com/zgiai/zgi/api/pkg/logger"
	"gorm.io/gorm"
)

type VideoRuntimeRouteDeps struct {
	DB              *gorm.DB
	AvailableModels llmmodelsvc.AvailableModelsService
	LLMClient       interface{}
	AccountService  interfaces.AccountService
}

func RegisterVideoRuntimeRoutes(router *gin.RouterGroup, deps VideoRuntimeRouteDeps) {
	if deps.DB == nil {
		panic("video runtime routes require db")
	}
	if deps.AvailableModels == nil {
		panic("video runtime routes require available models service")
	}
	if deps.LLMClient == nil {
		panic("video runtime routes require llm client")
	}
	if deps.AccountService == nil {
		panic("video runtime routes require account service")
	}
	module := videomodule.NewModule(deps.DB, deps.AvailableModels, deps.LLMClient)
	group := router.Group("")
	group.Use(middleware.SetupRequired())
	group.Use(middleware.JWTWithOrganizationAndService(deps.AccountService))
	module.RegisterRoutes(group)
	logger.Info("Video runtime routes registered", "path", "/console/api/video-runtime/*")
}
