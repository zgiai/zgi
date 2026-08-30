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

// upAddVendorToLLMModels is retained as a compatibility marker for
// environments that observed the original migration before vendor became
// runtime-only display metadata. New installations intentionally make no
// llm_models schema change.
func upAddVendorToLLMModels(_ *mschema.Builder) error {
	return nil
}
