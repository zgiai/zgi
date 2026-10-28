package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

// A version keeps the display name and category a value had before the
// change, so reverting a deletion restores the value as it was (ADR 0013).
// The name is user content and is purged with the version's content.
func init() {
	registerSchemaMigration("202610261000000000_memory_version_restore_fields", upMemoryVersionRestoreFields, downMemoryVersionRestoreFields)
}

func upMemoryVersionRestoreFields(schema *mschema.Builder) error {
	return schema.Raw(`
		ALTER TABLE public.memory_file_versions ADD COLUMN IF NOT EXISTS before_display_name varchar(80) NOT NULL DEFAULT '';
		ALTER TABLE public.memory_file_versions ADD COLUMN IF NOT EXISTS before_category varchar(32) NOT NULL DEFAULT '';
	`)
}

func downMemoryVersionRestoreFields(schema *mschema.Builder) error {
	return schema.Raw(`
		ALTER TABLE public.memory_file_versions DROP COLUMN IF EXISTS before_category;
		ALTER TABLE public.memory_file_versions DROP COLUMN IF EXISTS before_display_name;
	`)
}
