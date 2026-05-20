package repository

import (
	"context"
	"fmt"

	automationdto "github.com/zgiai/ginext/internal/modules/automation/dto"
	automationmodel "github.com/zgiai/ginext/internal/modules/automation/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// RunRepository handles persistence for task runs and action runs.
type RunRepository interface {
	CreateTaskRun(ctx context.Context, tx *gorm.DB, run *automationmodel.AutomationTaskRun) error
	CreateTaskRunIfNotExists(ctx context.Context, tx *gorm.DB, run *automationmodel.AutomationTaskRun) (bool, error)
	UpdateTaskRun(ctx context.Context, tx *gorm.DB, run *automationmodel.AutomationTaskRun) error
	GetTaskRunByID(ctx context.Context, db *gorm.DB, runID string) (*automationmodel.AutomationTaskRun, error)
	ListTaskRuns(ctx context.Context, db *gorm.DB, scope automationdto.TaskScope, taskID string, page, limit int) ([]*automationmodel.AutomationTaskRun, error)
	CountTaskRuns(ctx context.Context, db *gorm.DB, scope automationdto.TaskScope, taskID string) (int64, error)
	CountTaskRunsByTaskID(ctx context.Context, db *gorm.DB, taskID string) (int64, error)

	CreateActionRun(ctx context.Context, tx *gorm.DB, run *automationmodel.AutomationActionRun) error
	UpdateActionRun(ctx context.Context, tx *gorm.DB, run *automationmodel.AutomationActionRun) error
	ListActionRunsByTaskRunID(ctx context.Context, db *gorm.DB, taskRunID string) ([]*automationmodel.AutomationActionRun, error)
}

type runRepository struct {
	db *gorm.DB
}

// NewRunRepository creates a run repository backed by GORM.
func NewRunRepository(db *gorm.DB) RunRepository {
	return &runRepository{db: db}
}

func (r *runRepository) CreateTaskRun(ctx context.Context, tx *gorm.DB, run *automationmodel.AutomationTaskRun) error {
	if err := r.session(tx).WithContext(ctx).Create(run).Error; err != nil {
		return fmt.Errorf("create automation task run: %w", err)
	}
	return nil
}

func (r *runRepository) CreateTaskRunIfNotExists(ctx context.Context, tx *gorm.DB, run *automationmodel.AutomationTaskRun) (bool, error) {
	result := r.session(tx).
		WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns: []clause.Column{
				{Name: "task_id"},
				{Name: "scheduled_for"},
				{Name: "trigger_source"},
			},
			DoNothing: true,
		}).
		Create(run)
	if result.Error != nil {
		return false, fmt.Errorf("create automation task run if not exists: %w", result.Error)
	}
	return result.RowsAffected > 0, nil
}

func (r *runRepository) UpdateTaskRun(ctx context.Context, tx *gorm.DB, run *automationmodel.AutomationTaskRun) error {
	if err := r.session(tx).WithContext(ctx).Save(run).Error; err != nil {
		return fmt.Errorf("update automation task run %s: %w", run.ID, err)
	}
	return nil
}

func (r *runRepository) GetTaskRunByID(ctx context.Context, db *gorm.DB, runID string) (*automationmodel.AutomationTaskRun, error) {
	var run automationmodel.AutomationTaskRun
	if err := r.session(db).WithContext(ctx).Where("id = ?", runID).First(&run).Error; err != nil {
		return nil, fmt.Errorf("get automation task run %s: %w", runID, err)
	}
	return &run, nil
}

func (r *runRepository) ListTaskRuns(ctx context.Context, db *gorm.DB, scope automationdto.TaskScope, taskID string, page, limit int) ([]*automationmodel.AutomationTaskRun, error) {
	query := r.session(db).
		WithContext(ctx).
		Model(&automationmodel.AutomationTaskRun{}).
		Joins("JOIN automation_tasks ON automation_tasks.id = automation_task_runs.task_id").
		Where("automation_task_runs.task_id = ?", taskID).
		Where("automation_tasks.organization_id = ?", scope.OrganizationID).
		Where("automation_tasks.workspace_id = ?", scope.WorkspaceID).
		Order("automation_task_runs.created_at DESC")

	offset := paginationOffset(page, limit)
	if limit > 0 {
		query = query.Limit(limit)
	}
	if offset > 0 {
		query = query.Offset(offset)
	}

	var runs []*automationmodel.AutomationTaskRun
	if err := query.Find(&runs).Error; err != nil {
		return nil, fmt.Errorf("list automation task runs for task %s: %w", taskID, err)
	}
	return runs, nil
}

func (r *runRepository) CountTaskRuns(ctx context.Context, db *gorm.DB, scope automationdto.TaskScope, taskID string) (int64, error) {
	var count int64
	if err := r.session(db).
		WithContext(ctx).
		Model(&automationmodel.AutomationTaskRun{}).
		Joins("JOIN automation_tasks ON automation_tasks.id = automation_task_runs.task_id").
		Where("automation_task_runs.task_id = ?", taskID).
		Where("automation_tasks.organization_id = ?", scope.OrganizationID).
		Where("automation_tasks.workspace_id = ?", scope.WorkspaceID).
		Count(&count).Error; err != nil {
		return 0, fmt.Errorf("count automation task runs for task %s in scope: %w", taskID, err)
	}
	return count, nil
}

func (r *runRepository) CountTaskRunsByTaskID(ctx context.Context, db *gorm.DB, taskID string) (int64, error) {
	var count int64
	if err := r.session(db).WithContext(ctx).Model(&automationmodel.AutomationTaskRun{}).Where("task_id = ?", taskID).Count(&count).Error; err != nil {
		return 0, fmt.Errorf("count automation task runs for task %s: %w", taskID, err)
	}
	return count, nil
}

func (r *runRepository) CreateActionRun(ctx context.Context, tx *gorm.DB, run *automationmodel.AutomationActionRun) error {
	if err := r.session(tx).WithContext(ctx).Create(run).Error; err != nil {
		return fmt.Errorf("create automation action run: %w", err)
	}
	return nil
}

func (r *runRepository) UpdateActionRun(ctx context.Context, tx *gorm.DB, run *automationmodel.AutomationActionRun) error {
	if err := r.session(tx).WithContext(ctx).Save(run).Error; err != nil {
		return fmt.Errorf("update automation action run %s: %w", run.ID, err)
	}
	return nil
}

func (r *runRepository) ListActionRunsByTaskRunID(ctx context.Context, db *gorm.DB, taskRunID string) ([]*automationmodel.AutomationActionRun, error) {
	var runs []*automationmodel.AutomationActionRun
	err := r.session(db).
		WithContext(ctx).
		Where("task_run_id = ?", taskRunID).
		Order("created_at ASC").
		Find(&runs).Error
	if err != nil {
		return nil, fmt.Errorf("list automation action runs by task run %s: %w", taskRunID, err)
	}
	return runs, nil
}

func (r *runRepository) session(db *gorm.DB) *gorm.DB {
	if db != nil {
		return db
	}
	return r.db
}
