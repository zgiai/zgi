package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

const migrationDropPriceConfiguredBooleansID = "20260916151000_drop_price_configured_booleans"

func init() {
	registerSchemaMigration(migrationDropPriceConfiguredBooleansID, upDropPriceConfiguredBooleans, downDropPriceConfiguredBooleans)
}

func upDropPriceConfiguredBooleans(schema *mschema.Builder) error {
	return schema.Raw(`
		ALTER TABLE public.llm_models
			DROP COLUMN IF EXISTS input_price_configured,
			DROP COLUMN IF EXISTS output_price_configured,
			DROP COLUMN IF EXISTS cache_read_price_configured,
			DROP COLUMN IF EXISTS cache_write_price_configured,
			DROP COLUMN IF EXISTS cache_write_5m_price_configured,
			DROP COLUMN IF EXISTS cache_write_1h_price_configured;

		ALTER TABLE public.llm_custom_models
			DROP COLUMN IF EXISTS input_price_configured,
			DROP COLUMN IF EXISTS output_price_configured,
			DROP COLUMN IF EXISTS cache_read_price_configured,
			DROP COLUMN IF EXISTS cache_write_price_configured,
			DROP COLUMN IF EXISTS cache_write_5m_price_configured,
			DROP COLUMN IF EXISTS cache_write_1h_price_configured;
	`)
}

func downDropPriceConfiguredBooleans(schema *mschema.Builder) error {
	return schema.Raw(`
		ALTER TABLE public.llm_models
			ADD COLUMN IF NOT EXISTS input_price_configured boolean NOT NULL DEFAULT false,
			ADD COLUMN IF NOT EXISTS output_price_configured boolean NOT NULL DEFAULT false,
			ADD COLUMN IF NOT EXISTS cache_read_price_configured boolean NOT NULL DEFAULT false,
			ADD COLUMN IF NOT EXISTS cache_write_price_configured boolean NOT NULL DEFAULT false,
			ADD COLUMN IF NOT EXISTS cache_write_5m_price_configured boolean NOT NULL DEFAULT false,
			ADD COLUMN IF NOT EXISTS cache_write_1h_price_configured boolean NOT NULL DEFAULT false;

		ALTER TABLE public.llm_custom_models
			ADD COLUMN IF NOT EXISTS input_price_configured boolean NOT NULL DEFAULT false,
			ADD COLUMN IF NOT EXISTS output_price_configured boolean NOT NULL DEFAULT false,
			ADD COLUMN IF NOT EXISTS cache_read_price_configured boolean NOT NULL DEFAULT false,
			ADD COLUMN IF NOT EXISTS cache_write_price_configured boolean NOT NULL DEFAULT false,
			ADD COLUMN IF NOT EXISTS cache_write_5m_price_configured boolean NOT NULL DEFAULT false,
			ADD COLUMN IF NOT EXISTS cache_write_1h_price_configured boolean NOT NULL DEFAULT false;
	`)
}
