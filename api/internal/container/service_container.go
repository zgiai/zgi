package container

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/hibiken/asynq"
	"github.com/zgiai/zgi/api/config"
	shortlinkcap "github.com/zgiai/zgi/api/internal/capabilities/shortlink"
	"github.com/zgiai/zgi/api/internal/infra/platform"
	"github.com/zgiai/zgi/api/internal/infra/platform/console"
	"github.com/zgiai/zgi/api/internal/modules/agentmemory"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/diagnosis"
	workflow_file "github.com/zgiai/zgi/api/internal/modules/app/workflow/file"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/graph_engine"
	workflowruntime "github.com/zgiai/zgi/api/internal/modules/app/workflow/runtime"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/tool_file"
	workflowtest "github.com/zgiai/zgi/api/internal/modules/app/workflowtest"
	automationaction "github.com/zgiai/zgi/api/internal/modules/automation/service/action"
	automationdefinition "github.com/zgiai/zgi/api/internal/modules/automation/service/definition"
	datalibrarymodule "github.com/zgiai/zgi/api/internal/modules/datalibrary"
	"github.com/zgiai/zgi/api/internal/modules/dataset/graphflow"
	dataset_repo "github.com/zgiai/zgi/api/internal/modules/dataset/repository"
	dataset_service "github.com/zgiai/zgi/api/internal/modules/dataset/service"
	"github.com/zgiai/zgi/api/internal/modules/datasource/repository"
	"github.com/zgiai/zgi/api/internal/modules/datasource/service"
	file_repo "github.com/zgiai/zgi/api/internal/modules/file_process/repository"
	file_service "github.com/zgiai/zgi/api/internal/modules/file_process/service"
	"github.com/zgiai/zgi/api/internal/modules/file_process/service/extractor"
	notificationsms "github.com/zgiai/zgi/api/internal/modules/notification/sms"
	pluginrunner_client "github.com/zgiai/zgi/api/internal/modules/pluginrunner/client"
	pluginrunner_repo "github.com/zgiai/zgi/api/internal/modules/pluginrunner/repository"
	pluginrunner_service "github.com/zgiai/zgi/api/internal/modules/pluginrunner/service"
	promptsmodule "github.com/zgiai/zgi/api/internal/modules/prompts"
	promptservice "github.com/zgiai/zgi/api/internal/modules/prompts/service"
	"github.com/zgiai/zgi/api/internal/modules/shared/adapters"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	system_repo "github.com/zgiai/zgi/api/internal/modules/system/repository"
	system_service "github.com/zgiai/zgi/api/internal/modules/system/service"
	"github.com/zgiai/zgi/api/internal/modules/tools"
	auth_repo "github.com/zgiai/zgi/api/internal/modules/user/auth/repository"
	auth_service "github.com/zgiai/zgi/api/internal/modules/user/auth/service"
	workspace_repo "github.com/zgiai/zgi/api/internal/modules/workspace/repository"
	workspace_service "github.com/zgiai/zgi/api/internal/modules/workspace/service"

	// Shared repositories and services
	shared_repo "github.com/zgiai/zgi/api/internal/modules/shared/repository"
	shared_service "github.com/zgiai/zgi/api/internal/modules/shared/service"

	// Quota management
	quota_repo "github.com/zgiai/zgi/api/internal/modules/quota/repository"
	quota_service "github.com/zgiai/zgi/api/internal/modules/quota/service"

	// Payment services
	payment_service "github.com/zgiai/zgi/api/internal/modules/payment/service"

	// Provider/model management
	apikeyrepo "github.com/zgiai/zgi/api/internal/modules/llm/apikey/repository"
	channelsvc "github.com/zgiai/zgi/api/internal/modules/llm/channel/service"
	llm_client "github.com/zgiai/zgi/api/internal/modules/llm/client"
	llmdefaultmodel "github.com/zgiai/zgi/api/internal/modules/llm/defaultmodel"
	llmdefaultsvc "github.com/zgiai/zgi/api/internal/modules/llm/defaultmodel/service"
	"github.com/zgiai/zgi/api/internal/modules/llm/gateway"
	llmmodel "github.com/zgiai/zgi/api/internal/modules/llm/llmmodel"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/modules/memory"
	database_tools "github.com/zgiai/zgi/api/internal/modules/tools/builtin/database"
	knowledge_tools "github.com/zgiai/zgi/api/internal/modules/tools/builtin/knowledge"
	helper "github.com/zgiai/zgi/api/internal/util"
	"github.com/zgiai/zgi/api/pkg/logger"
	"github.com/zgiai/zgi/api/pkg/queue"
	redisPkg "github.com/zgiai/zgi/api/pkg/redis"
	"github.com/zgiai/zgi/api/pkg/scheduler"
	"github.com/zgiai/zgi/api/pkg/storage"
	"gorm.io/gorm"
)

