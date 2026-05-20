package gdpr

import (
	"github.com/zgiai/ginext/internal/modules/gdpr/handler"
	"github.com/zgiai/ginext/internal/modules/gdpr/repository"
	"github.com/zgiai/ginext/internal/modules/gdpr/service"
	"gorm.io/gorm"
)

// Module represents the GDPR module
type Module struct {
	Handler *handler.GDPRHandler
	Service service.GDPRService
}

// NewModule creates a new GDPR module with all dependencies
func NewModule(db *gorm.DB) *Module {
	repo := repository.NewGDPRRepository(db)
	svc := service.NewGDPRService(db, repo)
	h := handler.NewGDPRHandler(svc)

	return &Module{
		Handler: h,
		Service: svc,
	}
}
