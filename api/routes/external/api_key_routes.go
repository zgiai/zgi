package external

import (
	"context"

	"github.com/gin-gonic/gin"
	"github.com/zgiai/ginext/internal/modules/app/agents"
	"github.com/zgiai/ginext/internal/modules/app/workflow"
	"github.com/zgiai/ginext/internal/modules/app/workflow/diagnosis"
	workflow_file "github.com/zgiai/ginext/internal/modules/app/workflow/file"
	"github.com/zgiai/ginext/internal/modules/app/workflow/graph_engine"
	"github.com/zgiai/ginext/internal/modules/dataset/graphflow"
	"github.com/zgiai/ginext/internal/modules/llm/client"
	promptservice "github.com/zgiai/ginext/internal/modules/prompts/service"
	interfaces "github.com/zgiai/ginext/internal/modules/shared/interface"
	"github.com/zgiai/ginext/internal/util"
	"github.com/zgiai/ginext/middleware"
	"gorm.io/gorm"
)

// RegisterAPIKeyRoutes registers external API routes with API key authentication
func RegisterAPIKeyRoutes(r *gin.RouterGroup, db *gorm.DB, accountService interfaces.AccountService, fileService interfaces.FileService, contentExtractor workflow_file.ContentExtractor, quotaService interfaces.QuotaService, enterpriseService interfaces.OrganizationService, llmClient interface{}, toolEngine interface{}, graphFlowService *graphflow.Service, promptResolver promptservice.PromptService, engineFactory *graph_engine.EngineFactory) {
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
	}
}
