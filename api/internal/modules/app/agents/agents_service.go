package agents

import (
	"errors"

	runtimeservice "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/service"
	"github.com/zgiai/zgi/api/internal/modules/agentmemory"
	datasetservice "github.com/zgiai/zgi/api/internal/modules/dataset/service"
	datasourceservice "github.com/zgiai/zgi/api/internal/modules/datasource/service"
	llmclient "github.com/zgiai/zgi/api/internal/modules/llm/client"
	llmdefaultservice "github.com/zgiai/zgi/api/internal/modules/llm/defaultmodel/service"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	"gorm.io/gorm"
)

type AgentsService = interfaces.AgentsService

var errCurrentOrganizationNotFound = errors.New("current organization not found")

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
		db:                        db,
	}
}
