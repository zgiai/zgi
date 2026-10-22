package v1

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/config"
	executionruntime "github.com/zgiai/zgi/api/internal/capabilities/agentexecution/runtime"
	"github.com/zgiai/zgi/api/internal/capabilities/artifacts"
	"github.com/zgiai/zgi/api/internal/capabilities/channelruntime"
	channelproviders "github.com/zgiai/zgi/api/internal/capabilities/channelruntime/providers"
	runtimerepo "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/repository"
	runtimeservice "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/service"
	credentialcrypto "github.com/zgiai/zgi/api/internal/infra/credentialcrypto"
	workflowapp "github.com/zgiai/zgi/api/internal/modules/app/workflow"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/tool_file"
	connectordispatch "github.com/zgiai/zgi/api/internal/modules/connectors/dispatch"
	connectorexecution "github.com/zgiai/zgi/api/internal/modules/connectors/execution"
	connectorruntimeauth "github.com/zgiai/zgi/api/internal/modules/connectors/runtimeauth"
	datasetservice "github.com/zgiai/zgi/api/internal/modules/dataset/service"
	digitalhuman "github.com/zgiai/zgi/api/internal/modules/digitalhuman"
	"github.com/zgiai/zgi/api/internal/modules/inbox"
	llmclient "github.com/zgiai/zgi/api/internal/modules/llm/client"
	llmdefaultservice "github.com/zgiai/zgi/api/internal/modules/llm/defaultmodel/service"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	"github.com/zgiai/zgi/api/internal/modules/skills"
	"github.com/zgiai/zgi/api/internal/modules/tools"
	workspacemodel "github.com/zgiai/zgi/api/internal/modules/workspace/model"
	"github.com/zgiai/zgi/api/internal/modules/zgimarket"
	"github.com/zgiai/zgi/api/internal/util"
	"github.com/zgiai/zgi/api/middleware"
	"github.com/zgiai/zgi/api/pkg/apperror"
	appcatalog "github.com/zgiai/zgi/api/pkg/apperror/catalog"
	apptransport "github.com/zgiai/zgi/api/pkg/apperror/transport"
	"github.com/zgiai/zgi/api/pkg/logger"
	"github.com/zgiai/zgi/api/pkg/queue"
	pkgscheduler "github.com/zgiai/zgi/api/pkg/scheduler"
	"github.com/zgiai/zgi/api/pkg/vectordb"
	"gorm.io/gorm"
)

// DigitalHumanRouteDeps contains the lifecycle dependencies.
type DigitalHumanRouteDeps struct {
	Service                 digitalhuman.HandlerService
	Runtime                 digitalhuman.RuntimeHandlerService
	Channels                digitalhuman.ChannelHandlerService
	Tasks                   digitalhuman.TaskHandlerService
	Access                  *digitalhuman.AccessService
	AccountService          interfaces.AccountService
	OrganizationService     interfaces.OrganizationService
	ApplicationErrorCatalog *appcatalog.Catalog
	// NotificationChannels serves the webhook a DigitalHuman's API
	// integrations receive task notifications on (ADR 0008).
	NotificationChannels *inbox.ChannelHandler
	// PendingTaskChanges is optional. It marks personal-view tasks whose
	// proposed change waits for the reader's approval.
	PendingTaskChanges digitalhuman.TaskPendingChangeSource
	// Artifacts serves the DigitalHuman's artifact center (ADR 0009).
	Artifacts *artifacts.Service
}

