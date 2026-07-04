package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/datalibrary/model"
	"github.com/zgiai/zgi/api/internal/modules/datalibrary/repository"
)

func TestKnowledgeBaseAssetRefServiceValidatesRequiredFields(t *testing.T) {
	svc := NewKnowledgeBaseAssetRefService(&fakeKnowledgeBaseAssetRefRepository{})
	ctx := context.Background()
	assetID := uuid.New()
	versionID := uuid.New()

	tests := []struct {
		name string
		item *model.KnowledgeBaseAssetRef
		err  error
	}{
		{
			name: "requires organization",
			item: &model.KnowledgeBaseAssetRef{
				DatasetID: "dataset-1",
				AssetID:   assetID,
				VersionID: &versionID,
			},
			err: ErrOrganizationIDRequired,
		},
		{
			name: "requires dataset",
			item: &model.KnowledgeBaseAssetRef{
				OrganizationID: "org-1",
				AssetID:        assetID,
				VersionID:      &versionID,
			},
			err: ErrDatasetIDRequired,
		},
		{
			name: "requires asset",
			item: &model.KnowledgeBaseAssetRef{
				OrganizationID: "org-1",
				DatasetID:      "dataset-1",
				VersionID:      &versionID,
			},
			err: ErrAssetIDRequired,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := svc.CreateRef(ctx, tt.item); !errors.Is(err, tt.err) {
				t.Fatalf("CreateRef error=%v want=%v", err, tt.err)
			}
		})
	}

	if _, err := svc.GetRefViewByID(ctx, uuid.Nil); !errors.Is(err, ErrKnowledgeBaseAssetRefIDRequired) {
		t.Fatalf("GetRefViewByID error=%v", err)
	}
	if _, _, err := svc.ListRefViews(ctx, repository.KnowledgeBaseAssetRefListFilter{}); !errors.Is(err, ErrOrganizationIDRequired) {
		t.Fatalf("ListRefViews error=%v", err)
	}
	if _, err := svc.FindActiveRefView(ctx, "", "dataset-1", assetID, versionID); !errors.Is(err, ErrOrganizationIDRequired) {
		t.Fatalf("FindActiveRefView organization error=%v", err)
	}
	if _, err := svc.FindActiveRefView(ctx, "org-1", "", assetID, versionID); !errors.Is(err, ErrDatasetIDRequired) {
		t.Fatalf("FindActiveRefView dataset error=%v", err)
	}
	if _, err := svc.FindActiveRefView(ctx, "org-1", "dataset-1", uuid.Nil, versionID); !errors.Is(err, ErrAssetIDRequired) {
		t.Fatalf("FindActiveRefView asset error=%v", err)
	}
	if _, err := svc.DisableRef(ctx, "", uuid.New()); !errors.Is(err, ErrOrganizationIDRequired) {
		t.Fatalf("DisableRef organization error=%v", err)
	}
	if _, err := svc.DisableRef(ctx, "org-1", uuid.Nil); !errors.Is(err, ErrKnowledgeBaseAssetRefIDRequired) {
		t.Fatalf("DisableRef id error=%v", err)
	}
}

func TestKnowledgeBaseAssetRefServiceCreatesReadOnlyView(t *testing.T) {
	now := time.Now()
	assetID := uuid.New()
	versionID := uuid.New()
	chunkSetID := uuid.New()
	vectorID := uuid.New()
	repo := &fakeKnowledgeBaseAssetRefRepository{}
	svc := NewKnowledgeBaseAssetRefService(repo)

	view, err := svc.CreateRef(context.Background(), &model.KnowledgeBaseAssetRef{
		ID:                 uuid.New(),
		OrganizationID:     "org-1",
		DatasetID:          "dataset-1",
		AssetID:            assetID,
		VersionID:          &versionID,
		ChunkArtifactSetID: &chunkSetID,
		VectorArtifactID:   &vectorID,
		Status:             model.KnowledgeBaseAssetRefStatusActive,
		MetadataJSON: map[string]any{
			"source": "data_library",
		},
		CreatedBy: "account-1",
		CreatedAt: now,
		UpdatedAt: now,
	})
	if err != nil {
		t.Fatalf("CreateRef: %v", err)
	}
	if repo.created == nil {
		t.Fatal("expected repository create")
	}
	if view == nil ||
		view.OrganizationID != "org-1" ||
		view.DatasetID != "dataset-1" ||
		view.AssetID != assetID ||
		view.VersionID == nil ||
		*view.VersionID != versionID ||
		view.ChunkArtifactSetID == nil ||
		*view.ChunkArtifactSetID != chunkSetID ||
		view.VectorArtifactID == nil ||
		*view.VectorArtifactID != vectorID ||
		view.MetadataJSON["source"] != "data_library" {
		t.Fatalf("view=%+v", view)
	}
}

