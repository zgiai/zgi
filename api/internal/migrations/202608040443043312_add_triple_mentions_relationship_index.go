package migrations

import (
	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
)

const migration202608040443043312ID = "202608040443043312_add_triple_mentions_relationship_index"

func init() {
	registerSchemaMigration(migration202608040443043312ID, up202608040443043312, down202608040443043312)
}

func up202608040443043312(schema *mschema.Builder) error {
	return schema.Raw(`
		CREATE INDEX IF NOT EXISTS idx_triple_mentions_kb_relationship_active
			ON public.kb_triple_mentions (kb_id, relationship_id)
			WHERE is_deleted = false AND relationship_id IS NOT NULL;
	`)
}

func down202608040443043312(schema *mschema.Builder) error {
	return schema.Raw(`
		DROP INDEX IF EXISTS public.idx_triple_mentions_kb_relationship_active;
	`)
}
