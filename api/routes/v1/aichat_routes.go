package v1

import (
	"context"

	"github.com/gin-gonic/gin"
	chatruntime "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/service"
	"github.com/zgiai/zgi/api/internal/modules/agentmemory"
	"github.com/zgiai/zgi/api/internal/modules/aichat"
	"github.com/zgiai/zgi/api/internal/modules/integrations"
	integrationmetatools "github.com/zgiai/zgi/api/internal/modules/integrations/metatools"
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
	DB                           *gorm.DB
	LLMClient                    llmclient.LLMClient
	DefaultModelService          llmdefaultservice.DefaultModelService
	FileService                  chatruntime.FileLookupService
	ContentExtractor             chatruntime.ContentExtractionService
	WorkspacePermissionService   chatruntime.WorkspacePermissionService
	MemoryService                *memorymodule.Service
	AgentMemoryService           *agentmemory.Service
	SkillRuntime                 *skills.Runtime
	AccountService               interfaces.AccountService
	IntegrationPreferences       *integrations.DefaultAIChatIntegrationPreferenceService
	IntegrationActionProjections integrationmetatools.ActionProjectionResolver
}

func RegisterAIChatRoutes(router *gin.RouterGroup, deps AIChatRouteDeps) chatruntime.Service {
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

	preferenceResolver := chatruntime.AIChatIntegrationPreferenceResolver(nil)
	if deps.IntegrationPreferences != nil {
		preferenceResolver = chatruntime.AIChatIntegrationPreferenceResolverFunc(func(ctx context.Context, scope chatruntime.Scope) (chatruntime.AIChatIntegrationRuntimePreferences, error) {
			items, err := deps.IntegrationPreferences.List(ctx, scope.OrganizationID, scope.AccountID, scope.WorkspaceID)
			if err != nil {
				return chatruntime.AIChatIntegrationRuntimePreferences{}, err
			}
			selected := make(map[string][]string, len(items))
			preferred := make(map[string]string, len(items))
			for _, item := range items {
				selected[item.IntegrationID] = append([]string(nil), item.SelectedConnectionIDs...)
				if item.PreferredConnectionID != nil {
					preferred[item.IntegrationID] = item.PreferredConnectionID.String()
				}
			}
			return chatruntime.AIChatIntegrationRuntimePreferences{
				SelectedConnectionIDs: selected, PreferredConnectionIDs: preferred,
			}, nil
		})
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
		preferenceResolver,
		deps.IntegrationActionProjections,
	)
	group := router.Group("")
	group.Use(middleware.SetupRequired())
	group.Use(middleware.JWTWithOrganizationAndService(deps.AccountService))
	group.Use(aichatWorkspaceScopeMiddleware(deps.DB, deps.AccountService))
	module.RegisterRoutes(group)
	logger.Info("AIChat routes registered", "path", "/console/api/aichat/*")
	return module.Service
}