func TestKnowledgeBaseAssetRefServiceCreateRefRecordsKnowledgeBaseReuseEvent(t *testing.T) {
	refID := uuid.New()
	assetID := uuid.New()
	versionID := uuid.New()
	chunkSetID := uuid.New()
	vectorID := uuid.New()
	repo := &fakeKnowledgeBaseAssetRefRepository{}
	reuseRepo := &fakeReuseEventRepository{}
	svc := NewKnowledgeBaseAssetRefService(repo, reuseRepo)

	_, err := svc.CreateRef(context.Background(), &model.KnowledgeBaseAssetRef{
		ID:                 refID,
		OrganizationID:     "org-1",
		DatasetID:          "dataset-1",
		AssetID:            assetID,
		VersionID:          &versionID,
		ChunkArtifactSetID: &chunkSetID,
		VectorArtifactID:   &vectorID,
		CreatedBy:          "account-1",
	})
	if err != nil {
		t.Fatalf("CreateRef: %v", err)
	}
	if reuseRepo.created == nil {
		t.Fatal("expected reuse event")
	}
	if reuseRepo.created.OrganizationID != "org-1" ||
		reuseRepo.created.AssetID != assetID ||
		reuseRepo.created.VersionID == nil ||
		*reuseRepo.created.VersionID != versionID ||
		reuseRepo.created.ArtifactType != model.ReuseArtifactVectorArtifact ||
		reuseRepo.created.ArtifactID == nil ||
		*reuseRepo.created.ArtifactID != vectorID ||
		reuseRepo.created.ConsumerType != model.ReuseConsumerKnowledgeBase ||
		reuseRepo.created.ConsumerID != "dataset-1" ||
		reuseRepo.created.CreatedBy != "account-1" ||
		reuseRepo.created.MetadataJSON["knowledge_base_asset_ref_id"] != refID.String() {
		t.Fatalf("reuse event=%+v", reuseRepo.created)
	}
}

func TestKnowledgeBaseAssetRefServiceCreateRefReturnsExistingActiveRef(t *testing.T) {
	existingID := uuid.New()
	assetID := uuid.New()
	versionID := uuid.New()
	repo := &fakeKnowledgeBaseAssetRefRepository{
		active: &model.KnowledgeBaseAssetRef{
			ID:             existingID,
			OrganizationID: "org-1",
			DatasetID:      "dataset-1",
			AssetID:        assetID,
			VersionID:      &versionID,
			Status:         model.KnowledgeBaseAssetRefStatusActive,
		},
	}
	reuseRepo := &fakeReuseEventRepository{}
	svc := NewKnowledgeBaseAssetRefService(repo, reuseRepo)

	view, err := svc.CreateRef(context.Background(), &model.KnowledgeBaseAssetRef{
		OrganizationID: "org-1",
		DatasetID:      "dataset-1",
		AssetID:        assetID,
		VersionID:      &versionID,
	})
	if err != nil {
		t.Fatalf("CreateRef: %v", err)
	}
	if repo.created != nil {
		t.Fatalf("expected create to be skipped, created=%+v", repo.created)
	}
	if view == nil || view.ID != existingID {
		t.Fatalf("view=%+v", view)
	}
	if reuseRepo.created != nil {
		t.Fatalf("expected no reuse event for existing active ref, got %+v", reuseRepo.created)
	}
}

