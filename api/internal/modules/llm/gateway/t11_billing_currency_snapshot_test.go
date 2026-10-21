package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	llmmodel "github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/model"
	modelpricing "github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/pricing"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"gorm.io/datatypes"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func t11Snapshot(t *testing.T, raw datatypes.JSON) map[string]interface{} {
	t.Helper()
	values := map[string]interface{}{}
	if err := json.Unmarshal(raw, &values); err != nil {
		t.Fatalf("unmarshal pricing snapshot: %v", err)
	}
	return values
}

// T11 acceptance 1 and 2: a private pre-deduct writes rate_source=pre_org with
// the organization rate, the call-time display currency, and rate_at.
func TestT11PrivatePreDeductWritesCanonicalCurrencySnapshot(t *testing.T) {
	fixture := newT9BillingFixture(t)
	bc := fixture.billingContext("t11-private-prededuct-a1", 30)

	if err := fixture.service.PreDeduct(t.Context(), bc); err != nil {
		t.Fatalf("PreDeduct() error = %v", err)
	}

	snapshot := t11Snapshot(t, bc.PricingSnapshot)
	if snapshot["rate_source"] != "pre_org" {
		t.Fatalf("rate_source = %#v, want pre_org", snapshot["rate_source"])
	}
	if snapshot["rate"] != "7.2" {
		t.Fatalf("rate = %#v, want 7.2", snapshot["rate"])
	}
	if snapshot["display_currency"] != "USD" {
		t.Fatalf("display_currency = %#v, want USD", snapshot["display_currency"])
	}
	if snapshot["rate_at"] == nil || snapshot["rate_at"] == "" {
		t.Fatalf("rate_at missing: %#v", snapshot)
	}
	if snapshot["cny_per_usd"] != "7.2" {
		t.Fatalf("cny_per_usd = %#v, want 7.2", snapshot["cny_per_usd"])
	}
}

// T11 acceptance 2: the usage bill carries the same three-part currency
// snapshot after settlement.
func TestT11PrivateUsageBillCarriesCurrencySnapshot(t *testing.T) {
	fixture := newT9BillingFixture(t)
	bc := fixture.billingContext("t11-private-bill-a1", 30)

	if err := fixture.service.PreDeduct(t.Context(), bc); err != nil {
		t.Fatalf("PreDeduct() error = %v", err)
	}
	bc.ActualCredits = 12
	bc.Status = billingContextStatusSuccess
	if err := fixture.service.Settle(t.Context(), bc); err != nil {
		t.Fatalf("Settle() error = %v", err)
	}

	var bill UsageBill
	if err := fixture.db.First(&bill, "attempt_id = ?", bc.AttemptID).Error; err != nil {
		t.Fatalf("load usage bill: %v", err)
	}
	snapshot := t11Snapshot(t, bill.PricingSnapshot)
	if snapshot["rate_source"] != "pre_org" || snapshot["rate"] != "7.2" {
		t.Fatalf("usage bill currency segment = %#v", snapshot)
	}
}

// T11 acceptance 1: platform settlement mirrors the Console-recorded rate and
// marks it console_system.
func TestT11PlatformSettlementUsesConsoleRecordedRate(t *testing.T) {
	bc := &BillingContext{}
	if err := applyPlatformSettlementCostSnapshot(bc, &adapter.SettlementResult{
		SettlementID: "settlement-1",
		CNYPerUSD:    "6.5",
	}); err != nil {
		t.Fatalf("applyPlatformSettlementCostSnapshot() error = %v", err)
	}

	snapshot := t11Snapshot(t, bc.PricingSnapshot)
	if snapshot["rate_source"] != "console_system" {
		t.Fatalf("rate_source = %#v, want console_system", snapshot["rate_source"])
	}
	if snapshot["rate"] != "6.5" {
		t.Fatalf("rate = %#v, want 6.5", snapshot["rate"])
	}
	if snapshot["display_currency"] != "USD" {
		t.Fatalf("display_currency = %#v, want USD default", snapshot["display_currency"])
	}
}

