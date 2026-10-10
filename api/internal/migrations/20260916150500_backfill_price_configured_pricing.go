package migrations

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
	modelpricing "github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/pricing"
	"gorm.io/gorm"
)

const migrationBackfillPriceConfiguredPricingID = "20260916150500_backfill_price_configured_pricing"

// backfillPriceConfiguredPricing runs before the *_price_configured columns are
// dropped: the derived configured flags read only pricing JSON, so legacy rows
// must get their explicitly configured scalar prices projected into rules[]
// first. Without this, existing custom models would silently become
// "unconfigured" and fail closed after the column drop.
func init() {
	registerSchemaMigration(migrationBackfillPriceConfiguredPricingID, upBackfillPriceConfiguredPricing, nil)
}

func upBackfillPriceConfiguredPricing(schema *mschema.Builder) error {
	for _, target := range []struct {
		table         string
		catalogOrigin string
	}{
		{table: "llm_custom_models", catalogOrigin: "pre_custom"},
		{table: "llm_models", catalogOrigin: ""},
	} {
		table := target.table
		catalogOrigin := target.catalogOrigin
		if err := schema.DataFix("backfill "+table+".pricing from explicitly configured scalar prices", func(db *gorm.DB) error {
			return backfillPriceConfiguredPricing(db, table, catalogOrigin)
		}); err != nil {
			return err
		}
	}
	return nil
}

type priceConfiguredBackfillRow struct {
	ID                     string   `gorm:"column:id"`
	Currency               *string  `gorm:"column:currency"`
	Pricing                []byte   `gorm:"column:pricing"`
	InputPrice             *float64 `gorm:"column:input_price"`
	OutputPrice            *float64 `gorm:"column:output_price"`
	CacheReadPrice         *float64 `gorm:"column:cache_read_price"`
	CacheWritePrice        *float64 `gorm:"column:cache_write_price"`
	CacheWrite5mPrice      *float64 `gorm:"column:cache_write_5m_price"`
	CacheWrite1hPrice      *float64 `gorm:"column:cache_write_1h_price"`
	InputConfigured        bool     `gorm:"column:input_price_configured"`
	OutputConfigured       bool     `gorm:"column:output_price_configured"`
	CacheReadConfigured    bool     `gorm:"column:cache_read_price_configured"`
	CacheWriteConfigured   bool     `gorm:"column:cache_write_price_configured"`
	CacheWrite5mConfigured bool     `gorm:"column:cache_write_5m_price_configured"`
	CacheWrite1hConfigured bool     `gorm:"column:cache_write_1h_price_configured"`
}

func backfillPriceConfiguredPricing(db *gorm.DB, table, catalogOrigin string) error {
	if !db.Migrator().HasTable(table) || !db.Migrator().HasColumn(table, "pricing") {
		return nil
	}
	columns := []string{"id AS id"}
	appendColumn := func(column, fallback string) {
		if db.Migrator().HasColumn(table, column) {
			columns = append(columns, column+" AS "+column)
			return
		}
		columns = append(columns, fallback+" AS "+column)
	}
	appendAlias := func(source, alias string) {
		if db.Migrator().HasColumn(table, source) {
			columns = append(columns, source+" AS "+alias)
			return
		}
		columns = append(columns, "NULL AS "+alias)
	}
	appendColumn("currency", "NULL")
	appendColumn("pricing", "NULL")
	appendColumn("input_price", "NULL")
	appendColumn("output_price", "NULL")
	appendAlias("cost_cache_read", "cache_read_price")
	appendAlias("cost_cache_write", "cache_write_price")
	appendAlias("cost_cache_write_5m", "cache_write_5m_price")
	appendAlias("cost_cache_write_1h", "cache_write_1h_price")
	for _, column := range []string{
		"input_price_configured", "output_price_configured", "cache_read_price_configured",
		"cache_write_price_configured", "cache_write_5m_price_configured", "cache_write_1h_price_configured",
	} {
		appendColumn(column, "false")
	}

	var rows []priceConfiguredBackfillRow
	query := db.Table(table).Select(strings.Join(columns, ", "))
	if db.Migrator().HasColumn(table, "deleted_at") {
		query = query.Where("deleted_at IS NULL")
	}
	if err := query.Find(&rows).Error; err != nil {
		return fmt.Errorf("load %s pricing rows: %w", table, err)
	}

	updated := 0
	for _, row := range rows {
		next, ok, err := buildPriceConfiguredBackfillPricing(row, catalogOrigin)
		if err != nil {
			return fmt.Errorf("synthesize %s pricing for %s: %w", table, row.ID, err)
		}
		if !ok {
			continue
		}
		if err := db.Table(table).Where("id = ?", row.ID).Update("pricing", string(next)).Error; err != nil {
			return fmt.Errorf("persist %s pricing for %s: %w", table, row.ID, err)
		}
		updated++
	}
	if updated > 0 {
		fmt.Printf("pricing configured backfill: table=%s updated=%d\n", table, updated)
	}
	return nil
}

