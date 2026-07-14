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
		db:                        db,
	}
}
