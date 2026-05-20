package repository

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/llm/statistics/dto"
	"gorm.io/gorm"
)

const usageBillTable = "llm_usage_bills"

type modelUsageFilters struct {
	OrganizationID    string
	StartTime         int64
	EndTime           int64
	AppType           *string
	AppID             *uuid.UUID
	ModelName         *string
	BillingLane       *string
	UseSystemProvider *bool
}

type modelUsageSummaryRow struct {
	AttemptCount     int64 `gorm:"column:attempt_count"`
	SuccessCount     int64 `gorm:"column:success_count"`
	FailedCount      int64 `gorm:"column:failed_count"`
	PartialCount     int64 `gorm:"column:partial_count"`
	PromptTokens     int64 `gorm:"column:prompt_tokens"`
	CompletionTokens int64 `gorm:"column:completion_tokens"`
	TotalTokens      int64 `gorm:"column:total_tokens"`
	OfficialPoints   int64 `gorm:"column:official_points"`
	PrivatePoints    int64 `gorm:"column:private_points"`
	TotalPoints      int64 `gorm:"column:total_points"`
}

type modelUsageModelRow struct {
	ModelID          uuid.UUID `gorm:"column:model_id"`
	ModelName        string    `gorm:"column:model_name"`
	ProviderID       uuid.UUID `gorm:"column:provider_id"`
	ProviderName     string    `gorm:"column:provider_name"`
	AttemptCount     int64     `gorm:"column:attempt_count"`
	SuccessCount     int64     `gorm:"column:success_count"`
	FailedCount      int64     `gorm:"column:failed_count"`
	PartialCount     int64     `gorm:"column:partial_count"`
	PromptTokens     int64     `gorm:"column:prompt_tokens"`
	CompletionTokens int64     `gorm:"column:completion_tokens"`
	TotalTokens      int64     `gorm:"column:total_tokens"`
	OfficialPoints   int64     `gorm:"column:official_points"`
	PrivatePoints    int64     `gorm:"column:private_points"`
	TotalPoints      int64     `gorm:"column:total_points"`
}

type modelUsageAppTypeRow struct {
	AppType          string `gorm:"column:app_type"`
	AttemptCount     int64  `gorm:"column:attempt_count"`
	SuccessCount     int64  `gorm:"column:success_count"`
	FailedCount      int64  `gorm:"column:failed_count"`
	PartialCount     int64  `gorm:"column:partial_count"`
	PromptTokens     int64  `gorm:"column:prompt_tokens"`
	CompletionTokens int64  `gorm:"column:completion_tokens"`
	TotalTokens      int64  `gorm:"column:total_tokens"`
	OfficialPoints   int64  `gorm:"column:official_points"`
	PrivatePoints    int64  `gorm:"column:private_points"`
	TotalPoints      int64  `gorm:"column:total_points"`
}

type modelUsageDailyRow struct {
	Date             string `gorm:"column:date"`
	AttemptCount     int64  `gorm:"column:attempt_count"`
	SuccessCount     int64  `gorm:"column:success_count"`
	FailedCount      int64  `gorm:"column:failed_count"`
	PartialCount     int64  `gorm:"column:partial_count"`
	PromptTokens     int64  `gorm:"column:prompt_tokens"`
	CompletionTokens int64  `gorm:"column:completion_tokens"`
	TotalTokens      int64  `gorm:"column:total_tokens"`
	OfficialPoints   int64  `gorm:"column:official_points"`
	PrivatePoints    int64  `gorm:"column:private_points"`
	TotalPoints      int64  `gorm:"column:total_points"`
}

