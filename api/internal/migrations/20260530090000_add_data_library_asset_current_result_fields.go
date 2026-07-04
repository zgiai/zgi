package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

const migration20260530090000ID = "20260530090000_add_data_library_asset_current_result_fields"

func init() {
	registerSchemaMigration(
		migration20260530090000ID,
		upAddDataLibraryAssetCurrentResultFields,
		nil,
	)
}

func upAddDataLibraryAssetCurrentResultFields(schema *mschema.Builder) error {
	statements := []string{
		`
		ALTER TABLE public.data_library_document_assets
			ADD COLUMN IF NOT EXISTS product_status VARCHAR(32) NOT NULL DEFAULT 'stored_only',
			ADD COLUMN IF NOT EXISTS processing_stage VARCHAR(32) NULL,
			ADD COLUMN IF NOT EXISTS processing_progress INTEGER NOT NULL DEFAULT 0,
			ADD COLUMN IF NOT EXISTS active_processing_request_id UUID NULL,
			ADD COLUMN IF NOT EXISTS processing_run_id UUID NULL,
			ADD COLUMN IF NOT EXISTS generation_no BIGINT NOT NULL DEFAULT 0,
			ADD COLUMN IF NOT EXISTS parse_artifact_id UUID NULL,
			ADD COLUMN IF NOT EXISTS chunk_artifact_set_id UUID NULL,
			ADD COLUMN IF NOT EXISTS chunk_count INTEGER NOT NULL DEFAULT 0,
			ADD COLUMN IF NOT EXISTS embedding_provider VARCHAR(128) NULL,
			ADD COLUMN IF NOT EXISTS embedding_model VARCHAR(255) NULL,
			ADD COLUMN IF NOT EXISTS embedding_dimension INTEGER NULL,
			ADD COLUMN IF NOT EXISTS vector_status VARCHAR(32) NOT NULL DEFAULT 'none',
			ADD COLUMN IF NOT EXISTS last_error_code VARCHAR(128) NULL,
			ADD COLUMN IF NOT EXISTS last_error_message TEXT NULL
		`,
		`
		CREATE INDEX IF NOT EXISTS idx_data_library_assets_product_status
		ON public.data_library_document_assets (organization_id, workspace_id, product_status, updated_at DESC)
		`,
		`
		CREATE INDEX IF NOT EXISTS idx_data_library_assets_processing_run
		ON public.data_library_document_assets (processing_run_id)
		WHERE processing_run_id IS NOT NULL
		`,
		`
		CREATE INDEX IF NOT EXISTS idx_data_library_assets_parse_artifact
		ON public.data_library_document_assets (parse_artifact_id)
		WHERE parse_artifact_id IS NOT NULL
		`,
		`
		CREATE INDEX IF NOT EXISTS idx_data_library_assets_chunk_artifact
		ON public.data_library_document_assets (chunk_artifact_set_id)
		WHERE chunk_artifact_set_id IS NOT NULL
		`,
		`
		CREATE INDEX IF NOT EXISTS idx_data_library_assets_vector_status
		ON public.data_library_document_assets (organization_id, vector_status, updated_at DESC)
		`,
		`
		DO $$
		BEGIN
			IF NOT EXISTS (
				SELECT 1
				FROM pg_constraint
				WHERE conname = 'fk_data_library_assets_active_processing_request'
				  AND conrelid = 'public.data_library_document_assets'::regclass
			) THEN
				ALTER TABLE public.data_library_document_assets
					ADD CONSTRAINT fk_data_library_assets_active_processing_request
					FOREIGN KEY (active_processing_request_id)
					REFERENCES public.data_library_processing_requests (id)
					ON DELETE SET NULL;
			END IF;
		END $$;
		`,
		`
		DO $$
		BEGIN
			IF NOT EXISTS (
				SELECT 1
				FROM pg_constraint
				WHERE conname = 'fk_data_library_assets_parse_artifact'
				  AND conrelid = 'public.data_library_document_assets'::regclass
			) THEN
				ALTER TABLE public.data_library_document_assets
					ADD CONSTRAINT fk_data_library_assets_parse_artifact
					FOREIGN KEY (parse_artifact_id)
					REFERENCES public.content_parse_artifacts (id)
					ON DELETE SET NULL;
			END IF;
		END $$;
		`,
		`
		DO $$
		BEGIN
			IF NOT EXISTS (
				SELECT 1
				FROM pg_constraint
				WHERE conname = 'fk_data_library_assets_chunk_artifact_set'
				  AND conrelid = 'public.data_library_document_assets'::regclass
			) THEN
				ALTER TABLE public.data_library_document_assets
					ADD CONSTRAINT fk_data_library_assets_chunk_artifact_set
					FOREIGN KEY (chunk_artifact_set_id)
					REFERENCES public.content_parse_chunk_artifact_sets (id)
					ON DELETE SET NULL;
			END IF;
		END $$;
		`,
	}
	for _, statement := range statements {
		if err := schema.Raw(statement); err != nil {
			return err
		}
	}
	return nil
}
