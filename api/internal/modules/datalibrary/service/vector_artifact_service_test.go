package service

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/zgiai/ginext/internal/modules/datalibrary/model"
	"github.com/zgiai/ginext/internal/modules/datalibrary/repository"
)

func TestVectorArtifactServiceListsViews(t *testing.T) {
	artifactID := uuid.New()
	assetID := uuid.New()
	versionID := uuid.New()
	chunkSetID := uuid.New()
	repo := &vectorArtifactServiceTestRepository{
		items: []*model.VectorArtifact{
			{
				ID:                 artifactID,
				OrganizationID:     "org-1",
				AssetID:            assetID,
				VersionID:          versionID,
				ChunkArtifactSetID: chunkSetID,
				EmbeddingProvider:  "openai",
				EmbeddingModel:     "text-embedding-3-large",
				EmbeddingDimension: 3072,
				VectorCollection:   "data_library_vectors",
				VectorNamespace:    "workspace-1",
				VectorCount:        42,
				Status:             model.VectorArtifactStatusReady,
			},
		},
		total: 1,
	}
	svc := NewVectorArtifactService(repo)

	views, total, err := svc.ListArtifactViews(context.Background(), repository.VectorArtifactListFilter{
		OrganizationID:     "org-1",
		AssetID:            assetID,
		VersionID:          versionID,
		ChunkArtifactSetID: chunkSetID,
		Status:             model.VectorArtifactStatusReady,
		Limit:              10,
	})
	if err != nil {
		t.Fatalf("ListArtifactViews: %v", err)
	}
	if repo.lastFilter.OrganizationID != "org-1" ||
		repo.lastFilter.AssetID != assetID ||
		repo.lastFilter.VersionID != versionID ||
		repo.lastFilter.ChunkArtifactSetID != chunkSetID ||
		repo.lastFilter.Status != model.VectorArtifactStatusReady {
		t.Fatalf("filter=%+v", repo.lastFilter)
	}
	if total != 1 ||
		len(views) != 1 ||
		views[0].ID != artifactID ||
		views[0].EmbeddingProvider != "openai" ||
		views[0].EmbeddingDimension != 3072 ||
		views[0].VectorCount != 42 {
		t.Fatalf("views=%+v total=%d", views, total)
	}
}

func TestVectorArtifactServiceGetNotFound(t *testing.T) {
	svc := NewVectorArtifactService(&vectorArtifactServiceTestRepository{})

	_, err := svc.GetArtifactViewByID(context.Background(), uuid.New())
	if !errors.Is(err, ErrVectorArtifactNotFound) {
		t.Fatalf("err=%v", err)
	}
}

func TestVectorArtifactServiceCreateValidatesRequiredFields(t *testing.T) {
	svc := NewVectorArtifactService(&vectorArtifactServiceTestRepository{})

	_, err := svc.CreateArtifact(context.Background(), &model.VectorArtifact{
		OrganizationID: "org-1",
	})
	if !errors.Is(err, ErrAssetIDRequired) {
		t.Fatalf("err=%v", err)
	}
}

type vectorArtifactServiceTestRepository struct {
	created    *model.VectorArtifact
	current    *model.VectorArtifact
	items      []*model.VectorArtifact
	total      int64
	lastFilter repository.VectorArtifactListFilter
}

func (r *vectorArtifactServiceTestRepository) Create(ctx context.Context, item *model.VectorArtifact) error {
	if err := item.BeforeCreate(nil); err != nil {
		return err
	}
	r.created = item
	return nil
}

func (r *vectorArtifactServiceTestRepository) GetByID(ctx context.Context, id uuid.UUID) (*model.VectorArtifact, error) {
	if r.current != nil && r.current.ID == id {
		return r.current, nil
	}
	return nil, nil
}

func (r *vectorArtifactServiceTestRepository) List(ctx context.Context, filter repository.VectorArtifactListFilter) ([]*model.VectorArtifact, int64, error) {
	r.lastFilter = filter
	return r.items, r.total, nil
}

func (r *vectorArtifactServiceTestRepository) LatestReadyByVersionID(ctx context.Context, organizationID string, versionID uuid.UUID) (*model.VectorArtifact, error) {
	return r.current, nil
}

var _ repository.VectorArtifactRepository = (*vectorArtifactServiceTestRepository)(nil)
