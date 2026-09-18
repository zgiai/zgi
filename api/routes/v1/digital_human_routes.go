package v1

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/config"
	"github.com/zgiai/zgi/api/internal/capabilities/channelruntime"
	channelproviders "github.com/zgiai/zgi/api/internal/capabilities/channelruntime/providers"
	runtimerepo "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/repository"
	runtimeservice "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/service"
	credentialcrypto "github.com/zgiai/zgi/api/internal/infra/credentialcrypto"
	connectordispatch "github.com/zgiai/zgi/api/internal/modules/connectors/dispatch"
	connectorexecution "github.com/zgiai/zgi/api/internal/modules/connectors/execution"
	connectorruntimeauth "github.com/zgiai/zgi/api/internal/modules/connectors/runtimeauth"
	digitalhuman "github.com/zgiai/zgi/api/internal/modules/digitalhuman"
	llmclient "github.com/zgiai/zgi/api/internal/modules/llm/client"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	"github.com/zgiai/zgi/api/internal/modules/skills"
	"github.com/zgiai/zgi/api/internal/modules/tools"
	workspacemodel "github.com/zgiai/zgi/api/internal/modules/workspace/model"
	"github.com/zgiai/zgi/api/internal/util"
	"github.com/zgiai/zgi/api/middleware"
	"github.com/zgiai/zgi/api/pkg/apperror"
	appcatalog "github.com/zgiai/zgi/api/pkg/apperror/catalog"
	apptransport "github.com/zgiai/zgi/api/pkg/apperror/transport"
	"github.com/zgiai/zgi/api/pkg/logger"
	"github.com/zgiai/zgi/api/pkg/queue"
	pkgscheduler "github.com/zgiai/zgi/api/pkg/scheduler"
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
}

// DigitalHumanRuntimeDeps contains the shared bottom-layer runtime and durable
// worker dependencies.
type DigitalHumanRuntimeDeps struct {
	DB                                  *gorm.DB
	LLMClient                           llmclient.LLMClient
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
}

// DigitalHumanRuntimeBundle keeps the model runtime and channel application
// service on the same repository and invocation pipeline.
type DigitalHumanRuntimeBundle struct {
	Runtime   *digitalhuman.RuntimeService
	Access    *digitalhuman.AccessService
	Tasks     digitalhuman.TaskHandlerService
	Channels  *digitalhuman.ChannelService
	Receivers *digitalhuman.ChannelReceiverManager
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
	digitalhuman.NewRuntimeHandler(deps.Runtime, errorProjector).RegisterRoutes(group)
	digitalhuman.NewTaskHandler(deps.Tasks, errorProjector).RegisterRoutes(group)
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
		c.Next()
	}
}

func digitalHumanPermissionForRequest(method, routePath string) workspacemodel.WorkspacePermissionCode {
	path := strings.TrimSpace(routePath)
	if path == "" {
		return ""
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
	if strings.Contains(path, "/my-tasks") {
		return workspacemodel.WorkspacePermissionDigitalHumanRun
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
	if strings.Contains(path, "/access") || strings.Contains(path, "/api-keys") {
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
	)
	dispatcher := digitalhuman.NewAsynqRunDispatcher(deps.TaskManager)
	memoryService := digitalhuman.NewService(repository)
	memoryProvider := digitalhuman.NewMemoryRuntimeProvider(
		memoryService,
		digitalhuman.WithMemoryPromotionAuthorizer(digitalhuman.MemoryPromotionAuthorizerFunc(
			func(ctx context.Context, organizationID, workspaceID, actorID uuid.UUID) (bool, error) {
				return deps.OrganizationService.CheckWorkspacePermission(
					ctx, organizationID.String(), workspaceID.String(), actorID.String(), workspacemodel.WorkspacePermissionDigitalHumanMemoryManage,
				)
			},
		)),
	)
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
		digitalhuman.WithTaskRuntime(digitalhuman.NewTaskRuntimeProvider(deps.Tasks)),
		digitalhuman.WithChannelTransientDispatcher(transientDispatcher),
		digitalhuman.WithChannelContextRuntime(digitalhuman.NewChannelContextRuntime(repository, channelAdapters, modelSpecResolver)),
		digitalhuman.WithManualInvocationFileValidator(digitalhuman.NewConsoleFileValidator(deps.FileService)),
	)
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
	return &DigitalHumanRuntimeBundle{Runtime: runtime, Tasks: deps.Tasks, Channels: channels, Receivers: receivers}
}
