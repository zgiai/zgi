package repository

import (
	"context"
	"database/sql/driver"
	"fmt"
	"strings"
	"time"

	"github.com/zgiai/zgi/api/internal/modules/llm/statistics/dto"
	"gorm.io/gorm"
)

type invocationLogFilters struct {
	OrganizationID   string
	StartTime        int64
	EndTime          int64
	InvocationSource *string
	AppType          *string
	ModelName        *string
}

type invocationLogSummaryRow struct {
	InvocationCount int64 `gorm:"column:invocation_count"`
	APICount        int64 `gorm:"column:api_count"`
	ProductCount    int64 `gorm:"column:product_count"`
	UnknownCount    int64 `gorm:"column:unknown_count"`
	TotalTokens     int64 `gorm:"column:total_tokens"`
	TotalPoints     int64 `gorm:"column:total_points"`
}

type invocationPageRow struct {
	RequestID string `gorm:"column:request_id"`
	LatestAt  dbTime `gorm:"column:latest_at"`
}

type dbTime time.Time

func (value *dbTime) Scan(src any) error {
	switch typed := src.(type) {
	case time.Time:
		*value = dbTime(typed)
		return nil
	case string:
		return value.parse(typed)
	case []byte:
		return value.parse(string(typed))
	default:
		return fmt.Errorf("unsupported database time type %T", src)
	}
}

func (value *dbTime) parse(raw string) error {
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02 15:04:05.999999999-07:00", "2006-01-02 15:04:05.999999999"} {
		parsed, err := time.Parse(layout, raw)
		if err == nil {
			*value = dbTime(parsed)
			return nil
		}
	}
	return fmt.Errorf("invalid database time %q", raw)
}

func (value dbTime) Value() (driver.Value, error) {
	return time.Time(value), nil
}

type invocationBillRow struct {
	RequestID        string    `gorm:"column:request_id"`
	AppID            *string   `gorm:"column:app_id"`
	AppType          *string   `gorm:"column:app_type"`
	InvocationSource string    `gorm:"column:invocation_source"`
	ModelName        string    `gorm:"column:model_name"`
	ProviderName     string    `gorm:"column:provider_name"`
	Status           string    `gorm:"column:status"`
	PromptTokens     int64     `gorm:"column:prompt_tokens"`
	CompletionTokens int64     `gorm:"column:completion_tokens"`
	TotalTokens      int64     `gorm:"column:total_tokens"`
	TotalPoints      int64     `gorm:"column:total_points"`
	ErrorCode        *string   `gorm:"column:error_code"`
	RequestCreatedAt time.Time `gorm:"column:request_created_at"`
	SettledAt        time.Time `gorm:"column:settled_at"`
}

func (r *statisticsRepositoryImpl) GetInvocationLog(ctx context.Context, organizationID string, req *dto.InvocationLogRequest) (*dto.InvocationLogResponse, error) {
	filters := invocationLogFilters{
		OrganizationID:   organizationID,
		StartTime:        req.StartTime,
		EndTime:          req.EndTime,
		InvocationSource: req.InvocationSource,
		AppType:          req.AppType,
		ModelName:        req.ModelName,
	}

	summary, err := r.queryInvocationLogSummary(ctx, filters)
	if err != nil {
		return nil, err
	}
	page, hasMore, err := r.queryInvocationPage(ctx, filters, req)
	if err != nil {
		return nil, err
	}
	if len(page) == 0 {
		return &dto.InvocationLogResponse{Summary: summary, Items: []dto.InvocationLogItem{}}, nil
	}

	requestIDs := make([]string, 0, len(page))
	for _, row := range page {
		requestIDs = append(requestIDs, row.RequestID)
	}
	bills, err := r.queryInvocationBills(ctx, filters, requestIDs)
	if err != nil {
		return nil, err
	}
	items := aggregateInvocationBills(page, bills)

	response := &dto.InvocationLogResponse{Summary: summary, Items: items}
	if hasMore {
		last := page[len(page)-1]
		response.NextCursor = &dto.InvocationLogCursor{Time: time.Time(last.LatestAt).UTC().Format(time.RFC3339Nano), ID: last.RequestID}
	}
	return response, nil
}

func (r *statisticsRepositoryImpl) queryInvocationLogSummary(ctx context.Context, filters invocationLogFilters) (dto.InvocationLogSummary, error) {
	var row invocationLogSummaryRow
	query := r.db.WithContext(ctx).Table(usageBillTable + " b").Select(`
		COUNT(DISTINCT b.request_id) AS invocation_count,
		COUNT(DISTINCT CASE WHEN b.invocation_source = 'api' THEN b.request_id END) AS api_count,
		COUNT(DISTINCT CASE WHEN b.invocation_source = 'product' THEN b.request_id END) AS product_count,
		COUNT(DISTINCT CASE WHEN b.invocation_source = 'unknown' THEN b.request_id END) AS unknown_count,
		COALESCE(SUM(b.total_tokens), 0) AS total_tokens,
		COALESCE(SUM(b.total_points), 0) AS total_points
	`)
	query = applyInvocationLogFilters(query, "b", filters)
	if err := query.Scan(&row).Error; err != nil {
		return dto.InvocationLogSummary{}, err
	}
	return dto.InvocationLogSummary{
		InvocationCount: row.InvocationCount,
		APICount:        row.APICount,
		ProductCount:    row.ProductCount,
		UnknownCount:    row.UnknownCount,
		TotalTokens:     row.TotalTokens,
		TotalPoints:     row.TotalPoints,
	}, nil
}

