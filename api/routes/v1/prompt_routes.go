package v1

import (
	"github.com/gin-gonic/gin"
	llmclient "github.com/zgiai/zgi/api/internal/modules/llm/client"
	llmdefaultservice "github.com/zgiai/zgi/api/internal/modules/llm/defaultmodel/service"
	"github.com/zgiai/zgi/api/internal/modules/prompts"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	"gorm.io/gorm"
)

// PromptRouteDeps contains dependencies required by prompt routes.
type PromptRouteDeps struct {
	DB                  *gorm.DB
	AccountService      interfaces.AccountService
	OrganizationService interfaces.OrganizationService
	LLMClient           llmclient.LLMClient
	DefaultModelService llmdefaultservice.DefaultModelService
}

func RegisterPromptRoutes(router *gin.RouterGroup, deps PromptRouteDeps) {
	if deps.DB == nil {
		panic("prompt routes require db")
	}
	if deps.AccountService == nil {
		panic("prompt routes require account service")
	}
	if deps.OrganizationService == nil {
		panic("prompt routes require organization service")
	}
	if deps.LLMClient == nil {
		panic("prompt routes require llm client")
	}
	if deps.DefaultModelService == nil {
		panic("prompt routes require default model service")
	}

	module := prompts.NewModule(
		deps.DB,
		deps.AccountService,
		deps.OrganizationService,
		deps.LLMClient,
		deps.DefaultModelService,
	)
	module.PromptHandler.RegisterRoutes(router)
}
