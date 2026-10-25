package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

// The user-owned memory archive (ADR 0014): what a user's memory keeps but
// does not show in every conversation, found by memory_search. Entries go
// with their relationship, so erasure and permanent deletion leave none.
func init() {
	registerSchemaMigration("202610251300000000_user_memory_archive", upUserMemoryArchive, downUserMemoryArchive)
}

func upUserMemoryArchive(schema *mschema.Builder) error {
	return schema.Raw(`
		CREATE TABLE IF NOT EXISTS public.user_memory_archive_entries (
			id uuid PRIMARY KEY,
			relationship_id uuid NOT NULL REFERENCES public.user_memory_relationships(id) ON DELETE CASCADE,
			tags varchar(64) NOT NULL DEFAULT '',
			source_key varchar(64) NOT NULL DEFAULT '',
			source_kind varchar(32) NOT NULL DEFAULT '',
			content_payload text NOT NULL DEFAULT '',
			content_encoding varchar(24) NOT NULL DEFAULT 'plain',
			content_key_id varchar(128),
			created_at timestamptz NOT NULL,
			updated_at timestamptz NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_user_memory_archive_relationship ON public.user_memory_archive_entries (relationship_id, created_at DESC);
	`)
}

func downUserMemoryArchive(schema *mschema.Builder) error {
	return schema.Raw(`DROP TABLE IF EXISTS public.user_memory_archive_entries;`)
}
