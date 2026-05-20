package service

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/datalibrary/model"
	"github.com/zgiai/zgi/api/internal/modules/datalibrary/repository"
)

var (
	ErrVectorArtifactIDRequired = errors.New("vector_artifact_id is required")
	ErrVectorArtifactNotFound   = errors.New("vector artifact not found")
)

type VectorArtifactService interface {
	CreateArtifact(ctx context.Context, item *model.VectorArtifact) (*VectorArtifactView, error)
	GetArtifactViewByID(ctx context.Context, id uuid.UUID) (*VectorArtifactView, error)
	ListArtifactViews(ctx context.Context, filter repository.VectorArtifactListFilter) ([]*VectorArtifactView, int64, error)
	LatestReadyByVersionID(ctx context.Context, organizationID string, versionID uuid.UUID) (*VectorArtifactView, error)
}

type vectorArtifactService struct {
	repo repository.VectorArtifactRepository
}

func NewVectorArtifactService(repo repository.VectorArtifactRepository) VectorArtifactService {
	return &vectorArtifactService{repo: repo}
}

func (s *vectorArtifactService) CreateArtifact(ctx context.Context, item *model.VectorArtifact) (*VectorArtifactView, error) {
	if item == nil || item.OrganizationID == "" {
		return nil, ErrOrganizationIDRequired
	}
	if item.AssetID == uuid.Nil {
		return nil, ErrAssetIDRequired
	}
	if item.VersionID == uuid.Nil {
		return nil, ErrAssetIDRequired
	}
	if item.ChunkArtifactSetID == uuid.Nil {
		return nil, ErrAssetIDRequired
	}
	if err := s.repo.Create(ctx, item); err != nil {
		return nil, err
	}
	return newVectorArtifactView(item), nil
}

func (s *vectorArtifactService) GetArtifactViewByID(ctx context.Context, id uuid.UUID) (*VectorArtifactView, error) {
	if id == uuid.Nil {
		return nil, ErrVectorArtifactIDRequired
	}
	item, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if item == nil {
		return nil, ErrVectorArtifactNotFound
	}
	return newVectorArtifactView(item), nil
}

func (s *vectorArtifactService) ListArtifactViews(ctx context.Context, filter repository.VectorArtifactListFilter) ([]*VectorArtifactView, int64, error) {
	if filter.OrganizationID == "" {
		return nil, 0, ErrOrganizationIDRequired
	}
	items, total, err := s.repo.List(ctx, filter)
	if err != nil {
		return nil, 0, err
	}
	views := make([]*VectorArtifactView, 0, len(items))
	for _, item := range items {
		views = append(views, newVectorArtifactView(item))
	}
	return views, total, nil
}

func (s *vectorArtifactService) LatestReadyByVersionID(ctx context.Context, organizationID string, versionID uuid.UUID) (*VectorArtifactView, error) {
	if organizationID == "" {
		return nil, ErrOrganizationIDRequired
	}
	if versionID == uuid.Nil {
		return nil, ErrAssetIDRequired
	}
	item, err := s.repo.LatestReadyByVersionID(ctx, organizationID, versionID)
	if err != nil || item == nil {
		return nil, err
	}
	return newVectorArtifactView(item), nil
}
