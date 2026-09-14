package v1

import (
	"context"
	"fmt"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	pconsole "github.com/zgiai/zgi/api/internal/infra/platform/console"
	"github.com/zgiai/zgi/api/internal/modules/llm"
	apikeyrepo "github.com/zgiai/zgi/api/internal/modules/llm/apikey/repository"
	"github.com/zgiai/zgi/api/internal/modules/llm/client"
	"github.com/zgiai/zgi/api/internal/modules/llm/credential/upstreamstate"
	"github.com/zgiai/zgi/api/internal/modules/llm/gateway"
	"github.com/zgiai/zgi/api/internal/modules/llm/handler"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/modules/llm/shared"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	authservice "github.com/zgiai/zgi/api/internal/modules/user/auth/service"
	"github.com/zgiai/zgi/api/middleware"
	appcatalog "github.com/zgiai/zgi/api/pkg/apperror/catalog"
	apptransport "github.com/zgiai/zgi/api/pkg/apperror/transport"
	"github.com/zgiai/zgi/api/pkg/logger"
	redisPkg "github.com/zgiai/zgi/api/pkg/redis"
	pkgscheduler "github.com/zgiai/zgi/api/pkg/scheduler"
)

type LLMRouteDeps struct {
	DB                         *gorm.DB
	AccountService             interfaces.AccountService
	WorkspaceManagementService interfaces.WorkspaceManagementService
	OrganizationService        interfaces.OrganizationService
	ConsoleProvider            pconsole.ConsoleProvider
	Scheduler                  *pkgscheduler.Scheduler
	ApplicationErrorCatalog    *appcatalog.Catalog
}

// RegisterLLMRoutes registers all LLM-related routes
// Includes: /llm/* (internal for workflows/knowledge base)
func RegisterLLMRoutes(router *gin.RouterGroup, deps LLMRouteDeps) *llm.LLMModule {
	validateLLMRouteDeps(deps)

	// ========== Initialize V2 Module ==========
	cryptoService, err := shared.DefaultCryptoService()
	if err != nil {
		failCloudLLMInitialization(deps.ConsoleProvider, "LLM crypto service", err)
		logger.Error("failed to create LLM crypto service", err)
		return nil
	}
	errorProjector, err := apptransport.NewProjector(deps.ApplicationErrorCatalog)
	if err != nil {
		logger.Error("failed to create developer access error projector", err)
		return nil
	}
	llmV2Module := llm.NewLLMModule(
		deps.DB,
		cryptoService,
		deps.WorkspaceManagementService,
		deps.AccountService,
		deps.OrganizationService,
		deps.ConsoleProvider,
		errorProjector,
	)
	registerLLMUpstreamPolling(deps.Scheduler, llmV2Module)
	registerRegistrationProvisioningOutbox(deps, llmV2Module)

	// ========== Initialize Internal AI Service (for workflows/knowledge base) ==========
	llmAPIKeyRepo := apikeyrepo.NewAPIKeyRepository(deps.DB)

	gatewayService, err := gateway.NewLLMGatewayService(
		deps.DB,
		llmAPIKeyRepo,
		adapter.GlobalFactory,
	)
	if err != nil {
		logger.Warn("failed to initialize LLM gateway service, skipping internal route registration", err)
		return llmV2Module
	}

	// ===== Performance Optimizations =====
	// 1. Enable Config Cache (Model/Provider/ShadowTenant)
	redisClient := redisPkg.GetClient()
	if redisClient != nil {
		configCache := gateway.NewConfigCache(redisClient, deps.DB, nil)
		gatewayService.SetConfigCache(configCache)
		logger.Info("LLM config cache enabled")
	}

	// ===== End Performance Optimizations =====

	llmClient := client.New(gatewayService, llmAPIKeyRepo, deps.DB)
	llmInternalHandler := handler.NewLLMInternalHandler(llmClient)

	// ========== System /console/api/llm/modelmeta/* Routes ==========
	// ModelMeta is system-scoped and must not depend on workspace resolution.
	modelMetaGroup := router.Group("")
	modelMetaGroup.Use(middleware.JWT())
	llm.RegisterModelMetaRoutes(modelMetaGroup, llmV2Module)
	logger.Info("LLM model metadata routes registered", "path", "/console/api/llm/modelmeta/*")

	// ========== Console /console/api/llm/* Routes ==========
	// Register console routes for tenant LLM management
	// Note: router is already /console/api, RegisterConsoleRoutes will add /llm
	// Apply JWT middleware to set tenant_id in context
	consoleGroup := router.Group("")
	consoleGroup.Use(middleware.JWTWithOrganizationAndService(deps.AccountService))
	llm.RegisterConsoleRoutes(consoleGroup, llmV2Module)
	logger.Info("LLM console routes registered", "path", "/console/api/llm/*")

	// ========== Legacy /llm/* Routes (using V2 module) ==========
	// Note: These are for internal use by workflows/knowledge base
	// Console routes are registered separately via RegisterConsoleRoutes above
	llmGroup := router.Group("/llm")
	llmGroup.Use(middleware.JWTWithOrganizationAndService(deps.AccountService))

	// Internal AI Routes (for workflows/knowledge base)
	llmGroup.POST("/chat/completions", llmInternalHandler.ChatCompletions)
	llmGroup.POST("/embeddings", llmInternalHandler.Embeddings)
	llmGroup.POST("/rerank", llmInternalHandler.Rerank)
	llmGroup.POST("/responses", llmInternalHandler.CreateResponse)

	logger.Info("LLM legacy internal routes registered", "path", "/llm/*")
	return llmV2Module
}