// TaskHandlerRegistrar is a centralized registry for task handlers
type TaskHandlerRegistrar struct {
	mu       sync.RWMutex
	handlers map[string]func(context.Context, *asynq.Task) error
}

// NewTaskHandlerRegistrar creates a new task handler registrar
func NewTaskHandlerRegistrar() *TaskHandlerRegistrar {
	return &TaskHandlerRegistrar{
		handlers: make(map[string]func(context.Context, *asynq.Task) error),
	}
}

// Register registers a task handler for a specific task type
// Returns true if the handler was newly registered, false if it replaced an existing handler
func (r *TaskHandlerRegistrar) Register(taskType string, handler func(context.Context, *asynq.Task) error) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Check if handler already exists
	_, exists := r.handlers[taskType]
	r.handlers[taskType] = handler

	// Return true if it's a new registration, false if it replaced an existing one
	return !exists
}

// RegisterAll registers all handlers to the given mux
func (r *TaskHandlerRegistrar) RegisterAll(mux *asynq.ServeMux) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for taskType, handler := range r.handlers {
		mux.HandleFunc(taskType, func(ctx context.Context, task *asynq.Task) error {
			return handler(logger.WithTaskContext(ctx, task), task)
		})
	}
}

type SimpleEventBus struct{}

func (e *SimpleEventBus) Publish(ctx context.Context, topic string, payload interface{}) error {
	return nil
}

type ServiceContainer struct {
	db       *gorm.DB
	tokenMgr *helper.TokenManager
	config   *config.Config

	// Repositories
	workspaceRepo       workspace_repo.WorkspaceRepository
	workspaceMemberRepo workspace_repo.WorkspaceMemberRepository

	accountServiceImpl *auth_service.AccountService
	tenantServiceImpl  *workspace_service.WorkspaceManagementServiceImpl

	accountService       interfaces.AccountService
	tenantService        interfaces.WorkspaceManagementService
	accountTenantService *auth_service.AccountTenantService

	// Service Adapters
	accountServiceAdapter interfaces.AccountService
	tenantServiceAdapter  interfaces.WorkspaceManagementService

	organizationManagementService interfaces.OrganizationManagementService
	organizationService           interfaces.OrganizationService
	officialRouteBootstrapper     interfaces.OfficialRouteBootstrapper
	bootstrapService              *system_service.BootstrapService
	datasetService                dataset_service.DatasetService
	pluginRunnerService           pluginrunner_service.PluginRunnerService
	accountInstallationService    pluginrunner_service.AccountInstallationService
	memberSubscriptionService     pluginrunner_service.MemberSubscriptionService
	billingService                interfaces.BillingService
	registerService               interfaces.RegisterService
	fileService                   interfaces.FileService
	contentExtractor              workflow_file.ContentExtractor

	// DataSource service
	dataSourceService service.DataSourceService
	promptModule      *promptsmodule.Module

	// Data Library module
	dataLibraryModule *datalibrarymodule.Module

	// Tenant permission filter service
	tenantPermissionFilterService workspace_service.WorkspacePermissionFilterService

	departmentService workspace_service.DepartmentService

	// Permission services
	permissionRepo            shared_repo.PermissionRepository
	resourcePermissionService interfaces.ResourcePermissionService

	// Task management
	taskManager         *queue.TaskManager
	taskHandlerRegistry *TaskHandlerRegistrar

	// Scheduler
	scheduler                *scheduler.Scheduler
	schedulerTasksRegistered bool

	// LLM repositories (lazy-loaded)
	llmAPIKeyRepo apikeyrepo.APIKeyRepository

	// LLM services (lazy-loaded)
	llmClient         llm_client.LLMClient
	llmClientInitOnce sync.Once
	llmClientInitErr  error

	// Quota service
	quotaService interfaces.QuotaService

	// Payment services
	paymentServices *payment_service.PaymentServices

	// Tool manager for builtin and plugin runner tools
	toolManager *tools.ToolManager

	// Tool engine
	toolEngine *tools.ToolEngine

	// Account memory
	memoryService      *memory.Service
	agentMemoryService *agentmemory.Service

	// Knowledge retrieval for builtin knowledge tools
	knowledgeRetrievalService *dataset_service.KnowledgeRetrievalService

	// Platform container
	platformContainer *platform.Container

	// Default model service
	defaultModelModule  *llmdefaultmodel.Module
	defaultModelService llmdefaultsvc.DefaultModelService

	// GraphFlow service
	graphFlowService *graphflow.Service

	// Automation definition service
	automationDefinitionService automationdefinition.Service
	automationWorkflowRunner    automationaction.AutomationWorkflowRunner
	notificationSMSService      notificationsms.Service
	shortLinkService            shortlinkcap.Service

	// Workflow engine factory
	workflowEngineFactory *graph_engine.EngineFactory
	workflowTestService   *workflowtest.Service

	// Workflow Diagnoser
	workflowDiagnoser *diagnosis.Diagnoser
}

