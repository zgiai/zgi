package llm

import (
	"github.com/gin-gonic/gin"
	apikeyhandler "github.com/zgiai/zgi/api/internal/modules/llm/apikey/handler"
	availhandler "github.com/zgiai/zgi/api/internal/modules/llm/availability/handler"
	channelhandler "github.com/zgiai/zgi/api/internal/modules/llm/channel/handler"
	credentialhandler "github.com/zgiai/zgi/api/internal/modules/llm/credential/handler"
	defaultmodelhandler "github.com/zgiai/zgi/api/internal/modules/llm/defaultmodel/handler"
	llmmodelhandler "github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/handler"
	providerhandler "github.com/zgiai/zgi/api/internal/modules/llm/provider/handler"
	statisticshandler "github.com/zgiai/zgi/api/internal/modules/llm/statistics/handler"
	workspacequotahandler "github.com/zgiai/zgi/api/internal/modules/llm/workspacequota/handler"
	middleware "github.com/zgiai/zgi/api/middleware"
)

// RegisterConsoleRoutes registers all console/tenant routes
// Base path: /console/llm
func RegisterConsoleRoutes(r *gin.RouterGroup, m *LLMModule) {
	llm := r.Group("/llm")
	{
		// Routes below require organization context.
		llmWithOrg := llm.Group("")
		llmWithOrg.Use(providerhandler.ExtractOrganizationID(m.DB))

		// Credential management
		credentialhandler.RegisterTenantCredentialRoutes(llmWithOrg, m.TenantCredentialHandler)

		// Provider management (new modular structure)
		providerhandler.RegisterTenantProviderRoutes(llmWithOrg, m.ProviderHandler, m.DB)

		// Model management (new modular structure)
		llmmodelhandler.RegisterTenantModelRoutes(llmWithOrg, m.ModelHandler, m.LLMModelModule.AvailableModelsHandler)
		if m.DefaultModelHandler != nil {
			defaultmodelhandler.RegisterTenantDefaultModelRoutes(llmWithOrg, m.DefaultModelHandler)
		}

		// Model availability checks
		if m.AvailabilityHandler != nil {
			availhandler.RegisterAvailabilityRoutes(llm, m.AvailabilityHandler)
		}

		// Route management
		channelhandler.RegisterTenantChannelRoutes(llmWithOrg, m.ChannelHandler)

		// API Key management (new modular structure)
		if m.APIKeyHandler != nil {
			apikeyhandler.RegisterAPIKeyRoutes(llmWithOrg, m.APIKeyHandler)
		}

		// Statistics
		if m.StatisticsHandler != nil {
			statisticshandler.RegisterStatisticsRoutes(llmWithOrg, m.StatisticsHandler)
		}

		// Workspace quota management
		if m.WorkspaceQuotaHandler != nil {
			workspaceQuotaAdmin := llmWithOrg.Group("")
			workspaceQuotaAdmin.Use(middleware.EnterpriseAdminOrOwnerRequired())
			workspacequotahandler.RegisterWorkspaceQuotaRoutes(workspaceQuotaAdmin, m.WorkspaceQuotaHandler)
		}

	}
}

// RegisterModelMetaRoutes registers system-scoped ModelMeta routes.
func RegisterModelMetaRoutes(r *gin.RouterGroup, m *LLMModule) {
	if m == nil || m.ModelMetaHandler == nil {
		return
	}

	modelmetaGroup := r.Group("/llm/modelmeta")
	modelmetaGroup.Use(m.ModelMetaHandler.SuperAdminRequired())
	registerModelMetaEndpoints(modelmetaGroup, m)
}

