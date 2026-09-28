package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

const migrationCreateText2SQLSchemaIndexesID = "20260822090000_create_text2sql_schema_indexes"

func init() {
	registerSchemaMigration(migrationCreateText2SQLSchemaIndexesID, upCreateText2SQLSchemaIndexes, downCreateText2SQLSchemaIndexes)
}

func upCreateText2SQLSchemaIndexes(schema *mschema.Builder) error {
	return schema.Raw(`
		CREATE TABLE public.text2sql_schema_indexes (
			id uuid DEFAULT public.uuid_generate_v4() PRIMARY KEY,
			organization_id uuid NOT NULL,
			workspace_id uuid NOT NULL,
			data_source_id uuid NOT NULL,
			asset_table_id uuid NOT NULL,
			physical_table_id integer NOT NULL,
			schema_name varchar(255) NOT NULL,
			physical_table_name varchar(255) NOT NULL,
			fingerprint varchar(64) NOT NULL,
			embedding_provider varchar(100) NOT NULL,
			embedding_model varchar(255) NOT NULL,
			collection_name varchar(255) NOT NULL,
			vector_object_id varchar(255) NOT NULL,
			status varchar(16) NOT NULL,
			last_error_code varchar(128),
			indexed_at timestamp with time zone,
			created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
			updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
			CONSTRAINT text2sql_schema_indexes_status_check CHECK (status IN ('pending','indexing','ready','failed','deleting')),
			CONSTRAINT text2sql_schema_indexes_identity_unique UNIQUE (workspace_id, data_source_id, asset_table_id, fingerprint, embedding_provider, embedding_model)
		);
		CREATE INDEX idx_text2sql_schema_indexes_lookup
			ON public.text2sql_schema_indexes (workspace_id, data_source_id, embedding_provider, embedding_model, status);
		CREATE INDEX idx_text2sql_schema_indexes_pending
			ON public.text2sql_schema_indexes (status, updated_at);
	`)
}

func downCreateText2SQLSchemaIndexes(schema *mschema.Builder) error {
	return schema.Raw(`DROP TABLE IF EXISTS public.text2sql_schema_indexes`)
}
