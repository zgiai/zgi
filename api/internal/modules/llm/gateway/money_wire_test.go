package gateway

import (
	"math"
	"testing"

	"github.com/shopspring/decimal"
)

// TestWireMoneyBoundaryConversionIsExact pins the single explicit float
// boundary on the Pre billing client. Internal amounts are decimal and must
// survive the legacy double wire round trip exactly for representable prices.
func TestWireMoneyBoundaryConversionIsExact(t *testing.T) {
	for _, text := range []string{"0", "0.005", "0.000025", "0.1", "2.5", "10", "1234.5678"} {
		want := decimal.RequireFromString(text)
		wire := decimalToWireFloat(want)
		got, err := wireFloatToDecimal(wire)
		if err != nil {
			t.Fatalf("wireFloatToDecimal(%s) error = %v", text, err)
		}
		if !got.Equal(want) {
			t.Fatalf("boundary round trip for %s = %s", text, got)
		}
	}
}

// TestWireMoneyBoundaryRejectsNonFinite keeps NaN/Inf fail-closed instead of
// letting them silently become zero money.
func TestWireMoneyBoundaryRejectsNonFinite(t *testing.T) {
	for _, value := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		if _, err := wireFloatToDecimal(value); err == nil {
			t.Fatalf("wireFloatToDecimal(%v) = nil error, want rejection", value)
		}
	}
}

// TestDeriveAuthoritativeTokenPriceUsesDecimal locks the authoritative price
// derivation to exact decimal arithmetic: 0.005 USD over 1000 tokens is
// 5 USD per 1M tokens.
func TestDeriveAuthoritativeTokenPriceUsesDecimal(t *testing.T) {
	price, resolved, err := deriveAuthoritativeTokenPrice(decimal.RequireFromString("0.005"), 1000)
	if err != nil || !resolved {
		t.Fatalf("derive(0.005, 1000) resolved=%v err=%v", resolved, err)
	}
	if !price.Equal(decimal.NewFromInt(5)) {
		t.Fatalf("derive(0.005, 1000) = %s, want 5", price)
	}

	if _, _, err := deriveAuthoritativeTokenPrice(decimal.RequireFromString("0.005"), 0); err == nil {
		t.Fatal("derive(0.005, 0) = nil error, want non-zero-cost-without-tokens error")
	}
	if _, _, err := deriveAuthoritativeTokenPrice(decimal.RequireFromString("-0.005"), 10); err == nil {
		t.Fatal("derive(-0.005, 10) = nil error, want negative-cost rejection")
	}
}
