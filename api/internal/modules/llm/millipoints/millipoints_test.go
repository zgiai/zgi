package millipoints

import (
	"testing"

	"github.com/shopspring/decimal"
)

// TestFromUSDHalfCentIsExact locks the canonical example from the billing
// unification spec: 0.005 USD = 5 points = 5000 milli-points.
func TestFromUSDHalfCentIsExact(t *testing.T) {
	usd := decimal.RequireFromString("0.005")
	got := FromUSD(usd, 1000)

	if got.Int64() != 5000 {
		t.Fatalf("FromUSD(0.005) = %d milli-points, want 5000", got.Int64())
	}
	if !got.DecimalPoints().Equal(decimal.NewFromInt(5)) {
		t.Fatalf("FromUSD(0.005) points = %s, want 5", got.DecimalPoints())
	}
	if got.String() != "5.000" {
		t.Fatalf("FromUSD(0.005) storage string = %q, want %q", got.String(), "5.000")
	}
}

func TestFromUSDOneDollarIsOneMillionMilliPoints(t *testing.T) {
	got := FromUSD(decimal.NewFromInt(1), 1000)
	if got.Int64() != 1_000_000 {
		t.Fatalf("FromUSD(1) = %d milli-points, want 1000000", got.Int64())
	}
	if !got.DecimalPoints().Equal(decimal.NewFromInt(1000)) {
		t.Fatalf("FromUSD(1) points = %s, want 1000", got.DecimalPoints())
	}
}

func TestFromUSDDoesNotUseFloat(t *testing.T) {
	// 0.1 + 0.2 is the classic binary-float hazard; decimal arithmetic keeps
	// the sum exact and the conversion deterministic.
	sum := decimal.RequireFromString("0.1").Add(decimal.RequireFromString("0.2"))
	got := FromUSD(sum, 1000)
	if got.Int64() != 300_000 {
		t.Fatalf("FromUSD(0.1+0.2) = %d milli-points, want 300000", got.Int64())
	}
}

func TestScanDecimalPointsStringIsExact(t *testing.T) {
	var value MilliPoints
	if err := value.Scan("5.000"); err != nil {
		t.Fatalf("Scan(5.000) error = %v", err)
	}
	if value.Int64() != 5000 {
		t.Fatalf("Scan(5.000) = %d milli-points, want 5000", value.Int64())
	}
	if value.String() != "5.000" {
		t.Fatalf("Scan(5.000) storage string = %q, want %q", value.String(), "5.000")
	}
}

func TestFromDecimalPointsRoundsToNearestMilliPoint(t *testing.T) {
	if got := FromDecimalPoints(decimal.RequireFromString("0.0004")); got.Int64() != 0 {
		t.Fatalf("FromDecimalPoints(0.0004) = %d, want 0", got.Int64())
	}
	if got := FromDecimalPoints(decimal.RequireFromString("0.0005")); got.Int64() != 1 {
		t.Fatalf("FromDecimalPoints(0.0005) = %d, want 1", got.Int64())
	}
}
