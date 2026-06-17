package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	actionmodel "github.com/zgiai/zgi/api/internal/capabilities/actionruntime/model"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type Repository interface {
	CreateRunWithSteps(ctx context.Context, run *actionmodel.ActionRun, steps []*actionmodel.ActionStep) error
	GetRunScoped(ctx context.Context, id, organizationID uuid.UUID, workspaceID *uuid.UUID, accountID uuid.UUID) (*actionmodel.ActionRun, []*actionmodel.ActionStep, error)
	GetRunByIdempotencyKey(ctx context.Context, organizationID uuid.UUID, workspaceID *uuid.UUID, accountID uuid.UUID, capabilityID string, key string) (*actionmodel.ActionRun, []*actionmodel.ActionStep, error)
	UpdateRunFieldsScoped(ctx context.Context, id, organizationID uuid.UUID, workspaceID *uuid.UUID, accountID uuid.UUID, updates map[string]interface{}) error
	UpdateStepFields(ctx context.Context, runID, stepID uuid.UUID, updates map[string]interface{}) error
	IsOrganizationMember(ctx context.Context, organizationID, accountID uuid.UUID) (bool, error)
}

type repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) Repository {
	return &repository{db: db}
}

func (r *repository) CreateRunWithSteps(ctx context.Context, run *actionmodel.ActionRun, steps []*actionmodel.ActionStep) error {
	if run == nil {
		return fmt.Errorf("action run is required")
	}
	prepareRun(run)
	for index, step := range steps {
		prepareStep(run.ID, index, step)
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(run).Error; err != nil {
			return fmt.Errorf("failed to create action run: %w", err)
		}
		if len(steps) == 0 {
			return nil
		}
		if err := tx.Create(&steps).Error; err != nil {
			return fmt.Errorf("failed to create action steps: %w", err)
		}
		return nil
	})
}

func (r *repository) GetRunScoped(ctx context.Context, id, organizationID uuid.UUID, workspaceID *uuid.UUID, accountID uuid.UUID) (*actionmodel.ActionRun, []*actionmodel.ActionStep, error) {
	var run actionmodel.ActionRun
	err := applyWorkspaceScope(r.db.WithContext(ctx), workspaceID).
		Where("id = ? AND organization_id = ? AND account_id = ? AND deleted_at IS NULL", id, organizationID, accountID).
		Take(&run).Error
	if err != nil {
		return nil, nil, wrapNotFound(err, "action run")
	}
	steps, err := r.listSteps(ctx, id)
	if err != nil {
		return nil, nil, err
	}
	return &run, steps, nil
}

func (r *repository) GetRunByIdempotencyKey(ctx context.Context, organizationID uuid.UUID, workspaceID *uuid.UUID, accountID uuid.UUID, capabilityID string, key string) (*actionmodel.ActionRun, []*actionmodel.ActionStep, error) {
	var run actionmodel.ActionRun
	err := applyWorkspaceScope(r.db.WithContext(ctx), workspaceID).
		Where("organization_id = ? AND account_id = ? AND capability_id = ? AND idempotency_key = ? AND deleted_at IS NULL", organizationID, accountID, capabilityID, key).
		Order("created_at DESC, id DESC").
		Take(&run).Error
	if err != nil {
		return nil, nil, wrapNotFound(err, "action run")
	}
	steps, err := r.listSteps(ctx, run.ID)
	if err != nil {
		return nil, nil, err
	}
	return &run, steps, nil
}

