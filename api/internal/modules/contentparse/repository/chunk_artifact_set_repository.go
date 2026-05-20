package repository

import (
	"context"

	"github.com/google/uuid"
	"github.com/zgiai/ginext/internal/modules/contentparse/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type ChunkArtifactSetRepository interface {
	GetByID(ctx context.Context, id uuid.UUID) (*model.ChunkArtifactSet, error)
	GetBySignature(ctx context.Context, signature string) (*model.ChunkArtifactSet, error)
	Upsert(ctx context.Context, item *model.ChunkArtifactSet) error
}

type chunkArtifactSetRepository struct {
	db *gorm.DB
}

func NewChunkArtifactSetRepository(db *gorm.DB) ChunkArtifactSetRepository {
	return &chunkArtifactSetRepository{db: db}
}

func (r *chunkArtifactSetRepository) GetByID(ctx context.Context, id uuid.UUID) (*model.ChunkArtifactSet, error) {
	var item model.ChunkArtifactSet
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&item).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &item, nil
}

func (r *chunkArtifactSetRepository) GetBySignature(ctx context.Context, signature string) (*model.ChunkArtifactSet, error) {
	var item model.ChunkArtifactSet
	err := r.db.WithContext(ctx).Where("signature = ?", signature).First(&item).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &item, nil
}

func (r *chunkArtifactSetRepository) Upsert(ctx context.Context, item *model.ChunkArtifactSet) error {
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "signature"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"parse_artifact_id",
			"parse_run_id",
			"status",
			"unit_count",
			"content_hash",
			"quality_json",
			"summary_json",
			"artifact_storage_key",
			"updated_at",
		}),
	}).Create(item).Error
}
