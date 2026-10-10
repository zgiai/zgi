package gateway

import "testing"

// Platform-lane video settles from Console authority, so the local quote is
// intentionally zero and must not trigger the private-lane fail-closed path.
func TestVideoSettlementRequiresLocalPriceByLane(t *testing.T) {
	if videoSettlementRequiresLocalPrice(UsageBillingLanePlatform) {
		t.Fatal("platform lane must not require a local model price")
	}
	if !videoSettlementRequiresLocalPrice(UsageBillingLanePrivate) {
		t.Fatal("private lane must require a local model price")
	}
}
