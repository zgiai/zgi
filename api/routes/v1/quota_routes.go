package v1

import (
	"github.com/gin-gonic/gin"

	"github.com/zgiai/ginext/internal/container"
	"github.com/zgiai/ginext/internal/modules/quota/handler"
	"github.com/zgiai/ginext/middleware"
)

// RegisterQuotaRoutes registers all quota-related routes
func RegisterQuotaRoutes(router *gin.RouterGroup, container *container.ServiceContainer) {
	// Get quota service from container
	quotaService := container.GetQuotaService()

	// Initialize handler
	quotaHandler := handler.NewQuotaHandler(quotaService)

	// Create quota group with JWT middleware
	quotaGroup := router.Group("/quota")
	quotaGroup.Use(middleware.JWT())

	// Register routes
	quotaHandler.RegisterRoutes(quotaGroup)
}
