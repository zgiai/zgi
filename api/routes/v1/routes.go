package v1

import (
	"log"

	"github.com/gin-gonic/gin"
	"github.com/zgiai/zgi/api/config"
	"github.com/zgiai/zgi/api/internal/container"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/graph_engine"
	workspace_service "github.com/zgiai/zgi/api/internal/modules/workspace/service"
	"github.com/zgiai/zgi/api/pkg/database"
)

// RegisterRoutes registers all v1 version routes
func RegisterRoutes(engine *gin.Engine, v1 *gin.RouterGroup, serviceContainer *container.ServiceContainer, workflowEngineFactory *graph_engine.EngineFactory) {
	// Health & setup routes first
	RegisterHealthRoutes(v1)
	RegisterSetupRoutes(v1, serviceContainer)

	// TaskQueue debug routes removed (module deprecated)

	// Ensure DB ready
	db := database.GetDB()
	if db == nil {
		log.Fatal("Database connection not initialized")
	}

	accountService := serviceContainer.GetAccountServiceAdapter()
	tenantService := serviceContainer.GetTenantServiceAdapter()

	// ---------- User / Auth domain ----------
	RegisterUserRoutes(v1, serviceContainer, config.GlobalConfig.Email.ConsoleWebURL)

	// ---------- Workspace / Tenant ----------
	RegisterWorkspaceRoutes(v1, db, accountService, config.GlobalConfig.Email.ConsoleWebURL, serviceContainer)

	// ---------- Explore ----------
	RegisterExploreRoutes(v1, accountService)

	tenantServiceImpl := serviceContainer.GetTenantService()

	// ---------- Plugin Runner (Tenant Level) ----------
	RegisterPluginRunnerTenantRoutes(v1, accountService, tenantServiceImpl, db)

	// ---------- Tool ----------
	RegisterToolRoutes(v1, serviceContainer)

	// ---------- API Key ----------
	if tenantServiceImplConcrete, ok := tenantServiceImpl.(*workspace_service.WorkspaceManagementServiceImpl); ok {
		RegisterAPIKeyRoutes(v1, db, accountService, tenantServiceImplConcrete)
	}

	// ---------- File (common) ----------
	RegisterFileRoutes(v1, accountService, serviceContainer)

	// ---------- Memory (common) ----------
	RegisterMemoryRoutes(v1, MemoryRouteDeps{
		MemoryService:  serviceContainer.GetMemoryService(),
		AccountService: accountService,
	})

	// ---------- Dataset ----------
	RegisterDatasetRoutes(v1, serviceContainer)

	// ---------- Content Parse ----------
	RegisterContentParseRoutes(v1, serviceContainer)

	// ---------- Data Library ----------
	RegisterDataLibraryRoutes(v1, serviceContainer)

	// ---------- DataSource ----------
	RegisterDataSourceRoutes(v1, serviceContainer)

	// ---------- Automation ----------
	RegisterAutomationRoutes(v1, serviceContainer)

	// ---------- Payment ----------
	RegisterPaymentRoutes(v1, serviceContainer)

	// ---------- Quota ----------
	RegisterQuotaRoutes(v1, QuotaRouteDeps{
		QuotaService: serviceContainer.GetQuotaService(),
	})

	// ---------- Workflow ----------
	RegisterWorkflowRoutes(v1, accountService, tenantService, serviceContainer.GetFileService(), db, serviceContainer.GetContentExtractor(), serviceContainer.GetQuotaService(), serviceContainer.GetOrganizationService(), serviceContainer.GetLLMClient(), serviceContainer.GetToolEngine(), serviceContainer.GetGraphFlowService(), serviceContainer.GetPromptService(), serviceContainer.GetAutomationDefinitionService(), serviceContainer.GetTaskManager(), serviceContainer.GetTaskHandlerRegistry(), serviceContainer.GetScheduler(), workflowEngineFactory, serviceContainer)

	// ---------- Agent ----------
	resourcePermissionService := serviceContainer.GetResourcePermissionService()
	RegisterAgentsRoutes(v1, db, accountService, tenantService, resourcePermissionService, serviceContainer.GetOrganizationService(), serviceContainer.GetQuotaService(), serviceContainer.GetFileService(), serviceContainer.GetContentExtractor(), serviceContainer.GetLLMClient(), serviceContainer.GetToolEngine(), serviceContainer.GetGraphFlowService(), serviceContainer.GetPromptService(), workflowEngineFactory, serviceContainer.GetTaskManager(), serviceContainer.GetTaskHandlerRegistry())

	// ---------- Prompt Library ----------
	RegisterPromptRoutes(v1, PromptRouteDeps{
		DB:                  serviceContainer.GetDB(),
		AccountService:      serviceContainer.GetAccountService(),
		OrganizationService: serviceContainer.GetOrganizationService(),
		LLMClient:           serviceContainer.GetLLMClient(),
		DefaultModelService: serviceContainer.GetDefaultModelService(),
	})

	// ---------- LLM Management ----------
	llmModule := RegisterLLMRoutes(v1, serviceContainer)

	// ---------- AIChat ----------
	RegisterAIChatRoutes(v1, serviceContainer)

	// ---------- Dashboard ----------
	RegisterDashboardRoutes(v1, serviceContainer, llmModule)

	// ---------- GDPR Compliance ----------
	RegisterGDPRRoutes(v1, GDPRRouteDeps{
		DB:             db,
		AccountService: accountService,
	})
}
