package credential

import (
	"github.com/zgiai/ginext/internal/modules/llm/credential/handler"
	"github.com/zgiai/ginext/internal/modules/llm/credential/repository"
	"github.com/zgiai/ginext/internal/modules/llm/credential/service"
	"github.com/zgiai/ginext/internal/modules/llm/shared"
	"gorm.io/gorm"
)

// Module provides credential management functionality
type Module struct {
	TenantRepo    repository.TenantCredentialRepository
	TenantSvc     service.TenantCredentialService
	TenantHandler *handler.TenantCredentialHandler
}

// NewModule creates a new credential module with all dependencies wired
func NewModule(db *gorm.DB, crypto shared.CryptoService) *Module {
	tenantRepo := repository.NewTenantCredentialRepository(db)
	tenantSvc := service.NewTenantCredentialService(tenantRepo, crypto, db)
	tenantHandler := handler.NewTenantCredentialHandler(tenantSvc)

	return &Module{
		TenantRepo:    tenantRepo,
		TenantSvc:     tenantSvc,
		TenantHandler: tenantHandler,
	}
}
