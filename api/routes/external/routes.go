package external

import (
	"github.com/gin-gonic/gin"
	workflow_file "github.com/zgiai/zgi/api/internal/modules/app/workflow/file"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/graph_engine"
	"github.com/zgiai/zgi/api/internal/modules/dataset/graphflow"
	datasetservice "github.com/zgiai/zgi/api/internal/modules/dataset/service"
	datasourceservice "github.com/zgiai/zgi/api/internal/modules/datasource/service"
	llmclient "github.com/zgiai/zgi/api/internal/modules/llm/client"
	"github.com/zgiai/zgi/api/internal/modules/memory"
	promptservice "github.com/zgiai/zgi/api/internal/modules/prompts/service"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	"github.com/zgiai/zgi/api/internal/modules/tools"
	"gorm.io/gorm"
)

type ExternalRouteDeps struct {
	DB                    *gorm.DB
	AccountService        interfaces.AccountService
	FileService           interfaces.FileService
	ContentExtractor      workflow_file.ContentExtractor
	QuotaService          interfaces.QuotaService
	OrganizationService   interfaces.OrganizationService
	LLMClient             llmclient.LLMClient
	ToolEngine            *tools.ToolEngine
	ToolManager           *tools.ToolManager
	MemoryService         *memory.Service
	GraphFlowService      *graphflow.Service
	PromptResolver        promptservice.PromptService
	DataSourceService     datasourceservice.DataSourceService
	KnowledgeService      *datasetservice.KnowledgeRetrievalService
	ResourcePermission    interfaces.ResourcePermissionService
	WorkflowEngineFactory *graph_engine.EngineFactory
}

// RegisterExternalRoutes registers all external API routes
func RegisterExternalRoutes(r *gin.Engine, deps ExternalRouteDeps) {
	validateExternalRouteDeps(deps)

	// External API v1 routes
	externalV1 := r.Group("/api")
	{
		// Public APIs that don't require authentication
		RegisterPublicRoutes(externalV1)

		// APIs that require API key authentication
		RegisterAPIKeyRoutes(
			externalV1,
			deps.DB,
			deps.AccountService,
			deps.FileService,
			deps.ContentExtractor,
			deps.QuotaService,
			deps.OrganizationService,
			deps.LLMClient,
			deps.ToolEngine,
			deps.ToolManager,
			deps.MemoryService,
			deps.GraphFlowService,
			deps.PromptResolver,
			deps.DataSourceService,
			deps.KnowledgeService,
			deps.ResourcePermission,
			deps.WorkflowEngineFactory,
		)
	}

	// External API v2 routes (future)
	// externalV2 := r.Group("/api/v2")
}

func validateExternalRouteDeps(deps ExternalRouteDeps) {
	if deps.DB == nil {
		panic("external routes require db")
	}
	if deps.AccountService == nil {
		panic("external routes require account service")
	}
	if deps.FileService == nil {
		panic("external routes require file service")
	}
	if deps.ContentExtractor == nil {
		panic("external routes require content extractor")
	}
	if deps.QuotaService == nil {
		panic("external routes require quota service")
	}
	if deps.OrganizationService == nil {
		panic("external routes require organization service")
	}
	if deps.LLMClient == nil {
		panic("external routes require llm client")
	}
	if deps.ToolEngine == nil {
		panic("external routes require tool engine")
	}
	if deps.ToolManager == nil {
		panic("external routes require tool manager")
	}
	if deps.GraphFlowService == nil {
		panic("external routes require graph flow service")
	}
	if deps.PromptResolver == nil {
		panic("external routes require prompt resolver")
	}
	if deps.WorkflowEngineFactory == nil {
		panic("external routes require workflow engine factory")
	}
}
