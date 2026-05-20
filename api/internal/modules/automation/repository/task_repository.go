package repository

import (
	"context"
	"fmt"
	"strings"
	"time"

	automationdto "github.com/zgiai/ginext/internal/modules/automation/dto"
	automationmodel "github.com/zgiai/ginext/internal/modules/automation/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// TaskRepository handles persistence for automation task definitions.
type TaskRepository interface {
	Create(ctx context.Context, tx *gorm.DB, task *automationmodel.AutomationTask) error
	Update(ctx context.Context, tx *gorm.DB, task *automationmodel.AutomationTask) error
	UpdateLastRun(ctx context.Context, tx *gorm.DB, taskID string, finishedAt time.Time, status string) error
	Delete(ctx context.Context, tx *gorm.DB, taskID string) error
	GetByID(ctx context.Context, db *gorm.DB, scope automationdto.TaskScope, taskID string) (*automationmodel.AutomationTask, error)
	GetByIDAnyScope(ctx context.Context, db *gorm.DB, taskID string) (*automationmodel.AutomationTask, error)
	List(ctx context.Context, db *gorm.DB, filter automationdto.TaskFilter) ([]*automationmodel.AutomationTask, error)
	Count(ctx context.Context, db *gorm.DB, filter automationdto.TaskFilter) (int64, error)
	ListDueTasksForDispatch(ctx context.Context, tx *gorm.DB, now time.Time, limit int) ([]*automationmodel.AutomationTask, error)
}

type taskRepository struct {
	db *gorm.DB
}

// NewTaskRepository creates a task repository backed by GORM.
func NewTaskRepository(db *gorm.DB) TaskRepository {
	return &taskRepository{db: db}
}

func (r *taskRepository) Create(ctx context.Context, tx *gorm.DB, task *automationmodel.AutomationTask) error {
	if err := r.session(tx).WithContext(ctx).Create(task).Error; err != nil {
		return fmt.Errorf("create automation task: %w", err)
	}
	return nil
}

func (r *taskRepository) Update(ctx context.Context, tx *gorm.DB, task *automationmodel.AutomationTask) error {
	if err := r.session(tx).WithContext(ctx).Save(task).Error; err != nil {
		return fmt.Errorf("update automation task %s: %w", task.ID, err)
	}
	return nil
}

func (r *taskRepository) UpdateLastRun(ctx context.Context, tx *gorm.DB, taskID string, finishedAt time.Time, status string) error {
	if taskID == "" {
		return nil
	}
	updates := map[string]interface{}{
		"last_run_at":     finishedAt,
		"last_run_status": status,
	}
	if err := r.session(tx).WithContext(ctx).Model(&automationmodel.AutomationTask{}).Where("id = ?", taskID).UpdateColumns(updates).Error; err != nil {
		return fmt.Errorf("update automation task %s last run: %w", taskID, err)
	}
	return nil
}

func (r *taskRepository) Delete(ctx context.Context, tx *gorm.DB, taskID string) error {
	if err := r.session(tx).WithContext(ctx).Where("id = ?", taskID).Delete(&automationmodel.AutomationTask{}).Error; err != nil {
		return fmt.Errorf("delete automation task %s: %w", taskID, err)
	}
	return nil
}

func (r *taskRepository) GetByID(ctx context.Context, db *gorm.DB, scope automationdto.TaskScope, taskID string) (*automationmodel.AutomationTask, error) {
	var task automationmodel.AutomationTask
	err := r.session(db).
		WithContext(ctx).
		Where("id = ?", taskID).
		Where("organization_id = ?", scope.OrganizationID).
		Where("workspace_id = ?", scope.WorkspaceID).
		First(&task).Error
	if err != nil {
		return nil, fmt.Errorf("get automation task %s by scope: %w", taskID, err)
	}
	return &task, nil
}

func (r *taskRepository) GetByIDAnyScope(ctx context.Context, db *gorm.DB, taskID string) (*automationmodel.AutomationTask, error) {
	var task automationmodel.AutomationTask
	err := r.session(db).
		WithContext(ctx).
		Where("id = ?", taskID).
		First(&task).Error
	if err != nil {
		return nil, fmt.Errorf("get automation task %s: %w", taskID, err)
	}
	return &task, nil
}

func (r *taskRepository) List(ctx context.Context, db *gorm.DB, filter automationdto.TaskFilter) ([]*automationmodel.AutomationTask, error) {
	query := r.applyTaskFilter(r.session(db).WithContext(ctx), filter).
		Order("updated_at DESC")

	offset := paginationOffset(filter.Page, filter.Limit)
	if filter.Limit > 0 {
		query = query.Limit(filter.Limit)
	}
	if offset > 0 {
		query = query.Offset(offset)
	}

	var tasks []*automationmodel.AutomationTask
	if err := query.Find(&tasks).Error; err != nil {
		return nil, fmt.Errorf("list automation tasks: %w", err)
	}
	return tasks, nil
}

func (r *taskRepository) Count(ctx context.Context, db *gorm.DB, filter automationdto.TaskFilter) (int64, error) {
	query := r.applyTaskFilter(r.session(db).WithContext(ctx), filter)

	var count int64
	if err := query.Model(&automationmodel.AutomationTask{}).Count(&count).Error; err != nil {
		return 0, fmt.Errorf("count automation tasks: %w", err)
	}
	return count, nil
}

func (r *taskRepository) ListDueTasksForDispatch(ctx context.Context, tx *gorm.DB, now time.Time, limit int) ([]*automationmodel.AutomationTask, error) {
	if limit <= 0 {
		limit = 200
	}

	var tasks []*automationmodel.AutomationTask
	err := r.session(tx).
		WithContext(ctx).
		Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
		Where("status = ?", automationmodel.AutomationTaskStatusActive).
		Where("next_run_at IS NOT NULL").
		Where("next_run_at <= ?", now).
		Order("next_run_at ASC").
		Limit(limit).
		Find(&tasks).Error
	if err != nil {
		return nil, fmt.Errorf("list due automation tasks for dispatch: %w", err)
	}
	return tasks, nil
}

func (r *taskRepository) session(db *gorm.DB) *gorm.DB {
	if db != nil {
		return db
	}
	return r.db
}

func (r *taskRepository) applyTaskFilter(query *gorm.DB, filter automationdto.TaskFilter) *gorm.DB {
	query = query.
		Model(&automationmodel.AutomationTask{}).
		Where("organization_id = ?", filter.OrganizationID).
		Where("workspace_id = ?", filter.WorkspaceID)

	if len(filter.Statuses) > 0 {
		statuses := make([]string, 0, len(filter.Statuses))
		for _, status := range filter.Statuses {
			if value := strings.TrimSpace(string(status)); value != "" {
				statuses = append(statuses, value)
			}
		}
		if len(statuses) > 0 {
			query = query.Where("status IN ?", statuses)
		}
	}

	return query
}

func paginationOffset(page, limit int) int {
	if page <= 1 || limit <= 0 {
		return 0
	}
	return (page - 1) * limit
}
