package v1

import (
	"context"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	runtimerepo "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/repository"
	runtimeservice "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/service"
	shortlinkcap "github.com/zgiai/zgi/api/internal/capabilities/shortlink"
	agentsHandlerPkg "github.com/zgiai/zgi/api/internal/modules/app/agents"
	"github.com/zgiai/zgi/api/internal/modules/app/conversation"
	"github.com/zgiai/zgi/api/internal/modules/app/runtimeauth"
	workflowHandlerPkg "github.com/zgiai/zgi/api/internal/modules/app/workflow"
	announcementruntime "github.com/zgiai/zgi/api/internal/modules/app/workflow/announcement"
	approvalruntime "github.com/zgiai/zgi/api/internal/modules/app/workflow/approval"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/diagnosis"
	workflow_file "github.com/zgiai/zgi/api/internal/modules/app/workflow/file"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/graph_engine"
	automationaction "github.com/zgiai/zgi/api/internal/modules/automation/service/action"
	automationdefinition "github.com/zgiai/zgi/api/internal/modules/automation/service/definition"
	"github.com/zgiai/zgi/api/internal/modules/dataset/graphflow"
	llmclient "github.com/zgiai/zgi/api/internal/modules/llm/client"
	promptservice "github.com/zgiai/zgi/api/internal/modules/prompts/service"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	"github.com/zgiai/zgi/api/middleware"
	"github.com/zgiai/zgi/api/pkg/queue"
	pkgscheduler "github.com/zgiai/zgi/api/pkg/scheduler"
)

type automationWorkflowRunnerSetter interface {
	SetAutomationWorkflowRunner(runner automationaction.AutomationWorkflowRunner)
}

type WorkflowRouteDeps struct {
	DB                          *gorm.DB
	AccountService              interfaces.AccountService
	FileService                 interfaces.FileService
	ContentExtractor            workflow_file.ContentExtractor
	QuotaService                interfaces.QuotaService
	OrganizationService         interfaces.OrganizationService
	LLMClient                   interface{}
	ToolEngine                  interface{}
	GraphFlowService            *graphflow.Service
	PromptResolver              promptservice.PromptService
	AutomationDefinitionService automationdefinition.Service
	TaskManager                 *queue.TaskManager
	TaskRegistry                approvalTaskRegistry
	Scheduler                   *pkgscheduler.Scheduler
	EngineFactory               *graph_engine.EngineFactory
	AutomationRunnerSetter      automationWorkflowRunnerSetter
	ShortLinkService            shortlinkcap.Service
}

