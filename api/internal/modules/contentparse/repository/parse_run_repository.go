package repository

import (
	"context"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/contentparse/model"
	"gorm.io/gorm"
)

type ParseRunRepository interface {
	Create(ctx context.Context, item *model.ParseRun) error
	GetByID(ctx context.Context, id uuid.UUID) (*model.ParseRun, error)
	ListByDocumentID(ctx context.Context, documentID uuid.UUID, limit int) ([]*model.ParseRun, error)
	ListByDatasetID(ctx context.Context, datasetID uuid.UUID, limit int) ([]*model.ParseRun, error)
	ListLatestByDatasetID(ctx context.Context, datasetID uuid.UUID, limit int) ([]*model.ParseRun, error)
}

type parseRunRepository struct {
	db *gorm.DB
}

func NewParseRunRepository(db *gorm.DB) ParseRunRepository {
	return &parseRunRepository{db: db}
}

func (r *parseRunRepository) Create(ctx context.Context, item *model.ParseRun) error {
	return r.db.WithContext(ctx).Create(item).Error
}

func (r *parseRunRepository) GetByID(ctx context.Context, id uuid.UUID) (*model.ParseRun, error) {
	var item model.ParseRun
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&item).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &item, nil
}

func (r *parseRunRepository) ListByDocumentID(ctx context.Context, documentID uuid.UUID, limit int) ([]*model.ParseRun, error) {
	var items []*model.ParseRun
	query := r.db.WithContext(ctx).
		Where("document_id = ?", documentID).
		Order("created_at DESC")
	if limit > 0 {
		query = query.Limit(limit)
	}
	err := query.Find(&items).Error
	return items, err
}

func (r *parseRunRepository) ListByDatasetID(ctx context.Context, datasetID uuid.UUID, limit int) ([]*model.ParseRun, error) {
	var items []*model.ParseRun
	query := r.db.WithContext(ctx).
		Where("dataset_id = ?", datasetID).
		Order("created_at DESC")
	if limit > 0 {
		query = query.Limit(limit)
	}
	err := query.Find(&items).Error
	return items, err
}

func (r *parseRunRepository) ListLatestByDatasetID(ctx context.Context, datasetID uuid.UUID, limit int) ([]*model.ParseRun, error) {
	var items []*model.ParseRun
	query := `
		SELECT *
		FROM (
			SELECT DISTINCT ON (document_id) *
			FROM content_parse_runs
			WHERE dataset_id = ?
			ORDER BY document_id, created_at DESC
		) latest_runs
		ORDER BY created_at DESC
	`
	if limit > 0 {
		query += " LIMIT ?"
		if err := r.db.WithContext(ctx).Raw(query, datasetID, limit).Scan(&items).Error; err != nil {
			return nil, err
		}
		return items, nil
	}
	if err := r.db.WithContext(ctx).Raw(query, datasetID).Scan(&items).Error; err != nil {
		return nil, err
	}
	return items, nil
}
