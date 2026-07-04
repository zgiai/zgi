package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

const migration20260530091000ID = "20260530091000_create_data_library_parse_confirmation_items"

func init() {
	registerSchemaMigration(
		migration20260530091000ID,
		upCreateDataLibraryParseConfirmationItems,
		downCreateDataLibraryParseConfirmationItems,
	)
}

func upCreateDataLibraryParseConfirmationItems(schema *mschema.Builder) error {
	statements := []string{
		`
		CREATE TABLE IF NOT EXISTS public.data_library_parse_confirmation_items (
			id UUID PRIMARY KEY,
			organization_id VARCHAR(255) NOT NULL,
			workspace_id VARCHAR(255) NULL,
			asset_id UUID NOT NULL REFERENCES public.data_library_document_assets(id) ON DELETE CASCADE,
			processing_run_id UUID NOT NULL,
			generation_no BIGINT NOT NULL,
			item_type VARCHAR(64) NOT NULL,
			status VARCHAR(32) NOT NULL DEFAULT 'pending',
			source_locator_json JSONB NOT NULL DEFAULT '{}'::jsonb,
			original_content TEXT NOT NULL,
			suggested_content TEXT NULL,
			final_content TEXT NULL,
			confidence DOUBLE PRECISION NULL,
			review_reason VARCHAR(128) NULL,
			created_by VARCHAR(255) NULL,
			updated_by VARCHAR(255) NULL,
			resolved_at TIMESTAMPTZ NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			deleted_at TIMESTAMPTZ NULL
		)
		`,
		`
		CREATE INDEX IF NOT EXISTS idx_data_library_parse_confirm_org_asset_status
		ON public.data_library_parse_confirmation_items (organization_id, asset_id, status, created_at DESC)
		`,
		`
		CREATE INDEX IF NOT EXISTS idx_data_library_parse_confirm_run_status
		ON public.data_library_parse_confirmation_items (processing_run_id, status, created_at DESC)
		`,
		`
		CREATE INDEX IF NOT EXISTS idx_data_library_parse_confirm_asset_generation
		ON public.data_library_parse_confirmation_items (asset_id, generation_no, status, created_at DESC)
		`,
		`
		CREATE INDEX IF NOT EXISTS idx_data_library_parse_confirm_deleted_at
		ON public.data_library_parse_confirmation_items (deleted_at)
		`,
	}
	for _, statement := range statements {
		if err := schema.Raw(statement); err != nil {
			return err
		}
	}
	return nil
}

func downCreateDataLibraryParseConfirmationItems(schema *mschema.Builder) error {
	return schema.DropIfExists("data_library_parse_confirmation_items")
}