// buildPriceConfiguredBackfillPricing projects explicitly configured scalar
// prices into rules[] and legacy projections. It returns ok=false when the row
// has nothing to backfill or already carries every configured meter.
func buildPriceConfiguredBackfillPricing(row priceConfiguredBackfillRow, catalogOrigin string) ([]byte, bool, error) {
	overrides := modelpricing.LegacyScalarPrices{}
	if row.InputConfigured && row.InputPrice != nil {
		overrides.InputPrice = row.InputPrice
	}
	if row.OutputConfigured && row.OutputPrice != nil {
		overrides.OutputPrice = row.OutputPrice
	}
	if row.CacheReadConfigured && row.CacheReadPrice != nil {
		overrides.CacheReadPrice = row.CacheReadPrice
	}
	if row.CacheWriteConfigured && row.CacheWritePrice != nil {
		overrides.CacheWritePrice = row.CacheWritePrice
	}
	if row.CacheWrite5mConfigured && row.CacheWrite5mPrice != nil {
		overrides.CacheWrite5mPrice = row.CacheWrite5mPrice
	}
	if row.CacheWrite1hConfigured && row.CacheWrite1hPrice != nil {
		overrides.CacheWrite1hPrice = row.CacheWrite1hPrice
	}
	if !hasConfiguredScalarOverride(overrides) {
		return nil, false, nil
	}
	if len(bytes.TrimSpace(row.Pricing)) == 0 || bytes.Equal(bytes.TrimSpace(row.Pricing), []byte("null")) {
		row.Pricing = []byte(`{"deployment_scope":"global"}`)
	}
	if pricingHasAllConfiguredMeters(row.Pricing, overrides) {
		return nil, false, nil
	}

	currency := ""
	if row.Currency != nil {
		currency = strings.TrimSpace(*row.Currency)
	}
	prepared := modelpricing.PrepareStructuredPricingForWrite(
		row.Pricing,
		currency,
		modelpricing.LegacyScalarPrices{},
		modelpricing.PricingWriteOptions{UserAuthored: true, ScalarOverrides: &overrides},
	)
	if len(bytes.TrimSpace(prepared)) == 0 || bytes.Equal(bytes.TrimSpace(prepared), []byte("null")) {
		return nil, false, nil
	}
	if catalogOrigin == "" {
		return prepared, true, nil
	}
	enriched, err := withBackfillCatalogOrigin(prepared, catalogOrigin)
	if err != nil {
		return nil, false, err
	}
	return enriched, true, nil
}

func hasConfiguredScalarOverride(scalars modelpricing.LegacyScalarPrices) bool {
	return scalars.InputPrice != nil || scalars.OutputPrice != nil ||
		scalars.CacheReadPrice != nil || scalars.CacheWritePrice != nil ||
		scalars.CacheWrite5mPrice != nil || scalars.CacheWrite1hPrice != nil
}

func pricingHasAllConfiguredMeters(raw []byte, overrides modelpricing.LegacyScalarPrices) bool {
	meters := map[string]bool{}
	if overrides.InputPrice != nil {
		meters["input_token"] = true
	}
	if overrides.OutputPrice != nil {
		meters["output_token"] = true
	}
	if overrides.CacheReadPrice != nil {
		meters["cache_read_token"] = true
	}
	if overrides.CacheWritePrice != nil {
		meters["cache_write_token"] = true
	}
	if overrides.CacheWrite5mPrice != nil {
		meters["cache_write_5m_token"] = true
	}
	if overrides.CacheWrite1hPrice != nil {
		meters["cache_write_1h_token"] = true
	}
	present := map[string]bool{}
	var envelope struct {
		Rules []struct {
			Meter string `json:"meter"`
		} `json:"rules"`
	}
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) > 0 && !bytes.Equal(trimmed, []byte("null")) {
		if err := json.Unmarshal(trimmed, &envelope); err == nil {
			for _, rule := range envelope.Rules {
				present[strings.TrimSpace(rule.Meter)] = true
			}
		}
	}
	for meter := range meters {
		if !present[meter] {
			return false
		}
	}
	return true
}

func withBackfillCatalogOrigin(raw []byte, catalogOrigin string) ([]byte, error) {
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return nil, err
	}
	encodedOrigin, err := json.Marshal(catalogOrigin)
	if err != nil {
		return nil, err
	}
	envelope["catalog_origin"] = encodedOrigin
	if _, ok := envelope["deployment_scope"]; !ok {
		encodedScope, err := json.Marshal("organization")
		if err != nil {
			return nil, err
		}
		envelope["deployment_scope"] = encodedScope
	}
	return json.Marshal(envelope)
}
