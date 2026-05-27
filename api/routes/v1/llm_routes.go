package v1

import (
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	pconsole "github.com/zgiai/zgi/api/internal/infra/platform/console"
	"github.com/zgiai/zgi/api/internal/modules/llm"
	apikeyrepo "github.com/zgiai/zgi/api/internal/modules/llm/apikey/repository"
	"github.com/zgiai/zgi/api/internal/modules/llm/client"
	"github.com/zgiai/zgi/api/internal/modules/llm/gateway"
	"github.com/zgiai/zgi/api/internal/modules/llm/handler"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/modules/llm/shared"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	"github.com/zgiai/zgi/api/middleware"
	"github.com/zgiai/zgi/api/pkg/logger"
	redisPkg "github.com/zgiai/zgi/api/pkg/redis"
)

type LLMRouteDeps struct {
	DB                         *gorm.DB
	AccountService             interfaces.AccountService
	WorkspaceManagementService interfaces.WorkspaceManagementService
	OrganizationService        interfaces.OrganizationService
	ConsoleProvider            pconsole.ConsoleProvider
}

// RegisterLLMRoutes registers all LLM-related routes
// Includes: /llm/* (internal for workflows/knowledge base)
func RegisterLLMRoutes(router *gin.RouterGroup, deps LLMRouteDeps) *llm.LLMModule {
	validateLLMRouteDeps(deps)

	// ========== Initialize V2 Module ==========
	cryptoService, err := shared.DefaultCryptoService()
	if err != nil {
		logger.Error("failed to create LLM crypto service", err)
		return nil
	}
	llmV2Module := llm.NewLLMModule(
		deps.DB,
		cryptoService,
		deps.WorkspaceManagementService,
		deps.AccountService,
		deps.OrganizationService,
		deps.ConsoleProvider,
	)

	// ========== Initialize Internal AI Service (for workflows/knowledge base) ==========
	llmAPIKeyRepo := apikeyrepo.NewAPIKeyRepository(deps.DB)

	gatewayService, err := gateway.NewLLMGatewayService(
		deps.DB,
		llmAPIKeyRepo,
		adapter.GlobalFactory,
	)
	if err != nil {
		logger.Warn("failed to initialize LLM gateway service, skipping internal route registration", err)
		return llmV2Module
	}

	// ===== Performance Optimizations =====
	// 1. Enable Config Cache (Model/Provider/ShadowTenant)
	redisClient := redisPkg.GetClient()
	if redisClient != nil {
		configCache := gateway.NewConfigCache(redisClient, deps.DB, nil)
		gatewayService.SetConfigCache(configCache)
		logger.Info("LLM config cache enabled")
	}

	// ===== End Performance Optimizations =====

	llmClient := client.New(gatewayService, llmAPIKeyRepo, deps.DB)
	llmInternalHandler := handler.NewLLMInternalHandler(llmClient)

	// ========== System /console/api/llm/modelmeta/* Routes ==========
	// ModelMeta is system-scoped and must not depend on workspace resolution.
	modelMetaGroup := router.Group("")
	modelMetaGroup.Use(middleware.JWT())
	llm.RegisterModelMetaRoutes(modelMetaGroup, llmV2Module)
	logger.Info("LLM model metadata routes registered", "path", "/console/api/llm/modelmeta/*")

	// ========== Console /console/api/llm/* Routes ==========
	// Register console routes for tenant LLM management
	// Note: router is already /console/api, RegisterConsoleRoutes will add /llm
	// Apply JWT middleware to set tenant_id in context
	consoleGroup := router.Group("")
	consoleGroup.Use(middleware.JWTWithOrganizationAndService(deps.AccountService))
	llm.RegisterConsoleRoutes(consoleGroup, llmV2Module)
	logger.Info("LLM console routes registered", "path", "/console/api/llm/*")

	// ========== Legacy /llm/* Routes (using V2 module) ==========
	// Note: These are for internal use by workflows/knowledge base
	// Console routes are registered separately via RegisterConsoleRoutes above
	llmGroup := router.Group("/llm")
	llmGroup.Use(middleware.JWTWithOrganizationAndService(deps.AccountService))

	// Internal AI Routes (for workflows/knowledge base)
	llmGroup.POST("/chat/completions", llmInternalHandler.ChatCompletions)
	llmGroup.POST("/embeddings", llmInternalHandler.Embeddings)
	llmGroup.POST("/rerank", llmInternalHandler.Rerank)
	llmGroup.POST("/responses", llmInternalHandler.CreateResponse)

	logger.Info("LLM legacy internal routes registered", "path", "/llm/*")
	return llmV2Module
}

func validateLLMRouteDeps(deps LLMRouteDeps) {
	if deps.DB == nil {
		panic("llm routes require db")
	}
	if deps.AccountService == nil {
		panic("llm routes require account service")
	}
	if deps.WorkspaceManagementService == nil {
		panic("llm routes require workspace management service")
	}
	if deps.OrganizationService == nil {
		panic("llm routes require organization service")
	}
	if deps.ConsoleProvider == nil {
		panic("llm routes require console provider")
	}
}
