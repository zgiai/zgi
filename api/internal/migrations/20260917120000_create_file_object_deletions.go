package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

const migrationCreateFileObjectDeletionsID = "20260917120000_create_file_object_deletions"

// file_object_deletions is the durable handoff between a committed file delete and the
// removal of its object in storage. A row is inserted inside the delete transaction, so
// the pending object survives a crash or a storage rejection. The deleting request removes
// the object inline when it can and clears the row; the retry worker does the rest. A row
// therefore means "still not removed yet".
const createFileObjectDeletionsSQL = `
	CREATE TABLE IF NOT EXISTS public.file_object_deletions (
		id uuid PRIMARY KEY DEFAULT public.uuid_generate_v4(),
		object_key varchar(1024) NOT NULL,
		attempt_count integer NOT NULL DEFAULT 0,
		next_attempt_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
		last_error text NOT NULL DEFAULT '',
		created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
		CONSTRAINT file_object_deletions_attempt_check
			CHECK (attempt_count >= 0)
	);
	-- The sweep only queries by deadline; the table holds failures, not traffic, so it stays
	-- small enough that a lookup by object key does not need its own index.
	CREATE INDEX IF NOT EXISTS idx_file_object_deletions_ready
		ON public.file_object_deletions (next_attempt_at, created_at);
`

const rollbackFileObjectDeletionsSQL = `
	DO $$
	BEGIN
		IF EXISTS (
			SELECT 1
			FROM public.file_object_deletions
			LIMIT 1
		) THEN
			RAISE EXCEPTION 'cannot remove pending file object deletions while records exist';
		END IF;
	END
	$$;

	DROP TABLE public.file_object_deletions;
`

func init() {
	registerSchemaMigration(
		migrationCreateFileObjectDeletionsID,
		upCreateFileObjectDeletions,
		downCreateFileObjectDeletions,
	)
}

func upCreateFileObjectDeletions(schema *mschema.Builder) error {
	return schema.Raw(createFileObjectDeletionsSQL)
}

func downCreateFileObjectDeletions(schema *mschema.Builder) error {
	return schema.Raw(rollbackFileObjectDeletionsSQL)
}
