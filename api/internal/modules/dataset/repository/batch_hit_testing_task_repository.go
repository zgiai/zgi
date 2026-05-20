package repository

import (
	"context"
	"time"

	"github.com/zgiai/zgi/api/internal/modules/dataset/model"
	"gorm.io/gorm"
)

// BatchHitTestingTaskRepository defines the interface for batch hit testing task operations
type BatchHitTestingTaskRepository interface {
	// Create creates a new batch hit testing task
	Create(ctx context.Context, task *model.BatchHitTestingTask) error

	// GetByID retrieves a batch hit testing task by ID
	GetByID(ctx context.Context, taskID string) (*model.BatchHitTestingTask, error)

	// Update updates a batch hit testing task
	Update(ctx context.Context, task *model.BatchHitTestingTask) error

	// UpdateTaskStatus updates the status of a batch hit testing task
	UpdateTaskStatus(ctx context.Context, taskID, status string, startedAt, finishedAt *time.Time) error

	// UpdateQueryTaskStatus updates the status of a query task
	UpdateQueryTaskStatus(ctx context.Context, taskID string, queryIndex int, status string, result *model.QueryTask) error

	// ListByOrganizationID lists batch hit testing tasks by organization ID
	ListByOrganizationID(ctx context.Context, organizationID string, page, limit int) ([]*model.BatchHitTestingTask, int64, error)

	// ListByDatasetID lists batch hit testing tasks by dataset ID
	ListByDatasetID(ctx context.Context, datasetID string, page, limit int) ([]*model.BatchHitTestingTask, int64, error)

	// WithTx returns a new repository with transaction
	WithTx(tx *gorm.DB) BatchHitTestingTaskRepository
}

// batchHitTestingTaskRepository implements BatchHitTestingTaskRepository
type batchHitTestingTaskRepository struct {
	db *gorm.DB
}

// NewBatchHitTestingTaskRepository creates a new batch hit testing task repository
func NewBatchHitTestingTaskRepository(db *gorm.DB) BatchHitTestingTaskRepository {
	return &batchHitTestingTaskRepository{db: db}
}

// Create creates a new batch hit testing task
func (r *batchHitTestingTaskRepository) Create(ctx context.Context, task *model.BatchHitTestingTask) error {
	return r.db.WithContext(ctx).Create(task).Error
}

// GetByID retrieves a batch hit testing task by ID
func (r *batchHitTestingTaskRepository) GetByID(ctx context.Context, taskID string) (*model.BatchHitTestingTask, error) {
	var task model.BatchHitTestingTask
	err := r.db.WithContext(ctx).Where("task_id = ?", taskID).First(&task).Error
	if err != nil {
		return nil, err
	}
	return &task, nil
}

// Update updates a batch hit testing task
func (r *batchHitTestingTaskRepository) Update(ctx context.Context, task *model.BatchHitTestingTask) error {
	return r.db.WithContext(ctx).Save(task).Error
}

// UpdateTaskStatus updates the status of a batch hit testing task
func (r *batchHitTestingTaskRepository) UpdateTaskStatus(ctx context.Context, taskID, status string, startedAt, finishedAt *time.Time) error {
	updates := map[string]interface{}{
		"status": status,
	}

	if startedAt != nil {
		updates["started_at"] = startedAt
	}

	if finishedAt != nil {
		updates["finished_at"] = finishedAt
	}

	return r.db.WithContext(ctx).Model(&model.BatchHitTestingTask{}).Where("task_id = ?", taskID).Updates(updates).Error
}

// UpdateQueryTaskStatus updates the status of a query task
func (r *batchHitTestingTaskRepository) UpdateQueryTaskStatus(ctx context.Context, taskID string, queryIndex int, status string, result *model.QueryTask) error {
	// Get the task first
	task, err := r.GetByID(ctx, taskID)
	if err != nil {
		return err
	}

	// Update the query task
	if queryIndex >= len(task.Queries) {
		return nil // Index out of range, nothing to update
	}

	task.Queries[queryIndex] = *result

	// Save the updated task
	return r.Update(ctx, task)
}

// ListByTenantID lists batch hit testing tasks by tenant ID
func (r *batchHitTestingTaskRepository) ListByOrganizationID(ctx context.Context, organizationID string, page, limit int) ([]*model.BatchHitTestingTask, int64, error) {
	var tasks []*model.BatchHitTestingTask
	var total int64

	offset := (page - 1) * limit

	query := r.db.WithContext(ctx).Model(&model.BatchHitTestingTask{}).Where("organization_id = ?", organizationID)

	// Get total count
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// Get paginated results
	err := query.Order("created_at DESC").Offset(offset).Limit(limit).Find(&tasks).Error
	return tasks, total, err
}

// ListByDatasetID lists batch hit testing tasks by dataset ID
func (r *batchHitTestingTaskRepository) ListByDatasetID(ctx context.Context, datasetID string, page, limit int) ([]*model.BatchHitTestingTask, int64, error) {
	var tasks []*model.BatchHitTestingTask
	var total int64

	offset := (page - 1) * limit

	query := r.db.WithContext(ctx).Model(&model.BatchHitTestingTask{}).Where("dataset_id = ?", datasetID)

	// Get total count
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// Get paginated results
	err := query.Order("created_at DESC").Offset(offset).Limit(limit).Find(&tasks).Error
	return tasks, total, err
}

// WithTx returns a new repository with transaction
func (r *batchHitTestingTaskRepository) WithTx(tx *gorm.DB) BatchHitTestingTaskRepository {
	return NewBatchHitTestingTaskRepository(tx)
}
