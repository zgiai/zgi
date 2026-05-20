package excelimport

import (
	"context"
	"time"

	"github.com/google/uuid"
	model "github.com/zgiai/zgi/api/internal/modules/datasource/model/excelimport"
	"gorm.io/gorm"
)

type ErrorRepository interface {
	CreateBatch(ctx context.Context, errors []model.ImportError) error
	ListByJobID(ctx context.Context, jobID string, limit, offset int) ([]model.ImportError, int64, error)
	DeleteByJobID(ctx context.Context, jobID string) error
}

type errorRepository struct {
	db *gorm.DB
}

func NewErrorRepository(db *gorm.DB) ErrorRepository {
	return &errorRepository{db: db}
}

func (r *errorRepository) CreateBatch(ctx context.Context, errors []model.ImportError) error {
	if len(errors) == 0 {
		return nil
	}
	now := time.Now()
	for i := range errors {
		if errors[i].ID == "" {
			errors[i].ID = uuid.NewString()
		}
		if errors[i].CreatedAt.IsZero() {
			errors[i].CreatedAt = now
		}
	}
	return r.db.WithContext(ctx).CreateInBatches(errors, 200).Error
}

func (r *errorRepository) ListByJobID(ctx context.Context, jobID string, limit, offset int) ([]model.ImportError, int64, error) {
	var total int64
	query := r.db.WithContext(ctx).Model(&model.ImportError{}).Where("job_id = ?", jobID)
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var rows []model.ImportError
	if err := query.Order("row_index ASC, created_at ASC").Limit(limit).Offset(offset).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

func (r *errorRepository) DeleteByJobID(ctx context.Context, jobID string) error {
	return r.db.WithContext(ctx).Where("job_id = ?", jobID).Delete(&model.ImportError{}).Error
}
