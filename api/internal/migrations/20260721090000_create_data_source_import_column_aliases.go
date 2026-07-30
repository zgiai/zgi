package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

const migrationCreateDataSourceImportColumnAliasesID = "20260721090000_create_data_source_import_column_aliases"

func init() {
	registerSchemaMigration(migrationCreateDataSourceImportColumnAliasesID, upCreateDataSourceImportColumnAliases, nil)
}

func upCreateDataSourceImportColumnAliases(schema *mschema.Builder) error {
	return schema.Raw(`
		CREATE TABLE IF NOT EXISTS public.data_source_import_column_aliases (
			id uuid PRIMARY KEY,
			organization_id uuid NOT NULL,
			data_source_id uuid NOT NULL,
			table_id uuid NOT NULL,
			target_column_id varchar(255) NOT NULL,
			target_column_name varchar(255) NOT NULL,
			source_header varchar(512) NOT NULL,
			normalized_header varchar(512) NOT NULL,
			confirmed_count integer NOT NULL DEFAULT 1,
			created_by varchar(36) NOT NULL,
			updated_by varchar(36) NOT NULL,
			created_at timestamp without time zone NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at timestamp without time zone NOT NULL DEFAULT CURRENT_TIMESTAMP,
			CONSTRAINT data_source_import_column_aliases_count_check CHECK (confirmed_count > 0),
			CONSTRAINT data_source_import_column_aliases_data_source_fkey FOREIGN KEY (data_source_id) REFERENCES public.data_sources(id) ON DELETE CASCADE,
			CONSTRAINT data_source_import_column_aliases_table_fkey FOREIGN KEY (table_id) REFERENCES public.data_source_tables(id) ON DELETE CASCADE
		);

		CREATE UNIQUE INDEX IF NOT EXISTS idx_data_source_import_column_aliases_unique
		ON public.data_source_import_column_aliases (table_id, target_column_id, normalized_header);

		CREATE INDEX IF NOT EXISTS idx_data_source_import_column_aliases_lookup
		ON public.data_source_import_column_aliases (organization_id, data_source_id, table_id, normalized_header)
	`)
}
