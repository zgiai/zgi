package repository

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/ginext/internal/modules/dataset/graphflow/model"
	"gorm.io/gorm"
)

type GraphFlowTaskRepository struct {
	db *gorm.DB
}

func NewGraphFlowTaskRepository(db *gorm.DB) *GraphFlowTaskRepository {
	return &GraphFlowTaskRepository{db: db}
}

// CreateTask initializes a new task
func (r *GraphFlowTaskRepository) CreateTask(ctx context.Context, task *model.GraphFlowTask) error {
	return r.db.WithContext(ctx).Create(task).Error
}

// GetByID retrieves a task by its ID
func (r *GraphFlowTaskRepository) GetByID(ctx context.Context, taskID uuid.UUID) (*model.GraphFlowTask, error) {
	var task model.GraphFlowTask
	err := r.db.WithContext(ctx).Where("id = ?", taskID).First(&task).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	return &task, err
}

// UpdateStatus updates the status of a task
func (r *GraphFlowTaskRepository) UpdateStatus(ctx context.Context, taskID uuid.UUID, status string, errorMessage string) error {
	updates := map[string]interface{}{
		"status":     status,
		"updated_at": time.Now(),
	}
	if status == "processing" {
		updates["started_at"] = time.Now()
	}
	if status == "completed" || status == "failed" {
		updates["completed_at"] = time.Now()
	}
	if errorMessage != "" {
		updates["error_message"] = errorMessage
	}

	return r.db.WithContext(ctx).Model(&model.GraphFlowTask{}).Where("id = ?", taskID).Updates(updates).Error
}

// GetByDocumentIDs retrieves the latest task state for multiple documents
func (r *GraphFlowTaskRepository) GetByDocumentIDs(ctx context.Context, documentIDs []uuid.UUID) ([]*model.GraphFlowTask, error) {
	var tasks []*model.GraphFlowTask
	// Using DISTINCT ON to get the latest task for each document
	subQuery := r.db.Model(&model.GraphFlowTask{}).
		Select("DISTINCT ON (document_id) *").
		Where("document_id IN ?", documentIDs).
		Order("document_id, created_at DESC")

	err := r.db.WithContext(ctx).Table("(?) as latest_tasks", subQuery).Find(&tasks).Error
	return tasks, err
}

// GetByDocumentID retrieves the task state for a document
func (r *GraphFlowTaskRepository) GetByDocumentID(ctx context.Context, documentID uuid.UUID) (*model.GraphFlowTask, error) {
	var task model.GraphFlowTask
	err := r.db.WithContext(ctx).Where("document_id = ?", documentID).Order("created_at DESC").First(&task).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	return &task, err
}

// CreateTaskAndReturnID creates a new task and returns the generated ID
func (r *GraphFlowTaskRepository) CreateTaskAndReturnID(ctx context.Context, task *model.GraphFlowTask) (uuid.UUID, error) {
	if task.ID == uuid.Nil {
		task.ID = uuid.New()
	}
	if err := r.db.WithContext(ctx).Create(task).Error; err != nil {
		return uuid.Nil, err
	}
	return task.ID, nil
}

// UpdateTaskCompleted marks a task as completed
func (r *GraphFlowTaskRepository) UpdateTaskCompleted(ctx context.Context, taskID uuid.UUID) error {
	now := time.Now()
	return r.db.WithContext(ctx).Model(&model.GraphFlowTask{}).
		Where("id = ?", taskID).
		Updates(map[string]interface{}{
			"status":       "completed",
			"completed_at": now,
			"updated_at":   now,
			"progress":     100,
		}).Error
}

