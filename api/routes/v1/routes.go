package v1

import (
	"log"

	"github.com/gin-gonic/gin"
	"github.com/zgiai/zgi/api/config"
	"github.com/zgiai/zgi/api/internal/container"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/graph_engine"
	system_service "github.com/zgiai/zgi/api/internal/modules/system/service"
	agentmanagement_tools "github.com/zgiai/zgi/api/internal/modules/tools/builtin/agentmanagement"
	workspace_service "github.com/zgiai/zgi/api/internal/modules/workspace/service"
	"github.com/zgiai/zgi/api/pkg/database"
	"github.com/zgiai/zgi/api/pkg/storage"
)

// RegisterRoutes registers all v1 version routes
func RegisterRoutes(engine *gin.Engine, v1 *gin.RouterGroup, serviceContainer *container.ServiceContainer, workflowEngineFactory *graph_engine.EngineFactory) {
	// Health & setup routes first
	RegisterHealthRoutes(v1)
	RegisterSetupRoutes(v1, SetupRouteDeps{
		BootstrapService: serviceContainer.GetBootstrapService(),
		FeatureService:   system_service.NewFeatureService(),
	})
	RegisterShortLinkRoutes(v1, ShortLinkRouteDeps{
		ShortLinkService: serviceContainer.GetShortLinkService(),
		Scheduler:        serviceContainer.GetScheduler(),
	})

	// TaskQueue debug routes removed (module deprecated)

	// Ensure DB ready
	db := database.GetDB()
	if db == nil {
		log.Fatal("Database connection not initialized")
	}

	accountService := serviceContainer.GetAccountServiceAdapter()
	tenantService := serviceContainer.GetTenantServiceAdapter()

	// ---------- User / Auth domain ----------
	RegisterUserRoutes(v1, UserRouteDeps{
		DB:                         db,
		AccountService:             serviceContainer.GetAccountService(),
		WorkspaceManagementService: serviceContainer.GetTenantService(),
		OrganizationService:        serviceContainer.GetOrganizationService(),
		DepartmentService:          serviceContainer.GetDepartmentService(),
		ConsoleWebURL:              config.GlobalConfig.Email.ConsoleWebURL,
	})

	// ---------- Workspace / Tenant ----------
	RegisterWorkspaceRoutes(v1, WorkspaceRouteDeps{
		DB:                               db,
		AccountService:                   accountService,
		OrganizationService:              serviceContainer.GetOrganizationService(),
		WorkspacePermissionFilterService: serviceContainer.GetWorkspacePermissionFilterService(),
		DepartmentService:                serviceContainer.GetDepartmentService(),
		ConsoleWebURL:                    config.GlobalConfig.Email.ConsoleWebURL,
	})

	// ---------- Explore ----------
	RegisterExploreRoutes(v1, accountService)

	tenantServiceImpl := serviceContainer.GetTenantService()

	// ---------- Plugin Runner (Tenant Level) ----------
	RegisterPluginRunnerTenantRoutes(v1, accountService, tenantServiceImpl, db)

	// ---------- Tool ----------
	RegisterToolRoutes(v1, ToolRouteDeps{
		ToolManager:                serviceContainer.GetToolManager(),
		AccountInstallationService: serviceContainer.GetAccountInstallationService(),
		MemberSubscriptionService:  serviceContainer.GetMemberSubscriptionService(),
		AccountService:             accountService,
		WorkspaceManagementService: tenantService,
	})

	// ---------- API Key ----------
	if _, ok := tenantServiceImpl.(*workspace_service.WorkspaceManagementServiceImpl); ok {
		RegisterAPIKeyRoutes(v1, db, accountService, serviceContainer.GetOrganizationService())
	}

	// ---------- File (common) ----------
	RegisterFileRoutes(v1, FileRouteDeps{
		DB:                         db,
		Storage:                    storage.GetStorage(),
		AccountService:             accountService,
		WorkspaceManagementService: serviceContainer.GetTenantService(),
		OrganizationService:        serviceContainer.GetOrganizationService(),
		QuotaService:               serviceContainer.GetQuotaService(),
		LLMClient:                  serviceContainer.GetLLMClient(),
		DefaultModelService:        serviceContainer.GetDefaultModelService(),
		DataLibraryModule:          serviceContainer.GetDataLibraryModule(),
		TaskManager:                serviceContainer.GetTaskManager(),
		Scheduler:                  serviceContainer.GetScheduler(),
		ScheduledFileService:       serviceContainer.GetFileService(),
	})

	// ---------- Memory (common) ----------
	RegisterMemoryRoutes(v1, MemoryRouteDeps{
		MemoryService:  serviceContainer.GetMemoryService(),
		AccountService: accountService,
	})

	// ---------- Dataset ----------
	RegisterDatasetRoutes(v1, DatasetRouteDeps{
		DB:                         db,
		Storage:                    storage.GetStorage(),
		AccountService:             accountService,
		WorkspaceManagementService: tenantService,
		OrganizationService:        serviceContainer.GetOrganizationService(),
		BillingService:             serviceContainer.GetBillingService(),
		QuotaService:               serviceContainer.GetQuotaService(),
		LLMClient:                  serviceContainer.GetLLMClient(),
		DefaultModelService:        serviceContainer.GetDefaultModelService(),
		TaskManager:                serviceContainer.GetTaskManager(),
		GraphFlowService:           serviceContainer.GetGraphFlowService(),
		TaskHandlerRegistry:        serviceContainer.GetTaskHandlerRegistry(),
		ResourcePermissionService:  serviceContainer.GetResourcePermissionService(),
		AuthorizationService:       serviceContainer.GetAuthorizationService(),
	})

	// ---------- RAG Evaluation ----------
	RegisterRAGEvaluationRoutes(v1, RAGEvaluationRouteDeps{
		AccountService:            accountService,
		OrganizationService:       serviceContainer.GetOrganizationService(),
		KnowledgeRetrievalService: serviceContainer.GetKnowledgeRetrievalService(),
		LLMClient:                 serviceContainer.GetLLMClient(),
		DefaultModelService:       serviceContainer.GetDefaultModelService(),
	})

	// ---------- Content Parse ----------
	RegisterContentParseRoutes(v1, ContentParseRouteDeps{
		DB:                  db,
		AccountService:      accountService,
		OrganizationService: serviceContainer.GetOrganizationService(),
		LLMClient:           serviceContainer.GetLLMClient(),
		DefaultModelService: serviceContainer.GetDefaultModelService(),
		Module:              serviceContainer.GetContentParseModule(),
	})

	// ---------- Data Library ----------
	RegisterDataLibraryRoutes(v1, DataLibraryRouteDeps{
		AccountService:    accountService,
		DataLibraryModule: serviceContainer.GetDataLibraryModule(),
		TaskManager:       serviceContainer.GetTaskManager(),
		TaskRegistry:      serviceContainer.GetTaskHandlerRegistry(),
	})

	// ---------- DataSource ----------
	RegisterDataSourceRoutes(v1, DataSourceRouteDeps{
		DataSourceService:   serviceContainer.GetDataSourceService(),
		AccountService:      serviceContainer.GetAccountService(),
		OrganizationService: serviceContainer.GetOrganizationService(),
	})

	// ---------- Automation ----------
	automationDefinitionService := RegisterAutomationRoutes(v1, AutomationRouteDeps{
		DB:                               db,
		TaskManager:                      serviceContainer.GetTaskManager(),
		TaskHandlerRegistry:              serviceContainer.GetTaskHandlerRegistry(),
		Scheduler:                        serviceContainer.GetScheduler(),
		NotificationSMSService:           serviceContainer.GetNotificationSMSService(),
		AutomationWorkflowRunnerProvider: serviceContainer.GetAutomationWorkflowRunner,
		AccountService:                   accountService,
		OrganizationService:              serviceContainer.GetOrganizationService(),
		WorkspaceManagementService:       tenantService,
		LLMClient:                        serviceContainer.GetLLMClient(),
		DefaultModelService:              serviceContainer.GetDefaultModelService(),
	})
	serviceContainer.SetAutomationDefinitionService(automationDefinitionService)

	// ---------- Payment ----------
	RegisterPaymentRoutes(v1, PaymentRouteDeps{
		DB:              db,
		AccountService:  accountService,
		ConsoleProvider: serviceContainer.GetConsoleProvider(),
	})

	// ---------- Quota ----------
	RegisterQuotaRoutes(v1, QuotaRouteDeps{
		QuotaService: serviceContainer.GetQuotaService(),
	})

	// ---------- Workflow ----------
	RegisterWorkflowRoutes(v1, WorkflowRouteDeps{
		DB:                          db,
		AccountService:              accountService,
		FileService:                 serviceContainer.GetFileService(),
		ContentExtractor:            serviceContainer.GetContentExtractor(),
		QuotaService:                serviceContainer.GetQuotaService(),
		OrganizationService:         serviceContainer.GetOrganizationService(),
		LLMClient:                   serviceContainer.GetLLMClient(),
		ToolEngine:                  serviceContainer.GetToolEngine(),
		GraphFlowService:            serviceContainer.GetGraphFlowService(),
		PromptResolver:              serviceContainer.GetPromptService(),
		AutomationDefinitionService: automationDefinitionService,
		TaskManager:                 serviceContainer.GetTaskManager(),
		TaskRegistry:                serviceContainer.GetTaskHandlerRegistry(),
		Scheduler:                   serviceContainer.GetScheduler(),
		EngineFactory:               workflowEngineFactory,
		AutomationRunnerSetter:      serviceContainer,
		ShortLinkService:            serviceContainer.GetShortLinkService(),
	})

	// ---------- Agent ----------
	resourcePermissionService := serviceContainer.GetResourcePermissionService()
	agentsService := RegisterAgentsRoutes(v1, db, accountService, tenantService, resourcePermissionService, serviceContainer.GetOrganizationService(), serviceContainer.GetQuotaService(), serviceContainer.GetFileService(), serviceContainer.GetContentExtractor(), serviceContainer.GetLLMClient(), serviceContainer.GetToolEngine(), serviceContainer.GetToolManager(), serviceContainer.GetMemoryService(), serviceContainer.GetGraphFlowService(), serviceContainer.GetPromptService(), serviceContainer.GetDataSourceService(), serviceContainer.GetKnowledgeRetrievalService(), workflowEngineFactory, serviceContainer.GetTaskManager(), serviceContainer.GetTaskHandlerRegistry(), serviceContainer.GetWorkflowTestService(), serviceContainer.GetScheduler(), config.Current().TaskQueue.WorkflowTestTaskBackend)

	// ---------- Prompt Library ----------
	RegisterPromptRoutes(v1, PromptRouteDeps{
		DB:                  serviceContainer.GetDB(),
		AccountService:      serviceContainer.GetAccountService(),
		OrganizationService: serviceContainer.GetOrganizationService(),
		LLMClient:           serviceContainer.GetLLMClient(),
		DefaultModelService: serviceContainer.GetDefaultModelService(),
	})

	// ---------- LLM Management ----------
	llmModule := RegisterLLMRoutes(v1, LLMRouteDeps{
		DB:                         db,
		AccountService:             serviceContainer.GetAccountService(),
		WorkspaceManagementService: serviceContainer.GetTenantService(),
		OrganizationService:        serviceContainer.GetOrganizationService(),
		ConsoleProvider:            serviceContainer.GetConsoleProvider(),
		Scheduler:                  serviceContainer.GetScheduler(),
	})
	if llmModule != nil && llmModule.LLMModelModule != nil {
		if err := serviceContainer.GetToolManager().RegisterProvider(agentmanagement_tools.NewProvider(agentsService, serviceContainer.GetOrganizationService(), llmModule.LLMModelModule.AvailableModelsSvc)); err != nil {
			log.Printf("failed to register agent management tools: %v", err)
		}
	} else if err := serviceContainer.GetToolManager().RegisterProvider(agentmanagement_tools.NewProvider(agentsService, serviceContainer.GetOrganizationService(), nil)); err != nil {
		log.Printf("failed to register agent management tools: %v", err)
	}

	// ---------- AIChat ----------
	RegisterAIChatRoutes(v1, AIChatRouteDeps{
		DB:                         db,
		LLMClient:                  serviceContainer.GetLLMClient(),
		DefaultModelService:        serviceContainer.GetDefaultModelService(),
		FileService:                serviceContainer.GetFileService(),
		ContentExtractor:           serviceContainer.GetContentExtractor(),
		WorkspacePermissionService: serviceContainer.GetOrganizationService(),
		MemoryService:              serviceContainer.GetMemoryService(),
		AgentMemoryService:         serviceContainer.GetAgentMemoryService(),
		SkillRuntime:               newSkillRuntimeWithSandbox(serviceContainer.GetToolEngine(), serviceContainer.GetToolManager(), serviceContainer.GetFileService(), serviceContainer.GetOrganizationService()),
		AccountService:             accountService,
	})

	// ---------- Dashboard ----------
	var availableModels system_service.AvailableModelsLister
	if llmModule != nil && llmModule.LLMModelModule != nil {
		availableModels = llmModule.LLMModelModule.AvailableModelsSvc
	}
	RegisterDashboardRoutes(v1, DashboardRouteDeps{
		DB:                  db,
		AccountService:      accountService,
		OrganizationService: serviceContainer.GetOrganizationService(),
		AvailableModels:     availableModels,
	})

	// ---------- GDPR Compliance ----------
	RegisterGDPRRoutes(v1, GDPRRouteDeps{
		DB:             db,
		AccountService: accountService,
	})
}
