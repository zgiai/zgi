package service

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type videoTaskRecord struct {
	ID               uuid.UUID      `gorm:"column:id;type:uuid;default:public.uuid_generate_v4();primaryKey"`
	OrganizationID   uuid.UUID      `gorm:"column:organization_id;type:uuid;not null"`
	AccountID        uuid.UUID      `gorm:"column:account_id;type:uuid;not null"`
	WorkspaceID      *uuid.UUID     `gorm:"column:workspace_id;type:uuid"`
	TaskID           string         `gorm:"column:task_id"`
	UpstreamTaskID   string         `gorm:"column:upstream_task_id"`
	Provider         string         `gorm:"column:provider"`
	Model            string         `gorm:"column:model"`
	ModelLabel       string         `gorm:"column:model_label"`
	Prompt           string         `gorm:"column:prompt"`
	Status           string         `gorm:"column:status"`
	VideoURL         string         `gorm:"column:video_url"`
	ErrorMessage     string         `gorm:"column:error_message"`
	DurationSeconds  int            `gorm:"column:duration_seconds"`
	Resolution       string         `gorm:"column:resolution"`
	Ratio            string         `gorm:"column:ratio"`
	HasInputVideo    bool           `gorm:"column:has_input_video"`
	GenerateAudio    bool           `gorm:"column:generate_audio"`
	Voice            string         `gorm:"column:voice"`
	EstimatedCredits int64          `gorm:"column:estimated_credits"`
	ActualCredits    int64          `gorm:"column:actual_credits"`
	RequestPayload   datatypes.JSON `gorm:"column:request_payload;type:jsonb"`
	ResponsePayload  datatypes.JSON `gorm:"column:response_payload;type:jsonb"`
	CreatedAt        time.Time      `gorm:"column:created_at"`
	UpdatedAt        time.Time      `gorm:"column:updated_at"`
	CompletedAt      *time.Time     `gorm:"column:completed_at"`
}

func (videoTaskRecord) TableName() string {
	return "video_runtime_tasks"
}

type taskRepository struct {
	db *gorm.DB
}

type taskListParams struct {
	Limit           int
	Search          string
	BeforeCreatedAt *time.Time
	BeforeID        *uuid.UUID
}

type taskListPage struct {
	Records []videoTaskRecord
	Total   int64
	HasMore bool
}

func newTaskRepository(db *gorm.DB) *taskRepository {
	return &taskRepository{db: db}
}

func (r *taskRepository) create(ctx context.Context, record *videoTaskRecord) error {
	return r.db.WithContext(ctx).Create(record).Error
}

func (r *taskRepository) list(ctx context.Context, scope Scope, params taskListParams) (taskListPage, error) {
	query := r.db.WithContext(ctx).
		Model(&videoTaskRecord{}).
		Where("organization_id = ? AND account_id = ?", scope.OrganizationID, scope.AccountID)
	if scope.WorkspaceID != nil && *scope.WorkspaceID != uuid.Nil {
		query = query.Where("workspace_id = ?", *scope.WorkspaceID)
	}
	if search := strings.TrimSpace(params.Search); search != "" {
		pattern := "%" + escapeLikePattern(strings.ToLower(search)) + "%"
		query = query.Where(`(
			LOWER(task_id) LIKE ? ESCAPE '\' OR
			LOWER(provider) LIKE ? ESCAPE '\' OR
			LOWER(model) LIKE ? ESCAPE '\' OR
			LOWER(model_label) LIKE ? ESCAPE '\' OR
			LOWER(prompt) LIKE ? ESCAPE '\'
		)`, pattern, pattern, pattern, pattern, pattern)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return taskListPage{}, err
	}
	if params.BeforeCreatedAt != nil && params.BeforeID != nil {
		query = query.Where(
			"created_at < ? OR (created_at = ? AND id < ?)",
			*params.BeforeCreatedAt,
			*params.BeforeCreatedAt,
			*params.BeforeID,
		)
	}

	limit := params.Limit
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	var records []videoTaskRecord
	if err := query.Order("created_at DESC").Order("id DESC").Limit(limit + 1).Find(&records).Error; err != nil {
		return taskListPage{}, err
	}
	hasMore := len(records) > limit
	if hasMore {
		records = records[:limit]
	}
	return taskListPage{Records: records, Total: total, HasMore: hasMore}, nil
}

func escapeLikePattern(value string) string {
	return strings.NewReplacer("\\", "\\\\", "%", "\\%", "_", "\\_").Replace(value)
}

func (r *taskRepository) findByTaskID(ctx context.Context, scope Scope, taskID string) (*videoTaskRecord, error) {
	query := r.db.WithContext(ctx).
		Where("organization_id = ? AND account_id = ? AND task_id = ?", scope.OrganizationID, scope.AccountID, taskID)
	if scope.WorkspaceID != nil && *scope.WorkspaceID != uuid.Nil {
		query = query.Where("workspace_id = ?", *scope.WorkspaceID)
	}
	var record videoTaskRecord
	if err := query.Take(&record).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, ErrTaskNotFound
		}
		return nil, err
	}
	return &record, nil
}

func (r *taskRepository) deleteByTaskID(ctx context.Context, scope Scope, taskID string) error {
	query := r.db.WithContext(ctx).
		Where("organization_id = ? AND account_id = ? AND task_id = ?", scope.OrganizationID, scope.AccountID, taskID)
	if scope.WorkspaceID != nil && *scope.WorkspaceID != uuid.Nil {
		query = query.Where("workspace_id = ?", *scope.WorkspaceID)
	}
	result := query.Delete(&videoTaskRecord{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrTaskNotFound
	}
	return nil
}

func (r *taskRepository) listActiveForPolling(ctx context.Context, staleBefore time.Time, submitExpiredBefore time.Time, limit int) ([]videoTaskRecord, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	var records []videoTaskRecord
	err := r.db.WithContext(ctx).
		Where("status IN ?", []string{"pending", "running", "processing", "in_progress"}).
		Where("updated_at <= ?", staleBefore).
		Where("upstream_task_id <> '' OR created_at <= ?", submitExpiredBefore).
		Order("updated_at ASC").
		Limit(limit).
		Find(&records).Error
	if err != nil {
		return nil, err
	}
	return records, nil
}
func (r *taskRepository) save(ctx context.Context, record *videoTaskRecord) error {
	return r.db.WithContext(ctx).Save(record).Error
}
