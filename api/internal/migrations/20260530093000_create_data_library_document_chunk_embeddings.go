package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

const migration20260530093000ID = "20260530093000_create_data_library_document_chunk_embeddings"

func init() {
	registerSchemaMigration(
		migration20260530093000ID,
		upCreateDataLibraryDocumentChunkEmbeddings,
		downCreateDataLibraryDocumentChunkEmbeddings,
	)
}

func upCreateDataLibraryDocumentChunkEmbeddings(schema *mschema.Builder) error {
	statements := []string{
		`
		CREATE TABLE IF NOT EXISTS public.data_library_document_chunk_embeddings (
			id UUID PRIMARY KEY,
			organization_id VARCHAR(255) NOT NULL,
			workspace_id VARCHAR(255) NULL,
			asset_id UUID NOT NULL REFERENCES public.data_library_document_assets(id) ON DELETE CASCADE,
			chunk_id UUID NOT NULL REFERENCES public.data_library_document_chunks(id) ON DELETE CASCADE,
			processing_run_id UUID NOT NULL,
			generation_no BIGINT NOT NULL,
			embedding_provider VARCHAR(128) NOT NULL,
			embedding_model VARCHAR(255) NOT NULL,
			embedding_dimension INTEGER NOT NULL DEFAULT 0,
			embedding_vector REAL[] NOT NULL DEFAULT '{}'::real[],
			content_hash VARCHAR(255) NOT NULL,
			status VARCHAR(32) NOT NULL DEFAULT 'ready',
			metadata_json JSONB NOT NULL DEFAULT '{}'::jsonb,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			deleted_at TIMESTAMPTZ NULL
		)
		`,
		`
		CREATE INDEX IF NOT EXISTS idx_data_library_chunk_embeddings_org_asset_generation
		ON public.data_library_document_chunk_embeddings (organization_id, asset_id, generation_no)
		`,
		`
		CREATE INDEX IF NOT EXISTS idx_data_library_chunk_embeddings_chunk_model
		ON public.data_library_document_chunk_embeddings (chunk_id, embedding_provider, embedding_model)
		`,
		`
		CREATE INDEX IF NOT EXISTS idx_data_library_chunk_embeddings_asset_status
		ON public.data_library_document_chunk_embeddings (asset_id, status)
		`,
		`
		CREATE INDEX IF NOT EXISTS idx_data_library_chunk_embeddings_deleted_at
		ON public.data_library_document_chunk_embeddings (deleted_at)
		`,
		`
		CREATE UNIQUE INDEX IF NOT EXISTS uq_data_library_chunk_embeddings_active_model
		ON public.data_library_document_chunk_embeddings (chunk_id, embedding_provider, embedding_model)
		WHERE deleted_at IS NULL AND status <> 'deleted'
		`,
	}
	for _, statement := range statements {
		if err := schema.Raw(statement); err != nil {
			return err
		}
	}
	return nil
}

func downCreateDataLibraryDocumentChunkEmbeddings(schema *mschema.Builder) error {
	return schema.DropIfExists("data_library_document_chunk_embeddings")
}
