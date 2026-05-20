package service

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/zgiai/ginext/internal/modules/datalibrary/model"
	"github.com/zgiai/ginext/internal/modules/datalibrary/repository"
)

func TestDatabaseAssetRefServiceValidatesRequiredFields(t *testing.T) {
	svc := NewDatabaseAssetRefService(&fakeDatabaseAssetRefRepository{})
	ctx := context.Background()
	assetID := uuid.New()
	versionID := uuid.New()

	tests := []struct {
		name string
		item *model.DatabaseAssetRef
		err  error
	}{
		{
			name: "requires organization",
			item: &model.DatabaseAssetRef{
				DataSourceID: "database-1",
				AssetID:      assetID,
				VersionID:    versionID,
			},
			err: ErrOrganizationIDRequired,
		},
		{
			name: "requires datasource",
			item: &model.DatabaseAssetRef{
				OrganizationID: "org-1",
				AssetID:        assetID,
				VersionID:      versionID,
			},
			err: ErrDataSourceIDRequired,
		},
		{
			name: "requires asset",
			item: &model.DatabaseAssetRef{
				OrganizationID: "org-1",
				DataSourceID:   "database-1",
				VersionID:      versionID,
			},
			err: ErrAssetIDRequired,
		},
		{
			name: "requires version",
			item: &model.DatabaseAssetRef{
				OrganizationID: "org-1",
				DataSourceID:   "database-1",
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

	if _, err := svc.GetRefViewByID(ctx, uuid.Nil); !errors.Is(err, ErrDatabaseAssetRefIDRequired) {
		t.Fatalf("GetRefViewByID error=%v", err)
	}
	if _, _, err := svc.ListRefViews(ctx, repository.DatabaseAssetRefListFilter{}); !errors.Is(err, ErrOrganizationIDRequired) {
		t.Fatalf("ListRefViews error=%v", err)
	}
	if _, err := svc.DisableRef(ctx, "", uuid.New()); !errors.Is(err, ErrOrganizationIDRequired) {
		t.Fatalf("DisableRef organization error=%v", err)
	}
	if _, err := svc.DisableRef(ctx, "org-1", uuid.Nil); !errors.Is(err, ErrDatabaseAssetRefIDRequired) {
		t.Fatalf("DisableRef id error=%v", err)
	}
}

func TestDatabaseAssetRefServiceCreateRefRecordsDatabaseReuseEvent(t *testing.T) {
	refID := uuid.New()
	assetID := uuid.New()
	versionID := uuid.New()
	parseArtifactID := uuid.New()
	extractionArtifactID := uuid.New()
	tableID := uuid.NewString()
	repo := &fakeDatabaseAssetRefRepository{}
	reuseRepo := &fakeReuseEventRepository{}
	svc := NewDatabaseAssetRefService(repo, reuseRepo)

	view, err := svc.CreateRef(context.Background(), &model.DatabaseAssetRef{
		ID:                   refID,
		OrganizationID:       "org-1",
		DataSourceID:         "database-1",
		TableID:              &tableID,
		AssetID:              assetID,
		VersionID:            versionID,
		ParseArtifactID:      &parseArtifactID,
		ExtractionArtifactID: &extractionArtifactID,
		CreatedBy:            "account-1",
	})
	if err != nil {
		t.Fatalf("CreateRef: %v", err)
	}
	if repo.created == nil {
		t.Fatal("expected repository create")
	}
	if view == nil || view.ID != refID || view.DataSourceID != "database-1" || view.TableID == nil || *view.TableID != tableID {
		t.Fatalf("view=%+v", view)
	}
	if reuseRepo.created == nil {
		t.Fatal("expected reuse event")
	}
	if reuseRepo.created.OrganizationID != "org-1" ||
		reuseRepo.created.AssetID != assetID ||
		reuseRepo.created.VersionID == nil ||
		*reuseRepo.created.VersionID != versionID ||
		reuseRepo.created.ArtifactType != model.ReuseArtifactExtraction ||
		reuseRepo.created.ArtifactID == nil ||
		*reuseRepo.created.ArtifactID != extractionArtifactID ||
		reuseRepo.created.ConsumerType != model.ReuseConsumerDatabase ||
		reuseRepo.created.ConsumerID != "database-1" ||
		reuseRepo.created.CreatedBy != "account-1" ||
		reuseRepo.created.MetadataJSON["database_asset_ref_id"] != refID.String() {
		t.Fatalf("reuse event=%+v", reuseRepo.created)
	}
}

func TestDatabaseAssetRefServiceCreateRefReturnsExistingActiveRef(t *testing.T) {
	existingID := uuid.New()
	assetID := uuid.New()
	versionID := uuid.New()
	repo := &fakeDatabaseAssetRefRepository{
		active: &model.DatabaseAssetRef{
			ID:             existingID,
			OrganizationID: "org-1",
			DataSourceID:   "database-1",
			AssetID:        assetID,
			VersionID:      versionID,
			Status:         model.DatabaseAssetRefStatusActive,
		},
	}
	reuseRepo := &fakeReuseEventRepository{}
	svc := NewDatabaseAssetRefService(repo, reuseRepo)

	view, err := svc.CreateRef(context.Background(), &model.DatabaseAssetRef{
		OrganizationID: "org-1",
		DataSourceID:   "database-1",
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

func TestDatabaseAssetRefServiceListsFindsAndDisablesRefs(t *testing.T) {
	assetID := uuid.New()
	versionID := uuid.New()
	refID := uuid.New()
	tableID := uuid.NewString()
	repo := &fakeDatabaseAssetRefRepository{
		item: &model.DatabaseAssetRef{
			ID:             refID,
			OrganizationID: "org-1",
			DataSourceID:   "database-1",
			TableID:        &tableID,
			AssetID:        assetID,
			VersionID:      versionID,
			Status:         model.DatabaseAssetRefStatusDisabled,
		},
		items: []*model.DatabaseAssetRef{
			{
				ID:             refID,
				OrganizationID: "org-1",
				DataSourceID:   "database-1",
				TableID:        &tableID,
				AssetID:        assetID,
				VersionID:      versionID,
				Status:         model.DatabaseAssetRefStatusActive,
			},
		},
		total: 1,
	}
	svc := NewDatabaseAssetRefService(repo)

	views, total, err := svc.ListRefViews(context.Background(), repository.DatabaseAssetRefListFilter{
		OrganizationID: "org-1",
		DataSourceID:   "database-1",
		TableID:        tableID,
		Limit:          20,
	})
	if err != nil {
		t.Fatalf("ListRefViews: %v", err)
	}
	if repo.lastFilter.OrganizationID != "org-1" || repo.lastFilter.DataSourceID != "database-1" || repo.lastFilter.TableID != tableID {
		t.Fatalf("filter=%+v", repo.lastFilter)
	}
	if total != 1 || len(views) != 1 || views[0].ID != refID {
		t.Fatalf("views=%+v total=%d", views, total)
	}

	active, err := svc.FindActiveRefView(context.Background(), "org-1", "database-1", &tableID, assetID, versionID)
	if err != nil {
		t.Fatalf("FindActiveRefView: %v", err)
	}
	if active == nil || active.ID != refID {
		t.Fatalf("active=%+v", active)
	}

	disabled, err := svc.DisableRef(context.Background(), "org-1", refID)
	if err != nil {
		t.Fatalf("DisableRef: %v", err)
	}
	if repo.lastUpdateOrganizationID != "org-1" ||
		repo.lastUpdateID != refID ||
		repo.lastUpdateStatus != model.DatabaseAssetRefStatusDisabled {
		t.Fatalf("update args org=%s id=%s status=%s", repo.lastUpdateOrganizationID, repo.lastUpdateID, repo.lastUpdateStatus)
	}
	if disabled == nil || disabled.ID != refID || disabled.Status != model.DatabaseAssetRefStatusDisabled {
		t.Fatalf("disabled=%+v", disabled)
	}
}

func TestDatabaseAssetRefServiceDisableRefNotFound(t *testing.T) {
	svc := NewDatabaseAssetRefService(&fakeDatabaseAssetRefRepository{})

	_, err := svc.DisableRef(context.Background(), "org-1", uuid.New())
	if !errors.Is(err, ErrDatabaseAssetRefNotFound) {
		t.Fatalf("err=%v", err)
	}
}

type fakeDatabaseAssetRefRepository struct {
	created                  *model.DatabaseAssetRef
	item                     *model.DatabaseAssetRef
	active                   *model.DatabaseAssetRef
	items                    []*model.DatabaseAssetRef
	total                    int64
	activeCount              int64
	lastFilter               repository.DatabaseAssetRefListFilter
	lastUpdateOrganizationID string
	lastUpdateID             uuid.UUID
	lastUpdateStatus         string
}

func (r *fakeDatabaseAssetRefRepository) Create(ctx context.Context, item *model.DatabaseAssetRef) error {
	r.created = item
	return nil
}

func (r *fakeDatabaseAssetRefRepository) GetByID(ctx context.Context, id uuid.UUID) (*model.DatabaseAssetRef, error) {
	if r.item != nil && r.item.ID == id {
		return r.item, nil
	}
	return nil, nil
}

func (r *fakeDatabaseAssetRefRepository) List(ctx context.Context, filter repository.DatabaseAssetRefListFilter) ([]*model.DatabaseAssetRef, int64, error) {
	r.lastFilter = filter
	return r.items, r.total, nil
}

func (r *fakeDatabaseAssetRefRepository) FindActive(ctx context.Context, organizationID string, dataSourceID string, tableID *string, assetID uuid.UUID, versionID uuid.UUID) (*model.DatabaseAssetRef, error) {
	if r.active != nil {
		return r.active, nil
	}
	return r.item, nil
}

func (r *fakeDatabaseAssetRefRepository) CountActiveByAssetID(ctx context.Context, organizationID string, assetID uuid.UUID) (int64, error) {
	return r.activeCount, nil
}

func (r *fakeDatabaseAssetRefRepository) UpdateStatus(ctx context.Context, organizationID string, id uuid.UUID, status string) (*model.DatabaseAssetRef, error) {
	r.lastUpdateOrganizationID = organizationID
	r.lastUpdateID = id
	r.lastUpdateStatus = status
	return r.item, nil
}
