package external

import (
	"github.com/gin-gonic/gin"
	"github.com/zgiai/zgi/api/internal/container"
)

// RegisterExternalRoutes registers all external API routes
func RegisterExternalRoutes(r *gin.Engine, serviceContainer *container.ServiceContainer) {
	// External API v1 routes
	externalV1 := r.Group("/api")
	{
		// Public APIs that don't require authentication
		RegisterPublicRoutes(externalV1, serviceContainer)

		// APIs that require API key authentication
		RegisterAPIKeyRoutes(externalV1, serviceContainer.GetDB(), serviceContainer.GetAccountService(), serviceContainer.GetFileService(), serviceContainer.GetContentExtractor(), serviceContainer.GetQuotaService(), serviceContainer.GetOrganizationService(), serviceContainer.GetLLMClient(), serviceContainer.GetToolEngine(), serviceContainer.GetGraphFlowService(), serviceContainer.GetPromptService(), serviceContainer.GetWorkflowEngineFactory())
	}

	// External API v2 routes (future)
	// externalV2 := r.Group("/api/v2")
}
