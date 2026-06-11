package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

const migrationUpdateDataLibraryKnowledgeBaseAssetRefsV1ID = "20260602090000_update_data_library_knowledge_base_asset_refs_v1"

func init() {
	registerSchemaMigration(
		migrationUpdateDataLibraryKnowledgeBaseAssetRefsV1ID,
		upUpdateDataLibraryKnowledgeBaseAssetRefsV1,
		downUpdateDataLibraryKnowledgeBaseAssetRefsV1,
	)
}

func upUpdateDataLibraryKnowledgeBaseAssetRefsV1(schema *mschema.Builder) error {
	statements := []string{
		`
		ALTER TABLE public.data_library_knowledge_base_asset_refs
			ALTER COLUMN version_id DROP NOT NULL
		`,
		`
		ALTER TABLE public.data_library_knowledge_base_asset_refs
			ADD COLUMN IF NOT EXISTS dataset_document_id UUID NULL REFERENCES public.documents(id) ON DELETE SET NULL,
			ADD COLUMN IF NOT EXISTS sync_status VARCHAR(32) NOT NULL DEFAULT 'pending',
			ADD COLUMN IF NOT EXISTS synced_generation_no BIGINT NULL,
			ADD COLUMN IF NOT EXISTS sync_run_id UUID NULL,
			ADD COLUMN IF NOT EXISTS last_synced_at TIMESTAMPTZ NULL,
			ADD COLUMN IF NOT EXISTS sync_error_code VARCHAR(128) NULL,
			ADD COLUMN IF NOT EXISTS sync_error_message TEXT NULL
		`,
		`
		CREATE UNIQUE INDEX IF NOT EXISTS idx_data_library_kb_asset_refs_dataset_asset_active
		ON public.data_library_knowledge_base_asset_refs (organization_id, dataset_id, asset_id)
		WHERE deleted_at IS NULL
		`,
		`
		CREATE INDEX IF NOT EXISTS idx_data_library_kb_asset_refs_asset_active
		ON public.data_library_knowledge_base_asset_refs (organization_id, asset_id)
		WHERE deleted_at IS NULL
		`,
		`
		CREATE INDEX IF NOT EXISTS idx_data_library_kb_asset_refs_dataset_status
		ON public.data_library_knowledge_base_asset_refs (organization_id, dataset_id, sync_status)
		WHERE deleted_at IS NULL
		`,
		`
		CREATE INDEX IF NOT EXISTS idx_data_library_kb_asset_refs_document
		ON public.data_library_knowledge_base_asset_refs (dataset_document_id)
		WHERE dataset_document_id IS NOT NULL AND deleted_at IS NULL
		`,
		`
		CREATE INDEX IF NOT EXISTS idx_data_library_kb_asset_refs_sync_run
		ON public.data_library_knowledge_base_asset_refs (sync_run_id)
		WHERE sync_run_id IS NOT NULL AND deleted_at IS NULL
		`,
	}
	for _, statement := range statements {
		if err := schema.Raw(statement); err != nil {
			return err
		}
	}
	return nil
}

func downUpdateDataLibraryKnowledgeBaseAssetRefsV1(schema *mschema.Builder) error {
	return nil
}