// DigitalHumanRuntimeDeps contains the shared bottom-layer runtime and durable
// worker dependencies.
type DigitalHumanRuntimeDeps struct {
	DB                                  *gorm.DB
	LLMClient                           llmclient.LLMClient
	DefaultModelService                 llmdefaultservice.DefaultModelService
	KnowledgeRetrievalService           *datasetservice.KnowledgeRetrievalService
	FileService                         interfaces.FileService
	ContentExtractor                    runtimeservice.ContentExtractionService
	OrganizationService                 interfaces.OrganizationService
	ToolEngine                          *tools.ToolEngine
	ToolManager                         *tools.ToolManager
	TaskManager                         *queue.TaskManager
	TaskRegistry                        digitalhuman.RunTaskRegistry
	Scheduler                           *pkgscheduler.Scheduler
	ManifestResolver                    skills.ToolGovernanceManifestResolver
	CredentialCipher                    credentialcrypto.CredentialCipher
	ConnectorRuntime                    connectorruntimeauth.DigitalHumanRuntimeResolver
	ConnectorDispatcher                 connectordispatch.Dispatcher
	ConnectorCatalog                    digitalhuman.ConnectorCatalog
	ConnectorPersonalConnectionResolver digitalhuman.PersonalConnectionCredentialResolver
	WorkspaceMembership                 digitalhuman.WorkspaceMembershipChecker
	ConnectorConfirmationStore          connectorexecution.ConfirmationContinuationStore
	Tasks                               digitalhuman.TaskHandlerService
	MemoryRetriever                     digitalhuman.DigitalHumanMemoryRetriever
	WorkflowService                     interfaces.WorkflowService
	// Artifacts gives runs the artifact tools when it is set.
	Artifacts *artifacts.Service
}

// DigitalHumanRuntimeBundle keeps the model runtime and channel application
// service on the same repository and invocation pipeline.
type DigitalHumanRuntimeBundle struct {
	Runtime *digitalhuman.RuntimeService
	Access  *digitalhuman.AccessService
	Tasks   digitalhuman.TaskHandlerService
	// The HTTP memory surfaces run on the container-owned service, so the
	// projection repairer has to travel out of the runtime wiring to reach it.
	MemoryProjectionRepairer digitalhuman.DigitalHumanMemoryProjectionRepairer
	Channels                 *digitalhuman.ChannelService
	Receivers                *digitalhuman.ChannelReceiverManager
}

// RegisterDigitalHumanRoutes exposes the DigitalHuman module.
func RegisterDigitalHumanRoutes(router *gin.RouterGroup, deps DigitalHumanRouteDeps) {
	if router == nil || deps.Service == nil || deps.Runtime == nil || deps.Channels == nil || deps.Tasks == nil || deps.Access == nil || deps.AccountService == nil || deps.OrganizationService == nil {
		panic("digital human routes require lifecycle, runtime, channel, account, and permission services")
	}
	errorProjector, err := apptransport.NewProjector(deps.ApplicationErrorCatalog)
	if err != nil {
		panic("digital human routes require application error catalog")
	}
	channelHandler := digitalhuman.NewChannelHandler(deps.Channels, errorProjector)
	channelHandler.RegisterPublicRoutes(router)
	group := router.Group("")
	group.Use(middleware.SetupRequired())
	group.Use(middleware.JWTWithOrganizationAndService(deps.AccountService))
	group.Use(middleware.CurrentWorkspaceRequired())
	group.Use(digitalHumanPermissionMiddleware(deps.OrganizationService, errorProjector))
	digitalhuman.NewHandler(deps.Service, errorProjector).RegisterRoutes(group)
	digitalhuman.NewAccessHandler(deps.Access, errorProjector).RegisterRoutes(group)
	digitalhuman.NewRuntimeHandler(deps.Runtime, errorProjector, deps.Service).RegisterRoutes(group)
	// The runtime serves the files a task report carries when it can.
	reportFiles, _ := deps.Runtime.(digitalhuman.TaskReportFileSource)
	taskHandler := digitalhuman.NewTaskHandler(deps.Tasks, errorProjector).WithPendingChanges(deps.PendingTaskChanges).WithReportFiles(reportFiles)
	taskHandler.RegisterRoutes(group)
	taskHandler.RegisterReportRoutes(group)
	if deps.Artifacts != nil {
		digitalhuman.NewArtifactHandler(deps.Artifacts, errorProjector).RegisterRoutes(group)
	}
	if deps.NotificationChannels != nil {
		deps.NotificationChannels.RegisterRoutes(group, "/digital-humans/:id/notification-channels", digitalHumanChannelOwner(deps.Service, errorProjector))
		deps.NotificationChannels.RegisterChoiceRoute(group, "/team-notification-channels/options", workspaceChannelOwner(errorProjector))
		deps.NotificationChannels.RegisterRoutes(group, "/team-notification-channels", workspaceChannelOwner(errorProjector))
	}
	channelHandler.RegisterAuthenticatedRoutes(group)
}

