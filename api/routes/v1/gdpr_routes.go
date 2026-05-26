package v1

import (
	"github.com/gin-gonic/gin"
	"github.com/zgiai/zgi/api/internal/modules/gdpr"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	"gorm.io/gorm"
)

// GDPRRouteDeps contains dependencies required by GDPR routes.
type GDPRRouteDeps struct {
	DB             *gorm.DB
	AccountService interfaces.AccountService
}

// RegisterGDPRRoutes registers GDPR compliance routes.
// User routes: /console/api/gdpr/*
func RegisterGDPRRoutes(router *gin.RouterGroup, deps GDPRRouteDeps) {
	if deps.DB == nil {
		panic("gdpr routes require db")
	}
	if deps.AccountService == nil {
		panic("gdpr routes require account service")
	}

	gdprModule := gdpr.NewModule(deps.DB)
	gdprModule.RegisterRoutes(router, deps.AccountService)
}
