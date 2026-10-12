package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

const (
	migration202610122100000000ID = "202610122100000000_add_channel_approval_job_sender"
	// The sender identity binds a group approval to the member who started it.
	// Added in its own migration so databases that already applied the
	// approval job table receive the columns too.
	addChannelApprovalJobSenderSQL = `
		ALTER TABLE public.digital_human_channel_approval_jobs
			ADD COLUMN IF NOT EXISTS sender_external_id varchar(255) NOT NULL DEFAULT '',
			ADD COLUMN IF NOT EXISTS conversation_type varchar(16) NOT NULL DEFAULT '';
	`
)

func init() {
	registerSchemaMigration(migration202610122100000000ID, up202610122100000000, down202610122100000000)
}

func up202610122100000000(schema *mschema.Builder) error {
	return schema.Raw(addChannelApprovalJobSenderSQL)
}

func down202610122100000000(schema *mschema.Builder) error {
	return schema.Raw(`
		ALTER TABLE public.digital_human_channel_approval_jobs
			DROP COLUMN IF EXISTS conversation_type,
			DROP COLUMN IF EXISTS sender_external_id;
	`)
}
