package repository

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/shopspring/decimal"
	"github.com/zgiai/zgi/api/internal/modules/llm/statistics/dto"
	"github.com/zgiai/zgi/api/pkg/logger"
	"go.uber.org/zap"
	"gorm.io/datatypes"
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
	RequestID        string         `gorm:"column:request_id"`
	AppID            *string        `gorm:"column:app_id"`
	AppType          *string        `gorm:"column:app_type"`
	InvocationSource string         `gorm:"column:invocation_source"`
	ModelName        string         `gorm:"column:model_name"`
	ProviderName     string         `gorm:"column:provider_name"`
	ChannelName      string         `gorm:"column:channel_name"`
	Status           string         `gorm:"column:status"`
	PromptTokens     int64          `gorm:"column:prompt_tokens"`
	CacheReadTokens  int64          `gorm:"column:cache_read_tokens"`
	CacheWriteTokens int64          `gorm:"column:cache_write_tokens"`
	CompletionTokens int64          `gorm:"column:completion_tokens"`
	TotalTokens      int64          `gorm:"column:total_tokens"`
	TotalPoints      int64          `gorm:"column:total_points"`
	BillingLane      string         `gorm:"column:billing_lane"`
	PricingSource    string         `gorm:"column:pricing_source"`
	UsageSource      string         `gorm:"column:usage_source"`
	PricingSnapshot  datatypes.JSON `gorm:"column:pricing_snapshot"`
	ErrorCode        *string        `gorm:"column:error_code"`
	RequestCreatedAt time.Time      `gorm:"column:request_created_at"`
	SettledAt        time.Time      `gorm:"column:settled_at"`
}

