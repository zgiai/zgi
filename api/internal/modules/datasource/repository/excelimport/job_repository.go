package excelimport

import (
	"context"
	"time"

	"github.com/google/uuid"
	model "github.com/zgiai/ginext/internal/modules/datasource/model/excelimport"
	"gorm.io/gorm"
)

type JobRepository interface {
	Create(ctx context.Context, job *model.ImportJob) error
	FindByID(ctx context.Context, id string) (*model.ImportJob, error)
	FindLatestByTableID(ctx context.Context, tableID string) (*model.ImportJob, error)
	MarkImporting(ctx context.Context, id, organizationID, dataSourceID, updatedBy string) (bool, error)
	Update(ctx context.Context, job *model.ImportJob) error
}

type jobRepository struct {
	db *gorm.DB
}

func NewJobRepository(db *gorm.DB) JobRepository {
	return &jobRepository{db: db}
}

func (r *jobRepository) Create(ctx context.Context, job *model.ImportJob) error {
	if job.ID == "" {
		job.ID = uuid.NewString()
	}
	now := time.Now()
	if job.CreatedAt.IsZero() {
		job.CreatedAt = now
	}
	job.UpdatedAt = now
	return r.db.WithContext(ctx).Create(job).Error
}

func (r *jobRepository) FindByID(ctx context.Context, id string) (*model.ImportJob, error) {
	var job model.ImportJob
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&job).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &job, nil
}

func (r *jobRepository) FindLatestByTableID(ctx context.Context, tableID string) (*model.ImportJob, error) {
	var job model.ImportJob
	err := r.db.WithContext(ctx).
		Where("table_id = ?", tableID).
		Order("created_at DESC").
		First(&job).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &job, nil
}

func (r *jobRepository) MarkImporting(ctx context.Context, id, organizationID, dataSourceID, updatedBy string) (bool, error) {
	result := r.db.WithContext(ctx).
		Model(&model.ImportJob{}).
		Where("id = ? AND organization_id = ? AND data_source_id = ? AND status = ?", id, organizationID, dataSourceID, "needs_review").
		Updates(map[string]interface{}{
			"status":     "importing",
			"updated_by": updatedBy,
			"updated_at": time.Now(),
		})
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected == 1, nil
}

func (r *jobRepository) Update(ctx context.Context, job *model.ImportJob) error {
	job.UpdatedAt = time.Now()
	return r.db.WithContext(ctx).Save(job).Error
}
