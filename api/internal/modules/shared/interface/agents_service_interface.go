package interfaces

import (
	"context"

	"github.com/zgiai/ginext/internal/dto"
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
	UpdateWebAppStatus(ctx context.Context, agentID string, req dto.UpdateWebAppStatusRequest) (*dto.WebAppStatusResponse, error)
	DeleteAgent(ctx context.Context, agentID string) error
}
