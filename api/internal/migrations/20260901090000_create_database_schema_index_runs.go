package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

func init() {
	registerSchemaMigration("20260901090000_create_database_schema_index_runs", upDatabaseSchemaIndexRuns, nil)
}

func upDatabaseSchemaIndexRuns(s *mschema.Builder) error {
	return s.Raw(`
		ALTER TABLE public.database_schema_index_settings
			ADD COLUMN pending_run_id uuid,
			ADD COLUMN pending_trigger varchar(32);

		CREATE TABLE public.database_schema_index_runs (
			id uuid PRIMARY KEY,
			data_source_id uuid NOT NULL,
			organization_id uuid NOT NULL,
			workspace_id uuid NOT NULL,
			trigger varchar(32) NOT NULL,
			status varchar(16) NOT NULL,
			embedding_provider varchar(100) NOT NULL,
			embedding_model varchar(255) NOT NULL,
			target_ids jsonb NOT NULL DEFAULT '[]'::jsonb,
			tables jsonb NOT NULL DEFAULT '[]'::jsonb,
			table_count integer NOT NULL DEFAULT 0,
			failure_count integer NOT NULL DEFAULT 0,
			started_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
			completed_at timestamptz,
			created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP
		);
		CREATE INDEX idx_database_schema_index_runs_source_time
			ON public.database_schema_index_runs(data_source_id, workspace_id, started_at DESC, id DESC);
		CREATE INDEX idx_database_schema_index_runs_active
			ON public.database_schema_index_runs(data_source_id, status) WHERE status = 'building';
	`)
}
