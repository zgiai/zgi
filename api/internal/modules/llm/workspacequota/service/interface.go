package service

import (
	"context"

	"github.com/zgiai/ginext/internal/modules/llm/workspacequota/dto"
)

// WorkspaceQuotaService defines workspace-subject quota management APIs.
type WorkspaceQuotaService interface {
	ListWorkspaceQuotas(ctx context.Context, organizationID string, req *dto.ListWorkspaceQuotaRequest) (*dto.ListWorkspaceQuotaResponse, error)
	GetWorkspaceQuota(ctx context.Context, organizationID, workspaceID string) (*dto.WorkspaceQuotaResponse, error)
	UpdateWorkspaceQuota(ctx context.Context, organizationID, workspaceID string, req *dto.UpdateWorkspaceQuotaRequest) (*dto.WorkspaceQuotaResponse, error)
}
