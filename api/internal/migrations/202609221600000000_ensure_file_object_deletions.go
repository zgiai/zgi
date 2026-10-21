package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

const migrationEnsureFileObjectDeletionsID = "202609221600000000_ensure_file_object_deletions"

func init() {
	registerSchemaMigration(migrationEnsureFileObjectDeletionsID, upEnsureFileObjectDeletions, downEnsureFileObjectDeletions)
}

func upEnsureFileObjectDeletions(schema *mschema.Builder) error {
	return schema.Raw(`
		CREATE TABLE IF NOT EXISTS public.file_object_deletions (
			id uuid PRIMARY KEY DEFAULT public.uuid_generate_v4(),
			object_key varchar(1024) NOT NULL,
			attempt_count integer NOT NULL DEFAULT 0,
			next_attempt_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
			last_error text NOT NULL DEFAULT '',
			created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
			CONSTRAINT file_object_deletions_attempt_check CHECK (attempt_count >= 0)
		);
		CREATE INDEX IF NOT EXISTS idx_file_object_deletions_ready
			ON public.file_object_deletions (next_attempt_at, created_at);
	`)
}

func downEnsureFileObjectDeletions(*mschema.Builder) error {
	return nil
}
