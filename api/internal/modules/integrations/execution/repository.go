package execution

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	integrations "github.com/zgiai/zgi/api/internal/modules/integrations/core"
	"gorm.io/gorm"
)

// GormExecutionRepository persists integration execution audit records.
type GormExecutionRepository struct{ db *gorm.DB }

var (
	_ integrations.ExecutionRepository      = (*GormExecutionRepository)(nil)
	_ integrations.ExecutionQueryRepository = (*GormExecutionRepository)(nil)
)

func NewGormExecutionRepository(db *gorm.DB) *GormExecutionRepository {
	return &GormExecutionRepository{db: db}
}

func (r *GormExecutionRepository) Create(ctx context.Context, record *integrations.ExecutionRecord) error {
	if r == nil || r.db == nil {
		return fmt.Errorf("integration execution repository is unavailable")
	}
	if err := r.db.WithContext(ctx).Create(record).Error; err != nil {
		return fmt.Errorf("create integration execution: %w", err)
	}
	return nil
}

func (r *GormExecutionRepository) Complete(ctx context.Context, id uuid.UUID, completion integrations.ExecutionCompletion) error {
	if r == nil || r.db == nil {
		return fmt.Errorf("integration execution repository is unavailable")
	}
	diagnostics := integrations.NormalizeProviderDiagnostics(integrations.ProviderDiagnostics{
		ErrorCode:    completion.ProviderErrorCode,
		RequestID:    completion.ProviderRequestID,
		HTTPStatus:   intValue(completion.ProviderHTTPStatus),
		RetryAfterAt: completion.RetryAfterAt,
	})
	updates := map[string]interface{}{
		"status":        completion.Status,
		"duration_ms":   completion.DurationMS,
		"cost_usd":      completion.CostUSD,
		"result_count":  completion.ResultCount,
		"attempt_count": completion.AttemptCount,
		"updated_at":    gorm.Expr("CURRENT_TIMESTAMP"),
	}
	if diagnostics.RequestID != "" {
		updates["provider_request_id"] = diagnostics.RequestID
	}
	if diagnostics.ErrorCode != "" {
		updates["provider_error_code"] = diagnostics.ErrorCode
	}
	if diagnostics.HTTPStatus != 0 {
		updates["provider_http_status"] = diagnostics.HTTPStatus
	}
	if diagnostics.RetryAfterAt != nil {
		updates["retry_after_at"] = diagnostics.RetryAfterAt
	}
	if completion.ErrorCode != "" {
		updates["error_code"] = completion.ErrorCode
	}
	result := r.db.WithContext(ctx).Model(&integrations.ExecutionRecord{}).Where("id = ?", id).Updates(updates)
	if result.Error != nil {
		return fmt.Errorf("complete integration execution: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return fmt.Errorf("complete integration execution: record not found")
	}
	return nil
}

func (r *GormExecutionRepository) List(ctx context.Context, filter integrations.ExecutionListFilter) (integrations.ExecutionListPage, error) {
	if r == nil || r.db == nil {
		return integrations.ExecutionListPage{}, fmt.Errorf("integration execution repository is unavailable")
	}
	if filter.OrganizationID == uuid.Nil {
		return integrations.ExecutionListPage{}, fmt.Errorf("organization id is required")
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
	query := r.db.WithContext(ctx).Model(&integrations.ExecutionRecord{}).Where("organization_id = ?", filter.OrganizationID)
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
		return integrations.ExecutionListPage{}, fmt.Errorf("count integration executions: %w", err)
	}
	items := make([]integrations.ExecutionRecord, 0, pageSize)
	if err := query.Order("created_at DESC, id DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&items).Error; err != nil {
		return integrations.ExecutionListPage{}, fmt.Errorf("list integration executions: %w", err)
	}
	return integrations.ExecutionListPage{Items: items, Total: total, Page: page, PageSize: pageSize}, nil
}

func intValue(value *int) int {
	if value == nil {
		return 0
	}
	return *value
}
