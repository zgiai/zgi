package handler

import (
	"github.com/gin-gonic/gin"
)

// RegisterAPIKeyRoutes registers API key routes for tenant users
func RegisterAPIKeyRoutes(rg *gin.RouterGroup, h *APIKeyHandler) {
	apiKeys := rg.Group("/api-keys")
	{
		apiKeys.POST("", h.CreateAPIKey)
		apiKeys.GET("", h.ListAPIKeys)
		apiKeys.GET("/:id", h.GetAPIKey)
		apiKeys.PUT("/:id", h.UpdateAPIKey)
		apiKeys.DELETE("/:id", h.DeleteAPIKey)
		apiKeys.POST("/validate", h.ValidateAPIKey)
	}
}
