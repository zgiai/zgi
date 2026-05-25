package v1

import (
	"github.com/gin-gonic/gin"
	runtimerepo "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/repository"
	runtimeservice "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/service"
	app "github.com/zgiai/zgi/api/internal/modules/app/agents"
	workflow "github.com/zgiai/zgi/api/internal/modules/app/workflow"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/graph_engine"
	workflowtest "github.com/zgiai/zgi/api/internal/modules/app/workflowtest"
	"github.com/zgiai/zgi/api/internal/modules/dataset/graphflow"
	llmclient "github.com/zgiai/zgi/api/internal/modules/llm/client"
	memorymodule "github.com/zgiai/zgi/api/internal/modules/memory"
	promptservice "github.com/zgiai/zgi/api/internal/modules/prompts/service"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	"github.com/zgiai/zgi/api/internal/modules/skills"
	"github.com/zgiai/zgi/api/internal/modules/tools"
	"github.com/zgiai/zgi/api/middleware"
	"gorm.io/gorm"
)

func RegisterAgentsRoutes(v1 *gin.RouterGroup, db *gorm.DB, accountService interfaces.AccountService, tenantService interfaces.WorkspaceManagementService, resourcePermissionService interfaces.ResourcePermissionService, enterpriseService interfaces.OrganizationService, quotaService interfaces.QuotaService, fileService interfaces.FileService, contentExtractor runtimeservice.ContentExtractionService, llmClient llmclient.LLMClient, toolEngine *tools.ToolEngine, toolManager *tools.ToolManager, memoryService *memorymodule.Service, graphFlowService *graphflow.Service, promptResolver promptservice.PromptService, engineFactory *graph_engine.EngineFactory) {
	repo := app.NewAgentsRepository(db)

	// Initialize workflow service for agents with all required dependencies
	// Including LLMClient for knowledge retrieval embedding support
	workflowRepo := workflow.NewWorkflowRepository(db)
	workflowRunLogRepo := workflow.NewWorkflowRunLogRepository(db)
	workflowNodeRuntimeLogRepo := workflow.NewWorkflowNodeRuntimeLogRepository(db)
	workflowService := workflow.NewWorkflowServiceWithContentExtractor(
		workflowRepo,
		repo,
		workflowRunLogRepo,
		workflowNodeRuntimeLogRepo,
		accountService,
		fileService,
		contentExtractor,
		quotaService,
		enterpriseService,
		llmClient,
		toolEngine,
		graphFlowService,
		promptResolver,
		nil,
		engineFactory,
	)

	chatRuntimeService := runtimeservice.NewServiceWithSkillRuntime(
		runtimerepo.NewRepositories(db),
		llmClient,
		nil,
		runtimeservice.NewDatabaseModelSpecResolver(db),
		fileService,
		contentExtractor,
		enterpriseService,
		skills.NewRuntime(toolEngine, toolManager),
		memoryService,
	)
	service := app.NewAgentsService(repo, accountService, tenantService, workflowService, chatRuntimeService, resourcePermissionService, enterpriseService, quotaService, fileService, db)
	appHandler := app.NewAgentsHandler(service, tenantService, accountService, enterpriseService, db, chatRuntimeService)
	workflowTestService := workflowtest.NewService(workflowtest.NewRepository(db))
	workflowTestHandler := workflowtest.NewHandler(workflowTestService, workflowService, enterpriseService, llmClient)

	appsGroup := v1.Group("/agents")
	appsGroup.Use(middleware.SetupRequired())
	appsGroup.Use(middleware.JWTWithOrganizationAndService(accountService))
	appsGroup.Use(middleware.SetAccountService(accountService))

	// Agent management endpoints
	appsGroup.GET("", appHandler.GetAgentsList)
	appsGroup.GET("/runnable-webapps", appHandler.GetRunnableWebApps)
	appsGroup.POST("", appHandler.CreateAgent)
	appsGroup.GET("/:agent_id", appHandler.GetAgent)
	appsGroup.GET("/:agent_id/config", appHandler.GetAgentConfig)
	appsGroup.PUT("/:agent_id/config", appHandler.UpdateAgentConfig)
	appsGroup.POST("/:agent_id/publish", appHandler.PublishAgent)
	appsGroup.GET("/:agent_id/published-versions", appHandler.ListAgentPublishedVersions)
	appsGroup.POST("/:agent_id/chat", appHandler.ChatAgent)
	appsGroup.GET("/:agent_id/runtime/conversations", appHandler.ListAgentRuntimeConversations)
	appsGroup.GET("/:agent_id/runtime/conversations/:conversation_id", appHandler.GetAgentRuntimeConversation)
	appsGroup.PATCH("/:agent_id/runtime/conversations/:conversation_id", appHandler.UpdateAgentRuntimeConversation)
	appsGroup.DELETE("/:agent_id/runtime/conversations/:conversation_id", appHandler.DeleteAgentRuntimeConversation)
	appsGroup.GET("/:agent_id/runtime/conversations/:conversation_id/messages", appHandler.ListAgentRuntimeMessages)
	appsGroup.POST("/:agent_id/runtime/conversations/:conversation_id/stop", appHandler.StopAgentRuntimeConversation)
	appsGroup.GET("/:agent_id/runtime/conversations/:conversation_id/events", appHandler.StreamAgentRuntimeEvents)
	appsGroup.POST("/:agent_id/runtime/messages/:message_id/regenerate", appHandler.RegenerateAgentRuntimeMessage)
	appsGroup.PUT("/:agent_id", appHandler.UpdateAgent)
	appsGroup.PATCH("/:agent_id/webapp/status", appHandler.UpdateWebAppStatus)
	appsGroup.DELETE("/:agent_id", appHandler.DeleteAgent)

	publicWebApps := v1.Group("/webapps")
	publicWebApps.Use(middleware.SetupRequired())
	publicWebApps.GET("/:web_app_id/config", appHandler.GetWebAppRuntimeConfig)

	protectedWebApps := v1.Group("/webapps")
	protectedWebApps.Use(middleware.SetupRequired())
	protectedWebApps.Use(middleware.SetAccountService(accountService))
	protectedWebApps.Use(middleware.WebAppAuthMiddleware())
	protectedWebApps.POST("/:web_app_id/chat", appHandler.ChatWebAppAgent)
	protectedWebApps.GET("/:web_app_id/runtime/conversations", appHandler.ListWebAppAgentRuntimeConversations)
	protectedWebApps.GET("/:web_app_id/runtime/conversations/:conversation_id", appHandler.GetWebAppAgentRuntimeConversation)
	protectedWebApps.PATCH("/:web_app_id/runtime/conversations/:conversation_id", appHandler.UpdateWebAppAgentRuntimeConversation)
	protectedWebApps.DELETE("/:web_app_id/runtime/conversations/:conversation_id", appHandler.DeleteWebAppAgentRuntimeConversation)
	protectedWebApps.GET("/:web_app_id/runtime/conversations/:conversation_id/messages", appHandler.ListWebAppAgentRuntimeMessages)
	protectedWebApps.POST("/:web_app_id/runtime/conversations/:conversation_id/stop", appHandler.StopWebAppAgentRuntimeConversation)
	protectedWebApps.GET("/:web_app_id/runtime/conversations/:conversation_id/events", appHandler.StreamWebAppAgentRuntimeEvents)
	protectedWebApps.POST("/:web_app_id/runtime/messages/:message_id/regenerate", appHandler.RegenerateWebAppAgentRuntimeMessage)

	workflowTests := appsGroup.Group("/:agent_id/workflow-tests")
	workflowTests.GET("/settings", workflowTestHandler.GetSettings)
	workflowTests.PUT("/settings", workflowTestHandler.UpdateSettings)
	workflowTests.POST("/settings/reset-judge-prompt", workflowTestHandler.ResetSettings)
	workflowTests.GET("/scenarios", workflowTestHandler.ListScenarios)
	workflowTests.POST("/scenarios", workflowTestHandler.CreateScenario)
	workflowTests.PUT("/scenarios", workflowTestHandler.SaveScenarios)
	workflowTests.POST("/scenarios/recognize", workflowTestHandler.RecognizeScenarios)
	workflowTests.GET("/cases", workflowTestHandler.ListCases)
	workflowTests.POST("/cases", workflowTestHandler.CreateCase)
	workflowTests.DELETE("/cases", workflowTestHandler.DeleteCases)
	workflowTests.PUT("/cases/:case_id", workflowTestHandler.UpdateCase)
	workflowTests.DELETE("/cases/:case_id", workflowTestHandler.DeleteCase)
	workflowTests.POST("/cases/generate", workflowTestHandler.GenerateCases)
	workflowTests.GET("/batches", workflowTestHandler.ListBatches)
	workflowTests.POST("/batches", workflowTestHandler.CreateBatch)
	workflowTests.POST("/batches/:batch_id/retest", workflowTestHandler.RetestBatch)
	workflowTests.POST("/batches/:batch_id/start", workflowTestHandler.StartBatch)
	workflowTests.POST("/batches/:batch_id/execute", workflowTestHandler.ExecuteBatch)
	workflowTests.POST("/batches/:batch_id/cancel", workflowTestHandler.CancelBatch)
	workflowTests.GET("/batches/:batch_id/items", workflowTestHandler.ListBatchItems)
}
