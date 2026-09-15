package gateway

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestApplyRouteAttributionSnapshotFillsPrivateRouteFields(t *testing.T) {
	routeID := uuid.New()
	bc := &BillingContext{
		ChannelID:       &routeID,
		ChannelProvider: "siliconflow",
		ModelName:       "deepseek-ai/DeepSeek-V4-Pro",
		ProviderName:    "agicto",
	}

	applyRouteAttributionSnapshot(bc, "硅基流动")

	var snapshot map[string]string
	if err := json.Unmarshal(bc.PricingSnapshot, &snapshot); err != nil {
		t.Fatalf("unmarshal snapshot: %v", err)
	}
	if snapshot["actual_provider"] != "siliconflow" {
		t.Fatalf("actual_provider = %q", snapshot["actual_provider"])
	}
	if snapshot["actual_channel_name"] != "硅基流动" {
		t.Fatalf("actual_channel_name = %q", snapshot["actual_channel_name"])
	}
	if snapshot["actual_channel_id"] != routeID.String() {
		t.Fatalf("actual_channel_id = %q", snapshot["actual_channel_id"])
	}
	if snapshot["actual_upstream_model"] != "deepseek-ai/DeepSeek-V4-Pro" {
		t.Fatalf("actual_upstream_model = %q", snapshot["actual_upstream_model"])
	}
}

func TestSettlementFromQueryResultHydratesActualRoutingFromSnapshot(t *testing.T) {
	result := &settlementQueryResponse{
		SettlementID:     "settlement-1",
		SettlementStatus: "SETTLED",
		SettledCredits:   9,
		PricingSnapshot:  []byte(`{"actual_provider":"siliconflow","actual_channel_name":"硅基流动","total_cost_usd":"0.01"}`),
	}
	settlement := settlementFromQueryResult(result)
	if settlement == nil || settlement.SettlementID != "settlement-1" {
		t.Fatalf("settlement = %#v", settlement)
	}
	if settlement.ActualProvider != "siliconflow" || settlement.ActualChannelName != "硅基流动" || settlement.TotalCostUSD != "0.01" {
		t.Fatalf("settlement routing = %#v", settlement)
	}
}

func TestFinalizeMediaTimingSnapshotRecordsMediaDuration(t *testing.T) {
	startedAt := time.Date(2026, 8, 8, 9, 0, 0, 0, time.UTC)
	finishedAt := startedAt.Add(1500 * time.Millisecond)
	bc := &BillingContext{
		PricingOperation: PricingOperationImage,
		RequestCreatedAt: startedAt,
		SettledAt:        finishedAt,
		ResponseTime:     1500,
	}

	finalizeMediaTimingSnapshot(bc)

	var snapshot map[string]interface{}
	if err := json.Unmarshal(bc.PricingSnapshot, &snapshot); err != nil {
		t.Fatalf("unmarshal snapshot: %v", err)
	}
	if snapshot["media_started_at"] != startedAt.Format(time.RFC3339Nano) {
		t.Fatalf("media_started_at = %#v", snapshot["media_started_at"])
	}
	if snapshot["media_completed_at"] != finishedAt.Format(time.RFC3339Nano) {
		t.Fatalf("media_completed_at = %#v", snapshot["media_completed_at"])
	}
	if snapshot["media_operation"] != string(PricingOperationImage) {
		t.Fatalf("media_operation = %#v", snapshot["media_operation"])
	}
}

func TestApplyRouteAttributionWithLookupUsesRouteTable(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`CREATE TABLE llm_routes (id TEXT PRIMARY KEY, name TEXT NOT NULL)`).Error; err != nil {
		t.Fatal(err)
	}
	routeID := uuid.New()
	if err := db.Exec(`INSERT INTO llm_routes (id, name) VALUES (?, ?)`, routeID.String(), "Private Route").Error; err != nil {
		t.Fatal(err)
	}

	service := &BillingService{db: db}
	bc := &BillingContext{
		RouteID:         &routeID,
		ChannelProvider: "openai",
	}
	service.applyRouteAttributionWithLookup(context.Background(), bc)

	var snapshot map[string]string
	if err := json.Unmarshal(bc.PricingSnapshot, &snapshot); err != nil {
		t.Fatalf("unmarshal snapshot: %v", err)
	}
	if snapshot["actual_channel_name"] != "Private Route" || snapshot["actual_provider"] != "openai" {
		t.Fatalf("snapshot = %#v", snapshot)
	}
}

func TestApplyPlatformSettlementCostSnapshotFallsBackToBillingContext(t *testing.T) {
	bc := &BillingContext{
		ChannelProvider: "dashscope",
		ModelName:       "qwen-plus",
	}
	applyPlatformSettlementCostSnapshot(bc, &adapter.SettlementResult{
		SettlementID:   "settlement-1",
		OfficialPoints: 3,
		Status:         "settled",
	})

	var snapshot map[string]string
	if err := json.Unmarshal(bc.PricingSnapshot, &snapshot); err != nil {
		t.Fatalf("unmarshal snapshot: %v", err)
	}
	if snapshot["actual_provider"] != "dashscope" {
		t.Fatalf("actual_provider = %q", snapshot["actual_provider"])
	}
	if snapshot["actual_upstream_model"] != "qwen-plus" {
		t.Fatalf("actual_upstream_model = %q", snapshot["actual_upstream_model"])
	}
}
