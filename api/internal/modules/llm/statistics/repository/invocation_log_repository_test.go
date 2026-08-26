package repository

import (
	"context"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/zgiai/zgi/api/internal/modules/llm/statistics/dto"
	"gorm.io/datatypes"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestPricingSnapshotCostUSDUsesExactComponentsBeforeRoundedCredits(t *testing.T) {
	snapshot := datatypes.JSON(`{
		"input_cost_usd":"0.000043065",
		"cache_read_cost_usd":"0.000001856",
		"cache_write_cost_usd":"0",
		"output_cost_usd":"0.00012267"
	}`)
	got, ok := pricingSnapshotCostUSD(snapshot)
	want := decimal.RequireFromString("0.000167591")
	if !ok || !got.Equal(want) {
		t.Fatalf("pricingSnapshotCostUSD() = %s, %t; want %s, true", got, ok, want)
	}
}

func TestPricingSnapshotCostCNYUsesRecordedSettlementAmount(t *testing.T) {
	got, ok := pricingSnapshotCostCNY(datatypes.JSON(`{"total_cost_cny":"0.0475272"}`))
	want := decimal.RequireFromString("0.0475272")
	if !ok || !got.Equal(want) {
		t.Fatalf("pricingSnapshotCostCNY() = %s, %t; want %s, true", got, ok, want)
	}
}

func TestAggregateInvocationBillsKeepsRecordedCNYTotal(t *testing.T) {
	now := time.Now().UTC()
	items := aggregateInvocationBills(
		[]invocationPageRow{{RequestID: "request-1"}},
		[]invocationBillRow{{
			RequestID: "request-1", Status: "success", TotalPoints: 6601,
			PricingSnapshot:  datatypes.JSON(`{"total_cost_usd":"0.006601","total_cost_cny":"0.0475272","cny_per_usd":"7.2"}`),
			RequestCreatedAt: now, SettledAt: now,
		}},
	)
	if len(items) != 1 || items[0].TotalCostCNY == nil || *items[0].TotalCostCNY != "0.0475272" {
		t.Fatalf("aggregated items = %#v", items)
	}
}

func TestInvocationPricingDetailsExposeActualTokenPricesAndSources(t *testing.T) {
	bill := invocationBillRow{
		BillingLane:   "private",
		PricingSource: "upstream_model_price",
		UsageSource:   "provider_usage",
		PricingSnapshot: datatypes.JSON(`{
			"input_price_usd_per_1m_tokens":"5",
			"cache_read_price_usd_per_1m_tokens":"0.5",
			"cache_write_price_usd_per_1m_tokens":"0",
			"output_price_usd_per_1m_tokens":"30",
			"input_cost_usd":"0.001995",
			"cache_read_cost_usd":"0.004096",
			"cache_write_cost_usd":"0",
			"output_cost_usd":"0.00051",
			"cny_per_usd":"7.2",
			"billing_display_currency":"CNY",
			"cache_read_price_source":"synced_model",
			"cache_write_price_source":"organization_override"
		}`),
	}

	details := invocationPricingDetails(bill)
	if details == nil {
		t.Fatal("invocationPricingDetails() = nil")
	}
	if details.BillingLane != "private" || details.PricingSource != "upstream_model_price" {
		t.Fatalf("unexpected sources: %#v", details)
	}
	if details.CacheReadPriceUSDPer1MTokens == nil || *details.CacheReadPriceUSDPer1MTokens != "0.5" {
		t.Fatalf("cache read price = %#v, want 0.5", details.CacheReadPriceUSDPer1MTokens)
	}
	if details.CacheReadCostUSD == nil || *details.CacheReadCostUSD != "0.004096" {
		t.Fatalf("cache read cost = %#v, want 0.004096", details.CacheReadCostUSD)
	}
	if details.CNYPerUSD == nil || *details.CNYPerUSD != "7.2" {
		t.Fatalf("call-time exchange rate = %#v, want 7.2", details.CNYPerUSD)
	}
	if details.BillingDisplayCurrency != "CNY" {
		t.Fatalf("call-time billing currency = %q, want CNY", details.BillingDisplayCurrency)
	}
	if details.CacheReadPriceSource != "synced_model" || details.CacheWritePriceSource != "organization_override" {
		t.Fatalf("unexpected cache price sources: %#v", details)
	}
}

func TestInvocationPricingDetailsKeepPlatformSettlementWithoutInventingPrices(t *testing.T) {
	details := invocationPricingDetails(invocationBillRow{BillingLane: "platform"})
	if details == nil || details.BillingLane != "platform" {
		t.Fatalf("platform details = %#v", details)
	}
	if details.InputPriceUSDPer1MTokens != nil || details.CacheReadPriceUSDPer1MTokens != nil || details.OutputPriceUSDPer1MTokens != nil {
		t.Fatalf("platform settlement must not invent token prices: %#v", details)
	}
}

func TestGetInvocationLogGroupsAttemptsAndIsolatesOrganization(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`CREATE TABLE llm_usage_bills (
		request_id TEXT NOT NULL, organization_id TEXT NOT NULL, app_id TEXT, app_type TEXT,
		invocation_source TEXT NOT NULL, model_name TEXT NOT NULL, provider_name TEXT NOT NULL,
		channel_id TEXT,
		status TEXT NOT NULL, prompt_tokens INTEGER NOT NULL, cache_read_tokens INTEGER NOT NULL DEFAULT 0, cache_write_tokens INTEGER NOT NULL DEFAULT 0, completion_tokens INTEGER NOT NULL,
		total_tokens INTEGER NOT NULL, total_points INTEGER NOT NULL, response_time_ms INTEGER NOT NULL,
		error_code TEXT, request_created_at DATETIME NOT NULL, settled_at DATETIME NOT NULL
	)`).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`CREATE TABLE llm_routes (id TEXT PRIMARY KEY, name TEXT NOT NULL)`).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Table("llm_routes").Create(map[string]any{
		"id": "channel-1", "name": "Official Cloud A",
	}).Error; err != nil {
		t.Fatal(err)
	}
	createInvocationContentAvailabilityTable(t, db)

	started := time.Date(2026, 8, 8, 9, 0, 0, 0, time.UTC)
	insertInvocationBill(t, db, "org-1", "req-retried", "api", "failed", started, 0, 0)
	insertInvocationBill(t, db, "org-1", "req-retried", "api", "success", started.Add(time.Second), 30, 12)
	insertInvocationBill(t, db, "org-1", "req-product", "product", "success", started.Add(2*time.Second), 20, 8)
	insertInvocationBill(t, db, "org-2", "req-other-org", "api", "success", started.Add(3*time.Second), 999, 999)
	if err := db.Table("llm_invocation_contents").Create(map[string]any{
		"request_id": "req-product", "organization_id": "org-1", "expires_at": time.Now().UTC().Add(24 * time.Hour),
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Table(usageBillTable).Where("request_id = ?", "req-product").Updates(map[string]any{
		"app_type": "agent", "model_name": "agent-model", "channel_id": "channel-1",
	}).Error; err != nil {
		t.Fatal(err)
	}

	repo := &statisticsRepositoryImpl{db: db}
	result, err := repo.GetInvocationLog(context.Background(), "org-1", &dto.InvocationLogRequest{
		StartTime: started.Add(-time.Hour).Unix(), EndTime: started.Add(time.Hour).Unix(), Limit: 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Summary.InvocationCount != 2 || result.Summary.APICount != 1 || result.Summary.ProductCount != 1 {
		t.Fatalf("unexpected summary: %#v", result.Summary)
	}
	if result.Summary.TotalTokens != 50 || result.Summary.TotalPoints != 20 {
		t.Fatalf("unexpected totals: %#v", result.Summary)
	}
	if len(result.Items) != 2 {
		t.Fatalf("items = %d, want 2", len(result.Items))
	}
	var product *dto.InvocationLogItem
	var retried *dto.InvocationLogItem
	for index := range result.Items {
		if result.Items[index].InvocationID == "req-product" {
			product = &result.Items[index]
		}
		if result.Items[index].InvocationID == "req-retried" {
			retried = &result.Items[index]
		}
	}
	if retried == nil || retried.AttemptCount != 2 || retried.Status != "success" || retried.TotalTokens != 30 {
		t.Fatalf("unexpected retried invocation: %#v", retried)
	}
	if product == nil || product.ChannelName != "Official Cloud A" || !product.ContentAvailable || product.ContentExpiresAt == nil || retried.ContentAvailable {
		t.Fatalf("unexpected content availability: product=%#v retried=%#v", product, retried)
	}

	productSource := "product"
	agentApp := "agent"
	agentModel := "agent-model"
	filtered, err := repo.GetInvocationLog(context.Background(), "org-1", &dto.InvocationLogRequest{
		StartTime: started.Add(-time.Hour).Unix(), EndTime: started.Add(time.Hour).Unix(), Limit: 20,
		InvocationSource: &productSource, AppType: &agentApp, ModelName: &agentModel,
	})
	if err != nil || len(filtered.Items) != 1 || filtered.Items[0].InvocationID != "req-product" {
		t.Fatalf("unexpected filtered result: result=%#v err=%v", filtered, err)
	}
	if filtered.Summary.InvocationCount != 1 || filtered.Summary.ProductCount != 1 || filtered.Summary.TotalTokens != 20 {
		t.Fatalf("unexpected filtered summary: %#v", filtered.Summary)
	}

	firstPage, err := repo.GetInvocationLog(context.Background(), "org-1", &dto.InvocationLogRequest{
		StartTime: started.Add(-time.Hour).Unix(), EndTime: started.Add(time.Hour).Unix(), Limit: 1,
	})
	if err != nil || len(firstPage.Items) != 1 || firstPage.NextCursor == nil {
		t.Fatalf("unexpected first page: result=%#v err=%v", firstPage, err)
	}
	includeSummary := false
	secondPage, err := repo.GetInvocationLog(context.Background(), "org-1", &dto.InvocationLogRequest{
		StartTime: started.Add(-time.Hour).Unix(), EndTime: started.Add(time.Hour).Unix(), Limit: 1,
		CursorTime: &firstPage.NextCursor.Time, CursorID: &firstPage.NextCursor.ID,
		IncludeSummary: &includeSummary,
	})
	if err != nil || len(secondPage.Items) != 1 || secondPage.Items[0].InvocationID == firstPage.Items[0].InvocationID {
		t.Fatalf("unexpected second page: result=%#v err=%v", secondPage, err)
	}
	if secondPage.Summary.InvocationCount != 0 || secondPage.Summary.TotalTokens != 0 {
		t.Fatalf("summary should be omitted on cursor page: %#v", secondPage.Summary)
	}
}

func TestGetInvocationLogCursorPreservesSubMillisecondPrecision(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`CREATE TABLE llm_usage_bills (
		request_id TEXT NOT NULL, organization_id TEXT NOT NULL, app_id TEXT, app_type TEXT,
		invocation_source TEXT NOT NULL, model_name TEXT NOT NULL, provider_name TEXT NOT NULL,
		status TEXT NOT NULL, prompt_tokens INTEGER NOT NULL, cache_read_tokens INTEGER NOT NULL DEFAULT 0, cache_write_tokens INTEGER NOT NULL DEFAULT 0, completion_tokens INTEGER NOT NULL,
		total_tokens INTEGER NOT NULL, total_points INTEGER NOT NULL, response_time_ms INTEGER NOT NULL,
		error_code TEXT, request_created_at DATETIME NOT NULL, settled_at DATETIME NOT NULL
	)`).Error; err != nil {
		t.Fatal(err)
	}
	createInvocationContentAvailabilityTable(t, db)

	base := time.Date(2026, 8, 8, 9, 0, 0, 123_000_000, time.UTC)
	insertInvocationBill(t, db, "org-1", "req-a", "api", "success", base.Add(100*time.Nanosecond), 1, 1)
	insertInvocationBill(t, db, "org-1", "req-b", "api", "success", base.Add(200*time.Nanosecond), 1, 1)
	insertInvocationBill(t, db, "org-1", "req-c", "api", "success", base.Add(300*time.Nanosecond), 1, 1)

	repo := &statisticsRepositoryImpl{db: db}
	request := dto.InvocationLogRequest{
		StartTime: base.Add(-time.Second).Unix(), EndTime: base.Add(time.Second).Unix(), Limit: 1,
	}
	seen := map[string]bool{}
	for {
		page, err := repo.GetInvocationLog(context.Background(), "org-1", &request)
		if err != nil {
			t.Fatal(err)
		}
		if len(page.Items) != 1 {
			t.Fatalf("page items = %d, want 1: %#v", len(page.Items), page)
		}
		if seen[page.Items[0].InvocationID] {
			t.Fatalf("duplicate invocation %q", page.Items[0].InvocationID)
		}
		seen[page.Items[0].InvocationID] = true
		if page.NextCursor == nil {
			break
		}
		request.CursorTime = &page.NextCursor.Time
		request.CursorID = &page.NextCursor.ID
	}
	if len(seen) != 3 {
		t.Fatalf("paginated invocations = %v, want all three", seen)
	}
}

func TestGetInvocationLogKeepsRetryAttemptsInsideSelectedTimeRange(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`CREATE TABLE llm_usage_bills (
		request_id TEXT NOT NULL, organization_id TEXT NOT NULL, app_id TEXT, app_type TEXT,
		invocation_source TEXT NOT NULL, model_name TEXT NOT NULL, provider_name TEXT NOT NULL,
		status TEXT NOT NULL, prompt_tokens INTEGER NOT NULL, cache_read_tokens INTEGER NOT NULL DEFAULT 0, cache_write_tokens INTEGER NOT NULL DEFAULT 0, completion_tokens INTEGER NOT NULL,
		total_tokens INTEGER NOT NULL, total_points INTEGER NOT NULL, response_time_ms INTEGER NOT NULL,
		error_code TEXT, request_created_at DATETIME NOT NULL, settled_at DATETIME NOT NULL
	)`).Error; err != nil {
		t.Fatal(err)
	}
	createInvocationContentAvailabilityTable(t, db)

	boundary := time.Date(2026, 8, 9, 0, 0, 0, 0, time.UTC)
	insertInvocationBill(t, db, "org-1", "req-boundary", "product", "failed", boundary.Add(-time.Second), 100, 40)
	insertInvocationBill(t, db, "org-1", "req-boundary", "product", "success", boundary.Add(time.Second), 20, 8)

	repo := &statisticsRepositoryImpl{db: db}
	result, err := repo.GetInvocationLog(context.Background(), "org-1", &dto.InvocationLogRequest{
		StartTime: boundary.Unix(), EndTime: boundary.Add(time.Hour).Unix(), Limit: 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Summary.InvocationCount != 1 || result.Summary.TotalTokens != 20 || result.Summary.TotalPoints != 8 {
		t.Fatalf("unexpected summary: %#v", result.Summary)
	}
	if len(result.Items) != 1 {
		t.Fatalf("items = %d, want 1", len(result.Items))
	}
	item := result.Items[0]
	if item.AttemptCount != 1 || item.TotalTokens != 20 || item.TotalPoints != 8 || item.StartedAt < boundary.UnixMilli() {
		t.Fatalf("out-of-range retry leaked into item: %#v", item)
	}
}

func TestGetInvocationLogIncludesFractionalPartOfEndSecond(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`CREATE TABLE llm_usage_bills (
		request_id TEXT NOT NULL, organization_id TEXT NOT NULL, app_id TEXT, app_type TEXT,
		invocation_source TEXT NOT NULL, model_name TEXT NOT NULL, provider_name TEXT NOT NULL,
		status TEXT NOT NULL, prompt_tokens INTEGER NOT NULL, cache_read_tokens INTEGER NOT NULL DEFAULT 0, cache_write_tokens INTEGER NOT NULL DEFAULT 0, completion_tokens INTEGER NOT NULL,
		total_tokens INTEGER NOT NULL, total_points INTEGER NOT NULL, response_time_ms INTEGER NOT NULL,
		error_code TEXT, request_created_at DATETIME NOT NULL, settled_at DATETIME NOT NULL
	)`).Error; err != nil {
		t.Fatal(err)
	}
	createInvocationContentAvailabilityTable(t, db)

	endSecond := time.Date(2026, 8, 9, 23, 59, 59, 0, time.UTC)
	insertInvocationBill(t, db, "org-1", "req-last-millisecond", "api", "success", endSecond.Add(999*time.Millisecond), 12, 3)
	insertInvocationBill(t, db, "org-1", "req-next-second", "api", "success", endSecond.Add(time.Second), 99, 99)

	repo := &statisticsRepositoryImpl{db: db}
	result, err := repo.GetInvocationLog(context.Background(), "org-1", &dto.InvocationLogRequest{
		StartTime: endSecond.Add(-time.Hour).Unix(), EndTime: endSecond.Unix(), Limit: 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Summary.InvocationCount != 1 || result.Summary.TotalTokens != 12 || len(result.Items) != 1 || result.Items[0].InvocationID != "req-last-millisecond" {
		t.Fatalf("unexpected end-second result: %#v", result)
	}
}

func TestGetInvocationLogKeepsBaseLogAvailableWithoutOptionalContentTable(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`CREATE TABLE llm_usage_bills (
		request_id TEXT NOT NULL, organization_id TEXT NOT NULL, app_id TEXT, app_type TEXT,
		invocation_source TEXT NOT NULL, model_name TEXT NOT NULL, provider_name TEXT NOT NULL,
		status TEXT NOT NULL, prompt_tokens INTEGER NOT NULL, cache_read_tokens INTEGER NOT NULL DEFAULT 0, cache_write_tokens INTEGER NOT NULL DEFAULT 0, completion_tokens INTEGER NOT NULL,
		total_tokens INTEGER NOT NULL, total_points INTEGER NOT NULL, response_time_ms INTEGER NOT NULL,
		error_code TEXT, request_created_at DATETIME NOT NULL, settled_at DATETIME NOT NULL
	)`).Error; err != nil {
		t.Fatal(err)
	}

	started := time.Now().UTC().Add(-time.Minute)
	insertInvocationBill(t, db, "org-1", "req-no-content-table", "api", "success", started, 12, 3)

	repo := &statisticsRepositoryImpl{db: db}
	result, err := repo.GetInvocationLog(context.Background(), "org-1", &dto.InvocationLogRequest{
		StartTime: started.Add(-time.Hour).Unix(), EndTime: started.Add(time.Hour).Unix(), Limit: 20,
	})
	if err != nil {
		t.Fatalf("optional content table must not break base log: %v", err)
	}
	if len(result.Items) != 1 || result.Items[0].InvocationID != "req-no-content-table" || result.Items[0].ContentAvailable {
		t.Fatalf("unexpected base log result: %#v", result)
	}
}

func insertInvocationBill(t *testing.T, db *gorm.DB, orgID, requestID, source, status string, started time.Time, tokens, points int64) {
	t.Helper()
	if err := db.Table(usageBillTable).Create(map[string]any{
		"request_id": requestID, "organization_id": orgID, "app_type": "workflow",
		"invocation_source": source, "model_name": "gpt-test", "provider_name": "test-provider",
		"status": status, "prompt_tokens": tokens / 2, "completion_tokens": tokens - tokens/2,
		"total_tokens": tokens, "total_points": points, "response_time_ms": 100,
		"request_created_at": started, "settled_at": started.Add(100 * time.Millisecond),
	}).Error; err != nil {
		t.Fatal(err)
	}
}

func createInvocationContentAvailabilityTable(t *testing.T, db *gorm.DB) {
	t.Helper()
	if err := db.Exec(`CREATE TABLE llm_invocation_contents (
		request_id TEXT PRIMARY KEY, organization_id TEXT NOT NULL, expires_at DATETIME NOT NULL
	)`).Error; err != nil {
		t.Fatal(err)
	}
}
