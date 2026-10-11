// Package millipoints defines the exact points type used at the billing
// database boundary.
//
// Points are money: Pre keeps every in-process amount in milli-points
// (int64, 1 point = 1000 milli-points) so all arithmetic stays exact. The
// database stores that same integer verbatim in BIGINT columns — the storage
// unit is milli-points, matching the historical Console/Pre internal unit
// (1 USD = 1_000_000 units at 1000 points/USD) — and the gRPC/JSON wire carries
// the same int64 milli-point values, so the wire format is unchanged.
//
// Fractional points exist only for display: convert with DecimalPoints (or
// FromDecimalPoints in the other direction) at the presentation edge, never at
// the database boundary. float64 must never be used for money.
package millipoints

import (
	"database/sql/driver"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/shopspring/decimal"
)

// UnitsPerPoint is the number of milli-points in one point.
const UnitsPerPoint int64 = 1000

// MilliPoints is an exact amount of points scaled by 1000.
type MilliPoints int64

var (
	milliPointsPerPoint = decimal.NewFromInt(UnitsPerPoint)
	decimalMaxInt64     = decimal.NewFromInt(math.MaxInt64)
	decimalMinInt64     = decimal.NewFromInt(math.MinInt64)
)

// FromInt64 converts a wire/database integer in milli-points.
func FromInt64(value int64) MilliPoints { return MilliPoints(value) }

// FromPoints converts a whole number of points to milli-points.
func FromPoints(points int64) MilliPoints { return MilliPoints(points * UnitsPerPoint) }

// FromDecimalPoints converts a decimal points amount to milli-points, rounding
// to the nearest milli-point. This is a presentation-edge conversion; values
// read from or written to the database are already milli-points and must not
// pass through it.
func FromDecimalPoints(points decimal.Decimal) MilliPoints {
	scaled := points.Mul(milliPointsPerPoint).Round(0)
	if scaled.GreaterThan(decimalMaxInt64) || scaled.LessThan(decimalMinInt64) {
		return 0
	}
	return MilliPoints(scaled.IntPart())
}

// FromDecimalMilliPoints converts a decimal milli-point amount to milli-points,
// rounding to the nearest milli-point. It exists for the value shapes a JSON
// document produces (notably float64), which are already denominated in
// milli-points and must not be scaled by 1000.
func FromDecimalMilliPoints(milliPoints decimal.Decimal) MilliPoints {
	rounded := milliPoints.Round(0)
	if rounded.GreaterThan(decimalMaxInt64) || rounded.LessThan(decimalMinInt64) {
		return 0
	}
	return MilliPoints(rounded.IntPart())
}

// FromUSD converts a USD cost to milli-points at the given points-per-USD rate.
// The multiplication is exact decimal arithmetic and rounds UP to the next
// milli-point: the historical internal unit was ceil(usd x 1_000_000), and with
// points_per_usd = 1000 that is exactly ceil(usd x 1000) points, i.e. the same
// integer. Rounding down would undercharge by up to 0.000999 point.
func FromUSD(usd decimal.Decimal, pointsPerUSD int64) MilliPoints {
	scaled := usd.Mul(decimal.NewFromInt(pointsPerUSD)).Mul(milliPointsPerPoint)
	if scaled.IsNegative() {
		// Ceil rounds toward +Infinity; USD is never negative here, but guard
		// against a caller passing a negative remainder.
		return 0
	}
	if scaled.GreaterThan(decimalMaxInt64) {
		// Never wrap silently: an amount this large is a caller bug.
		return 0
	}
	return MilliPoints(scaled.Ceil().IntPart())
}

// Int64 returns the raw milli-point value used by the database and the
// gRPC/JSON wire.
func (m MilliPoints) Int64() int64 { return int64(m) }

// DecimalPoints renders the amount as fractional points (N/1000). Display only:
// the database and the wire both carry milli-points.
func (m MilliPoints) DecimalPoints() decimal.Decimal {
	return decimal.NewFromInt(int64(m)).Div(milliPointsPerPoint)
}

// IsZero reports whether the amount is zero.
func (m MilliPoints) IsZero() bool { return m == 0 }

