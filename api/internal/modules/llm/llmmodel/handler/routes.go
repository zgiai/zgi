package handler

import "github.com/gin-gonic/gin"

// RegisterTenantModelRoutes registers tenant routes for model management
func RegisterTenantModelRoutes(r *gin.RouterGroup, handler *ModelHandler, availableHandler *AvailableModelsHandler) {
	// Official models - separate route to avoid conflict with /:id
	r.GET("/official-models", handler.ListOfficialModels)

	g := r.Group("/models")

	// Available models API (must come before /:id to avoid route conflict)
	if availableHandler != nil {
		g.GET("/available", availableHandler.ListAvailable)
		g.POST("/available/refresh", availableHandler.RefreshCache)
	}

	g.GET("/configs", handler.ListModelConfigs)
	g.GET("/custom", handler.ListCustomModels)
	g.POST("/custom", handler.CreateCustom)
	g.POST("/config", handler.ConfigureModel)
	g.POST("/provider/toggle", handler.ToggleProviderModels)
	g.POST("/batch/toggle", handler.BatchToggleModels)
	g.GET("/parameters", handler.GetModelParameters)

	// Parameterized paths come after
	g.GET("", handler.ListTenantModels)
	g.GET("/config/:model_id", handler.GetModelConfig)
	g.GET("/custom/:id", handler.GetCustom)
	g.PUT("/custom/:id", handler.UpdateCustom)
	g.GET("/:id/availability", handler.CheckAvailability)
	g.POST("/availability/batch", handler.BatchCheckAvailability)
	g.DELETE("/custom/:id", handler.DeleteCustom)
}