// UpdateTaskFailed marks a task as failed with error message
func (r *GraphFlowTaskRepository) UpdateTaskFailed(ctx context.Context, taskID uuid.UUID, errorMessage string) error {
	now := time.Now()

	// Get the task to find the related document_id, then update its segments
	var task model.GraphFlowTask
	if err := r.db.WithContext(ctx).Where("id = ?", taskID).First(&task).Error; err == nil && task.DocumentID != uuid.Nil {
		// Cascade the failure state to the document segments
		r.db.WithContext(ctx).Table("document_segments").
			Where("document_id = ?", task.DocumentID).
			Update("graph_indexing_status", "failed")
	}

	return r.db.WithContext(ctx).Model(&model.GraphFlowTask{}).
		Where("id = ?", taskID).
		Updates(map[string]interface{}{
			"status":        "failed",
			"completed_at":  now,
			"updated_at":    now,
			"error_message": errorMessage,
		}).Error
}

// UpdateTaskProcessing marks a task as processing
func (r *GraphFlowTaskRepository) UpdateTaskProcessing(ctx context.Context, taskID uuid.UUID) error {
	now := time.Now()
	return r.db.WithContext(ctx).Model(&model.GraphFlowTask{}).
		Where("id = ?", taskID).
		Updates(map[string]interface{}{
			"status":     "processing",
			"started_at": now,
			"updated_at": now,
		}).Error
}

// GetTasksByDocumentAndTypes retrieves tasks for a document with specific task types
func (r *GraphFlowTaskRepository) GetTasksByDocumentAndTypes(ctx context.Context, documentID uuid.UUID, taskTypes []string) ([]*model.GraphFlowTask, error) {
	var tasks []*model.GraphFlowTask
	err := r.db.WithContext(ctx).
		Where("document_id = ? AND task_type IN ?", documentID, taskTypes).
		Find(&tasks).Error
	return tasks, err
}

// AreTasksCompletedByTypes checks if all tasks of specific types for a document are completed
func (r *GraphFlowTaskRepository) AreTasksCompletedByTypes(ctx context.Context, documentID uuid.UUID, taskTypes []string) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&model.GraphFlowTask{}).
		Where("document_id = ? AND task_type IN ? AND status != ?", documentID, taskTypes, "completed").
		Count(&count).Error
	if err != nil {
		return false, err
	}
	return count == 0, nil
}

// GetIncompleteTasksByTypes retrieves all non-completed tasks of specific types for a document
func (r *GraphFlowTaskRepository) GetIncompleteTasksByTypes(ctx context.Context, documentID uuid.UUID, taskTypes []string) ([]*model.GraphFlowTask, error) {
	var tasks []*model.GraphFlowTask
	err := r.db.WithContext(ctx).
		Where("document_id = ? AND task_type IN ? AND status != ?", documentID, taskTypes, "completed").
		Find(&tasks).Error
	return tasks, err
}

// DeleteByDocumentID deletes all tasks for a document
func (r *GraphFlowTaskRepository) DeleteByDocumentID(ctx context.Context, documentID uuid.UUID) error {
	return r.db.WithContext(ctx).
		Where("document_id = ?", documentID).
		Delete(&model.GraphFlowTask{}).Error
}

// IncrementRetryCount increments the retry count for a task
func (r *GraphFlowTaskRepository) IncrementRetryCount(ctx context.Context, taskID uuid.UUID) error {
	return r.db.WithContext(ctx).Model(&model.GraphFlowTask{}).
		Where("id = ?", taskID).
		Update("retry_count", gorm.Expr("retry_count + 1")).Error
}

// GetPendingTaskByDocumentAndType retrieves a pending task for a document with specific type
func (r *GraphFlowTaskRepository) GetPendingTaskByDocumentAndType(ctx context.Context, documentID uuid.UUID, taskType string) (*model.GraphFlowTask, error) {
	var task model.GraphFlowTask
	err := r.db.WithContext(ctx).
		Where("document_id = ? AND task_type = ? AND status = ?", documentID, taskType, "pending").
		First(&task).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	return &task, err
}

// UpdateTaskProgress updates only the progress count and updated_at
func (r *GraphFlowTaskRepository) UpdateTaskProgress(ctx context.Context, taskID uuid.UUID, progress int) error {
	return r.db.WithContext(ctx).Model(&model.GraphFlowTask{}).
		Where("id = ?", taskID).
		Updates(map[string]interface{}{
			"progress":   progress,
			"updated_at": time.Now(),
		}).Error
}
