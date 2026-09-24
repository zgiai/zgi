package model

import (
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// ModelConfig is written through GORM's struct mapping, so every price override
// field must map to the exact column created by the migrations.
func TestModelConfigPriceOverrideColumns(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}

	stmt := &gorm.Statement{DB: db}
	if err := stmt.Parse(&ModelConfig{}); err != nil {
		t.Fatalf("parse model config schema: %v", err)
	}

	expected := map[string]string{
		"InputPriceOverride":        "input_price_override",
		"OutputPriceOverride":       "output_price_override",
		"CacheReadPriceOverride":    "cache_read_price_override",
		"CacheWritePriceOverride":   "cache_write_price_override",
		"CacheWrite5mPriceOverride": "cache_write_5m_price_override",
		"CacheWrite1hPriceOverride": "cache_write_1h_price_override",
	}

	for fieldName, columnName := range expected {
		field := stmt.Schema.LookUpField(fieldName)
		if field == nil {
			t.Fatalf("ModelConfig field %s is missing", fieldName)
		}
		if field.DBName != columnName {
			t.Fatalf("ModelConfig field %s maps to column %q, want %q", fieldName, field.DBName, columnName)
		}
	}
}
