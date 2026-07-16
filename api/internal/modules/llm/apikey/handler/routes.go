package handler

import (
	"github.com/gin-gonic/gin"
	"github.com/zgiai/zgi/api/middleware"
)

// RegisterAPIKeyRoutes registers API key routes for tenant users
func RegisterAPIKeyRoutes(rg *gin.RouterGroup, h *APIKeyHandler) {
	apiKeys := rg.Group("/api-keys")
	admin := apiKeys.Group("")
	admin.Use(middleware.EnterpriseAdminOrOwnerRequired())
	{
		admin.POST("", h.CreateAPIKey)
		apiKeys.GET("", h.ListAPIKeys)
		apiKeys.GET("/:id", h.GetAPIKey)
		admin.PUT("/:id", h.UpdateAPIKey)
		admin.DELETE("/:id", h.DeleteAPIKey)
		apiKeys.POST("/validate", h.ValidateAPIKey)
	}
}
