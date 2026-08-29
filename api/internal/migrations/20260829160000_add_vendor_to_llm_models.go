package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

const migrationAddVendorToLLMModelsID = "20260829160000_add_vendor_to_llm_models"

func init() {
	registerSchemaMigration(
		migrationAddVendorToLLMModelsID,
		upAddVendorToLLMModels,
		nil,
	)
}

func upAddVendorToLLMModels(schema *mschema.Builder) error {
	return schema.Raw(`
		ALTER TABLE public.llm_models
			ADD COLUMN IF NOT EXISTS vendor varchar(100) NOT NULL DEFAULT '';

		CREATE INDEX IF NOT EXISTS idx_llm_models_vendor
			ON public.llm_models (vendor)
			WHERE vendor <> ''
	`)
}
