package external

import (
	"github.com/gin-gonic/gin"
	"github.com/zgiai/ginext/internal/container"
)

// RegisterPublicRoutes registers public API routes that don't require authentication
func RegisterPublicRoutes(r *gin.RouterGroup, serviceContainer *container.ServiceContainer) {
	// Health check for external API
	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"status":  "ok",
			"service": "zgi-external-api",
			"version": "1.0",
		})
	})

	// API documentation endpoint
	r.GET("/docs", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"message": "ZGI External API Documentation",
			"endpoints": gin.H{
				"health":    "/api/health",
				"agents":    "/api/v1/agents (requires API key)",
				"workflows": "/api/v1/workflows (requires API key)",
				"chat":      "/api/v1/chat (requires API key)",
			},
			"authentication": "Use X-API-Key header or Authorization: Bearer <api_key>",
		})
	})

}
