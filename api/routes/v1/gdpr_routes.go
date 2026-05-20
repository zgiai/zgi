package v1

import (
	"github.com/gin-gonic/gin"
	"github.com/zgiai/ginext/internal/container"
	"github.com/zgiai/ginext/internal/modules/gdpr"
)

// RegisterGDPRRoutes registers GDPR compliance routes.
// User routes: /console/api/gdpr/*
func RegisterGDPRRoutes(router *gin.RouterGroup, serviceContainer *container.ServiceContainer) {
	db := serviceContainer.GetDB()
	accountService := serviceContainer.GetAccountServiceAdapter()

	// Create GDPR module
	gdprModule := gdpr.NewModule(db)

	// Register routes
	gdprModule.RegisterRoutes(router, accountService)
}
