package v1

import (
	"github.com/gin-gonic/gin"
	chatruntime "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/service"
	imagemodule "github.com/zgiai/zgi/api/internal/modules/image"
	channelsvc "github.com/zgiai/zgi/api/internal/modules/llm/channel/service"
	llmclient "github.com/zgiai/zgi/api/internal/modules/llm/client"
	llmmodelsvc "github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/service"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	"github.com/zgiai/zgi/api/middleware"
	"github.com/zgiai/zgi/api/pkg/logger"
)

type ImageRuntimeRouteDeps struct {
	AvailableModels llmmodelsvc.AvailableModelsService
	Routes          channelsvc.ChannelService
	LLMClient       llmclient.LLMClient
	ChatService     chatruntime.Service
	AccountService  interfaces.AccountService
}

func RegisterImageRuntimeRoutes(router *gin.RouterGroup, deps ImageRuntimeRouteDeps) {
	if deps.AvailableModels == nil {
		panic("image runtime routes require available models service")
	}
	if deps.Routes == nil {
		panic("image runtime routes require channel route service")
	}
	if deps.LLMClient == nil {
		panic("image runtime routes require llm client")
	}
	if deps.ChatService == nil {
		panic("image runtime routes require chat service")
	}
	if deps.AccountService == nil {
		panic("image runtime routes require account service")
	}
	module := imagemodule.NewModule(deps.AvailableModels, deps.Routes, deps.LLMClient, deps.ChatService)
	group := router.Group("")
	group.Use(middleware.SetupRequired())
	group.Use(middleware.JWTWithOrganizationAndService(deps.AccountService))
	group.Use(middleware.CurrentWorkspaceRequired())
	module.RegisterRoutes(group)
	logger.Info("Image runtime routes registered", "path", "/console/api/image-runtime/*")
}
