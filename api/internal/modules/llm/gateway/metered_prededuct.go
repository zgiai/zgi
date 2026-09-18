package gateway

import (
	"context"
	"os"
	"strings"

	"github.com/zgiai/zgi/api/internal/modules/llm/shared"
)

// MeteredPredeductMode is the GATEWAY_METERED_PREDEDUCT rollout switch.
//
//	off   - never take the metered branch (default)
//	image - only images.generations / images.edits
//	all   - every whitelisted metered operation
//
// Token operations are never whitelisted, regardless of the mode.
type MeteredPredeductMode string

const (
	MeteredPredeductOff   MeteredPredeductMode = "off"
	MeteredPredeductImage MeteredPredeductMode = "image"
	MeteredPredeductAll   MeteredPredeductMode = "all"
)

const meteredPredeductEnv = "GATEWAY_METERED_PREDEDUCT"

var meteredPredeductEnvMode MeteredPredeductMode

// MeteredPredeductModeFromEnv resolves the switch once per process. An
// unknown or empty value fails closed to off.
func MeteredPredeductModeFromEnv() MeteredPredeductMode {
	if raw, ok := os.LookupEnv(meteredPredeductEnv); ok {
		return parseMeteredPredeductMode(raw)
	}
	return meteredPredeductEnvMode
}

func parseMeteredPredeductMode(raw string) MeteredPredeductMode {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case string(MeteredPredeductImage):
		return MeteredPredeductImage
	case string(MeteredPredeductAll):
		return MeteredPredeductAll
	default:
		return MeteredPredeductOff
	}
}

// meteredPredeductEnabledFor reports whether the declared metered identity may
// take the metered pre-deduct branch. It only decides the switch and the
// operation; the caller separately requires the platform lane.
func meteredPredeductEnabledFor(fact shared.MeteredUsageFact, hasFact bool) bool {
	if !hasFact {
		return false
	}
	switch MeteredPredeductModeFromEnv() {
	case MeteredPredeductAll:
		return isMeteredPredeductOperation(fact.Operation)
	case MeteredPredeductImage:
		return isImageMeteredOperation(fact.Operation)
	default:
		return false
	}
}

// meteredPredeductEnabledForRequest additionally requires the platform lane,
// because remote billing only exists there.
func meteredPredeductEnabledForRequest(_ context.Context, bc *BillingContext, fact shared.MeteredUsageFact, hasFact bool) bool {
	if !hasFact || bc == nil {
		return false
	}
	usageLane, err := normalizeBillingContextUsageLane(bc)
	if err != nil || usageLane != UsageBillingLanePlatform {
		return false
	}
	return meteredPredeductEnabledFor(fact, hasFact)
}

// isMeteredPredeductOperation is the explicit allowlist. Token operations are
// deliberately absent so the token lane can never enter the metered branch.
func isMeteredPredeductOperation(operation string) bool {
	// The metered identity carries the billing operation (image_generation),
	// not the routing endpoint (images.generations).
	switch strings.TrimSpace(operation) {
	case string(PricingOperationImage):
		return true
	default:
		return false
	}
}

func isImageMeteredOperation(operation string) bool {
	switch strings.TrimSpace(operation) {
	case string(PricingOperationImage):
		return true
	default:
		return false
	}
}
