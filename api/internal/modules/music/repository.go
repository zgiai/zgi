package music

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/tool_file"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Repository interface {
	Create(context.Context, *Task) error
	Get(context.Context, uuid.UUID) (*Task, error)
	GetScoped(context.Context, Scope, uuid.UUID) (*Task, error)
	GetByRequest(context.Context, Scope, uuid.UUID) (*Task, error)
	ListScoped(context.Context, Scope, ListQuery) ([]*Task, int64, error)
	UpdateGeneratedLyrics(context.Context, uuid.UUID, GeneratedLyrics) error
	Transition(context.Context, uuid.UUID, Status, Status, TaskUpdate) error
	ListIDsByStatus(context.Context, Status, time.Time, int) ([]uuid.UUID, error)
	TouchStatus(context.Context, uuid.UUID, Status) error
	DeleteScopedTerminal(context.Context, Scope, uuid.UUID) error
}

func (r *GormRepository) ListScoped(ctx context.Context, scope Scope, query ListQuery) ([]*Task, int64, error) {
	if scope.OrganizationID == uuid.Nil || scope.WorkspaceID == uuid.Nil || scope.AccountID == uuid.Nil ||
		query.Page <= 0 || query.PageSize <= 0 {
		return nil, 0, ErrInvalidRequest
	}
	db := r.db.WithContext(ctx).Model(&Task{}).Where(
		"organization_id = ? AND workspace_id = ? AND account_id = ?",
		scope.OrganizationID,
		scope.WorkspaceID,
		scope.AccountID,
	)
	if query.Search != "" {
		db = db.Where(`LOWER(prompt) LIKE LOWER(?) ESCAPE '\'`, "%"+escapeMusicTaskSearch(query.Search)+"%")
	}
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	items := make([]*Task, 0, query.PageSize)
	if err := db.Order("created_at DESC, id DESC").
		Offset((query.Page - 1) * query.PageSize).
		Limit(query.PageSize).
		Find(&items).Error; err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

func escapeMusicTaskSearch(value string) string {
	replacer := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return replacer.Replace(value)
}

type GormRepository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) *GormRepository {
	if db == nil {
		panic("music repository requires db")
	}
	return &GormRepository{db: db}
}

func (r *GormRepository) Create(ctx context.Context, task *Task) error {
	if task == nil || task.ID == uuid.Nil {
		return ErrInvalidRequest
	}
	if err := r.db.WithContext(ctx).Create(task).Error; err != nil {
		return err
	}
	return nil
}

func (r *GormRepository) Get(ctx context.Context, id uuid.UUID) (*Task, error) {
	var task Task
	if err := r.db.WithContext(ctx).Where("id = ?", id).Take(&task).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrTaskNotFound
		}
		return nil, err
	}
	return &task, nil
}

func (r *GormRepository) GetScoped(ctx context.Context, scope Scope, id uuid.UUID) (*Task, error) {
	var task Task
	err := r.db.WithContext(ctx).
		Where(
			"id = ? AND organization_id = ? AND workspace_id = ? AND account_id = ?",
			id,
			scope.OrganizationID,
			scope.WorkspaceID,
			scope.AccountID,
		).
		Take(&task).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrTaskNotFound
		}
		return nil, err
	}
	return &task, nil
}

func (r *GormRepository) GetByRequest(ctx context.Context, scope Scope, requestID uuid.UUID) (*Task, error) {
	var task Task
	err := r.db.WithContext(ctx).
		Where(
			"request_id = ? AND organization_id = ? AND workspace_id = ? AND account_id = ?",
			requestID,
			scope.OrganizationID,
			scope.WorkspaceID,
			scope.AccountID,
		).
		Take(&task).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrTaskNotFound
		}
		return nil, err
	}
	return &task, nil
}

