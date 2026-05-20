package utils

import (
	"github.com/shopspring/decimal"
)

// maxDecimal104 defines the maximum value for decimal(10,4): 999999.9999
var maxDecimal104 = decimal.NewFromFloat(999999.9999)

// minDecimal104 defines the minimum value for decimal(10,4): -999999.9999
var minDecimal104 = decimal.NewFromFloat(-999999.9999)

// NormalizeDecimal104 normalizes the decimal value within the decimal(10,4) range
// If the value exceeds the range, it will be truncated to the maximum/minimum value
// If the value is negative, returns zero
func NormalizeDecimal104(value decimal.Decimal) decimal.Decimal {
	// Negative price is considered invalid, returns zero
	if value.IsNegative() {
		return decimal.Zero
	}

	// When exceeding the maximum value, truncate to the maximum value
	if value.GreaterThan(maxDecimal104) {
		return maxDecimal104
	}

	// Keep 4 decimal places
	return value.Round(4)
}
