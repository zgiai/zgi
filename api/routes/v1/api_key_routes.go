package v1

import (
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	APIKey "github.com/zgiai/zgi/api/internal/modules/api_key"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	"github.com/zgiai/zgi/api/middleware"
)

// RegisterAPIKeyRoutes registers API Key management routes
func RegisterAPIKeyRoutes(v1 *gin.RouterGroup, db *gorm.DB, accountService interfaces.AccountService, organizationService interfaces.OrganizationService) {
	// Initialize repository, service and handler
	repo := APIKey.NewAPIKeyRepository(db)
	usageLogRepo := APIKey.NewAPIKeyUsageLogRepository(db)
	service := APIKey.NewAPIKeyService(db)
	handler := APIKey.NewAPIKeyHandler(service, repo, usageLogRepo, organizationService, db)

	// API Key routes - register under agents
	agentsGroup := v1.Group("/agents")
	agentsGroup.Use(middleware.SetupRequired())
	agentsGroup.Use(middleware.JWTWithOrganizationAndService(accountService))
	agentsGroup.Use(middleware.SetAccountService(accountService))

	// API Key management endpoints under agent
	agentsGroup.GET("/:agent_id/api-keys", handler.ListAPIKeys)
	agentsGroup.POST("/:agent_id/api-keys", handler.CreateAPIKey)
	agentsGroup.GET("/:agent_id/api-keys/:api_key_id", handler.GetAPIKey)
	agentsGroup.PUT("/:agent_id/api-keys/:api_key_id", handler.UpdateAPIKey)
	agentsGroup.DELETE("/:agent_id/api-keys/:api_key_id", handler.DeleteAPIKey)
	agentsGroup.POST("/:agent_id/api-keys/:api_key_id/revoke", handler.RevokeAPIKey)

	// API Key usage endpoints
	agentsGroup.GET("/:agent_id/api-keys/:api_key_id/usage", handler.GetAPIKeyUsageLogs)
	agentsGroup.GET("/:agent_id/api-keys/:api_key_id/usage/stats", handler.GetAPIKeyUsageStats)
}
