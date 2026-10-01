package millipoints

import (
	"database/sql/driver"
	"math"
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
	// String is the display renderer only; the database value is the raw int64.
	if got.String() != "5.000" {
		t.Fatalf("FromUSD(0.005) display string = %q, want %q", got.String(), "5.000")
	}
	if v, err := got.Value(); err != nil || v.(int64) != 5000 {
		t.Fatalf("FromUSD(0.005) Value() = %v (%v), want int64 5000", v, err)
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

// TestValuePersistsRawMilliPoints locks the database boundary: the column unit
// is milli-points, so Value must hand the exact integer to the BIGINT column.
// Rendering fractional points here would divide every amount by 1000.
func TestValuePersistsRawMilliPoints(t *testing.T) {
	for _, mp := range []int64{0, 1, 5, 1000, 5000, 1_000_000, -1500, math.MaxInt64, math.MinInt64} {
		got, err := MilliPoints(mp).Value()
		if err != nil {
			t.Fatalf("Value(%d) error = %v", mp, err)
		}
		asInt, ok := got.(int64)
		if !ok {
			t.Fatalf("Value(%d) returned %T, want int64", mp, got)
		}
		if asInt != mp {
			t.Fatalf("Value(%d) = %d, want it verbatim", mp, asInt)
		}
	}
}

// TestScanTakesIntegersVerbatim locks the read boundary for the integer forms a
// driver returns for BIGINT.
func TestScanTakesIntegersVerbatim(t *testing.T) {
	cases := []struct {
		name string
		in   any
		want int64
	}{
		{"int64", int64(5), 5},
		{"int", int(5), 5},
		{"int32", int32(5), 5},
		{"int64-large", int64(1_000_000), 1_000_000},
	}
	for _, tc := range cases {
		var m MilliPoints
		if err := m.Scan(tc.in); err != nil {
			t.Fatalf("%s: Scan(%v) error = %v", tc.name, tc.in, err)
		}
		if m.Int64() != tc.want {
			t.Fatalf("%s: Scan(%v) = %d, want %d verbatim (1000x regression)", tc.name, tc.in, m.Int64(), tc.want)
		}
	}

	var m MilliPoints
	if err := m.Scan(int64(5)); err != nil {
		t.Fatalf("Scan(int64 5) error = %v", err)
	}
	if m.String() != "0.005" {
		t.Fatalf("Scan(int64 5) display = %q, want 0.005", m.String())
	}
}

// TestScanDecimalFormsAreMilliPoints documents the only ambiguous input shape.
// Both "5.000 points" and "5.000 milli-points" render identically, so no runtime
// rule can tell them apart: the decimal forms are read as milli-points, and a
// leftover fractional-points column is caught by AssertMilliPointSchema at
// start-up instead of being guessed at here.
func TestScanDecimalFormsAreMilliPoints(t *testing.T) {
	cases := []struct {
		name string
		in   any
		want int64
	}{
		{"decimal-string", "5.000", 5},
		{"whole-string", "5000", 5000},
		{"bytes", []byte("5000"), 5000},
		{"float-integral", float64(5000), 5000},
		{"float-fraction", float64(5.6), 6},
		{"empty-string", "", 0},
	}
	for _, tc := range cases {
		var m MilliPoints
		if err := m.Scan(tc.in); err != nil {
			t.Fatalf("%s: Scan(%v) error = %v", tc.name, tc.in, err)
		}
		if m.Int64() != tc.want {
			t.Fatalf("%s: Scan(%v) = %d, want %d", tc.name, tc.in, m.Int64(), tc.want)
		}
	}
}

func TestScanRejectsInvalidShapes(t *testing.T) {
	var m MilliPoints
	if err := m.Scan("not-a-number"); err == nil {
		t.Fatal("Scan of a non-numeric string must fail")
	}
	if err := m.Scan(math.Inf(1)); err == nil {
		t.Fatal("Scan of +Inf must fail")
	}
	if err := m.Scan(math.NaN()); err == nil {
		t.Fatal("Scan of NaN must fail")
	}
	if err := m.Scan(struct{}{}); err == nil {
		t.Fatal("Scan of an unsupported type must fail")
	}
	if err := m.Scan("9223372036854775808"); err == nil {
		t.Fatal("Scan of an out-of-range value must fail rather than wrap")
	}
	var nilTarget *MilliPoints
	if err := nilTarget.Scan(int64(1)); err == nil {
		t.Fatal("Scan on a nil receiver must fail")
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

func TestFromUSDRejectsOverflow(t *testing.T) {
	if got := FromUSD(decimal.RequireFromString("-1"), 1000); got != 0 {
		t.Fatalf("FromUSD(negative) = %d, want 0", got)
	}
	if got := FromUSD(decimal.RequireFromString("1e30"), 1000); got != 0 {
		t.Fatalf("FromUSD(overflow) = %d, want 0 rather than a wrapped value", got)
	}
}

func TestValueIsADriverValuer(t *testing.T) {
	var _ driver.Valuer = MilliPoints(0)
}
