package routes

import (
	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
	"github.com/zgiai/zgi/api/internal/container"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/graph_engine"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/tool_file"
	contentparsemodule "github.com/zgiai/zgi/api/internal/modules/contentparse"
	"github.com/zgiai/zgi/api/middleware"
	"github.com/zgiai/zgi/api/pkg/logger"
	external "github.com/zgiai/zgi/api/routes/external"
	v1 "github.com/zgiai/zgi/api/routes/v1"
	"gorm.io/gorm"
)

// RegisterRoutes registers all routes
func RegisterRoutes(r *gin.Engine, serviceContainer *container.ServiceContainer, workflowEngineFactory *graph_engine.EngineFactory) {
	// Fail fast in CLOUD mode if LLM client cannot be initialized.
	if err := serviceContainer.EnsureLLMClient(); err != nil {
		logger.Fatal("BOOT_LLMCLIENT_INIT_FAILED: %v", err)
	}

	// Swagger documentation
	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	// Root endpoint - Welcome message
	r.GET("/", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"message": "Welcome to ZGI API Server",
			"version": "1.0",
			"endpoints": gin.H{
				"health": "/ping",
			},
		})
	})

	// Root health check
	r.GET("/ping", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"message": "pong",
		})
	})

	// Backward-compatible tool file download route for legacy signed URLs.
	toolFileHandler := tool_file.NewHTTPHandler(tool_file.GlobalToolFileManager)
	r.GET("/files/tools/:tool_file_id", toolFileHandler.GetToolFile)

	// API v1 routes
	v1Group := r.Group("/console/api")
	v1.RegisterRoutes(r, v1Group, serviceContainer, workflowEngineFactory)

	external.RegisterExternalRoutes(r, external.ExternalRouteDeps{
		DB:                    serviceContainer.GetDB(),
		AccountService:        serviceContainer.GetAccountService(),
		FileService:           serviceContainer.GetFileService(),
		ContentExtractor:      serviceContainer.GetContentExtractor(),
		QuotaService:          serviceContainer.GetQuotaService(),
		OrganizationService:   serviceContainer.GetOrganizationService(),
		LLMClient:             serviceContainer.GetLLMClient(),
		ToolEngine:            serviceContainer.GetToolEngine(),
		GraphFlowService:      serviceContainer.GetGraphFlowService(),
		PromptResolver:        serviceContainer.GetPromptService(),
		WorkflowEngineFactory: workflowEngineFactory,
	})

	registerConsoleInternalRoutes(r, serviceContainer.GetDB())

	// OpenAI-compatible LLM Gateway API (/v1/*)
	// Uses API Key authentication (sk-xxx)
	// Note: RegisterGatewayRoutes internally creates /v1 group
	gatewayDeps := v1.GatewayRouteDeps{
		DB:         serviceContainer.GetDB(),
		APIKeyRepo: serviceContainer.GetLLMAPIKeyRepository(),
	}
	platformChannels, err := serviceContainer.GetPlatformChannels()
	if err != nil {
		logger.Warn("failed to load platform channels, skipping channel provider wiring", err)
	} else {
		gatewayDeps.ChannelProvider = platformChannels.Channel
	}
	v1.RegisterGatewayRoutes(r.Group(""), gatewayDeps)
}

func registerConsoleInternalRoutes(r *gin.Engine, db *gorm.DB) {
	// Internal API routes for console-api callbacks (HMAC-signed)
	internalGroup := r.Group("/console/api/internal")
	internalGroup.Use(middleware.ConsoleAPIAuth())
	{
		if db != nil {
			contentParseModule := contentparsemodule.NewModule(db)
			contentParseModule.RegisterInternalRoutes(internalGroup)
		}
	}
}
