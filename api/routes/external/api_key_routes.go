package external

import (
	"context"

	"github.com/gin-gonic/gin"
	runtimerepo "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/repository"
	runtimeservice "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/service"
	"github.com/zgiai/zgi/api/internal/modules/agentmemory"
	"github.com/zgiai/zgi/api/internal/modules/app/agents"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/diagnosis"
	workflow_file "github.com/zgiai/zgi/api/internal/modules/app/workflow/file"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/graph_engine"
	"github.com/zgiai/zgi/api/internal/modules/dataset/graphflow"
	datasetservice "github.com/zgiai/zgi/api/internal/modules/dataset/service"
	datasourceservice "github.com/zgiai/zgi/api/internal/modules/datasource/service"
	"github.com/zgiai/zgi/api/internal/modules/llm/client"
	llmdefaultservice "github.com/zgiai/zgi/api/internal/modules/llm/defaultmodel/service"
	"github.com/zgiai/zgi/api/internal/modules/memory"
	promptservice "github.com/zgiai/zgi/api/internal/modules/prompts/service"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	"github.com/zgiai/zgi/api/internal/modules/tools"
	"github.com/zgiai/zgi/api/internal/util"
	"github.com/zgiai/zgi/api/middleware"
	"gorm.io/gorm"
)

// RegisterAPIKeyRoutes registers external API routes with API key authentication
func RegisterAPIKeyRoutes(r *gin.RouterGroup, db *gorm.DB, accountService interfaces.AccountService, fileService interfaces.FileService, contentExtractor workflow_file.ContentExtractor, quotaService interfaces.QuotaService, enterpriseService interfaces.OrganizationService, llmClient client.LLMClient, toolEngine *tools.ToolEngine, toolManager *tools.ToolManager, memoryService *memory.Service, graphFlowService *graphflow.Service, promptResolver promptservice.PromptService, dataSourceService datasourceservice.DataSourceService, knowledgeRetrievalService *datasetservice.KnowledgeRetrievalService, resourcePermissionService interfaces.ResourcePermissionService, engineFactory *graph_engine.EngineFactory) {
	// Create repositories
	workflowRepo := workflow.NewWorkflowRepository(db)
	workflowRunLogRepo := workflow.NewWorkflowRunLogRepository(db)
	workflowNodeRuntimeLogRepo := workflow.NewWorkflowNodeRuntimeLogRepository(db)
	agentsRepo := agents.NewAgentsRepository(db)

	// Create workflow service for external API calls with ContentExtractor, QuotaService, EnterpriseService, and LLMClient
	workflowService := workflow.NewWorkflowServiceWithContentExtractor(
		workflowRepo,
		agentsRepo,
		workflowRunLogRepo,
		workflowNodeRuntimeLogRepo,
		accountService,
		fileService,
		contentExtractor,
		quotaService,
		enterpriseService,
		llmClient,
		toolEngine,
		graphFlowService,
		promptResolver,
		nil,
		engineFactory,
	)

	// Create user migration service
	userMigrationService := workflow.NewUserMigrationServiceFromDB()

	// Create internal workflow handler to reuse existing logic
	internalWorkflowHandler := workflow.NewWorkflowHandler(workflowService, accountService, fileService, userMigrationService, enterpriseService)
	if llmClientTyped, ok := llmClient.(client.LLMClient); ok {
		internalWorkflowHandler.SetDiagnoser(diagnosis.NewDiagnoser(context.Background(), llmClientTyped))
	}

	// Create external workflow handler (kept for other endpoints)
	externalWorkflowHandler := NewExternalWorkflowHandler(workflowService, fileService, contentExtractor, enterpriseService, quotaService, db)

	agentMemoryService := agentmemory.NewService(db)
	chatRuntimeService := runtimeservice.NewServiceWithSkillRuntime(
		runtimerepo.NewRepositories(db),
		llmClient,
		nil,
		runtimeservice.NewDatabaseModelSpecResolver(db),
		fileService,
		contentExtractor,
		enterpriseService,
		newExternalSkillRuntimeWithSandbox(toolEngine, toolManager, fileService, enterpriseService),
		memoryService,
		agentMemoryService,
	)
	var defaultModelResolver llmdefaultservice.DefaultModelResolver
	if graphFlowService != nil {
		defaultModelResolver = graphFlowService.DefaultModelSvc
	}
	agentService := agents.NewAgentsService(agentsRepo, accountService, nil, workflowService, chatRuntimeService, agentMemoryService, dataSourceService, knowledgeRetrievalService, resourcePermissionService, enterpriseService, quotaService, fileService, llmClient, defaultModelResolver, db)
	agentHandler := agents.NewAgentsHandler(agentService, nil, accountService, enterpriseService, db, chatRuntimeService)
	agentHandler.SetFileService(fileService)

	externalGroup := r.Group("/v1")
	externalGroup.Use(middleware.APIKeyAuthMiddleware(db))
	externalGroup.Use(middleware.APIKeyUsageLoggingMiddleware(db)) // Log API key usage
	{
		// Workflow execution endpoint: delegate to internal handler RunPublishedWorkflow
		externalGroup.POST("/workflows/run", func(c *gin.Context) {
			// Inject required params/context for internal handler
			if v, exists := c.Get("api_key_info"); exists {
				if keyInfo, ok := v.(*middleware.APIKeyInfo); ok {
					// Set tenant/account context expected by internal handler
					util.SetWorkspaceScopeCompat(c, keyInfo.TenantID.String())
					// External API calls use API Key ID as account_id for traceability
					c.Set("account_id", keyInfo.ID.String())
					// Set invoke_from to mark this as external API call
					c.Set("invoke_from", string(workflow.InvokeFromExternalAPI))
					c.Set("created_from", "external-api")
					c.Set("created_by_role", "end_user")
					// Inject URL param agent_id for internal handler
					c.Params = append(c.Params, gin.Param{Key: "agent_id", Value: keyInfo.AgentID.String()})
				}
			}
			internalWorkflowHandler.RunPublishedWorkflow(c)
		})
		externalGroup.POST("/workflows/:workflow_id/run", externalWorkflowHandler.RunSpecificWorkflow)

		// Workflow run status endpoints
		externalGroup.GET("/workflows/runs/:run_id", externalWorkflowHandler.GetWorkflowRunDetail)

		externalGroup.POST("/workflows/tasks/:task_id/stop", externalWorkflowHandler.StopWorkflowTask)

		externalGroup.POST("/files/upload", externalWorkflowHandler.UploadFile)

		externalGroup.GET("/info", externalWorkflowHandler.GetAppInfo)
		externalGroup.GET("/parameters", externalWorkflowHandler.GetAppParameters)

		externalGroup.POST("/chat-workflows/run", func(c *gin.Context) {
			if v, exists := c.Get("api_key_info"); exists {
				if keyInfo, ok := v.(*middleware.APIKeyInfo); ok {
					util.SetWorkspaceScopeCompat(c, keyInfo.TenantID.String())
					c.Set("account_id", keyInfo.ID.String())
					c.Set("invoke_from", string(workflow.InvokeFromExternalAPI))
					c.Set("created_from", "external-api")
					c.Set("created_by_role", "end_user")
					c.Params = append(c.Params, gin.Param{Key: "agent_id", Value: keyInfo.AgentID.String()})
				}
			}
			internalWorkflowHandler.RunAdvancedChatWorkflow(c)
		})

		externalGroup.POST("/agents/chat", func(c *gin.Context) {
			if v, exists := c.Get("api_key_info"); exists {
				if keyInfo, ok := v.(*middleware.APIKeyInfo); ok {
					util.SetWorkspaceScopeCompat(c, keyInfo.TenantID.String())
					if enterpriseService != nil {
						if org, err := enterpriseService.GetOrganizationByWorkspaceID(c.Request.Context(), keyInfo.TenantID.String()); err == nil && org != nil && org.ID != "" {
							util.SetOrganizationID(c, org.ID)
						} else {
							util.SetOrganizationID(c, keyInfo.TenantID.String())
						}
					} else {
						util.SetOrganizationID(c, keyInfo.TenantID.String())
					}
					c.Set("account_id", keyInfo.ID.String())
					c.Set("agent_id", keyInfo.AgentID.String())
				}
			}
			agentHandler.ChatAPIKeyAgent(c)
		})
	}
}