func TestKnowledgeBaseAssetRefServiceDisablesRef(t *testing.T) {
	refID := uuid.New()
	versionID := uuid.New()
	repo := &fakeKnowledgeBaseAssetRefRepository{
		item: &model.KnowledgeBaseAssetRef{
			ID:             refID,
			OrganizationID: "org-1",
			DatasetID:      "dataset-1",
			AssetID:        uuid.New(),
			VersionID:      &versionID,
			Status:         model.KnowledgeBaseAssetRefStatusDisabled,
		},
	}
	svc := NewKnowledgeBaseAssetRefService(repo)

	view, err := svc.DisableRef(context.Background(), "org-1", refID)
	if err != nil {
		t.Fatalf("DisableRef: %v", err)
	}
	if repo.lastUpdateOrganizationID != "org-1" ||
		repo.lastUpdateID != refID ||
		repo.lastUpdateStatus != model.KnowledgeBaseAssetRefStatusDisabled {
		t.Fatalf("update args org=%s id=%s status=%s", repo.lastUpdateOrganizationID, repo.lastUpdateID, repo.lastUpdateStatus)
	}
	if view == nil || view.ID != refID || view.Status != model.KnowledgeBaseAssetRefStatusDisabled {
		t.Fatalf("view=%+v", view)
	}
}

func TestKnowledgeBaseAssetRefServiceDisableRefNotFound(t *testing.T) {
	svc := NewKnowledgeBaseAssetRefService(&fakeKnowledgeBaseAssetRefRepository{})

	_, err := svc.DisableRef(context.Background(), "org-1", uuid.New())
	if !errors.Is(err, ErrKnowledgeBaseAssetRefNotFound) {
		t.Fatalf("err=%v", err)
	}
}

func TestKnowledgeBaseAssetRefServiceListsAndFindsRefs(t *testing.T) {
	assetID := uuid.New()
	versionID := uuid.New()
	refID := uuid.New()
	repo := &fakeKnowledgeBaseAssetRefRepository{
		item: &model.KnowledgeBaseAssetRef{
			ID:             refID,
			OrganizationID: "org-1",
			DatasetID:      "dataset-1",
			AssetID:        assetID,
			VersionID:      &versionID,
			Status:         model.KnowledgeBaseAssetRefStatusActive,
		},
		items: []*model.KnowledgeBaseAssetRef{
			{
				ID:             refID,
				OrganizationID: "org-1",
				DatasetID:      "dataset-1",
				AssetID:        assetID,
				VersionID:      &versionID,
				Status:         model.KnowledgeBaseAssetRefStatusActive,
			},
		},
		total: 1,
	}
	svc := NewKnowledgeBaseAssetRefService(repo)

	views, total, err := svc.ListRefViews(context.Background(), repository.KnowledgeBaseAssetRefListFilter{
		OrganizationID: "org-1",
		DatasetID:      "dataset-1",
		Limit:          20,
	})
	if err != nil {
		t.Fatalf("ListRefViews: %v", err)
	}
	if repo.lastFilter.OrganizationID != "org-1" || repo.lastFilter.DatasetID != "dataset-1" {
		t.Fatalf("filter=%+v", repo.lastFilter)
	}
	if total != 1 || len(views) != 1 || views[0].ID != refID {
		t.Fatalf("views=%+v total=%d", views, total)
	}

	active, err := svc.FindActiveRefView(context.Background(), "org-1", "dataset-1", assetID, versionID)
	if err != nil {
		t.Fatalf("FindActiveRefView: %v", err)
	}
	if active == nil || active.ID != refID {
		t.Fatalf("active=%+v", active)
	}
	if repo.lastFindOrganizationID != "org-1" ||
		repo.lastFindDatasetID != "dataset-1" ||
		repo.lastFindAssetID != assetID ||
		repo.lastFindVersionID != versionID {
		t.Fatalf("find args org=%s dataset=%s asset=%s version=%s",
			repo.lastFindOrganizationID,
			repo.lastFindDatasetID,
			repo.lastFindAssetID,
			repo.lastFindVersionID)
	}
}

