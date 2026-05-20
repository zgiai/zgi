package v1

import (
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/zgiai/zgi/api/config"

	"github.com/zgiai/zgi/api/internal/container"
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

// RegisterSetupRoutes wires setup and system feature handlers.
func RegisterSetupRoutes(router *gin.RouterGroup, serviceContainer *container.ServiceContainer) {
	setupService := serviceContainer.GetBootstrapService()
	featureService := system_service.NewFeatureService()

	setupHandler := system_handler.NewSetupHandler(setupService)
	featureHandler := system_handler.NewFeatureHandler(featureService)

	RegisterSetupPaths(router, setupHandler.GetSetupStatus, setupHandler.SetupSystem, featureHandler.GetSystemFeatures)
}
