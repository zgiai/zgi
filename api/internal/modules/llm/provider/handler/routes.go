package handler

import (
	"github.com/gin-gonic/gin"
	"github.com/zgiai/zgi/api/middleware"
	"gorm.io/gorm"
)

// RegisterTenantProviderRoutes registers tenant provider routes
// Note: Requires ExtractOrganizationID middleware to be applied at router level
func RegisterTenantProviderRoutes(r *gin.RouterGroup, handler *ProviderHandler, db *gorm.DB) {
	g := r.Group("/providers")
	// Apply middleware to extract organization_id (tenant_id) from workspace
	g.Use(ExtractOrganizationID(db))
	admin := g.Group("")
	admin.Use(middleware.EnterpriseAdminOrOwnerRequired())
	{
		g.GET("/configs", handler.ListProviderConfigs)
		g.GET("/custom", handler.ListCustomProviders)
		admin.POST("/custom", handler.CreateCustom)
		admin.POST("/config", handler.ConfigureProvider)
		admin.POST("/toggle", handler.ToggleProvider)
		admin.POST("/:provider/models/toggle", handler.ToggleModel)

		// Parameterized paths come after
		g.GET("", handler.ListTenantProviders)
		g.GET("/:id", handler.GetTenantProvider)
		g.GET("/config/:provider_id", handler.GetProviderConfig)
		g.GET("/custom/:id", handler.GetCustom)
		admin.PUT("/custom/:id", handler.UpdateCustom)
		admin.DELETE("/custom/:id", handler.DeleteCustom)
	}
}
