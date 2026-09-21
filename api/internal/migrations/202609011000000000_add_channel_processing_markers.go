package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

const (
	migration202609011000000000ID  = "202609011000000000_add_channel_processing_markers"
	addChannelProcessingMarkersSQL = `
		CREATE TABLE IF NOT EXISTS public.channel_processing_markers (
			id uuid PRIMARY KEY,
			organization_id uuid NOT NULL REFERENCES public.organizations(id) ON DELETE RESTRICT,
			workspace_id uuid NOT NULL REFERENCES public.workspaces(id) ON DELETE RESTRICT,
			installation_id uuid NOT NULL REFERENCES public.channel_installations(id) ON DELETE CASCADE,
			inbound_event_id uuid NOT NULL REFERENCES public.channel_inbound_events(id) ON DELETE CASCADE,
			run_id uuid NOT NULL REFERENCES public.digital_human_runs(id) ON DELETE CASCADE,
			provider varchar(32) NOT NULL,
			connection_id uuid NOT NULL,
			actor_id uuid NOT NULL REFERENCES public.accounts(id) ON DELETE RESTRICT,
			subject_type varchar(32) NOT NULL,
			subject_id uuid NOT NULL,
			provider_message_id varchar(255) NOT NULL,
			provider_marker_id varchar(255) NOT NULL DEFAULT '',
			provider_request_id varchar(128) NOT NULL DEFAULT '',
			status varchar(16) NOT NULL,
			attempt_count integer NOT NULL DEFAULT 0,
			available_at timestamptz NOT NULL,
			claimed_at timestamptz,
			removed_at timestamptz,
			error_code varchar(128) NOT NULL DEFAULT '',
			error_message text NOT NULL DEFAULT '',
			created_at timestamptz NOT NULL,
			updated_at timestamptz NOT NULL,
			CONSTRAINT channel_processing_markers_inbound_unique UNIQUE (inbound_event_id),
			CONSTRAINT channel_processing_markers_provider_check CHECK (provider <> ''),
			CONSTRAINT channel_processing_markers_message_check CHECK (provider_message_id <> ''),
			CONSTRAINT channel_processing_markers_subject_check CHECK (subject_type = 'digital_human'),
			CONSTRAINT channel_processing_markers_status_check CHECK (
				status IN ('pending_add', 'adding', 'active', 'removing', 'removed', 'failed')
			),
			CONSTRAINT channel_processing_markers_attempt_check CHECK (attempt_count >= 0)
		);

		CREATE INDEX IF NOT EXISTS idx_channel_processing_markers_claim
			ON public.channel_processing_markers (status, available_at, created_at);
		CREATE INDEX IF NOT EXISTS idx_channel_processing_markers_run
			ON public.channel_processing_markers (run_id);
	`
)

func init() {
	registerSchemaMigration(migration202609011000000000ID, up202609011000000000, down202609011000000000)
}

func up202609011000000000(schema *mschema.Builder) error {
	return schema.Raw(addChannelProcessingMarkersSQL)
}

func down202609011000000000(schema *mschema.Builder) error {
	return schema.Raw(`DROP TABLE IF EXISTS public.channel_processing_markers;`)
}