// String renders the amount as a fixed-3-decimal points string for logs and
// error messages. It is a display helper and is deliberately NOT the database
// representation — Value persists the raw milli-point integer.
func (m MilliPoints) String() string {
	value := int64(m)
	var magnitude uint64
	if value < 0 {
		// Negate through uint64 so math.MinInt64 does not overflow.
		magnitude = uint64(-(value + 1)) + 1
	} else {
		magnitude = uint64(value)
	}
	whole := magnitude / uint64(UnitsPerPoint)
	frac := magnitude % uint64(UnitsPerPoint)
	rendered := strconv.FormatUint(whole, 10) + "." + fmt.Sprintf("%03d", frac)
	if value < 0 {
		rendered = "-" + rendered
	}
	return rendered
}

// Value implements driver.Valuer. The database unit is milli-points, so the
// exact int64 is persisted verbatim into the BIGINT column. Returning the
// fractional points form here would divide by 1000.
func (m MilliPoints) Value() (driver.Value, error) {
	return int64(m), nil
}

// Scan implements sql.Scanner for a BIGINT milli-point column. The integer
// forms a driver returns for BIGINT are taken verbatim; the decimal forms
// (string, []byte, float64) are only ever produced by a non-integer column, so
// they are interpreted as milli-points too and rounded to the nearest
// milli-point rather than being scaled by 1000. A fractional value in this
// column is a schema mismatch, which AssertMilliPointSchema rejects at start-up
// instead of letting it corrupt amounts silently.
func (m *MilliPoints) Scan(value any) error {
	if m == nil {
		return fmt.Errorf("millipoints: Scan on nil *MilliPoints")
	}
	switch v := value.(type) {
	case nil:
		*m = 0
		return nil
	case MilliPoints:
		*m = v
		return nil
	case int64:
		*m = MilliPoints(v)
		return nil
	case int32:
		*m = MilliPoints(int64(v))
		return nil
	case int:
		*m = MilliPoints(int64(v))
		return nil
	// SQL driver boundary: some drivers (for example SQLite) surface a numeric
	// column as a float. This is the only float entry point into MilliPoints;
	// the value is converted to exact decimal immediately and all arithmetic
	// afterward stays integer/decimal.
	case float64:
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return fmt.Errorf("millipoints: cannot scan %v", v)
		}
		return m.setMilliPoints(decimalFromFloatText(v))
	case float32:
		return m.setMilliPoints(decimalFromFloatText(float64(v)))
	case []byte:
		return m.scanText(string(v))
	case string:
		return m.scanText(v)
	default:
		return fmt.Errorf("millipoints: cannot scan type %T into MilliPoints", value)
	}
}

// decimalFromFloatText converts a float handed over by a database driver into an
// exact decimal through its shortest round-trip string. Money is never parsed
// through a float constructor, so the value follows the authored decimal
// instead of a binary approximation.
func decimalFromFloatText(value float64) decimal.Decimal {
	converted, err := decimal.NewFromString(strconv.FormatFloat(value, 'f', -1, 64))
	if err != nil {
		return decimal.Zero
	}
	return converted
}

func (m *MilliPoints) scanText(raw string) error {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		*m = 0
		return nil
	}
	amount, err := decimal.NewFromString(trimmed)
	if err != nil {
		return fmt.Errorf("millipoints: cannot parse %q: %w", raw, err)
	}
	return m.setMilliPoints(amount)
}

// setMilliPoints stores an exact milli-point amount, rounding a fractional input
// to the nearest milli-point (the storage resolution is one milli-point). The
// range is checked on the decimal before IntPart so an out-of-range value is
// reported instead of wrapping through big.Int.Int64.
func (m *MilliPoints) setMilliPoints(milliPoints decimal.Decimal) error {
	rounded := milliPoints.Round(0)
	if rounded.GreaterThan(decimalMaxInt64) || rounded.LessThan(decimalMinInt64) {
		return fmt.Errorf("millipoints: value %s overflows int64", milliPoints)
	}
	*m = MilliPoints(rounded.IntPart())
	return nil
}
