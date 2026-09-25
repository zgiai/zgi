package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

func init() {
	registerSchemaMigration("20260831090000_create_database_schema_index_settings", upDatabaseSchemaIndexSettings, nil)
}

func upDatabaseSchemaIndexSettings(s *mschema.Builder) error {
	return s.Raw(`
		CREATE TABLE public.database_schema_index_settings (
			data_source_id uuid PRIMARY KEY,
			organization_id uuid NOT NULL,
			workspace_id uuid NOT NULL,
			requested_by uuid NOT NULL,
			embedding_provider varchar(100) NOT NULL,
			embedding_model varchar(255) NOT NULL,
			target_ids jsonb NOT NULL DEFAULT '[]'::jsonb,
			last_error_code varchar(128),
			reconciled_at timestamptz,
			created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP
		);
		ALTER TABLE public.text2sql_schema_indexes ADD COLUMN build_attempts integer NOT NULL DEFAULT 0;
	`)
}
