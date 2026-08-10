package repository

import (
	"context"

	"github.com/zgiai/zgi/api/internal/modules/llm/statistics/dto"
)

// StatisticsRepository defines the interface for statistics operations
type StatisticsRepository interface {
	GetModelUsage(ctx context.Context, organizationID string, req *dto.ModelUsageRequest) (*dto.ModelUsageResponse, error)
	GetInvocationLog(ctx context.Context, organizationID string, req *dto.InvocationLogRequest) (*dto.InvocationLogResponse, error)
	GetInvocationContentSettings(ctx context.Context, organizationID string) (bool, error)
	UpdateInvocationContentSettings(ctx context.Context, organizationID string, enabled bool) error
	GetInvocationContent(ctx context.Context, organizationID, accountID, invocationID string) (*dto.InvocationContentDetail, error)
	GetWorkspaceQuota(ctx context.Context, organizationID string, req *dto.WorkspaceQuotaRequest) (*dto.WorkspaceQuotaResponse, error)
}
