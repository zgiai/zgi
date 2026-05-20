package handler

import (
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// RegisterTenantProviderRoutes registers tenant provider routes
// Note: Requires ExtractOrganizationID middleware to be applied at router level
func RegisterTenantProviderRoutes(r *gin.RouterGroup, handler *ProviderHandler, db *gorm.DB) {
	g := r.Group("/providers")
	// Apply middleware to extract organization_id (tenant_id) from workspace
	g.Use(ExtractOrganizationID(db))
	{
		g.GET("/configs", handler.ListProviderConfigs)
		g.GET("/custom", handler.ListCustomProviders)
		g.POST("/custom", handler.CreateCustom)
		g.POST("/config", handler.ConfigureProvider)
		g.POST("/toggle", handler.ToggleProvider)
		g.POST("/:provider/models/toggle", handler.ToggleModel)

		// Parameterized paths come after
		g.GET("", handler.ListTenantProviders)
		g.GET("/:id", handler.GetTenantProvider)
		g.GET("/config/:provider_id", handler.GetProviderConfig)
		g.GET("/custom/:id", handler.GetCustom)
		g.PUT("/custom/:id", handler.UpdateCustom)
		g.DELETE("/custom/:id", handler.DeleteCustom)
	}
}