func (c *ServiceContainer) GetEnterpriseGroupService() interfaces.OrganizationManagementService {
	if c.organizationManagementService == nil {
		var cp console.ConsoleProvider
		if c.platformContainer != nil {
			cp = c.platformContainer.Console
		}
		c.organizationManagementService = workspace_service.NewOrganizationManagementService(c.db, cp)
	}
	return c.organizationManagementService
}

func (c *ServiceContainer) GetBillingService() interfaces.BillingService {
	if c.billingService == nil {
		c.billingService = auth_service.NewBillingService()
	}
	return c.billingService
}

func (c *ServiceContainer) GetPaymentServices() *payment_service.PaymentServices {
	if c.paymentServices == nil {
		c.paymentServices = payment_service.NewPaymentServices(c.db)
	}
	return c.paymentServices
}

func (c *ServiceContainer) GetDB() *gorm.DB {
	return c.db
}

func (c *ServiceContainer) GetTaskManager() *queue.TaskManager {
	if c.taskManager == nil {
		taskManager, err := queue.NewTaskManager(c.config)
		if err != nil {
			panic("failed to create task manager: " + err.Error())
		}
		c.taskManager = taskManager
	}
	return c.taskManager
}

func (c *ServiceContainer) GetTaskHandlerRegistry() *TaskHandlerRegistrar {
	if c.taskHandlerRegistry == nil {
		c.taskHandlerRegistry = NewTaskHandlerRegistrar()
	}
	return c.taskHandlerRegistry
}

func (c *ServiceContainer) GetWorkflowTestService() *workflowtest.Service {
	if c.workflowTestService == nil {
		c.workflowTestService = workflowtest.NewService(workflowtest.NewRepository(c.db))
	}
	return c.workflowTestService
}

func (c *ServiceContainer) GetTenantService() interfaces.WorkspaceManagementService {
	if c.tenantService == nil {
		c.tenantService = workspace_service.NewWorkspaceManagementService(
			c.db,
			c.workspaceRepo,
			c.workspaceMemberRepo,
			nil, // accountService will be set later via SetAccountService
			c.GetQuotaService(),
			nil, // enterpriseService will be set later to avoid circular dependency
		)
	}
	return c.tenantService
}

func (c *ServiceContainer) GetAccountServiceImpl() *auth_service.AccountService {
	if c.accountServiceImpl == nil {
		accountRepo := auth_repo.NewAccountRepository(c.db)

		c.accountServiceImpl = auth_service.NewAccountService(
			accountRepo,
			c.db,
			c.tokenMgr,
			c.GetTenantService(),
			c.GetBillingService(),
			nil,
			c.GetEnterpriseGroupService(),
			nil,
			system_service.NewSystemConfigService(),
			&SimpleEventBus{},
			c.GetConsoleProvider(),
		)
		c.accountServiceImpl.SetOfficialRouteBootstrapper(c.GetOfficialRouteBootstrapper())
	}
	return c.accountServiceImpl
}

func (c *ServiceContainer) GetAccountServiceAdapter() interfaces.AccountService {
	if c.accountServiceAdapter == nil {
		c.accountServiceAdapter = adapters.NewAccountServiceAdapter(c.GetAccountServiceImpl(), nil)
	}
	return c.accountServiceAdapter
}

func (c *ServiceContainer) GetAccountService() interfaces.AccountService {
	if c.accountService == nil {
		accountServiceImpl := c.GetAccountServiceImpl()
		c.accountService = adapters.NewAccountServiceAdapter(accountServiceImpl, nil)
	}
	return c.accountService
}

func (c *ServiceContainer) GetWorkspacePermissionFilterService() workspace_service.WorkspacePermissionFilterService {
	if c.tenantPermissionFilterService == nil {
		enterpriseRepo := workspace_repo.NewOrganizationRepository(c.db)
		c.tenantPermissionFilterService = workspace_service.NewWorkspacePermissionFilterService(
			enterpriseRepo,
			c.workspaceRepo,
			c.workspaceMemberRepo,
		)
	}
	return c.tenantPermissionFilterService
}

