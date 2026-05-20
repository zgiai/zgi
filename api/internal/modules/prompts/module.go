package prompts

import (
	"github.com/zgiai/ginext/internal/modules/prompts/handler"
	"github.com/zgiai/ginext/internal/modules/prompts/repository"
	"github.com/zgiai/ginext/internal/modules/prompts/service"
	llmclient "github.com/zgiai/ginext/internal/modules/llm/client"
	llmdefaultservice "github.com/zgiai/ginext/internal/modules/llm/defaultmodel/service"
	interfaces "github.com/zgiai/ginext/internal/modules/shared/interface"
	"gorm.io/gorm"
)

type Module struct {
	PromptRepository repository.PromptRepository
	PromptService    service.PromptService
	PromptHandler    *handler.PromptHandler
}

func NewModule(
	db *gorm.DB,
	accountService interfaces.AccountService,
	organizationService interfaces.OrganizationService,
	llmClient llmclient.LLMClient,
	defaultModelSvc llmdefaultservice.DefaultModelService,
) *Module {
	promptRepo := repository.NewPromptRepository(db)
	promptService := service.NewPromptService(promptRepo, organizationService, llmClient, defaultModelSvc)
	return &Module{
		PromptRepository: promptRepo,
		PromptService:    promptService,
		PromptHandler:    handler.NewPromptHandler(promptService, accountService),
	}
}
