package v1

import (
	"github.com/gin-gonic/gin"
	platformchannel "github.com/zgiai/zgi/api/internal/infra/platform/channel"
	apikeyrepo "github.com/zgiai/zgi/api/internal/modules/llm/apikey/repository"
	"github.com/zgiai/zgi/api/internal/modules/llm/gateway"
	gatewayhandler "github.com/zgiai/zgi/api/internal/modules/llm/gateway/handler"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	_ "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters/provider" // Register adapters
	"github.com/zgiai/zgi/api/pkg/logger"
	redisPkg "github.com/zgiai/zgi/api/pkg/redis"
	"gorm.io/gorm"
)

// GatewayRouteDeps contains dependencies required by gateway routes.
type GatewayRouteDeps struct {
	DB              *gorm.DB
	APIKeyRepo      apikeyrepo.APIKeyRepository
	ChannelProvider platformchannel.ChannelProvider
}

// RegisterGatewayRoutes registers OpenAI-compatible AI API routes at /v1/*
// These routes use LLM API key authentication (sk-xxx)
func RegisterGatewayRoutes(router *gin.RouterGroup, deps GatewayRouteDeps) {
	if deps.DB == nil {
		panic("gateway routes require db")
	}
	if deps.APIKeyRepo == nil {
		panic("gateway routes require api key repository")
	}

	// Initialize gateway service
	gatewayService, err := gateway.NewLLMGatewayService(
		deps.DB,
		deps.APIKeyRepo,
		adapter.GlobalFactory,
	)
	if err != nil {
		logger.Warn("failed to initialize gateway service, skipping gateway route registration", err)
		return
	}

	// Set platform channel provider
	if deps.ChannelProvider != nil {
		if gwImpl, ok := gatewayService.(interface{ SetChannelProvider(p interface{}) }); ok {
			gwImpl.SetChannelProvider(deps.ChannelProvider)
		}
	}

	// Enable Config Cache if Redis is available
	redisClient := redisPkg.GetClient()
	if redisClient != nil {
		configCache := gateway.NewConfigCache(redisClient, deps.DB, nil)
		gatewayService.SetConfigCache(configCache)
		logger.Info("gateway config cache enabled")
	}

	// Initialize handler
	llmHandler := gatewayhandler.NewLLMHandler(gatewayService)

	// Create middleware
	authMiddleware := gatewayhandler.LLMAPIKeyAuthMiddleware(deps.APIKeyRepo)

	// Register routes with authentication middleware
	// OpenAI-compatible endpoints
	v1 := router.Group("/v1")
	v1.Use(authMiddleware)
	{
		// OpenAI-compatible endpoints
		v1.POST("/chat/completions", llmHandler.ChatCompletions)
		v1.POST("/embeddings", llmHandler.Embeddings)
		v1.POST("/responses", llmHandler.CreateResponse)
		v1.POST("/images/generations", llmHandler.CreateImage)
		v1.GET("/models", llmHandler.ListModels)
		// Anthropic-compatible endpoint, matching New API and Anthropic Messages format.
		v1.POST("/messages", llmHandler.CreateAnthropicMessage)
	}
	logger.Info("OpenAI-compatible gateway routes registered", "path", "/v1/*")

	// Anthropic-compatible endpoints
	anthropic := router.Group("/anthropic")
	anthropic.Use(authMiddleware)
	{
		anthropic.POST("/v1/messages", llmHandler.CreateAnthropicMessage)
	}
	logger.Info("Anthropic-compatible gateway routes registered", "path", "/anthropic/*")
}
