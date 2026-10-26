package external

import (
	"context"
	"github.com/zgiai/zgi/api/internal/modules/digitalhuman"

	"github.com/gin-gonic/gin"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	runtimerepo "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/repository"
	runtimeservice "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/service"
	"github.com/zgiai/zgi/api/internal/modules/agentmemory"
	"github.com/zgiai/zgi/api/internal/modules/app/agents"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/diagnosis"
	workflow_file "github.com/zgiai/zgi/api/internal/modules/app/workflow/file"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/graph_engine"
	connectordispatch "github.com/zgiai/zgi/api/internal/modules/connectors/dispatch"
	connectorexecution "github.com/zgiai/zgi/api/internal/modules/connectors/execution"
	connectorruntimeauth "github.com/zgiai/zgi/api/internal/modules/connectors/runtimeauth"
	"github.com/zgiai/zgi/api/internal/modules/dataset/graphflow"
	datasetservice "github.com/zgiai/zgi/api/internal/modules/dataset/service"
	datasourceservice "github.com/zgiai/zgi/api/internal/modules/datasource/service"
	"github.com/zgiai/zgi/api/internal/modules/llm/client"
	llmdefaultservice "github.com/zgiai/zgi/api/internal/modules/llm/defaultmodel/service"
	"github.com/zgiai/zgi/api/internal/modules/memory"
	promptservice "github.com/zgiai/zgi/api/internal/modules/prompts/service"
	"github.com/zgiai/zgi/api/internal/modules/relationshipmemory"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	"github.com/zgiai/zgi/api/internal/modules/skills"
	"github.com/zgiai/zgi/api/internal/modules/tools"
	"github.com/zgiai/zgi/api/internal/modules/zgimarket"
	"github.com/zgiai/zgi/api/internal/util"
	"github.com/zgiai/zgi/api/middleware"
	"github.com/zgiai/zgi/api/pkg/response"
	"gorm.io/gorm"
)

