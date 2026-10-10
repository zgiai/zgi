package handler

import (
	"errors"
	"fmt"
	"testing"

	llmerrors "github.com/zgiai/zgi/api/internal/modules/llm/errors"
	"github.com/zgiai/zgi/api/internal/modules/llm/gateway"
)

// T11 acceptance 2: a fail-closed exchange-rate error surfaces as the stable
// billing.exchange_rate.unavailable identity rather than a generic billing
// failure.
func TestT11ApplicationCodeMapsExchangeRateUnavailable(t *testing.T) {
	err := fmt.Errorf("capture billing currency snapshot: %w", gateway.ErrExchangeRateUnavailable)
	code, ok := applicationCode(err)
	if !ok {
		t.Fatalf("applicationCode(%v) was not classified", err)
	}
	if code.String() != llmerrors.AppCodeBillingExchangeRateUnavailable.String() {
		t.Fatalf("code = %s, want %s", code, llmerrors.AppCodeBillingExchangeRateUnavailable)
	}
	if !errors.Is(err, gateway.ErrExchangeRateUnavailable) {
		t.Fatalf("wrapped error lost the exchange-rate sentinel: %v", err)
	}
}