func TestKnowledgeBaseAssetRefServiceUpdatesSyncState(t *testing.T) {
	refID := uuid.New()
	documentID := uuid.New()
	repo := &fakeKnowledgeBaseAssetRefRepository{
		item: &model.KnowledgeBaseAssetRef{
			ID:             refID,
			OrganizationID: "org-1",
			DatasetID:      "dataset-1",
			AssetID:        uuid.New(),
			SyncStatus:     model.KnowledgeBaseAssetRefSyncStatusPending,
		},
	}
	svc := NewKnowledgeBaseAssetRefService(repo)

	pending, syncRunID, err := svc.MarkRefPending(context.Background(), "org-1", refID)
	if err != nil {
		t.Fatalf("MarkRefPending: %v", err)
	}
	if pending == nil || syncRunID == uuid.Nil || repo.lastSyncRunID != syncRunID {
		t.Fatalf("pending=%+v sync_run_id=%s repo_sync_run_id=%s", pending, syncRunID, repo.lastSyncRunID)
	}

	syncing, err := svc.MarkRefSyncing(context.Background(), "org-1", refID, syncRunID)
	if err != nil {
		t.Fatalf("MarkRefSyncing: %v", err)
	}
	if syncing == nil || repo.lastSyncStatus != model.KnowledgeBaseAssetRefSyncStatusSyncing {
		t.Fatalf("syncing=%+v status=%s", syncing, repo.lastSyncStatus)
	}

	synced, err := svc.MarkRefSynced(context.Background(), "org-1", refID, syncRunID, documentID, 12)
	if err != nil {
		t.Fatalf("MarkRefSynced: %v", err)
	}
	if synced == nil ||
		repo.lastDatasetDocumentID != documentID ||
		repo.lastSyncedGenerationNo != 12 ||
		repo.lastSyncStatus != model.KnowledgeBaseAssetRefSyncStatusSynced {
		t.Fatalf("synced=%+v doc=%s generation=%d status=%s", synced, repo.lastDatasetDocumentID, repo.lastSyncedGenerationNo, repo.lastSyncStatus)
	}

	failed, err := svc.MarkRefFailed(context.Background(), "org-1", refID, syncRunID, "sync_error", "failed to sync")
	if err != nil {
		t.Fatalf("MarkRefFailed: %v", err)
	}
	if failed == nil ||
		repo.lastSyncStatus != model.KnowledgeBaseAssetRefSyncStatusFailed ||
		repo.lastErrorCode != "sync_error" ||
		repo.lastErrorMessage != "failed to sync" {
		t.Fatalf("failed=%+v status=%s code=%s message=%s", failed, repo.lastSyncStatus, repo.lastErrorCode, repo.lastErrorMessage)
	}
}

type fakeKnowledgeBaseAssetRefRepository struct {
	created                  *model.KnowledgeBaseAssetRef
	item                     *model.KnowledgeBaseAssetRef
	active                   *model.KnowledgeBaseAssetRef
	items                    []*model.KnowledgeBaseAssetRef
	total                    int64
	activeCount              int64
	lastFilter               repository.KnowledgeBaseAssetRefListFilter
	lastFindOrganizationID   string
	lastFindDatasetID        string
	lastFindAssetID          uuid.UUID
	lastFindVersionID        uuid.UUID
	lastUpdateOrganizationID string
	lastUpdateID             uuid.UUID
	lastUpdateStatus         string
	lastSyncRunID            uuid.UUID
	lastSyncStatus           string
	lastDatasetDocumentID    uuid.UUID
	lastSyncedGenerationNo   int64
	lastErrorCode            string
	lastErrorMessage         string
	softDeleted              bool
}

func (r *fakeKnowledgeBaseAssetRefRepository) Create(ctx context.Context, item *model.KnowledgeBaseAssetRef) error {
	r.created = item
	return nil
}

func (r *fakeKnowledgeBaseAssetRefRepository) GetByID(ctx context.Context, id uuid.UUID) (*model.KnowledgeBaseAssetRef, error) {
	if r.item != nil && r.item.ID == id {
		return r.item, nil
	}
	return nil, nil
}

func (r *fakeKnowledgeBaseAssetRefRepository) List(ctx context.Context, filter repository.KnowledgeBaseAssetRefListFilter) ([]*model.KnowledgeBaseAssetRef, int64, error) {
	r.lastFilter = filter
	return r.items, r.total, nil
}

func (r *fakeKnowledgeBaseAssetRefRepository) FindActive(ctx context.Context, organizationID string, datasetID string, assetID uuid.UUID, versionID uuid.UUID) (*model.KnowledgeBaseAssetRef, error) {
	r.lastFindOrganizationID = organizationID
	r.lastFindDatasetID = datasetID
	r.lastFindAssetID = assetID
	r.lastFindVersionID = versionID
	if r.active != nil {
		return r.active, nil
	}
	return r.item, nil
}

func (r *fakeKnowledgeBaseAssetRefRepository) FindActiveByAsset(ctx context.Context, organizationID string, datasetID string, assetID uuid.UUID) (*model.KnowledgeBaseAssetRef, error) {
	r.lastFindOrganizationID = organizationID
	r.lastFindDatasetID = datasetID
	r.lastFindAssetID = assetID
	if r.active != nil {
		return r.active, nil
	}
	return r.item, nil
}

