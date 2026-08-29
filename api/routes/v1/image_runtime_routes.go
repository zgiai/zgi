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
	appcatalog "github.com/zgiai/zgi/api/pkg/apperror/catalog"
	apptransport "github.com/zgiai/zgi/api/pkg/apperror/transport"
	"github.com/zgiai/zgi/api/pkg/logger"
	"gorm.io/gorm"
)

type ImageRuntimeRouteDeps struct {
	DB                      *gorm.DB
	AvailableModels         llmmodelsvc.AvailableModelsService
	Routes                  channelsvc.ChannelService
	LLMClient               llmclient.LLMClient
	ChatService             chatruntime.Service
	AccountService          interfaces.AccountService
	FileService             interfaces.FileService
	ApplicationErrorCatalog *appcatalog.Catalog
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
	if deps.FileService == nil {
		panic("image runtime routes require file service")
	}
	errorProjector, err := apptransport.NewProjector(deps.ApplicationErrorCatalog)
	if err != nil {
		panic("image runtime routes require application error catalog")
	}
	module := imagemodule.NewModule(deps.DB, deps.AvailableModels, deps.Routes, deps.LLMClient, deps.ChatService, errorProjector, deps.FileService)
	group := router.Group("")
	group.Use(middleware.SetupRequired())
	group.Use(middleware.JWTWithOrganizationAndService(deps.AccountService))
	module.RegisterRoutes(group)
	logger.Info("Image runtime routes registered", "path", "/console/api/image-runtime/*")
}
