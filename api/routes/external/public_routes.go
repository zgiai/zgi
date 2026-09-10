package external

import (
	_ "embed"
	"net/http"

	"github.com/gin-gonic/gin"
)

// The DigitalHuman contract has its own source file so it can evolve without
// conflicting with the Agent external-v1 specification. Both products share
// envelope, error, pagination, and SSE event conventions.
//go:embed digital-human-v1.openapi.yaml
var digitalHumanV1OpenAPI []byte

// RegisterPublicRoutes registers public API routes that don't require authentication
func RegisterPublicRoutes(r *gin.RouterGroup) {
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
		c.JSON(http.StatusOK, gin.H{
			"message":       "ZGI External API Documentation",
			"openapi":       "/api/digital-human-openapi.yaml",
			"openapi_scope": "DigitalHuman external-v1 contract aligned with Agent conventions",
			"sse_contract":  "OpenAPI x-zgi-sse-event-data-schemas",
			"endpoints": gin.H{
				"health":         "/api/health",
				"agents":         "/api/v1/agents (requires Agent API key)",
				"digital_humans": "/api/v1/digital-humans (requires DigitalHuman API key)",
				"workflows":      "/api/v1/workflows (requires Agent API key)",
			},
			"authentication": "Use Authorization: Bearer <api_key>",
		})
	})

	r.GET("/openapi.yaml", func(c *gin.Context) {
		c.Data(http.StatusOK, "application/yaml; charset=utf-8", digitalHumanV1OpenAPI)
	})
	r.GET("/digital-human-openapi.yaml", func(c *gin.Context) {
		c.Data(http.StatusOK, "application/yaml; charset=utf-8", digitalHumanV1OpenAPI)
	})
}