func registerRegistrationProvisioningOutbox(deps LLMRouteDeps, module *llm.LLMModule) {
	if deps.ConsoleProvider == nil || !deps.ConsoleProvider.IsAvailable() ||
		!strings.EqualFold(deps.ConsoleProvider.GetMode(), "CLOUD") {
		return
	}
	if module == nil || module.ChannelSvc == nil {
		panic("cloud registration provisioning outbox requires the LLM channel service")
	}
	processor := authservice.NewRegistrationProvisioningOutboxProcessor(
		deps.DB,
		module.ChannelSvc,
		deps.ConsoleProvider,
	)
	if setter, ok := deps.AccountService.(interface {
		SetRegistrationProvisioningOutboxDispatcher(func(context.Context, string) error)
	}); ok {
		setter.SetRegistrationProvisioningOutboxDispatcher(processor.ProcessOne)
	}
	if deps.Scheduler == nil {
		panic("cloud registration provisioning outbox requires the scheduler")
	}
	if err := processor.ValidateConfiguration(); err != nil {
		panic(fmt.Sprintf("cloud registration provisioning outbox is invalid: %v", err))
	}
	task := &authservice.RegistrationProvisioningOutboxTask{}
	mustRegisterRegistrationProvisioningOutboxTask(
		deps.Scheduler,
		task,
		authservice.NewRegistrationProvisioningOutboxHandler(processor, 25),
	)
	logger.Info("Registration provisioning outbox task registered", map[string]interface{}{
		"interval": task.Interval().String(),
	})
}

type registrationProvisioningTaskRegistrar interface {
	RegisterTask(pkgscheduler.ScheduledTask, pkgscheduler.TaskHandler) error
}

func mustRegisterRegistrationProvisioningOutboxTask(
	scheduler registrationProvisioningTaskRegistrar,
	task pkgscheduler.ScheduledTask,
	handler pkgscheduler.TaskHandler,
) {
	if scheduler == nil || task == nil || handler == nil {
		panic("cloud registration provisioning outbox scheduler is not configured")
	}
	if err := scheduler.RegisterTask(task, handler); err != nil {
		panic(fmt.Sprintf("failed to register registration provisioning outbox task: %v", err))
	}
}

func failCloudLLMInitialization(consoleProvider pconsole.ConsoleProvider, component string, err error) {
	if err == nil || consoleProvider == nil || !consoleProvider.IsAvailable() ||
		!strings.EqualFold(consoleProvider.GetMode(), "CLOUD") {
		return
	}
	panic(fmt.Sprintf("cloud registration provisioning requires %s: %v", component, err))
}

func registerLLMUpstreamPolling(scheduler *pkgscheduler.Scheduler, module *llm.LLMModule) {
	if scheduler == nil || module == nil || module.UpstreamStateSvc == nil {
		return
	}
	task := upstreamstate.NewPollingTask()
	if err := scheduler.RegisterTask(task, upstreamstate.NewPollingHandler(module.UpstreamStateSvc)); err != nil {
		logger.Error("failed to register LLM upstream credential polling", err)
		return
	}
	logger.Info("LLM upstream credential polling registered", map[string]interface{}{
		"interval": task.Interval().String(),
	})
}

func validateLLMRouteDeps(deps LLMRouteDeps) {
	if deps.DB == nil {
		panic("llm routes require db")
	}
	if deps.AccountService == nil {
		panic("llm routes require account service")
	}
	if deps.WorkspaceManagementService == nil {
		panic("llm routes require workspace management service")
	}
	if deps.OrganizationService == nil {
		panic("llm routes require organization service")
	}
	if deps.ConsoleProvider == nil {
		panic("llm routes require console provider")
	}
}
