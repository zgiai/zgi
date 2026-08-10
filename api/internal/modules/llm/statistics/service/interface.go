package service

import (
	"context"

	"github.com/zgiai/zgi/api/internal/modules/llm/statistics/dto"
)

// StatisticsService defines the interface for statistics operations
type StatisticsService interface {
	GetModelUsage(ctx context.Context, organizationID string, req *dto.ModelUsageRequest) (*dto.ModelUsageResponse, error)
	GetInvocationLog(ctx context.Context, organizationID string, req *dto.InvocationLogRequest) (*dto.InvocationLogResponse, error)
	GetInvocationContentSettings(ctx context.Context, organizationID string) (*dto.InvocationContentSettings, error)
	UpdateInvocationContentSettings(ctx context.Context, organizationID string, req *dto.UpdateInvocationContentSettingsRequest) (*dto.InvocationContentSettings, error)
	PurgeInvocationContent(ctx context.Context, organizationID, accountID string) (*dto.InvocationContentPurgeResult, error)
	GetInvocationContent(ctx context.Context, organizationID, accountID, invocationID string) (*dto.InvocationContentDetail, error)
	GetWorkspaceQuota(ctx context.Context, organizationID string, req *dto.WorkspaceQuotaRequest) (*dto.WorkspaceQuotaResponse, error)
}