// T11 acceptance 2: a platform settlement with no Console rate is fail-closed.
func TestT11PlatformSettlementMissingRateFailsClosed(t *testing.T) {
	bc := &BillingContext{}
	err := applyPlatformSettlementCostSnapshot(bc, &adapter.SettlementResult{SettlementID: "settlement-1"})
	if !errors.Is(err, ErrExchangeRateUnavailable) {
		t.Fatalf("error = %v, want ErrExchangeRateUnavailable", err)
	}
}

// T11 acceptance 3: an organization with no configured presentment currency
// resolves to USD, never CNY.
func TestT11OrganizationDisplayCurrencyDefaultsToUSD(t *testing.T) {
	db := t11OrganizationDB(t, "", "7.25")
	bc := &BillingContext{OrganizationID: "org-1"}

	if err := captureOrganizationBillingCurrency(context.Background(), db, bc); err != nil {
		t.Fatalf("captureOrganizationBillingCurrency() error = %v", err)
	}
	if bc.DisplayCurrency != "USD" {
		t.Fatalf("display currency = %q, want USD", bc.DisplayCurrency)
	}
	if !bc.USDToCNYRate.Equal(decimal.RequireFromString("7.25")) {
		t.Fatalf("usd_to_cny_rate = %s, want 7.25", bc.USDToCNYRate)
	}
}

// T11 acceptance 2: an unreadable organization rate is fail-closed instead of
// silently defaulting to 7.
func TestT11OrganizationMissingRateFailsClosed(t *testing.T) {
	db := t11OrganizationDB(t, "CNY", "0")
	bc := &BillingContext{OrganizationID: "org-1"}

	err := captureOrganizationBillingCurrency(context.Background(), db, bc)
	if !errors.Is(err, ErrExchangeRateUnavailable) {
		t.Fatalf("error = %v, want ErrExchangeRateUnavailable", err)
	}
}

// T11: the shared organization rate loader never returns a hard-coded fallback.
func TestT11LoadOrganizationRateHasNoSilentFallback(t *testing.T) {
	if _, err := loadOrganizationUSDToCNYRate(context.Background(), nil, uuid.Nil); !errors.Is(err, ErrExchangeRateUnavailable) {
		t.Fatalf("nil database error = %v, want ErrExchangeRateUnavailable", err)
	}
	db := t11OrganizationDB(t, "CNY", "7.25")
	if _, err := loadOrganizationUSDToCNYRate(context.Background(), db, uuid.New()); !errors.Is(err, ErrExchangeRateUnavailable) {
		t.Fatalf("missing organization error = %v, want ErrExchangeRateUnavailable", err)
	}
}

// T11 §3.5#14: a CNY video price book without a rate is fail-closed instead of
// silently quoting at 7.
func TestT11VideoCNYPricingMissingRateFailsClosed(t *testing.T) {
	selection := &ProviderSelection{Model: llmmodel.LLMModel{
		ID:       uuid.New(),
		Provider: "example",
		Model:    "wan-video",
		Currency: "CNY",
		Pricing:  datatypes.JSON([]byte(`{"video_generation":{"currency":"CNY","billing_unit":"second","rates":[{"price_per_second":1}]}}`)),
	}}

	_, err := quoteVideoFromModelPricing(selection, "wan-video", modelpricing.VideoGenerationCostRequest{
		DurationSeconds: 5,
		Count:           1,
	}, decimal.Zero)
	if !errors.Is(err, ErrExchangeRateUnavailable) {
		t.Fatalf("error = %v, want ErrExchangeRateUnavailable", err)
	}
}

func t11OrganizationDB(t *testing.T, displayCurrency, rate string) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+uuid.NewString()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite db: %v", err)
	}
	if err := db.Exec(`CREATE TABLE organizations (
		id TEXT PRIMARY KEY,
		billing_display_currency TEXT NOT NULL,
		usd_to_cny_rate TEXT NOT NULL
	)`).Error; err != nil {
		t.Fatalf("create organizations table: %v", err)
	}
	if err := db.Exec(`INSERT INTO organizations (id, billing_display_currency, usd_to_cny_rate) VALUES (?, ?, ?)`, "org-1", displayCurrency, rate).Error; err != nil {
		t.Fatalf("seed organization: %v", err)
	}
	return db
}
