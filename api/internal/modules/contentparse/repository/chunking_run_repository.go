package repository

import (
	"context"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/contentparse/model"
	"gorm.io/gorm"
)

type ChunkingRunRepository interface {
	Create(ctx context.Context, item *model.ChunkingRun) error
	ListByParseRunID(ctx context.Context, parseRunID uuid.UUID) ([]*model.ChunkingRun, error)
	ListLatestByParseRunIDs(ctx context.Context, parseRunIDs []uuid.UUID) ([]*model.ChunkingRun, error)
}

type chunkingRunRepository struct {
	db *gorm.DB
}

func NewChunkingRunRepository(db *gorm.DB) ChunkingRunRepository {
	return &chunkingRunRepository{db: db}
}

func (r *chunkingRunRepository) Create(ctx context.Context, item *model.ChunkingRun) error {
	return r.db.WithContext(ctx).Create(item).Error
}

func (r *chunkingRunRepository) ListByParseRunID(ctx context.Context, parseRunID uuid.UUID) ([]*model.ChunkingRun, error) {
	var items []*model.ChunkingRun
	err := r.db.WithContext(ctx).
		Where("parse_run_id = ?", parseRunID).
		Order("created_at DESC").
		Find(&items).Error
	return items, err
}

func (r *chunkingRunRepository) ListLatestByParseRunIDs(ctx context.Context, parseRunIDs []uuid.UUID) ([]*model.ChunkingRun, error) {
	if len(parseRunIDs) == 0 {
		return []*model.ChunkingRun{}, nil
	}
	var items []*model.ChunkingRun
	err := r.db.WithContext(ctx).
		Where("parse_run_id IN ?", parseRunIDs).
		Order("parse_run_id ASC, created_at DESC").
		Find(&items).Error
	return items, err
}