func registerModelMetaEndpoints(modelmetaGroup *gin.RouterGroup, m *LLMModule) {
	// Diff - compare local vs upstream (read-only)
	modelmetaGroup.GET("/status", m.ModelMetaHandler.GetSyncStatus)
	modelmetaGroup.GET("/diff/providers", m.ModelMetaHandler.DiffProviders)
	modelmetaGroup.GET("/diff/:provider", m.ModelMetaHandler.GetDiff)

	// Sync - write operations
	modelmetaGroup.POST("/sync/:provider", m.ModelMetaHandler.SyncProvider)
	modelmetaGroup.POST("/sync-provider-full/:provider", m.ModelMetaHandler.SyncProviderWithModels)
}

// RegisterCommonRoutes registers common /llm/* routes for internal use
// Base path: /llm (used by workflows, knowledge base, and legacy frontend)
func RegisterCommonRoutes(r *gin.RouterGroup, m *LLMModule) {
	// Tenant Credential management
	credentialhandler.RegisterTenantCredentialRoutes(r, m.TenantCredentialHandler)

	// Provider management - Admin global CRUD only
	providers := r.Group("/providers")
	{
		providers.POST("", m.ProviderHandler.CreateGlobal)
		providers.GET("", m.ProviderHandler.ListGlobal)
		providers.GET("/:id", m.ProviderHandler.GetGlobal)
		providers.PUT("/:id", m.ProviderHandler.UpdateGlobal)
		providers.DELETE("/:id", m.ProviderHandler.DeleteGlobal)
	}

	// Official endpoints - separate routes to avoid /:id conflicts
	r.GET("/official-models", m.ModelHandler.ListOfficialModels)

	// Model management - Admin global CRUD only
	models := r.Group("/models")
	{
		models.POST("", m.ModelHandler.CreateGlobal)
		models.GET("", m.ModelHandler.ListGlobal)
		models.GET("/:id", m.ModelHandler.GetGlobal)
		models.PUT("/:id", m.ModelHandler.UpdateGlobal)
		models.DELETE("/:id", m.ModelHandler.DeleteGlobal)
	}

	// Provider management (tenant context)
	tenantProviders := r.Group("/providers")
	{
		tenantProviders.GET("", m.ProviderHandler.ListTenantProviders)
		tenantProviders.POST("/toggle", m.ProviderHandler.ToggleProvider)
		tenantProviders.GET("/:provider", m.ProviderHandler.GetProviderDetail)
		tenantProviders.POST("/:provider/models/toggle", m.ProviderHandler.ToggleModel)
		tenantProviders.POST("/config", m.ProviderHandler.ConfigureProvider)
		tenantProviders.GET("/config/:provider_id", m.ProviderHandler.GetProviderConfig)
		tenantProviders.GET("/configs", m.ProviderHandler.ListProviderConfigs)
		tenantProviders.GET("/custom", m.ProviderHandler.ListCustomProviders)
		tenantProviders.POST("/custom", m.ProviderHandler.CreateCustom)
		tenantProviders.GET("/custom/:id", m.ProviderHandler.GetCustom)
		tenantProviders.PUT("/custom/:id", m.ProviderHandler.UpdateCustom)
		tenantProviders.DELETE("/custom/:id", m.ProviderHandler.DeleteCustom)
	}

	// Model management (tenant context)
	tenantModels := r.Group("/models")
	{
		// Available models API (optimized for business use with caching)
		tenantModels.GET("/available", m.LLMModelModule.AvailableModelsHandler.ListAvailable)
		tenantModels.POST("/available/refresh", m.LLMModelModule.AvailableModelsHandler.RefreshCache)

		tenantModels.GET("", m.ModelHandler.ListTenantModels)
		tenantModels.GET("/parameters", m.ModelHandler.GetModelParameters)
		tenantModels.POST("/provider/toggle", m.ModelHandler.ToggleProviderModels)
		tenantModels.POST("/batch/toggle", m.ModelHandler.BatchToggleModels)
		tenantModels.POST("/config", m.ModelHandler.ConfigureModel)
		tenantModels.GET("/config/:model_id", m.ModelHandler.GetModelConfig)
		tenantModels.GET("/configs", m.ModelHandler.ListModelConfigs)
		tenantModels.GET("/custom", m.ModelHandler.ListCustomModels)
		tenantModels.POST("/custom", m.ModelHandler.CreateCustom)
		tenantModels.GET("/custom/:id", m.ModelHandler.GetCustom)
		tenantModels.PUT("/custom/:id", m.ModelHandler.UpdateCustom)
		tenantModels.DELETE("/custom/:id", m.ModelHandler.DeleteCustom)
		tenantModels.GET("/:id/availability", m.ModelHandler.CheckAvailability)
		tenantModels.POST("/availability/batch", m.ModelHandler.BatchCheckAvailability)
	}

	// Channels management (tenant context)
	tenantChannels := r.Group("/channels")
	{
		tenantChannels.POST("", m.ChannelHandler.CreateRoute)
		tenantChannels.GET("", m.ChannelHandler.ListRoutesAggregated) // Unified list with summary (excludes ZGI_CLOUD)
		tenantChannels.GET("/all", m.ChannelHandler.ListRoutes)       // Raw paginated list
		tenantChannels.GET("/:id", m.ChannelHandler.GetRoute)
		tenantChannels.PUT("/:id", m.ChannelHandler.UpdateRoute)
		tenantChannels.DELETE("/:id", m.ChannelHandler.DeleteRoute)
		tenantChannels.POST("/:id/toggle", m.ChannelHandler.ToggleRoute)
		tenantChannels.POST("/:id/test", m.ChannelHandler.TestRoute)
		tenantChannels.PUT("/:id/balance", m.ChannelHandler.UpdateChannelBalance)
		tenantChannelsAdmin := tenantChannels.Group("")
		tenantChannelsAdmin.Use(channelhandler.AdjustChannelWalletAdminRequired())
		tenantChannelsAdmin.POST("/:id/wallet/adjust", m.ChannelHandler.AdjustChannelWallet)
		tenantChannels.POST("/select", m.ChannelHandler.SelectRoute)
		tenantChannels.GET("/by-model", m.ChannelHandler.GetRoutesForModel)
		tenantChannels.POST("/init", m.ChannelHandler.InitTenantRoutes)
		tenantChannels.POST("/draft/discover-models", m.ChannelHandler.DiscoverDraftChannelModels)
		tenantChannels.POST("/draft/test/model", m.ChannelHandler.TestDraftChannelModel)
		tenantChannels.POST("/ollama/discover-models", m.ChannelHandler.DiscoverOllamaModels)
		tenantChannels.POST("/:id/test/model", m.ChannelHandler.TestChannelModel)
		tenantChannels.POST("/:id/test/batch", m.ChannelHandler.BatchTestChannelModels)

		// Cloud-only: platform channel management (ZGI_EDITION=CLOUD)
		if m.IsCloudMode {
			tenantChannels.GET("/platform", m.ChannelHandler.GetPlatformChannel)
			tenantChannels.PUT("/platform", m.ChannelHandler.UpdatePlatformChannelSettings)
			tenantChannels.PATCH("/platform/:id", m.ChannelHandler.UpdatePlatformChannel)
			tenantChannels.POST("/official/init", m.ChannelHandler.InitOfficialChannel)
			tenantChannels.PUT("/official/:group_id/settings", m.ChannelHandler.UpdateOfficialChannelSettings)
		}
	}

	// API Key management
	if m.APIKeyHandler != nil {
		apikeyhandler.RegisterAPIKeyRoutes(r, m.APIKeyHandler)
	}

	// Statistics
	if m.StatisticsHandler != nil {
		statisticshandler.RegisterStatisticsRoutes(r, m.StatisticsHandler)
	}

	// Workspace quota management
	if m.WorkspaceQuotaHandler != nil {
		workspaceQuotaAdmin := r.Group("")
		workspaceQuotaAdmin.Use(middleware.EnterpriseAdminOrOwnerRequired())
		workspacequotahandler.RegisterWorkspaceQuotaRoutes(workspaceQuotaAdmin, m.WorkspaceQuotaHandler)
	}
}
