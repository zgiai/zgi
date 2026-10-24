package v1

import (
	"context"
	"fmt"
	"log"

	"github.com/gin-gonic/gin"
	"github.com/zgiai/zgi/api/config"
	"github.com/zgiai/zgi/api/internal/capabilities/artifacts"
	"github.com/zgiai/zgi/api/internal/container"
	approvalruntime "github.com/zgiai/zgi/api/internal/modules/app/workflow/approval"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/graph_engine"
	automationaction "github.com/zgiai/zgi/api/internal/modules/automation/service/action"
	"github.com/zgiai/zgi/api/internal/modules/digitalhuman"
	"github.com/zgiai/zgi/api/internal/modules/inbox"
	system_service "github.com/zgiai/zgi/api/internal/modules/system/service"
	agentmanagement_tools "github.com/zgiai/zgi/api/internal/modules/tools/builtin/agentmanagement"
	workspace_service "github.com/zgiai/zgi/api/internal/modules/workspace/service"
	"github.com/zgiai/zgi/api/internal/modules/zgimarket"
	appcatalog "github.com/zgiai/zgi/api/pkg/apperror/catalog"
	apptransport "github.com/zgiai/zgi/api/pkg/apperror/transport"
	"github.com/zgiai/zgi/api/pkg/database"
	"github.com/zgiai/zgi/api/pkg/storage"
)