func digitalHumanPermissionMiddleware(organizationService interfaces.OrganizationService, projector *apptransport.Projector) gin.HandlerFunc {
	return func(c *gin.Context) {
		permission := digitalHumanPermissionForRequest(c.Request.Method, c.FullPath())
		if strings.Contains(c.FullPath(), "/my-tasks") {
			permission = workspacemodel.WorkspacePermissionDigitalHumanRun
		} else if strings.Contains(c.FullPath(), "/tasks/:task_id/actions/:action") {
			switch digitalhuman.TaskAction(strings.TrimSpace(c.Param("action"))) {
			case digitalhuman.TaskActionRunNow, digitalhuman.TaskActionRetry, digitalhuman.TaskActionCancel:
				permission = workspacemodel.WorkspacePermissionDigitalHumanRun
			default:
				permission = workspacemodel.WorkspacePermissionDigitalHumanUpdate
			}
		}
		if permission == "" {
			writeDigitalHumanRouteError(c, projector, apperror.New(appcatalog.CodePermissionDenied, apperror.WithOperation("digital_human.permission.route")))
			return
		}
		allowed, err := organizationService.CheckWorkspacePermission(
			c.Request.Context(), util.GetOrganizationID(c), util.GetWorkspaceID(c), util.GetAccountID(c), permission,
		)
		if err != nil {
			writeDigitalHumanRouteError(c, projector, apperror.Wrap(err, appcatalog.CodeInternal, apperror.WithOperation("digital_human.permission.check")))
			return
		}
		if !allowed {
			writeDigitalHumanRouteError(c, projector, apperror.New(appcatalog.CodePermissionDenied, apperror.WithOperation("digital_human.permission.check")))
			return
		}
		if (isDigitalHumanTaskCenterPath(c.FullPath()) && digitalhuman.TaskVisibilityEnabled()) || strings.Contains(c.FullPath(), "/digital-humans/:id/artifacts") {
			manager := permission == workspacemodel.WorkspacePermissionDigitalHumanUpdate
			if !manager {
				manager, err = organizationService.CheckWorkspacePermission(
					c.Request.Context(), util.GetOrganizationID(c), util.GetWorkspaceID(c), util.GetAccountID(c), workspacemodel.WorkspacePermissionDigitalHumanUpdate,
				)
				if err != nil {
					writeDigitalHumanRouteError(c, projector, apperror.Wrap(err, appcatalog.CodeInternal, apperror.WithOperation("digital_human.permission.check")))
					return
				}
			}
			c.Set(digitalhuman.TaskManagerContextKey, manager)
		}
		c.Next()
	}
}

// isDigitalHumanTaskCenterPath matches the shared task center routes, not the
// personal my-tasks view.
func isDigitalHumanTaskCenterPath(routePath string) bool {
	return strings.Contains(routePath, "/digital-humans/:id/tasks") || strings.Contains(routePath, "/digital-humans/:id/task-reports")
}

