package v1

import (
	"github.com/gin-gonic/gin"
	runtimerepo "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/repository"
	runtimeservice "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/service"
	"github.com/zgiai/zgi/api/internal/modules/agentmemory"
	app "github.com/zgiai/zgi/api/internal/modules/app/agents"
	workflow "github.com/zgiai/zgi/api/internal/modules/app/workflow"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/graph_engine"
	workflowtest "github.com/zgiai/zgi/api/internal/modules/app/workflowtest"
	"github.com/zgiai/zgi/api/internal/modules/dataset/graphflow"
	datasetservice "github.com/zgiai/zgi/api/internal/modules/dataset/service"
	datasourceservice "github.com/zgiai/zgi/api/internal/modules/datasource/service"
	channelrepo "github.com/zgiai/zgi/api/internal/modules/llm/channel/repository"
	llmclient "github.com/zgiai/zgi/api/internal/modules/llm/client"
	llmdefaultservice "github.com/zgiai/zgi/api/internal/modules/llm/defaultmodel/service"
	memorymodule "github.com/zgiai/zgi/api/internal/modules/memory"
	promptservice "github.com/zgiai/zgi/api/internal/modules/prompts/service"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	"github.com/zgiai/zgi/api/internal/modules/shared/titlegen"
	"github.com/zgiai/zgi/api/internal/modules/tools"
	"github.com/zgiai/zgi/api/middleware"
	"github.com/zgiai/zgi/api/pkg/logger"
	"github.com/zgiai/zgi/api/pkg/queue"
	"gorm.io/gorm"
)