// RegisterWorkflowRoutes now uses modular services.
func RegisterWorkflowRoutes(router *gin.RouterGroup, deps WorkflowRouteDeps) {
	validateWorkflowRouteDeps(deps)

	// Initialize workflow repository
	workflowRepo := workflowHandlerPkg.NewWorkflowRepository(deps.DB)
	workflowRunLogRepo := workflowHandlerPkg.NewWorkflowRunLogRepository(deps.DB)
	workflowNodeRuntimeLogRepo := workflowHandlerPkg.NewWorkflowNodeRuntimeLogRepository(deps.DB)
	conversationRepo := conversation.NewAgentConversationRepository(deps.DB)
	messageRepo := conversation.NewAgentMessageRepository(deps.DB)

	// Initialize agents repository
	agentsRepo := agentsHandlerPkg.NewAgentsRepository(deps.DB)

	// Initialize workflow service with ContentExtractor, QuotaService, EnterpriseService, and LLMClient
	workflowService := workflowHandlerPkg.NewWorkflowServiceWithContentExtractor(
		workflowRepo,
		agentsRepo,
		workflowRunLogRepo,
		workflowNodeRuntimeLogRepo,
		deps.AccountService,
		deps.FileService,
		deps.ContentExtractor,
		deps.QuotaService,
		deps.OrganizationService,
		deps.LLMClient,
		deps.ToolEngine,
		deps.GraphFlowService,
		deps.PromptResolver,
		deps.AutomationDefinitionService,
		deps.EngineFactory,
	)
	deps.AutomationRunnerSetter.SetAutomationWorkflowRunner(workflowService)

	// Initialize user migration service
	userMigrationService := workflowHandlerPkg.NewUserMigrationServiceFromDB()

	// Initialize workflow handler with proper dependencies
	handler := workflowHandlerPkg.NewWorkflowHandler(workflowService, deps.AccountService, deps.FileService, userMigrationService, deps.OrganizationService)
	handler.SetWebAppMigrationAuthorizer(workflowHandlerPkg.NewWebAppMigrationAuthorizer(agentsRepo, deps.DB))
	if llmClientTyped, ok := deps.LLMClient.(llmclient.LLMClient); ok {
		diag := diagnosis.NewDiagnoser(context.Background(), llmClientTyped)
		handler.SetDiagnoser(diag)
		workflowService.SetDiagnoser(diag)
	}

	agentHistoryHandler := workflowHandlerPkg.NewAgentWorkflowHistoryHandler(
		conversation.NewAgentConversationService(conversationRepo, messageRepo),
		conversation.NewAgentMessageService(messageRepo, conversationRepo),
	)
	runtimeLogHandler := workflowHandlerPkg.NewRuntimeLogHandler(
		workflowRunLogRepo,
		workflowNodeRuntimeLogRepo,
		workflowHandlerPkg.WithRuntimeLogAuthorization(agentsRepo, deps.OrganizationService),
	)
	chatRuntimeService := runtimeservice.NewServiceWithDependencies(
		runtimerepo.NewRepositories(deps.DB),
		nil,
		nil,
		nil,
		nil,
		nil,
		deps.OrganizationService,
	)
	agentHistoryDispatchHandler := workflowHandlerPkg.NewAgentHistoryDispatchHandler(
		agentsRepo,
		handler,
		agentHistoryHandler,
		runtimeLogHandler,
		chatRuntimeService,
	)
	agentRuntimeLogsHandler := workflowHandlerPkg.NewAgentRuntimeLogsHandler(agentsRepo, chatRuntimeService, deps.OrganizationService)

	apps := router.Group("/agents")
	// Add middleware for workflow routes
	apps.Use(middleware.SetupRequired())
	apps.Use(middleware.JWTWithOrganizationAndService(deps.AccountService))
	apps.Use(middleware.SetAccountService(deps.AccountService))

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
	apps.GET("/:agent_id/workflow-runs", agentHistoryDispatchHandler.GetWorkflowRuns)
	apps.GET("/:agent_id/workflow-runs/:run_id", agentHistoryDispatchHandler.GetWorkflowRunDetail)
	apps.GET("/:agent_id/workflow-runs/:run_id/node-executions", agentHistoryDispatchHandler.GetWorkflowRunNodeExecutions)
	apps.GET("/:agent_id/runtime-runs", agentRuntimeLogsHandler.GetRuntimeRuns)
	apps.GET("/:agent_id/runtime-runs/:message_id", agentRuntimeLogsHandler.GetRuntimeRunDetail)
	apps.GET("/:agent_id/runtime-runs/:message_id/steps", agentRuntimeLogsHandler.GetRuntimeRunSteps)
	apps.POST("/:agent_id/workflow-runs/:run_id/nodes/:node_log_id/diagnose", handler.ManualDiagnoseNode)
	apps.GET("/:agent_id/conversations", agentHistoryDispatchHandler.GetConversations)
	apps.GET("/:agent_id/conversations/:conversation_id", agentHistoryDispatchHandler.GetConversationDetail)
	apps.GET("/:agent_id/chat-messages", agentHistoryDispatchHandler.GetChatMessages)

	// Get latest version
	apps.GET("/:agent_id/workflows/published-versions", handler.GetPublishedWorkflowVersions)
	apps.GET("/:agent_id/workflows/latest-version", handler.GetLatestWorkflowVersion)

	// Import / Export
	apps.GET("/:agent_id/workflows/export", handler.ExportWorkflow)
	apps.POST("/workflows/import", handler.ImportWorkflow)

	// Runtime log handler (still use agent_id for internal management)
	apps.POST("/:agent_id/runtime-logs", agentHistoryDispatchHandler.GetRuntimeLogs)
	apps.GET("/:agent_id/workflow-runs/:run_id/nodes", runtimeLogHandler.GetWorkflowRunNodeLogs)

	approvalService := approvalruntime.NewServiceWithShortLinkService(deps.DB, deps.ShortLinkService)
	registerApprovalTaskHandlers(deps.TaskRegistry, deps.TaskManager, approvalService, handler)
	registerApprovalScheduledTasks(deps.Scheduler, approvalService, handler)

	approvalHandler := approvalruntime.NewHandler(approvalService, deps.TaskManager)
	approvalRoutes := router.Group("/approval")
	approvalRoutes.Use(middleware.SetupRequired())
	approvalRoutes.GET("/forms/:token", approvalHandler.GetForm)
	approvalRoutes.GET("/forms/:token/events", approvalHandler.GetRunEvents)
	approvalRoutes.POST("/forms/:token/submit", approvalHandler.SubmitForm)

	announcementService := announcementruntime.NewServiceWithShortLinkService(deps.DB, deps.ShortLinkService)
	registerAnnouncementScheduledTasks(deps.Scheduler, announcementService)
	announcementHandler := announcementruntime.NewHandler(announcementService)
	announcementRoutes := router.Group("/announcements")
	announcementRoutes.Use(middleware.SetupRequired())
	announcementRoutes.GET("/:token", announcementHandler.GetAnnouncement)

	workflowRunEvents := router.Group("/workflow-runs")
	workflowRunEvents.Use(middleware.SetupRequired())
	workflowRunEvents.Use(middleware.JWTWithOrganizationAndService(deps.AccountService))
	workflowRunEvents.Use(middleware.SetAccountService(deps.AccountService))
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
	protectedWorkflows.Use(middleware.SetAccountService(deps.AccountService))
	protectedWorkflows.Use(middleware.WebAppAuthMiddleware()) // Use new dual-header authentication middleware

	// Workflow execution - now uses WebAppAuthMiddleware for dual-header authentication
	protectedWorkflows.POST("/:web_app_id/precheck", handler.PrecheckWorkflowByWebAppID)
	protectedWorkflows.POST("/:web_app_id/run", handler.RunWorkflowByWebAppID)

	// Conversation management (use web_app_id instead of agent_id)
	protectedWorkflows.GET("/:web_app_id/search", conversationQueryHandler.SearchConversationList)
	protectedWorkflows.GET("/:web_app_id/conversations", conversationQueryHandler.GetConversationList)
	protectedWorkflows.GET("/:web_app_id/conversations/:conversation_id", conversationQueryHandler.GetConversationDetail)
	protectedWorkflows.DELETE("/:web_app_id/conversations/:conversation_id", conversationQueryHandler.DeleteConversation)

	// User migration endpoint - requires both Authorization and X-User-Account-Id headers
	protectedWorkflows.POST("/:web_app_id/migrate-user", handler.MigrateUserForWebApp)
	protectedWorkflows.POST("/migrate-user", handler.MigrateUser)

	// Built-in workflows API (organization-level product surface, no workspace required)
	// Requirements: 3.1, 3.2
	builtInWorkflows := router.Group("/built-in-workflows")
	builtInWorkflows.Use(middleware.SetupRequired())
	builtInWorkflows.Use(middleware.JWTWithOrganizationAndService(deps.AccountService))
	builtInWorkflows.Use(middleware.SetAccountService(deps.AccountService))

	// Initialize built-in workflow repository and service
	builtInRepo := workflowHandlerPkg.NewBuiltInWorkflowRepository(deps.DB)
	builtInService := workflowHandlerPkg.NewBuiltInWorkflowService(builtInRepo, runtimeauth.NewStore(deps.DB), deps.DB)
	builtInHandler := workflowHandlerPkg.NewBuiltInWorkflowHandler(builtInService)

	// Register built-in workflow routes
	builtInWorkflows.GET("", builtInHandler.GetBuiltInWorkflows)

	builtInWorkflowRuntimeSurfaces := builtInWorkflows.Group("")
	builtInWorkflowRuntimeSurfaces.Use(middleware.EnterpriseAdminOrOwnerRequired())
	builtInWorkflowRuntimeSurfaces.GET("/:scenario/runtime-surfaces", builtInHandler.GetBuiltInWorkflowRuntimeSurfaces)
	builtInWorkflowRuntimeSurfaces.PATCH("/:scenario/runtime-surfaces", builtInHandler.UpdateBuiltInWorkflowRuntimeSurfaces)

	builtInWorkflows.GET("/:scenario", builtInHandler.GetBuiltInWorkflowByScenario)
}

