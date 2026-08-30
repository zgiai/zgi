package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

const migrationIncreaseLLMPricePrecisionID = "20260823110000_increase_llm_price_precision"

func init() {
	registerSchemaMigration(migrationIncreaseLLMPricePrecisionID, upIncreaseLLMPricePrecision, nil)
}

func upIncreaseLLMPricePrecision(schema *mschema.Builder) error {
	return schema.Raw(`
		ALTER TABLE public.llm_models
			ALTER COLUMN input_price TYPE numeric(24,12) USING input_price::numeric(24,12),
			ALTER COLUMN output_price TYPE numeric(24,12) USING output_price::numeric(24,12),
			ALTER COLUMN cached_input_price TYPE numeric(24,12) USING cached_input_price::numeric(24,12),
			ALTER COLUMN cost_cache_read TYPE numeric(24,12) USING cost_cache_read::numeric(24,12),
			ALTER COLUMN cost_cache_write TYPE numeric(24,12) USING cost_cache_write::numeric(24,12);

		ALTER TABLE public.llm_model_configs
			ALTER COLUMN input_price_override TYPE numeric(24,12) USING input_price_override::numeric(24,12),
			ALTER COLUMN output_price_override TYPE numeric(24,12) USING output_price_override::numeric(24,12),
			ALTER COLUMN cache_read_price_override TYPE numeric(24,12) USING cache_read_price_override::numeric(24,12),
			ALTER COLUMN cache_write_price_override TYPE numeric(24,12) USING cache_write_price_override::numeric(24,12);

		ALTER TABLE public.llm_custom_models
			ALTER COLUMN input_price TYPE numeric(24,12) USING input_price::numeric(24,12),
			ALTER COLUMN output_price TYPE numeric(24,12) USING output_price::numeric(24,12);
	`)
}
