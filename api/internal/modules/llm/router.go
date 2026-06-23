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
		providersAdmin := providers.Group("")
		providersAdmin.Use(middleware.EnterpriseAdminOrOwnerRequired())
		providersAdmin.POST("", m.ProviderHandler.CreateGlobal)
		providers.GET("", m.ProviderHandler.ListGlobal)
		providers.GET("/:id", m.ProviderHandler.GetGlobal)
		providersAdmin.PUT("/:id", m.ProviderHandler.UpdateGlobal)
		providersAdmin.DELETE("/:id", m.ProviderHandler.DeleteGlobal)
	}

	// Official endpoints - separate routes to avoid /:id conflicts
	r.GET("/official-models", m.ModelHandler.ListOfficialModels)

	// Model management - Admin global CRUD only
	models := r.Group("/models")
	{
		modelsAdmin := models.Group("")
		modelsAdmin.Use(middleware.EnterpriseAdminOrOwnerRequired())
		modelsAdmin.POST("", m.ModelHandler.CreateGlobal)
		models.GET("", m.ModelHandler.ListGlobal)
		models.GET("/:id", m.ModelHandler.GetGlobal)
		modelsAdmin.PUT("/:id", m.ModelHandler.UpdateGlobal)
		modelsAdmin.DELETE("/:id", m.ModelHandler.DeleteGlobal)
	}

	// Provider management (tenant context)
	tenantProviders := r.Group("/providers")
	{
		tenantProvidersAdmin := tenantProviders.Group("")
		tenantProvidersAdmin.Use(middleware.EnterpriseAdminOrOwnerRequired())
		tenantProviders.GET("", m.ProviderHandler.ListTenantProviders)
		tenantProvidersAdmin.POST("/toggle", m.ProviderHandler.ToggleProvider)
		tenantProviders.GET("/:provider", m.ProviderHandler.GetProviderDetail)
		tenantProvidersAdmin.POST("/:provider/models/toggle", m.ProviderHandler.ToggleModel)
		tenantProvidersAdmin.POST("/config", m.ProviderHandler.ConfigureProvider)
		tenantProviders.GET("/config/:provider_id", m.ProviderHandler.GetProviderConfig)
		tenantProviders.GET("/configs", m.ProviderHandler.ListProviderConfigs)
		tenantProviders.GET("/custom", m.ProviderHandler.ListCustomProviders)
		tenantProvidersAdmin.POST("/custom", m.ProviderHandler.CreateCustom)
		tenantProviders.GET("/custom/:id", m.ProviderHandler.GetCustom)
		tenantProvidersAdmin.PUT("/custom/:id", m.ProviderHandler.UpdateCustom)
		tenantProvidersAdmin.DELETE("/custom/:id", m.ProviderHandler.DeleteCustom)
	}

	// Model management (tenant context)
	tenantModels := r.Group("/models")
	{
		tenantModelsAdmin := tenantModels.Group("")
		tenantModelsAdmin.Use(middleware.EnterpriseAdminOrOwnerRequired())
		// Available models API (optimized for business use with caching)
		tenantModels.GET("/available", m.LLMModelModule.AvailableModelsHandler.ListAvailable)
		tenantModelsAdmin.POST("/available/refresh", m.LLMModelModule.AvailableModelsHandler.RefreshCache)

		tenantModels.GET("", m.ModelHandler.ListTenantModels)
		tenantModels.GET("/parameters", m.ModelHandler.GetModelParameters)
		tenantModelsAdmin.POST("/provider/toggle", m.ModelHandler.ToggleProviderModels)
		tenantModelsAdmin.POST("/batch/toggle", m.ModelHandler.BatchToggleModels)
		tenantModelsAdmin.POST("/config", m.ModelHandler.ConfigureModel)
		tenantModels.GET("/config/:model_id", m.ModelHandler.GetModelConfig)
		tenantModels.GET("/configs", m.ModelHandler.ListModelConfigs)
		tenantModels.GET("/custom", m.ModelHandler.ListCustomModels)
		tenantModelsAdmin.POST("/custom", m.ModelHandler.CreateCustom)
		tenantModels.GET("/custom/:id", m.ModelHandler.GetCustom)
		tenantModelsAdmin.PUT("/custom/:id", m.ModelHandler.UpdateCustom)
		tenantModelsAdmin.DELETE("/custom/:id", m.ModelHandler.DeleteCustom)
		tenantModels.GET("/:id/availability", m.ModelHandler.CheckAvailability)
		tenantModels.POST("/availability/batch", m.ModelHandler.BatchCheckAvailability)
	}

	// Channels management (tenant context)
	tenantChannels := r.Group("/channels")
	{
		tenantChannelsAdmin := tenantChannels.Group("")
		tenantChannelsAdmin.Use(middleware.EnterpriseAdminOrOwnerRequired())
		tenantChannelsAdmin.POST("", m.ChannelHandler.CreateRoute)
		tenantChannels.GET("", m.ChannelHandler.ListRoutesAggregated) // Unified list with summary (excludes ZGI_CLOUD)
		tenantChannels.GET("/all", m.ChannelHandler.ListRoutes)       // Raw paginated list
		tenantChannels.GET("/:id", m.ChannelHandler.GetRoute)
		tenantChannelsAdmin.PUT("/:id", m.ChannelHandler.UpdateRoute)
		tenantChannelsAdmin.DELETE("/:id", m.ChannelHandler.DeleteRoute)
		tenantChannelsAdmin.POST("/:id/toggle", m.ChannelHandler.ToggleRoute)
		tenantChannelsAdmin.POST("/:id/test", m.ChannelHandler.TestRoute)
		tenantChannelsAdmin.PUT("/:id/balance", m.ChannelHandler.UpdateChannelBalance)
		tenantChannelsWalletAdmin := tenantChannels.Group("")
		tenantChannelsWalletAdmin.Use(channelhandler.AdjustChannelWalletAdminRequired())
		tenantChannelsWalletAdmin.POST("/:id/wallet/adjust", m.ChannelHandler.AdjustChannelWallet)
		tenantChannels.POST("/select", m.ChannelHandler.SelectRoute)
		tenantChannels.GET("/by-model", m.ChannelHandler.GetRoutesForModel)
		tenantChannelsAdmin.POST("/init", m.ChannelHandler.InitTenantRoutes)
		tenantChannelsAdmin.POST("/draft/discover-models", m.ChannelHandler.DiscoverDraftChannelModels)
		tenantChannelsAdmin.POST("/draft/test/model", m.ChannelHandler.TestDraftChannelModel)
		tenantChannelsAdmin.POST("/ollama/discover-models", m.ChannelHandler.DiscoverOllamaModels)
		tenantChannelsAdmin.POST("/:id/test/model", m.ChannelHandler.TestChannelModel)
		tenantChannelsAdmin.POST("/:id/test/batch", m.ChannelHandler.BatchTestChannelModels)

		// Cloud-only: platform channel management (ZGI_EDITION=CLOUD)
		if m.IsCloudMode {
			tenantChannels.GET("/platform", m.ChannelHandler.GetPlatformChannel)
			tenantChannelsAdmin.PUT("/platform", m.ChannelHandler.UpdatePlatformChannelSettings)
			tenantChannelsAdmin.PATCH("/platform/:id", m.ChannelHandler.UpdatePlatformChannel)
			tenantChannelsAdmin.POST("/official/init", m.ChannelHandler.InitOfficialChannel)
			tenantChannelsAdmin.PUT("/official/:group_id/settings", m.ChannelHandler.UpdateOfficialChannelSettings)
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
