package ratingcore

import "github.com/shopspring/decimal"

// Milli-point definitions. 1 USD = 1000 points = 1,000,000 milli-points.
// milli-point is the only integer unit carried on the wire and in-process.
const (
	// Version is the ratingcore contract version. Bump it whenever the
	// registry, take-price semantics, or rounding behavior changes, and mirror
	// the new source into Pre in the same change.
	Version = "v1"

	// PointsPerUSD is the product points-per-USD anchor (current 1000).
	PointsPerUSD int64 = 1000
	// MilliPointsPerPoint is the fixed resolution of one point.
	MilliPointsPerPoint int64 = 1000
	// MilliPointsPerUSD is the exact integer scale between USD and milli-points.
	MilliPointsPerUSD int64 = PointsPerUSD * MilliPointsPerPoint

	// tokensPerMillion is the per_1m_tokens denominator.
	tokensPerMillion int64 = 1_000_000
)

var (
	decimalThousand = decimal.NewFromInt(1000)
	decimalMillion  = decimal.NewFromInt(1_000_000)
	decimalMaxInt64 = decimal.NewFromInt(9223372036854775807)
)

// USDToMilliPoints converts a canonical USD amount to milli-points, rounding UP
// to the next milli-point exactly once. Rounding down would undercharge.
func USDToMilliPoints(usd decimal.Decimal) int64 {
	return USDToMilliPointsAt(usd, PointsPerUSD)
}

// USDToMilliPointsAt converts a canonical USD amount to milli-points at an
// explicit points-per-USD rate, rounding UP exactly once. An amount that cannot
// be represented in an int64 yields 0 instead of wrapping: big.Int.Int64 is
// undefined out of range, so the range is checked on the decimal first (the
// same order Quote uses).
func USDToMilliPointsAt(usd decimal.Decimal, pointsPerUSD int64) int64 {
	scaled := usd.Mul(decimal.NewFromInt(pointsPerUSD)).Mul(decimalThousand)
	if scaled.IsNegative() {
		return 0
	}
	ceiled := scaled.Ceil()
	if ceiled.GreaterThan(decimalMaxInt64) {
		return 0
	}
	return ceiled.IntPart()
}

// MilliPointsToPoints renders milli-points as decimal points (N/1000) for
// display; the wire and the database both carry the integer milli-point value.
func MilliPointsToPoints(milliPoints int64) decimal.Decimal {
	return decimal.NewFromInt(milliPoints).Div(decimalThousand)
}

// MilliPointsToUSD converts milli-points back to canonical USD.
func MilliPointsToUSD(milliPoints int64) decimal.Decimal {
	return decimal.NewFromInt(milliPoints).Div(decimalMillion)
}