func (c *ServiceContainer) GetDepartmentService() workspace_service.DepartmentService {
	if c.departmentService == nil {
		enterpriseRepo := workspace_repo.NewOrganizationRepository(c.db)
		deptRepo := workspace_repo.NewDepartmentRepository(c.db)
		c.departmentService = workspace_service.NewDepartmentService(deptRepo, enterpriseRepo)
	}
	return c.departmentService
}

func (c *ServiceContainer) InitializeDependencies() {
	c.initializeCoreServices()
	c.wireAccountDependencies()
	c.wireWorkspaceDependencies()
	c.initializeWorkflowFileDependencies()
}

func (c *ServiceContainer) initializeCoreServices() {
	_ = c.GetQuotaService()
	_ = c.GetTenantService()
	_ = c.GetAccountServiceImpl()
	_ = c.GetOrganizationService()
}

func (c *ServiceContainer) wireAccountDependencies() {
	accountServiceImpl := c.GetAccountServiceImpl()
	if accountServiceImpl != nil {
		accountServiceImpl.SetRegisterService(c.GetRegisterService())
		accountServiceImpl.SetOrganizationService(c.GetOrganizationService())
	}

	c.wireRegisterServiceForAccountAdapter(c.GetAccountService())
	c.wireRegisterServiceForAccountAdapter(c.GetAccountServiceAdapter())
}

func (c *ServiceContainer) wireWorkspaceDependencies() {
	if c.tenantService == nil {
		return
	}

	tenantServiceImpl, ok := c.tenantService.(*workspace_service.WorkspaceManagementServiceImpl)
	if !ok {
		return
	}

	tenantServiceImpl.SetAccountService(c.GetAccountServiceAdapter())
	// Set enterprise service after initialization to break circular dependency.
	tenantServiceImpl.SetOrganizationService(c.GetOrganizationService())
}

func (c *ServiceContainer) wireRegisterServiceForAccountAdapter(accountService interfaces.AccountService) {
	if adapter, ok := accountService.(*adapters.AccountServiceAdapter); ok {
		adapter.SetRegisterService(c.GetRegisterService())
	}
}

func (c *ServiceContainer) initializeWorkflowFileDependencies() {
	// Rebuild FileService after quota and organization dependencies have been wired.
	c.fileService = nil
	fileService := c.GetFileService()

	storageClient := storage.GetStorage()
	extractProcessor := extractor.NewExtractProcessor(storageClient)
	workflow_file.InitGlobalContentExtractor(fileService, extractProcessor)
}

func (c *ServiceContainer) GetRegisterService() interfaces.RegisterService {
	if c.registerService == nil {
		accountRepo := auth_repo.NewAccountRepository(c.db)
		invitationRepo := auth_repo.NewInvitationRepository(c.db)

		registerService := auth_service.NewRegisterService(
			c.db,
			accountRepo,
			invitationRepo,
			c.GetTenantService(),
			c.GetEnterpriseGroupService(),
			c.tokenMgr,
			c.GetBillingService(),
		)
		registerService.SetOfficialRouteBootstrapper(c.GetOfficialRouteBootstrapper())
		c.registerService = registerService
	}
	return c.registerService
}

func (c *ServiceContainer) GetTenantServiceAdapter() interfaces.WorkspaceManagementService {
	if c.tenantServiceAdapter == nil {
		tenantService := c.GetTenantService()
		if tenantServiceImpl, ok := tenantService.(*workspace_service.WorkspaceManagementServiceImpl); ok {
			c.tenantServiceAdapter = adapters.NewTenantServiceAdapter(tenantServiceImpl)
		}
	}
	return c.tenantServiceAdapter
}

func (c *ServiceContainer) GetAccountTenantService() *auth_service.AccountTenantService {
	if c.accountTenantService == nil {
		accountRepo := auth_repo.NewAccountRepository(c.db)
		c.accountTenantService = auth_service.NewAccountTenantService(
			accountRepo,
			c.GetTenantService(),
		)
	}
	return c.accountTenantService
}

func NewServiceContainer(
	db *gorm.DB,
	tokenMgr *helper.TokenManager,
	cfg *config.Config,
	workspaceRepo workspace_repo.WorkspaceRepository,
	workspaceMemberRepo workspace_repo.WorkspaceMemberRepository,
	platformContainer *platform.Container,
) *ServiceContainer {
	storageClient := storage.GetStorage()
	tool_file.InitToolFileManager(db, storageClient)
	tool_file.InitFileSignature(cfg)

	return &ServiceContainer{
		db:                  db,
		tokenMgr:            tokenMgr,
		config:              cfg,
		workspaceRepo:       workspaceRepo,
		workspaceMemberRepo: workspaceMemberRepo,
		platformContainer:   platformContainer,
	}
}

