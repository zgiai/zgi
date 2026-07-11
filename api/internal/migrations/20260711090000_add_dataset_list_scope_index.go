package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

const migration20260711090000ID = "20260711090000_add_dataset_list_scope_index"

func init() {
	registerSchemaMigration(
		migration20260711090000ID,
		upAddDatasetListScopeIndex,
		downAddDatasetListScopeIndex,
	)
}

func upAddDatasetListScopeIndex(schema *mschema.Builder) error {
	return schema.Raw(`
		CREATE INDEX IF NOT EXISTS idx_datasets_organization_workspace_created
		ON public.datasets (organization_id, workspace_id, created_at DESC)
	`)
}

func downAddDatasetListScopeIndex(schema *mschema.Builder) error {
	return schema.Raw(`DROP INDEX IF EXISTS public.idx_datasets_organization_workspace_created`)
}
