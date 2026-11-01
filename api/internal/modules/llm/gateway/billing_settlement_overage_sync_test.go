package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"gorm.io/gorm"
)

// seedSynchronousSettlement books the 100-credit hold seedStaleRemotePrededuct
// uses, behind a Console that fails the test if queried: the synchronous path
// must mirror the settlement that rode back on the proxied response.
func seedSynchronousSettlement(t *testing.T, meteredReserved int64) (*RemoteBilling, *BillingContext) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected Console settlement query: %s", r.URL.Path)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(server.Close)
	remote, bc := seedStaleRemotePrededuct(t, openRemoteBillingTestDB(t), &countingSettleQuotaClient{meteredReserved: meteredReserved}, server.URL)
	bc.Status = "success"
	return remote, bc
}

func syncSettlement(bc *BillingContext, official int64) *adapter.SettlementResult {
	return &adapter.SettlementResult{SettlementID: bc.DeductionID, OfficialPoints: official, Status: "settled", CNYPerUSD: "7"}
}

func usageBillSnapshot(t *testing.T, remote *RemoteBilling, bc *BillingContext) (UsageBill, map[string]interface{}) {
	t.Helper()
	var bill UsageBill
	if err := remote.localService.db.Where("attempt_id = ?", bc.AttemptID).First(&bill).Error; err != nil {
		t.Fatalf("usage bill missing: %v", err)
	}
	snapshot := map[string]interface{}{}
	if len(bill.PricingSnapshot) > 0 {
		if err := json.Unmarshal(bill.PricingSnapshot, &snapshot); err != nil {
			t.Fatal(err)
		}
	}
	return bill, snapshot
}

func assertNoOverageKeys(t *testing.T, remote *RemoteBilling, bc *BillingContext, official int64) {
	t.Helper()
	var attempt BillingAttempt
	if err := remote.localService.db.First(&attempt, "attempt_id = ?", bc.AttemptID).Error; err != nil {
		t.Fatal(err)
	}
	if attempt.Status != billingAttemptStatusSettled {
		t.Fatalf("attempt status = %s, want %s", attempt.Status, billingAttemptStatusSettled)
	}
	bill, snapshot := usageBillSnapshot(t, remote, bc)
	if bill.OfficialPoints.Int64() != official {
		t.Fatalf("usage bill official points = %d, want %d", bill.OfficialPoints, official)
	}
	for _, key := range []string{"overage_credits", "overage_strategy"} {
		if v, ok := snapshot[key]; ok {
			t.Fatalf("snapshot %s = %v, want absent", key, v)
		}
	}
}

// TestFinalizePlatformProxySettlementSyncRecordsOverage pins the ZGI-01
// synchronous gap: a settlement on the proxied response that charged above the
// hold must leave the same audit keys the Console-query path writes.
func TestFinalizePlatformProxySettlementSyncRecordsOverage(t *testing.T) {
	remote, bc := seedSynchronousSettlement(t, 0)
	if err := remote.FinalizePlatformProxySettlement(context.Background(), bc, syncSettlement(bc, 150)); err != nil {
		t.Fatalf("finalize synchronous overage settlement: %v", err)
	}
	assertOverageMirrored(t, remote, bc, 150, 50)
	if bc.ConsoleReservedCredits != nil {
		t.Fatalf("token hold = %d, want the estimate (nil)", *bc.ConsoleReservedCredits)
	}
}

func TestFinalizePlatformProxySettlementSyncWithinReserveOmitsOverage(t *testing.T) {
	for _, official := range []int64{60, 100} {
		remote, bc := seedSynchronousSettlement(t, 0)
		if err := remote.FinalizePlatformProxySettlement(context.Background(), bc, syncSettlement(bc, official)); err != nil {
			t.Fatalf("finalize synchronous settlement %d: %v", official, err)
		}
		assertNoOverageKeys(t, remote, bc, official)
	}
}

func fundEntry(t *testing.T, remote *RemoteBilling, bc *BillingContext) BillingAttemptEntry {
	t.Helper()
	var fund BillingAttemptEntry
	if err := remote.localService.db.Where("attempt_id = ? AND entry_type = ? AND ledger_type = ?", bc.AttemptID, billingEntryTypeFund, billingLedgerTypeOrgFunds).First(&fund).Error; err != nil {
		t.Fatal(err)
	}
	return fund
}

