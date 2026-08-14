package fxapp

import (
	"context"
	"errors"
	"net"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/hibiken/asynq"
	"github.com/zgiai/zgi/api/config"
	"github.com/zgiai/zgi/api/internal/container"
	grpcinfra "github.com/zgiai/zgi/api/internal/infra/grpc"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/graph_engine"
	workflowtest "github.com/zgiai/zgi/api/internal/modules/app/workflowtest"
	"github.com/zgiai/zgi/api/internal/modules/dataset/graphflow"
	graphflowworker "github.com/zgiai/zgi/api/internal/modules/dataset/graphflow/worker"
	llmclient "github.com/zgiai/zgi/api/internal/modules/llm/client"
	system_service "github.com/zgiai/zgi/api/internal/modules/system/service"
	videoservice "github.com/zgiai/zgi/api/internal/modules/video/service"
	"github.com/zgiai/zgi/api/internal/observability"
	appcatalog "github.com/zgiai/zgi/api/pkg/apperror/catalog"
	"github.com/zgiai/zgi/api/pkg/queue"
	pkgscheduler "github.com/zgiai/zgi/api/pkg/scheduler"
	"github.com/zgiai/zgi/api/routes"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

type runtimeParams struct {
	fx.In

	HTTPServer          *http.Server
	GRPCServer          *grpcinfra.Server
	HTTPListener        net.Listener `name:"http_listener"`
	GRPCListener        net.Listener `name:"grpc_listener"`
	Config              *config.Config
	BootstrapService    *system_service.BootstrapService
	ServiceContainer    *container.ServiceContainer
	GraphFlowService    *graphflow.Service
	WorkflowTestService *workflowtest.Service
	LLMClient           llmclient.LLMClient
	VideoTaskPoller     *videoservice.TaskPoller
	TaskManager         *queue.TaskManager
	TaskHandlerRegistry *container.TaskHandlerRegistrar
	Scheduler           *pkgscheduler.Scheduler
	OpenTelemetry       *OpenTelemetryResource
	ZGIReporter         *observability.ZGIReporter
	Logger              *zap.Logger
}

type routeParams struct {
	fx.In

	Engine                  *gin.Engine
	ServiceContainer        *container.ServiceContainer
	WorkflowEngineFactory   *graph_engine.EngineFactory
	ApplicationErrorCatalog *appcatalog.Catalog
}

// GRPCServerLifecycle describes the runtime operations required for a gRPC server.
type GRPCServerLifecycle interface {
	Serve(listener net.Listener) error
	Stop()
}

// TaskManagerLifecycle describes the runtime operations required for a task server.
type TaskManagerLifecycle interface {
	StartServer(mux *asynq.ServeMux) error
	StopServer()
	Close() error
}

// TaskHandlerRegistrar registers all task handlers into a mux.
type TaskHandlerRegistrar interface {
	RegisterAll(mux *asynq.ServeMux)
}

// SchedulerLifecycle describes the runtime operations required for a scheduler.
type SchedulerLifecycle interface {
	Start() error
	Stop() error
}

var runtimeModule = fx.Module("runtime",
	fx.Invoke(
		registerRoutes,
		registerRuntime,
	),
)

func registerRoutes(params routeParams) {
	routes.RegisterRoutes(params.Engine, params.ServiceContainer, params.WorkflowEngineFactory, params.ApplicationErrorCatalog)
}

func registerRuntime(lc fx.Lifecycle, params runtimeParams) error {
	graphLifecycle := params.ServiceContainer.GetGraphLifecycleService()
	_ = params.ServiceContainer.GetGraphVisibilityService()
	graphRuntimeHealth := params.ServiceContainer.GetGraphRuntimeHealthService()

	graphflowworker.RegisterGraphFlowHandlers(
		params.TaskHandlerRegistry,
		params.GraphFlowService,
		params.TaskManager,
	)
	outboxReconciler := graphflowworker.NewOutboxReconciler(
		params.GraphFlowService.DB,
		graphflowworker.NewRunOutboxHandler(params.GraphFlowService, params.TaskManager),
		graphflowworker.NewVisibilityHandler(params.GraphFlowService),
		graphflowworker.NewDatasetPurgeHandler(params.GraphFlowService),
	)

	if params.Scheduler != nil {
		globalCleanupTask := graphflowworker.NewGlobalCleanupTask()
		globalCleanupHandler := graphflowworker.NewGlobalCleanupHandler(params.GraphFlowService, params.TaskManager)
		if err := params.Scheduler.RegisterTask(globalCleanupTask, globalCleanupHandler); err != nil {
			return err
		}
	}

	RegisterCloudBootstrapLifecycle(
		lc,
		system_service.NewCloudBootstrapRunner(params.Config, params.BootstrapService),
		params.Logger,
	)
	RegisterGraphRuntimeHealthLifecycle(lc, graphRuntimeHealth)
	RegisterGraphEvidenceRepairLifecycle(lc, params.GraphFlowService, params.Logger)
	RegisterGraphOutboxReconcilerLifecycle(lc, outboxReconciler, params.Logger)
	RegisterGraphRunReconcilerLifecycle(lc, graphLifecycle, params.Logger)
	registerOpenTelemetryLifecycle(lc, params.OpenTelemetry, params.Logger)
	RegisterWorkflowTestLocalWorkerLifecycle(lc, params.Config, params.WorkflowTestService, params.LLMClient, params.Logger)
	RegisterVideoRuntimeTaskPollerLifecycle(lc, params.VideoTaskPoller, params.Logger)
	RegisterTaskManagerLifecycle(lc, params.TaskManager, params.TaskHandlerRegistry, params.Logger)
	RegisterSchedulerLifecycle(lc, params.Scheduler, params.Logger)
	RegisterGRPCServerLifecycle(lc, params.GRPCServer, params.GRPCListener, params.Logger)
	RegisterHTTPServerLifecycle(lc, params.HTTPServer, params.HTTPListener, params.Logger)
	RegisterSQLAuditRecorderLifecycle(lc, params.ServiceContainer, params.Logger)
	registerZGIReporterLifecycle(lc, params.ZGIReporter, params.Logger)

	return nil
}

func RegisterGraphEvidenceRepairLifecycle(
	lc fx.Lifecycle,
	service *graphflow.Service,
	log *zap.Logger,
) {
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			if service == nil {
				return nil
			}
			repairedDatasets, err := service.RepairLegacyEvidenceProvenance(ctx)
			if err != nil {
				log.Warn("Legacy graph evidence repair failed", zap.Error(err))
				return nil
			}
			if repairedDatasets > 0 {
				log.Info("Legacy graph evidence repaired", zap.Int("dataset_count", repairedDatasets))
			}
			return nil
		},
	})
}

