package v1

import (
	"github.com/gin-gonic/gin"
	"github.com/zgiai/ginext/internal/container"
	"github.com/zgiai/ginext/internal/modules/llm/gateway"
	gatewayhandler "github.com/zgiai/ginext/internal/modules/llm/gateway/handler"
	adapter "github.com/zgiai/ginext/internal/modules/llm/protocol/adapters"
	_ "github.com/zgiai/ginext/internal/modules/llm/protocol/adapters/provider" // Register adapters
	"github.com/zgiai/ginext/pkg/logger"
	redisPkg "github.com/zgiai/ginext/pkg/redis"
)

// RegisterGatewayRoutes registers OpenAI-compatible AI API routes at /v1/*
// These routes use LLM API key authentication (sk-xxx)
func RegisterGatewayRoutes(router *gin.RouterGroup, serviceContainer *container.ServiceContainer) {
	db := serviceContainer.GetDB()

	// Initialize repositories
	apiKeyRepo := serviceContainer.GetLLMAPIKeyRepository()

	// Initialize gateway service
	gatewayService, err := gateway.NewLLMGatewayService(
		db,
		apiKeyRepo,
		adapter.GlobalFactory,
	)
	if err != nil {
		logger.Warn("failed to initialize gateway service, skipping gateway route registration", err)
		return
	}

	// Set platform channel provider
	platformChannels, err := serviceContainer.GetPlatformChannels()
	if err != nil {
		logger.Warn("failed to load platform channels, skipping channel provider wiring", err)
	} else if platformChannels.Channel != nil {
		if gwImpl, ok := gatewayService.(interface{ SetChannelProvider(p interface{}) }); ok {
			gwImpl.SetChannelProvider(platformChannels.Channel)
		}
	}

	// Enable Config Cache if Redis is available
	redisClient := redisPkg.GetClient()
	if redisClient != nil {
		configCache := gateway.NewConfigCache(redisClient, db, nil)
		gatewayService.SetConfigCache(configCache)
		logger.Info("gateway config cache enabled")
	}

	// Initialize handler
	llmHandler := gatewayhandler.NewLLMHandler(gatewayService)

	// Create middleware
	authMiddleware := gatewayhandler.LLMAPIKeyAuthMiddleware(apiKeyRepo)

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