func assertFundEntry(t *testing.T, remote *RemoteBilling, bc *BillingContext, reserved, actual, refunded int64) {
	t.Helper()
	fund := fundEntry(t, remote, bc)
	if fund.ReservedAmount.Int64() != reserved || fund.ActualAmount.Int64() != actual || fund.RefundedAmount.Int64() != refunded {
		t.Fatalf("fund entry reserved=%d actual=%d refunded=%d, want %d/%d/%d", fund.ReservedAmount, fund.ActualAmount, fund.RefundedAmount, reserved, actual, refunded)
	}
}

func assertOverageKeys(t *testing.T, remote *RemoteBilling, bc *BillingContext, official, overage int64) {
	t.Helper()
	bill, snapshot := usageBillSnapshot(t, remote, bc)
	if bill.OfficialPoints.Int64() != official {
		t.Fatalf("usage bill official points = %d, want %d", bill.OfficialPoints, official)
	}
	if snapshot["overage_credits"] != float64(overage) || snapshot["overage_strategy"] != settlementOverageStrategyChargeDifference {
		t.Fatalf("snapshot overage = %v/%v, want %d/%s", snapshot["overage_credits"], snapshot["overage_strategy"], overage, settlementOverageStrategyChargeDifference)
	}
}

// TestMeteredPreDeductRecordsConsoleHold pins that the org_funds entry mirrors
// the hold Console priced from the metered quantity, not the local estimate,
// while the subject quota keeps the reservation Pre made itself.
func TestMeteredPreDeductRecordsConsoleHold(t *testing.T) {
	remote, bc := seedSynchronousSettlement(t, 120)
	if bc.ConsoleReservedCredits == nil || *bc.ConsoleReservedCredits != 120 {
		t.Fatalf("console hold = %v, want 120", bc.ConsoleReservedCredits)
	}
	fund := fundEntry(t, remote, bc)
	if fund.ReservedAmount.Int64() != 120 || fund.IdempotencyKey == nil || *fund.IdempotencyKey != bc.DeductionID {
		t.Fatalf("fund entry reserved=%d key=%v, want 120/%s", fund.ReservedAmount, fund.IdempotencyKey, bc.DeductionID)
	}
	var subject BillingAttemptEntry
	if err := remote.localService.db.Where("attempt_id = ? AND entry_type = ?", bc.AttemptID, billingEntryTypeSubject).First(&subject).Error; err != nil {
		t.Fatal(err)
	}
	if subject.ReservedAmount.Int64() != 100 {
		t.Fatalf("subject entry reserved = %d, want the local 100", subject.ReservedAmount)
	}
}

// TestFinalizePlatformProxySettlementSyncMeteredUsesConsoleHold pins that a
// synchronous metered settlement refunds and measures overage against the hold
// Console booked, including on a later request (a video retrieve) that rebuilds
// the context from the local estimate.
func TestFinalizePlatformProxySettlementSyncMeteredUsesConsoleHold(t *testing.T) {
	for _, tc := range []struct {
		name            string
		laterRequest    bool
		official        int64
		refunded        int64
		overage         int64
		overageExpected bool
	}{
		{"same request overage", false, 150, 0, 30, true},
		{"later request overage", true, 150, 0, 30, true},
		{"within the console hold", false, 110, 10, 0, false},
		{"later request within the console hold", true, 110, 10, 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			remote, bc := seedSynchronousSettlement(t, 120)
			if tc.laterRequest {
				bc.ConsoleReservedCredits = nil
			}
			if err := remote.FinalizePlatformProxySettlement(context.Background(), bc, syncSettlement(bc, tc.official)); err != nil {
				t.Fatalf("finalize metered settlement: %v", err)
			}
			assertFundEntry(t, remote, bc, 120, tc.official, tc.refunded)
			if tc.overageExpected {
				assertOverageKeys(t, remote, bc, tc.official, tc.overage)
			} else {
				assertNoOverageKeys(t, remote, bc, tc.official)
			}
		})
	}
}