func RegisterAgentsRoutes(v1 *gin.RouterGroup, db *gorm.DB, accountService interfaces.AccountService, tenantService interfaces.WorkspaceManagementService, resourcePermissionService interfaces.ResourcePermissionService, enterpriseService interfaces.OrganizationService, quotaService interfaces.QuotaService, fileService interfaces.FileService, contentExtractor runtimeservice.ContentExtractionService, llmClient llmclient.LLMClient, toolEngine *tools.ToolEngine, toolManager *tools.ToolManager, memoryService *memorymodule.Service, graphFlowService *graphflow.Service, promptResolver promptservice.PromptService, dataSourceService datasourceservice.DataSourceService, knowledgeRetrievalService *datasetservice.KnowledgeRetrievalService, engineFactory *graph_engine.EngineFactory, taskManager *queue.TaskManager, taskRegistry workflowtest.TaskHandlerRegistry, workflowTestService *workflowtest.Service, workflowTestTaskBackend string) app.AgentsService {
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

	agentMemoryService := agentmemory.NewService(db)
	var defaultModelResolver llmdefaultservice.DefaultModelResolver
	var titleGenerator titlegen.Service
	if graphFlowService != nil {
		defaultModelResolver = graphFlowService.DefaultModelSvc
		if graphFlowService.DefaultModelSvc != nil {
			titleGenerator = titlegen.NewServiceWithRouteModelProvider(
				llmClient,
				graphFlowService.DefaultModelSvc,
				titlegen.NewTenantRouteModelProvider(channelrepo.NewTenantRouteRepository(db)),
			)
		}
	}
	chatRuntimeService := runtimeservice.NewServiceWithSkillRuntime(
		runtimerepo.NewRepositories(db),
		llmClient,
		titleGenerator,
		runtimeservice.NewDatabaseModelSpecResolver(db),
		fileService,
		contentExtractor,
		enterpriseService,
		newSkillRuntimeWithSandbox(toolEngine, toolManager, fileService, enterpriseService),
		memoryService,
		agentMemoryService,
	)
	service := app.NewAgentsService(repo, accountService, tenantService, workflowService, chatRuntimeService, agentMemoryService, dataSourceService, knowledgeRetrievalService, resourcePermissionService, enterpriseService, quotaService, fileService, llmClient, defaultModelResolver, db)
	appHandler := app.NewAgentsHandler(service, tenantService, accountService, enterpriseService, db, chatRuntimeService)
	appHandler.SetFileService(fileService)
	appHandler.SetWorkflowContinuationRunner(workflow.NewWorkflowHandler(workflowService, accountService, fileService, nil, enterpriseService))
	if workflowTestService == nil {
		workflowTestService = workflowtest.NewService(workflowtest.NewRepository(db))
	}
	if defaultModelResolver != nil {
		workflowTestService.SetDefaultModelResolver(defaultModelResolver)
	}
	workflowTestHandler := workflowtest.NewHandler(workflowTestService, workflowService, enterpriseService, llmClient, taskManager, workflowTestTaskBackend)
	if workflowtest.NormalizeTaskBackend(workflowTestTaskBackend) == workflowtest.WorkflowTestTaskBackendAsynq && taskRegistry != nil && taskManager != nil {
		if client, ok := llmClient.(llmclient.LLMClient); ok {
			taskType := taskManager.GetTaskTypeWithPrefix(workflowtest.WorkflowTestGenerationTaskType)
			handler := workflowtest.NewGenerationTaskHandler(workflowTestService, client)
			if isNew := taskRegistry.Register(taskType, handler); isNew {
				logger.Info("Registered workflow test generation task handler", map[string]interface{}{"task_type": taskType})
			} else {
				logger.Warn("Workflow test generation task handler was replaced", map[string]interface{}{"task_type": taskType})
			}
			scenarioTaskType := taskManager.GetTaskTypeWithPrefix(workflowtest.WorkflowTestScenarioRecognitionTaskType)
			scenarioHandler := workflowtest.NewScenarioRecognitionTaskHandler(workflowTestService, client)
			if isNew := taskRegistry.Register(scenarioTaskType, scenarioHandler); isNew {
				logger.Info("Registered workflow test scenario recognition task handler", map[string]interface{}{"task_type": scenarioTaskType})
			} else {
				logger.Warn("Workflow test scenario recognition task handler was replaced", map[string]interface{}{"task_type": scenarioTaskType})
			}
		}
	}

	appsGroup := v1.Group("/agents")
	appsGroup.Use(middleware.SetupRequired())
	appsGroup.Use(middleware.JWTWithOrganizationAndService(accountService))
	appsGroup.Use(middleware.SetAccountService(accountService))

	// Agent management endpoints
	appsGroup.GET("", appHandler.GetAgentsList)
	appsGroup.GET("/runnable-webapps", appHandler.GetRunnableWebApps)
	appsGroup.POST("", appHandler.CreateAgent)
	appsGroup.GET("/:agent_id/workflow-bindings/candidates", appHandler.ListAgentWorkflowBindingCandidates)
	appsGroup.GET("/:agent_id", appHandler.GetAgent)
	appsGroup.GET("/:agent_id/config", appHandler.GetAgentConfig)
	appsGroup.GET("/:agent_id/runtime-surfaces", appHandler.GetAgentRuntimeSurfaces)
	appsGroup.PATCH("/:agent_id/runtime-surfaces", appHandler.UpdateAgentRuntimeSurfaces)
	appsGroup.PUT("/:agent_id/config", appHandler.UpdateAgentConfig)
	appsGroup.POST("/:agent_id/suggested-questions/generate", appHandler.GenerateAgentSuggestedQuestions)
	appsGroup.POST("/:agent_id/publish", appHandler.PublishAgent)
	appsGroup.GET("/:agent_id/published-versions", appHandler.ListAgentPublishedVersions)
	appsGroup.POST("/:agent_id/published-versions/rollback", appHandler.RollbackAgentPublishedVersion)
	appsGroup.POST("/:agent_id/chat", appHandler.ChatAgent)
	appsGroup.GET("/:agent_id/runtime/conversations", appHandler.ListAgentRuntimeConversations)
	appsGroup.GET("/:agent_id/runtime/conversations/:conversation_id", appHandler.GetAgentRuntimeConversation)
	appsGroup.PATCH("/:agent_id/runtime/conversations/:conversation_id", appHandler.UpdateAgentRuntimeConversation)
	appsGroup.DELETE("/:agent_id/runtime/conversations/:conversation_id", appHandler.DeleteAgentRuntimeConversation)
	appsGroup.GET("/:agent_id/runtime/conversations/:conversation_id/messages", appHandler.ListAgentRuntimeMessages)
	appsGroup.POST("/:agent_id/runtime/conversations/:conversation_id/stop", appHandler.StopAgentRuntimeConversation)
	appsGroup.GET("/:agent_id/runtime/conversations/:conversation_id/events", appHandler.StreamAgentRuntimeEvents)
	appsGroup.POST("/:agent_id/runtime/conversations/:conversation_id/messages/:message_id/workflow-continuation", appHandler.ContinueAgentRuntimeWorkflowApproval)
	appsGroup.POST("/:agent_id/runtime/messages/:message_id/regenerate", appHandler.RegenerateAgentRuntimeMessage)
	appsGroup.PUT("/:agent_id", appHandler.UpdateAgent)
	appsGroup.PATCH("/:agent_id/webapp/status", appHandler.UpdateWebAppStatus)
	appsGroup.DELETE("/:agent_id", appHandler.DeleteAgent)
	appsGroup.GET("/:agent_id/memory/slots", appHandler.ListAgentMemorySlots)
	appsGroup.PUT("/:agent_id/memory/slots", appHandler.ReplaceAgentMemorySlots)
	appsGroup.GET("/:agent_id/memory/values", appHandler.ListAgentMemoryValues)
	appsGroup.PUT("/:agent_id/memory/values", appHandler.UpdateAgentMemoryValue)
	appsGroup.DELETE("/:agent_id/memory/values/:key", appHandler.ClearAgentMemoryValue)

	publicWebApps := v1.Group("/webapps")
	publicWebApps.Use(middleware.SetupRequired())
	publicWebApps.GET("/:web_app_id/config", appHandler.GetWebAppRuntimeConfig)

	protectedWebApps := v1.Group("/webapps")
	protectedWebApps.Use(middleware.SetupRequired())
	protectedWebApps.Use(middleware.SetAccountService(accountService))
	protectedWebApps.Use(middleware.WebAppAuthMiddleware())
	protectedWebApps.GET("/:web_app_id/capability", appHandler.GetWebAppRuntimeCapability)
	protectedWebApps.POST("/:web_app_id/chat", appHandler.ChatWebAppAgent)
	protectedWebApps.GET("/:web_app_id/files/upload", appHandler.GetWebAppUploadConfig)
	protectedWebApps.POST("/:web_app_id/files/upload", appHandler.UploadWebAppFile)
	protectedWebApps.GET("/:web_app_id/runtime/search", appHandler.SearchWebAppAgentRuntimeConversations)
	protectedWebApps.GET("/:web_app_id/runtime/conversations", appHandler.ListWebAppAgentRuntimeConversations)
	protectedWebApps.GET("/:web_app_id/runtime/conversations/:conversation_id", appHandler.GetWebAppAgentRuntimeConversation)
	protectedWebApps.PATCH("/:web_app_id/runtime/conversations/:conversation_id", appHandler.UpdateWebAppAgentRuntimeConversation)
	protectedWebApps.DELETE("/:web_app_id/runtime/conversations/:conversation_id", appHandler.DeleteWebAppAgentRuntimeConversation)
	protectedWebApps.GET("/:web_app_id/runtime/conversations/:conversation_id/messages", appHandler.ListWebAppAgentRuntimeMessages)
	protectedWebApps.POST("/:web_app_id/runtime/conversations/:conversation_id/stop", appHandler.StopWebAppAgentRuntimeConversation)
	protectedWebApps.GET("/:web_app_id/runtime/conversations/:conversation_id/events", appHandler.StreamWebAppAgentRuntimeEvents)
	protectedWebApps.POST("/:web_app_id/runtime/conversations/:conversation_id/messages/:message_id/workflow-continuation", appHandler.ContinueWebAppAgentRuntimeWorkflowApproval)
	protectedWebApps.POST("/:web_app_id/runtime/messages/:message_id/regenerate", appHandler.RegenerateWebAppAgentRuntimeMessage)

	workflowTests := appsGroup.Group("/:agent_id/workflow-tests")
	workflowTests.GET("/settings", workflowTestHandler.GetSettings)
	workflowTests.PUT("/settings", workflowTestHandler.UpdateSettings)
	workflowTests.POST("/settings/reset-judge-prompt", workflowTestHandler.ResetSettings)
	workflowTests.GET("/scenarios", workflowTestHandler.ListScenarios)
	workflowTests.POST("/scenarios", workflowTestHandler.CreateScenario)
	workflowTests.PUT("/scenarios", workflowTestHandler.SaveScenarios)
	workflowTests.POST("/scenarios/recognize", workflowTestHandler.RecognizeScenarios)
	workflowTests.POST("/scenarios/recognition-tasks", workflowTestHandler.CreateScenarioRecognitionTask)
	workflowTests.GET("/scenarios/recognition-tasks", workflowTestHandler.GetLatestScenarioRecognitionTask)
	workflowTests.GET("/scenarios/recognition-tasks/active", workflowTestHandler.GetActiveScenarioRecognitionTask)
	workflowTests.GET("/scenarios/recognition-tasks/:task_id", workflowTestHandler.GetScenarioRecognitionTask)
	workflowTests.POST("/scenarios/recognition-tasks/:task_id/cancel", workflowTestHandler.CancelScenarioRecognitionTask)
	workflowTests.GET("/cases", workflowTestHandler.ListCases)
	workflowTests.POST("/cases", workflowTestHandler.CreateCase)
	workflowTests.DELETE("/cases", workflowTestHandler.DeleteCases)
	workflowTests.PUT("/cases/:case_id", workflowTestHandler.UpdateCase)
	workflowTests.DELETE("/cases/:case_id", workflowTestHandler.DeleteCase)
	workflowTests.POST("/cases/generation-tasks", workflowTestHandler.CreateGenerationTask)
	workflowTests.GET("/cases/generation-tasks", workflowTestHandler.GetLatestGenerationTask)
	workflowTests.GET("/cases/generation-tasks/active", workflowTestHandler.GetActiveGenerationTask)
	workflowTests.GET("/cases/generation-tasks/latest", workflowTestHandler.GetLatestGenerationTask)
	workflowTests.GET("/cases/generation-tasks/:task_id", workflowTestHandler.GetGenerationTask)
	workflowTests.POST("/cases/generation-tasks/:task_id/cancel", workflowTestHandler.CancelGenerationTask)
	workflowTests.POST("/cases/generate", workflowTestHandler.GenerateCases)
	workflowTests.GET("/batches", workflowTestHandler.ListBatches)
	workflowTests.POST("/batches", workflowTestHandler.CreateBatch)
	workflowTests.POST("/batches/:batch_id/retest", workflowTestHandler.RetestBatch)
	workflowTests.POST("/batches/:batch_id/start", workflowTestHandler.StartBatch)
	workflowTests.POST("/batches/:batch_id/execute", workflowTestHandler.ExecuteBatch)
	workflowTests.POST("/batches/:batch_id/cancel", workflowTestHandler.CancelBatch)
	workflowTests.GET("/batches/:batch_id/items", workflowTestHandler.ListBatchItems)

	return service
}
