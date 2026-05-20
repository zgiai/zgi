package service

import (
	"context"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/contentparse/model"
	"github.com/zgiai/zgi/api/internal/modules/contentparse/repository"
)

type ChunkArtifactSetService interface {
	GetByID(ctx context.Context, id uuid.UUID) (*model.ChunkArtifactSet, error)
	GetBySignature(ctx context.Context, signature string) (*model.ChunkArtifactSet, error)
	Upsert(ctx context.Context, item *model.ChunkArtifactSet) error
}

type chunkArtifactSetService struct {
	repo repository.ChunkArtifactSetRepository
}

func NewChunkArtifactSetService(repo repository.ChunkArtifactSetRepository) ChunkArtifactSetService {
	return &chunkArtifactSetService{repo: repo}
}

func (s *chunkArtifactSetService) GetByID(ctx context.Context, id uuid.UUID) (*model.ChunkArtifactSet, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *chunkArtifactSetService) GetBySignature(ctx context.Context, signature string) (*model.ChunkArtifactSet, error) {
	return s.repo.GetBySignature(ctx, signature)
}

func (s *chunkArtifactSetService) Upsert(ctx context.Context, item *model.ChunkArtifactSet) error {
	return s.repo.Upsert(ctx, item)
}