func digitalHumanPermissionForRequest(method, routePath string) workspacemodel.WorkspacePermissionCode {
	path := strings.TrimSpace(routePath)
	if path == "" {
		return ""
	}
	// Previewing the draft is part of editing it.
	if strings.Contains(path, "/digital-humans/:id/preview/") {
		return workspacemodel.WorkspacePermissionDigitalHumanUpdate
	}
	// Creation needs the workspace's bindable skills and owner Connector
	// connections before the draft exists, so these routes are owned by the
	// create permission rather than the view one.
	if strings.Contains(path, "digital-human-skill-candidates") || strings.Contains(path, "digital-human-connector-candidates") {
		if method == http.MethodGet {
			return workspacemodel.WorkspacePermissionDigitalHumanCreate
		}
		return ""
	}
	// Private tasks are part of the formal run surface. A user who can run a
	// DigitalHuman may manage only their own task namespace from chat; viewers
	// must not discover it through a direct URL.
	if strings.Contains(path, "/my-tasks") || strings.Contains(path, "/my-task-reports") {
		return workspacemodel.WorkspacePermissionDigitalHumanRun
	}
	// Task reports are read with view permission. Reviewing one may rerun its
	// task, so it needs run permission; the task service then requires the
	// reader to be the task's requester or a task center manager.
	if strings.Contains(path, "/task-reports") {
		switch method {
		case http.MethodGet:
			return workspacemodel.WorkspacePermissionDigitalHumanView
		case http.MethodPost:
			return workspacemodel.WorkspacePermissionDigitalHumanRun
		default:
			return ""
		}
	}
	if strings.Contains(path, "/tasks") || strings.Contains(path, "/my-tasks") {
		switch method {
		case http.MethodGet:
			return workspacemodel.WorkspacePermissionDigitalHumanView
		case http.MethodPost, http.MethodPatch, http.MethodPut:
			return workspacemodel.WorkspacePermissionDigitalHumanUpdate
		default:
			return ""
		}
	}
	if strings.Contains(path, "/runs") {
		if method == http.MethodGet {
			return workspacemodel.WorkspacePermissionDigitalHumanLogsView
		}
		if method == http.MethodPost && strings.HasSuffix(path, "/cancel") {
			return workspacemodel.WorkspacePermissionDigitalHumanRun
		}
		return ""
	}
	// Artifacts are read with view permission; the artifact center decides
	// which ones the reader sees. Archiving one needs run permission, and the
	// artifact center then requires the reader to manage it.
	if strings.Contains(path, "/digital-humans/:id/artifacts") {
		switch method {
		case http.MethodGet:
			return workspacemodel.WorkspacePermissionDigitalHumanView
		case http.MethodPost:
			if strings.HasSuffix(path, "/archive") || strings.HasSuffix(path, "/restore") {
				return workspacemodel.WorkspacePermissionDigitalHumanRun
			}
			return ""
		default:
			return ""
		}
	}
	// Team channels receive the reports of tasks shared with the workspace.
	// Task center managers keep the list; anyone who can run a DigitalHuman
	// may see the choices for their own task.
	if strings.Contains(path, "/team-notification-channels") {
		if strings.HasSuffix(path, "/options") {
			if method == http.MethodGet {
				return workspacemodel.WorkspacePermissionDigitalHumanRun
			}
			return ""
		}
		return workspacemodel.WorkspacePermissionDigitalHumanUpdate
	}
	if strings.Contains(path, "/access") || strings.Contains(path, "/api-keys") || strings.Contains(path, "/notification-channels") {
		return workspacemodel.WorkspacePermissionDigitalHumanPublish
	}
	if strings.Contains(path, "/memory") {
		if method == http.MethodGet {
			return workspacemodel.WorkspacePermissionDigitalHumanMemoryView
		}
		if method == http.MethodPost || method == http.MethodPatch || method == http.MethodDelete {
			return workspacemodel.WorkspacePermissionDigitalHumanMemoryManage
		}
		return ""
	}
	if strings.Contains(path, "/channels/") {
		if strings.Contains(path, "/discoveries") {
			if method == http.MethodGet {
				return workspacemodel.WorkspacePermissionDigitalHumanUpdate
			}
			return ""
		}
		if strings.Contains(path, "/events") || strings.Contains(path, "/deliveries") {
			if method == http.MethodGet {
				return workspacemodel.WorkspacePermissionDigitalHumanLogsView
			}
			if method == http.MethodPost && (strings.HasSuffix(path, "/retry") || strings.HasSuffix(path, "/dismiss")) {
				return workspacemodel.WorkspacePermissionDigitalHumanRun
			}
			return ""
		}
		switch method {
		case http.MethodGet:
			return workspacemodel.WorkspacePermissionDigitalHumanView
		case http.MethodPost, http.MethodPatch, http.MethodPut, http.MethodDelete:
			return workspacemodel.WorkspacePermissionDigitalHumanUpdate
		default:
			return ""
		}
	}
	if strings.Contains(path, "/runtime/") {
		switch method {
		case http.MethodGet, http.MethodPost, http.MethodPatch, http.MethodDelete:
			return workspacemodel.WorkspacePermissionDigitalHumanRun
		default:
			return ""
		}
	}
	if method == http.MethodPost && strings.HasSuffix(path, "/invocations") {
		return workspacemodel.WorkspacePermissionDigitalHumanRun
	}
	if method == http.MethodPost && strings.HasSuffix(path, "/publish") {
		return workspacemodel.WorkspacePermissionDigitalHumanPublish
	}
	if strings.Contains(path, "/versions") {
		switch method {
		case http.MethodGet:
			return workspacemodel.WorkspacePermissionDigitalHumanView
		case http.MethodPost:
			return workspacemodel.WorkspacePermissionDigitalHumanPublish
		default:
			return ""
		}
	}
	switch method {
	case http.MethodGet:
		return workspacemodel.WorkspacePermissionDigitalHumanView
	case http.MethodPost:
		return workspacemodel.WorkspacePermissionDigitalHumanCreate
	case http.MethodPatch, http.MethodPut:
		return workspacemodel.WorkspacePermissionDigitalHumanUpdate
	case http.MethodDelete:
		return workspacemodel.WorkspacePermissionDigitalHumanDelete
	default:
		return ""
	}
}

