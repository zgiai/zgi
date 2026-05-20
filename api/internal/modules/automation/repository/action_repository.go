package repository

import (
	"context"
	"fmt"

	automationmodel "github.com/zgiai/zgi/api/internal/modules/automation/model"
	"gorm.io/gorm"
)

// ActionRepository handles persistence for automation task actions.
type ActionRepository interface {
	BatchCreate(ctx context.Context, tx *gorm.DB, actions []*automationmodel.AutomationTaskAction) error
	DeleteByTaskID(ctx context.Context, tx *gorm.DB, taskID string) error
	ListByTaskID(ctx context.Context, db *gorm.DB, taskID string) ([]*automationmodel.AutomationTaskAction, error)
}

type actionRepository struct {
	db *gorm.DB
}

// NewActionRepository creates an action repository backed by GORM.
func NewActionRepository(db *gorm.DB) ActionRepository {
	return &actionRepository{db: db}
}

func (r *actionRepository) BatchCreate(ctx context.Context, tx *gorm.DB, actions []*automationmodel.AutomationTaskAction) error {
	if len(actions) == 0 {
		return nil
	}

	if err := r.session(tx).WithContext(ctx).Create(&actions).Error; err != nil {
		return fmt.Errorf("batch create automation task actions: %w", err)
	}
	return nil
}

func (r *actionRepository) DeleteByTaskID(ctx context.Context, tx *gorm.DB, taskID string) error {
	if taskID == "" {
		return nil
	}
	if err := r.session(tx).WithContext(ctx).Where("task_id = ?", taskID).Delete(&automationmodel.AutomationTaskAction{}).Error; err != nil {
		return fmt.Errorf("delete automation task actions by task %s: %w", taskID, err)
	}
	return nil
}

func (r *actionRepository) ListByTaskID(ctx context.Context, db *gorm.DB, taskID string) ([]*automationmodel.AutomationTaskAction, error) {
	var actions []*automationmodel.AutomationTaskAction
	err := r.session(db).
		WithContext(ctx).
		Where("task_id = ?", taskID).
		Order("action_order ASC").
		Find(&actions).Error
	if err != nil {
		return nil, fmt.Errorf("list automation task actions by task %s: %w", taskID, err)
	}
	return actions, nil
}

func (r *actionRepository) session(db *gorm.DB) *gorm.DB {
	if db != nil {
		return db
	}
	return r.db
}
