package aichat

import (
	"context"

	"github.com/gin-gonic/gin"
	actionrepo "github.com/zgiai/zgi/api/internal/capabilities/actionruntime/repository"
	actionservice "github.com/zgiai/zgi/api/internal/capabilities/actionruntime/service"
	"github.com/zgiai/zgi/api/internal/capabilities/chatruntime/repository"
	"github.com/zgiai/zgi/api/internal/capabilities/chatruntime/service"
	"github.com/zgiai/zgi/api/internal/modules/agentmemory"
	"github.com/zgiai/zgi/api/internal/modules/aichat/handler"
	llmclient "github.com/zgiai/zgi/api/internal/modules/llm/client"
	llmdefaultservice "github.com/zgiai/zgi/api/internal/modules/llm/defaultmodel/service"
	memorymodule "github.com/zgiai/zgi/api/internal/modules/memory"
	"github.com/zgiai/zgi/api/internal/modules/shared/titlegen"
	"github.com/zgiai/zgi/api/internal/modules/skills"
	"github.com/zgiai/zgi/api/pkg/logger"
	"gorm.io/gorm"
)

type Module struct {
	Handler       *handler.Handler
	Service       service.Service
	ActionService actionservice.Service
}

func NewModule(db *gorm.DB, llmClient llmclient.LLMClient, defaultModelSvc llmdefaultservice.DefaultModelService) *Module {
	return NewModuleWithDependencies(db, llmClient, defaultModelSvc, nil, nil, nil, nil, nil)
}

func NewModuleWithDependencies(
	db *gorm.DB,
	llmClient llmclient.LLMClient,
	defaultModelSvc llmdefaultservice.DefaultModelService,
	fileService service.FileLookupService,
	contentExtractor service.ContentExtractionService,
	workspacePerms service.WorkspacePermissionService,
	memoryService *memorymodule.Service,
	agentMemoryService *agentmemory.Service,
	skillRuntimes ...*skills.Runtime,
) *Module {
	repos := repository.NewRepositories(db)
	var titleGenerator titlegen.Service
	if defaultModelSvc != nil {
		titleGenerator = titlegen.NewService(llmClient, defaultModelSvc)
	}
	var skillRuntime *skills.Runtime
	if len(skillRuntimes) > 0 {
		skillRuntime = skillRuntimes[0]
	}
	if skillRuntime != nil {
		if err := skillRuntime.ValidateCatalog(context.Background()); err != nil {
			logger.Error("failed to validate aichat skill catalog", err)
		}
	}
	actionOptions := []actionservice.Option{}
	if fileService != nil {
		actionOptions = append(actionOptions, actionservice.WithExecutor(
			"file.read",
			actionservice.NewFileReadExecutor(fileService, contentExtractor, workspacePerms),
		))
	}
	actionSvc := actionservice.NewService(actionrepo.NewRepository(db), actionservice.NewDefaultRegistry(), actionOptions...)
	svc := service.NewServiceWithSkillRuntime(
		repos,
		llmClient,
		titleGenerator,
		service.NewDatabaseModelSpecResolver(db),
		fileService,
		contentExtractor,
		workspacePerms,
		skillRuntime,
		memoryService,
		agentMemoryService,
		actionSvc,
	)
	if _, err := svc.CleanupStaleActiveMessages(context.Background()); err != nil {
		logger.Warn("failed to cleanup stale aichat messages", err)
	}
	if err := svc.CleanupExpiredCustomSkillImportPreviews(context.Background()); err != nil {
		logger.Warn("failed to cleanup expired aichat skill import previews", err)
	}
	return &Module{
		Handler:       handler.NewHandler(svc, actionSvc),
		Service:       svc,
		ActionService: actionSvc,
	}
}

func (m *Module) RegisterRoutes(router *gin.RouterGroup) {
	m.Handler.RegisterRoutes(router)
}
