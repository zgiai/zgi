package aichat

import (
	"context"

	"github.com/gin-gonic/gin"
	"github.com/zgiai/ginext/internal/modules/aichat/handler"
	"github.com/zgiai/ginext/internal/modules/aichat/repository"
	"github.com/zgiai/ginext/internal/modules/aichat/service"
	llmclient "github.com/zgiai/ginext/internal/modules/llm/client"
	llmdefaultservice "github.com/zgiai/ginext/internal/modules/llm/defaultmodel/service"
	"github.com/zgiai/ginext/internal/modules/shared/titlegen"
	"github.com/zgiai/ginext/internal/modules/skills"
	"github.com/zgiai/ginext/pkg/logger"
	"gorm.io/gorm"
)

type Module struct {
	Handler *handler.Handler
	Service service.Service
}

func NewModule(db *gorm.DB, llmClient llmclient.LLMClient, defaultModelSvc llmdefaultservice.DefaultModelService) *Module {
	return NewModuleWithDependencies(db, llmClient, defaultModelSvc, nil, nil, nil)
}

func NewModuleWithDependencies(
	db *gorm.DB,
	llmClient llmclient.LLMClient,
	defaultModelSvc llmdefaultservice.DefaultModelService,
	fileService service.FileLookupService,
	contentExtractor service.ContentExtractionService,
	workspacePerms service.WorkspacePermissionService,
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
			logger.Warn("failed to validate aichat skill catalog", err)
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
	)
	if _, err := svc.CleanupStaleActiveMessages(context.Background()); err != nil {
		logger.Warn("failed to cleanup stale aichat messages", err)
	}
	return &Module{
		Handler: handler.NewHandler(svc),
		Service: svc,
	}
}

func (m *Module) RegisterRoutes(router *gin.RouterGroup) {
	m.Handler.RegisterRoutes(router)
}
