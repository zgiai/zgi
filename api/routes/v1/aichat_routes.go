package v1

import (
	"github.com/gin-gonic/gin"
	runtimeservice "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/service"
	"github.com/zgiai/zgi/api/internal/modules/agentmemory"
	"github.com/zgiai/zgi/api/internal/modules/aichat"
	llmclient "github.com/zgiai/zgi/api/internal/modules/llm/client"
	llmdefaultservice "github.com/zgiai/zgi/api/internal/modules/llm/defaultmodel/service"
	memorymodule "github.com/zgiai/zgi/api/internal/modules/memory"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	"github.com/zgiai/zgi/api/internal/modules/skills"
	"github.com/zgiai/zgi/api/middleware"
	"github.com/zgiai/zgi/api/pkg/logger"
	"gorm.io/gorm"
)

// AIChatRouteDeps contains dependencies required by AIChat routes.
type AIChatRouteDeps struct {
	DB                         *gorm.DB
	LLMClient                  llmclient.LLMClient
	DefaultModelService        llmdefaultservice.DefaultModelService
	FileService                runtimeservice.FileLookupService
	ContentExtractor           runtimeservice.ContentExtractionService
	WorkspacePermissionService runtimeservice.WorkspacePermissionService
	MemoryService              *memorymodule.Service
	AgentMemoryService         *agentmemory.Service
	SkillRuntime               *skills.Runtime
	AccountService             interfaces.AccountService
}

func RegisterAIChatRoutes(router *gin.RouterGroup, deps AIChatRouteDeps) {
	if deps.DB == nil {
		panic("aichat routes require db")
	}
	if deps.LLMClient == nil {
		panic("aichat routes require llm client")
	}
	if deps.DefaultModelService == nil {
		panic("aichat routes require default model service")
	}
	if deps.FileService == nil {
		panic("aichat routes require file service")
	}
	if deps.ContentExtractor == nil {
		panic("aichat routes require content extractor")
	}
	if deps.WorkspacePermissionService == nil {
		panic("aichat routes require workspace permission service")
	}
	if deps.MemoryService == nil {
		panic("aichat routes require memory service")
	}
	if deps.AgentMemoryService == nil {
		panic("aichat routes require agent memory service")
	}
	if deps.SkillRuntime == nil {
		panic("aichat routes require skill runtime")
	}
	if deps.AccountService == nil {
		panic("aichat routes require account service")
	}

	module := aichat.NewModuleWithDependencies(
		deps.DB,
		deps.LLMClient,
		deps.DefaultModelService,
		deps.FileService,
		deps.ContentExtractor,
		deps.WorkspacePermissionService,
		deps.MemoryService,
		deps.AgentMemoryService,
		deps.SkillRuntime,
	)
	group := router.Group("")
	group.Use(middleware.SetupRequired())
	group.Use(middleware.JWTWithOrganizationAndService(deps.AccountService))
	module.RegisterRoutes(group)
	logger.Info("AIChat routes registered", "path", "/console/api/aichat/*")
}
