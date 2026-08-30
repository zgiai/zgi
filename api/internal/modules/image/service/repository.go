package service

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type imageTaskRecord struct {
	ID              uuid.UUID      `gorm:"column:id;type:uuid;default:public.uuid_generate_v4();primaryKey"`
	OrganizationID  uuid.UUID      `gorm:"column:organization_id;type:uuid;not null"`
	AccountID       uuid.UUID      `gorm:"column:account_id;type:uuid;not null"`
	WorkspaceID     *uuid.UUID     `gorm:"column:workspace_id;type:uuid"`
	TaskID          string         `gorm:"column:task_id"`
	ClientRequestID string         `gorm:"column:client_request_id"`
	ConversationID  string         `gorm:"column:conversation_id"`
	MessageID       string         `gorm:"column:message_id"`
	Provider        string         `gorm:"column:provider"`
	Model           string         `gorm:"column:model"`
	ModelLabel      string         `gorm:"column:model_label"`
	Prompt          string         `gorm:"column:prompt"`
	Status          string         `gorm:"column:status"`
	Size            string         `gorm:"column:size"`
	Count           int            `gorm:"column:count"`
	GenerationMode  string         `gorm:"column:generation_mode"`
	MaxImages       *int           `gorm:"column:max_images"`
	Files           datatypes.JSON `gorm:"column:files;type:jsonb"`
	ReferenceImage  datatypes.JSON `gorm:"column:reference_image;type:jsonb"`
	ErrorMessage    string         `gorm:"column:error_message"`
	RequestPayload  datatypes.JSON `gorm:"column:request_payload;type:jsonb"`
	ResponsePayload datatypes.JSON `gorm:"column:response_payload;type:jsonb"`
	CreatedAt       time.Time      `gorm:"column:created_at"`
	UpdatedAt       time.Time      `gorm:"column:updated_at"`
	CompletedAt     *time.Time     `gorm:"column:completed_at"`
}

func (imageTaskRecord) TableName() string {
	return "image_runtime_tasks"
}

type imageTaskRepository struct {
	db *gorm.DB
}

type imageTaskListParams struct {
	Limit           int
	Search          string
	BeforeCreatedAt *time.Time
	BeforeID        *uuid.UUID
	ActiveOnly      bool
}

type imageTaskListPage struct {
	Records []imageTaskRecord
	Total   int64
	HasMore bool
}

func newImageTaskRepository(db *gorm.DB) *imageTaskRepository {
	return &imageTaskRepository{db: db}
}

func (r *imageTaskRepository) createIfActiveBelowLimit(ctx context.Context, scope Scope, record *imageTaskRecord, limit int64) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if tx.Dialector.Name() == "postgres" {
			if err := tx.Exec("LOCK TABLE public.image_runtime_tasks IN SHARE ROW EXCLUSIVE MODE").Error; err != nil {
				return err
			}
		}
		var active int64
		query := scopedImageTaskQuery(tx.WithContext(ctx), scope).Model(&imageTaskRecord{}).
			Where("status IN ?", activeImageTaskStatuses())
		if err := query.Count(&active).Error; err != nil {
			return err
		}
		if active >= limit {
			return ErrTaskConflict
		}
		return tx.Create(record).Error
	})
}

func (r *imageTaskRepository) save(ctx context.Context, record *imageTaskRecord) error {
	return r.db.WithContext(ctx).Save(record).Error
}

