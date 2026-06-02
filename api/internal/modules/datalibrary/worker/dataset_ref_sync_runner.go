package worker

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	datalibModel "github.com/zgiai/zgi/api/internal/modules/datalibrary/model"
)

type datasetRefSyncRefStore interface {
	GetByID(ctx context.Context, id uuid.UUID) (*datalibModel.KnowledgeBaseAssetRef, error)
	MarkSyncing(ctx context.Context, organizationID string, id uuid.UUID, syncRunID uuid.UUID) (*datalibModel.KnowledgeBaseAssetRef, error)
	MarkFailed(ctx context.Context, organizationID string, id uuid.UUID, syncRunID uuid.UUID, errorCode, errorMessage string) (*datalibModel.KnowledgeBaseAssetRef, error)
}

type datasetRefSyncAssetStore interface {
	GetAssetByID(ctx context.Context, id uuid.UUID) (*datalibModel.DocumentAsset, error)
}

type DatasetRefSyncRunnerDeps struct {
	Refs   datasetRefSyncRefStore
	Assets datasetRefSyncAssetStore
}

type DatasetRefSyncRunner struct {
	refs   datasetRefSyncRefStore
	assets datasetRefSyncAssetStore
}

func NewDatasetRefSyncRunner(deps DatasetRefSyncRunnerDeps) *DatasetRefSyncRunner {
	return &DatasetRefSyncRunner{
		refs:   deps.Refs,
		assets: deps.Assets,
	}
}

func (r *DatasetRefSyncRunner) Run(ctx context.Context, payload DatasetRefSyncPayload) error {
	if r == nil || r.refs == nil || r.assets == nil {
		return fmt.Errorf("dataset ref sync runner is not configured")
	}
	refID, err := uuid.Parse(payload.RefID)
	if err != nil || refID == uuid.Nil {
		return fmt.Errorf("invalid ref_id %q", payload.RefID)
	}
	assetID, err := uuid.Parse(payload.AssetID)
	if err != nil || assetID == uuid.Nil {
		return fmt.Errorf("invalid asset_id %q", payload.AssetID)
	}
	syncRunID, err := uuid.Parse(payload.SyncRunID)
	if err != nil || syncRunID == uuid.Nil {
		return fmt.Errorf("invalid sync_run_id %q", payload.SyncRunID)
	}

	ref, err := r.refs.GetByID(ctx, refID)
	if err != nil {
		return err
	}
	if ref == nil {
		return nil
	}
	if ref.SyncRunID == nil || *ref.SyncRunID != syncRunID {
		return nil
	}
	if ref.AssetID != assetID || ref.DatasetID != payload.DatasetID {
		_, markErr := r.refs.MarkFailed(ctx, ref.OrganizationID, ref.ID, syncRunID, "ref_payload_mismatch", "sync task payload does not match ref")
		return markErr
	}

	asset, err := r.assets.GetAssetByID(ctx, assetID)
	if err != nil {
		return err
	}
	if asset == nil || asset.OrganizationID != ref.OrganizationID {
		_, markErr := r.refs.MarkFailed(ctx, ref.OrganizationID, ref.ID, syncRunID, "asset_not_found", "asset not found")
		return markErr
	}
	if asset.ProductStatus != datalibModel.DocumentAssetProductStatusReady || asset.VectorStatus != datalibModel.DocumentAssetVectorStatusReady {
		_, markErr := r.refs.MarkFailed(ctx, ref.OrganizationID, ref.ID, syncRunID, "asset_not_ready", "asset is not ready for dataset sync")
		return markErr
	}
	if payload.GenerationNo != asset.GenerationNo {
		_, markErr := r.refs.MarkFailed(ctx, ref.OrganizationID, ref.ID, syncRunID, "generation_mismatch", "sync task generation does not match asset current generation")
		return markErr
	}

	_, err = r.refs.MarkSyncing(ctx, ref.OrganizationID, ref.ID, syncRunID)
	return err
}
