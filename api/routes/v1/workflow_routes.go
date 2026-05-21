package v1

import (
	"context"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	agentsHandlerPkg "github.com/zgiai/zgi/api/internal/modules/app/agents"
	"github.com/zgiai/zgi/api/internal/modules/app/conversation"
	workflowHandlerPkg "github.com/zgiai/zgi/api/internal/modules/app/workflow"
	announcementruntime "github.com/zgiai/zgi/api/internal/modules/app/workflow/announcement"
	approvalruntime "github.com/zgiai/zgi/api/internal/modules/app/workflow/approval"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/diagnosis"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/graph_engine"
	automationaction "github.com/zgiai/zgi/api/internal/modules/automation/service/action"
	automationdefinition "github.com/zgiai/zgi/api/internal/modules/automation/service/definition"
	"github.com/zgiai/zgi/api/internal/modules/dataset/graphflow"
	"github.com/zgiai/zgi/api/internal/modules/llm/client"
	promptservice "github.com/zgiai/zgi/api/internal/modules/prompts/service"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	"github.com/zgiai/zgi/api/middleware"
	"github.com/zgiai/zgi/api/pkg/queue"
	pkgscheduler "github.com/zgiai/zgi/api/pkg/scheduler"
)

type automationWorkflowRunnerSetter interface {
	SetAutomationWorkflowRunner(runner automationaction.AutomationWorkflowRunner)
}

