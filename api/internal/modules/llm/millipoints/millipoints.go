// Package millipoints defines the exact Console points type used at the
// billing database boundary.
//
// Points are money: Console keeps every in-process amount in milli-points
// (int64, 1 point = 1000 milli-points) so all arithmetic stays exact. The gRPC
// and JSON wire carry the same int64 milli-point values, so the wire format is
// unchanged. Only the PostgreSQL numeric(20,3) boundary converts to and from
// decimal points. float64 must never be used for money.
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

var milliPointsPerPoint = decimal.NewFromInt(UnitsPerPoint)

// FromInt64 converts a wire/database integer in milli-points.
func FromInt64(value int64) MilliPoints { return MilliPoints(value) }

// FromPoints converts a whole number of points to milli-points.
func FromPoints(points int64) MilliPoints { return MilliPoints(points * UnitsPerPoint) }

// FromDecimalPoints converts a decimal points amount to milli-points, rounding
// to the nearest milli-point (the storage precision).
func FromDecimalPoints(points decimal.Decimal) MilliPoints {
	return MilliPoints(points.Mul(milliPointsPerPoint).Round(0).IntPart())
}

// FromFloatPoints converts a float points amount to milli-points. Prefer
// FromDecimalPoints whenever the source value is already decimal.
func FromFloatPoints(points float64) MilliPoints {
	return FromDecimalPoints(decimal.NewFromFloat(points))
}

// FromUSD converts a USD cost to milli-points at the given points-per-USD rate.
// The multiplication is exact decimal arithmetic and rounds UP to the next
// milli-point: legacy Console used ceil(usd x 1_000_000) internal units, and
// with points_per_usd = 1000 that is exactly ceil(usd x 1000) points. Rounding
// down would undercharge by up to 0.000999 point per call.
func FromUSD(usd decimal.Decimal, pointsPerUSD int64) MilliPoints {
	scaled := usd.Mul(decimal.NewFromInt(pointsPerUSD)).Mul(milliPointsPerPoint)
	if scaled.IsNegative() {
		// Ceil rounds toward +Infinity; USD is never negative here, but guard
		// against a caller passing a negative remainder.
		return 0
	}
	return MilliPoints(scaled.Ceil().IntPart())
}

// Int64 returns the raw milli-point value used by the gRPC/JSON wire.
func (m MilliPoints) Int64() int64 { return int64(m) }

// DecimalPoints returns the value as decimal points (N/1000).
func (m MilliPoints) DecimalPoints() decimal.Decimal {
	return decimal.NewFromInt(int64(m)).Div(milliPointsPerPoint)
}

// IsZero reports whether the amount is zero.
func (m MilliPoints) IsZero() bool { return m == 0 }

// String renders the amount as a fixed-3-decimal points string, which is the
// exact representation persisted to the numeric(20,3) column.
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

// Value implements driver.Valuer. It persists points as a fixed-3-decimal
// string so numeric(20,3) receives N/1000 for N milli-points.
func (m MilliPoints) Value() (driver.Value, error) {
	return m.String(), nil
}

// Scan implements sql.Scanner. It accepts the decimal points form produced by
// a numeric(20,3) column (string, []byte or float64) as well as an integer
// points value, and returns exact milli-points.
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
		return m.setPoints(decimal.NewFromInt(v))
	case int32:
		return m.setPoints(decimal.NewFromInt(int64(v)))
	case int:
		return m.setPoints(decimal.NewFromInt(int64(v)))
	case float64:
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return fmt.Errorf("millipoints: cannot scan %v", v)
		}
		return m.setPoints(decimal.NewFromFloat(v))
	case float32:
		return m.setPoints(decimal.NewFromFloat(float64(v)))
	case []byte:
		return m.scanText(string(v))
	case string:
		return m.scanText(v)
	default:
		return fmt.Errorf("millipoints: cannot scan type %T into MilliPoints", value)
	}
}

func (m *MilliPoints) scanText(raw string) error {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		*m = 0
		return nil
	}
	points, err := decimal.NewFromString(trimmed)
	if err != nil {
		return fmt.Errorf("millipoints: cannot parse %q: %w", raw, err)
	}
	return m.setPoints(points)
}

func (m *MilliPoints) setPoints(points decimal.Decimal) error {
	scaled := points.Mul(milliPointsPerPoint).Round(0)
	if !scaled.IsInteger() {
		return fmt.Errorf("millipoints: value %s is not representable at 3 decimals", points)
	}
	value := scaled.IntPart()
	if value > math.MaxInt64 || value < math.MinInt64 {
		return fmt.Errorf("millipoints: value %s overflows int64", points)
	}
	*m = MilliPoints(value)
	return nil
}
