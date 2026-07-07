package handler

import (
	"github.com/gin-gonic/gin"
	"github.com/zgiai/zgi/api/middleware"
)

// RegisterTenantModelRoutes registers tenant routes for model management
func RegisterTenantModelRoutes(r *gin.RouterGroup, handler *ModelHandler, availableHandler *AvailableModelsHandler) {
	// Official models - separate route to avoid conflict with /:id
	r.GET("/official-models", handler.ListOfficialModels)

	g := r.Group("/models")

	// Available models API (must come before /:id to avoid route conflict)
	if availableHandler != nil {
		g.GET("/available", availableHandler.ListAvailable)
	}

	admin := g.Group("")
	admin.Use(middleware.EnterpriseAdminOrOwnerRequired())
	if availableHandler != nil {
		admin.POST("/available/refresh", availableHandler.RefreshCache)
	}

	g.GET("/configs", handler.ListModelConfigs)
	g.GET("/custom", handler.ListCustomModels)
	g.GET("/parameters", handler.GetModelParameters)

	// Parameterized paths come after
	g.GET("", handler.ListTenantModels)
	g.GET("/config/:model_id", handler.GetModelConfig)
	g.GET("/custom/:id", handler.GetCustom)
	g.GET("/:id/availability", handler.CheckAvailability)
	g.POST("/availability/batch", handler.BatchCheckAvailability)

	admin.POST("/custom", handler.CreateCustom)
	admin.POST("/config", handler.ConfigureModel)
	admin.POST("/provider/toggle", handler.ToggleProviderModels)
	admin.POST("/batch/toggle", handler.BatchToggleModels)
	admin.PUT("/custom/:id", handler.UpdateCustom)
	admin.DELETE("/custom/:id", handler.DeleteCustom)
}
