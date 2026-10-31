package external

import (
	_ "embed"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/zgiai/zgi/api/internal/modules/runtimeworkspace"
)

// The DigitalHuman contract has its own source file so it can evolve without
// conflicting with the Agent external-v1 specification. Both products share
// envelope, error, pagination, and SSE event conventions.
//
//go:embed digital-human-v1.openapi.yaml
var digitalHumanV1OpenAPI []byte

//go:embed external-v1.openapi.yaml
var externalV1OpenAPI []byte

//go:embed runtime-workspace-v1.openapi.yaml
var runtimeWorkspaceV1OpenAPI []byte

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
		payload := gin.H{
			"message":                   "ZGI External API Documentation",
			"openapi":                   "/api/openapi.yaml",
			"digital_human_openapi":     "/api/digital-human-openapi.yaml",
			"runtime_workspace_openapi": "/api/runtime-workspace-openapi.yaml",
			"openapi_scope":             "Agent, DigitalHuman, and Runtime Workspace APIs",
			"sse_contract":              "OpenAPI x-zgi-sse-event-data-schemas",
			"swagger":                   "/swagger/index.html",
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
					"POST /api/v1/agents/conversations/{conversation_id}/messages/{message_id}/token-budget/{request_id}/continue",
					"POST /api/v1/agents/conversations/{conversation_id}/messages/{message_id}/resume",
				},
				"memory": []string{
					"GET|DELETE /api/v1/agents/memory",
					"GET /api/v1/agents/memory/export",
					"GET|PATCH /api/v1/agents/memory/settings",
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
				"digital_human": []string{
					"POST /api/v1/digital-humans/chat",
					"GET /api/v1/digital-humans/runs/{run_id}/events",
					"GET /api/v1/digital-humans/conversations/{conversation_id}/messages",
					"POST /api/v1/digital-humans/files/upload",
				},
				"runtime_workspace": []string{
					"GET /api/connect/v1/apps",
					"GET /api/connect/v1/apps/{app_key}",
					"POST /api/connect/v1/apps/{app_key}/chat",
					"GET|POST /api/connect/v1/apps/{app_key}/conversations",
					"GET|PATCH|DELETE /api/connect/v1/apps/{app_key}/conversations/{conversation_id}",
					"GET /api/connect/v1/apps/{app_key}/conversations/{conversation_id}/messages",
					"GET /api/connect/v1/apps/{app_key}/conversations/{conversation_id}/events",
					"POST /api/connect/v1/apps/{app_key}/conversations/{conversation_id}/stop",
					"POST /api/connect/v1/apps/{app_key}/conversations/{conversation_id}/messages/{message_id}/workflow-continuation",
					"POST /api/connect/v1/apps/{app_key}/conversations/{conversation_id}/messages/{message_id}/user-input/{request_id}/continue",
					"POST /api/connect/v1/apps/{app_key}/conversations/{conversation_id}/messages/{message_id}/token-budget/{request_id}/continue",
					"POST /api/connect/v1/apps/{app_key}/conversations/{conversation_id}/messages/{message_id}/resume",
					"GET|DELETE /api/connect/v1/apps/{app_key}/memory",
					"GET /api/connect/v1/apps/{app_key}/memory/export",
					"PUT|DELETE /api/connect/v1/apps/{app_key}/memory/{key}",
					"POST|GET|DELETE /api/connect/v1/files",
				},
				"runtime_workspace_management": []string{
					"GET|POST /console/api/runtime-workspaces",
					"GET|PATCH /console/api/runtime-workspaces/{workspace_id}",
					"GET /console/api/runtime-workspaces/{workspace_id}/credentials",
					"POST /console/api/runtime-workspaces/{workspace_id}/credentials (Idempotency-Key required)",
					"POST /console/api/runtime-workspaces/{workspace_id}/credentials/{credential_id}/revoke",
					"GET /console/api/runtime-workspaces/{workspace_id}/agent-candidates",
					"GET|PUT /console/api/runtime-workspaces/{workspace_id}/agents",
					"GET /console/api/runtime-workspaces/{workspace_id}/audit-events",
				},
			},
			"authentication": "Use X-API-Key or Authorization: Bearer <api_key>; Runtime Workspace routes also require X-ZGI-External-User-ID for app and file operations",
		}
		if !runtimeworkspace.RuntimeEnabled() {
			delete(payload, "runtime_workspace_openapi")
			payload["openapi_scope"] = "Agent and DigitalHuman APIs"
			endpoints := payload["endpoints"].(gin.H)
			delete(endpoints, "runtime_workspace")
			delete(endpoints, "runtime_workspace_management")
		}
		c.JSON(http.StatusOK, payload)
	})

	r.GET("/openapi.yaml", func(c *gin.Context) {
		c.Data(http.StatusOK, "application/yaml; charset=utf-8", externalV1OpenAPI)
	})
	r.GET("/digital-human-openapi.yaml", func(c *gin.Context) {
		c.Data(http.StatusOK, "application/yaml; charset=utf-8", digitalHumanV1OpenAPI)
	})
	r.GET("/runtime-workspace-openapi.yaml", func(c *gin.Context) {
		if !runtimeworkspace.RuntimeEnabled() {
			c.Status(http.StatusNotFound)
			return
		}
		c.Data(http.StatusOK, "application/yaml; charset=utf-8", runtimeWorkspaceV1OpenAPI)
	})
}