// RegisterWorkflowRoutes now uses modular services.
func RegisterWorkflowRoutes(router *gin.RouterGroup, accountService interfaces.AccountService, _ interfaces.WorkspaceManagementService, fileService interfaces.FileService, db *gorm.DB, contentExtractor interface{}, quotaService interfaces.QuotaService, enterpriseService interfaces.OrganizationService, llmClient interface{}, toolEngine interface{}, graphFlowService *graphflow.Service, promptResolver promptservice.PromptService, automationDefinitionService automationdefinition.Service, taskManager *queue.TaskManager, taskRegistry approvalTaskRegistry, scheduler *pkgscheduler.Scheduler, engineFactory *graph_engine.EngineFactory, automationRunnerSetter automationWorkflowRunnerSetter) {
	if taskManager == nil {
		panic("workflow approval task manager is required")
	}
	if taskRegistry == nil {
		panic("workflow approval task registry is required")
	}
	// Initialize workflow repository
	workflowRepo := workflowHandlerPkg.NewWorkflowRepository(db)
	workflowRunLogRepo := workflowHandlerPkg.NewWorkflowRunLogRepository(db)
	workflowNodeRuntimeLogRepo := workflowHandlerPkg.NewWorkflowNodeRuntimeLogRepository(db)
	conversationRepo := conversation.NewAgentConversationRepository(db)
	messageRepo := conversation.NewAgentMessageRepository(db)

	// Initialize agents repository
	agentsRepo := agentsHandlerPkg.NewAgentsRepository(db)

	// Initialize workflow service with ContentExtractor, QuotaService, EnterpriseService, and LLMClient
	workflowService := workflowHandlerPkg.NewWorkflowServiceWithContentExtractor(
		workflowRepo,
		agentsRepo,
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
		automationDefinitionService,
		engineFactory,
	)
	if automationRunnerSetter != nil {
		automationRunnerSetter.SetAutomationWorkflowRunner(workflowService)
	}

	// Initialize user migration service
	userMigrationService := workflowHandlerPkg.NewUserMigrationServiceFromDB()

	// Initialize workflow handler with proper dependencies
	handler := workflowHandlerPkg.NewWorkflowHandler(workflowService, accountService, fileService, userMigrationService, enterpriseService)
	if llmClientTyped, ok := llmClient.(client.LLMClient); ok {
		diag := diagnosis.NewDiagnoser(context.Background(), llmClientTyped)
		handler.SetDiagnoser(diag)
		workflowService.SetDiagnoser(diag)
	}

	agentHistoryHandler := workflowHandlerPkg.NewAgentWorkflowHistoryHandler(
		conversation.NewAgentConversationService(conversationRepo, messageRepo),
		conversation.NewAgentMessageService(messageRepo, conversationRepo),
	)

	apps := router.Group("/agents")
	// Add middleware for workflow routes
	apps.Use(middleware.SetupRequired())
	apps.Use(middleware.JWTWithOrganizationAndService(accountService))
	apps.Use(middleware.SetAccountService(accountService))

	apps.GET("/:agent_id/workflows/draft", handler.GetDraftWorkflow)
	apps.POST("/:agent_id/workflows/draft", handler.SyncDraftWorkflow)
	apps.POST("/:agent_id/workflows/draft/suggested-questions/generate", handler.GenerateDraftWorkflowSuggestedQuestions)
	apps.GET("/:agent_id/workflows/draft/config", handler.GetWorkflowConfig)
	apps.POST("/:agent_id/workflows/draft/precheck", handler.PrecheckDraftWorkflow)
	apps.POST("/:agent_id/workflows/draft/run", handler.RunDraftWorkflow)
	apps.POST("/:agent_id/advanced-chat/workflows/draft/precheck", handler.PrecheckAdvancedChatDraftWorkflow)
	apps.POST("/:agent_id/advanced-chat/workflows/draft/run", handler.RunAdvancedChatDraftWorkflow)
	apps.POST("/:agent_id/workflow-runs/tasks/:task_id/stop", handler.StopWorkflowTask)
	apps.POST("/:agent_id/workflows/draft/nodes/:node_id/run", handler.RunDraftWorkflowNode)
	apps.POST("/:agent_id/workflows/publish", handler.PublishWorkflow)
	apps.POST("/:agent_id/advanced-chat/workflows/precheck", handler.PrecheckAdvancedChatWorkflow)
	apps.POST("/:agent_id/advanced-chat/workflows/run", handler.RunAdvancedChatWorkflow)

	apps.POST("/:agent_id/workflows/precheck", handler.PrecheckPublishedWorkflow)
	apps.POST("/:agent_id/workflows/run", handler.RunPublishedWorkflow)
	apps.GET("/:agent_id/workflow-runs", handler.GetWorkflowRuns)
	apps.GET("/:agent_id/workflow-runs/:run_id", handler.GetWorkflowRunDetail)
	apps.GET("/:agent_id/workflow-runs/:run_id/node-executions", handler.GetWorkflowRunNodeExecutions)
	apps.POST("/:agent_id/workflow-runs/:run_id/nodes/:node_log_id/diagnose", handler.ManualDiagnoseNode)
	apps.GET("/:agent_id/conversations", agentHistoryHandler.GetConversations)
	apps.GET("/:agent_id/conversations/:conversation_id", agentHistoryHandler.GetConversationDetail)
	apps.GET("/:agent_id/chat-messages", agentHistoryHandler.GetChatMessages)

	// Get latest version
	apps.GET("/:agent_id/workflows/published-versions", handler.GetPublishedWorkflowVersions)
	apps.GET("/:agent_id/workflows/latest-version", handler.GetLatestWorkflowVersion)

	// Import / Export
	apps.GET("/:agent_id/workflows/export", handler.ExportWorkflow)
	apps.POST("/workflows/import", handler.ImportWorkflow)

	// Runtime log handler (still use agent_id for internal management)
	runtimeLogHandler := workflowHandlerPkg.NewRuntimeLogHandler(workflowRunLogRepo, workflowNodeRuntimeLogRepo)
	apps.POST("/:agent_id/runtime-logs", runtimeLogHandler.GetRuntimeLogs)
	apps.GET("/:agent_id/workflow-runs/:run_id/nodes", runtimeLogHandler.GetWorkflowRunNodeLogs)

	approvalService := approvalruntime.NewService(db)
	registerApprovalTaskHandlers(taskRegistry, taskManager, approvalService, handler)
	registerApprovalScheduledTasks(scheduler, approvalService, handler)

	approvalHandler := approvalruntime.NewHandler(approvalService, taskManager)
	approvalRoutes := router.Group("/approval")
	approvalRoutes.Use(middleware.SetupRequired())
	approvalRoutes.GET("/forms/:token", approvalHandler.GetForm)
	approvalRoutes.GET("/forms/:token/events", approvalHandler.GetRunEvents)
	approvalRoutes.POST("/forms/:token/submit", approvalHandler.SubmitForm)

	announcementService := announcementruntime.NewService(db)
	registerAnnouncementScheduledTasks(scheduler, announcementService)
	announcementHandler := announcementruntime.NewHandler(announcementService)
	announcementRoutes := router.Group("/announcements")
	announcementRoutes.Use(middleware.SetupRequired())
	announcementRoutes.GET("/:token", announcementHandler.GetAnnouncement)

	workflowRunEvents := router.Group("/workflow-runs")
	workflowRunEvents.Use(middleware.SetupRequired())
	workflowRunEvents.Use(middleware.JWTWithOrganizationAndService(accountService))
	workflowRunEvents.Use(middleware.SetAccountService(accountService))
	workflowRunEvents.GET("/:workflow_run_id/events", handler.GetWorkflowRunEvents)

	// Web app workflow configuration is public, but still requires the system to be initialized.
	publicWorkflows := router.Group("/workflows")
	publicWorkflows.Use(middleware.SetupRequired())
	conversationQueryHandler := workflowHandlerPkg.NewConversationQueryHandler(workflowRepo, agentsRepo)

	// Get workflow configuration
	publicWorkflows.GET("/:web_app_id/config", conversationQueryHandler.GetWebAppConfig)

	// Web app workflow execution and conversation management by web_app_id
	protectedWorkflows := router.Group("/workflows")
	protectedWorkflows.Use(middleware.SetupRequired())
	protectedWorkflows.Use(middleware.SetAccountService(accountService))
	protectedWorkflows.Use(middleware.WebAppAuthMiddleware()) // Use new dual-header authentication middleware

	// Workflow execution - now uses WebAppAuthMiddleware for dual-header authentication
	protectedWorkflows.POST("/:web_app_id/precheck", handler.PrecheckWorkflowByWebAppID)
	protectedWorkflows.POST("/:web_app_id/run", handler.RunWorkflowByWebAppID)

	// Conversation management (use web_app_id instead of agent_id)
	protectedWorkflows.GET("/:web_app_id/conversations", conversationQueryHandler.GetConversationList)
	protectedWorkflows.GET("/:web_app_id/conversations/:conversation_id", conversationQueryHandler.GetConversationDetail)
	protectedWorkflows.DELETE("/:web_app_id/conversations/:conversation_id", conversationQueryHandler.DeleteConversation)

	// User migration endpoint - requires both Authorization and X-User-Account-Id headers
	protectedWorkflows.POST("/migrate-user", handler.MigrateUser)

	// Built-in workflows API (public, no authentication required)
	// Requirements: 3.1, 3.2
	builtInWorkflows := router.Group("/built-in-workflows")
	builtInWorkflows.Use(middleware.SetupRequired())

	// Initialize built-in workflow repository and service
	builtInRepo := workflowHandlerPkg.NewBuiltInWorkflowRepository(db)
	builtInService := workflowHandlerPkg.NewBuiltInWorkflowService(builtInRepo)
	builtInHandler := workflowHandlerPkg.NewBuiltInWorkflowHandler(builtInService)

	// Register built-in workflow routes
	builtInWorkflows.GET("", builtInHandler.GetBuiltInWorkflows)
	builtInWorkflows.GET("/:scenario", builtInHandler.GetBuiltInWorkflowByScenario)
}