func (c *ServiceContainer) GetOrganizationService() interfaces.OrganizationService {
	if c.organizationService == nil {
		organizationRepo := workspace_repo.NewOrganizationRepository(c.db)
		systemConfigService := system_service.NewSystemConfigService()
		featureService := system_service.NewFeatureService()

		c.organizationService = workspace_service.NewOrganizationService(
			organizationRepo,
			c.GetAccountService(),
			c.workspaceRepo,
			c.GetTenantService(),
			featureService,
			systemConfigService,
			c.GetDatasetService(),
			c.db,
			c.getConsoleProvider(),
			c.GetOfficialRouteBootstrapper(),
		)
		c.GetDatasetService().SetOrganizationService(c.organizationService)
	}
	return c.organizationService
}

func (c *ServiceContainer) GetOfficialRouteBootstrapper() interfaces.OfficialRouteBootstrapper {
	if c.officialRouteBootstrapper == nil {
		c.officialRouteBootstrapper = channelsvc.NewOfficialRouteBootstrapper(c.db, c.getConsoleProvider())
	}
	return c.officialRouteBootstrapper
}

func (c *ServiceContainer) GetBootstrapService() *system_service.BootstrapService {
	if c.bootstrapService == nil {
		setupRepo := system_repo.NewSetupRepository(c.db)
		lockRepo := system_repo.NewBootstrapLockRepository(c.db)
		accountRepo := auth_repo.NewAccountRepository(c.db)

		c.bootstrapService = system_service.NewBootstrapService(
			setupRepo,
			lockRepo,
			accountRepo,
			c.db,
			c.GetTenantService(),
			c.GetEnterpriseGroupService(),
			system_service.NewSystemConfigService(),
		)
	}

	return c.bootstrapService
}

func (c *ServiceContainer) getConsoleProvider() console.ConsoleProvider {
	if c.platformContainer == nil {
		return nil
	}
	return c.platformContainer.Console
}

func (c *ServiceContainer) GetDatasetService() dataset_service.DatasetService {
	if c.datasetService == nil {
		datasetRepo := dataset_repo.NewDatasetRepository(c.db)
		documentRepo := dataset_repo.NewDocumentRepository(c.db)
		chunkRepo := dataset_repo.NewChunkRepository(c.db)

		tenantService := c.GetTenantService()
		storageInstance := storage.GetStorage()
		quotaService := c.GetQuotaService()
		llmClient := c.GetLLMClient()

		c.datasetService = dataset_service.NewDatasetService(
			datasetRepo,
			documentRepo,
			chunkRepo,
			tenantService,
			nil,
			nil,
			nil,
			nil,
			storageInstance,
			c.db,
			quotaService,
			nil,
			llmClient,
			c.GetTaskManager(),
		)
		if c.organizationService != nil {
			c.datasetService.SetOrganizationService(c.organizationService)
		}
	}
	return c.datasetService
}

func (c *ServiceContainer) GetFileService() interfaces.FileService {
	if c.fileService == nil {
		fileRepo := file_repo.NewFileRepository(c.db)
		storageClient := storage.GetStorage()
		quotaService := c.GetQuotaService()
		logger.Info("GetFileService: quotaService is nil?", "is_nil", quotaService == nil)
		enterpriseService := c.GetOrganizationService()
		logger.Info("GetFileService: enterpriseService is nil?", "is_nil", enterpriseService == nil)
		c.fileService = file_service.NewFileServiceWithVision(fileRepo, storageClient, c.db, quotaService, enterpriseService, c.GetLLMClient(), c.GetDefaultModelService())
	}
	return c.fileService
}

func (c *ServiceContainer) GetContentExtractor() workflow_file.ContentExtractor {
	if c.contentExtractor == nil {
		// Get FileService dependency
		fileService := c.GetFileService()

		// Get storage instance
		storageClient := storage.GetStorage()

		// Create ExtractProcessor with storage
		extractProcessor := extractor.NewExtractProcessor(storageClient)

		// Get content extractor configuration
		config := workflow_file.GetContentExtractorConfig()

		// Create ContentExtractor with FileService, ExtractProcessor, and Config
		c.contentExtractor = workflow_file.NewContentExtractor(fileService, extractProcessor, config)
	}
	return c.contentExtractor
}