type invocationContentAvailabilityRow struct {
	RequestID string    `gorm:"column:request_id"`
	ExpiresAt time.Time `gorm:"column:expires_at"`
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

	var summary dto.InvocationLogSummary
	if req.IncludeSummary == nil || *req.IncludeSummary {
		var err error
		summary, err = r.queryInvocationLogSummary(ctx, filters)
		if err != nil {
			return nil, err
		}
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
	contentAvailability, err := r.queryInvocationContentAvailability(ctx, organizationID, requestIDs)
	if err != nil {
		// Content snapshots are optional and must never make the lightweight
		// billing log unavailable during a rolling deploy or storage incident.
		logger.WarnContext(ctx, "failed to load llm invocation content availability",
			zap.Error(err),
			zap.String("organization_id", organizationID),
			zap.Int("request_count", len(requestIDs)),
		)
		contentAvailability = nil
	}
	for index := range items {
		expiresAt, available := contentAvailability[items[index].InvocationID]
		if !available {
			continue
		}
		expiresAtMillis := expiresAt.UnixMilli()
		items[index].ContentAvailable = true
		items[index].ContentExpiresAt = &expiresAtMillis
	}

	response := &dto.InvocationLogResponse{Summary: summary, Items: items}
	if hasMore {
		last := page[len(page)-1]
		response.NextCursor = &dto.InvocationLogCursor{Time: time.Time(last.LatestAt).UTC().Format(time.RFC3339Nano), ID: last.RequestID}
	}
	return response, nil
}

// queryInvocationContentAvailability intentionally runs after the bounded page
// query. It never joins content payloads into the billing query and only checks
// the at-most 100 request IDs returned for the current page.
func (r *statisticsRepositoryImpl) queryInvocationContentAvailability(ctx context.Context, organizationID string, requestIDs []string) (map[string]time.Time, error) {
	rows := make([]invocationContentAvailabilityRow, 0, len(requestIDs))
	if err := r.db.WithContext(ctx).Table("llm_invocation_contents").
		Select("request_id", "expires_at").
		Where("organization_id = ? AND request_id IN ? AND expires_at > ?", organizationID, requestIDs, time.Now().UTC()).
		Find(&rows).Error; err != nil {
		return nil, err
	}
	availability := make(map[string]time.Time, len(rows))
	for _, row := range rows {
		availability[row.RequestID] = row.ExpiresAt
	}
	return availability, nil
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
	pricingSnapshotColumn := "'{}' AS pricing_snapshot"
	if r.db.Migrator().HasColumn(usageBillTable, "pricing_snapshot") {
		pricingSnapshotColumn = "b.pricing_snapshot"
	}
	billingLaneColumn := "'' AS billing_lane"
	if r.db.Migrator().HasColumn(usageBillTable, "billing_lane") {
		billingLaneColumn = "b.billing_lane"
	}
	pricingSourceColumn := "'' AS pricing_source"
	if r.db.Migrator().HasColumn(usageBillTable, "pricing_source") {
		pricingSourceColumn = "b.pricing_source"
	}
	usageSourceColumn := "'' AS usage_source"
	if r.db.Migrator().HasColumn(usageBillTable, "usage_source") {
		usageSourceColumn = "b.usage_source"
	}
	channelNameColumn := "'' AS channel_name"
	joinChannel := r.db.Migrator().HasColumn(usageBillTable, "channel_id") &&
		r.db.Migrator().HasTable("llm_routes") &&
		r.db.Migrator().HasColumn("llm_routes", "name")
	if joinChannel {
		channelNameColumn = "COALESCE(r.name, '') AS channel_name"
	}
	query := r.db.WithContext(ctx).Table(usageBillTable+" b").
		Select(fmt.Sprintf(`b.request_id, b.app_id, b.app_type, b.invocation_source, b.model_name, b.provider_name,
			b.status, b.prompt_tokens, b.cache_read_tokens, b.cache_write_tokens, b.completion_tokens, b.total_tokens, b.total_points,
			b.error_code, b.request_created_at, b.settled_at, %s, %s, %s, %s, %s`,
			pricingSnapshotColumn, channelNameColumn, billingLaneColumn, pricingSourceColumn, usageSourceColumn)).
		Where("b.request_id IN ?", requestIDs)
	if joinChannel {
		query = query.Joins("LEFT JOIN llm_routes r ON r.id = b.channel_id")
	}
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
	item         dto.InvocationLogItem
	totalCostUSD decimal.Decimal
	totalCostCNY decimal.Decimal
	hasExactCost bool
	hasCostCNY   bool
	missingCNY   bool
	startedAt    time.Time
	settledAt    time.Time
	hasSuccess   bool
	hasPartial   bool
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
		acc.item.CacheReadTokens += bill.CacheReadTokens
		acc.item.CacheWriteTokens += bill.CacheWriteTokens
		acc.item.CompletionTokens += bill.CompletionTokens
		acc.item.TotalTokens += bill.TotalTokens
		acc.item.TotalPoints += bill.TotalPoints
		if costUSD, ok := pricingSnapshotCostUSD(bill.PricingSnapshot); ok {
			acc.totalCostUSD = acc.totalCostUSD.Add(costUSD)
			acc.hasExactCost = true
		}
		if costCNY, ok := pricingSnapshotCostCNY(bill.PricingSnapshot); ok {
			acc.totalCostCNY = acc.totalCostCNY.Add(costCNY)
			acc.hasCostCNY = true
		} else if bill.TotalPoints > 0 {
			acc.missingCNY = true
		}
		if details := invocationPricingDetails(bill); details != nil && (acc.item.PricingDetails == nil || bill.Status == "success") {
			acc.item.PricingDetails = details
		}
		if acc.startedAt.IsZero() || bill.RequestCreatedAt.Before(acc.startedAt) {
			acc.startedAt = bill.RequestCreatedAt
		}
		if bill.SettledAt.After(acc.settledAt) {
			acc.settledAt = bill.SettledAt
		}
		if bill.Status == "success" || !acc.hasSuccess {
			acc.item.ModelName = bill.ModelName
			acc.item.ProviderName = bill.ProviderName
			acc.item.ChannelName = bill.ChannelName
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
		if acc.hasExactCost {
			value := acc.totalCostUSD.String()
			acc.item.TotalCostUSD = &value
		}
		if acc.hasCostCNY && !acc.missingCNY {
			value := acc.totalCostCNY.String()
			acc.item.TotalCostCNY = &value
		}
		items = append(items, acc.item)
	}
	return items
}

func invocationPricingDetails(bill invocationBillRow) *dto.InvocationPricingDetails {
	details := &dto.InvocationPricingDetails{
		BillingLane:   strings.TrimSpace(bill.BillingLane),
		PricingSource: strings.TrimSpace(bill.PricingSource),
		UsageSource:   strings.TrimSpace(bill.UsageSource),
	}
	values := pricingSnapshotValues(bill.PricingSnapshot)
	details.InputPriceUSDPer1MTokens = snapshotDecimalString(values["input_price_usd_per_1m_tokens"])
	details.CacheReadPriceUSDPer1MTokens = snapshotDecimalString(values["cache_read_price_usd_per_1m_tokens"])
	details.CacheWritePriceUSDPer1MTokens = snapshotDecimalString(values["cache_write_price_usd_per_1m_tokens"])
	details.OutputPriceUSDPer1MTokens = snapshotDecimalString(values["output_price_usd_per_1m_tokens"])
	details.InputCostUSD = snapshotDecimalString(values["input_cost_usd"])
	details.CacheReadCostUSD = snapshotDecimalString(values["cache_read_cost_usd"])
	details.CacheWriteCostUSD = snapshotDecimalString(values["cache_write_cost_usd"])
	details.OutputCostUSD = snapshotDecimalString(values["output_cost_usd"])
	details.CNYPerUSD = snapshotDecimalString(values["cny_per_usd"])
	details.BillingDisplayCurrency = snapshotString(values["billing_display_currency"])
	details.InputPriceSource = snapshotString(values["input_price_source"])
	details.CacheReadPriceSource = snapshotString(values["cache_read_price_source"])
	details.CacheWritePriceSource = snapshotString(values["cache_write_price_source"])
	details.OutputPriceSource = snapshotString(values["output_price_source"])

	if details.BillingLane == "" && details.PricingSource == "" && details.UsageSource == "" &&
		details.InputPriceUSDPer1MTokens == nil && details.CacheReadPriceUSDPer1MTokens == nil &&
		details.CacheWritePriceUSDPer1MTokens == nil && details.OutputPriceUSDPer1MTokens == nil &&
		details.InputCostUSD == nil && details.CacheReadCostUSD == nil && details.CacheWriteCostUSD == nil &&
		details.OutputCostUSD == nil && details.CNYPerUSD == nil && details.BillingDisplayCurrency == "" {
		return nil
	}
	return details
}

func pricingSnapshotValues(snapshot datatypes.JSON) map[string]interface{} {
	if len(snapshot) == 0 {
		return nil
	}
	decoder := json.NewDecoder(strings.NewReader(string(snapshot)))
	decoder.UseNumber()
	values := map[string]interface{}{}
	if err := decoder.Decode(&values); err != nil {
		return nil
	}
	return values
}

func snapshotDecimalString(value interface{}) *string {
	parsed, ok := snapshotDecimal(value)
	if !ok {
		return nil
	}
	result := parsed.String()
	return &result
}

func snapshotString(value interface{}) string {
	result, _ := value.(string)
	return strings.TrimSpace(result)
}

func pricingSnapshotCostUSD(snapshot datatypes.JSON) (decimal.Decimal, bool) {
	values := pricingSnapshotValues(snapshot)
	if values == nil {
		return decimal.Zero, false
	}
	if total, ok := snapshotDecimal(values["total_cost_usd"]); ok {
		return total, true
	}
	total := decimal.Zero
	found := false
	for _, key := range []string{"input_cost_usd", "cache_read_cost_usd", "cache_write_cost_usd", "output_cost_usd"} {
		if value, ok := snapshotDecimal(values[key]); ok {
			total = total.Add(value)
			found = true
		}
	}
	return total, found
}

func pricingSnapshotCostCNY(snapshot datatypes.JSON) (decimal.Decimal, bool) {
	values := pricingSnapshotValues(snapshot)
	if values == nil {
		return decimal.Zero, false
	}
	return snapshotDecimal(values["total_cost_cny"])
}

func snapshotDecimal(value interface{}) (decimal.Decimal, bool) {
	var raw string
	switch typed := value.(type) {
	case string:
		raw = typed
	case json.Number:
		raw = typed.String()
	default:
		return decimal.Zero, false
	}
	parsed, err := decimal.NewFromString(strings.TrimSpace(raw))
	if err != nil || parsed.IsNegative() {
		return decimal.Zero, false
	}
	return parsed, true
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