func writeDigitalHumanRouteError(c *gin.Context, projector *apptransport.Projector, err error) {
	presentation := projector.Project(err, apptransport.LocaleFromAcceptLanguage(c.GetHeader("Accept-Language"))).Presentation
	c.AbortWithStatusJSON(presentation.HTTPStatus, gin.H{
		"code": presentation.Code.String(), "message": presentation.Message, "retryable": presentation.Retryable,
	})
}

// validateDigitalHumanConnectorDependencies enforces the complete Connector
// boundary before the runtime touches any database, queue, or provider, so a
// missing dependency fails fast instead of degrading to an unauthenticated run.
func validateDigitalHumanConnectorDependencies(deps DigitalHumanRuntimeDeps) {
	if deps.ConnectorDispatcher == nil || deps.ConnectorRuntime == nil || deps.ConnectorCatalog == nil ||
		deps.ConnectorConfirmationStore == nil {
		panic("DigitalHuman runtime requires complete Connector dependencies")
	}
}

// newDigitalHumanSkillRuntime installs the owner-scoped Connector dispatcher on
// the same skill runtime used for tool governance and sandbox scripts.
func newDigitalHumanSkillRuntime(deps DigitalHumanRuntimeDeps) *skills.Runtime {
	validateDigitalHumanConnectorDependencies(deps)
	return newSkillRuntimeWithSandbox(
		deps.ToolEngine, deps.ToolManager, deps.FileService, deps.OrganizationService, deps.ManifestResolver,
	).WithConnectorDispatcher(deps.ConnectorDispatcher)
}

