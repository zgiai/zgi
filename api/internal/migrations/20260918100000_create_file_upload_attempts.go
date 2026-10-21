package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

const migrationCreateFileUploadAttemptsID = "20260918100000_create_file_upload_attempts"

const createFileUploadAttemptsSQL = `
	CREATE TABLE IF NOT EXISTS public.file_upload_attempts (
		id uuid PRIMARY KEY,
		owner_id varchar(255) NOT NULL,
		file_id varchar(255) NOT NULL,
		object_key varchar(1024) NOT NULL,
		status varchar(32) NOT NULL,
		expires_at timestamptz NOT NULL,
		created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
		CONSTRAINT file_upload_attempts_status_check
			CHECK (status IN ('pending', 'completed', 'claimed', 'cancelled'))
	);
	CREATE INDEX IF NOT EXISTS idx_file_upload_attempts_expiry
		ON public.file_upload_attempts (status, expires_at);
	CREATE INDEX IF NOT EXISTS idx_file_upload_attempts_file
		ON public.file_upload_attempts (file_id);
`

const rollbackFileUploadAttemptsSQL = `
	DO $$
	BEGIN
		IF EXISTS (
			SELECT 1
			FROM public.file_upload_attempts
			WHERE status IN ('pending', 'completed', 'cancelled')
			LIMIT 1
		) THEN
			RAISE EXCEPTION 'cannot remove active file upload attempts while records exist';
		END IF;
	END
	$$;

	DROP TABLE public.file_upload_attempts;
`

func init() {
	registerSchemaMigration(
		migrationCreateFileUploadAttemptsID,
		upCreateFileUploadAttempts,
		downCreateFileUploadAttempts,
	)
}

func upCreateFileUploadAttempts(schema *mschema.Builder) error {
	return schema.Raw(createFileUploadAttemptsSQL)
}

func downCreateFileUploadAttempts(schema *mschema.Builder) error {
	return schema.Raw(rollbackFileUploadAttemptsSQL)
}
