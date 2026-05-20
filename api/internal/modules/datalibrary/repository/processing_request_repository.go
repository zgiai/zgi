package repository

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/datalibrary/model"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type ProcessingRequestListFilter struct {
	OrganizationID string
	AssetID        uuid.UUID
	TargetLevel    string
	Status         string
	ExecutorKey    string
	Limit          int
	Offset         int
}

type ProcessingRequestStatusPatch struct {
	OrganizationID    string
	Status            string
	AllowedFrom       []string
	ExecutorKey       *string
	ErrorCode         *string
	ErrorMessage      *string
	AttemptCountDelta int
	QueuedAt          *time.Time
	StartedAt         *time.Time
	CompletedAt       *time.Time
	FailedAt          *time.Time
	CanceledAt        *time.Time
	ExecutionMetadata map[string]any
}

type ProcessingRequestClaimFilter struct {
	OrganizationID string
	ExecutorKey    string
	TargetLevels   []string
}

type ProcessingRequestStatusSummary struct {
	Status string
	Count  int64
}

type ProcessingRequestQueueSummaryFilter struct {
	OrganizationID string
	TargetLevel    string
	Status         string
	ExecutorKey    string
}

type ProcessingRequestQueueSummary struct {
	TargetLevel     string
	Status          string
	ExecutorKey     string
	Count           int64
	OldestQueuedAt  *time.Time
	OldestCreatedAt *time.Time
	NewestCreatedAt *time.Time
}

type processingRequestQueueSummaryRow struct {
	TargetLevel         string
	Status              string
	ExecutorKey         string
	Count               int64
	OldestQueuedAtText  *string `gorm:"column:oldest_queued_at"`
	OldestCreatedAtText *string `gorm:"column:oldest_created_at"`
	NewestCreatedAtText *string `gorm:"column:newest_created_at"`
}

type ProcessingRequestRepository interface {
	Create(ctx context.Context, item *model.ProcessingRequest) error
	GetByID(ctx context.Context, id uuid.UUID) (*model.ProcessingRequest, error)
	List(ctx context.Context, filter ProcessingRequestListFilter) ([]*model.ProcessingRequest, int64, error)
	StatusSummaryByAssetID(ctx context.Context, organizationID string, assetID uuid.UUID) ([]ProcessingRequestStatusSummary, error)
	QueueSummary(ctx context.Context, filter ProcessingRequestQueueSummaryFilter) ([]ProcessingRequestQueueSummary, error)
	TransitionStatus(ctx context.Context, id uuid.UUID, patch ProcessingRequestStatusPatch) (*model.ProcessingRequest, error)
	ClaimNextQueued(ctx context.Context, filter ProcessingRequestClaimFilter) (*model.ProcessingRequest, error)
}

type processingRequestRepository struct {
	db *gorm.DB
}

func NewProcessingRequestRepository(db *gorm.DB) ProcessingRequestRepository {
	return &processingRequestRepository{db: db}
}

func (r *processingRequestRepository) Create(ctx context.Context, item *model.ProcessingRequest) error {
	return r.db.WithContext(ctx).Create(item).Error
}

func (r *processingRequestRepository) GetByID(ctx context.Context, id uuid.UUID) (*model.ProcessingRequest, error) {
	var item model.ProcessingRequest
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&item).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &item, nil
}