func InitializeDigitalHumanRuntime(deps DigitalHumanRuntimeDeps) *DigitalHumanRuntimeBundle {
	validateDigitalHumanConnectorDependencies(deps)
	if deps.DB == nil || deps.LLMClient == nil || deps.FileService == nil || deps.ContentExtractor == nil ||
		deps.OrganizationService == nil || deps.ToolEngine == nil || deps.ToolManager == nil || deps.TaskManager == nil ||
		deps.TaskRegistry == nil || deps.Scheduler == nil || deps.CredentialCipher == nil || deps.Tasks == nil {
		panic("DigitalHuman runtime requires database, model, tool, queue, scheduler, and credential dependencies")
	}
	repository := digitalhuman.NewRepository(deps.DB)
	repository.SetConnectorCatalog(deps.ConnectorCatalog)
	channelAdapters, err := channelruntime.NewRegistry()
	if err != nil {
		panic("initialize DigitalHuman channel registry: " + err.Error())
	}
	channelCredentials := digitalhuman.NewInstallationCredentialResolver(
		repository, deps.CredentialCipher, channelAdapters,
	).WithPersonalConnectionResolver(deps.ConnectorPersonalConnectionResolver, deps.WorkspaceMembership)
	if err := channelproviders.RegisterProduction(channelAdapters, channelCredentials); err != nil {
		panic("register DigitalHuman channel providers: " + err.Error())
	}
	transientDispatcher := digitalhuman.NewAsynqChannelTransientDispatcher(deps.TaskManager)
	runtimeInputDispatcher := digitalhuman.NewRuntimeInputFollowUpDispatcher(deps.TaskManager)
	modelSpecResolver := runtimeservice.NewDatabaseModelSpecResolver(deps.DB)
	chatRuntime := runtimeservice.NewServiceWithSkillRuntime(
		runtimerepo.NewRepositories(deps.DB),
		deps.LLMClient,
		nil,
		modelSpecResolver,
		deps.FileService,
		deps.ContentExtractor,
		deps.OrganizationService,
		newDigitalHumanSkillRuntime(deps),
		nil,
		runtimeInputDispatcher, deps.ConnectorRuntime, deps.ConnectorConfirmationStore,
		zgimarket.RuntimeResolver(),
	)
	dispatcher := digitalhuman.NewAsynqRunDispatcher(deps.TaskManager)
	memoryService := digitalhuman.NewService(repository)
	memoryPromotionAuthorizer := digitalhuman.MemoryPromotionAuthorizerFunc(
		func(ctx context.Context, organizationID, workspaceID, actorID uuid.UUID) (bool, error) {
			return deps.OrganizationService.CheckWorkspacePermission(
				ctx, organizationID.String(), workspaceID.String(), actorID.String(), workspacemodel.WorkspacePermissionDigitalHumanMemoryManage,
			)
		},
	)
	memoryOptions := []digitalhuman.MemoryRuntimeProviderOption{
		digitalhuman.WithMemoryPromotionAuthorizer(memoryPromotionAuthorizer),
	}
	var memoryRetriever digitalhuman.DigitalHumanMemoryRetriever
	if deps.MemoryRetriever != nil {
		memoryRetriever = deps.MemoryRetriever
	} else {
		vectorStore, err := vectordb.NewVectorDB(&config.Current().VectorStore)
		if err != nil {
			panic("initialize DigitalHuman memory vector store: " + err.Error())
		}
		knowledgeService := deps.KnowledgeRetrievalService
		if knowledgeService == nil {
			knowledgeService = datasetservice.NewKnowledgeRetrievalService(deps.DB, config.Current(), deps.LLMClient, deps.DefaultModelService, nil)
		}
		memoryRetriever = digitalhuman.NewDigitalHumanMemoryProjectionProvider(
			deps.DB, knowledgeService, deps.LLMClient, deps.DefaultModelService, vectorStore,
		)
	}
	memoryOptions = append(memoryOptions, digitalhuman.WithDigitalHumanMemoryRetriever(memoryRetriever))
	var memoryProjectionRepairer digitalhuman.DigitalHumanMemoryProjectionRepairer
	if repairer, ok := memoryRetriever.(digitalhuman.DigitalHumanMemoryProjectionRepairer); ok {
		memoryProjectionRepairer = repairer
		memoryService.SetMemoryProjectionRepairer(repairer)
	}
	memoryProvider := digitalhuman.NewMemoryRuntimeProvider(memoryService, memoryOptions...)
	runtime := digitalhuman.NewRuntimeService(
		repository,
		digitalhuman.NewResolver(repository),
		digitalhuman.NewBrainAdapter(repository),
		chatRuntime,
		dispatcher,
		digitalhuman.WithMemoryRuntime(digitalhuman.NewCompositeMemoryRuntimeProvider(
			memoryProvider,
			digitalhuman.NewRelationshipMemoryRuntimeProvider(deps.DB),
		)),
		digitalhuman.WithTaskRuntime(digitalhuman.NewArtifactRuntimeProvider(digitalhuman.NewTaskRuntimeProvider(deps.Tasks), deps.Artifacts)),
		digitalhuman.WithChannelTransientDispatcher(transientDispatcher),
		digitalhuman.WithChannelContextRuntime(digitalhuman.NewChannelContextRuntime(repository, channelAdapters, modelSpecResolver)),
		digitalhuman.WithManualInvocationFileValidator(digitalhuman.NewConsoleFileValidator(deps.FileService)),
		digitalhuman.WithLinuxExecutionRuntime(newDigitalHumanLinuxExecution(deps)),
		digitalhuman.WithSourceTrustAuthorizer(memoryPromotionAuthorizer),
	)
	if deps.WorkflowService != nil {
		runtimeOption := digitalhuman.WithWorkflowContinuationRunner(deps.DB,
			workflowapp.NewWorkflowHandler(deps.WorkflowService, nil, deps.FileService, nil, deps.OrganizationService))
		// The handler only uses its workflow runner methods here; HTTP permission
		// dependencies are not consulted by continuation execution.
		runtimeOption(runtime)
	}
	digitalhuman.RegisterRunTaskHandler(deps.TaskRegistry, deps.TaskManager, runtime)
	digitalhuman.RegisterRuntimeInputFollowUpTaskHandler(deps.TaskRegistry, deps.TaskManager, runtime)
	if err := deps.Scheduler.RegisterTask(&digitalhuman.ReconcileTask{}, digitalhuman.NewReconcileHandler(runtime)); err != nil {
		panic("register DigitalHuman run reconciler: " + err.Error())
	}
	if err := deps.Scheduler.RegisterTask(
		&digitalhuman.RuntimeInputRecoveryTask{}, digitalhuman.NewRuntimeInputRecoveryHandler(chatRuntime),
	); err != nil {
		panic("register DigitalHuman runtime input recovery: " + err.Error())
	}
	recoveryContext, cancelRecovery := context.WithTimeout(context.Background(), 30*time.Second)
	if _, err := chatRuntime.RecoverStaleRuntimeInputs(recoveryContext); err != nil {
		logger.Error("Failed to recover stale DigitalHuman runtime inputs during startup", err)
	}
	if _, err := chatRuntime.EnqueuePendingRuntimeInputFollowUps(recoveryContext); err != nil {
		logger.Error("Failed to enqueue pending DigitalHuman runtime input follow-ups during startup", err)
	}
	cancelRecovery()
	memoryRunner := digitalhuman.NewMemoryExtractionRunner(
		repository, memoryService, digitalhuman.NewLLMMemoryExtractor(deps.LLMClient),
	)
	if err := deps.Scheduler.RegisterTask(
		&digitalhuman.MemoryExtractionSweepTask{}, digitalhuman.NewMemoryExtractionSweepHandler(memoryRunner),
	); err != nil {
		panic("register DigitalHuman memory extraction sweep: " + err.Error())
	}
	channels := digitalhuman.NewChannelService(
		repository, repository, runtime, channelAdapters, channelCredentials, transientDispatcher,
	).WithAPIBaseURL(config.Current().Console.APIURL)
	receivers := digitalhuman.NewChannelReceiverManager(repository, channelAdapters, channelCredentials, channels)
	channels.WithReceiverStatus(receivers)
	digitalhuman.RegisterChannelTransientTaskHandler(deps.TaskRegistry, deps.TaskManager, channels)
	if err := deps.Scheduler.RegisterTask(
		&digitalhuman.ChannelDeliverySweepTask{}, digitalhuman.NewChannelDeliverySweepHandler(channels),
	); err != nil {
		panic("register DigitalHuman channel delivery sweep: " + err.Error())
	}
	return &DigitalHumanRuntimeBundle{
		Runtime: runtime, Tasks: deps.Tasks, Channels: channels, Receivers: receivers,
		MemoryProjectionRepairer: memoryProjectionRepairer,
	}
}

