package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

const migration20260530092000ID = "20260530092000_create_data_library_document_chunks"

func init() {
	registerSchemaMigration(
		migration20260530092000ID,
		upCreateDataLibraryDocumentChunks,
		downCreateDataLibraryDocumentChunks,
	)
}

func upCreateDataLibraryDocumentChunks(schema *mschema.Builder) error {
	statements := []string{
		`
		CREATE TABLE IF NOT EXISTS public.data_library_document_chunks (
			id UUID PRIMARY KEY,
			organization_id VARCHAR(255) NOT NULL,
			workspace_id VARCHAR(255) NULL,
			asset_id UUID NOT NULL REFERENCES public.data_library_document_assets(id) ON DELETE CASCADE,
			processing_run_id UUID NOT NULL,
			generation_no BIGINT NOT NULL,
			chunk_artifact_set_id UUID NULL REFERENCES public.content_parse_chunk_artifact_sets(id) ON DELETE SET NULL,
			parent_chunk_id UUID NULL REFERENCES public.data_library_document_chunks(id) ON DELETE CASCADE,
			position INTEGER NOT NULL DEFAULT 0,
			chunk_type VARCHAR(32) NOT NULL,
			content TEXT NOT NULL,
			content_hash VARCHAR(255) NOT NULL,
			source_locator_json JSONB NOT NULL DEFAULT '{}'::jsonb,
			enabled BOOLEAN NOT NULL DEFAULT TRUE,
			status VARCHAR(32) NOT NULL DEFAULT 'ready',
			metadata_json JSONB NOT NULL DEFAULT '{}'::jsonb,
			created_by VARCHAR(255) NULL,
			updated_by VARCHAR(255) NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			deleted_at TIMESTAMPTZ NULL
		)
		`,
		`
		CREATE INDEX IF NOT EXISTS idx_data_library_document_chunks_org_asset_generation
		ON public.data_library_document_chunks (organization_id, asset_id, generation_no, position)
		`,
		`
		CREATE INDEX IF NOT EXISTS idx_data_library_document_chunks_parent_position
		ON public.data_library_document_chunks (asset_id, parent_chunk_id, position)
		`,
		`
		CREATE INDEX IF NOT EXISTS idx_data_library_document_chunks_asset_enabled_status
		ON public.data_library_document_chunks (asset_id, enabled, status)
		`,
		`
		CREATE INDEX IF NOT EXISTS idx_data_library_document_chunks_artifact_set
		ON public.data_library_document_chunks (chunk_artifact_set_id)
		`,
		`
		CREATE INDEX IF NOT EXISTS idx_data_library_document_chunks_deleted_at
		ON public.data_library_document_chunks (deleted_at)
		`,
	}
	for _, statement := range statements {
		if err := schema.Raw(statement); err != nil {
			return err
		}
	}
	return nil
}

func downCreateDataLibraryDocumentChunks(schema *mschema.Builder) error {
	return schema.DropIfExists("data_library_document_chunks")
}