func (c *ServiceContainer) GetDataSourceService() service.DataSourceService {
	if c.dataSourceService == nil {
		dataSourceRepo := repository.NewPostgresDataSourceRepository(c.db)
		tableRepo := repository.NewPostgresTableRepository(c.db)
		promptRepo := repository.NewPostgresPromptRepository(c.db)
		sqlOperationRepo := repository.NewPostgresSQLOperationRepository(c.db)
		c.dataSourceService = service.NewDataSourceService(dataSourceRepo, tableRepo, promptRepo, sqlOperationRepo, c.GetAccountService(), c.GetFileService(), c.GetOrganizationService(), c.GetResourcePermissionService(), c.GetQuotaService(), c.GetLLMClient(), c.db)
	}

	return c.dataSourceService
}

func (c *ServiceContainer) GetDataLibraryModule() *datalibrarymodule.Module {
	if c.dataLibraryModule == nil {
		c.dataLibraryModule = datalibrarymodule.NewModule(c.db)
	}
	return c.dataLibraryModule
}

func (c *ServiceContainer) GetPromptService() promptservice.PromptService {
	if c.promptModule == nil {
		c.promptModule = promptsmodule.NewModule(
			c.db,
			c.GetAccountService(),
			c.GetOrganizationService(),
			c.GetLLMClient(),
			c.GetDefaultModelService(),
		)
	}
	return c.promptModule.PromptService
}

// LLM Repository Getters

func (c *ServiceContainer) GetLLMAPIKeyRepository() apikeyrepo.APIKeyRepository {
	if c.llmAPIKeyRepo == nil {
		c.llmAPIKeyRepo = apikeyrepo.NewAPIKeyRepository(c.db)
	}
	return c.llmAPIKeyRepo
}

// Permission Repository and Service Getters

func (c *ServiceContainer) GetPermissionRepository() shared_repo.PermissionRepository {
	if c.permissionRepo == nil {
		c.permissionRepo = shared_repo.NewPermissionRepository(c.db)
	}
	return c.permissionRepo
}

func (c *ServiceContainer) GetResourcePermissionService() interfaces.ResourcePermissionService {
	if c.resourcePermissionService == nil {
		c.resourcePermissionService = shared_service.NewResourcePermissionService(c.GetPermissionRepository())
	}
	return c.resourcePermissionService
}

// GetScheduler returns the scheduler instance
func (c *ServiceContainer) GetScheduler() *scheduler.Scheduler {
	if c.scheduler == nil {
		s, err := scheduler.NewScheduler(c.config)
		if err != nil {
			panic("failed to create scheduler: " + err.Error())
		}
		c.scheduler = s
	}
	return c.scheduler
}

// SetScheduler sets the scheduler instance
func (c *ServiceContainer) SetScheduler(s *scheduler.Scheduler) {
	c.scheduler = s
}

// StartScheduler starts the scheduler with all registered tasks
// This should be called after all modules have registered their tasks
func (c *ServiceContainer) StartScheduler() error {
	if c.scheduler == nil {
		return nil // No scheduler configured
	}
	return c.scheduler.Start()
}

// StopScheduler stops the scheduler
func (c *ServiceContainer) StopScheduler() {
	if c.scheduler != nil {
		c.scheduler.Stop()
	}
}

// GetLLMClient returns the LLM client for internal modules
// This client can be used by workflows, knowledge base, and other internal modules
// to access LLM capabilities without managing API keys manually.
func (c *ServiceContainer) GetLLMClient() llm_client.LLMClient {
	c.initLLMClient()
	return c.llmClient
}

// EnsureLLMClient initializes LLM client once and enforces fail-fast in CLOUD mode.
func (c *ServiceContainer) EnsureLLMClient() error {
	c.initLLMClient()

	consoleProvider := c.GetConsoleProvider()
	if consoleProvider != nil && consoleProvider.GetMode() == "CLOUD" && c.llmClientInitErr != nil {
		return fmt.Errorf("cloud llm client initialization failed: %w", c.llmClientInitErr)
	}
	return nil
}

func (c *ServiceContainer) initLLMClient() {
	c.llmClientInitOnce.Do(func() {
		gatewayService, err := gateway.NewLLMGatewayService(
			c.db,
			c.GetLLMAPIKeyRepository(),
			adapter.GlobalFactory,
		)
		if err != nil {
			c.llmClientInitErr = fmt.Errorf("failed to initialize gateway service for llm client: %w", err)
			return
		}

		// Set config cache if Redis is available.
		redisClient := redisPkg.GetClient()
		if redisClient != nil {
			configCache := gateway.NewConfigCache(redisClient, c.db, nil)
			gatewayService.SetConfigCache(configCache)
		}

		c.llmClient = llm_client.New(
			gatewayService,
			c.GetLLMAPIKeyRepository(),
			c.db,
		)
	})
}

