package gateway

import "testing"

func TestAdvanceSettlementStateDoesNotDefaultMissingToSuccess(t *testing.T) {
	if err := advanceSettlementState("", billingAttemptStatusSettled); err == nil {
		t.Fatal("unknown state must not become settled")
	}
	if err := advanceSettlementState(billingAttemptStatusSettlePending, billingAttemptStatusSettled); err != nil {
		t.Fatalf("settle pending should allow idempotent authoritative settle: %v", err)
	}
}
