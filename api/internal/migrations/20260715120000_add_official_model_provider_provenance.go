package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

const migrationAddOfficialModelProviderProvenanceID = "20260715120000_add_official_model_provider_provenance"

func init() {
	registerSchemaMigration(
		migrationAddOfficialModelProviderProvenanceID,
		upAddOfficialModelProviderProvenance,
		nil,
	)
}

func upAddOfficialModelProviderProvenance(schema *mschema.Builder) error {
	return schema.Raw(`
		ALTER TABLE public.llm_official_model_snapshots
			ADD COLUMN IF NOT EXISTS effective_provider_models jsonb NOT NULL DEFAULT '[]'::jsonb
	`)
}