func newDigitalHumanLinuxExecution(deps DigitalHumanRuntimeDeps) *digitalhuman.LinuxExecutionRuntime {
	entries, err := executionruntime.EntryAdmissionFromEnv()
	if err != nil {
		logger.Warn("DigitalHuman Linux entry allowlist is invalid; execution remains closed")
	}
	return digitalhuman.NewLinuxExecutionRuntime(digitalhuman.LinuxExecutionOptions{
		DB: deps.DB, Approvals: executionruntime.ApprovalsFromEnv(), Gateway: executionruntime.GatewayFromEnv(),
		PlatformCLI: executionruntime.EnvBool("ZGI_AGENT_RUNTIME_PLATFORM_CLI_ENABLED", false),
		EntryReady:  executionruntime.DigitalHumanEntryImplemented,
		Files:       deps.FileService, Access: digitalhuman.NewAccessService(deps.DB), Entries: entries,
		ArtifactManager: func() *tool_file.ToolFileManager { return tool_file.GlobalToolFileManager },
		Artifacts:       deps.Artifacts,
		AuthorizeConsole: func(ctx context.Context, scope digitalhuman.Scope) error {
			if deps.OrganizationService == nil {
				return digitalhuman.ErrNotFound
			}
			allowed, err := deps.OrganizationService.CheckWorkspacePermission(ctx, scope.OrganizationID.String(), scope.WorkspaceID.String(), scope.ActorID.String(), workspacemodel.WorkspacePermissionDigitalHumanRun)
			if err != nil {
				return err
			}
			if !allowed {
				return digitalhuman.ErrNotFound
			}
			return nil
		},
	})
}

