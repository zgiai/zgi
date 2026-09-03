package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

const (
	migration20260902130000ID                 = "20260902130000_create_connector_platform_lifecycle_events"
	createConnectorPlatformLifecycleEventsSQL = `
		CREATE TABLE public.connector_platform_lifecycle_events (
			id uuid PRIMARY KEY DEFAULT public.uuid_generate_v4(),
			subject_type varchar(32) NOT NULL,
			subject_id varchar(128) NOT NULL,
			actor_kind varchar(32) NOT NULL,
			actor_id varchar(128),
			decision varchar(64) NOT NULL,
			status varchar(32) NOT NULL,
			correlation_id varchar(128) NOT NULL,
			details jsonb NOT NULL DEFAULT '{}'::jsonb,
			created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
			CONSTRAINT connector_platform_lifecycle_subject_check
				CHECK (subject_type = 'connector_plugin' AND subject_id ~ '^[a-z0-9][a-z0-9._-]{0,127}$'),
			CONSTRAINT connector_platform_lifecycle_actor_check
				CHECK (
					actor_kind IN ('system', 'platform_maintainer')
					AND (actor_id IS NULL OR (btrim(actor_id) <> '' AND actor_id !~ '[[:space:]]'))
					AND (actor_kind <> 'platform_maintainer' OR actor_id IS NOT NULL)
				),
			CONSTRAINT connector_platform_lifecycle_decision_check
				CHECK (btrim(decision) <> '' AND status = 'successful' AND btrim(correlation_id) <> ''),
			CONSTRAINT connector_platform_lifecycle_details_check
				CHECK (jsonb_typeof(details) = 'object'),
			CONSTRAINT connector_platform_lifecycle_correlation_unique
				UNIQUE (subject_type, subject_id, correlation_id)
		);
		CREATE INDEX idx_connector_platform_lifecycle_subject_created
			ON public.connector_platform_lifecycle_events (subject_type, subject_id, created_at DESC);
		CREATE TRIGGER connector_platform_lifecycle_events_immutable
			BEFORE UPDATE OR DELETE ON public.connector_platform_lifecycle_events
			FOR EACH ROW EXECUTE FUNCTION public.reject_connector_immutable_mutation();
	`
	dropConnectorPlatformLifecycleEventsSQL = `
		DO $$
		BEGIN
			IF EXISTS (SELECT 1 FROM public.connector_platform_lifecycle_events LIMIT 1) THEN
				RAISE EXCEPTION 'cannot remove Connector platform lifecycle audit while events exist';
			END IF;
		END $$;
		DROP TABLE IF EXISTS public.connector_platform_lifecycle_events;
	`
)

func init() {
	registerSchemaMigration(migration20260902130000ID, up20260902130000, down20260902130000)
}

func up20260902130000(schema *mschema.Builder) error {
	return schema.Raw(createConnectorPlatformLifecycleEventsSQL)
}

func down20260902130000(schema *mschema.Builder) error {
	return schema.Raw(dropConnectorPlatformLifecycleEventsSQL)
}