// GetQuotaService returns the quota service for quota management
func (c *ServiceContainer) GetQuotaService() interfaces.QuotaService {
	if c.quotaService == nil {
		logger.Debug("GetQuotaService: creating new quota service instance")
		quotaRepo := quota_repo.NewQuotaRepository(c.db)
		c.quotaService = quota_service.NewQuotaService(quotaRepo, c.db)
		logger.Info("GetQuotaService: quota service created successfully")
	}
	return c.quotaService
}

// GetPluginRunnerService returns the plugin runner service for managing plugins
func (c *ServiceContainer) GetPluginRunnerService() pluginrunner_service.PluginRunnerService {
	if c.pluginRunnerService == nil {
		if !c.config.PluginRunner.Enabled {
			return nil // Plugin runner is disabled
		}

		cfg := &pluginrunner_client.Config{
			BaseURL: c.config.PluginRunner.BaseURL,
			APIKey:  c.config.PluginRunner.APIKey,
			Timeout: time.Duration(c.config.PluginRunner.Timeout) * time.Second,
		}

		// Create repos for database writes during installation
		installRepo := pluginrunner_repo.NewAccountInstallationRepository(c.db)
		infoRepo := pluginrunner_repo.NewInstalledPluginInfoRepository(c.db)

		c.pluginRunnerService = pluginrunner_service.NewPluginRunnerServiceWithRepos(cfg, installRepo, infoRepo)
	}
	return c.pluginRunnerService
}

// GetAccountInstallationService returns the account installation service for reading plugin declarations
func (c *ServiceContainer) GetAccountInstallationService() pluginrunner_service.AccountInstallationService {
	if c.accountInstallationService == nil {
		infoRepo := pluginrunner_repo.NewInstalledPluginInfoRepository(c.db)
		installRepo := pluginrunner_repo.NewAccountInstallationRepository(c.db)
		c.accountInstallationService = pluginrunner_service.NewAccountInstallationService(installRepo, infoRepo)
	}
	return c.accountInstallationService
}

// GetToolManager returns the tool manager for builtin and plugin runner tools
func (c *ServiceContainer) GetToolManager() *tools.ToolManager {
	if c.toolManager == nil {
		// Create plugin runner adapter if plugin runner is enabled
		var pluginRunnerAdapter tools.PluginRunnerToolManagerInterface
		pluginRunnerService := c.GetPluginRunnerService()
		if pluginRunnerService != nil {
			pluginRunnerAdapter = tools.NewPluginRunnerToolAdapter(
				pluginRunnerService,
				c.GetAccountInstallationService(),
			)
		}

		// Create tool manager
		c.toolManager = tools.NewToolManager(pluginRunnerAdapter)

		// Register builtin tool providers
		c.toolManager.RegisterBuiltinProviders(getBuiltinToolProviders())
		_ = c.toolManager.RegisterProvider(knowledge_tools.NewProvider(c.GetKnowledgeRetrievalService()))
		_ = c.toolManager.RegisterProvider(database_tools.NewProvider(c.GetDataSourceService(), c.GetOrganizationService()))

		logger.Info("ToolManager initialized with builtin providers")
	}
	return c.toolManager
}

// GetToolEngine returns the tool engine
func (c *ServiceContainer) GetToolEngine() *tools.ToolEngine {
	if c.toolEngine == nil {
		c.toolEngine = tools.NewToolEngine(c.GetToolManager())
	}
	return c.toolEngine
}

func (c *ServiceContainer) GetMemoryService() *memory.Service {
	if c.memoryService == nil {
		c.memoryService = memory.NewService(c.db)
	}
	return c.memoryService
}

func (c *ServiceContainer) GetKnowledgeRetrievalService() *dataset_service.KnowledgeRetrievalService {
	if c.knowledgeRetrievalService == nil {
		c.knowledgeRetrievalService = dataset_service.NewKnowledgeRetrievalService(
			c.db,
			c.config,
			c.GetLLMClient(),
			c.GetDefaultModelService(),
			c.GetGraphFlowService(),
		)
	}
	return c.knowledgeRetrievalService
}

func (c *ServiceContainer) GetAgentMemoryService() *agentmemory.Service {
	if c.agentMemoryService == nil {
		c.agentMemoryService = agentmemory.NewService(c.db)
	}
	return c.agentMemoryService
}

func (c *ServiceContainer) GetConsoleProvider() console.ConsoleProvider {
	if c.platformContainer != nil {
		return c.platformContainer.Console
	}
	return console.NewStandalone()
}

