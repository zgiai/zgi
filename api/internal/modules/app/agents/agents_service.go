package agents

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/agentbindings"
	runtimeservice "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/service"
	"github.com/zgiai/zgi/api/internal/modules/agentmemory"
	datasetservice "github.com/zgiai/zgi/api/internal/modules/dataset/service"
	datasourceservice "github.com/zgiai/zgi/api/internal/modules/datasource/service"
	"github.com/zgiai/zgi/api/internal/modules/integrations"
	channelrepo "github.com/zgiai/zgi/api/internal/modules/llm/channel/repository"
	llmclient "github.com/zgiai/zgi/api/internal/modules/llm/client"
	llmdefaultservice "github.com/zgiai/zgi/api/internal/modules/llm/defaultmodel/service"
	llmmodelrepo "github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/repository"
	llmmodelservice "github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/service"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	"gorm.io/gorm"
)

type AgentsService = interfaces.AgentsService

var errCurrentOrganizationNotFound = errors.New("current organization not found")

type agentModelEligibility interface {
	ListAvailable(ctx context.Context, organizationID uuid.UUID, provider string, useCase string) ([]*llmmodelservice.AvailableModel, error)
}

type IntegrationActionCatalog interface {
	HasAction(integrationID, actionID string) bool
	ActionDetail(integrationID, actionID string) (integrations.ActionDefinition, bool)
	Actions(integrationID string) []integrations.ActionDefinition
}

type agentsService struct {
	agentsRepo                AgentsRepository
	accountService            interfaces.AccountService
	tenantService             interfaces.WorkspaceManagementService
	workflowService           interfaces.WorkflowService
	chatRuntimeService        runtimeservice.Service
	agentMemoryService        *agentmemory.Service
	dataSourceService         datasourceservice.DataSourceService
	knowledgeRetrievalService *datasetservice.KnowledgeRetrievalService
	resourcePermissionService interfaces.ResourcePermissionService
	enterpriseService         interfaces.OrganizationService
	quotaService              interfaces.QuotaService
	fileService               interfaces.FileService
	llmClient                 llmclient.LLMClient
	defaultModelResolver      llmdefaultservice.DefaultModelResolver
	agentBindings             *agentbindings.Repository
	agentModels               agentModelEligibility
	integrationActions        IntegrationActionCatalog
	integrationActionPolicies integrations.ActionPolicyResolver
	integrationConnections    integrations.ConnectionRepository
	integrationAccess         *integrations.DefaultConnectionAccessService
	db                        *gorm.DB
}

func NewAgentsService(
	agentsRepo AgentsRepository,
	accountService interfaces.AccountService,
	tenantService interfaces.WorkspaceManagementService,
	workflowService interfaces.WorkflowService,
	chatRuntimeService runtimeservice.Service,
	agentMemoryService *agentmemory.Service,
	dataSourceService datasourceservice.DataSourceService,
	knowledgeRetrievalService *datasetservice.KnowledgeRetrievalService,
	resourcePermissionService interfaces.ResourcePermissionService,
	enterpriseService interfaces.OrganizationService,
	quotaService interfaces.QuotaService,
	fileService interfaces.FileService,
	llmClient llmclient.LLMClient,
	defaultModelResolver llmdefaultservice.DefaultModelResolver,
	db *gorm.DB,
	integrationActionCatalogs ...IntegrationActionCatalog,
) AgentsService {
	var agentModels agentModelEligibility
	if db != nil {
		agentModels = llmmodelservice.NewAvailableModelsService(
			llmmodelrepo.NewModelRepository(db),
			llmmodelrepo.NewModelConfigRepository(db),
			llmmodelrepo.NewCustomModelRepository(db),
			channelrepo.NewTenantRouteRepository(db),
		)
	}
	var integrationActions IntegrationActionCatalog
	if len(integrationActionCatalogs) > 0 {
		integrationActions = integrationActionCatalogs[0]
	}
	var integrationConnections integrations.ConnectionRepository
	var integrationAccess *integrations.DefaultConnectionAccessService
	var integrationActionPolicies integrations.ActionPolicyResolver
	if db != nil {
		integrationConnections = integrations.NewGormConnectionRepository(db)
		integrationAccess = integrations.NewConnectionAccessService(integrationConnections, integrations.NewGormConnectionGrantRepository(db))
		integrationActionPolicies = integrations.NewActionPolicyService(integrations.NewGormActionPolicyRepository(db), nil)
	}
	return &agentsService{
		agentsRepo:                agentsRepo,
		accountService:            accountService,
		tenantService:             tenantService,
		workflowService:           workflowService,
		chatRuntimeService:        chatRuntimeService,
		agentMemoryService:        agentMemoryService,
		dataSourceService:         dataSourceService,
		knowledgeRetrievalService: knowledgeRetrievalService,
		resourcePermissionService: resourcePermissionService,
		enterpriseService:         enterpriseService,
		quotaService:              quotaService,
		fileService:               fileService,
		llmClient:                 llmClient,
		defaultModelResolver:      defaultModelResolver,
		agentBindings:             agentbindings.NewRepository(db),
		agentModels:               agentModels,
		integrationActions:        integrationActions,
		integrationActionPolicies: integrationActionPolicies,
		integrationConnections:    integrationConnections,
		integrationAccess:         integrationAccess,
		db:                        db,
	}
}
