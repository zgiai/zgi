package aichat

import (
	"context"

	"github.com/gin-gonic/gin"
	"github.com/zgiai/zgi/api/internal/capabilities/chatruntime/repository"
	"github.com/zgiai/zgi/api/internal/capabilities/chatruntime/service"
	"github.com/zgiai/zgi/api/internal/modules/aichat/handler"
	connectorexecution "github.com/zgiai/zgi/api/internal/modules/connectors/execution"
	connectorruntimeauth "github.com/zgiai/zgi/api/internal/modules/connectors/runtimeauth"
	llmclient "github.com/zgiai/zgi/api/internal/modules/llm/client"
	llmdefaultservice "github.com/zgiai/zgi/api/internal/modules/llm/defaultmodel/service"
	"github.com/zgiai/zgi/api/internal/modules/relationshipmemory"
	"github.com/zgiai/zgi/api/internal/modules/shared/titlegen"
	"github.com/zgiai/zgi/api/internal/modules/skills"
	apptransport "github.com/zgiai/zgi/api/pkg/apperror/transport"
	"github.com/zgiai/zgi/api/pkg/logger"
	"gorm.io/gorm"
)

type Module struct {
	Handler *handler.Handler
	Service service.Service
}

func NewModule(db *gorm.DB, llmClient llmclient.LLMClient, defaultModelSvc llmdefaultservice.DefaultModelService) *Module {
	return NewModuleWithDependencies(db, llmClient, defaultModelSvc, nil, nil, nil, nil)
}

func NewModuleWithDependencies(
	db *gorm.DB,
	llmClient llmclient.LLMClient,
	defaultModelSvc llmdefaultservice.DefaultModelService,
	fileService service.FileLookupService,
	contentExtractor service.ContentExtractionService,
	workspacePerms service.WorkspacePermissionService,
	agentMemoryService *relationshipmemory.AgentAdapter,
	optionalServices ...interface{},
) *Module {
	modelPrechecker, ok := llmClient.(llmclient.AppModelPrechecker)
	if !ok {
		panic("aichat requires app model prechecker")
	}
	repos := repository.NewRepositories(db)
	var titleGenerator titlegen.Service
	if defaultModelSvc != nil {
		titleGenerator = titlegen.NewService(llmClient, defaultModelSvc)
	}
	dependencies := resolveOptionalModuleDependencies(optionalServices)
	skillRuntime := dependencies.skillRuntime
	connectorChatRuntime := dependencies.connectorChatRuntime
	connectorAgentRuntime := dependencies.connectorAgentRuntime
	connectorConfirmationStore := dependencies.connectorConfirmationStore
	if skillRuntime != nil {
		if err := skillRuntime.ValidateCatalog(context.Background()); err != nil {
			logger.Error("failed to validate aichat skill catalog", err)
		}
	}
	svc := service.NewServiceWithSkillRuntime(
		repos,
		llmClient,
		titleGenerator,
		service.NewDatabaseModelSpecResolver(db),
		fileService,
		contentExtractor,
		workspacePerms,
		skillRuntime,
		agentMemoryService,
		connectorChatRuntime,
		connectorAgentRuntime,
		connectorConfirmationStore,
	)
	if _, err := svc.CleanupStaleActiveMessages(context.Background()); err != nil {
		logger.Warn("failed to cleanup stale aichat messages", err)
	}
	if err := svc.CleanupExpiredCustomSkillImportPreviews(context.Background()); err != nil {
		logger.Warn("failed to cleanup expired aichat skill import previews", err)
	}
	return &Module{
		Handler: handler.NewHandler(svc, modelPrechecker, dependencies.errorProjector),
		Service: svc,
	}
}

type optionalModuleDependencies struct {
	skillRuntime               *skills.Runtime
	connectorChatRuntime       connectorruntimeauth.ChatRuntimeResolver
	connectorAgentRuntime      connectorruntimeauth.AgentRuntimeResolver
	connectorConfirmationStore connectorexecution.ConfirmationContinuationStore
	errorProjector             *apptransport.Projector
}

func resolveOptionalModuleDependencies(optionalServices []interface{}) optionalModuleDependencies {
	var result optionalModuleDependencies
	for _, dependency := range optionalServices {
		switch typed := dependency.(type) {
		case *apptransport.Projector:
			if result.errorProjector == nil {
				result.errorProjector = typed
			}
		case *skills.Runtime:
			if result.skillRuntime == nil {
				result.skillRuntime = typed
			}
		case connectorruntimeauth.ChatRuntimeResolver:
			if result.connectorChatRuntime == nil {
				result.connectorChatRuntime = typed
			}
		case connectorruntimeauth.AgentRuntimeResolver:
			if result.connectorAgentRuntime == nil {
				result.connectorAgentRuntime = typed
			}
		case connectorexecution.ConfirmationContinuationStore:
			if result.connectorConfirmationStore == nil {
				result.connectorConfirmationStore = typed
			}
		}
	}
	return result
}

func (m *Module) RegisterRoutes(router *gin.RouterGroup) {
	m.Handler.RegisterRoutes(router)
}
