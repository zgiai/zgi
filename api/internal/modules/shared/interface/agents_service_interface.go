package interfaces

import (
	"context"

	"github.com/zgiai/zgi/api/internal/dto"
)

// AgentService defines the interface for agent-related operations
type AgentsService interface {
	GetAgentsListWithPermissions(ctx context.Context, accountID string, req dto.GetAgentsListRequest) (*dto.AgentsListResponse, error)
	GetRunnableWebApps(ctx context.Context, accountID string, req dto.GetRunnableWebAppsRequest) (*dto.RunnableWebAppsResponse, error)
	CreateAgent(ctx context.Context, tenantID string, req interface{}, accountID string) (interface{}, error)
	GetAgent(ctx context.Context, agentID string) (interface{}, error)
	UpdateAgent(ctx context.Context, agentID string, req interface{}) (interface{}, error)
	GetAgentConfig(ctx context.Context, agentID, accountID string) (*dto.AgentConfigResponse, error)
	GetAgentDraftRuntimeConfig(ctx context.Context, agentID, accountID string) (*dto.AgentDraftRuntimeConfigResponse, error)
	UpdateAgentConfig(ctx context.Context, agentID, accountID string, req dto.AgentConfigRequest) (*dto.AgentConfigResponse, error)
	ListAgentSkillCandidates(ctx context.Context, agentID, accountID string, req dto.AgentSkillCandidatesRequest) (*dto.AgentSkillCandidatesResponse, error)
	ListAgentKnowledgeCandidates(ctx context.Context, agentID, accountID string, req dto.AgentKnowledgeCandidatesRequest) (*dto.AgentKnowledgeCandidatesResponse, error)
	ListAgentDatabaseCandidates(ctx context.Context, agentID, accountID string, req dto.AgentDatabaseCandidatesRequest) (*dto.AgentDatabaseCandidatesResponse, error)
	ListAgentDatabaseTables(ctx context.Context, agentID, accountID string, req dto.AgentDatabaseTablesRequest) (*dto.AgentDatabaseTablesResponse, error)
	ListAgentWorkflowBindingCandidates(ctx context.Context, agentID, accountID string, req dto.AgentWorkflowBindingCandidatesRequest) (*dto.AgentWorkflowBindingCandidatesResponse, error)
	ListAgentMemorySlots(ctx context.Context, agentID, accountID string) ([]dto.AgentMemorySlotConfig, error)
	ReplaceAgentMemorySlots(ctx context.Context, agentID, accountID string, slots []dto.AgentMemorySlotConfig) ([]dto.AgentMemorySlotConfig, error)
	ListAgentMemoryValues(ctx context.Context, agentID, accountID string) (*dto.AgentMemoryValuesResponse, error)
	UpdateAgentMemoryValue(ctx context.Context, agentID, accountID string, req dto.UpdateAgentMemoryValueRequest) (*dto.AgentMemoryValueResponse, error)
	ClearAgentMemoryValue(ctx context.Context, agentID, accountID, key string) (*dto.AgentMemoryValueResponse, error)
	GenerateAgentSuggestedQuestions(ctx context.Context, agentID, accountID string, req *dto.GenerateAgentSuggestedQuestionsRequest) (*dto.GenerateSuggestedQuestionsResponse, error)
	PublishAgent(ctx context.Context, agentID, accountID string, req dto.PublishAgentRequest) (*dto.PublishAgentResponse, error)
	ListAgentPublishedVersions(ctx context.Context, agentID, accountID string, page, limit int) (*dto.AgentPublishedVersionsResponse, error)
	RollbackAgentPublishedVersion(ctx context.Context, agentID, accountID string, req dto.RollbackAgentPublishedVersionRequest) (*dto.AgentConfigResponse, error)
	GetPublishedAgentWebAppConfig(ctx context.Context, webAppID string) (*dto.AgentWebAppRuntimeConfigResponse, error)
	UpdateWebAppStatus(ctx context.Context, agentID string, req dto.UpdateWebAppStatusRequest) (*dto.WebAppStatusResponse, error)
	DeleteAgent(ctx context.Context, agentID string) error
}
