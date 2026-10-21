package gateway

import (
	"encoding/json"
	"testing"

	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"gorm.io/datatypes"
)

func platformSettlementFromSnapshot(raw string) (*adapter.SettlementResult, datatypes.JSON) {
	body := json.RawMessage(raw)
	return settlementFromQueryResult(&settlementQueryResponse{
		SettlementID:     "settlement-1",
		SettlementStatus: "SETTLED",
		SettledCredits:   3,
		PricingSnapshot:  body,
	}), datatypes.JSON(body)
}

// TestApplyPlatformSettlementCostSnapshotReadsConsoleCnyPerUsd guards the
// canonical §7.1 USD settlement form: cny_per_usd carries the USD->CNY rate.
func TestApplyPlatformSettlementCostSnapshotReadsConsoleCnyPerUsd(t *testing.T) {
	settlement, snapshot := platformSettlementFromSnapshot(`{"currency":"USD","exchange_rate":"6","cny_per_usd":"6"}`)
	bc := &BillingContext{PricingSnapshot: snapshot}
	if err := applyPlatformSettlementCostSnapshot(bc, settlement); err != nil {
		t.Fatalf("canonical cny_per_usd should be accepted, got error = %v", err)
	}
	assertCurrencySegment(t, bc.PricingSnapshot, "6")
}

// TestApplyPlatformSettlementCostSnapshotReadsLegacyMeteredCnyRate guards the
// legacy metered CNY form, which records the USD->CNY rate in exchange_rate
// because it carries no cny_per_usd. The gateway must mirror Console's reader.
func TestApplyPlatformSettlementCostSnapshotReadsLegacyMeteredCnyRate(t *testing.T) {
	settlement, snapshot := platformSettlementFromSnapshot(`{"currency":"CNY","exchange_rate":"6.5","amount":"1"}`)
	bc := &BillingContext{PricingSnapshot: snapshot}
	if err := applyPlatformSettlementCostSnapshot(bc, settlement); err != nil {
		t.Fatalf("legacy metered CNY exchange_rate should be accepted, got error = %v", err)
	}
	assertCurrencySegment(t, bc.PricingSnapshot, "6.5")
}

// TestApplyPlatformSettlementCostSnapshotRejectsUsdIdentityExchangeRate guards
// that a USD identity exchange_rate of "1" is never mistaken for a CNY rate,
// exactly as Console's recordedCNYPerUSDRateFromPricingSnapshot does.
func TestApplyPlatformSettlementCostSnapshotRejectsUsdIdentityExchangeRate(t *testing.T) {
	settlement, snapshot := platformSettlementFromSnapshot(`{"currency":"USD","exchange_rate":"1"}`)
	bc := &BillingContext{PricingSnapshot: snapshot}
	if err := applyPlatformSettlementCostSnapshot(bc, settlement); err == nil {
		t.Fatalf("USD identity exchange_rate must not be accepted as a CNY rate")
	}
}

func assertCurrencySegment(t *testing.T, snapshot datatypes.JSON, want string) {
	t.Helper()
	values := pricingSnapshotMap(snapshot)
	if values["rate"] != want || values["cny_per_usd"] != want {
		t.Fatalf("currency segment = %#v, want rate/cny_per_usd = %s", values, want)
	}
	if values["rate_source"] != exchangeRateSourceConsoleSystem {
		t.Fatalf("rate_source = %#v, want %q", values["rate_source"], exchangeRateSourceConsoleSystem)
	}
}
