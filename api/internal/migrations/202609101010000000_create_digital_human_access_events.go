package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

const migrationCreateDigitalHumanAccessEventsID = "202609101010000000_create_digital_human_access_events"

func init() {
	registerSchemaMigration(
		migrationCreateDigitalHumanAccessEventsID,
		upCreateDigitalHumanAccessEvents,
		downCreateDigitalHumanAccessEvents,
	)
}

func upCreateDigitalHumanAccessEvents(schema *mschema.Builder) error {
	return schema.Raw(`
		CREATE TABLE IF NOT EXISTS public.digital_human_access_events (
			id uuid PRIMARY KEY DEFAULT public.uuid_generate_v4(),
			digital_human_id uuid NOT NULL REFERENCES public.digital_humans(id) ON DELETE CASCADE,
			organization_id uuid NOT NULL REFERENCES public.organizations(id) ON DELETE CASCADE,
			workspace_id uuid NOT NULL REFERENCES public.workspaces(id) ON DELETE CASCADE,
			actor_id uuid REFERENCES public.accounts(id) ON DELETE SET NULL,
			action varchar(64) NOT NULL,
			target_type varchar(32) NOT NULL,
			target_id uuid,
			source varchar(32) NOT NULL,
			result varchar(16) NOT NULL,
			details jsonb NOT NULL DEFAULT '{}'::jsonb,
			created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
			CONSTRAINT ck_digital_human_access_event_result CHECK (result IN ('success', 'failure'))
		);
		CREATE INDEX IF NOT EXISTS idx_digital_human_access_events_timeline
			ON public.digital_human_access_events (digital_human_id, created_at DESC, id DESC);
		CREATE INDEX IF NOT EXISTS idx_digital_human_access_events_actor
			ON public.digital_human_access_events (organization_id, actor_id, created_at DESC);
	`)
}

func downCreateDigitalHumanAccessEvents(schema *mschema.Builder) error {
	return schema.DropIfExists("digital_human_access_events")
}