func (r *imageTaskRepository) list(ctx context.Context, scope Scope, params imageTaskListParams) (imageTaskListPage, error) {
	query := r.scoped(ctx, scope).Model(&imageTaskRecord{})
	if params.ActiveOnly {
		query = query.Where("status IN ?", activeImageTaskStatuses())
	}
	if search := strings.TrimSpace(params.Search); search != "" {
		pattern := "%" + escapeImageLikePattern(strings.ToLower(search)) + "%"
		query = query.Where(`(
			LOWER(task_id) LIKE ? ESCAPE '\' OR
			LOWER(client_request_id) LIKE ? ESCAPE '\' OR
			LOWER(provider) LIKE ? ESCAPE '\' OR
			LOWER(model) LIKE ? ESCAPE '\' OR
			LOWER(model_label) LIKE ? ESCAPE '\' OR
			LOWER(prompt) LIKE ? ESCAPE '\'
		)`, pattern, pattern, pattern, pattern, pattern, pattern)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return imageTaskListPage{}, err
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
	var records []imageTaskRecord
	if err := query.Order("created_at DESC").Order("id DESC").Limit(limit + 1).Find(&records).Error; err != nil {
		return imageTaskListPage{}, err
	}
	hasMore := len(records) > limit
	if hasMore {
		records = records[:limit]
	}
	return imageTaskListPage{Records: records, Total: total, HasMore: hasMore}, nil
}

func (r *imageTaskRepository) findByTaskID(ctx context.Context, scope Scope, taskID string) (*imageTaskRecord, error) {
	var record imageTaskRecord
	if err := r.scoped(ctx, scope).Where("task_id = ?", strings.TrimSpace(taskID)).Take(&record).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, ErrTaskNotFound
		}
		return nil, err
	}
	return &record, nil
}

func (r *imageTaskRepository) findByClientRequestID(ctx context.Context, scope Scope, clientRequestID string) (*imageTaskRecord, error) {
	var record imageTaskRecord
	if err := r.scoped(ctx, scope).Where("client_request_id = ?", strings.TrimSpace(clientRequestID)).Take(&record).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, ErrTaskNotFound
		}
		return nil, err
	}
	return &record, nil
}

func (r *imageTaskRepository) countActive(ctx context.Context, scope Scope) (int64, error) {
	var count int64
	if err := r.scoped(ctx, scope).Model(&imageTaskRecord{}).Where("status IN ?", activeImageTaskStatuses()).Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

func (r *imageTaskRepository) markRunning(ctx context.Context, taskID string) (bool, error) {
	result := r.db.WithContext(ctx).
		Model(&imageTaskRecord{}).
		Where("task_id = ? AND status = ?", strings.TrimSpace(taskID), "pending").
		Updates(map[string]any{"status": "running", "updated_at": time.Now().UTC()})
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected > 0, nil
}

func (r *imageTaskRepository) cancelByTaskID(ctx context.Context, scope Scope, taskID string) (*imageTaskRecord, error) {
	record, err := r.findByTaskID(ctx, scope, taskID)
	if err != nil {
		return nil, err
	}
	if isTerminalImageTaskStatus(record.Status) {
		return record, nil
	}
	now := time.Now().UTC()
	record.Status = "cancelled"
	record.UpdatedAt = now
	record.CompletedAt = &now
	if err := r.save(ctx, record); err != nil {
		return nil, err
	}
	return record, nil
}

func (r *imageTaskRepository) listExpiredActive(ctx context.Context, expiredBefore time.Time, limit int) ([]imageTaskRecord, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	var records []imageTaskRecord
	err := r.db.WithContext(ctx).
		Where("status IN ?", activeImageTaskStatuses()).
		Where("updated_at <= ?", expiredBefore).
		Order("updated_at ASC").
		Limit(limit).
		Find(&records).Error
	return records, err
}

func (r *imageTaskRepository) scoped(ctx context.Context, scope Scope) *gorm.DB {
	return scopedImageTaskQuery(r.db.WithContext(ctx), scope)
}

func scopedImageTaskQuery(db *gorm.DB, scope Scope) *gorm.DB {
	query := db.Where("organization_id = ? AND account_id = ?", scope.OrganizationID, scope.AccountID)
	if scope.WorkspaceID != nil && *scope.WorkspaceID != uuid.Nil {
		query = query.Where("workspace_id = ?", *scope.WorkspaceID)
	}
	return query
}

func escapeImageLikePattern(value string) string {
	return strings.NewReplacer("\\", "\\\\", "%", "\\%", "_", "\\_").Replace(value)
}
