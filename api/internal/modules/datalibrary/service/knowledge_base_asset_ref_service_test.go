package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/ginext/internal/modules/datalibrary/model"
	"github.com/zgiai/ginext/internal/modules/datalibrary/repository"
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
				VersionID: versionID,
			},
			err: ErrOrganizationIDRequired,
		},
		{
			name: "requires dataset",
			item: &model.KnowledgeBaseAssetRef{
				OrganizationID: "org-1",
				AssetID:        assetID,
				VersionID:      versionID,
			},
			err: ErrDatasetIDRequired,
		},
		{
			name: "requires asset",
			item: &model.KnowledgeBaseAssetRef{
				OrganizationID: "org-1",
				DatasetID:      "dataset-1",
				VersionID:      versionID,
			},
			err: ErrAssetIDRequired,
		},
		{
			name: "requires version",
			item: &model.KnowledgeBaseAssetRef{
				OrganizationID: "org-1",
				DatasetID:      "dataset-1",
				AssetID:        assetID,
			},
			err: ErrVersionIDRequired,
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
	if _, err := svc.FindActiveRefView(ctx, "org-1", "dataset-1", assetID, uuid.Nil); !errors.Is(err, ErrVersionIDRequired) {
		t.Fatalf("FindActiveRefView version error=%v", err)
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
		VersionID:          versionID,
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
		view.VersionID != versionID ||
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
		VersionID:          versionID,
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
			VersionID:      versionID,
			Status:         model.KnowledgeBaseAssetRefStatusActive,
		},
	}
	reuseRepo := &fakeReuseEventRepository{}
	svc := NewKnowledgeBaseAssetRefService(repo, reuseRepo)

	view, err := svc.CreateRef(context.Background(), &model.KnowledgeBaseAssetRef{
		OrganizationID: "org-1",
		DatasetID:      "dataset-1",
		AssetID:        assetID,
		VersionID:      versionID,
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
	repo := &fakeKnowledgeBaseAssetRefRepository{
		item: &model.KnowledgeBaseAssetRef{
			ID:             refID,
			OrganizationID: "org-1",
			DatasetID:      "dataset-1",
			AssetID:        uuid.New(),
			VersionID:      uuid.New(),
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
			VersionID:      versionID,
			Status:         model.KnowledgeBaseAssetRefStatusActive,
		},
		items: []*model.KnowledgeBaseAssetRef{
			{
				ID:             refID,
				OrganizationID: "org-1",
				DatasetID:      "dataset-1",
				AssetID:        assetID,
				VersionID:      versionID,
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

func (r *fakeKnowledgeBaseAssetRefRepository) CountActiveByAssetID(ctx context.Context, organizationID string, assetID uuid.UUID) (int64, error) {
	return r.activeCount, nil
}

func (r *fakeKnowledgeBaseAssetRefRepository) UpdateStatus(ctx context.Context, organizationID string, id uuid.UUID, status string) (*model.KnowledgeBaseAssetRef, error) {
	r.lastUpdateOrganizationID = organizationID
	r.lastUpdateID = id
	r.lastUpdateStatus = status
	return r.item, nil
}
