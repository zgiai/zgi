package v1

import (
	"github.com/gin-gonic/gin"
	contentparsemodule "github.com/zgiai/zgi/api/internal/modules/contentparse"
	llmclient "github.com/zgiai/zgi/api/internal/modules/llm/client"
	llmdefaultservice "github.com/zgiai/zgi/api/internal/modules/llm/defaultmodel/service"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	"github.com/zgiai/zgi/api/middleware"
	"gorm.io/gorm"
)

// ContentParseRouteDeps contains dependencies required by content parse routes.
type ContentParseRouteDeps struct {
	DB                  *gorm.DB
	AccountService      interfaces.AccountService
	LLMClient           llmclient.LLMClient
	DefaultModelService llmdefaultservice.DefaultModelService
}

func RegisterContentParseRoutes(v1 *gin.RouterGroup, deps ContentParseRouteDeps) {
	if deps.DB == nil {
		panic("content parse routes require db")
	}
	if deps.AccountService == nil {
		panic("content parse routes require account service")
	}
	if deps.LLMClient == nil {
		panic("content parse routes require llm client")
	}
	if deps.DefaultModelService == nil {
		panic("content parse routes require default model service")
	}

	group := v1.Group("")
	group.Use(middleware.SetupRequired())
	group.Use(middleware.JWTWithOrganizationAndService(deps.AccountService))

	contentparsemodule.NewModule(
		deps.DB,
		contentparsemodule.WithSystemVisionModel(deps.LLMClient, deps.DefaultModelService),
	).RegisterPlaygroundRoutes(group)
}
