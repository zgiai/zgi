package integrations

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type ExecutionRepository interface {
	Create(ctx context.Context, record *ExecutionRecord) error
	Complete(ctx context.Context, id uuid.UUID, completion ExecutionCompletion) error
}

type ExecutionListFilter struct {
	OrganizationID uuid.UUID
	IntegrationID  string
	ActionID       string
	Status         string
	ConnectionID   *uuid.UUID
	Page           int
	PageSize       int
}

type ExecutionListPage struct {
	Items    []ExecutionRecord `json:"items"`
	Total    int64             `json:"total"`
	Page     int               `json:"page"`
	PageSize int               `json:"page_size"`
}

type ExecutionQueryRepository interface {
	List(ctx context.Context, filter ExecutionListFilter) (ExecutionListPage, error)
}

type GormExecutionRepository struct{ db *gorm.DB }

func NewGormExecutionRepository(db *gorm.DB) *GormExecutionRepository {
	return &GormExecutionRepository{db: db}
}

func (r *GormExecutionRepository) Create(ctx context.Context, record *ExecutionRecord) error {
	if r == nil || r.db == nil {
		return fmt.Errorf("integration execution repository is unavailable")
	}
	if err := r.db.WithContext(ctx).Create(record).Error; err != nil {
		return fmt.Errorf("create integration execution: %w", err)
	}
	return nil
}

func (r *GormExecutionRepository) Complete(ctx context.Context, id uuid.UUID, completion ExecutionCompletion) error {
	if r == nil || r.db == nil {
		return fmt.Errorf("integration execution repository is unavailable")
	}
	updates := map[string]interface{}{
		"status":        completion.Status,
		"duration_ms":   completion.DurationMS,
		"cost_usd":      completion.CostUSD,
		"result_count":  completion.ResultCount,
		"attempt_count": completion.AttemptCount,
		"updated_at":    gorm.Expr("CURRENT_TIMESTAMP"),
	}
	if completion.ProviderRequestID != "" {
		updates["provider_request_id"] = completion.ProviderRequestID
	}
	if completion.ErrorCode != "" {
		updates["error_code"] = completion.ErrorCode
	}
	result := r.db.WithContext(ctx).Model(&ExecutionRecord{}).Where("id = ?", id).Updates(updates)
	if result.Error != nil {
		return fmt.Errorf("complete integration execution: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return fmt.Errorf("complete integration execution: record not found")
	}
	return nil
}

func (r *GormExecutionRepository) List(ctx context.Context, filter ExecutionListFilter) (ExecutionListPage, error) {
	if r == nil || r.db == nil {
		return ExecutionListPage{}, fmt.Errorf("integration execution repository is unavailable")
	}
	if filter.OrganizationID == uuid.Nil {
		return ExecutionListPage{}, fmt.Errorf("organization id is required")
	}
	page := filter.Page
	if page < 1 {
		page = 1
	}
	pageSize := filter.PageSize
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	query := r.db.WithContext(ctx).Model(&ExecutionRecord{}).Where("organization_id = ?", filter.OrganizationID)
	if integrationID := strings.ToLower(strings.TrimSpace(filter.IntegrationID)); integrationID != "" {
		query = query.Where("integration_id = ?", integrationID)
	}
	if actionID := strings.ToLower(strings.TrimSpace(filter.ActionID)); actionID != "" {
		query = query.Where("action_id = ?", actionID)
	}
	if status := strings.ToLower(strings.TrimSpace(filter.Status)); status != "" {
		query = query.Where("status = ?", status)
	}
	if filter.ConnectionID != nil {
		query = query.Where("connection_id = ?", *filter.ConnectionID)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return ExecutionListPage{}, fmt.Errorf("count integration executions: %w", err)
	}
	items := make([]ExecutionRecord, 0, pageSize)
	if err := query.Order("created_at DESC, id DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&items).Error; err != nil {
		return ExecutionListPage{}, fmt.Errorf("list integration executions: %w", err)
	}
	return ExecutionListPage{Items: items, Total: total, Page: page, PageSize: pageSize}, nil
}