func RegisterGraphRunReconcilerLifecycle(
	lc fx.Lifecycle,
	lifecycle *graphflow.LifecycleService,
	log *zap.Logger,
) {
	var cancel context.CancelFunc
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			if lifecycle == nil {
				return nil
			}
			if err := lifecycle.ReconcileActiveRuns(ctx); err != nil {
				log.Warn("Initial graph run reconciliation failed", zap.Error(err))
			}
			monitorContext, stop := context.WithCancel(context.Background())
			cancel = stop
			go func() {
				ticker := time.NewTicker(10 * time.Second)
				defer ticker.Stop()
				for {
					select {
					case <-monitorContext.Done():
						return
					case <-ticker.C:
						if err := lifecycle.ReconcileActiveRuns(monitorContext); err != nil {
							log.Warn("Graph run reconciliation failed", zap.Error(err))
						}
					}
				}
			}()
			return nil
		},
		OnStop: func(context.Context) error {
			if cancel != nil {
				cancel()
			}
			return nil
		},
	})
}

func RegisterGraphOutboxReconcilerLifecycle(
	lc fx.Lifecycle,
	reconciler *graphflowworker.OutboxReconciler,
	log *zap.Logger,
) {
	var cancel context.CancelFunc
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			if err := reconciler.RunOnce(ctx); err != nil {
				log.Warn("Initial graph outbox reconciliation failed", zap.Error(err))
			}
			monitorContext, stop := context.WithCancel(context.Background())
			cancel = stop
			go func() {
				ticker := time.NewTicker(5 * time.Second)
				defer ticker.Stop()
				for {
					select {
					case <-monitorContext.Done():
						return
					case <-ticker.C:
						if err := reconciler.RunOnce(monitorContext); err != nil {
							log.Warn("Graph outbox reconciliation failed", zap.Error(err))
						}
					}
				}
			}()
			return nil
		},
		OnStop: func(context.Context) error {
			if cancel != nil {
				cancel()
			}
			return nil
		},
	})
}

