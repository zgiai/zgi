package video

import (
	"github.com/gin-gonic/gin"
	llmmodelsvc "github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/service"
	"github.com/zgiai/zgi/api/internal/modules/video/handler"
	videoservice "github.com/zgiai/zgi/api/internal/modules/video/service"
	"gorm.io/gorm"
)

type Module struct {
	Handler *handler.Handler
	Service videoservice.Service
}

func NewModule(db *gorm.DB, availableModels llmmodelsvc.AvailableModelsService, llmClient interface{}) *Module {
	svc := videoservice.NewService(db, availableModels, llmClient)
	return &Module{
		Handler: handler.NewHandler(svc),
		Service: svc,
	}
}

func (m *Module) RegisterRoutes(router *gin.RouterGroup) {
	m.Handler.RegisterRoutes(router)
}
