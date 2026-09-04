package repository

import (
	"strings"
	"testing"
)

func TestAvailableModelColumnsIncludePricingFields(t *testing.T) {
	requiredColumns := []string{
		"currency",
		"input_price",
		"output_price",
		"input_price_configured",
		"output_price_configured",
		"cached_input_price",
		"cost_cache_read",
		"cost_cache_write",
		"cost_cache_write_5m",
		"cost_cache_write_1h",
		"cache_read_price_configured",
		"cache_write_price_configured",
		"cache_write_5m_price_configured",
		"cache_write_1h_price_configured",
		"pricing",
	}

	for _, column := range requiredColumns {
		if !selectColumnListContains(availableModelColumns, column) {
			t.Fatalf("availableModelColumns missing %q", column)
		}
	}
}

func TestAvailableModelConfigColumnsIncludePriceOverrides(t *testing.T) {
	requiredColumns := []string{
		"input_price_override",
		"output_price_override",
		"cache_read_price_override",
		"cache_write_price_override",
		"cache_write_5m_price_override",
		"cache_write_1h_price_override",
	}

	for _, column := range requiredColumns {
		if !selectColumnListContains(availableModelConfigColumns, column) {
			t.Fatalf("availableModelConfigColumns missing %q", column)
		}
	}
}

func selectColumnListContains(columns string, target string) bool {
	for _, column := range strings.Split(columns, ",") {
		if strings.TrimSpace(column) == target {
			return true
		}
	}
	return false
}
