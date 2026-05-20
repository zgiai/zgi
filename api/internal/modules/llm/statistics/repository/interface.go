package repository

import (
	"context"

	"github.com/zgiai/ginext/internal/modules/llm/statistics/dto"
)

// StatisticsRepository defines the interface for statistics operations
type StatisticsRepository interface {
	GetModelUsage(ctx context.Context, organizationID string, req *dto.ModelUsageRequest) (*dto.ModelUsageResponse, error)
	GetWorkspaceQuota(ctx context.Context, organizationID string, req *dto.WorkspaceQuotaRequest) (*dto.WorkspaceQuotaResponse, error)
}
