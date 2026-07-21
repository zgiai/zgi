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
)

type Module struct {
	Handler *handler.Handler
	Service imageservice.Service
}

func NewModule(availableModels llmmodelsvc.AvailableModelsService, routes imageservice.RouteLister, llmClient llmclient.LLMClient, chatService service.Service) *Module {
	svc := imageservice.NewService(registry.NewRegistry(), availableModels, routes, llmClient, chatService, imageasset.NewService())
	return &Module{
		Handler: handler.NewHandler(svc),
		Service: svc,
	}
}

func (m *Module) RegisterRoutes(router *gin.RouterGroup, generateMiddleware gin.HandlerFunc) {
	m.Handler.RegisterRoutes(router, generateMiddleware)
}