func RegisterGraphRuntimeHealthLifecycle(
	lc fx.Lifecycle,
	health *system_service.GraphRuntimeHealthService,
) {
	var cancel context.CancelFunc
	lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			if health == nil {
				return nil
			}
			monitorContext, stop := context.WithCancel(context.Background())
			cancel = stop
			go func() {
				initialContext, initialCancel := context.WithTimeout(monitorContext, 10*time.Second)
				health.Check(initialContext)
				initialCancel()
				ticker := time.NewTicker(30 * time.Second)
				defer ticker.Stop()
				for {
					select {
					case <-monitorContext.Done():
						return
					case <-ticker.C:
						checkContext, checkCancel := context.WithTimeout(monitorContext, 10*time.Second)
						health.Check(checkContext)
						checkCancel()
					}
				}
			}()
			return nil
		},
		OnStop: func(context.Context) error {
			if cancel != nil {
				cancel()
			}
			return nil
		},
	})
}

// RegisterCloudBootstrapLifecycle runs cloud bootstrap before network listeners start.
func RegisterCloudBootstrapLifecycle(
	lc fx.Lifecycle,
	runner *system_service.CloudBootstrapRunner,
	log *zap.Logger,
) {
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			if err := runner.Run(ctx); err != nil {
				log.Error("Cloud bootstrap failed", zap.Error(err))
				return err
			}
			return nil
		},
	})
}

// RegisterSQLAuditRecorderLifecycle flushes queued SQL audit records during shutdown.
func RegisterSQLAuditRecorderLifecycle(lc fx.Lifecycle, serviceContainer *container.ServiceContainer, log *zap.Logger) {
	lc.Append(fx.Hook{
		OnStop: func(ctx context.Context) error {
			var closeErr error
			if serviceContainer != nil {
				if err := serviceContainer.CloseSQLAuditRecorder(ctx); err != nil {
					closeErr = err
					log.Error("failed to close workflow SQL audit recorder", zap.Error(err))
				}
				if err := serviceContainer.CloseDataSourceSQLAuditRecorder(ctx); err != nil {
					closeErr = err
					log.Error("failed to close datasource SQL audit recorder", zap.Error(err))
				}
			}
			return closeErr
		},
	})
}

// RegisterHTTPServerLifecycle registers the HTTP server lifecycle hooks.
func RegisterHTTPServerLifecycle(lc fx.Lifecycle, server *http.Server, listener net.Listener, log *zap.Logger) {
	lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			go func() {
				log.Info("Starting HTTP server", zap.String("addr", listener.Addr().String()))
				if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) && !errors.Is(err, net.ErrClosed) {
					log.Error("HTTP server error", zap.Error(err))
				}
			}()
			return nil
		},
		OnStop: func(ctx context.Context) error {
			log.Info("Stopping HTTP server", zap.String("addr", listener.Addr().String()))
			err := server.Shutdown(ctx)
			closeErr := listener.Close()
			if errors.Is(closeErr, net.ErrClosed) {
				closeErr = nil
			}
			return errors.Join(err, closeErr)
		},
	})
}

