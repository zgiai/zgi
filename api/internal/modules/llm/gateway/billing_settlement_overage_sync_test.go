package gateway

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/modules/llm/shared"
)

// seedSynchronousSettlement books the 100-credit hold seedStaleRemotePrededuct
// uses, behind a Console that fails the test if queried: the synchronous path
// must mirror the settlement that rode back on the proxied response.
func seedSynchronousSettlement(t *testing.T) (*RemoteBilling, *BillingContext) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected Console settlement query: %s", r.URL.Path)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(server.Close)
	remote, bc := seedStaleRemotePrededuct(t, openRemoteBillingTestDB(t), &countingSettleQuotaClient{}, server.URL)
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
	remote, bc := seedSynchronousSettlement(t)
	if err := remote.FinalizePlatformProxySettlement(context.Background(), bc, syncSettlement(bc, 150)); err != nil {
		t.Fatalf("finalize synchronous overage settlement: %v", err)
	}
	assertOverageMirrored(t, remote, bc, 150, 50)
}

func TestFinalizePlatformProxySettlementSyncWithinReserveOmitsOverage(t *testing.T) {
	for _, official := range []int64{60, 100} {
		remote, bc := seedSynchronousSettlement(t)
		if err := remote.FinalizePlatformProxySettlement(context.Background(), bc, syncSettlement(bc, official)); err != nil {
			t.Fatalf("finalize synchronous settlement %d: %v", official, err)
		}
		assertNoOverageKeys(t, remote, bc, official)
	}
}

// TestFinalizePlatformProxySettlementSyncMeteredOmitsOverage pins that the
// local estimate never stands in for a metered hold Console priced itself.
func TestFinalizePlatformProxySettlementSyncMeteredOmitsOverage(t *testing.T) {
	for _, tc := range []struct {
		name      string
		operation PricingOperation
		ctx       context.Context
	}{
		{"image operation", PricingOperationImage, context.Background()},
		{"video operation", PricingOperationVideo, context.Background()},
		{"declared metered fact", PricingOperationChat, context.WithValue(context.Background(), shared.ContextKeyMeteredUsage, &shared.MeteredUsageFact{Operation: "image_generation", Quantity: 1})},
	} {
		t.Run(tc.name, func(t *testing.T) {
			remote, bc := seedSynchronousSettlement(t)
			bc.PricingOperation = tc.operation
			if err := remote.FinalizePlatformProxySettlement(tc.ctx, bc, syncSettlement(bc, 150)); err != nil {
				t.Fatalf("finalize metered settlement: %v", err)
			}
			assertNoOverageKeys(t, remote, bc, 150)
		})
	}
}

func TestSynchronousSettlementReservedCredits(t *testing.T) {
	meteredCtx := context.WithValue(context.Background(), shared.ContextKeyMeteredUsage, &shared.MeteredUsageFact{Operation: "x"})
	for _, tc := range []struct {
		name string
		ctx  context.Context
		bc   *BillingContext
		want int64
	}{
		{"nil context", context.Background(), nil, 0},
		{"token estimate", context.Background(), &BillingContext{PricingOperation: PricingOperationChat, EstimatedCredits: 185}, 185},
		{"zero estimate", context.Background(), &BillingContext{EstimatedCredits: 0}, 0},
		{"tool postpaid", context.Background(), &BillingContext{ToolPostpaid: true, EstimatedCredits: 185}, 0},
		{"image", context.Background(), &BillingContext{PricingOperation: PricingOperationImage, EstimatedCredits: 185}, 0},
		{"video", context.Background(), &BillingContext{PricingOperation: PricingOperationVideo, EstimatedCredits: 185}, 0},
		{"metered fact", meteredCtx, &BillingContext{PricingOperation: PricingOperationChat, EstimatedCredits: 185}, 0},
	} {
		if got := synchronousSettlementReservedCredits(tc.ctx, tc.bc); got != tc.want {
			t.Errorf("%s: reserved = %d, want %d", tc.name, got, tc.want)
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