// TestStaleMeteredRecoveryUsesConsoleHold pins the recovery mirror: it checks
// Console's amounts and measures overage against the org_funds hold, so that
// hold must be Console's. With the local estimate (100) a refund inside the
// 120 hold was rejected and an overage was overstated.
func TestStaleMeteredRecoveryUsesConsoleHold(t *testing.T) {
	for _, tc := range []struct {
		name              string
		settled, refunded int64
		overageExpected   bool
	}{
		{"refund inside the console hold", 110, 10, false},
		{"overage above the console hold", 150, 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := openRemoteBillingTestDB(t)
			var bc *BillingContext
			server := consoleSettlementServer(t, func(bc *BillingContext) string {
				return fmt.Sprintf(`{"found":true,"settlement_id":%q,"deduction_id":%q,"attempt_id":%q,"organization_id":%q,"request_id":%q,"settlement_status":"SETTLED","reserved_credits":120,"settled_credits":%d,"refunded_credits":%d,"retryable":false,"pricing_snapshot":{"currency":"USD","cny_per_usd":"7","points_per_usd":"1000"}}`,
					bc.DeductionID, bc.DeductionID, bc.AttemptID, bc.OrganizationID, bc.RequestID, tc.settled, tc.refunded)
			}, &bc)
			remote, seeded := seedStaleRemotePrededuct(t, db, &countingSettleQuotaClient{meteredReserved: 120}, server.URL)
			bc = seeded
			if err := remote.recoverStalePredeductedRemoteAttempts(context.Background()); err != nil {
				t.Fatal(err)
			}
			var attempt BillingAttempt
			if err := db.First(&attempt, "attempt_id = ?", bc.AttemptID).Error; err != nil {
				t.Fatal(err)
			}
			if attempt.Status != billingAttemptStatusSettled {
				t.Fatalf("attempt status = %s, want %s", attempt.Status, billingAttemptStatusSettled)
			}
			assertFundEntry(t, remote, bc, 120, tc.settled, tc.refunded)
			if tc.overageExpected {
				assertOverageKeys(t, remote, bc, tc.settled, tc.settled-120)
			} else {
				assertNoOverageKeys(t, remote, bc, tc.settled)
			}
		})
	}
}

func TestFundReservedCredits(t *testing.T) {
	hold := int64(120)
	for _, tc := range []struct {
		name string
		bc   *BillingContext
		want int64
	}{
		{"nil context", nil, 0},
		{"token estimate", &BillingContext{EstimatedCredits: 185}, 185},
		{"console hold", &BillingContext{EstimatedCredits: 100, ConsoleReservedCredits: &hold}, 120},
	} {
		if got := fundReservedCredits(tc.bc); got != tc.want {
			t.Errorf("%s: fund hold = %d, want %d", tc.name, got, tc.want)
		}
	}
	// A zero reservation must leave the snapshot alone rather than record the
	// whole charge as overage.
	bc := &BillingContext{PricingSnapshot: []byte(`{"rate":"6"}`)}
	recordSettlementOverageSnapshot(bc, 0, 42500)
	recordSettlementOverageSnapshot(nil, 185, 42500)
	if string(bc.PricingSnapshot) != `{"rate":"6"}` {
		t.Fatalf("snapshot = %s, want unchanged", bc.PricingSnapshot)
	}
}

// TestBoundOrgFundsHoldSurvivesReinit pins that a settle or failure path which
// re-runs the attempt init from a context rebuilt with the local estimate
// keeps the hold Console booked, while an unbound entry still takes the
// caller's reservation.
func TestBoundOrgFundsHoldSurvivesReinit(t *testing.T) {
	remote, bc := seedSynchronousSettlement(t, 120)
	rebuilt := *bc
	rebuilt.ConsoleReservedCredits = nil
	if err := remote.localService.db.Transaction(func(tx *gorm.DB) error {
		return remote.localService.upsertAttemptInit(context.Background(), tx, &rebuilt)
	}); err != nil {
		t.Fatalf("re-run attempt init: %v", err)
	}
	if fund := fundEntry(t, remote, bc); fund.ReservedAmount.Int64() != 120 {
		t.Fatalf("bound fund reserved = %d, want Console's 120", fund.ReservedAmount)
	}

	if err := remote.localService.db.Model(&BillingAttemptEntry{}).
		Where("attempt_id = ? AND entry_type = ?", bc.AttemptID, billingEntryTypeFund).
		Update("idempotency_key", nil).Error; err != nil {
		t.Fatal(err)
	}
	if err := remote.localService.db.Transaction(func(tx *gorm.DB) error {
		return remote.localService.upsertAttemptInit(context.Background(), tx, &rebuilt)
	}); err != nil {
		t.Fatalf("re-run unbound attempt init: %v", err)
	}
	if fund := fundEntry(t, remote, bc); fund.ReservedAmount.Int64() != 100 {
		t.Fatalf("unbound fund reserved = %d, want the caller's 100", fund.ReservedAmount)
	}
}
