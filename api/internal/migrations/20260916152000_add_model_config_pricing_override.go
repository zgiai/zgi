package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

const migrationAddModelConfigPricingOverrideID = "20260916152000_add_model_config_pricing_override"

func init() {
	registerSchemaMigration(migrationAddModelConfigPricingOverrideID, upAddModelConfigPricingOverride, downAddModelConfigPricingOverride)
}

func upAddModelConfigPricingOverride(schema *mschema.Builder) error {
	return schema.Raw(`
		ALTER TABLE public.llm_model_configs
			ADD COLUMN IF NOT EXISTS pricing_override jsonb;
	`)
}

func downAddModelConfigPricingOverride(schema *mschema.Builder) error {
	return schema.Raw(`
		ALTER TABLE public.llm_model_configs
			DROP COLUMN IF EXISTS pricing_override;
	`)
}
