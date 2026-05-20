package service

import (
	"context"

	"github.com/zgiai/zgi/api/internal/modules/workspace/model"
)

// WorkspaceService interface defines the contract for workspace operations
type WorkspaceService interface {
	CreateWorkspace(ctx context.Context, name string, ownerAccountID string) error
	UpdateWorkspace(ctx context.Context, tenantID, name string, status *model.WorkspaceStatus, userID string, hasAdminPermission bool) (*model.WorkspaceUpdateResponse, error)
	GetWorkspaceStatistics(ctx context.Context, tenantID string) (*model.WorkspaceInfo, error)
}