// workspaceChannelOwner resolves the current workspace's team channels.
func workspaceChannelOwner(projector *apptransport.Projector) inbox.OwnerResolver {
	return func(c *gin.Context) (inbox.ChannelOwner, bool) {
		organizationID, orgErr := uuid.Parse(strings.TrimSpace(util.GetOrganizationID(c)))
		workspaceID, workspaceErr := uuid.Parse(strings.TrimSpace(util.GetWorkspaceID(c)))
		if orgErr != nil || workspaceErr != nil {
			writeDigitalHumanRouteError(c, projector, apperror.New(appcatalog.CodeRequestInvalid, apperror.WithOperation("digital_human.notification_channel.workspace")))
			return inbox.ChannelOwner{}, false
		}
		return inbox.ChannelOwner{OrganizationID: organizationID, WorkspaceID: &workspaceID, Kind: inbox.OwnerWorkspace, ID: workspaceID}, true
	}
}

// digitalHumanChannelOwner resolves the DigitalHuman whose webhook channels
// a request manages, and only when it belongs to the current workspace, so a
// webhook can never receive another workspace's end-user notifications.
func digitalHumanChannelOwner(service digitalhuman.HandlerService, projector *apptransport.Projector) inbox.OwnerResolver {
	return func(c *gin.Context) (inbox.ChannelOwner, bool) {
		organizationID, orgErr := uuid.Parse(strings.TrimSpace(util.GetOrganizationID(c)))
		workspaceID, workspaceErr := uuid.Parse(strings.TrimSpace(util.GetWorkspaceID(c)))
		actorID, actorErr := uuid.Parse(strings.TrimSpace(util.GetAccountID(c)))
		digitalHumanID, idErr := uuid.Parse(strings.TrimSpace(c.Param("id")))
		if orgErr != nil || workspaceErr != nil || actorErr != nil || idErr != nil {
			writeDigitalHumanRouteError(c, projector, apperror.New(appcatalog.CodeRequestInvalid, apperror.WithOperation("digital_human.notification_channel.owner")))
			return inbox.ChannelOwner{}, false
		}
		scope := digitalhuman.Scope{OrganizationID: organizationID, WorkspaceID: workspaceID, ActorID: actorID}
		if _, err := service.Get(c.Request.Context(), scope, digitalHumanID); err != nil {
			writeDigitalHumanRouteError(c, projector, apperror.New(appcatalog.CodeResourceNotFound, apperror.WithOperation("digital_human.notification_channel.owner")))
			return inbox.ChannelOwner{}, false
		}
		return inbox.ChannelOwner{OrganizationID: organizationID, WorkspaceID: &workspaceID, Kind: inbox.OwnerDigitalHuman, ID: digitalHumanID}, true
	}
}
