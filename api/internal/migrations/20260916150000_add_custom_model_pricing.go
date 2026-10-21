package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

const migrationAddCustomModelPricingID = "20260916150000_add_custom_model_pricing"

func init() {
	registerSchemaMigration(migrationAddCustomModelPricingID, upAddCustomModelPricing, downAddCustomModelPricing)
}

func upAddCustomModelPricing(schema *mschema.Builder) error {
	return schema.Raw(`
		ALTER TABLE public.llm_custom_models
			ADD COLUMN IF NOT EXISTS pricing jsonb NOT NULL DEFAULT '{}'::jsonb;
	`)
}

func downAddCustomModelPricing(schema *mschema.Builder) error {
	return schema.Raw(`
		ALTER TABLE public.llm_custom_models
			DROP COLUMN IF EXISTS pricing;
	`)
}
