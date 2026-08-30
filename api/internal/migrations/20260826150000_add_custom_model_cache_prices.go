package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

const migrationAddCustomModelCachePricesID = "20260826150000_add_custom_model_cache_prices"

func init() {
	registerSchemaMigration(migrationAddCustomModelCachePricesID, upAddCustomModelCachePrices, nil)
}

func upAddCustomModelCachePrices(schema *mschema.Builder) error {
	return schema.Raw(`
		ALTER TABLE public.llm_custom_models
			ADD COLUMN IF NOT EXISTS cost_cache_read numeric(24,12) NOT NULL DEFAULT 0,
			ADD COLUMN IF NOT EXISTS cost_cache_write numeric(24,12) NOT NULL DEFAULT 0,
			ADD COLUMN IF NOT EXISTS cache_read_price_configured boolean NOT NULL DEFAULT false,
			ADD COLUMN IF NOT EXISTS cache_write_price_configured boolean NOT NULL DEFAULT false;
	`)
}