func (c *ServiceContainer) GetPlatformChannels() (platform.Container, error) {
	if c.platformContainer == nil {
		return platform.Container{}, fmt.Errorf("platform container not initialized")
	}
	return *c.platformContainer, nil
}

// GetMemberSubscriptionService returns the member subscription service
func (c *ServiceContainer) GetMemberSubscriptionService() pluginrunner_service.MemberSubscriptionService {
	if c.memberSubscriptionService == nil {
		subRepo := pluginrunner_repo.NewMemberSubscriptionRepository(c.db)
		installRepo := pluginrunner_repo.NewAccountInstallationRepository(c.db)
		infoRepo := pluginrunner_repo.NewInstalledPluginInfoRepository(c.db)
		c.memberSubscriptionService = pluginrunner_service.NewMemberSubscriptionService(subRepo, installRepo, infoRepo)
	}
	return c.memberSubscriptionService
}

func (c *ServiceContainer) GetDefaultModelService() llmdefaultsvc.DefaultModelService {
	if c.defaultModelService == nil {
		modelModule := llmmodel.NewModule(c.db)
		c.defaultModelModule = llmdefaultmodel.NewModule(c.db, modelModule.AvailableModelsSvc, modelModule.GlobalRepo, modelModule.CustomRepo)
		c.defaultModelService = c.defaultModelModule.Service
	}
	return c.defaultModelService
}

// GetGraphFlowService returns the GraphFlow service
func (c *ServiceContainer) GetGraphFlowService() *graphflow.Service {
	if c.graphFlowService == nil {
		// Initialize dependencies for GraphFlow service
		documentRepo := dataset_repo.NewDocumentRepository(c.db)
		datasetRepo := dataset_repo.NewDatasetRepository(c.db)

		c.graphFlowService = graphflow.NewService(
			c.config,
			c.db,
			documentRepo,
			datasetRepo,
			c.GetLLMClient(),
			c.GetDefaultModelService(),
			c.GetTaskManager(),
		)
	}
	return c.graphFlowService
}

func (c *ServiceContainer) SetAutomationDefinitionService(service automationdefinition.Service) {
	c.automationDefinitionService = service
}

func (c *ServiceContainer) GetAutomationDefinitionService() automationdefinition.Service {
	return c.automationDefinitionService
}

func (c *ServiceContainer) GetNotificationSMSService() notificationsms.Service {
	if c.notificationSMSService == nil {
		cfg := notificationsms.ConfigFromLookup(config.Lookup)
		c.notificationSMSService = notificationsms.NewService(
			cfg,
			notificationsms.NewAliyunProvider(cfg.Aliyun),
			notificationsms.NewChuanglanProvider(cfg.Chuanglan),
		)
	}
	return c.notificationSMSService
}

func (c *ServiceContainer) GetShortLinkService() shortlinkcap.Service {
	if c.shortLinkService == nil {
		c.shortLinkService = shortlinkcap.NewServiceWithDB(c.db)
	}
	return c.shortLinkService
}

func (c *ServiceContainer) SetAutomationWorkflowRunner(runner automationaction.AutomationWorkflowRunner) {
	c.automationWorkflowRunner = runner
}

func (c *ServiceContainer) GetAutomationWorkflowRunner() automationaction.AutomationWorkflowRunner {
	return c.automationWorkflowRunner
}

func (c *ServiceContainer) SetWorkflowEngineFactory(factory *graph_engine.EngineFactory) {
	c.workflowEngineFactory = factory
}

func (c *ServiceContainer) GetWorkflowEngineFactory() *graph_engine.EngineFactory {
	if c.workflowEngineFactory == nil {
		nodeRunner := workflowruntime.NewNodeRunner(workflowruntime.Dependencies{
			ContentExtractor:            c.GetContentExtractor(),
			LLMClient:                   c.GetLLMClient(),
			ToolEngine:                  c.GetToolEngine(),
			GraphFlowService:            c.GetGraphFlowService(),
			FileService:                 c.GetFileService(),
			PromptResolver:              c.GetPromptService(),
			AutomationDefinitionService: c.GetAutomationDefinitionService(),
			NotificationSMSService:      c.GetNotificationSMSService(),
		})
		c.workflowEngineFactory = graph_engine.NewEngineFactory(10, nodeRunner)
	}
	return c.workflowEngineFactory
}

func (c *ServiceContainer) GetWorkflowDiagnoser() *diagnosis.Diagnoser {
	if c.workflowDiagnoser == nil {
		c.workflowDiagnoser = diagnosis.NewDiagnoser(context.Background(), c.GetLLMClient())
	}
	return c.workflowDiagnoser
}
