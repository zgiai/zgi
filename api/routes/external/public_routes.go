package external

import (
	_ "embed"
	"net/http"

	"github.com/gin-gonic/gin"
)

//go:embed external-v1.openapi.yaml
var externalV1OpenAPI []byte

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
			"openapi":       "/api/openapi.yaml",
			"openapi_scope": "Agent API",
			"sse_contract":  "OpenAPI x-zgi-sse-event-data-schemas",
			"swagger":       "/swagger/index.html",
			"endpoints": gin.H{
				"agent": []string{
					"GET /api/v1/agents/config",
					"POST /api/v1/agents/chat",
					"GET /api/v1/agents/conversations",
					"GET /api/v1/agents/conversations/search",
					"GET|PATCH|DELETE /api/v1/agents/conversations/{conversation_id}",
					"GET /api/v1/agents/conversations/{conversation_id}/messages",
					"GET /api/v1/agents/conversations/{conversation_id}/events",
					"POST /api/v1/agents/conversations/{conversation_id}/stop",
					"POST /api/v1/agents/messages/{message_id}/regenerate",
					"POST /api/v1/agents/conversations/{conversation_id}/messages/{message_id}/workflow-continuation",
					"POST /api/v1/agents/conversations/{conversation_id}/messages/{message_id}/user-input/{request_id}/continue",
				},
				"memory": []string{
					"GET|DELETE /api/v1/agents/memory",
					"GET /api/v1/agents/memory/export",
					"PUT|DELETE /api/v1/agents/memory/{key}",
					"POST /api/v1/agents/memory/operations/{operation_id}/undo",
				},
				"files_and_metadata": []string{
					"POST /api/v1/files/upload",
					"GET /api/v1/info",
					"GET /api/v1/parameters",
				},
				"workflow": []string{
					"POST /api/v1/workflows/run",
					"POST /api/v1/workflows/{workflow_id}/run",
					"GET /api/v1/workflows/runs/{run_id}",
					"GET /api/v1/workflows/runs/{run_id}/events",
					"POST /api/v1/workflows/tasks/{task_id}/stop",
					"POST /api/v1/chat-workflows/run",
				},
			},
			"authentication": "Use X-API-Key header or Authorization: Bearer <api_key>",
		})
	})

	r.GET("/openapi.yaml", func(c *gin.Context) {
		c.Data(http.StatusOK, "application/yaml; charset=utf-8", externalV1OpenAPI)
	})
}
