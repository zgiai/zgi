package v1

import (
	"github.com/gin-gonic/gin"
	llmmodelsvc "github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/service"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	videomodule "github.com/zgiai/zgi/api/internal/modules/video"
	"github.com/zgiai/zgi/api/middleware"
	appcatalog "github.com/zgiai/zgi/api/pkg/apperror/catalog"
	apptransport "github.com/zgiai/zgi/api/pkg/apperror/transport"
	"github.com/zgiai/zgi/api/pkg/logger"
	"gorm.io/gorm"
)

type VideoRuntimeRouteDeps struct {
	DB                      *gorm.DB
	AvailableModels         llmmodelsvc.AvailableModelsService
	LLMClient               interface{}
	AccountService          interfaces.AccountService
	ApplicationErrorCatalog *appcatalog.Catalog
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
	if deps.ApplicationErrorCatalog == nil {
		panic("video runtime routes require application error catalog")
	}
	errorProjector, err := apptransport.NewProjector(deps.ApplicationErrorCatalog)
	if err != nil {
		panic("video runtime routes require application error projector: " + err.Error())
	}
	module := videomodule.NewModule(deps.DB, deps.AvailableModels, deps.LLMClient, errorProjector)
	group := router.Group("")
	group.Use(middleware.SetupRequired())
	group.Use(middleware.JWTWithOrganizationAndService(deps.AccountService))
	module.RegisterRoutes(group)
	logger.Info("Video runtime routes registered", "path", "/console/api/video-runtime/*")
}
