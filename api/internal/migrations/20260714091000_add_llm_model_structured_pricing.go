package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

const migrationAddLLMModelStructuredPricingID = "20260714091000_add_llm_model_structured_pricing"

func init() {
	registerSchemaMigration(
		migrationAddLLMModelStructuredPricingID,
		upAddLLMModelStructuredPricing,
		nil,
	)
}

func upAddLLMModelStructuredPricing(schema *mschema.Builder) error {
	return schema.Raw(`
		ALTER TABLE public.llm_models
			ADD COLUMN IF NOT EXISTS pricing jsonb NOT NULL DEFAULT '{}'::jsonb
	`)
}