// RegisterAPIKeyRoutes registers external API routes with API key authentication
func RegisterAPIKeyRoutes(r *gin.RouterGroup, db *gorm.DB, accountService interfaces.AccountService, fileService interfaces.FileService, contentExtractor workflow_file.ContentExtractor, quotaService interfaces.QuotaService, enterpriseService interfaces.OrganizationService, llmClient client.LLMClient, toolEngine *tools.ToolEngine, toolManager *tools.ToolManager, memoryService *memory.Service, graphFlowService *graphflow.Service, promptResolver promptservice.PromptService, dataSourceService datasourceservice.DataSourceService, knowledgeRetrievalService *datasetservice.KnowledgeRetrievalService, resourcePermissionService interfaces.ResourcePermissionService, engineFactory *graph_engine.EngineFactory, manifestResolver skills.ToolGovernanceManifestResolver, connectorDispatcher connectordispatch.Dispatcher, connectorRuntime connectorruntimeauth.AgentRuntimeResolver, connectorConfirmationStore connectorexecution.ConfirmationContinuationStore, connectorCatalog connectorexecution.DefinitionCatalog, extras ...APIKeyRouteExtras) {
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
	workflowService.SetConnectorCatalog(connectorCatalog)

	// Create user migration service
	userMigrationService := workflow.NewUserMigrationServiceFromDB()

	// Create internal workflow handler to reuse existing logic
	internalWorkflowHandler := workflow.NewWorkflowHandler(workflowService, accountService, fileService, userMigrationService, enterpriseService)
	if llmClientTyped, ok := llmClient.(client.LLMClient); ok {
		internalWorkflowHandler.SetDiagnoser(diagnosis.NewDiagnoser(context.Background(), llmClientTyped))
	}

	agentMemoryService := agentmemory.NewService(db)
	userRelationshipMemory := relationshipmemory.NewAgentAdapter(db)
	var runtimeAgentMemory any = userRelationshipMemory
	chatRuntimeService := runtimeservice.NewServiceWithSkillRuntime(
		runtimerepo.NewRepositories(db),
		llmClient,
		nil,
		runtimeservice.NewDatabaseModelSpecResolver(db),
		fileService,
		contentExtractor,
		enterpriseService,
		newExternalSkillRuntimeWithSandbox(toolEngine, toolManager, fileService, enterpriseService, manifestResolver),
		memoryService,
		runtimeAgentMemory,
		connectorRuntime, connectorConfirmationStore,
		zgimarket.RuntimeResolver(),
	)
	var defaultModelResolver llmdefaultservice.DefaultModelResolver
	if graphFlowService != nil {
		defaultModelResolver = graphFlowService.DefaultModelSvc
	}
	agentService := agents.NewAgentsService(agentsRepo, accountService, nil, workflowService, chatRuntimeService, agentMemoryService, dataSourceService, knowledgeRetrievalService, resourcePermissionService, enterpriseService, quotaService, fileService, llmClient, defaultModelResolver, db)
	agentHandler := agents.NewAgentsHandler(agentService, nil, accountService, enterpriseService, db, chatRuntimeService)
	agentHandler.SetFileService(fileService)
	agentHandler.SetWorkflowContinuationRunner(internalWorkflowHandler)

	// Create external workflow handler after the Agent runtime service so the
	// compatibility metadata endpoints can return the real published app data.
	externalWorkflowHandler := NewExternalWorkflowHandler(workflowService, fileService, contentExtractor, enterpriseService, quotaService, db, agentsRepo, agentService)

	externalGroup := r.Group("/v1")
	externalGroup.Use(middleware.APIKeyAuthMiddleware(db))
	externalGroup.Use(middleware.APIKeyUsageLoggingMiddleware(db)) // Log API key usage
	withAgentAPIKeyContext := func(handler gin.HandlerFunc) gin.HandlerFunc {
		return func(c *gin.Context) {
			value, exists := c.Get("api_key_info")
			keyInfo, ok := value.(*middleware.APIKeyInfo)
			if !exists || !ok || keyInfo == nil {
				response.Fail(c, response.ErrUnauthorized)
				c.Abort()
				return
			}
			util.SetWorkspaceScopeCompat(c, keyInfo.TenantID.String())
			c.Set("workspace_id", keyInfo.TenantID.String())
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
			c.Set("invoke_from", runtimemodel.ConversationSourceExternalAPI)
			handler(c)
		}
	}
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
		externalGroup.GET("/workflows/runs/:run_id/events", func(c *gin.Context) {
			if v, exists := c.Get("api_key_info"); exists {
				if keyInfo, ok := v.(*middleware.APIKeyInfo); ok {
					util.SetWorkspaceScopeCompat(c, keyInfo.TenantID.String())
					c.Set("account_id", keyInfo.ID.String())
					c.Params = append(c.Params, gin.Param{Key: "workflow_run_id", Value: c.Param("run_id")})
				}
			}
			internalWorkflowHandler.GetWorkflowRunEvents(c)
		})

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

		externalGroup.POST("/agents/chat", withAgentAPIKeyContext(agentHandler.ChatAPIKeyAgent))
		externalGroup.GET("/agents/config", withAgentAPIKeyContext(agentHandler.GetAPIKeyAgentRuntimeConfig))
		externalGroup.GET("/agents/memory", withAgentAPIKeyContext(agentHandler.GetAPIKeyAgentMemory))
		externalGroup.GET("/agents/memory/export", withAgentAPIKeyContext(agentHandler.ExportAPIKeyAgentMemory))
		externalGroup.PUT("/agents/memory/:key", withAgentAPIKeyContext(agentHandler.PutAPIKeyAgentMemory))
		externalGroup.DELETE("/agents/memory/:key", withAgentAPIKeyContext(agentHandler.DeleteAPIKeyAgentMemoryValue))
		externalGroup.DELETE("/agents/memory", withAgentAPIKeyContext(agentHandler.DeleteAllAPIKeyAgentMemory))
		externalGroup.POST("/agents/memory/operations/:operation_id/undo", withAgentAPIKeyContext(agentHandler.UndoAPIKeyAgentMemoryOperation))
		externalGroup.GET("/agents/memory/history", withAgentAPIKeyContext(agentHandler.GetAPIKeyAgentMemoryHistory))
		externalGroup.POST("/agents/memory/history/:version_id/revert", withAgentAPIKeyContext(agentHandler.RevertAPIKeyAgentMemoryVersion))
		externalGroup.GET("/agents/memory/archive", withAgentAPIKeyContext(agentHandler.GetAPIKeyAgentMemoryArchive))
		externalGroup.DELETE("/agents/memory/archive/:entry_id", withAgentAPIKeyContext(agentHandler.DeleteAPIKeyAgentMemoryArchiveEntry))
		externalGroup.GET("/agents/memory/settings", withAgentAPIKeyContext(agentHandler.GetAPIKeyAgentMemorySettings))
		externalGroup.PATCH("/agents/memory/settings", withAgentAPIKeyContext(agentHandler.PatchAPIKeyAgentMemorySettings))
		externalGroup.GET("/agents/conversations", withAgentAPIKeyContext(agentHandler.ListAPIKeyAgentRuntimeConversations))
		externalGroup.GET("/agents/conversations/search", withAgentAPIKeyContext(agentHandler.SearchAPIKeyAgentRuntimeConversations))
		externalGroup.GET("/agents/conversations/:conversation_id", withAgentAPIKeyContext(agentHandler.GetAPIKeyAgentRuntimeConversation))
		externalGroup.PATCH("/agents/conversations/:conversation_id", withAgentAPIKeyContext(agentHandler.UpdateAPIKeyAgentRuntimeConversation))
		externalGroup.DELETE("/agents/conversations/:conversation_id", withAgentAPIKeyContext(agentHandler.DeleteAPIKeyAgentRuntimeConversation))
		externalGroup.GET("/agents/conversations/:conversation_id/messages", withAgentAPIKeyContext(agentHandler.ListAPIKeyAgentRuntimeMessages))
		externalGroup.GET("/agents/conversations/:conversation_id/files/:file_id", withAgentAPIKeyContext(agentHandler.GetAPIKeyAgentRuntimeExecutionFile))
		externalGroup.GET("/agents/conversations/:conversation_id/events", withAgentAPIKeyContext(agentHandler.StreamAPIKeyAgentRuntimeEvents))
		externalGroup.POST("/agents/conversations/:conversation_id/stop", withAgentAPIKeyContext(agentHandler.StopAPIKeyAgentRuntimeConversation))
		externalGroup.POST("/agents/messages/:message_id/regenerate", withAgentAPIKeyContext(agentHandler.RegenerateAPIKeyAgentRuntimeMessage))
		externalGroup.POST("/agents/conversations/:conversation_id/messages/:message_id/workflow-continuation", withAgentAPIKeyContext(agentHandler.ContinueAPIKeyAgentRuntimeWorkflowContinuation))
		externalGroup.POST("/agents/conversations/:conversation_id/messages/:message_id/user-input/:request_id/continue", withAgentAPIKeyContext(agentHandler.ContinueAPIKeyAgentRuntimeUserInput))
		externalGroup.POST("/agents/conversations/:conversation_id/messages/:message_id/tool-governance/:correlation_id/continue", withAgentAPIKeyContext(agentHandler.ContinueAPIKeyAgentRuntimeToolGovernance))
		externalGroup.POST("/agents/conversations/:conversation_id/messages/:message_id/token-budget/:request_id/continue", withAgentAPIKeyContext(agentHandler.ContinueAPIKeyAgentRuntimeTokenBudget))
	}

	// Runtime Workspace exposes the same published Agent runtime through one
	// rotatable group credential while keeping the legacy /v1 contract intact.
	runtimeDeps := RuntimeWorkspaceRouteDeps{
		DB:           db,
		FileService:  fileService,
		AgentHandler: agentHandler,
	}
	for _, extra := range extras {
		if extra.DigitalHuman != nil {
			runtimeDeps.DigitalHuman = extra.DigitalHuman
		}
	}
	RegisterRuntimeWorkspaceRoutes(r, runtimeDeps)
}

// APIKeyRouteExtras carries optional collaborators of the external routes:
// the DigitalHuman handler lets Runtime Workspaces serve bound DigitalHumans.
type APIKeyRouteExtras struct {
	DigitalHuman *digitalhuman.ExternalHandler
}