func (r *processingRequestRepository) List(ctx context.Context, filter ProcessingRequestListFilter) ([]*model.ProcessingRequest, int64, error) {
	query := r.db.WithContext(ctx).Model(&model.ProcessingRequest{}).
		Where("organization_id = ?", filter.OrganizationID)
	if filter.AssetID != uuid.Nil {
		query = query.Where("asset_id = ?", filter.AssetID)
	}
	if filter.TargetLevel != "" {
		query = query.Where("target_level = ?", filter.TargetLevel)
	}
	if filter.Status != "" {
		query = query.Where("status = ?", filter.Status)
	}
	if filter.ExecutorKey != "" {
		query = query.Where("executor_key = ?", filter.ExecutorKey)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	limit := filter.Limit
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}

	var items []*model.ProcessingRequest
	if err := query.Order("created_at DESC").Limit(limit).Offset(offset).Find(&items).Error; err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

func (r *processingRequestRepository) StatusSummaryByAssetID(ctx context.Context, organizationID string, assetID uuid.UUID) ([]ProcessingRequestStatusSummary, error) {
	if organizationID == "" || assetID == uuid.Nil {
		return nil, nil
	}
	var summaries []ProcessingRequestStatusSummary
	err := r.db.WithContext(ctx).Model(&model.ProcessingRequest{}).
		Select("status, COUNT(*) AS count").
		Where("organization_id = ?", organizationID).
		Where("asset_id = ?", assetID).
		Where("deleted_at IS NULL").
		Group("status").
		Scan(&summaries).Error
	if err != nil {
		return nil, err
	}
	return summaries, nil
}

func (r *processingRequestRepository) QueueSummary(ctx context.Context, filter ProcessingRequestQueueSummaryFilter) ([]ProcessingRequestQueueSummary, error) {
	if filter.OrganizationID == "" {
		return nil, nil
	}
	query := r.db.WithContext(ctx).Model(&model.ProcessingRequest{}).
		Select("target_level, status, executor_key, COUNT(*) AS count, CAST(MIN(queued_at) AS TEXT) AS oldest_queued_at, CAST(MIN(created_at) AS TEXT) AS oldest_created_at, CAST(MAX(created_at) AS TEXT) AS newest_created_at").
		Where("organization_id = ?", filter.OrganizationID).
		Where("deleted_at IS NULL")
	if filter.TargetLevel != "" {
		query = query.Where("target_level = ?", filter.TargetLevel)
	}
	if filter.Status != "" {
		query = query.Where("status = ?", filter.Status)
	}
	if filter.ExecutorKey != "" {
		query = query.Where("executor_key = ?", filter.ExecutorKey)
	}

	var rows []processingRequestQueueSummaryRow
	err := query.
		Group("target_level, status, executor_key").
		Order("target_level ASC, status ASC, executor_key ASC").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	summaries := make([]ProcessingRequestQueueSummary, 0, len(rows))
	for _, row := range rows {
		summaries = append(summaries, ProcessingRequestQueueSummary{
			TargetLevel:     row.TargetLevel,
			Status:          row.Status,
			ExecutorKey:     row.ExecutorKey,
			Count:           row.Count,
			OldestQueuedAt:  parseAggregateTime(row.OldestQueuedAtText),
			OldestCreatedAt: parseAggregateTime(row.OldestCreatedAtText),
			NewestCreatedAt: parseAggregateTime(row.NewestCreatedAtText),
		})
	}
	return summaries, nil
}

func parseAggregateTime(value *string) *time.Time {
	if value == nil {
		return nil
	}
	text := strings.TrimSpace(*value)
	if text == "" {
		return nil
	}
	layouts := []string{
		time.RFC3339Nano,
		"2006-01-02 15:04:05.999999999-07:00",
		"2006-01-02 15:04:05.999999999Z07:00",
		"2006-01-02 15:04:05.999999999",
		"2006-01-02 15:04:05-07:00",
		"2006-01-02 15:04:05Z07:00",
		"2006-01-02 15:04:05",
	}
	for _, layout := range layouts {
		parsed, err := time.Parse(layout, text)
		if err == nil {
			return &parsed
		}
	}
	return nil
}

func (r *processingRequestRepository) TransitionStatus(ctx context.Context, id uuid.UUID, patch ProcessingRequestStatusPatch) (*model.ProcessingRequest, error) {
	if id == uuid.Nil {
		return nil, nil
	}
	updates := map[string]any{
		"status":     patch.Status,
		"updated_at": time.Now(),
	}
	if patch.ExecutorKey != nil {
		updates["executor_key"] = *patch.ExecutorKey
	}
	if patch.ErrorCode != nil {
		updates["error_code"] = *patch.ErrorCode
	}
	if patch.ErrorMessage != nil {
		updates["error_message"] = *patch.ErrorMessage
	}
	if patch.AttemptCountDelta != 0 {
		updates["attempt_count"] = gorm.Expr("attempt_count + ?", patch.AttemptCountDelta)
	}
	if patch.QueuedAt != nil {
		updates["queued_at"] = *patch.QueuedAt
	}
	if patch.StartedAt != nil {
		updates["started_at"] = *patch.StartedAt
	}
	if patch.CompletedAt != nil {
		updates["completed_at"] = *patch.CompletedAt
	}
	if patch.FailedAt != nil {
		updates["failed_at"] = *patch.FailedAt
	}
	if patch.CanceledAt != nil {
		updates["cancelled_at"] = *patch.CanceledAt
	}
	if patch.ExecutionMetadata != nil {
		metadataJSON, err := json.Marshal(patch.ExecutionMetadata)
		if err != nil {
			return nil, err
		}
		updates["execution_metadata"] = datatypes.JSON(metadataJSON)
	}

	query := r.db.WithContext(ctx).Model(&model.ProcessingRequest{}).
		Where("id = ?", id).
		Where("deleted_at IS NULL")
	if patch.OrganizationID != "" {
		query = query.Where("organization_id = ?", patch.OrganizationID)
	}
	if len(patch.AllowedFrom) > 0 {
		query = query.Where("status IN ?", patch.AllowedFrom)
	}
	result := query.Updates(updates)
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 0 {
		item, err := r.GetByID(ctx, id)
		if err != nil || item == nil {
			return item, err
		}
		if patch.OrganizationID != "" && item.OrganizationID != patch.OrganizationID {
			return nil, nil
		}
		return item, nil
	}
	return r.GetByID(ctx, id)
}

func (r *processingRequestRepository) ClaimNextQueued(ctx context.Context, filter ProcessingRequestClaimFilter) (*model.ProcessingRequest, error) {
	if filter.OrganizationID == "" || filter.ExecutorKey == "" {
		return nil, nil
	}
	var claimed *model.ProcessingRequest
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var item model.ProcessingRequest
		query := tx.Where("organization_id = ?", filter.OrganizationID).
			Where("status = ?", model.ProcessingRequestStatusQueued).
			Where("deleted_at IS NULL").
			Order("created_at ASC")
		if len(filter.TargetLevels) > 0 {
			query = query.Where("target_level IN ?", filter.TargetLevels)
		}
		if tx.Dialector.Name() != "sqlite" {
			query = query.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"})
		}
		err := query.First(&item).Error
		if err != nil {
			if err == gorm.ErrRecordNotFound {
				return nil
			}
			return err
		}

		now := time.Now()
		updates := map[string]any{
			"status":        model.ProcessingRequestStatusRunning,
			"executor_key":  filter.ExecutorKey,
			"attempt_count": gorm.Expr("attempt_count + ?", 1),
			"started_at":    now,
			"updated_at":    now,
		}
		result := tx.Model(&model.ProcessingRequest{}).
			Where("id = ?", item.ID).
			Where("status = ?", model.ProcessingRequestStatusQueued).
			Where("deleted_at IS NULL").
			Updates(updates)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return nil
		}
		if err := tx.Where("id = ?", item.ID).First(&item).Error; err != nil {
			return err
		}
		claimed = &item
		return nil
	})
	if err != nil {
		return nil, err
	}
	return claimed, nil
}