func (r *GormRepository) UpdateGeneratedLyrics(ctx context.Context, id uuid.UUID, generated GeneratedLyrics) error {
	if id == uuid.Nil || strings.TrimSpace(generated.Title) == "" || strings.TrimSpace(generated.Lyrics) == "" {
		return ErrInvalidRequest
	}
	result := r.db.WithContext(ctx).Model(&Task{}).
		Where("id = ? AND status = ?", id, StatusGeneratingLyrics).
		Updates(map[string]any{
			"title":      generated.Title,
			"style_tags": datatypes.NewJSONSlice(generated.StyleTags),
			"lyrics":     generated.Lyrics,
			"updated_at": time.Now().UTC(),
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrInvalidTransition
	}
	return nil
}

func (r *GormRepository) Transition(ctx context.Context, id uuid.UUID, from, to Status, update TaskUpdate) error {
	now := time.Now().UTC()
	values := map[string]any{
		"status":     to,
		"updated_at": now,
	}
	if update.FileID != nil {
		values["file_id"] = *update.FileID
	}
	if update.DurationMS > 0 {
		values["duration_ms"] = update.DurationMS
	}
	if len(update.WaveformPeaks) > 0 {
		values["waveform_peaks"] = datatypes.NewJSONSlice(update.WaveformPeaks)
	}
	if update.ErrorCode != "" {
		values["error_code"] = update.ErrorCode
	}
	if update.ErrorMessage != "" {
		values["error_message"] = update.ErrorMessage
	}
	if to == StatusGenerating {
		values["started_at"] = now
	}
	if to == StatusSucceeded || to == StatusFailed {
		values["completed_at"] = now
	}
	result := r.db.WithContext(ctx).Model(&Task{}).
		Where("id = ? AND status = ?", id, from).
		Updates(values)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrInvalidTransition
	}
	return nil
}

func (r *GormRepository) ListIDsByStatus(ctx context.Context, status Status, updatedBefore time.Time, limit int) ([]uuid.UUID, error) {
	if !isReconciledStatus(status) || updatedBefore.IsZero() || limit <= 0 {
		return nil, ErrInvalidRequest
	}
	var ids []uuid.UUID
	if err := r.db.WithContext(ctx).Model(&Task{}).
		Where("status = ? AND updated_at <= ?", status, updatedBefore).
		Order("updated_at ASC").
		Limit(limit).
		Pluck("id", &ids).Error; err != nil {
		return nil, err
	}
	return ids, nil
}

func (r *GormRepository) TouchStatus(ctx context.Context, id uuid.UUID, status Status) error {
	if id == uuid.Nil || !isReconciledStatus(status) {
		return ErrInvalidRequest
	}
	result := r.db.WithContext(ctx).Model(&Task{}).
		Where("id = ? AND status = ?", id, status).
		Update("updated_at", time.Now().UTC())
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrInvalidTransition
	}
	return nil
}

func (r *GormRepository) DeleteScopedTerminal(ctx context.Context, scope Scope, id uuid.UUID) error {
	if scope.OrganizationID == uuid.Nil || scope.WorkspaceID == uuid.Nil || scope.AccountID == uuid.Nil || id == uuid.Nil {
		return ErrInvalidRequest
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var task Task
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where(
				"id = ? AND organization_id = ? AND workspace_id = ? AND account_id = ?",
				id,
				scope.OrganizationID,
				scope.WorkspaceID,
				scope.AccountID,
			).
			Take(&task).Error
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrTaskNotFound
			}
			return err
		}
		if !isDeletableStatus(task.Status) {
			return ErrTaskNotDeletable
		}
		if task.Status == StatusSucceeded && task.FileID == nil {
			return ErrTaskAssetMissing
		}
		if err := tx.Delete(&task).Error; err != nil {
			return err
		}
		if task.FileID == nil {
			return nil
		}
		result := tx.Where(
			"id = ? AND tenant_id = ? AND user_id = ?",
			task.FileID.String(),
			scope.OrganizationID.String(),
			scope.AccountID.String(),
		).Delete(&tool_file.ToolFile{})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return fmt.Errorf("delete music tool file metadata: deleted %d rows, want 1", result.RowsAffected)
		}
		return nil
	})
}

func isReconciledStatus(status Status) bool {
	return status == StatusQueued || status == StatusGeneratingLyrics || status == StatusGenerating || status == StatusCompensationPending
}
