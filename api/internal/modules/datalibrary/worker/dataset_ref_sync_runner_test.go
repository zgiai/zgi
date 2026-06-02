package worker

import (
	"context"
	"testing"

	"github.com/google/uuid"
	datalibModel "github.com/zgiai/zgi/api/internal/modules/datalibrary/model"
)

func TestDatasetRefSyncRunnerMarksSyncingWhenPayloadMatchesCurrentAsset(t *testing.T) {
	refID := uuid.New()
	assetID := uuid.New()
	syncRunID := uuid.New()
	refStore := &fakeDatasetRefSyncRefStore{
		ref: &datalibModel.KnowledgeBaseAssetRef{
			ID:             refID,
			OrganizationID: "org-1",
			DatasetID:      "dataset-1",
			AssetID:        assetID,
			SyncRunID:      &syncRunID,
		},
	}
	assetStore := &fakeDatasetRefSyncAssetStore{
		asset: &datalibModel.DocumentAsset{
			ID:             assetID,
			OrganizationID: "org-1",
			ProductStatus:  datalibModel.DocumentAssetProductStatusReady,
			VectorStatus:   datalibModel.DocumentAssetVectorStatusReady,
			GenerationNo:   5,
		},
	}
	runner := NewDatasetRefSyncRunner(DatasetRefSyncRunnerDeps{Refs: refStore, Assets: assetStore})

	err := runner.Run(context.Background(), DatasetRefSyncPayload{
		RefID:        refID.String(),
		AssetID:      assetID.String(),
		DatasetID:    "dataset-1",
		GenerationNo: 5,
		SyncRunID:    syncRunID.String(),
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if refStore.markSyncingID != refID || refStore.markSyncingRunID != syncRunID || refStore.failedCode != "" {
		t.Fatalf("syncing_id=%s sync_run=%s failed=%s", refStore.markSyncingID, refStore.markSyncingRunID, refStore.failedCode)
	}
}

func TestDatasetRefSyncRunnerSkipsStaleSyncRun(t *testing.T) {
	refID := uuid.New()
	assetID := uuid.New()
	currentSyncRunID := uuid.New()
	staleSyncRunID := uuid.New()
	refStore := &fakeDatasetRefSyncRefStore{
		ref: &datalibModel.KnowledgeBaseAssetRef{
			ID:             refID,
			OrganizationID: "org-1",
			DatasetID:      "dataset-1",
			AssetID:        assetID,
			SyncRunID:      &currentSyncRunID,
		},
	}
	runner := NewDatasetRefSyncRunner(DatasetRefSyncRunnerDeps{Refs: refStore, Assets: &fakeDatasetRefSyncAssetStore{}})

	err := runner.Run(context.Background(), DatasetRefSyncPayload{
		RefID:        refID.String(),
		AssetID:      assetID.String(),
		DatasetID:    "dataset-1",
		GenerationNo: 5,
		SyncRunID:    staleSyncRunID.String(),
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if refStore.markSyncingID != uuid.Nil || refStore.failedCode != "" {
		t.Fatalf("unexpected writes syncing=%s failed=%s", refStore.markSyncingID, refStore.failedCode)
	}
}

func TestDatasetRefSyncRunnerMarksFailedWhenAssetNotReady(t *testing.T) {
	refID := uuid.New()
	assetID := uuid.New()
	syncRunID := uuid.New()
	refStore := &fakeDatasetRefSyncRefStore{
		ref: &datalibModel.KnowledgeBaseAssetRef{
			ID:             refID,
			OrganizationID: "org-1",
			DatasetID:      "dataset-1",
			AssetID:        assetID,
			SyncRunID:      &syncRunID,
		},
	}
	assetStore := &fakeDatasetRefSyncAssetStore{
		asset: &datalibModel.DocumentAsset{
			ID:             assetID,
			OrganizationID: "org-1",
			ProductStatus:  datalibModel.DocumentAssetProductStatusGenerating,
			VectorStatus:   datalibModel.DocumentAssetVectorStatusIndexing,
			GenerationNo:   5,
		},
	}
	runner := NewDatasetRefSyncRunner(DatasetRefSyncRunnerDeps{Refs: refStore, Assets: assetStore})

	err := runner.Run(context.Background(), DatasetRefSyncPayload{
		RefID:        refID.String(),
		AssetID:      assetID.String(),
		DatasetID:    "dataset-1",
		GenerationNo: 5,
		SyncRunID:    syncRunID.String(),
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if refStore.failedCode != "asset_not_ready" || refStore.markSyncingID != uuid.Nil {
		t.Fatalf("failed=%s syncing=%s", refStore.failedCode, refStore.markSyncingID)
	}
}

type fakeDatasetRefSyncRefStore struct {
	ref              *datalibModel.KnowledgeBaseAssetRef
	markSyncingID    uuid.UUID
	markSyncingRunID uuid.UUID
	failedCode       string
	failedMessage    string
}

func (f *fakeDatasetRefSyncRefStore) GetByID(ctx context.Context, id uuid.UUID) (*datalibModel.KnowledgeBaseAssetRef, error) {
	if f.ref != nil && f.ref.ID == id {
		return f.ref, nil
	}
	return nil, nil
}

func (f *fakeDatasetRefSyncRefStore) MarkSyncing(ctx context.Context, organizationID string, id uuid.UUID, syncRunID uuid.UUID) (*datalibModel.KnowledgeBaseAssetRef, error) {
	f.markSyncingID = id
	f.markSyncingRunID = syncRunID
	return f.ref, nil
}

func (f *fakeDatasetRefSyncRefStore) MarkFailed(ctx context.Context, organizationID string, id uuid.UUID, syncRunID uuid.UUID, errorCode, errorMessage string) (*datalibModel.KnowledgeBaseAssetRef, error) {
	f.failedCode = errorCode
	f.failedMessage = errorMessage
	return f.ref, nil
}

type fakeDatasetRefSyncAssetStore struct {
	asset *datalibModel.DocumentAsset
}

func (f *fakeDatasetRefSyncAssetStore) GetAssetByID(ctx context.Context, id uuid.UUID) (*datalibModel.DocumentAsset, error) {
	if f.asset != nil && f.asset.ID == id {
		return f.asset, nil
	}
	return nil, nil
}
