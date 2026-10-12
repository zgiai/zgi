package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

const (
	migration202610122000000000ID         = "202610122000000000_add_digital_human_channel_approval_jobs"
	addDigitalHumanChannelApprovalJobsSQL = `
		CREATE TABLE IF NOT EXISTS public.digital_human_channel_approval_jobs (
			id uuid PRIMARY KEY,
			organization_id uuid NOT NULL REFERENCES public.organizations(id) ON DELETE RESTRICT,
			workspace_id uuid NOT NULL REFERENCES public.workspaces(id) ON DELETE RESTRICT,
			installation_id uuid NOT NULL REFERENCES public.channel_installations(id) ON DELETE CASCADE,
			inbound_event_id uuid NOT NULL REFERENCES public.channel_inbound_events(id) ON DELETE CASCADE,
			actor_id uuid NOT NULL REFERENCES public.accounts(id) ON DELETE RESTRICT,
			digital_human_id uuid NOT NULL REFERENCES public.digital_humans(id) ON DELETE CASCADE,
			conversation_id uuid NOT NULL,
			sender_external_id varchar(255) NOT NULL DEFAULT '',
			conversation_type varchar(16) NOT NULL DEFAULT '',
			text text NOT NULL,
			status varchar(16) NOT NULL,
			attempt_count integer NOT NULL DEFAULT 0,
			available_at timestamptz NOT NULL,
			claimed_at timestamptz,
			completed_at timestamptz,
			error_code varchar(128) NOT NULL DEFAULT '',
			created_at timestamptz NOT NULL,
			updated_at timestamptz NOT NULL,
			CONSTRAINT digital_human_channel_approval_jobs_inbound_unique UNIQUE (inbound_event_id),
			CONSTRAINT digital_human_channel_approval_jobs_status_check CHECK (
				status IN ('pending', 'running', 'succeeded', 'failed')
			),
			CONSTRAINT digital_human_channel_approval_jobs_attempt_check CHECK (attempt_count >= 0)
		);

		CREATE INDEX IF NOT EXISTS idx_digital_human_channel_approval_jobs_claim
			ON public.digital_human_channel_approval_jobs (status, available_at, created_at);
	`
)

func init() {
	registerSchemaMigration(migration202610122000000000ID, up202610122000000000, down202610122000000000)
}

func up202610122000000000(schema *mschema.Builder) error {
	return schema.Raw(addDigitalHumanChannelApprovalJobsSQL)
}

func down202610122000000000(schema *mschema.Builder) error {
	return schema.Raw(`DROP TABLE IF EXISTS public.digital_human_channel_approval_jobs;`)
}
