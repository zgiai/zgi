package migrations

import (
	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
)

const migrationAddLLMPricingFallbackID = "202606230900000000_add_llm_pricing_fallback"

func init() {
	registerSchemaMigration(
		migrationAddLLMPricingFallbackID,
		upAddLLMPricingFallback,
		downAddLLMPricingFallback,
	)
}

func upAddLLMPricingFallback(schema *mschema.Builder) error {
	for _, tableName := range []string{"llm_models", "llm_custom_models"} {
		if err := schema.WhenTableDoesntHaveColumn(tableName, "input_price_configured", func() error {
			return schema.Table(tableName, func(table *mschema.Blueprint) {
				table.Boolean("input_price_configured").DefaultSQL("false").NotNull()
			})
		}); err != nil {
			return err
		}
		if err := schema.WhenTableDoesntHaveColumn(tableName, "output_price_configured", func() error {
			return schema.Table(tableName, func(table *mschema.Blueprint) {
				table.Boolean("output_price_configured").DefaultSQL("false").NotNull()
			})
		}); err != nil {
			return err
		}
		if err := schema.UpdateRowsWhereNotEqual(tableName, "input_price_configured", true, "input_price", 0); err != nil {
			return err
		}
		if err := schema.UpdateRowsWhereNotEqual(tableName, "output_price_configured", true, "output_price", 0); err != nil {
			return err
		}
	}

	if err := schema.WhenTableDoesntHaveColumn("llm_usage_bills", "pricing_source", func() error {
		return schema.Table("llm_usage_bills", func(table *mschema.Blueprint) {
			table.String("pricing_source", 50).DefaultSQL("''").NotNull()
		})
	}); err != nil {
		return err
	}
	if err := schema.WhenTableDoesntHaveColumn("llm_usage_bills", "usage_source", func() error {
		return schema.Table("llm_usage_bills", func(table *mschema.Blueprint) {
			table.String("usage_source", 50).DefaultSQL("''").NotNull()
		})
	}); err != nil {
		return err
	}
	if err := schema.WhenTableDoesntHaveColumn("llm_usage_bills", "pricing_snapshot", func() error {
		return schema.Table("llm_usage_bills", func(table *mschema.Blueprint) {
			table.JSONB("pricing_snapshot").DefaultSQL("'{}'::jsonb").NotNull()
		})
	}); err != nil {
		return err
	}

	exists, err := schema.HasTable("llm_pricing_fallback_overrides")
	if err != nil {
		return err
	}
	if exists {
		return nil
	}
	return schema.Create("llm_pricing_fallback_overrides", func(table *mschema.Blueprint) {
		table.String("id", 64).NotNull().Primary()
		table.Boolean("enabled").DefaultSQL("true").NotNull()
		table.JSONB("rules").DefaultSQL("'[]'::jsonb").NotNull()
		table.String("updated_by", 100).DefaultSQL("''").NotNull()
		table.TimestampsTz()
	})
}

func downAddLLMPricingFallback(schema *mschema.Builder) error {
	if err := schema.DropIfExists("llm_pricing_fallback_overrides"); err != nil {
		return err
	}
	for _, tableName := range []string{"llm_usage_bills", "llm_models", "llm_custom_models"} {
		for _, column := range []string{"pricing_source", "usage_source", "pricing_snapshot", "input_price_configured", "output_price_configured"} {
			if err := schema.WhenTableHasColumn(tableName, column, func() error {
				return schema.Table(tableName, func(table *mschema.Blueprint) {
					table.DropColumn(column)
				})
			}); err != nil {
				return err
			}
		}
	}
	return nil
}
