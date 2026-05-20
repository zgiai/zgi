package workspacequota

import (
	"github.com/zgiai/ginext/internal/modules/llm/workspacequota/handler"
	"github.com/zgiai/ginext/internal/modules/llm/workspacequota/service"
	"gorm.io/gorm"
)

// Module provides workspace quota management APIs for LLM billing.
type Module struct {
	Service service.WorkspaceQuotaService
	Handler *handler.WorkspaceQuotaHandler
}

func NewModule(db *gorm.DB) *Module {
	svc := service.NewWorkspaceQuotaService(db)
	h := handler.NewWorkspaceQuotaHandler(svc)
	return &Module{
		Service: svc,
		Handler: h,
	}
}