func (r *fakeKnowledgeBaseAssetRefRepository) ListActiveByAsset(ctx context.Context, organizationID string, assetID uuid.UUID) ([]*model.KnowledgeBaseAssetRef, error) {
	r.lastFindOrganizationID = organizationID
	r.lastFindAssetID = assetID
	return r.items, nil
}

func (r *fakeKnowledgeBaseAssetRefRepository) CountActiveByAssetID(ctx context.Context, organizationID string, assetID uuid.UUID) (int64, error) {
	return r.activeCount, nil
}

func (r *fakeKnowledgeBaseAssetRefRepository) UpdateStatus(ctx context.Context, organizationID string, id uuid.UUID, status string) (*model.KnowledgeBaseAssetRef, error) {
	r.lastUpdateOrganizationID = organizationID
	r.lastUpdateID = id
	r.lastUpdateStatus = status
	return r.item, nil
}

func (r *fakeKnowledgeBaseAssetRefRepository) MarkPending(ctx context.Context, organizationID string, id uuid.UUID, syncRunID uuid.UUID, errorCode, errorMessage *string) (*model.KnowledgeBaseAssetRef, error) {
	r.lastUpdateOrganizationID = organizationID
	r.lastUpdateID = id
	r.lastSyncRunID = syncRunID
	r.lastSyncStatus = model.KnowledgeBaseAssetRefSyncStatusPending
	if r.item != nil {
		r.item.SyncRunID = &syncRunID
		r.item.SyncStatus = model.KnowledgeBaseAssetRefSyncStatusPending
	}
	return r.item, nil
}

func (r *fakeKnowledgeBaseAssetRefRepository) MarkSyncing(ctx context.Context, organizationID string, id uuid.UUID, syncRunID uuid.UUID) (*model.KnowledgeBaseAssetRef, error) {
	r.lastUpdateOrganizationID = organizationID
	r.lastUpdateID = id
	r.lastSyncRunID = syncRunID
	r.lastSyncStatus = model.KnowledgeBaseAssetRefSyncStatusSyncing
	if r.item != nil {
		r.item.SyncStatus = model.KnowledgeBaseAssetRefSyncStatusSyncing
	}
	return r.item, nil
}

func (r *fakeKnowledgeBaseAssetRefRepository) MarkSynced(ctx context.Context, organizationID string, id uuid.UUID, syncRunID uuid.UUID, datasetDocumentID uuid.UUID, generationNo int64, syncedAt time.Time) (*model.KnowledgeBaseAssetRef, error) {
	r.lastUpdateOrganizationID = organizationID
	r.lastUpdateID = id
	r.lastSyncRunID = syncRunID
	r.lastDatasetDocumentID = datasetDocumentID
	r.lastSyncedGenerationNo = generationNo
	r.lastSyncStatus = model.KnowledgeBaseAssetRefSyncStatusSynced
	if r.item != nil {
		r.item.DatasetDocumentID = &datasetDocumentID
		r.item.SyncedGenerationNo = &generationNo
		r.item.LastSyncedAt = &syncedAt
		r.item.SyncStatus = model.KnowledgeBaseAssetRefSyncStatusSynced
	}
	return r.item, nil
}

func (r *fakeKnowledgeBaseAssetRefRepository) MarkFailed(ctx context.Context, organizationID string, id uuid.UUID, syncRunID uuid.UUID, errorCode, errorMessage string) (*model.KnowledgeBaseAssetRef, error) {
	r.lastUpdateOrganizationID = organizationID
	r.lastUpdateID = id
	r.lastSyncRunID = syncRunID
	r.lastSyncStatus = model.KnowledgeBaseAssetRefSyncStatusFailed
	r.lastErrorCode = errorCode
	r.lastErrorMessage = errorMessage
	if r.item != nil {
		r.item.SyncStatus = model.KnowledgeBaseAssetRefSyncStatusFailed
		r.item.SyncErrorCode = &errorCode
		r.item.SyncErrorMessage = &errorMessage
	}
	return r.item, nil
}

func (r *fakeKnowledgeBaseAssetRefRepository) SoftDelete(ctx context.Context, organizationID string, id uuid.UUID) (*model.KnowledgeBaseAssetRef, error) {
	r.lastUpdateOrganizationID = organizationID
	r.lastUpdateID = id
	r.softDeleted = true
	return r.item, nil
}
