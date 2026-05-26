package v1

import (
	"github.com/gin-gonic/gin"

	"github.com/zgiai/zgi/api/internal/modules/quota/handler"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	"github.com/zgiai/zgi/api/middleware"
)

// QuotaRouteDeps contains dependencies required by quota routes.
type QuotaRouteDeps struct {
	QuotaService interfaces.QuotaService
}

// RegisterQuotaRoutes registers all quota-related routes
func RegisterQuotaRoutes(router *gin.RouterGroup, deps QuotaRouteDeps) {
	if deps.QuotaService == nil {
		panic("quota routes require quota service")
	}

	quotaHandler := handler.NewQuotaHandler(deps.QuotaService)
	quotaGroup := router.Group("/quota")
	quotaGroup.Use(middleware.JWT())

	quotaHandler.RegisterRoutes(quotaGroup)
}
