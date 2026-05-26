package v1

import (
	"github.com/gin-gonic/gin"
	app "github.com/zgiai/zgi/api/internal/modules/app/agents"
	workflow "github.com/zgiai/zgi/api/internal/modules/app/workflow"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/graph_engine"
	workflowtest "github.com/zgiai/zgi/api/internal/modules/app/workflowtest"
	"github.com/zgiai/zgi/api/internal/modules/dataset/graphflow"
	llmclient "github.com/zgiai/zgi/api/internal/modules/llm/client"
	promptservice "github.com/zgiai/zgi/api/internal/modules/prompts/service"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	"github.com/zgiai/zgi/api/middleware"
	"github.com/zgiai/zgi/api/pkg/logger"
	"github.com/zgiai/zgi/api/pkg/queue"
	"gorm.io/gorm"
)

func RegisterAgentsRoutes(v1 *gin.RouterGroup, db *gorm.DB, accountService interfaces.AccountService, tenantService interfaces.WorkspaceManagementService, resourcePermissionService interfaces.ResourcePermissionService, enterpriseService interfaces.OrganizationService, quotaService interfaces.QuotaService, fileService interfaces.FileService, contentExtractor interface{}, llmClient interface{}, toolEngine interface{}, graphFlowService *graphflow.Service, promptResolver promptservice.PromptService, engineFactory *graph_engine.EngineFactory, taskManager *queue.TaskManager, taskRegistry workflowtest.TaskHandlerRegistry) {
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

	service := app.NewAgentsService(repo, accountService, tenantService, workflowService, resourcePermissionService, enterpriseService, quotaService, fileService, db)
	appHandler := app.NewAgentsHandler(service, tenantService, accountService, enterpriseService, db)
	workflowTestService := workflowtest.NewService(workflowtest.NewRepository(db))
	workflowTestHandler := workflowtest.NewHandler(workflowTestService, workflowService, enterpriseService, llmClient, taskManager)
	if taskRegistry != nil && taskManager != nil {
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
	appsGroup.GET("/:agent_id", appHandler.GetAgent)
	appsGroup.PUT("/:agent_id", appHandler.UpdateAgent)
	appsGroup.PATCH("/:agent_id/webapp/status", appHandler.UpdateWebAppStatus)
	appsGroup.DELETE("/:agent_id", appHandler.DeleteAgent)

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
}
