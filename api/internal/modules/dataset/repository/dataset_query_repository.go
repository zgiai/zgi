package repository

import (
	"context"

	"github.com/zgiai/ginext/internal/modules/dataset/model"
	"gorm.io/gorm"
)

// DatasetQueryRepository defines the interface for dataset query operations
type DatasetQueryRepository interface {
	// Create creates a new dataset query
	Create(ctx context.Context, query *model.DatasetQuery) error

	// GetByID retrieves a dataset query by ID
	GetByID(ctx context.Context, id string) (*model.DatasetQuery, error)

	// GetByDatasetID retrieves dataset queries by dataset ID with pagination
	GetByDatasetID(ctx context.Context, datasetID string, page, limit int, queryType *string) ([]*model.DatasetQuery, int64, error)

	// Delete deletes a dataset query by ID
	Delete(ctx context.Context, id string) error

	// WithTx returns a new repository with transaction
	WithTx(tx *gorm.DB) DatasetQueryRepository
}

// datasetQueryRepository implements DatasetQueryRepository
type datasetQueryRepository struct {
	db *gorm.DB
}

// NewDatasetQueryRepository creates a new dataset query repository
func NewDatasetQueryRepository(db *gorm.DB) DatasetQueryRepository {
	return &datasetQueryRepository{db: db}
}

// Create creates a new dataset query
func (r *datasetQueryRepository) Create(ctx context.Context, query *model.DatasetQuery) error {
	return r.db.WithContext(ctx).Create(query).Error
}

// GetByID retrieves a dataset query by ID
func (r *datasetQueryRepository) GetByID(ctx context.Context, id string) (*model.DatasetQuery, error) {
	var query model.DatasetQuery
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&query).Error
	if err != nil {
		return nil, err
	}
	return &query, nil
}

// GetByDatasetID retrieves dataset queries by dataset ID with pagination
func (r *datasetQueryRepository) GetByDatasetID(ctx context.Context, datasetID string, page, limit int, queryType *string) ([]*model.DatasetQuery, int64, error) {
	var queries []*model.DatasetQuery
	var total int64

	offset := (page - 1) * limit

	// Build the query
	query := r.db.WithContext(ctx).Model(&model.DatasetQuery{}).Where("dataset_id = ?", datasetID)

	// Add query type filter
	if queryType != nil && *queryType != "" {
		query = query.Where("query_type = ?", *queryType)
	} else {
		// By default, only query "single" and "batch_saved" types, excluding "batch" type (individual queries in batch testing)
		query = query.Where("query_type IN ?", []string{"single", "batch_saved"})
	}

	// Count total number of records
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// Apply pagination
	err := query.Order("created_at DESC").Offset(offset).Limit(limit).Find(&queries).Error
	return queries, total, err
}

// Delete deletes a dataset query by ID
func (r *datasetQueryRepository) Delete(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Where("id = ?", id).Delete(&model.DatasetQuery{}).Error
}

// WithTx returns a new repository with transaction
func (r *datasetQueryRepository) WithTx(tx *gorm.DB) DatasetQueryRepository {
	return NewDatasetQueryRepository(tx)
}