func (r *statisticsRepositoryImpl) queryInvocationPage(ctx context.Context, filters invocationLogFilters, req *dto.InvocationLogRequest) ([]invocationPageRow, bool, error) {
	limit := req.Limit
	query := r.db.WithContext(ctx).Table(usageBillTable + " b").
		Select("b.request_id, MAX(b.request_created_at) AS latest_at")
	query = applyInvocationLogFilters(query, "b", filters).Group("b.request_id")
	if req.CursorTime != nil && req.CursorID != nil && strings.TrimSpace(*req.CursorID) != "" {
		cursorAt, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(*req.CursorTime))
		if err != nil {
			return nil, false, err
		}
		query = query.Having("MAX(b.request_created_at) < ? OR (MAX(b.request_created_at) = ? AND b.request_id < ?)", cursorAt, cursorAt, strings.TrimSpace(*req.CursorID))
	}

	var rows []invocationPageRow
	if err := query.Order("latest_at DESC").Order("b.request_id DESC").Limit(limit + 1).Scan(&rows).Error; err != nil {
		return nil, false, err
	}
	hasMore := len(rows) > limit
	if hasMore {
		rows = rows[:limit]
	}
	return rows, hasMore, nil
}

func (r *statisticsRepositoryImpl) queryInvocationBills(ctx context.Context, filters invocationLogFilters, requestIDs []string) ([]invocationBillRow, error) {
	var rows []invocationBillRow
	query := r.db.WithContext(ctx).Table(usageBillTable+" b").
		Select(`b.request_id, b.app_id, b.app_type, b.invocation_source, b.model_name, b.provider_name,
			b.status, b.prompt_tokens, b.completion_tokens, b.total_tokens, b.total_points,
			b.error_code, b.request_created_at, b.settled_at`).
		Where("b.request_id IN ?", requestIDs)
	query = applyInvocationLogFilters(query, "b", filters)
	err := query.Order("b.request_created_at ASC").Scan(&rows).Error
	return rows, err
}

func applyInvocationLogFilters(query *gorm.DB, alias string, filters invocationLogFilters) *gorm.DB {
	// API timestamps have second precision. Treat end_time as the inclusive
	// final second by comparing against the start of the following second.
	endExclusive := time.Unix(filters.EndTime, 0).UTC().Add(time.Second)
	query = query.Where(column(alias, "organization_id")+" = ?", filters.OrganizationID).
		Where(column(alias, "request_created_at")+" >= ?", time.Unix(filters.StartTime, 0).UTC()).
		Where(column(alias, "request_created_at")+" < ?", endExclusive)
	if hasText(filters.InvocationSource) {
		query = query.Where(column(alias, "invocation_source")+" = ?", strings.TrimSpace(*filters.InvocationSource))
	}
	if hasText(filters.AppType) {
		query = query.Where(usageBillAppTypeBucketExpr(alias)+" = ?", strings.TrimSpace(*filters.AppType))
	}
	if hasText(filters.ModelName) {
		query = query.Where(column(alias, "model_name")+" = ?", strings.TrimSpace(*filters.ModelName))
	}
	return query
}

type invocationAccumulator struct {
	item       dto.InvocationLogItem
	startedAt  time.Time
	settledAt  time.Time
	hasSuccess bool
	hasPartial bool
}

func aggregateInvocationBills(page []invocationPageRow, bills []invocationBillRow) []dto.InvocationLogItem {
	accumulators := make(map[string]*invocationAccumulator, len(page))
	for _, bill := range bills {
		acc := accumulators[bill.RequestID]
		if acc == nil {
			acc = &invocationAccumulator{item: dto.InvocationLogItem{InvocationID: bill.RequestID}}
			accumulators[bill.RequestID] = acc
		}
		acc.item.AttemptCount++
		acc.item.PromptTokens += bill.PromptTokens
		acc.item.CompletionTokens += bill.CompletionTokens
		acc.item.TotalTokens += bill.TotalTokens
		acc.item.TotalPoints += bill.TotalPoints
		if acc.startedAt.IsZero() || bill.RequestCreatedAt.Before(acc.startedAt) {
			acc.startedAt = bill.RequestCreatedAt
		}
		if bill.SettledAt.After(acc.settledAt) {
			acc.settledAt = bill.SettledAt
		}
		if bill.Status == "success" || !acc.hasSuccess {
			acc.item.ModelName = bill.ModelName
			acc.item.ProviderName = bill.ProviderName
			acc.item.InvocationSource = normalizedInvocationSource(bill.InvocationSource)
			acc.item.AppType = normalizedAppType(bill.AppType)
			acc.item.AppID = bill.AppID
			acc.item.ErrorCode = bill.ErrorCode
		}
		acc.hasSuccess = acc.hasSuccess || bill.Status == "success"
		acc.hasPartial = acc.hasPartial || bill.Status == "partial"
	}

	items := make([]dto.InvocationLogItem, 0, len(page))
	for _, pageRow := range page {
		acc := accumulators[pageRow.RequestID]
		if acc == nil {
			continue
		}
		switch {
		case acc.hasSuccess:
			acc.item.Status = "success"
			acc.item.ErrorCode = nil
		case acc.hasPartial:
			acc.item.Status = "partial"
		default:
			acc.item.Status = "failed"
		}
		acc.item.StartedAt = acc.startedAt.UnixMilli()
		acc.item.SettledAt = acc.settledAt.UnixMilli()
		acc.item.DurationMS = max(acc.settledAt.Sub(acc.startedAt).Milliseconds(), 0)
		items = append(items, acc.item)
	}
	return items
}

func normalizedInvocationSource(source string) string {
	switch strings.TrimSpace(source) {
	case "api", "product":
		return strings.TrimSpace(source)
	default:
		return "unknown"
	}
}

func normalizedAppType(appType *string) string {
	if appType == nil || strings.TrimSpace(*appType) == "" {
		return "unknown"
	}
	return strings.TrimSpace(*appType)
}