func validateWorkflowRouteDeps(deps WorkflowRouteDeps) {
	if deps.DB == nil {
		panic("workflow routes require db")
	}
	if deps.AccountService == nil {
		panic("workflow routes require account service")
	}
	if deps.FileService == nil {
		panic("workflow routes require file service")
	}
	if deps.ContentExtractor == nil {
		panic("workflow routes require content extractor")
	}
	if deps.QuotaService == nil {
		panic("workflow routes require quota service")
	}
	if deps.OrganizationService == nil {
		panic("workflow routes require organization service")
	}
	if deps.LLMClient == nil {
		panic("workflow routes require llm client")
	}
	if deps.ToolEngine == nil {
		panic("workflow routes require tool engine")
	}
	if deps.GraphFlowService == nil {
		panic("workflow routes require graph flow service")
	}
	if deps.PromptResolver == nil {
		panic("workflow routes require prompt resolver")
	}
	if deps.AutomationDefinitionService == nil {
		panic("workflow routes require automation definition service")
	}
	if deps.TaskManager == nil {
		panic("workflow routes require task manager")
	}
	if deps.TaskRegistry == nil {
		panic("workflow routes require task registry")
	}
	if deps.Scheduler == nil {
		panic("workflow routes require scheduler")
	}
	if deps.EngineFactory == nil {
		panic("workflow routes require workflow engine factory")
	}
	if deps.AutomationRunnerSetter == nil {
		panic("workflow routes require automation runner setter")
	}
	if deps.ShortLinkService == nil {
		panic("workflow routes require short link service")
	}
}
