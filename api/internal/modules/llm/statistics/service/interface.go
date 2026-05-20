package service

import (
	"context"

	"github.com/zgiai/ginext/internal/modules/llm/statistics/dto"
)

// StatisticsService defines the interface for statistics operations
type StatisticsService interface {
	GetModelUsage(ctx context.Context, organizationID string, req *dto.ModelUsageRequest) (*dto.ModelUsageResponse, error)
	GetWorkspaceQuota(ctx context.Context, organizationID string, req *dto.WorkspaceQuotaRequest) (*dto.WorkspaceQuotaResponse, error)
}