func (r *repository) UpdateRunFieldsScoped(ctx context.Context, id, organizationID uuid.UUID, workspaceID *uuid.UUID, accountID uuid.UUID, updates map[string]interface{}) error {
	if len(updates) == 0 {
		return nil
	}
	normalized, err := normalizeUpdates(updates)
	if err != nil {
		return err
	}
	normalized["updated_at"] = time.Now()
	result := applyWorkspaceScope(r.db.WithContext(ctx).Model(&actionmodel.ActionRun{}), workspaceID).
		Where("id = ? AND organization_id = ? AND account_id = ? AND deleted_at IS NULL", id, organizationID, accountID).
		Updates(normalized)
	if result.Error != nil {
		return fmt.Errorf("failed to update action run: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func applyWorkspaceScope(db *gorm.DB, workspaceID *uuid.UUID) *gorm.DB {
	if workspaceID == nil || *workspaceID == uuid.Nil {
		return db.Where("workspace_id IS NULL")
	}
	return db.Where("workspace_id = ?", *workspaceID)
}

func (r *repository) UpdateStepFields(ctx context.Context, runID, stepID uuid.UUID, updates map[string]interface{}) error {
	if len(updates) == 0 {
		return nil
	}
	normalized, err := normalizeUpdates(updates)
	if err != nil {
		return err
	}
	normalized["updated_at"] = time.Now()
	result := r.db.WithContext(ctx).Model(&actionmodel.ActionStep{}).
		Where("id = ? AND run_id = ?", stepID, runID).
		Updates(normalized)
	if result.Error != nil {
		return fmt.Errorf("failed to update action step: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (r *repository) IsOrganizationMember(ctx context.Context, organizationID, accountID uuid.UUID) (bool, error) {
	var count int64
	if err := r.db.WithContext(ctx).Table("members").
		Where("organization_id = ? AND account_id = ?", organizationID, accountID).
		Count(&count).Error; err != nil {
		return false, fmt.Errorf("failed to check action runtime organization membership: %w", err)
	}
	return count > 0, nil
}

func (r *repository) listSteps(ctx context.Context, runID uuid.UUID) ([]*actionmodel.ActionStep, error) {
	var steps []*actionmodel.ActionStep
	if err := r.db.WithContext(ctx).
		Where("run_id = ?", runID).
		Order("step_index ASC, created_at ASC, id ASC").
		Find(&steps).Error; err != nil {
		return nil, fmt.Errorf("failed to list action steps: %w", err)
	}
	return steps, nil
}

func prepareRun(run *actionmodel.ActionRun) {
	now := time.Now()
	if run.ID == uuid.Nil {
		run.ID = uuid.New()
	}
	if run.Status == "" {
		run.Status = actionmodel.ActionRunStatusPlanned
	}
	if run.RiskLevel == "" {
		run.RiskLevel = actionmodel.RiskLevelLow
	}
	if run.Resources == nil {
		run.Resources = map[string]interface{}{}
	}
	if run.Arguments == nil {
		run.Arguments = map[string]interface{}{}
	}
	if run.Ledger == nil {
		run.Ledger = map[string]interface{}{}
	}
	if run.Metadata == nil {
		run.Metadata = map[string]interface{}{}
	}
	run.CreatedAt = now
	run.UpdatedAt = now
}

func prepareStep(runID uuid.UUID, index int, step *actionmodel.ActionStep) {
	now := time.Now()
	if step.ID == uuid.Nil {
		step.ID = uuid.New()
	}
	step.RunID = runID
	if step.StepIndex == 0 {
		step.StepIndex = index
	}
	if step.Status == "" {
		step.Status = actionmodel.ActionStepStatusPending
	}
	if step.RiskLevel == "" {
		step.RiskLevel = actionmodel.RiskLevelLow
	}
	if step.Input == nil {
		step.Input = map[string]interface{}{}
	}
	if step.Output == nil {
		step.Output = map[string]interface{}{}
	}
	if step.Metadata == nil {
		step.Metadata = map[string]interface{}{}
	}
	step.CreatedAt = now
	step.UpdatedAt = now
}

func normalizeUpdates(updates map[string]interface{}) (map[string]interface{}, error) {
	out := make(map[string]interface{}, len(updates))
	for key, value := range updates {
		switch key {
		case "resources", "arguments", "ledger", "metadata", "input", "output":
			encoded, err := json.Marshal(value)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal action runtime %s: %w", key, err)
			}
			out[key] = datatypes.JSON(encoded)
		default:
			out[key] = value
		}
	}
	return out, nil
}

func wrapNotFound(err error, name string) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return fmt.Errorf("%s not found: %w", name, gorm.ErrRecordNotFound)
	}
	return fmt.Errorf("failed to get %s: %w", name, err)
}
