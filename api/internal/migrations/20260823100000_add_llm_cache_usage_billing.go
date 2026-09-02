package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

const migrationAddLLMCacheUsageBillingID = "20260823100000_add_llm_cache_usage_billing"

func init() {
	registerSchemaMigration(migrationAddLLMCacheUsageBillingID, upAddLLMCacheUsageBilling, nil)
}

func upAddLLMCacheUsageBilling(schema *mschema.Builder) error {
	return schema.Raw(`
		ALTER TABLE public.llm_usage_bills
			ADD COLUMN IF NOT EXISTS cache_read_tokens bigint NOT NULL DEFAULT 0,
			ADD COLUMN IF NOT EXISTS cache_write_tokens bigint NOT NULL DEFAULT 0;

		ALTER TABLE public.llm_usage_bills
			DROP CONSTRAINT IF EXISTS ck_llm_usage_bills_total_tokens;
		ALTER TABLE public.llm_usage_bills
			ADD CONSTRAINT ck_llm_usage_bills_total_tokens
			CHECK (total_tokens = prompt_tokens + cache_read_tokens + cache_write_tokens + completion_tokens);

		ALTER TABLE public.llm_usage_bills
			DROP CONSTRAINT IF EXISTS ck_llm_usage_bills_non_negative;
		ALTER TABLE public.llm_usage_bills
			ADD CONSTRAINT ck_llm_usage_bills_non_negative CHECK (
				prompt_tokens >= 0 AND completion_tokens >= 0 AND
				cache_read_tokens >= 0 AND cache_write_tokens >= 0 AND total_tokens >= 0 AND
				official_points >= 0 AND private_points >= 0 AND total_points >= 0 AND response_time_ms >= 0
			);

		ALTER TABLE public.llm_model_configs
			ADD COLUMN IF NOT EXISTS cache_read_price_override numeric(10,6),
			ADD COLUMN IF NOT EXISTS cache_write_price_override numeric(10,6);

		ALTER TABLE public.llm_model_configs
			ALTER COLUMN input_price_override TYPE numeric(10,6) USING input_price_override::numeric(10,6),
			ALTER COLUMN output_price_override TYPE numeric(10,6) USING output_price_override::numeric(10,6);

		ALTER TABLE public.llm_models
			ADD COLUMN IF NOT EXISTS cache_read_price_configured boolean NOT NULL DEFAULT false,
			ADD COLUMN IF NOT EXISTS cache_write_price_configured boolean NOT NULL DEFAULT false,
			ALTER COLUMN cost_cache_read TYPE numeric(10,6) USING cost_cache_read::numeric(10,6),
			ALTER COLUMN cost_cache_write TYPE numeric(10,6) USING cost_cache_write::numeric(10,6);

		UPDATE public.llm_models
		SET cache_read_price_configured = true
		WHERE cost_cache_read <> 0 OR cached_input_price <> 0;
		UPDATE public.llm_models
		SET cache_write_price_configured = true
		WHERE cost_cache_write <> 0;
	`)
}
