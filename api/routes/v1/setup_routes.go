package v1

import (
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/zgiai/zgi/api/config"

	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	system_handler "github.com/zgiai/zgi/api/internal/modules/system/handler"
	system_service "github.com/zgiai/zgi/api/internal/modules/system/service"
)

// RegisterSetupPaths registers setup endpoints for the current edition.
func RegisterSetupPaths(router *gin.RouterGroup, setupStatusHandler, setupSystemHandler, systemFeaturesHandler gin.HandlerFunc) {
	if strings.EqualFold(strings.TrimSpace(config.Current().Platform.Edition), "SELF_HOSTED") {
		router.GET("/setup", setupStatusHandler)
		router.POST("/setup", system_handler.OnlyEditionSelfHosted(), setupSystemHandler)
	}

	router.GET("/system-features", systemFeaturesHandler)
}

// SetupRouteDeps contains dependencies required by setup routes.
type SetupRouteDeps struct {
	BootstrapService *system_service.BootstrapService
	FeatureService   interfaces.FeatureService
}

// RegisterSetupRoutes wires setup and system feature handlers.
func RegisterSetupRoutes(router *gin.RouterGroup, deps SetupRouteDeps) {
	if deps.BootstrapService == nil {
		panic("setup routes require bootstrap service")
	}
	if deps.FeatureService == nil {
		panic("setup routes require feature service")
	}

	setupHandler := system_handler.NewSetupHandler(deps.BootstrapService)
	featureHandler := system_handler.NewFeatureHandler(deps.FeatureService)

	RegisterSetupPaths(router, setupHandler.GetSetupStatus, setupHandler.SetupSystem, featureHandler.GetSystemFeatures)
}
