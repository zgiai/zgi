package service

import (
	"context"
	"errors"

	"github.com/zgiai/zgi/api/internal/dto"
	workspace_model "github.com/zgiai/zgi/api/internal/modules/workspace/model"
	"gorm.io/gorm"
)

// AssetMover moves one asset type the workspace module does not own, such
// as DigitalHumans. The owning module implements it and registers it, so the
// move framework stays the single place for targets, permissions, locking,
// preview, the transaction, and audit.
type AssetMover interface {
	// AssetType is the type name in move requests.
	AssetType() string
	// Permission is the workspace permission needed in both workspaces.
	Permission() workspace_model.WorkspacePermissionCode
	// SourceWorkspace returns the asset's current workspace, or
	// gorm.ErrRecordNotFound.
	SourceWorkspace(ctx context.Context, db *gorm.DB, organizationID, id string) (string, error)
	// Lock locks the asset's row for the move transaction.
	Lock(ctx context.Context, tx *gorm.DB, organizationID, id string) error
	// Preview adds the asset's own blockers and warnings. The framework has
	// already checked that the asset exists, its source workspace, and the
	// target.
	Preview(ctx context.Context, db *gorm.DB, organizationID, id string, item *dto.WorkspaceAssetMovePreviewItem) error
	// Move moves a previewed, movable item inside the move transaction.
	Move(ctx context.Context, tx *gorm.DB, organizationID, accountID string, item dto.WorkspaceAssetMovePreviewItem) error
}

// RegisterAssetMover adds an asset type to workspace moves.
func (s *WorkspaceAssetMoveService) RegisterAssetMover(mover AssetMover) *WorkspaceAssetMoveService {
	if s == nil || mover == nil || mover.AssetType() == "" {
		return s
	}
	if s.movers == nil {
		s.movers = map[string]AssetMover{}
	}
	s.movers[mover.AssetType()] = mover
	return s
}

func (s *WorkspaceAssetMoveService) mover(assetType string) AssetMover {
	if s == nil || s.movers == nil {
		return nil
	}
	return s.movers[assetType]
}

// assetMovePermission returns the move permission of an asset type.
func (s *WorkspaceAssetMoveService) assetMovePermission(assetType string) (workspace_model.WorkspacePermissionCode, bool) {
	if mover := s.mover(assetType); mover != nil {
		return mover.Permission(), true
	}
	return assetMovePermissionForType(assetType)
}

// previewWithMover runs the checks every asset shares, then the mover's own.
func (s *WorkspaceAssetMoveService) previewWithMover(ctx context.Context, db *gorm.DB, organizationID string, mover AssetMover, id string, item *dto.WorkspaceAssetMovePreviewItem) error {
	workspaceID, err := mover.SourceWorkspace(ctx, db, organizationID, id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		item.Blockers = append(item.Blockers, mover.AssetType()+" not found")
		return nil
	}
	if err != nil {
		return err
	}
	if err := s.attachAndCheckSourceWorkspace(ctx, db, organizationID, workspaceID, item); err != nil {
		return err
	}
	if blockIfAlreadyInTargetWorkspace(item) || len(item.Blockers) > 0 {
		return nil
	}
	return mover.Preview(ctx, db, organizationID, id, item)
}