// RegisterRoutes registers all v1 version routes
func RegisterRoutes(engine *gin.Engine, v1 *gin.RouterGroup, serviceContainer *container.ServiceContainer, workflowEngineFactory *graph_engine.EngineFactory, applicationErrorCatalog *appcatalog.Catalog) *DigitalHumanRuntimeBundle {
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
	var digitalHumanRuntime *DigitalHumanRuntimeBundle

	// ---------- User / Auth domain ----------
	RegisterUserRoutes(v1, UserRouteDeps{
		DB:                         db,
		AccountService:             serviceContainer.GetAccountService(),
		PhoneAuthAccounts:          serviceContainer.GetAccountServiceImpl(),
		NotificationSMSService:     serviceContainer.GetNotificationSMSService(),
		WorkspaceManagementService: serviceContainer.GetTenantService(),
		OrganizationService:        serviceContainer.GetOrganizationService(),
		DepartmentService:          serviceContainer.GetDepartmentService(),
		ConsoleWebURL:              config.GlobalConfig.Email.ConsoleWebURL,
		ApplicationErrorCatalog:    applicationErrorCatalog,
	})

	// ---------- Workspace / Tenant ----------
	RegisterWorkspaceRoutes(v1, WorkspaceRouteDeps{
		DB:                               db,
		AccountService:                   accountService,
		OrganizationService:              serviceContainer.GetOrganizationService(),
		AuthorizationService:             serviceContainer.GetAuthorizationService(),
		WorkspacePermissionFilterService: serviceContainer.GetWorkspacePermissionFilterService(),
		DepartmentService:                serviceContainer.GetDepartmentService(),
		ConsoleWebURL:                    config.GlobalConfig.Email.ConsoleWebURL,
	})
	RegisterRuntimeWorkspaceManagementRoutes(v1, db, accountService, serviceContainer.GetTenantService(), serviceContainer.GetDigitalHumanCredentialCipher())

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
	})

	// ---------- Memory (common) ----------
	RegisterMemoryRoutes(v1, MemoryRouteDeps{
		MemoryService:  serviceContainer.GetMemoryService(),
		AccountService: accountService,
	})
	RegisterRelationshipMemoryRoutes(v1, db, accountService)

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
		GraphLifecycleService:      serviceContainer.GetGraphLifecycleService(),
		GraphRuntimeHealthService:  serviceContainer.GetGraphRuntimeHealthService(),
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
		SchemaIndexes:           serviceContainer.GetDatabaseSchemaIndexes(),
		ApplicationErrorCatalog: applicationErrorCatalog,
		DataSourceService:       serviceContainer.GetDataSourceService(),
		AccountService:          serviceContainer.GetAccountService(),
		OrganizationService:     serviceContainer.GetOrganizationService(),
	})

	// ---------- Automation ----------
	automationBundle := RegisterAutomationRoutes(v1, AutomationRouteDeps{
		DB:                               db,
		TaskManager:                      serviceContainer.GetTaskManager(),
		TaskHandlerRegistry:              serviceContainer.GetTaskHandlerRegistry(),
		Scheduler:                        serviceContainer.GetScheduler(),
		NotificationSMSService:           serviceContainer.GetNotificationSMSService(),
		AutomationWorkflowRunnerProvider: serviceContainer.GetAutomationWorkflowRunner,
		DigitalHumanRunnerProvider: func() automationaction.AutomationDigitalHumanRunner {
			if digitalHumanRuntime == nil || digitalHumanRuntime.Runtime == nil {
				return nil
			}
			return &digitalHumanAutomationRunner{runtime: digitalHumanRuntime.Runtime, db: db}
		},
		AccountService:             accountService,
		OrganizationService:        serviceContainer.GetOrganizationService(),
		WorkspaceManagementService: tenantService,
		LLMClient:                  serviceContainer.GetLLMClient(),
		DefaultModelService:        serviceContainer.GetDefaultModelService(),
	})
	automationDefinitionService := automationBundle.Definition
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
	approvalResumeHosts := approvalruntime.NewResumeHosts()
	workflowService := RegisterWorkflowRoutes(v1, WorkflowRouteDeps{
		DB:                          db,
		AccountService:              accountService,
		FileService:                 serviceContainer.GetFileService(),
		ContentExtractor:            serviceContainer.GetContentExtractor(),
		QuotaService:                serviceContainer.GetQuotaService(),
		OrganizationService:         serviceContainer.GetOrganizationService(),
		LLMClient:                   serviceContainer.GetLLMClient(),
		ToolEngine:                  serviceContainer.GetToolEngine(),
		ConnectorCatalog:            serviceContainer.GetConnectorPluginManager(),
		GraphFlowService:            serviceContainer.GetGraphFlowService(),
		PromptResolver:              serviceContainer.GetPromptService(),
		AutomationDefinitionService: automationDefinitionService,
		TaskManager:                 serviceContainer.GetTaskManager(),
		TaskRegistry:                serviceContainer.GetTaskHandlerRegistry(),
		Scheduler:                   serviceContainer.GetScheduler(),
		EngineFactory:               workflowEngineFactory,
		AutomationRunnerSetter:      serviceContainer,
		ShortLinkService:            serviceContainer.GetShortLinkService(),
		ApprovalResumeHosts:         approvalResumeHosts,
	})

	if err := serviceContainer.EnsureConnectorDefinitions(context.Background()); err != nil {
		panic(fmt.Sprintf("admit Connector definitions: %v", err))
	}
	RegisterConnectorRoutes(v1, ConnectorRouteDeps{
		Catalog:                 serviceContainer.GetConnectorPluginManager(),
		Connections:             serviceContainer.GetConnectorConnectionService(),
		Bindings:                serviceContainer.GetConnectorBindingService(),
		References:              serviceContainer.GetConnectorReferenceService(),
		Maintenance:             serviceContainer.GetConnectorMaintenanceService(),
		Onboarding:              serviceContainer.GetConnectorOnboardingService(),
		HostKeyProbe:            serviceContainer.GetServerOpsSSHHostKeyProbe(),
		Preferences:             serviceContainer.GetConnectorChatPreferenceService(),
		ApplicationErrorCatalog: applicationErrorCatalog,
		AccountService:          accountService,
	})

	// ---------- Agent ----------
	resourcePermissionService := serviceContainer.GetResourcePermissionService()
	// ZGI Market tools are shared before any chat runtime is built.
	zgiMarketTools := NewZGIMarketTools(db, serviceContainer.GetConnectorCredentialSealer(), serviceContainer.GetToolManager())
	// An agent or a digital human that is deleted gives up its market binding. Registered before
	// their routes, the middleware sees both deletes.
	v1.Use(zgimarket.ReleaseOnDelete(zgiMarketTools))
	agentsService := RegisterAgentsRoutes(v1, db, accountService, tenantService, resourcePermissionService, serviceContainer.GetOrganizationService(), serviceContainer.GetQuotaService(), serviceContainer.GetFileService(), serviceContainer.GetContentExtractor(), serviceContainer.GetLLMClient(), serviceContainer.GetToolEngine(), serviceContainer.GetToolManager(), serviceContainer.GetMemoryService(), serviceContainer.GetGraphFlowService(), serviceContainer.GetPromptService(), serviceContainer.GetDataSourceService(), serviceContainer.GetKnowledgeRetrievalService(), workflowEngineFactory, serviceContainer.GetTaskManager(), serviceContainer.GetTaskHandlerRegistry(), serviceContainer.GetWorkflowTestService(), serviceContainer.GetScheduler(), config.Current().TaskQueue.WorkflowTestTaskBackend, serviceContainer.GetConnectorAgentRuntimeResolver(), serviceContainer.GetConnectorConfirmationContinuationStore(), serviceContainer.GetConnectorSurfaceDispatcher(), serviceContainer.GetConnectorPluginManager(), applicationErrorCatalog)

	// ---------- DigitalHuman ----------
	// Artifacts outlive the runs that produce them (ADR 0009).
	artifactService := artifacts.NewService(db, digitalhuman.ToolFileArtifactBlobs{})
	digitalHumanRuntime = InitializeDigitalHumanRuntime(DigitalHumanRuntimeDeps{
		Artifacts:                           artifactService,
		DB:                                  db,
		LLMClient:                           serviceContainer.GetLLMClient(),
		FileService:                         serviceContainer.GetFileService(),
		ContentExtractor:                    serviceContainer.GetContentExtractor(),
		OrganizationService:                 serviceContainer.GetOrganizationService(),
		ToolEngine:                          serviceContainer.GetToolEngine(),
		ToolManager:                         serviceContainer.GetToolManager(),
		TaskManager:                         serviceContainer.GetTaskManager(),
		TaskRegistry:                        serviceContainer.GetTaskHandlerRegistry(),
		Scheduler:                           serviceContainer.GetScheduler(),
		ManifestResolver:                    nil,
		CredentialCipher:                    serviceContainer.GetDigitalHumanCredentialCipher(),
		ConnectorRuntime:                    serviceContainer.GetConnectorDigitalHumanRuntimeResolver(),
		ConnectorDispatcher:                 serviceContainer.GetConnectorSurfaceDispatcher(),
		ConnectorCatalog:                    serviceContainer.GetConnectorPluginManager(),
		ConnectorPersonalConnectionResolver: serviceContainer.GetConnectorPersonalConnectionResolver(),
		WorkspaceMembership:                 serviceContainer.GetWorkspaceMemberRepository(),
		ConnectorConfirmationStore:          serviceContainer.GetConnectorConfirmationContinuationStore(),
		Tasks:                               automationBundle.DigitalHumanTasks,
		DefaultModelService:                 serviceContainer.GetDefaultModelService(),
		KnowledgeRetrievalService:           serviceContainer.GetKnowledgeRetrievalService(),
		WorkflowService:                     workflowService,
	})
	if digitalHumanRuntime.Runtime != nil {
		approvalResumeHosts.Register(digitalHumanRuntime.Runtime)
	}
	inboxService := inbox.NewService(db)
	notificationChannels, notificationDispatcher := newNotificationChannels(inboxService, db, serviceContainer.GetDigitalHumanCredentialCipher(),
		serviceContainer.GetTaskHandlerRegistry(), serviceContainer.GetTaskManager(), serviceContainer.GetNotificationIMSender())
	if automationBundle.DigitalHumanTasks != nil {
		automationBundle.DigitalHumanTasks.SetNotifyChannels(notificationChannels)
	}
	if digitalHumanRuntime.Runtime != nil {
		digitalHumanRuntime.Runtime.SetTaskApprovalNotifier(digitalHumanApprovalInbox{inbox: inboxService})
	}
	if automationBundle.DigitalHumanTaskReconciler != nil && digitalHumanRuntime.Runtime != nil {
		automationBundle.DigitalHumanTaskReconciler.SetResultDeliverer(digitalHumanRuntime.Runtime)
		automationBundle.DigitalHumanTaskReconciler.SetReportNotifier(reportNotifiers{
			digitalHumanReportInbox{inbox: inboxService},
			digitalhuman.NewArtifactReportCapture(digitalHumanRuntime.Runtime, artifactService),
		})
	}
	inboxErrorProjector, err := apptransport.NewProjector(applicationErrorCatalog)
	if err != nil {
		panic("inbox routes require application error catalog")
	}
	channelHandler := inbox.NewChannelHandler(notificationChannels, notificationDispatcher, inboxErrorProjector)
	RegisterInboxRoutes(v1, InboxRouteDeps{Service: inboxService, Channels: channelHandler, AccountService: accountService, ApplicationErrorCatalog: applicationErrorCatalog})
	digitalHumanRuntime.Access = digitalhuman.NewAccessService(db)
	serviceContainer.SetDigitalHumanChannelReceiverManager(digitalHumanRuntime.Receivers)
	consoleDigitalHumanService := serviceContainer.GetDigitalHumanService()
	// Draft saves and publication admit Connector bindings against the same
	// plugin catalog the runtime uses.
	consoleDigitalHumanService.SetConnectorCatalog(serviceContainer.GetConnectorPluginManager())
	if digitalHumanRuntime.MemoryProjectionRepairer != nil {
		consoleDigitalHumanService.SetMemoryProjectionRepairer(digitalHumanRuntime.MemoryProjectionRepairer)
	}
	RegisterDigitalHumanRoutes(v1, DigitalHumanRouteDeps{
		Service: consoleDigitalHumanService, Runtime: digitalHumanRuntime.Runtime, Channels: digitalHumanRuntime.Channels,
		Tasks: automationBundle.DigitalHumanTasks, Access: digitalHumanRuntime.Access, Artifacts: artifactService,
		AccountService: accountService, OrganizationService: serviceContainer.GetOrganizationService(),
		ApplicationErrorCatalog: applicationErrorCatalog,
		PendingTaskChanges:      digitalhuman.NewRepository(db),
		NotificationChannels:    channelHandler,
	})

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
		ApplicationErrorCatalog:    applicationErrorCatalog,
	})
	if llmModule != nil && llmModule.LLMModelModule != nil {
		if err := serviceContainer.GetToolManager().RegisterProvider(agentmanagement_tools.NewProvider(
			agentsService,
			serviceContainer.GetOrganizationService(),
			llmModule.LLMModelModule.AvailableModelsSvc,
			agentmanagement_tools.WithManagedFileService(serviceContainer.GetFileService()),
		)); err != nil {
			log.Printf("failed to register agent management tools: %v", err)
		}
	} else if err := serviceContainer.GetToolManager().RegisterProvider(agentmanagement_tools.NewProvider(
		agentsService,
		serviceContainer.GetOrganizationService(),
		nil,
		agentmanagement_tools.WithManagedFileService(serviceContainer.GetFileService()),
	)); err != nil {
		log.Printf("failed to register agent management tools: %v", err)
	}

	// ---------- AIChat ----------
	chatRouteDeps := AIChatRouteDeps{
		DB:                         db,
		LLMClient:                  serviceContainer.GetLLMClient(),
		DefaultModelService:        serviceContainer.GetDefaultModelService(),
		FileService:                serviceContainer.GetFileService(),
		ContentExtractor:           serviceContainer.GetContentExtractor(),
		WorkspacePermissionService: serviceContainer.GetOrganizationService(),
		MemoryService:              serviceContainer.GetMemoryService(),
		AgentMemoryService:         serviceContainer.GetAgentMemoryService(),
		SkillRuntime: newSkillRuntimeWithSandbox(
			serviceContainer.GetToolEngine(),
			serviceContainer.GetToolManager(),
			serviceContainer.GetFileService(),
			serviceContainer.GetOrganizationService(),
		).WithConnectorDispatcher(serviceContainer.GetConnectorSurfaceDispatcher()),
		AccountService:             accountService,
		ConnectorChatRuntime:       serviceContainer.GetConnectorChatRuntimeResolver(),
		ConnectorAgentRuntime:      serviceContainer.GetConnectorAgentRuntimeResolver(),
		ConnectorConfirmationStore: serviceContainer.GetConnectorConfirmationContinuationStore(),
		ApplicationErrorCatalog:    applicationErrorCatalog,
	}
	chatService := RegisterAIChatRoutes(v1, chatRouteDeps)

	// ---------- ZGI Market (members of the current organization) ----------
	RegisterZGIMarketRoutes(v1, accountService, tenantServiceImpl, chatService, zgiMarketTools)
	RegisterConnectorRuntimeConfirmationRoutes(v1, ConnectorRuntimeConfirmationRouteDeps{
		Service: serviceContainer.GetConnectorRuntimeConfirmationService(),
		Recorder: connectorRuntimeConfirmationSurfaceRecorder{
			chatAndAgent: chatService,
			workflow:     workflowService,
		},
		AccountService: accountService,
	})
	// End users connect their own keys for per-user DigitalHuman Connector
	// slots through the same verified key setup as members (ADR 0007).
	if connectionService := serviceContainer.GetConnectorConnectionService(); connectionService != nil {
		digitalhuman.UseEndUserKeySetup(connectionService)
	}
	// Channel replies decide connector confirmations through the same path.
	if digitalHumanRuntime != nil && digitalHumanRuntime.Runtime != nil {
		digitalHumanRuntime.Runtime.SetChannelConnectorConfirmer(digitalHumanChannelConnectorConfirmer{
			service:  serviceContainer.GetConnectorRuntimeConfirmationService(),
			recorder: connectorRuntimeConfirmationSurfaceRecorder{chatAndAgent: chatService, workflow: workflowService},
		})
	}
	if llmModule != nil && llmModule.LLMModelModule != nil {
		RegisterImageRuntimeRoutes(v1, ImageRuntimeRouteDeps{
			DB:                      db,
			AvailableModels:         llmModule.LLMModelModule.AvailableModelsSvc,
			Routes:                  llmModule.ChannelSvc,
			LLMClient:               serviceContainer.GetLLMClient(),
			ChatService:             chatService,
			AccountService:          accountService,
			FileService:             serviceContainer.GetFileService(),
			ApplicationErrorCatalog: applicationErrorCatalog,
		})
		RegisterVideoRuntimeRoutes(v1, VideoRuntimeRouteDeps{
			DB:                      db,
			AvailableModels:         llmModule.LLMModelModule.AvailableModelsSvc,
			LLMClient:               serviceContainer.GetLLMClient(),
			AccountService:          accountService,
			ApplicationErrorCatalog: applicationErrorCatalog,
		})
		RegisterMusicRoutes(v1, MusicRouteDeps{
			DB:                      db,
			AvailableModels:         llmModule.LLMModelModule.AvailableModelsSvc,
			Generator:               serviceContainer.GetMusicGenerator(),
			LyricsGenerator:         serviceContainer.GetMusicLyricsGenerator(),
			Compensator:             serviceContainer.GetMusicDeliveryCompensator(),
			AccountService:          accountService,
			TaskManager:             serviceContainer.GetTaskManager(),
			TaskRegistry:            serviceContainer.GetTaskHandlerRegistry(),
			Scheduler:               serviceContainer.GetScheduler(),
			ApplicationErrorCatalog: applicationErrorCatalog,
		})
	}

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
	return digitalHumanRuntime
}
