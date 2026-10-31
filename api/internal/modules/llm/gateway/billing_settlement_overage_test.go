package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

// overageSettlementBody is a Console settlement that charged more than the
// 100-credit hold seedStaleRemotePrededuct books (charge_difference).
func overageSettlementBody(settled, refunded int64) func(bc *BillingContext) string {
	return func(bc *BillingContext) string {
		return fmt.Sprintf(`{"found":true,"settlement_id":%q,"deduction_id":%q,"attempt_id":%q,"organization_id":%q,"request_id":%q,"settlement_status":"SETTLED","reserved_credits":100,"settled_credits":%d,"refunded_credits":%d,"retryable":false,"pricing_snapshot":{"currency":"USD","cny_per_usd":"7","points_per_usd":"1000"}}`,
			bc.DeductionID, bc.DeductionID, bc.AttemptID, bc.OrganizationID, bc.RequestID, settled, refunded)
	}
}

func TestConsoleSettlementAmountsValid(t *testing.T) {
	for _, tc := range []struct {
		name                       string
		reserved, settled, refunds int64
		want                       bool
	}{
		{"within reserve with refund", 100, 60, 40, true},
		{"within reserve partial refund", 100, 60, 10, true},
		{"overage without refund", 100, 150, 0, true},
		{"unreported reserve", 0, 150, 0, true},
		{"negative settled", 100, -1, 0, false},
		{"negative refunded", 100, 60, -1, false},
		{"within reserve over-refunded", 100, 60, 41, false},
		{"overage with refund", 100, 150, 1, false},
	} {
		if got := consoleSettlementAmountsValid(tc.reserved, tc.settled, tc.refunds); got != tc.want {
			t.Errorf("%s: consoleSettlementAmountsValid(%d, %d, %d) = %t, want %t", tc.name, tc.reserved, tc.settled, tc.refunds, got, tc.want)
		}
	}
}

func assertOverageMirrored(t *testing.T, remote *RemoteBilling, bc *BillingContext, settled, overage int64) {
	t.Helper()
	db := remote.localService.db
	var attempt BillingAttempt
	if err := db.First(&attempt, "attempt_id = ?", bc.AttemptID).Error; err != nil {
		t.Fatal(err)
	}
	if attempt.Status != billingAttemptStatusSettled {
		t.Fatalf("attempt status = %s, want %s", attempt.Status, billingAttemptStatusSettled)
	}
	var fund BillingAttemptEntry
	if err := db.Where("attempt_id = ? AND entry_type = ? AND ledger_type = ?", bc.AttemptID, billingEntryTypeFund, billingLedgerTypeOrgFunds).First(&fund).Error; err != nil {
		t.Fatal(err)
	}
	if fund.ReservedAmount != 100 || fund.ActualAmount.Int64() != settled || fund.RefundedAmount != 0 || fund.Status != billingEntryStatusSettled {
		t.Fatalf("fund entry reserved=%d actual=%d refunded=%d status=%s, want 100/%d/0/%s", fund.ReservedAmount, fund.ActualAmount, fund.RefundedAmount, fund.Status, settled, billingEntryStatusSettled)
	}
	var bill UsageBill
	if err := db.Where("attempt_id = ?", bc.AttemptID).First(&bill).Error; err != nil {
		t.Fatalf("usage bill missing: %v", err)
	}
	if bill.OfficialPoints.Int64() != settled {
		t.Fatalf("usage bill official points = %d, want %d", bill.OfficialPoints, settled)
	}
	var snapshot map[string]interface{}
	if err := json.Unmarshal(bill.PricingSnapshot, &snapshot); err != nil {
		t.Fatal(err)
	}
	if snapshot["overage_credits"] != float64(overage) || snapshot["overage_strategy"] != settlementOverageStrategyChargeDifference {
		t.Fatalf("snapshot overage = %v/%v, want %d/%s", snapshot["overage_credits"], snapshot["overage_strategy"], overage, settlementOverageStrategyChargeDifference)
	}
}

// TestFinalizePlatformProxySettlementMirrorsConsoleOverage pins ZGI-01: Console
// charges the difference when the actual exceeds the hold, and Pre must mirror
// that settlement instead of rejecting it.
func TestFinalizePlatformProxySettlementMirrorsConsoleOverage(t *testing.T) {
	db := openRemoteBillingTestDB(t)
	var bc *BillingContext
	server := consoleSettlementServer(t, overageSettlementBody(150, 0), &bc)
	client := &countingSettleQuotaClient{}
	remote, seeded := seedStaleRemotePrededuct(t, db, client, server.URL)
	bc = seeded
	bc.Status = "success"

	if err := remote.FinalizePlatformProxySettlement(context.Background(), bc, nil); err != nil {
		t.Fatalf("finalize overage settlement: %v", err)
	}
	if client.settleCalls != 0 {
		t.Fatalf("settle calls = %d, want 0: Pre only mirrors Console's charge", client.settleCalls)
	}
	assertOverageMirrored(t, remote, bc, 150, 50)
}