func (r *statisticsRepositoryImpl) GetModelUsage(ctx context.Context, organizationID string, req *dto.ModelUsageRequest) (*dto.ModelUsageResponse, error) {
	filters := modelUsageFilters{
		OrganizationID:    organizationID,
		StartTime:         req.StartTime,
		EndTime:           req.EndTime,
		AppType:           req.AppType,
		AppID:             req.AppID,
		ModelName:         req.ModelName,
		BillingLane:       req.BillingLane,
		UseSystemProvider: req.UseSystemProvider,
	}

	summary, err := r.queryModelUsageSummary(ctx, filters)
	if err != nil {
		return nil, err
	}

	byModel, err := r.queryModelUsageByModel(ctx, filters)
	if err != nil {
		return nil, err
	}

	byAppType, err := r.queryModelUsageByAppType(ctx, filters)
	if err != nil {
		return nil, err
	}

	dailyTrend, err := r.queryModelUsageDailyTrend(ctx, filters)
	if err != nil {
		return nil, err
	}

	return &dto.ModelUsageResponse{
		Period: dto.ModelUsagePeriod{
			StartTime: req.StartTime,
			EndTime:   req.EndTime,
		},
		Summary:    summary,
		ByModel:    buildModelUsageByModelItems(byModel, summary.TotalPoints),
		ByAppType:  buildModelUsageByAppTypeItems(byAppType, summary.TotalPoints),
		DailyTrend: buildModelUsageDailyItems(dailyTrend),
	}, nil
}

func (r *statisticsRepositoryImpl) queryModelUsageSummary(ctx context.Context, filters modelUsageFilters) (dto.ModelUsageSummary, error) {
	var row modelUsageSummaryRow
	query := r.db.WithContext(ctx).
		Table(usageBillTable + " b").
		Select(`
			COUNT(*) as attempt_count,
			COALESCE(SUM(CASE WHEN b.status = 'success' THEN 1 ELSE 0 END), 0) as success_count,
			COALESCE(SUM(CASE WHEN b.status = 'failed' THEN 1 ELSE 0 END), 0) as failed_count,
			COALESCE(SUM(CASE WHEN b.status = 'partial' THEN 1 ELSE 0 END), 0) as partial_count,
			COALESCE(SUM(b.prompt_tokens), 0) as prompt_tokens,
			COALESCE(SUM(b.completion_tokens), 0) as completion_tokens,
			COALESCE(SUM(b.total_tokens), 0) as total_tokens,
			COALESCE(SUM(b.official_points), 0) as official_points,
			COALESCE(SUM(b.private_points), 0) as private_points,
			COALESCE(SUM(b.total_points), 0) as total_points
		`)
	query = applyUsageBillFilters(query, "b", filters)
	if err := query.Scan(&row).Error; err != nil {
		return dto.ModelUsageSummary{}, err
	}

	return dto.ModelUsageSummary{
		AttemptCount:     row.AttemptCount,
		SuccessCount:     row.SuccessCount,
		FailedCount:      row.FailedCount,
		PartialCount:     row.PartialCount,
		PromptTokens:     row.PromptTokens,
		CompletionTokens: row.CompletionTokens,
		TotalTokens:      row.TotalTokens,
		OfficialPoints:   row.OfficialPoints,
		PrivatePoints:    row.PrivatePoints,
		TotalPoints:      row.TotalPoints,
	}, nil
}

