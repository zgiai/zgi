package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

const migrationAddText2SQLSchemaIndexValueSamplesID = "20260922090000_add_text2sql_schema_index_value_samples"

func init() {
	registerSchemaMigration(migrationAddText2SQLSchemaIndexValueSamplesID, upAddText2SQLSchemaIndexValueSamples, nil)
}

func upAddText2SQLSchemaIndexValueSamples(schema *mschema.Builder) error {
	return schema.Raw(`
		ALTER TABLE public.text2sql_schema_indexes
			ADD COLUMN value_samples jsonb NOT NULL DEFAULT '{}'::jsonb;
	`)
}
