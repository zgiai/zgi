package gateway

import (
	"strings"

	"github.com/zgiai/zgi/api/internal/modules/llm/shared"
)

// meteredPredeductEligible reports whether a request takes the metered
// pre-deduct branch.
//
// There is deliberately no feature switch and no per-capability allowlist:
// the metered chain is unified by declaration. A capability opts in by putting
// a shared.MeteredUsageFact on the context before beginBillingAttempt, and
// every declared fact on the platform lane uses the same PreDeductMeteredUsage
// RPC. Token lanes never declare a fact, so they keep the token path.
func meteredPredeductEligible(bc *BillingContext, fact shared.MeteredUsageFact, hasFact bool) bool {
	if !hasFact || bc == nil {
		return false
	}
	if strings.TrimSpace(fact.Operation) == "" {
		return false
	}
	usageLane, err := normalizeBillingContextUsageLane(bc)
	if err != nil || usageLane != UsageBillingLanePlatform {
		return false
	}
	return true
}