func (r *statisticsRepositoryImpl) queryModelUsageByModel(ctx context.Context, filters modelUsageFilters) ([]modelUsageModelRow, error) {
	var rows []modelUsageModelRow
	query := r.db.WithContext(ctx).
		Table(usageBillTable + " b").
		Select(`
			b.model_id,
			b.model_name,
			b.provider_id,
			b.provider_name,
			COUNT(*) as attempt_count,
			COALESCE(SUM(CASE WHEN b.status = 'success' THEN 1 ELSE 0 END), 0) as success_count,
			COALESCE(SUM(CASE WHEN b.status = 'failed' THEN 1 ELSE 0 END), 0) as failed_count,
			COALESCE(SUM(CASE WHEN b.status = 'partial' THEN 1 ELSE 0 END), 0) as partial_count,
			COALESCE(SUM(b.prompt_tokens), 0) as prompt_tokens,
			COALESCE(SUM(b.completion_tokens), 0) as completion_tokens,
			COALESCE(SUM(b.total_tokens), 0) as total_tokens,
			COALESCE(SUM(b.official_points), 0) as official_points,
			COALESCE(SUM(b.private_points), 0) as private_points,
			COALESCE(SUM(b.total_points), 0) as total_points
		`)
	query = applyUsageBillFilters(query, "b", filters).
		Group("b.model_id, b.model_name, b.provider_id, b.provider_name").
		Order("total_points DESC").
		Order("total_tokens DESC")
	if err := query.Scan(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *statisticsRepositoryImpl) queryModelUsageByAppType(ctx context.Context, filters modelUsageFilters) ([]modelUsageAppTypeRow, error) {
	var rows []modelUsageAppTypeRow
	appTypeExpr := usageBillAppTypeBucketExpr("b")
	query := r.db.WithContext(ctx).
		Table(usageBillTable + " b").
		Select(`
			` + appTypeExpr + ` as app_type,
			COUNT(*) as attempt_count,
			COALESCE(SUM(CASE WHEN b.status = 'success' THEN 1 ELSE 0 END), 0) as success_count,
			COALESCE(SUM(CASE WHEN b.status = 'failed' THEN 1 ELSE 0 END), 0) as failed_count,
			COALESCE(SUM(CASE WHEN b.status = 'partial' THEN 1 ELSE 0 END), 0) as partial_count,
			COALESCE(SUM(b.prompt_tokens), 0) as prompt_tokens,
			COALESCE(SUM(b.completion_tokens), 0) as completion_tokens,
			COALESCE(SUM(b.total_tokens), 0) as total_tokens,
			COALESCE(SUM(b.official_points), 0) as official_points,
			COALESCE(SUM(b.private_points), 0) as private_points,
			COALESCE(SUM(b.total_points), 0) as total_points
		`)
	query = applyUsageBillFilters(query, "b", filters).
		Group(appTypeExpr).
		Order("total_points DESC").
		Order("app_type ASC")
	if err := query.Scan(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *statisticsRepositoryImpl) queryModelUsageDailyTrend(ctx context.Context, filters modelUsageFilters) ([]modelUsageDailyRow, error) {
	var rows []modelUsageDailyRow
	query := r.db.WithContext(ctx).
		Table(usageBillTable + " b").
		Select(`
			CAST(DATE(b.request_created_at) AS TEXT) as date,
			COUNT(*) as attempt_count,
			COALESCE(SUM(CASE WHEN b.status = 'success' THEN 1 ELSE 0 END), 0) as success_count,
			COALESCE(SUM(CASE WHEN b.status = 'failed' THEN 1 ELSE 0 END), 0) as failed_count,
			COALESCE(SUM(CASE WHEN b.status = 'partial' THEN 1 ELSE 0 END), 0) as partial_count,
			COALESCE(SUM(b.prompt_tokens), 0) as prompt_tokens,
			COALESCE(SUM(b.completion_tokens), 0) as completion_tokens,
			COALESCE(SUM(b.total_tokens), 0) as total_tokens,
			COALESCE(SUM(b.official_points), 0) as official_points,
			COALESCE(SUM(b.private_points), 0) as private_points,
			COALESCE(SUM(b.total_points), 0) as total_points
		`)
	query = applyUsageBillFilters(query, "b", filters).
		Group("CAST(DATE(b.request_created_at) AS TEXT)").
		Order("date ASC")
	if err := query.Scan(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func applyUsageBillFilters(query *gorm.DB, alias string, filters modelUsageFilters) *gorm.DB {
	query = query.Where(column(alias, "organization_id")+" = ?", filters.OrganizationID)
	query = query.Where(column(alias, "request_created_at")+" >= ?", time.Unix(filters.StartTime, 0).UTC())
	query = query.Where(column(alias, "request_created_at")+" <= ?", time.Unix(filters.EndTime, 0).UTC())

	if hasText(filters.AppType) {
		query = query.Where(usageBillAppTypeBucketExpr(alias)+" = ?", strings.TrimSpace(*filters.AppType))
	}
	if filters.AppID != nil {
		query = query.Where(column(alias, "app_id")+" = ?", *filters.AppID)
	}
	if hasText(filters.ModelName) {
		query = query.Where(column(alias, "model_name")+" = ?", strings.TrimSpace(*filters.ModelName))
	}
	if hasText(filters.BillingLane) {
		query = query.Where(column(alias, "billing_lane")+" = ?", strings.TrimSpace(*filters.BillingLane))
	}
	if filters.UseSystemProvider != nil {
		query = query.Where(column(alias, "use_system_provider")+" = ?", *filters.UseSystemProvider)
	}

	return query
}

func usageBillAppTypeBucketExpr(alias string) string {
	return "COALESCE(NULLIF(" + column(alias, "app_type") + ", ''), 'unknown')"
}

func buildModelUsageByModelItems(rows []modelUsageModelRow, totalPoints int64) []dto.ModelUsageByModelItem {
	items := make([]dto.ModelUsageByModelItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, dto.ModelUsageByModelItem{
			ModelID:          row.ModelID,
			ModelName:        row.ModelName,
			ProviderID:       row.ProviderID,
			ProviderName:     row.ProviderName,
			AttemptCount:     row.AttemptCount,
			SuccessCount:     row.SuccessCount,
			FailedCount:      row.FailedCount,
			PartialCount:     row.PartialCount,
			PromptTokens:     row.PromptTokens,
			CompletionTokens: row.CompletionTokens,
			TotalTokens:      row.TotalTokens,
			OfficialPoints:   row.OfficialPoints,
			PrivatePoints:    row.PrivatePoints,
			TotalPoints:      row.TotalPoints,
			PointsShare:      usageBillShare(row.TotalPoints, totalPoints),
		})
	}
	return items
}

func buildModelUsageByAppTypeItems(rows []modelUsageAppTypeRow, totalPoints int64) []dto.ModelUsageByAppTypeItem {
	items := make([]dto.ModelUsageByAppTypeItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, dto.ModelUsageByAppTypeItem{
			AppType:          row.AppType,
			AttemptCount:     row.AttemptCount,
			SuccessCount:     row.SuccessCount,
			FailedCount:      row.FailedCount,
			PartialCount:     row.PartialCount,
			PromptTokens:     row.PromptTokens,
			CompletionTokens: row.CompletionTokens,
			TotalTokens:      row.TotalTokens,
			OfficialPoints:   row.OfficialPoints,
			PrivatePoints:    row.PrivatePoints,
			TotalPoints:      row.TotalPoints,
			PointsShare:      usageBillShare(row.TotalPoints, totalPoints),
		})
	}
	return items
}

func buildModelUsageDailyItems(rows []modelUsageDailyRow) []dto.ModelUsageDailyItem {
	items := make([]dto.ModelUsageDailyItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, dto.ModelUsageDailyItem{
			Date:             row.Date,
			AttemptCount:     row.AttemptCount,
			SuccessCount:     row.SuccessCount,
			FailedCount:      row.FailedCount,
			PartialCount:     row.PartialCount,
			PromptTokens:     row.PromptTokens,
			CompletionTokens: row.CompletionTokens,
			TotalTokens:      row.TotalTokens,
			OfficialPoints:   row.OfficialPoints,
			PrivatePoints:    row.PrivatePoints,
			TotalPoints:      row.TotalPoints,
		})
	}
	return items
}

func usageBillShare(total, overall int64) float64 {
	if overall <= 0 {
		return 0
	}
	return float64(total) / float64(overall)
}
