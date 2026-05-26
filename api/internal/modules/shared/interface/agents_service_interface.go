package interfaces

import (
	"context"

	"github.com/zgiai/zgi/api/internal/dto"
)

// AgentService defines the interface for agent-related operations
type AgentsService interface {
	GetAgentsList(ctx context.Context, accountID, tenantID string, req interface{}) (interface{}, error)
	GetAgentsListMultipleTenants(ctx context.Context, accountID string, tenantIDs []string, req interface{}) (interface{}, error)
	GetInternalAgentsList(ctx context.Context, accountID string, tenantIDs []string, req interface{}) (interface{}, error)
	GetAgentsListWithPermissions(ctx context.Context, accountID string, req dto.GetAgentsListRequest) (*dto.AgentsListResponse, error)
	GetRunnableWebApps(ctx context.Context, accountID string, req dto.GetRunnableWebAppsRequest) (*dto.RunnableWebAppsResponse, error)
	CreateAgent(ctx context.Context, tenantID string, req interface{}, accountID string) (interface{}, error)
	GetAgent(ctx context.Context, agentID string) (interface{}, error)
	UpdateAgent(ctx context.Context, agentID string, req interface{}) (interface{}, error)
	GetAgentConfig(ctx context.Context, agentID, accountID string) (*dto.AgentConfigResponse, error)
	UpdateAgentConfig(ctx context.Context, agentID, accountID string, req dto.AgentConfigRequest) (*dto.AgentConfigResponse, error)
	GenerateAgentSuggestedQuestions(ctx context.Context, agentID, accountID string, req *dto.GenerateAgentSuggestedQuestionsRequest) (*dto.GenerateSuggestedQuestionsResponse, error)
	PublishAgent(ctx context.Context, agentID, accountID string, req dto.PublishAgentRequest) (*dto.PublishAgentResponse, error)
	ListAgentPublishedVersions(ctx context.Context, agentID, accountID string, page, limit int) (*dto.AgentPublishedVersionsResponse, error)
	RollbackAgentPublishedVersion(ctx context.Context, agentID, accountID string, req dto.RollbackAgentPublishedVersionRequest) (*dto.AgentConfigResponse, error)
	GetPublishedAgentWebAppConfig(ctx context.Context, webAppID string) (*dto.AgentWebAppRuntimeConfigResponse, error)
	UpdateWebAppStatus(ctx context.Context, agentID string, req dto.UpdateWebAppStatusRequest) (*dto.WebAppStatusResponse, error)
	DeleteAgent(ctx context.Context, agentID string) error
}
