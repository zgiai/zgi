package migrations

import (
	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
)

// A DigitalHuman task can carry one server watch: a model-free service check
// that starts the task when a watched service goes down (ADR 0031).
func init() {
	registerSchemaMigration("202610281300000000_digital_human_server_watches", upDigitalHumanServerWatches, downDigitalHumanServerWatches)
}

func upDigitalHumanServerWatches(schema *mschema.Builder) error {
	return schema.Raw(`
		CREATE TABLE IF NOT EXISTS public.digital_human_server_watches (
			id uuid PRIMARY KEY,
			organization_id uuid NOT NULL,
			workspace_id uuid NOT NULL,
			digital_human_id uuid NOT NULL,
			task_id uuid NOT NULL,
			owner_account_id uuid NOT NULL,
			connection_id uuid NOT NULL,
			services jsonb NOT NULL DEFAULT '[]',
			interval_seconds integer NOT NULL CHECK (interval_seconds BETWEEN 60 AND 3600),
			failure_threshold integer NOT NULL CHECK (failure_threshold BETWEEN 1 AND 5),
			enabled boolean NOT NULL DEFAULT true,
			state jsonb NOT NULL DEFAULT '{}',
			next_check_at timestamptz NOT NULL,
			last_checked_at timestamptz,
			last_outcome varchar(64) NOT NULL DEFAULT '',
			last_triggered_at timestamptz,
			created_at timestamptz NOT NULL DEFAULT now(),
			updated_at timestamptz NOT NULL DEFAULT now(),
			CONSTRAINT digital_human_server_watches_task_unique UNIQUE (task_id)
		);
		CREATE INDEX IF NOT EXISTS idx_digital_human_server_watches_due ON public.digital_human_server_watches (next_check_at) WHERE enabled;
	`)
}

func downDigitalHumanServerWatches(schema *mschema.Builder) error {
	return schema.Raw(`DROP TABLE IF EXISTS public.digital_human_server_watches`)
}
