package gateway

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestQueryConsoleSettlementUsesAttemptAndOrganization(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/settlements/attempt-1" || r.URL.Query().Get("organization_id") != "org-1" {
			t.Fatalf("unexpected query: %s", r.URL.String())
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"found":true,"settlement_id":"s-1","deduction_id":"s-1","attempt_id":"attempt-1","organization_id":"org-1","settlement_status":"SETTLED","reserved_credits":20,"settled_credits":12,"refunded_credits":8,"actual_quantity":3,"pricing_snapshot":{"rule_id":"rule-1"},"request_id":"req-1"}`))
	}))
	defer server.Close()
	got, err := queryConsoleSettlement(context.Background(), server.URL+"/settlements", "org-1", "attempt-1")
	if err != nil {
		t.Fatal(err)
	}
	if !got.Found || got.SettlementID != "s-1" || got.DeductionID != "s-1" || got.AttemptID != "attempt-1" || got.SettledCredits != 12 || got.RefundedCredits != 8 || string(got.PricingSnapshot) != `{"rule_id":"rule-1"}` {
		t.Fatalf("unexpected response: %+v", got)
	}
}

func TestWaitForConsoleSettlementReturnsFailedFinalImmediately(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"found":true,"settlement_id":"s-1","deduction_id":"s-1","attempt_id":"attempt-1","organization_id":"org-1","settlement_status":"FAILED_FINAL","retryable":false}`))
	}))
	defer server.Close()

	got, err := waitForConsoleSettlement(context.Background(), server.URL+"/settlements", "org-1", "attempt-1")
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("FAILED_FINAL queries = %d, want 1", calls)
	}
	if !got.Found || got.SettlementStatus != "FAILED_FINAL" || got.Retryable {
		t.Fatalf("response = %+v", got)
	}
}
