package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

const migrationAlterVideoRuntimeTaskTimestampsTzID = "20260812100000_alter_video_runtime_task_timestamps_tz"

func init() {
	registerSchemaMigration(
		migrationAlterVideoRuntimeTaskTimestampsTzID,
		upAlterVideoRuntimeTaskTimestampsTz,
		nil,
	)
}

func upAlterVideoRuntimeTaskTimestampsTz(schema *mschema.Builder) error {
	return schema.Raw(`
		ALTER TABLE public.video_runtime_tasks
			ALTER COLUMN created_at TYPE timestamp with time zone USING created_at AT TIME ZONE 'UTC',
			ALTER COLUMN updated_at TYPE timestamp with time zone USING updated_at AT TIME ZONE 'UTC',
			ALTER COLUMN completed_at TYPE timestamp with time zone USING completed_at AT TIME ZONE 'UTC';
	`)
}
