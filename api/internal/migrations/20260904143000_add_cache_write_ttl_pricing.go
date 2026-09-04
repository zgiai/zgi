package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

const migrationAddCacheWriteTTLPricingID = "20260904143000_add_cache_write_ttl_pricing"

func init() {
	registerSchemaMigration(migrationAddCacheWriteTTLPricingID, upAddCacheWriteTTLPricing, nil)
}

func upAddCacheWriteTTLPricing(schema *mschema.Builder) error {
	return schema.Raw(`
		ALTER TABLE public.llm_models
			ADD COLUMN IF NOT EXISTS cost_cache_write_5m numeric(24,12),
			ADD COLUMN IF NOT EXISTS cost_cache_write_1h numeric(24,12),
			ADD COLUMN IF NOT EXISTS cache_write_5m_price_configured boolean NOT NULL DEFAULT false,
			ADD COLUMN IF NOT EXISTS cache_write_1h_price_configured boolean NOT NULL DEFAULT false;

		ALTER TABLE public.llm_custom_models
			ADD COLUMN IF NOT EXISTS cost_cache_write_5m numeric(24,12) NOT NULL DEFAULT 0,
			ADD COLUMN IF NOT EXISTS cost_cache_write_1h numeric(24,12) NOT NULL DEFAULT 0,
			ADD COLUMN IF NOT EXISTS cache_write_5m_price_configured boolean NOT NULL DEFAULT false,
			ADD COLUMN IF NOT EXISTS cache_write_1h_price_configured boolean NOT NULL DEFAULT false;

		ALTER TABLE public.llm_model_configs
			ADD COLUMN IF NOT EXISTS cache_write_5m_price_override numeric(24,12),
			ADD COLUMN IF NOT EXISTS cache_write_1h_price_override numeric(24,12);
	`)
}