func TestFinalizePlatformProxySettlementRejectsInvalidConsoleAmounts(t *testing.T) {
	for _, tc := range []struct {
		name              string
		settled, refunded int64
	}{
		{"negative settled", -1, 0},
		{"negative refunded", 60, -1},
		{"within reserve over-refunded", 60, 41},
		{"overage with refund", 150, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := openRemoteBillingTestDB(t)
			var bc *BillingContext
			server := consoleSettlementServer(t, overageSettlementBody(tc.settled, tc.refunded), &bc)
			client := &countingSettleQuotaClient{}
			remote, seeded := seedStaleRemotePrededuct(t, db, client, server.URL)
			bc = seeded
			bc.Status = "success"

			err := remote.FinalizePlatformProxySettlement(context.Background(), bc, nil)
			if err == nil || !strings.Contains(err.Error(), "invalid console settlement amounts") {
				t.Fatalf("finalize error = %v, want invalid console settlement amounts", err)
			}
			var attempt BillingAttempt
			if err := db.First(&attempt, "attempt_id = ?", bc.AttemptID).Error; err != nil {
				t.Fatal(err)
			}
			if attempt.Status != billingAttemptStatusPre {
				t.Fatalf("attempt status = %s, want %s", attempt.Status, billingAttemptStatusPre)
			}
		})
	}
}

func TestMirrorQueriedSettlementAcceptsOverage(t *testing.T) {
	db := openRemoteBillingTestDB(t)
	client := &countingSettleQuotaClient{}
	remote, bc := seedStaleRemotePrededuct(t, db, client, "")
	var attempt BillingAttempt
	if err := db.First(&attempt, "attempt_id = ?", bc.AttemptID).Error; err != nil {
		t.Fatal(err)
	}
	var result settlementQueryResponse
	if err := json.Unmarshal([]byte(overageSettlementBody(150, 0)(bc)), &result); err != nil {
		t.Fatal(err)
	}

	if err := remote.mirrorQueriedSettlement(context.Background(), &attempt, &result); err != nil {
		t.Fatalf("mirror overage settlement: %v", err)
	}
	assertOverageMirrored(t, remote, bc, 150, 50)
}

func TestMirrorQueriedSettlementRejectsOverageWithRefund(t *testing.T) {
	db := openRemoteBillingTestDB(t)
	client := &countingSettleQuotaClient{}
	remote, bc := seedStaleRemotePrededuct(t, db, client, "")
	var attempt BillingAttempt
	if err := db.First(&attempt, "attempt_id = ?", bc.AttemptID).Error; err != nil {
		t.Fatal(err)
	}
	var result settlementQueryResponse
	if err := json.Unmarshal([]byte(overageSettlementBody(150, 1)(bc)), &result); err != nil {
		t.Fatal(err)
	}

	err := remote.mirrorQueriedSettlement(context.Background(), &attempt, &result)
	if err == nil || !strings.Contains(err.Error(), "settlement amounts exceed reserved credits") {
		t.Fatalf("mirror error = %v, want settlement amounts exceed reserved credits", err)
	}
}

// TestStalePredeductRecoveryMirrorsConsoleOverage reproduces the 240 backlog:
// a stale PREDEDUCTED hold that Console settled above the reservation closes in
// a single sweep.
func TestStalePredeductRecoveryMirrorsConsoleOverage(t *testing.T) {
	db := openRemoteBillingTestDB(t)
	var bc *BillingContext
	server := consoleSettlementServer(t, overageSettlementBody(150, 0), &bc)
	client := &countingSettleQuotaClient{}
	remote, seeded := seedStaleRemotePrededuct(t, db, client, server.URL)
	bc = seeded

	if err := remote.recoverStalePredeductedRemoteAttempts(context.Background()); err != nil {
		t.Fatal(err)
	}
	if client.settleCalls != 0 {
		t.Fatalf("settle calls = %d, want 0: a Console-settled hold must not be re-settled", client.settleCalls)
	}
	assertOverageMirrored(t, remote, bc, 150, 50)
}
