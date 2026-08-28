package image

import (
	"github.com/gin-gonic/gin"
	"github.com/zgiai/zgi/api/internal/capabilities/chatruntime/service"
	"github.com/zgiai/zgi/api/internal/capabilities/imageasset"
	"github.com/zgiai/zgi/api/internal/modules/image/handler"
	"github.com/zgiai/zgi/api/internal/modules/image/registry"
	imageservice "github.com/zgiai/zgi/api/internal/modules/image/service"
	llmclient "github.com/zgiai/zgi/api/internal/modules/llm/client"
	llmmodelsvc "github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/service"
	apptransport "github.com/zgiai/zgi/api/pkg/apperror/transport"
	"gorm.io/gorm"
)

type Module struct {
	Handler *handler.Handler
	Service imageservice.Service
}

func NewModule(db *gorm.DB, availableModels llmmodelsvc.AvailableModelsService, routes imageservice.RouteLister, llmClient llmclient.LLMClient, chatService service.Service, errorProjector *apptransport.Projector, fileServices ...imageservice.ReferenceFileService) *Module {
	var fileService imageservice.ReferenceFileService
	if len(fileServices) > 0 {
		fileService = fileServices[0]
	}
	svc := imageservice.NewServiceWithTasks(db, registry.NewRegistry(), availableModels, routes, llmClient, chatService, imageasset.NewService(), fileService)
	return &Module{
		Handler: handler.NewHandler(svc, errorProjector),
		Service: svc,
	}
}

func (m *Module) RegisterRoutes(router *gin.RouterGroup) {
	m.Handler.RegisterRoutes(router)
}