// RegisterGRPCServerLifecycle registers the gRPC server lifecycle hooks.
func RegisterGRPCServerLifecycle(lc fx.Lifecycle, server GRPCServerLifecycle, listener net.Listener, log *zap.Logger) {
	if listener == nil {
		log.Info("gRPC server disabled")
		return
	}

	lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			go func() {
				log.Info("Starting gRPC server", zap.String("addr", listener.Addr().String()))
				if err := server.Serve(listener); err != nil && !errors.Is(err, net.ErrClosed) {
					log.Error("gRPC server error", zap.Error(err))
				}
			}()
			return nil
		},
		OnStop: func(context.Context) error {
			log.Info("Stopping gRPC server", zap.String("addr", listener.Addr().String()))
			server.Stop()
			if err := listener.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
				return err
			}
			return nil
		},
	})
}

// RegisterTaskManagerLifecycle registers the task manager lifecycle hooks.
func RegisterTaskManagerLifecycle(
	lc fx.Lifecycle,
	taskManager TaskManagerLifecycle,
	registry TaskHandlerRegistrar,
	log *zap.Logger,
) {
	lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			mux := asynq.NewServeMux()
			registry.RegisterAll(mux)

			go func() {
				if err := taskManager.StartServer(mux); err != nil {
					log.Error("Task manager server error", zap.Error(err))
				}
			}()
			return nil
		},
		OnStop: func(context.Context) error {
			log.Info("Stopping task manager")
			taskManager.StopServer()
			return taskManager.Close()
		},
	})
}

func RegisterWorkflowTestLocalWorkerLifecycle(
	lc fx.Lifecycle,
	cfg *config.Config,
	service *workflowtest.Service,
	client llmclient.LLMClient,
	log *zap.Logger,
) {
	if cfg == nil || workflowtest.NormalizeTaskBackend(cfg.TaskQueue.WorkflowTestTaskBackend) != workflowtest.WorkflowTestTaskBackendLocal {
		return
	}
	worker := workflowtest.NewLocalWorker(service, client)
	if service != nil {
		service.SetTaskCanceler(worker)
	}
	var cancel context.CancelFunc
	lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			var workerCtx context.Context
			workerCtx, cancel = context.WithCancel(context.Background())
			go worker.Start(workerCtx)
			log.Info("Started workflow test local worker")
			return nil
		},
		OnStop: func(context.Context) error {
			if cancel != nil {
				cancel()
			}
			log.Info("Stopped workflow test local worker")
			return nil
		},
	})
}

// RegisterSchedulerLifecycle registers the scheduler lifecycle hooks.
func RegisterSchedulerLifecycle(lc fx.Lifecycle, scheduler SchedulerLifecycle, log *zap.Logger) {
	lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			log.Info("Starting scheduler")
			return scheduler.Start()
		},
		OnStop: func(context.Context) error {
			log.Info("Stopping scheduler")
			return scheduler.Stop()
		},
	})
}

func registerZGIReporterLifecycle(lc fx.Lifecycle, reporter *observability.ZGIReporter, log *zap.Logger) {
	if reporter == nil || !reporter.Enabled() {
		return
	}

	lc.Append(fx.Hook{
		OnStop: func(ctx context.Context) error {
			flushCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
			defer cancel()
			if err := reporter.Flush(flushCtx); err != nil {
				log.Warn("failed to flush ZGI Reporter", zap.Error(err))
			}
			return nil
		},
	})
}

func RegisterVideoRuntimeTaskPollerLifecycle(
	lc fx.Lifecycle,
	poller *videoservice.TaskPoller,
	log *zap.Logger,
) {
	var cancel context.CancelFunc
	lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			if poller == nil {
				if log != nil {
					log.Info("Video runtime task poller skipped")
				}
				return nil
			}
			var pollerCtx context.Context
			pollerCtx, cancel = context.WithCancel(context.Background())
			go poller.Start(pollerCtx)
			if log != nil {
				log.Info("Video runtime task poller started")
			}
			return nil
		},
		OnStop: func(context.Context) error {
			if cancel != nil {
				cancel()
			}
			return nil
		},
	})
}
