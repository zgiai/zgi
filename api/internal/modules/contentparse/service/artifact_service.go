package service

import (
	"context"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/contentparse/model"
	"github.com/zgiai/zgi/api/internal/modules/contentparse/repository"
)

type ArtifactService interface {
	GetByID(ctx context.Context, id uuid.UUID) (*model.Artifact, error)
	GetBySignature(ctx context.Context, sourceContentHash, profile, canonicalIRVersion, providerSignature string) (*model.Artifact, error)
	Upsert(ctx context.Context, item *model.Artifact) error
}

type artifactService struct {
	repo repository.ArtifactRepository
}

func NewArtifactService(repo repository.ArtifactRepository) ArtifactService {
	return &artifactService{repo: repo}
}

func (s *artifactService) GetByID(ctx context.Context, id uuid.UUID) (*model.Artifact, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *artifactService) GetBySignature(ctx context.Context, sourceContentHash, profile, canonicalIRVersion, providerSignature string) (*model.Artifact, error) {
	return s.repo.GetBySignature(ctx, sourceContentHash, profile, canonicalIRVersion, providerSignature)
}

func (s *artifactService) Upsert(ctx context.Context, item *model.Artifact) error {
	return s.repo.Upsert(ctx, item)
}
